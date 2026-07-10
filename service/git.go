package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// GitService Git服务
type GitService struct {
	gitCmd *util.GitCommand
}

// NewGitService 创建服务
func NewGitService() *GitService {
	return &GitService{
		gitCmd: util.NewGitCommand(),
	}
}

// GetInfo 获取仓库信息
func (s *GitService) GetInfo(dirPath string) (*model.GitRepoInfo, error) {
	info := &model.GitRepoInfo{
		Path:   dirPath,
		IsRepo: s.gitCmd.IsGitRepository(dirPath),
	}

	if !info.IsRepo {
		return info, nil
	}

	branch, err := s.gitCmd.GetBranch(dirPath)
	if err == nil {
		info.Branch = strings.TrimSpace(branch)
	}

	remote, remoteURL, err := s.gitCmd.GetRemote(dirPath)
	if err == nil {
		info.Remote = remote
		info.RemoteURL = remoteURL
	}

	return info, nil
}

// GetLog 获取提交历史（需要补充util/git.go的方法）
func (s *GitService) GetLog(dirPath string, page, pageSize int) (*model.PageResult, error) {
	if !s.gitCmd.IsGitRepository(dirPath) {
		return nil, fmt.Errorf("不是Git仓库")
	}

	// 简化实现，这里假设使用固定逻辑
	commits := []model.GitCommit{}

	// 这里需要调用Git命令获取日志
	// 暂时返回空结果
	return model.NewPageResult(commits, 0, page, pageSize), nil
}

// Clone 克隆仓库
func (s *GitService) Clone(url, targetPath string) (string, error) {
	if _, err := os.Stat(targetPath); err == nil {
		return "", fmt.Errorf("目标路径已存在")
	}

	return s.gitCmd.Clone(url, targetPath)
}

// Pull 拉取更新
func (s *GitService) Pull(dirPath string) (string, error) {
	if !s.gitCmd.IsGitRepository(dirPath) {
		return "", fmt.Errorf("不是Git仓库")
	}

	return s.gitCmd.Pull(dirPath)
}

// HasRemote 检测仓库是否配置了远程仓库（git remote -v 是否非空）。
// 用于一键更新跳过无远程的本地测试仓库，避免 pull 报错。
func (s *GitService) HasRemote(dirPath string) bool {
	_, _, err := s.gitCmd.GetRemote(dirPath)
	return err == nil
}

// ExtractRepoName 提取仓库名
func (s *GitService) ExtractRepoName(url string) string {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "repo"
}

// getLocalBranchNames 获取本地分支名列表
func (s *GitService) getLocalBranchNames(dirPath string) []string {
	output, err := s.gitCmd.Execute(dirPath, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names
}

// GetBranches 获取仓库的分支列表
func (s *GitService) GetBranches(dirPath string) (*model.BranchList, error) {
	if !s.gitCmd.IsGitRepository(dirPath) {
		return nil, fmt.Errorf("不是Git仓库")
	}

	output, err := s.gitCmd.GetBranchesAll(dirPath)
	if err != nil {
		return nil, fmt.Errorf("获取分支列表失败: %w", err)
	}

	var branches []model.BranchInfo
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		isCurrent := strings.HasPrefix(line, "* ")
		if isCurrent {
			line = strings.TrimSpace(line[2:])
		} else {
			line = strings.TrimSpace(strings.TrimPrefix(line, "  "))
		}

		// 过滤 HEAD -> 引用和 detached HEAD
		if strings.Contains(line, "HEAD ->") || strings.Contains(line, "(HEAD detached") {
			continue
		}

		if strings.HasPrefix(line, "remotes/") {
			name := strings.TrimPrefix(line, "remotes/")
			branches = append(branches, model.BranchInfo{
				Name:      name,
				IsRemote:  true,
				IsCurrent: isCurrent,
			})
		} else {
			branches = append(branches, model.BranchInfo{
				Name:      line,
				IsRemote:  false,
				IsCurrent: isCurrent,
			})
		}
	}

	return &model.BranchList{Branches: branches}, nil
}

