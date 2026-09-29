package util

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
	"workbench/util/testutil"
)

// lockStderrSample 本机 git 实测的 index.lock 被占用 stderr 全文（2026-09-29，
// git for Windows）。识别与提取均以该真实文案为准。
const lockStderrSample = "fatal: Unable to create 'C:/Users/demo/repo/.git/index.lock': File exists.\n\n" +
	"Another git process seems to be running in this repository, e.g.\n" +
	"an editor opened by 'git commit'. Please make sure all processes\n" +
	"are terminated then try again. If it still fails, a git process\n" +
	"may have crashed in this repository earlier:\n" +
	"remove the file manually to continue.\n"

// TestParseGitLockError_RealStderr 真实 stderr 文案识别为 E_GIT_LOCKED AppError，
// message 含锁路径与人工处理引导（验收项：锁残留错误识别单测）。
func TestParseGitLockError_RealStderr(t *testing.T) {
	appErr := parseGitLockError(lockStderrSample)
	if appErr == nil {
		t.Fatal("真实锁残留 stderr 应识别为锁错误")
	}
	if appErr.Code != model.ErrCodeGitLocked {
		t.Errorf("错误码应为 E_GIT_LOCKED，实际: %s", appErr.Code)
	}
	if !strings.Contains(appErr.Message, "C:/Users/demo/repo/.git/index.lock") {
		t.Errorf("message 应含从 stderr 提取的锁路径，实际: %s", appErr.Message)
	}
	if !strings.Contains(appErr.Message, "手动删除") {
		t.Errorf("message 应含人工处理引导，实际: %s", appErr.Message)
	}
}

// TestParseGitLockError_Negative 非锁错误不误判：普通 stderr、含 index.lock 但
// 非「无法创建」形态（如锁清理后的提示）均返回 nil，走既有裸错误路径。
func TestParseGitLockError_Negative(t *testing.T) {
	cases := map[string]string{
		"普通失败":                            "error: your local changes to the following files would be overwritten by merge",
		"含 index.lock 但非创建失败":             "warning: some hint about index.lock",
		"Unable to create 但非 File exists": "fatal: Unable to create 'x': Permission denied",
		"空输出":                             "",
	}
	for name, stderr := range cases {
		if err := parseGitLockError(stderr); err != nil {
			t.Errorf("%s 不应识别为锁错误，实际: %v", name, err)
		}
	}
}

