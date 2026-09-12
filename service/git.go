package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"workbench/model"
	"workbench/util"
)

// ErrOperationInProgress 表示目标仓库已有变更类 Git 操作正在进行，本次请求被拒绝。
// 前端据此错误码统一弹 warning 提示用户稍后重试，而非当作普通失败。
// 类型为 *model.AppError，经 Wails ErrorFormatter 结构化传前端 {code, message}，
// 前端按 code=E_GIT_IN_PROGRESS 分流；后端内部 errors.As 仍可识别。
var ErrOperationInProgress = model.NewAppError(model.ErrCodeGitInProgress, "该仓库有 Git 操作进行中，请稍后重试")

// IsOperationInProgressError 判断错误是否为操作进行中拒绝。
// 优先 errors.As 识别 *AppError 的 Code；兜底字符串匹配兼容未迁移路径。
func IsOperationInProgressError(err error) bool {
	if err == nil {
		return false
	}
	var appErr *model.AppError
	if errors.As(err, &appErr) {
		return appErr.Code == model.ErrCodeGitInProgress
	}
	return strings.Contains(err.Error(), ErrOperationInProgress.Message)
}

// GitService Git服务
type GitService struct {
	gitCmd    *util.GitCommand
	scanCache *ScanCacheManager // 可为 nil：未注入时走纯 .git 预筛路径（兼容旧调用方与测试）

	opMu     sync.Mutex              // 保护 opLocks map 的并发读写
	opLocks  map[string]*sync.Mutex  // 仓库路径 -> 该仓变更操作互斥锁（懒创建）
}

// NewGitService 创建服务（不注入扫描缓存，兼容现有调用方与测试）。
func NewGitService() *GitService {
	return &GitService{
		gitCmd: util.NewGitCommand(),
	}
}

// NewGitServiceWithCache 创建服务并注入扫描缓存管理器，启用 .git 预筛 + mtime 缓存优化。
// cachePath 为缓存文件路径（如 data/repo_scan_cache.json）。
func NewGitServiceWithCache(cachePath string) *GitService {
	return &GitService{
		gitCmd:    util.NewGitCommand(),
		scanCache: NewScanCacheManager(cachePath),
	}
}