// CheckoutBranch 切换分支
func (s *GitService) CheckoutBranch(dirPath string, branchName string, isRemote bool) error {
	if !s.gitCmd.IsGitRepository(dirPath) {
		return fmt.Errorf("不是Git仓库")
	}

	hasChanges, err := s.gitCmd.HasLocalChanges(dirPath)
	if err != nil {
		return fmt.Errorf("检查工作区状态失败: %w", err)
	}
	if hasChanges {
		return fmt.Errorf("当前有未提交的变更，请先提交或暂存后再切换分支")
	}

	if isRemote {
		parts := strings.SplitN(branchName, "/", 2)
		localName := branchName
		if len(parts) == 2 {
			localName = parts[1]
		}
		// 如果本地已有同名分支，直接切换；否则从远程创建
		localExists := false
		for _, b := range s.getLocalBranchNames(dirPath) {
			if b == localName {
				localExists = true
				break
			}
		}
		if localExists {
			_, err = s.gitCmd.CheckoutLocal(dirPath, localName)
		} else {
			_, err = s.gitCmd.CheckoutRemote(dirPath, branchName, localName)
		}
		return err
	}

	_, err = s.gitCmd.CheckoutLocal(dirPath, branchName)
	return err
}

// ScanGitRepos 递归扫描目录下所有 Git 仓库
// 如果 rootPath 本身是 git 仓库，直接返回 [rootPath]
// 否则递归遍历子目录，收集所有 git 仓库路径
func (s *GitService) ScanGitRepos(rootPath string) []string {
	if s.gitCmd.IsGitRepository(rootPath) {
		return []string{rootPath}
	}

	var repos []string
	s.scanDir(rootPath, &repos)
	return repos
}

func (s *GitService) scanDir(dir string, repos *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		// 跳过 .git 目录本身
		if entry.Name() == ".git" {
			continue
		}

		if s.gitCmd.IsGitRepository(fullPath) {
			*repos = append(*repos, fullPath)
		} else {
			s.scanDir(fullPath, repos)
		}
	}
}

