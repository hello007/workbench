package util

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"workbench/model"
)

// GitCommand Git命令执行器
type GitCommand struct {
	timeout time.Duration
}

// NewGitCommand 创建Git命令执行器
func NewGitCommand() *GitCommand {
	return &GitCommand{
		timeout: 30 * time.Second,
	}
}

// NewGitCommandWithTimeout 创建指定超时时间的 Git 命令执行器
func NewGitCommandWithTimeout(timeout time.Duration) *GitCommand {
	return &GitCommand{
		timeout: timeout,
	}
}

// run Git 命令统一执行实现：ctx 的超时/取消语义由调用方给定（WithTimeout 包在外层），
// acceptedExitCodes 非 nil 时命中退出码视为正常返回。
// 外部取消（ctx.Err() == context.Canceled）时错误以 %w 包装 context.Canceled，
// 供上层 errors.Is 识别并映射为「操作已取消」；超时（DeadlineExceeded）返回
// E_GIT_TIMEOUT 引导级 AppError（%w 包装 DeadlineExceeded 保留 errors.Is 穿透），
// 与取消分支对称提前返回：超时被杀 exit 1 曾被退出码白名单吞为「冲突正常返回」
// 致冲突面板假态，且被杀子进程 stderr 半截不可信，不参与锁识别（2026-09-29 收口，
// 既有裸错误格式 `git [args] failed: <stderr>` 有意推翻，详见 docs/spec/logging-and-errors.md）。
func (g *GitCommand) run(ctx context.Context, workDir string, acceptedExitCodes map[int]bool, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir
	HideCommandWindow(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// 取消检测先于退出码白名单：Windows 下 exec.CommandContext 取消经
		// TerminateProcess 终止进程（exit code 1），若白名单分支先行会把被取消的
		// pull --rebase（conflictExitCodes 含 1）误报为「冲突正常返回」，取消静默丢失。
		// 被杀子进程可能残留 .git/index.lock（commit 写窗口被杀，如 ContinueMerge 显式
		// commit），探测仅告警不自动删（残留不能断定 stale），探测失败静默降级不影响取消返回
		if ctx.Err() == context.Canceled {
			warnIndexLockResidue(findIndexLockResidue(workDir, true))
			return "", fmt.Errorf("git %v failed: %w", args, ctx.Err())
		}
		// 超时同样 TerminateProcess 杀子进程，对称探测锁残留（置于白名单分支前：
		// 超时被杀 exit 1 可能命中 conflictExitCodes，曾被白名单吞为「冲突正常返回」）；
		// 告警后立即返回，不再落入白名单与锁 parse——超时不得静默吞为冲突假态，
		// 也不得因被杀进程 stderr 半截内容误判为锁错误
		if ctx.Err() == context.DeadlineExceeded {
			warnIndexLockResidue(findIndexLockResidue(workDir, true))
			return "", model.WrapAppError(model.ErrCodeGitTimeout, "Git 操作超时已终止，请重试", ctx.Err())
		}
		if exitErr, ok := err.(*exec.ExitError); ok && acceptedExitCodes[exitErr.ExitCode()] {
			return stdout.String(), nil
		}
		// 锁被占用错误识别为 AppError（E_GIT_LOCKED）透传前端 warning 引导，
		// 未命中走既有裸错误路径（非锁残留错误行为逐字不变）
		if lockErr := parseGitLockError(stderr.String()); lockErr != nil {
			return "", lockErr
		}
		return "", fmt.Errorf("git %v failed: %s", args, stderr.String())
	}

	return stdout.String(), nil
}

// Execute 执行Git命令
func (g *GitCommand) Execute(workDir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()
	return g.run(ctx, workDir, nil, args...)
}

// ExecuteWithContext 执行 Git 命令，外部 ctx 取消信号可贯穿终止 git 子进程
// （exec.CommandContext 杀进程机制现成，此变体补外部取消入口）。
// 内部保留单命令超时语义（g.timeout），外部取消经 parent ctx 传播。
func (g *GitCommand) ExecuteWithContext(ctx context.Context, workDir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	return g.run(ctx, workDir, nil, args...)
}