// tryLockRepo 尝试获取目标仓库的变更操作互斥锁。
// 锁粒度按仓库绝对路径为键（A1 纯仓库锁）：同一仓库的变更操作串行化，不同仓库互不阻塞。
// 采用 TryLock 语义（方案 A 互斥拒绝）：锁已被占用立即返回 ErrOperationInProgress，不等待、不排队。
// 成功返回 release 闭包，调用方须 defer 调用以释放锁，保证 panic 路径下也无泄漏。
// repoPath 经 filepath.Abs 规范化为绝对路径作为键，避免相对路径与绝对路径双键绕过互斥。
func (s *GitService) tryLockRepo(repoPath string) (release func(), err error) {
	abs, aerr := filepath.Abs(repoPath)
	if aerr != nil {
		// Abs 失败极少见（路径非法），退回用原路径作键，不阻断主流程
		abs = repoPath
	}

	s.opMu.Lock()
	mu, ok := s.opLocks[abs]
	if !ok {
		mu = &sync.Mutex{}
		if s.opLocks == nil {
			s.opLocks = make(map[string]*sync.Mutex)
		}
		s.opLocks[abs] = mu
	}
	s.opMu.Unlock()

	if !mu.TryLock() {
		return nil, model.WrapAppError(model.ErrCodeGitInProgress, "该仓库有 Git 操作进行中，请稍后重试", ErrOperationInProgress)
	}

	return mu.Unlock, nil
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

// Clone 克隆仓库
func (s *GitService) Clone(url, targetPath string) (string, error) {
	release, err := s.tryLockRepo(targetPath)
	if err != nil {
		return "", err
	}
	defer release()

	if _, err := os.Stat(targetPath); err == nil {
		return "", fmt.Errorf("目标路径已存在")
	}

	return s.gitCmd.Clone(url, targetPath)
}

// Pull 拉取更新。useRebase=true 走 git pull --rebase（变基模式，冲突走统一冲突解决入口），
// useRebase=false 走普通 pull，与历史行为完全一致。
func (s *GitService) Pull(dirPath string, useRebase bool) (string, error) {
	release, err := s.tryLockRepo(dirPath)
	if err != nil {
		return "", err
	}
	defer release()

	if !s.gitCmd.IsGitRepository(dirPath) {
		return "", fmt.Errorf("不是Git仓库")
	}
	if useRebase {
		return s.gitCmd.PullRebase(dirPath)
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
	release, err := s.tryLockRepo(dirPath)
	if err != nil {
		return err
	}
	defer release()

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

// ScanGitRepos 递归扫描目录下所有 Git 仓库。
// 如果 rootPath 本身是 git 仓库，直接返回 [rootPath]；
// 否则递归遍历子目录，收集所有 git 仓库路径。
//
// 优化（PRD F12）：用 .git 存在性预筛（util.IsGitRepositoryFast，os.Stat 不要求 IsDir，
// 覆盖 worktree/submodule）替代逐目录 fork git rev-parse，降低 90%+ 子进程开销。
// 若注入了 scanCache，叠加 mtime 缓存差量：二次扫描近乎瞬时（TTL 5min + 手动刷新兜底）。
// 签名保持 []string 兼容现有调用方（ScanAndPullRepos 等）。
func (s *GitService) ScanGitRepos(rootPath string) []string {
	if util.IsGitRepositoryFast(rootPath) {
		return []string{rootPath}
	}

	// 注入了缓存则走差量扫描路径
	if s.scanCache != nil {
		return s.scanGitReposCached(rootPath)
	}

	var repos []string
	s.scanDir(rootPath, &repos)
	return repos
}

// scanDir 递归扫描子目录，用 .git 预筛判定仓库（不 fork git 子进程）。
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

		if util.IsGitRepositoryFast(fullPath) {
			*repos = append(*repos, fullPath)
		} else {
			s.scanDir(fullPath, repos)
		}
	}
}

// scanGitReposCached 带 mtime 缓存的递归扫描。
// 策略（激进跳过子树 + TTL + 手动刷新兜底，参考 research/git-scan-optimization.md 方案 B）：
//   - 缓存命中且目录 mtime 未变：沿用缓存结论（仓库则收录，非仓库则跳过整棵子树）
//   - 缓存未命中或 mtime 变化：实际判定 + 递归子目录
//
// 并发安全：整次「扫描 + 落盘」在 scanCache.mu 互斥锁保护下串行执行，
// 规避 Entries map 并发读写竞态（并发扫描同一 rootPath、或跨 rootPath 扫描与
// 落盘序列化交错均可触发 map 并发读写 panic）。
//
// 已知风险：深层新增仓库（如 a/b/c 下 git init）若未更新 a/b 的 mtime，激进跳过会漏扫。
// 由 TTL（5min 强制全扫）+ 手动刷新按钮（ClearScanCache，PRD F9）兜底，不丢数据。
func (s *GitService) scanGitReposCached(rootPath string) []string {
	s.scanCache.mu.Lock()
	defer s.scanCache.mu.Unlock()

	cache := s.scanCache.getCacheLocked(rootPath)
	// TTL 过期则清空 entries 强制全扫
	if cache.ttlExpired() {
		cache.Entries = make(map[string]CacheEntry)
	}

	var repos []string
	s.scanDirCached(rootPath, &repos, cache)

	cache.ScannedAt = time.Now()
	s.scanCache.saveLocked() // 持锁落盘，失败静默降级，不阻塞返回
	return repos
}

// scanDirCached 单目录的缓存差量扫描。返回该目录子树下的所有仓库路径（含自身，若为仓库），
// 同时追加到顶层 repos 累加器。
//
// 策略（激进跳过子树 + TTL + 手动刷新兜底，参考 research/git-scan-optimization.md 方案 B）：
//   - 缓存命中且目录 mtime 未变：直接复用缓存的 SubtreeRepos，跳过整棵子树
//   - 缓存未命中或 mtime 变化：实际判定 + 递归子目录，结果回写缓存
//
// 已知风险：深层新增仓库（如 a/b/c 下 git init）若未更新 a/b 的 mtime，激进跳过会漏扫。
// 由 TTL（5min 强制全扫）+ 手动刷新按钮（ClearScanCache，PRD F9）兜底，不丢数据。
func (s *GitService) scanDirCached(dir string, repos *[]string, cache *RepoScanCache) []string {
	info, err := os.Stat(dir)
	if err != nil {
		return nil
	}
	curMtime := info.ModTime()

	// 缓存命中：mtime 未变 -> 复用缓存的子树仓库列表，跳过整棵子树
	if entry, hit := cache.Entries[dir]; hit && entry.ModTime.Equal(curMtime) {
		*repos = append(*repos, entry.SubtreeRepos...)
		return entry.SubtreeRepos
	}

	// 缓存未命中或 mtime 变化：实际判定本目录是否仓库
	isRepo := util.IsGitRepositoryFast(dir)
	if isRepo {
		subtree := []string{dir}
		*repos = append(*repos, dir)
		cache.Entries[dir] = CacheEntry{ModTime: curMtime, IsRepo: true, SubtreeRepos: subtree}
		return subtree
	}

	// 非仓库：递归子目录，聚合子树仓库
	var subtree []string
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == ".git" {
				continue
			}
			childSubtree := s.scanDirCached(filepath.Join(dir, entry.Name()), repos, cache)
			subtree = append(subtree, childSubtree...)
		}
	}
	cache.Entries[dir] = CacheEntry{ModTime: curMtime, IsRepo: false, SubtreeRepos: subtree}
	return subtree
}

// ClearScanCache 清除指定工作目录的扫描缓存，供手动刷新按钮（PRD F9）绕过缓存强制全扫。
// 未注入缓存时为空操作。
func (s *GitService) ClearScanCache(rootPath string) {
	if s.scanCache == nil {
		return
	}
	s.scanCache.clear(rootPath)
}

