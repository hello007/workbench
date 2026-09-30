// service/content_search.go
package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"workbench/model"
	"workbench/util"
)

// 默认排除配置
var defaultExcludeDirs = []string{
	".git", "node_modules", "dist", "build", "target",
	".idea", "__pycache__", ".gradle", "bin", ".settings",
	".vscode", ".cache", "vendor",
}

var defaultExcludeFiles = []string{
	".log", ".tmp", ".class", ".jar", ".war", ".zip", ".tar",
	".gz", ".exe", ".dll", ".so", ".dylib", ".png", ".jpg",
	".jpeg", ".gif", ".ico", ".pdf", ".woff", ".woff2", ".ttf",
	".eot", ".mp3", ".mp4", ".avi", ".mov",
}

// ContentSearchService 内容搜索服务
type ContentSearchService struct {
	rgAvailable bool   // 是否检测到 ripgrep
	rgPath      string // ripgrep 可执行文件路径
}

// NewContentSearchService 创建内容搜索服务
func NewContentSearchService() *ContentSearchService {
	svc := &ContentSearchService{}
	svc.detectRipgrep()
	return svc
}

// detectRipgrep 检测系统是否安装 ripgrep
func (s *ContentSearchService) detectRipgrep() {
	path, err := exec.LookPath("rg")
	if err == nil {
		s.rgAvailable = true
		s.rgPath = path
	}
}

// searchTimeoutMessage 内容搜索超时统一文案（warning 引导级，前端按错误码分流展示）
const searchTimeoutMessage = "内容搜索超时已终止，可缩小范围后重试"

// ContentSearch 执行内容搜索
func (s *ContentSearchService) ContentSearch(
	ctx context.Context,
	dirs []string,
	repoNames []string,
	keyword, fileExt, subDir string,
	excludeDirs, excludeFiles []string,
	maxPerRepo int,
) ([]*model.ContentSearchGroup, error) {
	if keyword == "" {
		return nil, nil
	}
	if len(excludeDirs) == 0 {
		excludeDirs = defaultExcludeDirs
	}
	if len(excludeFiles) == 0 {
		excludeFiles = defaultExcludeFiles
	}

	var (
		mu           sync.Mutex
		groups       []*model.ContentSearchGroup
		timeoutCount int
		wg           sync.WaitGroup
	)

	for i, dir := range dirs {
		wg.Add(1)
		go func(idx int, workDir string) {
			defer wg.Done()
			searchDir := workDir
			if subDir != "" {
				searchDir = filepath.Join(workDir, subDir)
			}

			var items []*model.ContentSearchResult
			var err error
			usedGoFallback := false
			timedOut := false

			if s.rgAvailable {
				items, err = s.searchWithRipgrep(ctx, searchDir, keyword, fileExt, excludeDirs, excludeFiles, maxPerRepo)
				if err != nil {
					if ctx.Err() != nil {
						// rg 超时被杀：已超时 ctx 下回退 searchWithGo 只会立即空返，掩盖超时，禁回退
						timedOut = true
					} else {
						// rg 其他失败（如 exit 2 真错误）：保持降级原生搜索现状
						usedGoFallback = true
						items = s.searchWithGo(ctx, searchDir, keyword, fileExt, excludeDirs, excludeFiles, maxPerRepo)
					}
				}
			} else {
				usedGoFallback = true
				items = s.searchWithGo(ctx, searchDir, keyword, fileExt, excludeDirs, excludeFiles, maxPerRepo)
			}

			// searchWithGo 无错误返回，ctx 被中断时 Walk 中途 SkipAll 留下的部分结果不可信，
			// 与 rg 路径超时语义对称：丢弃并计超时（ctx 未超时时结果完整，不受影响）
			if usedGoFallback && ctx.Err() != nil {
				timedOut = true
			}

			if timedOut {
				mu.Lock()
				timeoutCount++
				mu.Unlock()
				return
			}

			// 补全结果中的仓库信息
			for _, item := range items {
				item.RepoName = repoNames[idx]
				item.RepoPath = workDir
				rel, _ := filepath.Rel(workDir, filepath.Join(searchDir, item.FilePath))
				if rel != "" {
					item.FilePath = rel
				}
			}

			if len(items) > 0 {
				mu.Lock()
				groups = append(groups, &model.ContentSearchGroup{
					RepoName: repoNames[idx],
					RepoPath: workDir,
					Items:    items,
				})
				mu.Unlock()
			}
		}(i, dir)
	}

	wg.Wait()

	// 共享 ctx 一旦超时所有在途目录同时超时：结果为空时上报引导级错误，
	// 已有非空结果时返回结果并告警（Wails error 返回与结果互斥，报错会丢已有结果）
	if timeoutCount > 0 {
		if len(groups) == 0 {
			return nil, model.NewAppError(model.ErrCodeSearchTimeout, searchTimeoutMessage)
		}
		Logger().Warn("content search partially timed out", "timeoutDirs", timeoutCount, "totalDirs", len(dirs))
	}

	return groups, nil
}