// GetLocalChanges 获取本地变动文件列表
func (s *GitService) GetLocalChanges(dirPath string) ([]model.FileChange, error) {
	gitRoot, err := util.FindGitRoot(dirPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	// 使用 -z 以 NUL 分隔输出，避免路径引号和八进制转义问题
	// 追加 --untracked-files=all 展开未跟踪目录内的每个文件，
	// 避免 git 默认 --untracked-files=normal 把未跟踪目录折叠为单行 ?? dir/
	// 导致本地变动面板显示不完整。仍尊重 .gitignore，被忽略文件不会出现。
	output, err := s.gitCmd.Execute(gitRoot, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("获取本地变动失败: %w", err)
	}

	if output == "" {
		return []model.FileChange{}, nil
	}

	segments := strings.Split(output, "\x00")
	changes := make([]model.FileChange, 0, len(segments))

	for i := 0; i < len(segments); i++ {
		seg := segments[i]
		if seg == "" || len(seg) < 4 || seg[2] != ' ' {
			continue
		}

		staged := seg[0] != ' ' && seg[0] != '?'
		statusRaw := seg[:2]
		filePath := seg[3:]

		// 重命名/复制：git -z 格式为 "XY <目标路径> NUL <源路径> NUL"，
		// 目标路径已在 seg[3:]，下一段是源路径（仅跳过，不取作 filePath）。
		if (statusRaw[0] == 'R' || statusRaw[0] == 'C') && i+1 < len(segments) && segments[i+1] != "" {
			i++ // 跳过源路径
		}

		// 取工作区状态码
		status := strings.TrimSpace(statusRaw)
		if len(status) == 2 {
			status = string(status[1])
		}

		changes = append(changes, model.FileChange{
			Path:   filePath,
			Status: status,
			Staged: staged,
		})
	}

	return changes, nil
}

// DiscardChanges 回滚本地变动
func (s *GitService) DiscardChanges(dirPath string, filePaths []string) error {
	gitRoot, err := util.FindGitRoot(dirPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	if len(filePaths) == 0 {
		// 回滚全部：从 HEAD 恢复已跟踪文件，再清理未跟踪文件
		if _, err := s.gitCmd.Execute(gitRoot, "checkout", "HEAD", "--", "."); err != nil {
			return fmt.Errorf("回滚失败: %w", err)
		}
		if _, err := s.gitCmd.Execute(gitRoot, "clean", "-fd"); err != nil {
			return fmt.Errorf("清理未跟踪文件失败: %w", err)
		}
		return nil
	}

	// 查询文件状态，区分已跟踪和未跟踪
	changes, err := s.GetLocalChanges(dirPath)
	if err != nil {
		return fmt.Errorf("获取文件状态失败: %w", err)
	}

	untrackedSet := make(map[string]bool)
	for _, c := range changes {
		if c.Status == "?" {
			untrackedSet[c.Path] = true
		}
	}

	var tracked, untracked []string
	for _, p := range filePaths {
		if untrackedSet[p] {
			untracked = append(untracked, p)
		} else {
			tracked = append(tracked, p)
		}
	}

	if len(tracked) > 0 {
		// 从 HEAD 恢复已跟踪文件（同时更新索引和工作区）
		args := append([]string{"checkout", "HEAD", "--"}, tracked...)
		if _, err := s.gitCmd.Execute(gitRoot, args...); err != nil {
			return fmt.Errorf("回滚失败: %w", err)
		}
	}

	if len(untracked) > 0 {
		args := append([]string{"clean", "-fd", "--"}, untracked...)
		if _, err := s.gitCmd.Execute(gitRoot, args...); err != nil {
			return fmt.Errorf("清理未跟踪文件失败: %w", err)
		}
	}

	return nil
}
func safeEmit(ctx context.Context, event string, data ...interface{}) {
	if ctx == nil || ctx.Value("events") == nil {
		return
	}
	runtime.EventsEmit(ctx, event, data...)
}

// Commit 选择性提交：仅提交 files 列表中的文件（pathspec 语义）。
// 先 git add -- <files> 把选中文件（含未跟踪）加入 index，
// 再 git commit -m <message> -- <files>，pathspec 确保不影响 index 中其他文件。
func (s *GitService) Commit(repoPath, message string, files []string) error {
	if len(files) == 0 {
		return fmt.Errorf("未选择要提交的文件")
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("提交信息不能为空")
	}

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	addArgs := append([]string{"add", "--"}, files...)
	if _, err := s.gitCmd.Execute(gitRoot, addArgs...); err != nil {
		return fmt.Errorf("暂存文件失败: %w", err)
	}

	commitArgs := append([]string{"commit", "-m", message, "--"}, files...)
	if _, err := s.gitCmd.Execute(gitRoot, commitArgs...); err != nil {
		return fmt.Errorf("提交失败: %w", err)
	}

	return nil
}

// Push 推送当前分支到远程。setUpstream=true 时使用 git push --set-upstream origin <branch>。
// 返回 git stdout（trim 后）用于结果展示。
func (s *GitService) Push(repoPath string, setUpstream bool) (string, error) {
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	var args []string
	if setUpstream {
		branch, err := s.gitCmd.GetBranch(gitRoot)
		if err != nil {
			return "", fmt.Errorf("获取当前分支失败: %w", err)
		}
		branch = strings.TrimSpace(branch)
		if branch == "" {
			return "", fmt.Errorf("当前处于 detached HEAD，无法 set-upstream")
		}
		args = []string{"push", "--set-upstream", "origin", branch}
	} else {
		args = []string{"push"}
	}

	output, err := s.gitCmd.Execute(gitRoot, args...)
	if err != nil {
		return "", fmt.Errorf("推送失败: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// HasUpstream 判断当前分支是否配置了上游跟踪分支。
// 通过 git rev-parse --abbrev-ref @{u} 判定：成功且输出非空即有上游，失败（无上游）返回 false。
func (s *GitService) HasUpstream(repoPath string) (bool, error) {
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return false, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	output, err := s.gitCmd.Execute(gitRoot, "rev-parse", "--abbrev-ref", "@{u}")
	if err != nil {
		// 无上游时 git 返回非零退出码，stderr 包含 "No upstream" 类信息
		return false, nil
	}
	return strings.TrimSpace(output) != "", nil
}

// GetDiff 获取单个文件的 unified diff 文本。
// 已跟踪文件：git diff HEAD -- <file>（对比 HEAD 与工作区）。
// 未跟踪文件：git diff --no-index /dev/null <file>（展示为新增全文）。
// 无差异时返回空字符串。
func (s *GitService) GetDiff(repoPath, file string) (string, error) {
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	// 判断文件是否未跟踪
	untracked, err := s.isUntracked(gitRoot, file)
	if err != nil {
		return "", err
	}

	if untracked {
		// 未跟踪文件：用 --no-index 与空设备对比生成全量新增 diff
		// git diff --no-index 在有差异时退出码为 1（git 标准行为），需容忍
		devNull := os.DevNull
		output, err := s.gitCmd.ExecuteWithCodes(gitRoot, map[int]bool{1: true}, "diff", "--no-index", devNull, file)
		if err != nil {
			return "", fmt.Errorf("获取差异失败: %w", err)
		}
		return strings.TrimSpace(output), nil
	}

	output, err := s.gitCmd.Execute(gitRoot, "diff", "HEAD", "--", file)
	if err != nil {
		return "", fmt.Errorf("获取差异失败: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// isUntracked 判断 file 是否为未跟踪文件（status 行首为 ??）。
func (s *GitService) isUntracked(gitRoot, file string) (bool, error) {
	output, err := s.gitCmd.Execute(gitRoot, "status", "--porcelain", "-z", "--", file)
	if err != nil {
		return false, fmt.Errorf("获取文件状态失败: %w", err)
	}
	if output == "" {
		// 无输出表示该路径无变动（已提交且工作区干净），不是未跟踪
		return false, nil
	}
	// -z 分隔，第一段形如 "?? path" 或 "M  path" 等
	seg := output
	if idx := strings.Index(output, "\x00"); idx >= 0 {
		seg = output[:idx]
	}
	if len(seg) >= 2 && seg[0] == '?' && seg[1] == '?' {
		return true, nil
	}
	return false, nil
}

// BatchPull 并行拉取多个 Git 仓库
func (s *GitService) BatchPull(repos []string, concurrency int, ctx context.Context) []model.PullResult {
	if concurrency <= 0 {
		concurrency = 5
	}

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		results      []model.PullResult
		sem          = make(chan struct{}, concurrency)
		successCount int
		failCount    int
		skippedCount int
	)

	for _, repo := range repos {
		wg.Add(1)
		go func(repoPath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			name := filepath.Base(repoPath)
			result := model.PullResult{
				Path: repoPath,
				Name: name,
			}

			if !s.gitCmd.IsGitRepository(repoPath) {
				result.Success = false
				result.Error = "不是 Git 仓库"
			} else if !s.HasRemote(repoPath) {
				// 无远程配置的本地仓库无法 pull，跳过而非报错
				result.Skipped = true
				result.Output = "未配置远程仓库，已跳过"
			} else {
				gitCmd := util.NewGitCommandWithTimeout(5 * time.Minute)
				output, err := gitCmd.Pull(repoPath)
				if err != nil {
					result.Success = false
					result.Error = err.Error()
				} else {
					result.Success = true
					result.Output = strings.TrimSpace(output)
				}
			}

			mu.Lock()
			results = append(results, result)
			if result.Skipped {
				skippedCount++
			} else if result.Success {
				successCount++
			} else {
				failCount++
			}
			mu.Unlock()

			safeEmit(ctx, "pull-progress", result)
		}(repo)
	}

	wg.Wait()

	safeEmit(ctx, "pull-complete", map[string]int{
		"success": successCount,
		"skipped": skippedCount,
		"failed":  failCount,
	})

	return results
}