// ExecuteWithCodes 执行 Git 命令，允许指定退出码视为正常（如 git diff --no-index 有差异时退出码 1）。
// 若退出码在 acceptedExitCodes 中，返回 stdout 与 nil；否则按 Execute 语义返回错误。
func (g *GitCommand) ExecuteWithCodes(workDir string, acceptedExitCodes map[int]bool, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()
	return g.run(ctx, workDir, acceptedExitCodes, args...)
}

// ExecuteWithCodesWithContext 同 ExecuteWithCodes（冲突类操作退出码白名单语义保留），
// 外部 ctx 取消信号可贯穿。pull --rebase 变基模式 Pull 的取消贯穿依赖此变体保留
// 冲突 exit 1 接受语义。
func (g *GitCommand) ExecuteWithCodesWithContext(ctx context.Context, workDir string, acceptedExitCodes map[int]bool, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	return g.run(ctx, workDir, acceptedExitCodes, args...)
}

// IsGitRepository 检查目录是否是Git仓库
func (g *GitCommand) IsGitRepository(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	HideCommandWindow(cmd)
	return cmd.Run() == nil
}

// IsGitRepositoryFast 基于 .git 条目存在性快速判定目录是否为 Git 仓库。
// 仅判 os.Stat 无错，不要求 .git 是目录，从而覆盖 worktree/submodule 的 .git 文件场景
// （这两类的 .git 是文件，内容形如 "gitdir: /path/..."，若用 IsDir() 判定会漏判）。
// 与现有 IsGitRepository（fork git rev-parse）相比，单次 os.Stat 约 0.1ms，快 200~1000 倍。
//
// 已知边界：bare repo（无 .git 条目）会漏判，桌面工作目录场景可忽略；
// 损坏/残留 .git 会误判为仓库，后续真实 git 操作时会报错暴露，不产生静默数据错误。
func IsGitRepositoryFast(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// GetBranch 获取当前分支名
func (g *GitCommand) GetBranch(dir string) (string, error) {
	return g.Execute(dir, "branch", "--show-current")
}

// GetRemote 获取远程仓库URL
func (g *GitCommand) GetRemote(dir string) (string, string, error) {
	lines, err := g.ExecuteWithOutput(dir, "remote", "-v")
	if err != nil {
		return "", "", err
	}

	if len(lines) == 0 {
		return "", "", fmt.Errorf("no remote configured")
	}

	parts := strings.Fields(lines[0])
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid remote format")
	}

	return parts[0], strings.TrimSuffix(parts[1], " (fetch)"), nil
}

// ExecuteWithOutput 执行并返回行分割输出
func (g *GitCommand) ExecuteWithOutput(workDir string, args ...string) ([]string, error) {
	output, err := g.Execute(workDir, args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	return lines, nil
}

// Clone 克隆仓库
func (g *GitCommand) Clone(url, targetPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return g.clone(ctx, url, targetPath)
}

// CloneWithContext 克隆仓库，外部 ctx 取消信号可贯穿终止 git clone 子进程。
// 内部保留 5 分钟克隆超时语义，与 Clone 一致。
func (g *GitCommand) CloneWithContext(ctx context.Context, url, targetPath string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return g.clone(ctx, url, targetPath)
}

// clone 克隆统一实现，取消错误包装语义同 run。
func (g *GitCommand) clone(ctx context.Context, url, targetPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "clone", url, targetPath)
	HideCommandWindow(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// 取消/超时杀子进程后探测目标仓 index.lock 残留（resolveUpward=false：目标可能
		// 尚未建成仓库形态，禁向上防误报父仓的锁）；仅告警不自动删，探测失败静默降级
		if ctx.Err() == context.Canceled {
			warnIndexLockResidue(findIndexLockResidue(targetPath, false))
			return "", fmt.Errorf("git clone failed: %w", ctx.Err())
		}
		// 超时对称探测后立即返回 E_GIT_TIMEOUT（与 run 收口一致：告警不丢、
		// 被杀子进程 stderr 半截不参与锁识别、超时升引导级非裸错误）
		if ctx.Err() == context.DeadlineExceeded {
			warnIndexLockResidue(findIndexLockResidue(targetPath, false))
			return "", model.WrapAppError(model.ErrCodeGitTimeout, "Git 操作超时已终止，请重试", ctx.Err())
		}
		if lockErr := parseGitLockError(stderr.String()); lockErr != nil {
			return "", lockErr
		}
		return "", fmt.Errorf("git clone failed: %s", stderr.String())
	}

	return stdout.String(), nil
}