// TestFindIndexLockResidue 探测逻辑：构造/清除 index.lock 文件驱动，无时序依赖
// （验收项：探测逻辑单测）。覆盖常规仓、子目录向上、worktree gitdir 文件、
// clone 场景禁向上、非仓库与无锁静默降级。
func TestFindIndexLockResidue(t *testing.T) {
	t.Run("常规仓库根存在锁", func(t *testing.T) {
		dir := t.TempDir()
		testutil.RunGit(t, dir, "init")
		lockPath := filepath.Join(dir, ".git", "index.lock")
		testutil.WriteFile(t, lockPath, "")

		got := findIndexLockResidue(dir, true)
		if got != lockPath {
			t.Errorf("应返回锁路径 %s，实际: %q", lockPath, got)
		}
	})

	t.Run("仓库子目录向上定位", func(t *testing.T) {
		dir := t.TempDir()
		testutil.RunGit(t, dir, "init")
		sub := filepath.Join(dir, "a", "b")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(dir, ".git", "index.lock")
		testutil.WriteFile(t, lockPath, "")

		if got := findIndexLockResidue(sub, true); got != lockPath {
			t.Errorf("子目录应向上定位到仓库根锁路径，实际: %q", got)
		}
	})

	t.Run("锁不存在静默返回空", func(t *testing.T) {
		dir := t.TempDir()
		testutil.RunGit(t, dir, "init")
		if got := findIndexLockResidue(dir, true); got != "" {
			t.Errorf("无锁应返回空串，实际: %q", got)
		}
	})

	t.Run("worktree形式的git文件解析gitdir", func(t *testing.T) {
		dir := t.TempDir()
		realGitDir := filepath.Join(dir, "main", ".git", "worktrees", "wt1")
		if err := os.MkdirAll(realGitDir, 0o755); err != nil {
			t.Fatal(err)
		}
		wt := filepath.Join(dir, "wt")
		if err := os.MkdirAll(wt, 0o755); err != nil {
			t.Fatal(err)
		}
		// worktree 的 .git 是文件，内容形如 "gitdir: <path>"（相对路径以 worktree 根为基准）
		testutil.WriteFile(t, filepath.Join(wt, ".git"), "gitdir: ../main/.git/worktrees/wt1\n")
		lockPath := filepath.Join(realGitDir, "index.lock")
		testutil.WriteFile(t, lockPath, "")

		if got := findIndexLockResidue(wt, true); got != lockPath {
			t.Errorf("worktree 应解析 gitdir 定位锁路径 %s，实际: %q", lockPath, got)
		}
	})

	t.Run("畸形gitdir指针静默返回空", func(t *testing.T) {
		dir := t.TempDir()
		testutil.WriteFile(t, filepath.Join(dir, ".git"), "gitdir:\n")

		if got := findIndexLockResidue(dir, true); got != "" {
			t.Errorf("畸形 gitdir 指针应静默返回空串（防相对路径误解析 CWD），实际: %q", got)
		}
	})

	t.Run("clone场景禁向上防误报父仓锁", func(t *testing.T) {
		parent := t.TempDir()
		testutil.RunGit(t, parent, "init")
		testutil.WriteFile(t, filepath.Join(parent, ".git", "index.lock"), "")
		target := filepath.Join(parent, "new-repo") // 尚未建成仓库的目标
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}

		if got := findIndexLockResidue(target, false); got != "" {
			t.Errorf("clone 目标禁向上，不应误报父仓锁，实际: %q", got)
		}
	})

	t.Run("非仓库目录静默返回空", func(t *testing.T) {
		if got := findIndexLockResidue(t.TempDir(), true); got != "" {
			t.Errorf("非仓库应静默返回空串，实际: %q", got)
		}
		if got := findIndexLockResidue("", true); got != "" {
			t.Errorf("空路径应静默返回空串，实际: %q", got)
		}
	})
}

// TestWarnIndexLockResidue_EmitsLog 探测命中时 slog.Warn 带锁路径（取消路径收口
// 的可观测面）；空路径静默不输出。
func TestWarnIndexLockResidue_EmitsLog(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	warnIndexLockResidue("C:/repo/.git/index.lock")
	if !strings.Contains(buf.String(), "index.lock 残留") || !strings.Contains(buf.String(), "C:/repo/.git/index.lock") {
		t.Errorf("应输出含锁路径的残留告警，实际: %s", buf.String())
	}

	buf.Reset()
	warnIndexLockResidue("")
	if buf.Len() != 0 {
		t.Errorf("空路径应静默，实际输出: %s", buf.String())
	}
}

// TestRun_LockErrorIdentifiedAsAppError 仓内锁存在时写操作识别为 E_GIT_LOCKED
// AppError（真 git 命令驱动，构造/清除 index.lock 文件，无时序依赖）。
// 显式设 LC_ALL/LANG=C 强制 git 英文 stderr——run 不设 cmd.Env 继承测试进程 env，
// 消除本地化 locale（LC_MESSAGES）导致的文案漂移（测试禁隐式依赖环境外部状态）。
func TestRun_LockErrorIdentifiedAsAppError(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")

	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "a")
	testutil.WriteFile(t, filepath.Join(dir, ".git", "index.lock"), "")

	g := NewGitCommand()
	_, err := g.Execute(dir, "add", "a.txt")
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("锁存在时应返回 AppError，实际: %v", err)
	}
	if appErr.Code != model.ErrCodeGitLocked {
		t.Errorf("错误码应为 E_GIT_LOCKED，实际: %s", appErr.Code)
	}
}

