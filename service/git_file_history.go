package service

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"workbench/model"
	"workbench/util"
)

// 文件级提交历史域 GitService 方法（git log --follow 单文件演进史）。
//
// 入参 filePath 为文件绝对路径：内部 FindGitRoot 定位仓库根并换算仓库相对路径，
// 单参数自包含、无「相对哪一级目录」的口径歧义（文件树右键直接传节点绝对路径）。
//
// 编码：提交消息与文件名可含中文，git 调用统一前置 UTF-8 前缀（utf8Prefix，
// docs/spec/branch-compare.md）与 core.quotePath=off（原始非转义路径）。
//
// 重命名追溯：--follow 让历史跨重命名延续（旧段提交按旧路径呈现）；--name-status
// 供 rename 检出（R/C 行 old→new），PathAtCommit 随历史逐段变化供 diff 查看定位。
//
// pathspec 用 :(literal) 魔术前缀：文件名可含 glob 元字符（如「笔记[1].md」），
// 缺省 glob 语义会将其解释为模式导致匹配不到；literal 语义按字节精确匹配。
//
// 分页：-n offset+limit 批量读取后内存窗口截取（文件级历史通常数十至数百条，
// --follow 无法廉价取总数，前端以「返回满 limit 条即有下一页」判定 hasMore，
// 与仓库级 CommitHistory 分页语义一致）。

// fileHistoryMaxFetch 单次读取条数上限（offset+limit 的钳制值），防疯批 offset
// 撑爆子进程输出缓冲；超过即视为翻到历史尽头。
const fileHistoryMaxFetch = 10000

// fileHistoryFormat 文件级历史输出格式：\x00 记录分隔、\x01 字段分隔，
// 字段序 SHA/author/email/authorUnixTs/authorDateISO/rawMessage（无 parents——
// 文件级历史无消费方）。记录结构：
//
//	\0<SHA>\x01<author>\x01<email>\x01<ts>\x01<ai>\x01<rawMessage>\x01\n\n<statusLines>
//
// rawMessage 为 %B（含尾部换行），其后 \x01 + \n\n 即消息终止锚点，与
// commit_history_cli.go 的仓库级格式同构（解析可复用同一反向定位策略）。
const fileHistoryFormat = "%x00%H%x01%an%x01%ae%x01%at%x01%ai%x01%B%x01"