// FindGitRoot 从给定路径向上查找 Git 仓库根目录。
// 判定复用 IsGitRepositoryFast（仅判 .git 条目存在，不要求 IsDir），覆盖 submodule / worktree
// 的 .git 文件场景（这两类的 .git 是文件，内容形如 "gitdir: /path/..."，IsDir 判定会漏判）。
func FindGitRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for {
		if IsGitRepositoryFast(abs) {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("not a git repository: %s", path)
		}
		abs = parent
	}
}

// Pull 拉取更新
func (g *GitCommand) Pull(dir string) (string, error) {
	return g.Execute(dir, "pull")
}

// PullWithContext 拉取更新，外部 ctx 取消信号可贯穿终止 git pull 子进程。
func (g *GitCommand) PullWithContext(ctx context.Context, dir string) (string, error) {
	return g.ExecuteWithContext(ctx, dir, "pull")
}

// PullRebaseWithContext 拉取远程更新并以变基方式重放本地提交，外部 ctx 取消可贯穿。
// 冲突 exit 1 接受语义与 PullRebase 一致（经 conflictExitCodes）。
func (g *GitCommand) PullRebaseWithContext(ctx context.Context, dir string) (string, error) {
	return g.ExecuteWithCodesWithContext(ctx, dir, conflictExitCodes, "pull", "--rebase")
}

// GetBranchesAll 获取所有分支（本地+远程）
func (g *GitCommand) GetBranchesAll(dir string) (string, error) {
	return g.Execute(dir, "branch", "-a")
}

// HasLocalChanges 检查是否有未提交的变更
func (g *GitCommand) HasLocalChanges(dir string) (bool, error) {
	output, err := g.Execute(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// CheckoutLocal 切换到本地分支
func (g *GitCommand) CheckoutLocal(dir, branch string) (string, error) {
	return g.Execute(dir, "checkout", branch)
}

// CheckoutRemote 从远程分支创建本地分支并跟踪
func (g *GitCommand) CheckoutRemote(dir, remoteBranch, localBranch string) (string, error) {
	return g.Execute(dir, "checkout", "-b", localBranch, remoteBranch)
}

// ===== 合并 / 变基 / 拣选 =====
//
// 以下操作均可能因冲突以 exit 1 正常结束，统一走 ExecuteWithCodes 接受退出码 1。
// 调用方据返回的 stdout/stderr 与 IsXxxInProgress 判定是否进入冲突态。
// workDir 须为仓库根目录（由 service 层 FindGitRoot 保证），冲突态检测依赖 .git 目录。

// conflictExitCodes 冲突类操作可接受的退出码集合：exit 1 表示存在冲突，视为正常返回。
var conflictExitCodes = map[int]bool{1: true}

// Merge 合并指定分支到当前分支。mode 取 ff/no-ff/squash：
// ff 走 git 默认行为（可快进时快进），no-ff 强制合并提交，squash 压缩为单个暂存变更。
// mode 以 string 传入而非 model.MergeMode，避免 util 层反向依赖 model，保持底层纯净。
func (g *GitCommand) Merge(workDir, branch, mode string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, mergeArgs(branch, mode)...)
}

// mergeArgs 构造 git merge 参数（含按 mode 的策略 flag），Merge 与 MergeWithContext 共用，
// 保证两条路径参数构造单点不漂移。
func mergeArgs(branch, mode string) []string {
	args := []string{"merge"}
	switch mode {
	case "no-ff":
		args = append(args, "--no-ff")
	case "squash":
		args = append(args, "--squash")
	default:
		// ff：git 默认即可快进时快进，不附加 flag
	}
	return append(args, branch)
}

// MergeWithContext 合并指定分支到当前分支，外部 ctx 取消信号可贯穿终止 git merge 子进程。
// 冲突 exit 1 接受语义与 Merge 一致（经 conflictExitCodes）。
func (g *GitCommand) MergeWithContext(ctx context.Context, workDir, branch, mode string) (string, error) {
	return g.ExecuteWithCodesWithContext(ctx, workDir, conflictExitCodes, mergeArgs(branch, mode)...)
}

// Rebase 将当前分支变基到指定分支之上（git rebase <branch>）。
func (g *GitCommand) Rebase(workDir, branch string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "rebase", branch)
}