// searchWithRipgrep 使用 ripgrep 搜索
func (s *ContentSearchService) searchWithRipgrep(
	ctx context.Context,
	dir, keyword, fileExt string,
	excludeDirs, excludeFiles []string,
	maxResults int,
) ([]*model.ContentSearchResult, error) {
	args := []string{
		"--no-heading",
		"--line-number",
		"--color", "never",
		"--max-count", fmt.Sprintf("%d", maxResults),
		"-F",
		"-e", keyword,
	}

	// 文件类型过滤
	if fileExt != "" {
		ext := strings.TrimPrefix(fileExt, ".")
		args = append(args, "--type-add", fmt.Sprintf("custom:*.%s", ext))
		args = append(args, "-t", "custom")
	}

	// 排除目录
	for _, d := range excludeDirs {
		args = append(args, "--glob", "!"+d)
	}

	// 排除文件（按扩展名）
	for _, f := range excludeFiles {
		args = append(args, "--glob", "!*"+f)
	}

	args = append(args, dir)

	cmd := exec.CommandContext(ctx, s.rgPath, args...)
	util.HideCommandWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// 超时/取消被杀先于 exit code 判定：Windows TerminateProcess exit 1 恰命中
		// 无匹配分支、Unix signal kill exit -1 落入失败分支，两平台均须先判 ctx，
		// 否则超时表现为「搜索无结果」静默假态
		if ctx.Err() != nil {
			return nil, model.WrapAppError(model.ErrCodeSearchTimeout, searchTimeoutMessage, ctx.Err())
		}
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("rg failed: %s", stderr.String())
	}

	var results []*model.ContentSearchResult
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		filePath := parts[0]
		lineNum := 0
		fmt.Sscanf(parts[1], "%d", &lineNum)
		lineText := parts[2]

		results = append(results, &model.ContentSearchResult{
			FilePath: filePath,
			LineNum:  lineNum,
			LineText: lineText,
		})
		if len(results) >= maxResults {
			break
		}
	}

	return results, nil
}

// searchWithGo Go 原生搜索（ripgrep 降级方案）
func (s *ContentSearchService) searchWithGo(
	ctx context.Context,
	dir, keyword, fileExt string,
	excludeDirs, excludeFiles []string,
	maxResults int,
) []*model.ContentSearchResult {
	keywordLower := strings.ToLower(keyword)
	var results []*model.ContentSearchResult

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return filepath.SkipAll
		default:
		}

		if info.IsDir() {
			if isExcludedDir(info.Name(), excludeDirs) {
				return filepath.SkipDir
			}
			return nil
		}

		if fileExt != "" {
			if !strings.HasSuffix(strings.ToLower(info.Name()), strings.ToLower(fileExt)) {
				return nil
			}
		}

		if isExcludedFile(info.Name(), excludeFiles) {
			return nil
		}

		fileResults := searchFileContent(path, keywordLower, info.Size())
		results = append(results, fileResults...)

		if len(results) >= maxResults {
			return filepath.SkipAll
		}

		return nil
	})

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	return results
}

// searchFileContent 搜索单个文件内容
func searchFileContent(path, keywordLower string, fileSize int64) []*model.ContentSearchResult {
	if fileSize > 10*1024*1024 {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	checkSize := len(data)
	if checkSize > 8192 {
		checkSize = 8192
	}
	if bytes.Contains(data[:checkSize], []byte{0}) {
		return nil
	}

	var results []*model.ContentSearchResult
	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		if bytes.Contains(bytes.ToLower(line), []byte(keywordLower)) {
			lineStr := string(line)
			if len(lineStr) > 300 {
				lineStr = lineStr[:300] + "..."
			}
			results = append(results, &model.ContentSearchResult{
				FilePath: path,
				LineNum:  i + 1,
				LineText: lineStr,
			})
		}
	}

	return results
}

// isExcludedDir 检查目录是否在排除列表中
func isExcludedDir(name string, excludeDirs []string) bool {
	for _, d := range excludeDirs {
		if strings.EqualFold(name, d) {
			return true
		}
	}
	return false
}

// isExcludedFile 检查文件是否在排除列表中（按扩展名匹配）
func isExcludedFile(name string, excludeFiles []string) bool {
	nameLower := strings.ToLower(name)
	for _, pattern := range excludeFiles {
		if strings.HasSuffix(nameLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}