// GetFileCommitHistory 获取单文件提交历史（--follow 跨重命名追溯，倒序）。
// fileAbsPath 为文件绝对路径；limit<=0 默认 20，offset<0 归零。返回 [offset,
// offset+limit) 窗口；起始 offset 超出历史长度返回空切片（非 nil，前端零值稳定）。
func (s *GitService) GetFileCommitHistory(fileAbsPath string, limit, offset int) ([]model.FileCommit, error) {
	if fileAbsPath == "" {
		return nil, fmt.Errorf("文件路径不能为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	gitRoot, err := util.FindGitRoot(fileAbsPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	rel, err := filepath.Rel(gitRoot, fileAbsPath)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("文件不在 Git 仓库内: %s", fileAbsPath)
	}

	count := offset + limit
	if count > fileHistoryMaxFetch {
		count = fileHistoryMaxFetch
	}

	// :(literal) 禁用 pathspec glob 解释（文件名可含 [ ] * 等元字符）；路径归一
	// 正斜杠（Windows 下 filepath.Rel 返回反斜杠，git pathspec 按正斜杠匹配）
	args := append(utf8Prefix(),
		"log", "--follow", "--name-status",
		"--format="+fileHistoryFormat,
		"-n", strconv.Itoa(count),
		"--", ":(literal)"+filepath.ToSlash(rel))
	out, err := s.gitCmd.Execute(gitRoot, args...)
	if err != nil {
		return nil, fmt.Errorf("获取文件提交历史失败: %w", err)
	}

	commits := parseFileCommitHistory(out)
	if offset >= len(commits) {
		return []model.FileCommit{}, nil
	}
	end := offset + limit
	if end > len(commits) {
		end = len(commits)
	}
	return commits[offset:end], nil
}

// parseFileCommitHistory 解析 git log --follow --name-status 输出（\x00 记录分隔）
// 为 FileCommit 切片。单条解析失败跳过并 Warn（列表展示场景，脏记录不阻断整体，
// 对齐 parseNameStatus 畸形行跳过先例）。
func parseFileCommitHistory(out string) []model.FileCommit {
	commits := []model.FileCommit{}
	for _, rec := range strings.Split(out, "\x00") {
		rec = strings.Trim(rec, "\x00")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		c, err := parseFileCommitChunk(rec)
		if err != nil {
			Logger().Warn("文件提交历史单条记录解析失败，跳过", "err", err, "rec", truncateNumstatForLog(rec))
			continue
		}
		commits = append(commits, *c)
	}
	return commits
}

// parseFileCommitChunk 解析单条记录：头部 5 字段（\x01 分隔）+ 消息（终止锚点
// "\x01\n\n" 反向定位，消化消息内 0x01/换行）+ 状态行段。
func parseFileCommitChunk(chunk string) (*model.FileCommit, error) {
	// 定位第 5 个 \x01（其前为 SHA/author/email/ts/ai 五字段，其后为消息区；
	// 5 字段间共 4 个分隔符，第 5 个为消息与其起始边界间的分隔符）
	headerEnd := -1
	seps := 0
	for i := 0; i < len(chunk); i++ {
		if chunk[i] == '\x01' {
			seps++
			if seps == 5 {
				headerEnd = i
				break
			}
		}
	}
	if seps < 5 {
		return nil, fmt.Errorf("记录头部字段不完整: %s", truncateNumstatForLog(chunk))
	}
	header := strings.SplitN(chunk[:headerEnd], "\x01", 5)
	if !isHexSHA40(header[0]) {
		return nil, fmt.Errorf("记录 SHA 非法（字段可能错位）: %s", truncateNumstatForLog(header[0]))
	}
	ts, err := strconv.ParseInt(header[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("提交时间戳解析失败: %q", header[3])
	}

	// 消息终止符反向定位（%B 尾部换行 + "\x01\n\n" 锚点，其后为状态行段）。
	// 消息内可含 "\x01\n\n" 序列，取 LastIndex 以记录尾部真实锚点为准。
	term := strings.LastIndex(chunk, "\x01\n\n")
	if term < headerEnd {
		return nil, fmt.Errorf("消息终止符缺失: %s", truncateNumstatForLog(chunk))
	}
	pathAt, renameFrom, renameTo := parseFileStatusLines(chunk[term+len("\x01\n\n"):])

	return &model.FileCommit{
		SHA:          header[0],
		ShortSHA:     shortFileSHA(header[0]),
		Message:      chunk[headerEnd+1 : term],
		Author:       header[1],
		Email:        header[2],
		Timestamp:    ts,
		DateTime:     fileHistoryDateTime(header[4]),
		PathAtCommit: pathAt,
		RenameFrom:   renameFrom,
		RenameTo:     renameTo,
	}, nil
}

// parseFileStatusLines 解析记录尾部的 --name-status 状态行段：
// 普通行 "<status>\t<path>"（A/D/M/T），重命名/复制行 "<status><score>\t<old>\t<new>"。
// 取第一条有效行作为目标文件状态（--follow 单路径语义下至多一行；防御多行时取首行）。
// 无有效行返回空串路径（空提交等罕见形态，不视为错误）。
func parseFileStatusLines(raw string) (pathAt, renameFrom, renameTo string) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if parts[0] == "" {
			continue
		}
		switch letter := parts[0][0]; letter {
		case 'R', 'C':
			if len(parts) >= 3 {
				return parts[2], parts[1], parts[2]
			}
		default:
			if len(parts) >= 2 {
				return parts[1], "", ""
			}
		}
	}
	return "", "", ""
}

// shortFileSHA 取前 8 位短 SHA，异常短时原样返回。
func shortFileSHA(sha string) string {
	if len(sha) >= 8 {
		return sha[:8]
	}
	return sha
}

// fileHistoryDateTime 取 %ai（如 "2020-01-01 12:00:00 +0500"）首 19 字节作者时区
// 墙钟，与仓库级 Commit.DateTime 口径一致（非本地时区，避免转换漂移）。
func fileHistoryDateTime(ai string) string {
	if len(ai) >= 19 {
		return ai[:19]
	}
	return ai
}