// RebaseWithContext 将当前分支变基到指定分支之上，外部 ctx 取消信号可贯穿终止 git rebase 子进程。
// 冲突 exit 1 接受语义与 Rebase 一致（经 conflictExitCodes）。
func (g *GitCommand) RebaseWithContext(ctx context.Context, workDir, branch string) (string, error) {
	return g.ExecuteWithCodesWithContext(ctx, workDir, conflictExitCodes, "rebase", branch)
}

// CherryPick 将指定提交拣选到当前分支（git cherry-pick <sha>）。
func (g *GitCommand) CherryPick(workDir, sha string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "cherry-pick", sha)
}

// CherryPickWithContext 将指定提交拣选到当前分支，外部 ctx 取消信号可贯穿终止 git cherry-pick 子进程。
// 冲突 exit 1 接受语义与 CherryPick 一致（经 conflictExitCodes）。
func (g *GitCommand) CherryPickWithContext(ctx context.Context, workDir, sha string) (string, error) {
	return g.ExecuteWithCodesWithContext(ctx, workDir, conflictExitCodes, "cherry-pick", sha)
}

// PullRebase 拉取远程更新并以变基方式重放本地提交（git pull --rebase）。
func (g *GitCommand) PullRebase(workDir string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "pull", "--rebase")
}

// MergeAbort 中止进行中的合并，回滚到合并前状态（git merge --abort）。
func (g *GitCommand) MergeAbort(workDir string) (string, error) {
	return g.Execute(workDir, "merge", "--abort")
}

// MergeContinue 合并冲突解决后提交合并。走 git commit --no-edit 复用预生成的 MERGE_MSG，
// 避免弹出编辑器。仍可能因遗留冲突以 exit 1 失败，走 ExecuteWithCodes 接受。
func (g *GitCommand) MergeContinue(workDir string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "commit", "--no-edit")
}

// MergeContinueWithContext 合并冲突解决后提交合并，外部 ctx 取消信号可贯穿终止子进程。
// 冲突 exit 1 接受语义与 MergeContinue 一致（经 conflictExitCodes）。
func (g *GitCommand) MergeContinueWithContext(ctx context.Context, workDir string) (string, error) {
	return g.ExecuteWithCodesWithContext(ctx, workDir, conflictExitCodes, "commit", "--no-edit")
}

// RebaseAbort 中止进行中的变基，回滚到变基前分支位置（git rebase --abort）。
func (g *GitCommand) RebaseAbort(workDir string) (string, error) {
	return g.Execute(workDir, "rebase", "--abort")
}

// RebaseContinue 解决冲突后继续变基。可能再次冲突，走 ExecuteWithCodes 接受 exit 1。
func (g *GitCommand) RebaseContinue(workDir string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "rebase", "--continue")
}