// TestRun_NonLockErrorUnchanged 非锁残留错误行为逐字不变：保持既有
// 「git [args] failed: <stderr>」格式，不被误包装为 AppError。
func TestRun_NonLockErrorUnchanged(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")

	g := NewGitCommand()
	_, err := g.Execute(dir, "rev-parse", "--verify", "nonexistent-branch")
	if err == nil {
		t.Fatal("无效分支应返回错误")
	}
	var appErr *model.AppError
	if errors.As(err, &appErr) {
		t.Errorf("非锁错误不应被包装为 AppError: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "git [rev-parse --verify nonexistent-branch] failed:") {
		t.Errorf("非锁错误格式应逐字保持既有形态，实际: %v", err)
	}
}

// TestExecuteWithContext_CanceledProbeDoesNotAlterCancelError 取消语义回归：
// 锁存在场景下取消路径仍返回 context.Canceled 包装错误（E_GIT_CANCELLED 链路
// 语义不变），探测仅日志副作用。
func TestExecuteWithContext_CanceledProbeDoesNotAlterCancelError(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")
	testutil.WriteFile(t, filepath.Join(dir, ".git", "index.lock"), "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := NewGitCommand()
	_, err := g.ExecuteWithContext(ctx, dir, "status")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消语义应保持 context.Canceled 包装，实际: %v", err)
	}
	var appErr *model.AppError
	if errors.As(err, &appErr) {
		t.Errorf("取消路径不应被锁识别改写: %v", err)
	}
}

// TestCloneWithContext_CancelProbesTargetLock clone 取消分支接线：目标仓已建仓库
// 形态且锁存在时探测命中（resolveUpward=false 语义，禁向上误报父仓锁）；
// ctx 预先 cancel 稳定触发，命令不执行无时序依赖。
func TestCloneWithContext_CancelProbesTargetLock(t *testing.T) {
	parent := t.TempDir()
	testutil.RunGit(t, parent, "init")
	testutil.WriteFile(t, filepath.Join(parent, ".git", "index.lock"), "")
	target := filepath.Join(parent, "new-repo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, filepath.Join(target, ".git", "index.lock"), "")

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := NewGitCommand()
	_, err := g.CloneWithContext(ctx, "https://example.com/repo.git", target)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消语义应保持 context.Canceled 包装，实际: %v", err)
	}
	// 告警应指向目标仓自身的锁（目标路径尾段在 TextHandler 引号包裹前后均不变）
	if !strings.Contains(buf.String(), "index.lock") || !strings.Contains(buf.String(), filepath.Base(target)) {
		t.Errorf("clone 取消应探测目标仓锁残留，实际日志: %s", buf.String())
	}
	// 禁向上：父仓的锁不应误报（父仓也有锁，若实现误向上会告警父仓路径——
	// 两仓锁均在，靠 target 尾段区分断言已覆盖；此处再断言日志仅一条告警）
	if n := strings.Count(buf.String(), "index.lock 残留"); n != 1 {
		t.Errorf("应仅告警目标仓一条，实际 %d 条: %s", n, buf.String())
	}
}

// TestExecuteWithContext_TimeoutProbesLock 超时同样杀子进程，锁存在时探测命中
// （告警副作用经 slog 捕获断言），且超时返回 E_GIT_TIMEOUT 引导级 AppError
// （2026-09-29 收口：探测告警副作用保留，错误不再落入锁 parse 误判）。
func TestExecuteWithContext_TimeoutProbesLock(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")
	lockPath := filepath.Join(dir, ".git", "index.lock")
	testutil.WriteFile(t, lockPath, "")

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	g := NewGitCommandWithTimeout(1 * time.Nanosecond)
	_, err := g.ExecuteWithContext(context.Background(), dir, "status")
	if err == nil {
		t.Fatal("超时应返回错误")
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("超时不应误判为取消: %v", err)
	}
	// 超时收口后即使 stderr 疑似锁特征也不得误判为 E_GIT_LOCKED（被杀子进程
	// stderr 半截不可信，不参与锁识别）
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeGitTimeout {
		t.Fatalf("超时应返回 E_GIT_TIMEOUT 而非锁错误，实际: %v", err)
	}
	// 断言用稳定片段而非完整路径：TextHandler 对含空格路径走 strconv.Quote
	// （反斜杠翻倍），完整路径断言会随临时目录命名偶发挂（test-stability 规则面）；
	// "index.lock" 字样与 TempDir 随机尾段（8 位无空格）在 Quote 前后均不变
	if !strings.Contains(buf.String(), "index.lock") || !strings.Contains(buf.String(), filepath.Base(dir)) {
		t.Errorf("超时路径应探测并告警锁残留，实际日志: %s", buf.String())
	}
}