// HasRemotesBatch 批量检测多个仓库是否配置了远程仓库。
// 用 go-git（读取 .git/config）实现，不 fork git 子进程，规避逐个 git remote -v 的子进程开销。
// 并发执行（concurrency=8），用于仓库筛选器列表场景，保证 NF1（100 仓库 < 3s）。
// 返回 map[路径]是否配置远程；判定失败（路径不存在/非仓库）记为 false。
func (s *GitService) HasRemotesBatch(repoPaths []string) map[string]bool {
	result := make(map[string]bool, len(repoPaths))
	if len(repoPaths) == 0 {
		return result
	}

	const concurrency = 8
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, p := range repoPaths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			has := false
			if repo, err := git.PlainOpen(path); err == nil {
				if remotes, err := repo.Remotes(); err == nil {
					has = len(remotes) > 0
				}
			}
			mu.Lock()
			result[path] = has
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return result
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
	release, err := s.tryLockRepo(dirPath)
	if err != nil {
		return err
	}
	defer release()

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
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

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
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

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

// emptyTreeSHA 为 Git 通用空树对象哈希，用作 root commit（无 parent）的对比基准，
// 使首条提交的文件改动呈现为全增 diff。该哈希为 Git 内置常量，非仓库相关，跨仓库稳定。
const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// hasParent 判断给定 SHA 是否存在父提交。
// `git rev-parse <sha>^` 在 root commit 上以非零退出码失败并输出错误到 stderr，
// 据此区分 root commit（无 parent）与普通提交。
func (s *GitService) hasParent(gitRoot, sha string) bool {
	_, err := s.gitCmd.Execute(gitRoot, "rev-parse", sha+"^")
	return err == nil
}

// GetCommitFileDiff 获取指定提交中单个文件相对其父提交的 unified diff 文本。
//   - 普通提交：`git diff <sha>^ <sha> -- <file>`
//   - root commit（无 parent）：对比空树，`git diff <emptyTree> <sha> -- <file>`，
//     文件改动呈现为全增
//   - 返回 unified diff 文本，空串表示无差异或二进制文件（前端按 Binary files 兜底提示）
//
// file 为空时返回全 commit diff（不限定 pathspec），供需要整提交概览的调用方使用。
func (s *GitService) GetCommitFileDiff(repoPath, sha, file string) (string, error) {
	if sha == "" {
		return "", fmt.Errorf("提交 SHA 不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	args := []string{"diff"}
	if s.hasParent(gitRoot, sha) {
		args = append(args, sha+"^", sha)
	} else {
		// root commit：对比空树，使文件改动呈现为全增
		args = append(args, emptyTreeSHA, sha)
	}
	if file != "" {
		args = append(args, "--", file)
	}

	output, err := s.gitCmd.Execute(gitRoot, args...)
	if err != nil {
		return "", fmt.Errorf("获取提交差异失败: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// GetRangeDiff 获取两个提交之间的 unified diff 文本（全文件，不限定 pathspec）。
// `git diff <baseSHA> <headSHA>`，base 到 head 的变更方向。
// 返回多文件 unified diff，前端按 `diff --git a/ b/` 头拆分文件分组展示。
func (s *GitService) GetRangeDiff(repoPath, baseSHA, headSHA string) (string, error) {
	if baseSHA == "" || headSHA == "" {
		return "", fmt.Errorf("提交 SHA 不能为空")
	}
	if baseSHA == headSHA {
		return "", nil
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	output, err := s.gitCmd.Execute(gitRoot, "diff", baseSHA, headSHA)
	if err != nil {
		return "", fmt.Errorf("获取区间差异失败: %w", err)
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
				// 抢仓级锁，防止与用户手动单仓 PullRepo 并发冲突；抢失败记为失败而非跳过
				release, lockErr := s.tryLockRepo(repoPath)
				if lockErr != nil {
					result.Success = false
					result.Error = lockErr.Error()
				} else {
					defer release()
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

// ===== 标签管理 =====

// ListTags 列出仓库所有标签。
// 用 git for-each-ref 以 Tab 分隔输出 name/objecttype/objectname/*objectname/taggername/taggerdate/contents:subject。
// objecttype=="tag" 为注释标签（sha 取 *objectname 即指向的提交，含 tagger/message），
// 否则为轻量标签（sha 取 objectname 即提交本身，无 tagger/message）。
func (s *GitService) ListTags(repoPath string) ([]model.GitTag, error) {
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	// taggerdate 取 :relative（相对时间）；:format-relative 非合法 git 语法。
	const format = "%(refname:short)%09%(objecttype)%09%(objectname)%09%(*objectname)%09%(taggername)%09%(taggerdate:relative)%09%(contents:subject)"
	output, err := s.gitCmd.Execute(gitRoot, "for-each-ref", "--format="+format, "refs/tags")
	if err != nil {
		return nil, fmt.Errorf("获取标签列表失败: %w", err)
	}

	var tags []model.GitTag
	for _, line := range strings.Split(output, "\n") {
		// 仅去行尾 \r（Windows CRLF），不去 TrimSpace 以保留尾部的空字段 Tab 分隔
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		get := func(i int) string {
			if i < len(parts) {
				return parts[i]
			}
			return ""
		}

		tag := model.GitTag{Name: get(0)}
		if get(1) == "tag" {
			// 注释标签：sha 取 *objectname（指向的提交）
			tag.Type = "annotated"
			tag.Sha = get(3)
			tag.Tagger = get(4)
			tag.Date = get(5)
			tag.Message = get(6)
		} else {
			// 轻量标签：sha 取 objectname（提交本身），无 tagger/message
			tag.Type = "lightweight"
			tag.Sha = get(2)
		}
		if len(tag.Sha) > 8 {
			tag.ShortSha = tag.Sha[:8]
		} else {
			tag.ShortSha = tag.Sha
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// CreateTag 创建标签。message 为空创建轻量标签，非空创建注释标签（-a -m），仅钉 HEAD。
func (s *GitService) CreateTag(repoPath, name, message string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("标签名不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	var args []string
	if strings.TrimSpace(message) == "" {
		args = []string{"tag", name}
	} else {
		args = []string{"tag", "-a", "-m", message, name}
	}
	if _, err := s.gitCmd.Execute(gitRoot, args...); err != nil {
		return fmt.Errorf("创建标签失败: %w", err)
	}
	return nil
}

// DeleteTag 删除本地标签（git tag -d）。
func (s *GitService) DeleteTag(repoPath, name string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("标签名不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	if _, err := s.gitCmd.Execute(gitRoot, "tag", "-d", name); err != nil {
		return fmt.Errorf("删除标签失败: %w", err)
	}
	return nil
}

// PushTag 推送单个标签到远程 origin，返回 trim 后的 stdout。
func (s *GitService) PushTag(repoPath, name string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("标签名不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	output, err := s.gitCmd.Execute(gitRoot, "push", "origin", name)
	if err != nil {
		return "", fmt.Errorf("推送标签失败: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// ===== 远程仓库管理 =====

// ListRemotes 列出远程仓库（名称 + URL），每个 remote 取首个 URL（去重，保持顺序）。
func (s *GitService) ListRemotes(repoPath string) ([]model.GitRemote, error) {
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	output, err := s.gitCmd.Execute(gitRoot, "remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("获取远程列表失败: %w", err)
	}

	seen := make(map[string]bool)
	var remotes []model.GitRemote
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// git remote -v 每行格式: "<name>\t<url> (fetch|push)"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		if seen[name] {
			continue
		}
		seen[name] = true
		remotes = append(remotes, model.GitRemote{Name: name, URL: parts[1]})
	}
	return remotes, nil
}

// AddRemote 新增远程仓库（git remote add）。
func (s *GitService) AddRemote(repoPath, name, url string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("远程仓库名不能为空")
	}
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("远程仓库地址不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	if _, err := s.gitCmd.Execute(gitRoot, "remote", "add", name, url); err != nil {
		return fmt.Errorf("添加远程仓库失败: %w", err)
	}
	return nil
}

// RemoveRemote 删除远程仓库（git remote remove）。
func (s *GitService) RemoveRemote(repoPath, name string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("远程仓库名不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	if _, err := s.gitCmd.Execute(gitRoot, "remote", "remove", name); err != nil {
		return fmt.Errorf("删除远程仓库失败: %w", err)
	}
	return nil
}

// Fetch 拉取远程更新。remote 为空时对所有远程执行；prune 控制是否清理远端已删分支。返回 trim 后的 stdout。
func (s *GitService) Fetch(repoPath, remote string, prune bool) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	args := []string{"fetch"}
	if prune {
		args = append(args, "--prune")
	}
	if strings.TrimSpace(remote) != "" {
		args = append(args, remote)
	}

	output, err := s.gitCmd.Execute(gitRoot, args...)
	if err != nil {
		return "", fmt.Errorf("fetch 失败: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// SetBranchUpstream 为指定分支设置上游跟踪分支（git branch --set-upstream-to=<remote>/<branch> <branch>）。
func (s *GitService) SetBranchUpstream(repoPath, branch, remote string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("分支名不能为空")
	}
	if strings.TrimSpace(remote) == "" {
		return fmt.Errorf("远程仓库名不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	upstream := remote + "/" + branch
	if _, err := s.gitCmd.Execute(gitRoot, "branch", "--set-upstream-to="+upstream, branch); err != nil {
		return fmt.Errorf("设置上游分支失败: %w", err)
	}
	return nil
}

// ===== 分支管理（增删改） =====

// CreateBranch 从当前 HEAD 创建新分支（git branch <name>）。name 空报错，重名透传 git 报错。
func (s *GitService) CreateBranch(repoPath, name string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("分支名不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	if _, err := s.gitCmd.Execute(gitRoot, "branch", name); err != nil {
		return fmt.Errorf("创建分支失败: %w", err)
	}
	return nil
}

// DeleteBranch 删除本地分支。force=false 走 git branch -d（安全删除，未合并会失败），
// force=true 走 git branch -D（强制删除）。name 空报错。
func (s *GitService) DeleteBranch(repoPath, name string, force bool) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("分支名不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	flag := "-d"
	if force {
		flag = "-D"
	}
	if _, err := s.gitCmd.Execute(gitRoot, "branch", flag, name); err != nil {
		return fmt.Errorf("删除分支失败: %w", err)
	}
	return nil
}

// RenameBranch 重命名本地分支（git branch -m <oldName> <newName>），仅本地不触远程。
// oldName/newName 空报错；当前分支重命名由前端显式传入当前分支名实现。
func (s *GitService) RenameBranch(repoPath, oldName, newName string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if strings.TrimSpace(oldName) == "" {
		return fmt.Errorf("原分支名不能为空")
	}
	if strings.TrimSpace(newName) == "" {
		return fmt.Errorf("新分支名不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	if _, err := s.gitCmd.Execute(gitRoot, "branch", "-m", oldName, newName); err != nil {
		return fmt.Errorf("重命名分支失败: %w", err)
	}
	return nil
}

// ===== 暂存区管理 =====

// StageFiles 暂存文件（git add -- <files>），files 空报错。
// 与 Commit 内部暂存逻辑一致，但单独暴露供工作区整理使用。
func (s *GitService) StageFiles(repoPath string, files []string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if len(files) == 0 {
		return fmt.Errorf("未选择要暂存的文件")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	args := append([]string{"add", "--"}, files...)
	if _, err := s.gitCmd.Execute(gitRoot, args...); err != nil {
		return fmt.Errorf("暂存文件失败: %w", err)
	}
	return nil
}

// UnstageFiles 取消暂存文件（git restore --staged -- <files>），统一命令通配已跟踪/未跟踪（git 2.25+）。
// files 空报错。
func (s *GitService) UnstageFiles(repoPath string, files []string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if len(files) == 0 {
		return fmt.Errorf("未选择要取消暂存的文件")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	args := append([]string{"restore", "--staged", "--"}, files...)
	if _, err := s.gitCmd.Execute(gitRoot, args...); err != nil {
		return fmt.Errorf("取消暂存文件失败: %w", err)
	}
	return nil
}

// ===== 合并 / 变基 / 拣选 / 冲突解决 =====
//
// 设计要点：
//   - merge/rebase/cherry-pick 共用 precheckMutation 前置校验（仓库存在 + 非 detached HEAD + 工作区干净）
//   - 冲突类操作 git 以 exit 1 正常返回，util 层 ExecuteWithCodes 已接受，service 层不把冲突当错误
//   - continue/abort/skip 通过 requireInProgress 守卫，避免无进行中操作时调 --continue 产生 git 报错
//   - Pull 增 useRebase 参数为破坏性签名变更，须同步 app 层与前端 wailsjs 绑定（见 cross-layer-contracts.md）

// precheckMutation 校验变更类操作前置条件：仓库存在 + 非 detached HEAD + 工作区干净。
// 返回 git 根目录供后续操作使用。detached HEAD 判定依据 branch --show-current 返回空串。
func (s *GitService) precheckMutation(repoPath string) (string, error) {
	if !s.gitCmd.IsGitRepository(repoPath) {
		return "", fmt.Errorf("不是Git仓库")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	branch, err := s.gitCmd.GetBranch(gitRoot)
	if err != nil {
		return "", fmt.Errorf("获取当前分支失败: %w", err)
	}
	if strings.TrimSpace(branch) == "" {
		return "", fmt.Errorf("当前处于分离头指针状态，禁止合并/变基/拣选")
	}
	hasChanges, err := s.gitCmd.HasLocalChanges(gitRoot)
	if err != nil {
		return "", fmt.Errorf("检查工作区状态失败: %w", err)
	}
	if hasChanges {
		return "", fmt.Errorf("工作区不干净，请先提交或暂存变更")
	}
	return gitRoot, nil
}

// Merge 合并 branch 到当前分支，mode 取 ff/no-ff/squash。前置校验通过后委托 util。
func (s *GitService) Merge(repoPath, branch string, mode model.MergeMode) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	if strings.TrimSpace(branch) == "" {
		return "", fmt.Errorf("目标分支不能为空")
	}
	gitRoot, err := s.precheckMutation(repoPath)
	if err != nil {
		return "", err
	}
	return s.gitCmd.Merge(gitRoot, branch, string(mode))
}

// Rebase 将当前分支变基到 branch 之上。前置校验通过后委托 util。
func (s *GitService) Rebase(repoPath, branch string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	if strings.TrimSpace(branch) == "" {
		return "", fmt.Errorf("目标分支不能为空")
	}
	gitRoot, err := s.precheckMutation(repoPath)
	if err != nil {
		return "", err
	}
	return s.gitCmd.Rebase(gitRoot, branch)
}

// CherryPick 将 sha 拣选到当前分支。前置校验通过后委托 util。
func (s *GitService) CherryPick(repoPath, sha string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	if strings.TrimSpace(sha) == "" {
		return "", fmt.Errorf("提交 SHA 不能为空")
	}
	gitRoot, err := s.precheckMutation(repoPath)
	if err != nil {
		return "", err
	}
	return s.gitCmd.CherryPick(gitRoot, sha)
}

// GetConflictState 返回当前冲突态快照。按 merge > rebase > cherry-pick 优先级判定类型，
// 并列出未解决冲突文件。无冲突态时 Type=none、Files 为空切片。
func (s *GitService) GetConflictState(repoPath string) (*model.ConflictState, error) {
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	state := &model.ConflictState{Type: model.ConflictTypeNone, Files: []string{}}
	switch {
	case s.gitCmd.IsMergeInProgress(gitRoot):
		state.Type = model.ConflictTypeMerge
	case s.gitCmd.IsRebaseInProgress(gitRoot):
		state.Type = model.ConflictTypeRebase
	case s.gitCmd.IsCherryPickInProgress(gitRoot):
		state.Type = model.ConflictTypeCherryPick
	default:
		return state, nil
	}
	files, err := s.gitCmd.ListConflictFiles(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("获取冲突文件列表失败: %w", err)
	}
	state.Files = files
	return state, nil
}

// ResolveConflict 标记单个冲突文件已解决（git add -- <file>）。file 空报错。
func (s *GitService) ResolveConflict(repoPath, file string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if strings.TrimSpace(file) == "" {
		return fmt.Errorf("文件路径不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	if _, err := s.gitCmd.Execute(gitRoot, "add", "--", file); err != nil {
		return fmt.Errorf("标记已解决失败: %w", err)
	}
	return nil
}

// requireInProgress 守卫 continue/abort/skip：要求指定操作进行中，否则报错。返回 git 根。
func (s *GitService) requireInProgress(repoPath string, op model.ConflictType) (string, error) {
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	var inProgress bool
	switch op {
	case model.ConflictTypeMerge:
		inProgress = s.gitCmd.IsMergeInProgress(gitRoot)
	case model.ConflictTypeRebase:
		inProgress = s.gitCmd.IsRebaseInProgress(gitRoot)
	case model.ConflictTypeCherryPick:
		inProgress = s.gitCmd.IsCherryPickInProgress(gitRoot)
	default:
		return "", fmt.Errorf("不支持的操作类型: %s", op)
	}
	if !inProgress {
		return "", fmt.Errorf("无进行中的%s操作", op)
	}
	return gitRoot, nil
}

// ContinueMerge 合并冲突解决后提交合并（git commit --no-edit，复用 MERGE_MSG）。
func (s *GitService) ContinueMerge(repoPath string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	gitRoot, err := s.requireInProgress(repoPath, model.ConflictTypeMerge)
	if err != nil {
		return "", err
	}
	return s.gitCmd.MergeContinue(gitRoot)
}

// ContinueRebase 变基冲突解决后继续。可能再次冲突。
func (s *GitService) ContinueRebase(repoPath string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	gitRoot, err := s.requireInProgress(repoPath, model.ConflictTypeRebase)
	if err != nil {
		return "", err
	}
	return s.gitCmd.RebaseContinue(gitRoot)
}

// ContinueCherryPick 拣选冲突解决后继续。可能再次冲突。
func (s *GitService) ContinueCherryPick(repoPath string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	gitRoot, err := s.requireInProgress(repoPath, model.ConflictTypeCherryPick)
	if err != nil {
		return "", err
	}
	return s.gitCmd.CherryPickContinue(gitRoot)
}

// AbortMerge 中止合并，回滚到合并前状态。
func (s *GitService) AbortMerge(repoPath string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	gitRoot, err := s.requireInProgress(repoPath, model.ConflictTypeMerge)
	if err != nil {
		return err
	}
	_, err = s.gitCmd.MergeAbort(gitRoot)
	return err
}

// AbortRebase 中止变基，回滚到变基前分支位置。
func (s *GitService) AbortRebase(repoPath string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	gitRoot, err := s.requireInProgress(repoPath, model.ConflictTypeRebase)
	if err != nil {
		return err
	}
	_, err = s.gitCmd.RebaseAbort(gitRoot)
	return err
}

// AbortCherryPick 中止拣选，回滚到拣选前状态。
func (s *GitService) AbortCherryPick(repoPath string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	gitRoot, err := s.requireInProgress(repoPath, model.ConflictTypeCherryPick)
	if err != nil {
		return err
	}
	_, err = s.gitCmd.CherryPickAbort(gitRoot)
	return err
}

// SkipRebase 跳过当前冲突提交继续变基（仅 rebase 有 --skip 语义，merge/cherry-pick 无）。
func (s *GitService) SkipRebase(repoPath string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	gitRoot, err := s.requireInProgress(repoPath, model.ConflictTypeRebase)
	if err != nil {
		return "", err
	}
	return s.gitCmd.RebaseSkip(gitRoot)
}

// ===== Submodule 管理 =====
//
// 设计要点：
//   - submodule 操作的 repoPath 恒为 superproject 根（用户在 WorkBench 添加的顶层仓库）
//   - 列表走双命令融合：git submodule status（前导码/SHA/path/describe/init 态）+ git status --porcelain=2（dirty）
//   - 只读查询不抢 tryLockRepo（与 ListTags/ListRemotes 一致）；变更类抢锁，add/remove 加 precheckMutation
//   - util 层封装 git 子命令，mode 以 string 传入避免 util 反向依赖 model

// ListSubmodules 列出 superproject 下所有 submodule，双命令融合产出 GitSubmodule。
// `git submodule status` 提供前导码/SHA/path/describe/init 态/SHA 不一致/冲突；
// `git status --porcelain=2` 提供 submodule 工作区 dirty 标记（submodule status 不检测 dirty）。
// 仅读查询，不抢仓级锁。
func (s *GitService) ListSubmodules(repoPath string) ([]model.GitSubmodule, error) {
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	statusOut, err := s.gitCmd.SubmoduleStatus(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("获取 submodule 状态失败: %w", err)
	}

	// 解析 git submodule status 每行：前导码 + 40位SHA + path + 可选 (describe)
	submods := make(map[string]*model.GitSubmodule)
	var order []string
	for _, line := range strings.Split(statusOut, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 42 { // 至少 1 前导 + 40 SHA + 1 空格 + 1 path
			continue
		}
		prefix := line[0]
		sha := line[1:41]
		rest := strings.TrimLeft(line[41:], " ")
		// describe 在末尾括号 (...)，未初始化时无此段
		var describe, path string
		if idx := strings.LastIndex(rest, " ("); idx >= 0 && strings.HasSuffix(rest, ")") {
			path = strings.TrimSpace(rest[:idx])
			describe = rest[idx+2 : len(rest)-1]
		} else {
			path = strings.TrimSpace(rest)
		}
		if path == "" {
			continue
		}
		sm := &model.GitSubmodule{
			Path:     path,
			Sha:      sha,
			ShortSha: shortSHA(sha),
			Describe: describe,
		}
		switch prefix {
		case '-':
			sm.Initialized = false
		case '+':
			sm.Initialized = true
			sm.ShaMismatch = true
		case 'U':
			sm.Initialized = true
			sm.Conflict = true
		default: // 空格
			sm.Initialized = true
		}
		submods[path] = sm
		order = append(order, path)
	}

	// 融合 .gitmodules 的 branch/url 配置
	s.fillSubmoduleConfig(gitRoot, submods)

	// 融合 git status --porcelain=2 的 dirty 标记
	// 注意：git submodule status 不检测 submodule 工作区 dirty，须 porcelain=2 融合。
	// porcelain=2 ordinary 行: "1 <XY> <subFlags> <mH> <mI> <mW> <hH> <hI> <path>"
	// submodule 行 subFlags 以 "S" 开头，后跟 4 位标志（git 2.41 实测）：
	//   位1（S 后第1位）: C=committed change（submodule HEAD≠index）/ 空格
	//   位2（S 后第2位）: M=submodule 内已跟踪文件被修改 / .
	//   位3（S 后第3位）: U=submodule 内有未跟踪文件 / .
	//   位4: 其他
	// dirty（工作区有改动）= 位2 'M' 或 位3 'U'。ShaMismatch 已由 submodule status 前导码 + 判定，不重复。
	// 不用 XY 的 Y 字段判 dirty：Y 在「新提交干净」与「dirty」两种场景均为 M，无法区分（实测
	// 新提交=SC.. / dirty=S.CMU 或 SC.U，差异在 subFlags 位2/位3）。
	porcelainOut, err := s.gitCmd.StatusPorcelain2(gitRoot)
	if err == nil {
		for _, line := range strings.Split(porcelainOut, "\n") {
			line = strings.TrimRight(line, "\r")
			if line == "" || line[0] != '1' {
				continue // 仅普通变更行（"1 ..."），忽略 "?" 未跟踪与 "2" 重命名
			}
			fields := strings.Fields(line)
			// 前 8 字段无空格；第 9 字段起为 path（含空格时整段引号包裹，strings.Fields 按空格拆开，
			// 故取 fields[8:] 拼回复原）。不足 9 字段说明非 ordinary 变更行，跳过。
			if len(fields) < 9 {
				continue
			}
			subFlags := fields[2]
			// 仅处理 submodule 行（subFlags 以 S 开头且至少 4 字符含位2/位3）
			if len(subFlags) < 4 || subFlags[0] != 'S' {
				continue
			}
			path := strings.Join(fields[8:], " ")
			path = strings.Trim(path, "\"")
			sm, ok := submods[path]
			if !ok {
				continue
			}
			// subFlags 位2（已跟踪文件修改）或 位3（未跟踪文件）非 '.' 表示 submodule 工作区 dirty
			if subFlags[2] != '.' || subFlags[3] != '.' {
				sm.Dirty = true
			}
		}
	}

	// 融合 detached 检测：对每个已初始化 submodule 查 branch --show-current 是否为空
	for _, p := range order {
		sm := submods[p]
		if !sm.Initialized {
			continue
		}
		subDir := filepath.Join(gitRoot, sm.Path)
		if branch, err := s.gitCmd.BranchShowCurrent(subDir); err == nil && strings.TrimSpace(branch) == "" {
			sm.Detached = true
		}
	}

	result := make([]model.GitSubmodule, 0, len(order))
	for _, p := range order {
		result = append(result, *submods[p])
	}
	return result, nil
}

// fillSubmoduleConfig 从 .gitmodules 读取 branch/url 配置填充到 GitSubmodule。
// .gitmodules 为 ini 格式，[submodule "name"] 段下 path/url/branch 键。
// 解析失败不阻断（返回的 GitSubmodule 仅缺 branch/url，不影响状态展示）。
func (s *GitService) fillSubmoduleConfig(gitRoot string, submods map[string]*model.GitSubmodule) {
	gitmodulesPath := filepath.Join(gitRoot, ".gitmodules")
	data, err := os.ReadFile(gitmodulesPath)
	if err != nil {
		return // 无 .gitmodules（无 submodule 或未提交），跳过
	}

	// 简易 ini 解析：按段 [submodule "x"] 收集键值，段内 path/url/branch 映射
	type section struct {
		path, url, branch string
	}
	sections := make(map[string]*section)
	var curName string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// 段头: [submodule "name"] -> 取 name
			inner := line[1 : len(line)-1]
			parts := strings.SplitN(inner, " ", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == "submodule" {
				curName = strings.Trim(strings.TrimSpace(parts[1]), "\"")
				if _, ok := sections[curName]; !ok {
					sections[curName] = &section{}
				}
			} else {
				curName = ""
			}
			continue
		}
		if curName == "" {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		sec := sections[curName]
		switch key {
		case "path":
			sec.path = val
		case "url":
			sec.url = val
		case "branch":
			sec.branch = val
		}
	}

	// .gitmodules 段名可能与 path 不同（name 是逻辑名，path 是路径），按 path 匹配
	for _, sec := range sections {
		if sm, ok := submods[sec.path]; ok {
			sm.Url = sec.url
			sm.Branch = sec.branch
		}
	}
}

// shortSHA 返回 SHA 前 8 位，不足 8 位返回原值。
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// InitSubmodules 初始化 submodule（git submodule init）。
// 仅注册到本地 .git/config，不克隆不检出，一般直接走 UpdateSubmodules(--init)。抢锁。
func (s *GitService) InitSubmodules(repoPath string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	output, err := s.gitCmd.SubmoduleInit(gitRoot)
	if err != nil {
		return "", fmt.Errorf("初始化 submodule 失败: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// UpdateSubmodules 更新 submodule。mode 取 checkout/merge/rebase/remote，recursive 下探嵌套。
// path 非空时仅更新单个 submodule，为空时更新全部。init=true 走 update --init（含首次检出）。
// 抢锁（变更类操作），但不走 precheckMutation——submodule 更新不要求 superproject 工作区干净，
// 仅要求非 detached HEAD（superproject 本身须在分支上）。
func (s *GitService) UpdateSubmodules(repoPath string, mode model.SubmoduleUpdateMode, recursive, init bool, path string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	// mode 白名单校验：SubmoduleUpdateMode 为具名 string 类型，编译期不约束取值，
	// 非法值（前端传错或调用方拼错）会在 util.SubmoduleUpdate 的 switch default 静默走 checkout，
	// 与用户意图不符且无报错。此处显式拒绝，给出可定位的错误。空串兜底为 checkout。
	switch mode {
	case model.SubmoduleUpdateCheckout, model.SubmoduleUpdateMerge,
		model.SubmoduleUpdateRebase, model.SubmoduleUpdateRemote:
	case "":
		mode = model.SubmoduleUpdateCheckout
	default:
		return "", fmt.Errorf("不支持的子模块更新模式: %q（有效值 checkout/merge/rebase/remote）", string(mode))
	}

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	var output string
	if init {
		output, err = s.gitCmd.SubmoduleUpdateInit(gitRoot, recursive, path)
	} else {
		output, err = s.gitCmd.SubmoduleUpdate(gitRoot, string(mode), recursive, path)
	}
	if err != nil {
		return "", fmt.Errorf("更新 submodule 失败: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// AddSubmodule 新增 submodule（git submodule add [-b branch] <url> <path>）。
// 一次完成：生成 .gitmodules（版本化）+ 写 .git/config（本地注册）+ .git/modules/<name>（git 目录存储）+ 工作区检出。
// 抢锁 + precheckMutation（要求 superproject 工作区干净 + 非 detached HEAD）。
// url/path 空校验。返回 trim 后的 stdout。
// 注意：git 2.41+ 默认禁 file 协议（CVE-2022-39253），file:// 或本地路径作 url 时
// 须用户环境配置 protocol.file.allow=always，否则 git 报错原样透传（WorkBench 不处理）。
func (s *GitService) AddSubmodule(repoPath, url, path, branch string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	if strings.TrimSpace(url) == "" {
		return "", fmt.Errorf("submodule 仓库地址不能为空")
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("submodule 路径不能为空")
	}

	gitRoot, err := s.precheckMutation(repoPath)
	if err != nil {
		return "", err
	}

	output, err := s.gitCmd.SubmoduleAdd(gitRoot, url, path, branch)
	if err != nil {
		return "", fmt.Errorf("添加 submodule 失败: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// RemoveSubmodule 删除 submodule，三步清理确保无残留：
//  1. git submodule deinit -f <path>：清空工作区 + 移除 .git/config 段
//  2. git rm -f <path>：移除 superproject index gitlink + .gitmodules 条目 + 工作区目录
//  3. os.RemoveAll(.git/modules/<path>)：手动清除 .git/modules 残留
//     （git 不自动清此目录，遗漏致同名 submodule 重加时复用旧 git 目录、历史错乱）
//
// 抢锁 + precheckMutation（要求工作区干净 + 非 detached HEAD）。path 空校验。
// path 同时作 .git/modules 下的段名（git 默认段名等于 path）。
func (s *GitService) RemoveSubmodule(repoPath, path string) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("submodule 路径不能为空")
	}

	// path 穿越防御：path 最终拼入 os.RemoveAll(.git/modules/<path>)，虽步骤 1/2 的 git
	// submodule deinit / git rm 会拒绝仓库外路径，仍在此显式拦截绝对路径与 .. 上溯，
	// 作为深度防御给出更清晰错误，避免依赖 git 子命令副作用保安全。
	cleanPath := filepath.Clean(path)
	if filepath.IsAbs(path) ||
		filepath.ToSlash(cleanPath) == ".." ||
		strings.HasPrefix(filepath.ToSlash(cleanPath), "../") {
		return fmt.Errorf("非法 submodule 路径（禁止绝对路径或 .. 上溯）: %s", path)
	}

	gitRoot, err := s.precheckMutation(repoPath)
	if err != nil {
		return err
	}

	// 步骤 1：deinit 清工作区 + .git/config 段（不清 .gitmodules、不清 .git/modules）
	if _, err := s.gitCmd.SubmoduleDeinit(gitRoot, path); err != nil {
		return fmt.Errorf("注销 submodule 失败: %w", err)
	}
	// 步骤 2：git rm 清 superproject index gitlink + .gitmodules 条目 + 工作区目录
	if _, err := s.gitCmd.Execute(gitRoot, "rm", "-f", path); err != nil {
		return fmt.Errorf("移除 submodule gitlink 失败: %w", err)
	}
	// 步骤 3：手动清 .git/modules/<path>（git 不自动清此目录）。
	// path 即 .git/modules 下的目录名——git submodule add 默认段名=name=path，故默认场景命中。
	// 已知边界：若 submodule 以 `git submodule add --name <name>` 添加（.gitmodules 段名 ≠ path），
	// git 目录存于 .git/modules/<name> 而非 .git/modules/<path>，本步删除落空（RemoveAll 对不存在
	// 路径返回 nil 不报错），残留 .git/modules/<name>。WorkBench 的 AddSubmodule 不带 --name，
	// 故本工具添加的 submodule 不触发此边界；外部以自定义 name 添加的 submodule 删除后需手动清理。
	modulesDir := filepath.Join(gitRoot, ".git", "modules", path)
	if err := os.RemoveAll(modulesDir); err != nil {
		return fmt.Errorf("清理 .git/modules 残留失败: %w", err)
	}
	return nil
}

// CheckoutSubmoduleBranch 将 detached 的 submodule 切换到跟踪分支，避免用户在分离头指针上开发丢提交。
// subPath 为 submodule 在 superproject 中的相对路径，branch 为目标分支名（通常读 .gitmodules 的 branch 配置）。
// 抢锁（变更类操作），但不走 precheckMutation——此操作作用于 submodule 自身工作区，
// 不要求 superproject 工作区干净。subPath/branch 空校验。
func (s *GitService) CheckoutSubmoduleBranch(repoPath, subPath, branch string) (string, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return "", err
	}
	defer release()

	if strings.TrimSpace(subPath) == "" {
		return "", fmt.Errorf("submodule 路径不能为空")
	}
	if strings.TrimSpace(branch) == "" {
		return "", fmt.Errorf("分支名不能为空")
	}

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	subDir := filepath.Join(gitRoot, subPath)
	output, err := s.gitCmd.Checkout(subDir, branch)
	if err != nil {
		return "", fmt.Errorf("切换 submodule 分支失败: %w", err)
	}
	return strings.TrimSpace(output), nil
}