// RebaseContinueWithContext 解决冲突后继续变基，外部 ctx 取消信号可贯穿终止子进程。
// 冲突 exit 1 接受语义与 RebaseContinue 一致（经 conflictExitCodes）。
func (g *GitCommand) RebaseContinueWithContext(ctx context.Context, workDir string) (string, error) {
	return g.ExecuteWithCodesWithContext(ctx, workDir, conflictExitCodes, "rebase", "--continue")
}

// RebaseSkip 跳过当前冲突提交继续变基（git rebase --skip）。
func (g *GitCommand) RebaseSkip(workDir string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "rebase", "--skip")
}

// RebaseSkipWithContext 跳过当前冲突提交继续变基，外部 ctx 取消信号可贯穿终止子进程。
// 冲突 exit 1 接受语义与 RebaseSkip 一致（经 conflictExitCodes）。
func (g *GitCommand) RebaseSkipWithContext(ctx context.Context, workDir string) (string, error) {
	return g.ExecuteWithCodesWithContext(ctx, workDir, conflictExitCodes, "rebase", "--skip")
}

// CherryPickAbort 中止进行中的拣选（git cherry-pick --abort）。
func (g *GitCommand) CherryPickAbort(workDir string) (string, error) {
	return g.Execute(workDir, "cherry-pick", "--abort")
}

// CherryPickContinue 解决冲突后继续拣选。可能再次冲突，走 ExecuteWithCodes 接受 exit 1。
func (g *GitCommand) CherryPickContinue(workDir string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "cherry-pick", "--continue")
}

// CherryPickContinueWithContext 解决冲突后继续拣选，外部 ctx 取消信号可贯穿终止子进程。
// 冲突 exit 1 接受语义与 CherryPickContinue 一致（经 conflictExitCodes）。
func (g *GitCommand) CherryPickContinueWithContext(ctx context.Context, workDir string) (string, error) {
	return g.ExecuteWithCodesWithContext(ctx, workDir, conflictExitCodes, "cherry-pick", "--continue")
}

// ListConflictFiles 列出未解决冲突文件（git diff --name-only --diff-filter=U）。
// 返回相对仓库根的文件路径列表，无冲突时返回空切片。
func (g *GitCommand) ListConflictFiles(workDir string) ([]string, error) {
	output, err := g.Execute(workDir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return []string{}, nil
	}
	return strings.Split(output, "\n"), nil
}

// IsMergeInProgress 检测是否处于合并冲突态：.git/MERGE_HEAD 存在即合并进行中。
func (g *GitCommand) IsMergeInProgress(workDir string) bool {
	return fileExists(filepath.Join(workDir, ".git", "MERGE_HEAD"))
}

// IsRebaseInProgress 检测是否处于变基态：.git/rebase-merge/ 或 .git/rebase-apply/ 存在。
func (g *GitCommand) IsRebaseInProgress(workDir string) bool {
	return fileExists(filepath.Join(workDir, ".git", "rebase-merge")) ||
		fileExists(filepath.Join(workDir, ".git", "rebase-apply"))
}

// IsCherryPickInProgress 检测是否处于拣选冲突态：.git/CHERRY_PICK_HEAD 存在即拣选进行中。
func (g *GitCommand) IsCherryPickInProgress(workDir string) bool {
	return fileExists(filepath.Join(workDir, ".git", "CHERRY_PICK_HEAD"))
}

// fileExists 判定路径存在性（文件或目录均可），供冲突态检测复用。
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ===== Submodule =====
//
// 以下封装 git submodule 子命令。workDir 须为 superproject 根目录（由 service 层
// FindGitRoot 保证），submodule 命令在 superproject 根执行才能正确读写 .gitmodules
// 与 .git/modules/<name>。

// SubmoduleStatus 执行 `git submodule status`，返回原始多行输出。
// 每行格式: "<前导码><40位SHA> <path> (<describe>)" 或 "<前导码><SHA> <path>"（未初始化无 describe）。
// 前导码: 空格=一致, -=未初始化, +=SHA 不一致, U=合并冲突。
// 注意：本命令不检测 submodule 工作区 dirty，dirty 须另取 StatusPorcelain2。
func (g *GitCommand) SubmoduleStatus(workDir string) (string, error) {
	return g.Execute(workDir, "submodule", "status")
}

