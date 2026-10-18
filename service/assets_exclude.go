package service

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// 素材库扫描排除规则（SRS §2.7.3 索引配置，素材管理第十批 PR1）。
//
// 匹配语义定稿见 .trellis/tasks/10-18-srs-2-7-3/research/exclude-rules-semantics.md：
//   - ExcludeDirs 每项一个 glob 模式（filepath.Match 单段语法 * ? [...]），对相对
//     库根路径的每一层目录段名做完整匹配（等价 gitignore「无斜杠模式」语义），
//     大小写不敏感（模式与段名各自 ToLower 后匹配，对齐 Windows FS 大小写不敏感
//     与仓库内 EqualFold 先例）；不支持「/」与「**」多段路径模式（四个对标工具
//     Billfish/Eagle/Bridge/Everything 均无多段路径 glob 排除语法，标准库即足额
//     零新增依赖）；库根目录自身豁免匹配（root 参与匹配会把整库剪空）。
//   - ExcludeExts 黑名单（忽略后缀），存储归一带点小写（.tmp），输入宽容
//     tmp / .tmp / *.tmp 三形态等价；匹配 strings.EqualFold(path.Ext(p), ext)
//     与既有 .pptx 判断同构。
//
// 两条链路（PRD 实现注意）：收集层磁盘过滤（collectPptxEntries 的 WalkDir 剪枝
// + 后缀过滤）与 excluded 判定（「规则 × 存量索引 path」独立链路，不依赖本次
// 磁盘扫描是否发现该文件）共用同一 matcher，禁两套口径。

// assetExcludeMatcher 排除规则编译结果（扫描入口构建一次，WalkDir 回调闭包捕获
// 复用，禁每文件重编译/重复 ToLower）。零值/nil 安全：全部匹配方法对空规则恒
// 不命中。
type assetExcludeMatcher struct {
	dirPatterns []string        // 目录段 glob 模式（预 ToLower）
	exts        map[string]bool // 带点小写后缀集合（如 ".tmp"）
}

// newAssetExcludeMatcher 从库配置编译排除规则（registry 读取的原始值，可能含
// 手改 libraries.json 绕过保存校验的坏规则）：空白项丢弃；目录模式坏语法
// （filepath.Match ErrBadPattern）防御性剔除并告警一次（保存 API 校验为第一道
// 防线，本剔除防手改坏规则致「模式永不命中」或整库行为异常；剔除后该规则不
// 生效而非扫描失败）。
func newAssetExcludeMatcher(dirs, exts []string) *assetExcludeMatcher {
	m := &assetExcludeMatcher{}
	for _, d := range dirs {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if _, err := filepath.Match(d, ""); err != nil {
			Logger().Warn("素材库排除目录规则非法，已忽略该规则", "pattern", d, "err", err)
			continue
		}
		m.dirPatterns = append(m.dirPatterns, d)
	}
	for _, e := range exts {
		e = normalizeAssetExcludeExt(e)
		if e == "" {
			continue
		}
		if m.exts == nil {
			m.exts = make(map[string]bool)
		}
		m.exts[e] = true
	}
	return m
}

// active 是否配置了任一规则（空规则时扫描侧零行为变化快速路径）。
func (m *assetExcludeMatcher) active() bool {
	return m != nil && (len(m.dirPatterns) > 0 || len(m.exts) > 0)
}

// matchDir 目录段名匹配（WalkDir 目录剪枝与存量路径逐段判定共用）：双侧归一
// 小写后 filepath.Match 完整匹配。Match 语法错误防御性不命中（编译期已剔除
// 坏模式，此处双保险——手改注入的坏规则不得使扫描报错）。nil receiver 安全。
func (m *assetExcludeMatcher) matchDir(name string) bool {
	if m == nil || len(m.dirPatterns) == 0 {
		return false
	}
	lower := strings.ToLower(name)
	for _, pat := range m.dirPatterns {
		ok, err := filepath.Match(pat, lower)
		if err != nil {
			continue
		}
		if ok {
			return true
		}
	}
	return false
}

// matchExt 文件后缀匹配（集合存储已归一带点小写，比较侧 ToLower 容忍任意输入
// 大小写）。空后缀（无扩展名文件）与裸「.」不命中。nil receiver 安全。
func (m *assetExcludeMatcher) matchExt(ext string) bool {
	if m == nil || len(m.exts) == 0 || ext == "" || ext == "." {
		return false
	}
	return m.exts[strings.ToLower(ext)]
}

// matchPath 存量索引路径判定（「规则 × 存量索引 path」独立链路）：相对路径
// （正斜杠）按段拆分，任一目录段命中目录规则或文件后缀命中后缀规则即命中。
// 文件基名不参与目录规则匹配（目录规则只对目录段）；库内相对路径天然不含库根
// 自身段名，root 豁免无需特判。
func (m *assetExcludeMatcher) matchPath(relSlash string) bool {
	if !m.active() {
		return false
	}
	segments := strings.Split(relSlash, "/")
	for _, seg := range segments[:len(segments)-1] {
		if m.matchDir(seg) {
			return true
		}
	}
	// 正斜杠路径统一用 path.Ext（与 isPptxSplitDerivative 对 slash 路径的取段
	// 口径一致；filepath.Ext 在 Windows 对「/」的分隔语义依赖平台分支，不取）
	return m.matchExt(path.Ext(relSlash))
}

// normalizeAssetExcludeExt 后缀规则归一：TrimSpace → 剥「*.」前缀 → 剥前导点
// → 补前导点 → ToLower。三输入形态 tmp / .tmp / *.tmp 归一为同一存储值 .tmp；
// 空白与归一后为空（如输入「.」）返回空串（调用方丢弃）。
func normalizeAssetExcludeExt(raw string) string {
	e := strings.TrimSpace(raw)
	if e == "" {
		return ""
	}
	e = strings.TrimPrefix(e, "*.")
	e = strings.TrimPrefix(e, ".")
	if e == "" {
		return ""
	}
	return "." + strings.ToLower(e)
}

// normalizeAssetExcludeRules 库规则保存前的校验与归一（UpdateLibraryRules 入口，
// 第一道防线）：目录模式 TrimSpace、空白项丢弃、含路径分隔符（「/」与「\」——
// 单段语义下永不命中，拒绝防用户误配静默失效）与坏 glob 语法（filepath.Match
// ErrBadPattern 探测）拒绝；后缀三形态归一。返回归一后的两规则列表（可能为空
// 切片，整体替换语义）。
func normalizeAssetExcludeRules(dirs, exts []string) ([]string, []string, error) {
	outDirs := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if strings.ContainsRune(d, '/') || strings.ContainsRune(d, '\\') {
			return nil, nil, fmt.Errorf("排除文件夹规则 %q 非法：按文件夹名匹配（支持 * ? 通配），不含路径分隔符", d)
		}
		if _, err := filepath.Match(d, ""); err != nil {
			return nil, nil, fmt.Errorf("排除文件夹规则 %q 非法：%w", d, err)
		}
		outDirs = append(outDirs, d)
	}
	outExts := make([]string, 0, len(exts))
	for _, e := range exts {
		e = normalizeAssetExcludeExt(e)
		if e == "" {
			continue
		}
		outExts = append(outExts, e)
	}
	return outDirs, outExts, nil
}