// StatusPorcelain2 执行 `git status --porcelain=2`，用于检测 submodule 工作区 dirty。
// submodule 行含 "S" 标志位与 Y 字段，service 层解析后与 submodule status 按 path 关联补 dirty。
func (g *GitCommand) StatusPorcelain2(workDir string) (string, error) {
	return g.Execute(workDir, "status", "--porcelain=2")
}

// SubmoduleInit 执行 `git submodule init`，将 .gitmodules 条目复制到本地 .git/config。
func (g *GitCommand) SubmoduleInit(workDir string) (string, error) {
	return g.Execute(workDir, "submodule", "init")
}

// SubmoduleUpdate 执行 `git submodule update`，mode 取 checkout/merge/rebase/remote：
// checkout 为默认（git 原生，检出 index SHA），merge/rebase/remote 对应同名 flag。
// recursive=true 附加 --recursive 下探嵌套 submodule。path 非空时限定单 submodule。
// mode 以 string 传入而非 model.SubmoduleUpdateMode，避免 util 层反向依赖 model，保持底层纯净。
func (g *GitCommand) SubmoduleUpdate(workDir, mode string, recursive bool, path string) (string, error) {
	args := []string{"submodule", "update"}
	switch mode {
	case "merge":
		args = append(args, "--merge")
	case "rebase":
		args = append(args, "--rebase")
	case "remote":
		args = append(args, "--remote")
	default:
		// checkout：git 默认行为，不附加 flag
	}
	if recursive {
		args = append(args, "--recursive")
	}
	if path != "" {
		args = append(args, "--", path)
	}
	return g.Execute(workDir, args...)
}

// SubmoduleUpdateInit 等价 `git submodule update --init [--recursive]`，一步完成注册 + 检出。
// clone 后首次检出 / 切到含新 submodule 的分支时首选。path 非空时限定单 submodule。
func (g *GitCommand) SubmoduleUpdateInit(workDir string, recursive bool, path string) (string, error) {
	args := []string{"submodule", "update", "--init"}
	if recursive {
		args = append(args, "--recursive")
	}
	if path != "" {
		args = append(args, "--", path)
	}
	return g.Execute(workDir, args...)
}

// SubmoduleAdd 执行 `git submodule add [-b branch] -- <url> <path>`，新增 submodule。
// 生成 .gitmodules + 写 .git/config + .git/modules/<name> + 工作区检出。
// `--` 分隔 positional 参数，防止 url/path 以 `-` 起头被 git 误解析为 flag（参数注入深度防御）。
func (g *GitCommand) SubmoduleAdd(workDir, url, path, branch string) (string, error) {
	args := []string{"submodule", "add"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, "--", url, path)
	return g.Execute(workDir, args...)
}

// SubmoduleDeinit 执行 `git submodule deinit -f <path>`，清空工作区 + 移除 .git/config 段。
// 不清 .gitmodules、不清 .git/modules/<name>、不清 superproject index gitlink，须调用方后续处理。
func (g *GitCommand) SubmoduleDeinit(workDir, path string) (string, error) {
	return g.Execute(workDir, "submodule", "deinit", "-f", path)
}

// BranchShowCurrent 执行 `git -C <dir> branch --show-current`，返回当前分支名。
// 返回空串表示处于 detached HEAD 状态，供 submodule detached 检测使用。
func (g *GitCommand) BranchShowCurrent(dir string) (string, error) {
	return g.Execute(dir, "branch", "--show-current")
}

// Checkout 执行 `git -C <dir> checkout <branch>`，切换分支。供 submodule 切换跟踪分支使用。
func (g *GitCommand) Checkout(dir, branch string) (string, error) {
	return g.Execute(dir, "checkout", branch)
}
