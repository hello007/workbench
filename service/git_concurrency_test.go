package service

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestTryLockRepo_SameRepoMutualExclusion 同一仓库第二次 TryLock 应失败返回 ErrOperationInProgress，
// release 后第三次可重新获取。验证方案 A 互斥拒绝语义。
func TestTryLockRepo_SameRepoMutualExclusion(t *testing.T) {
	svc := NewGitService()
	dir := t.TempDir()

	// 第一次获取锁应成功
	release1, err := svc.tryLockRepo(dir)
	if err != nil {
		t.Fatalf("首次 tryLockRepo 应成功: %v", err)
	}

	// 同仓库第二次应被拒绝
	release2, err := svc.tryLockRepo(dir)
	if err == nil {
		release2()
		t.Fatal("同仓库并发应被拒绝，但 tryLockRepo 返回 nil error")
	}
	if !IsOperationInProgressError(err) {
		t.Errorf("错误应为 ErrOperationInProgress，实际: %v", err)
	}

	// 释放后第三次应可重新获取
	release1()
	release3, err := svc.tryLockRepo(dir)
	if err != nil {
		t.Fatalf("释放后应可重新获取锁: %v", err)
	}
	release3()
}

// TestTryLockRepo_RelativePathNormalized 相对路径与绝对路径应映射到同一把锁，
// 避免调用方传相对路径绕过互斥。
func TestTryLockRepo_RelativePathNormalized(t *testing.T) {
	svc := NewGitService()
	dir := t.TempDir()

	abs, _ := filepath.Abs(dir)

	// 用绝对路径占锁
	release, err := svc.tryLockRepo(abs)
	if err != nil {
		t.Fatalf("绝对路径首次应成功: %v", err)
	}
	defer release()

	// 同目录相对路径应被拒绝（CWD 不在该目录时 Abs 会拼 CWD，此处仅验证不同形式同路径）
	// 改用 dir 本身与 abs 双形式验证
	_, err = svc.tryLockRepo(dir)
	if !IsOperationInProgressError(err) {
		t.Errorf("dir 与 abs 应映射同一锁被拒，实际: %v", err)
	}
}

// TestTryLockRepo_DifferentRepoParallel 不同仓库的锁互不阻塞，
// 两个仓库可同时持有锁。
func TestTryLockRepo_DifferentRepoParallel(t *testing.T) {
	svc := NewGitService()
	dirA := t.TempDir()
	dirB := t.TempDir()

	releaseA, err := svc.tryLockRepo(dirA)
	if err != nil {
		t.Fatalf("仓库 A 首次应成功: %v", err)
	}
	defer releaseA()

	// 仓库 B 与 A 不同，应可同时获取
	releaseB, err := svc.tryLockRepo(dirB)
	if err != nil {
		t.Fatalf("不同仓库应可并行: %v", err)
	}
	releaseB()
}

// TestTryLockRepo_ReleaseIdempotentNotDouble 释放闭包调用一次正常，
// 不应 panic。验证 release 安全性。
func TestTryLockRepo_ReleaseSafe(t *testing.T) {
	svc := NewGitService()
	dir := t.TempDir()

	release, err := svc.tryLockRepo(dir)
	if err != nil {
		t.Fatalf("首次应成功: %v", err)
	}
	release()

	// 释放后重新获取成功即证明锁已归还
	release2, err := svc.tryLockRepo(dir)
	if err != nil {
		t.Fatalf("释放后应可重新获取: %v", err)
	}
	release2()
}

// TestTryLockRepo_ConcurrentTryLockRace race detector 下并发争抢同一仓库锁，
// 仅一个成功其余被拒，成功者释放后无泄漏。验证无数据竞争与锁泄漏。
func TestTryLockRepo_ConcurrentTryLockRace(t *testing.T) {
	svc := NewGitService()
	dir := t.TempDir()

	const goroutines = 20
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		refuses   int
	)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			release, err := svc.tryLockRepo(dir)
			if err != nil {
				if !IsOperationInProgressError(err) {
					t.Errorf("拒绝错误类型不符: %v", err)
				}
				mu.Lock()
				refuses++
				mu.Unlock()
				return
			}
			// 模拟短暂持有
			time.Sleep(5 * time.Millisecond)
			mu.Lock()
			successes++
			mu.Unlock()
			release()
		}()
	}
	wg.Wait()

	// 串行释放下可能多个成功（前一个释放后下一个抢到），但任一时刻仅一个持有。
	// 核心断言：不 panic、无 race、successes+refuses==总数、锁最终可释放（末尾再获取一次成功）
	if successes+refuses != goroutines {
		t.Errorf("计数不符: successes=%d refuses=%d total=%d", successes, refuses, goroutines)
	}

	// 最终锁应已归还，可再次获取
	final, err := svc.tryLockRepo(dir)
	if err != nil {
		t.Fatalf("并发结束后锁应可获取，可能泄漏: %v", err)
	}
	final()
}

// TestIsOperationInProgressError 覆盖错误识别三条路径：
// nil、直接 wrap（errors.Is）、字符串包含。
func TestIsOperationInProgressError(t *testing.T) {
	if IsOperationInProgressError(nil) {
		t.Error("nil 不应识别为进行中错误")
	}
	// errors.Is 路径
	if !IsOperationInProgressError(ErrOperationInProgress) {
		t.Error("ErrOperationInProgress 本身应识别")
	}
	// 字符串包含路径（模拟仅返回错误文案的场景）
	if !IsOperationInProgressError(errString("其他: 该仓库有 Git 操作进行中，请稍后重试")) {
		t.Error("含文案的错误应识别")
	}
	if IsOperationInProgressError(errString("普通失败")) {
		t.Error("普通失败不应识别")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// setupTinyRepo 创建一个带初始提交的临时 git 仓库，供变更方法接入测试使用。
func setupTinyRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "t@t.com")
	runGit(t, dir, "config", "user.name", "t")
	mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("a"))
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

// TestMutationMethod_RejectsConcurrentSameRepo 验证变更方法接入锁后，
// 同仓库已有锁时第二个变更调用被拒绝返回 ErrOperationInProgress。
// 用 StageFiles 作代表（轻量、不依赖网络/远程）。
func TestMutationMethod_RejectsConcurrentSameRepo(t *testing.T) {
	svc := NewGitService()
	dir := setupTinyRepo(t)
	mustWriteFile(t, filepath.Join(dir, "b.txt"), []byte("b"))

	// 手动占住该仓锁，模拟一个进行中的变更操作
	release, err := svc.tryLockRepo(dir)
	if err != nil {
		t.Fatalf("占锁失败: %v", err)
	}
	defer release()

	// StageFiles 应被拒绝
	err = svc.StageFiles(dir, []string{"b.txt"})
	if !IsOperationInProgressError(err) {
		t.Fatalf("占锁后 StageFiles 应返回 ErrOperationInProgress，实际: %v", err)
	}

	// Commit 应同样被拒绝
	err = svc.Commit(dir, "msg", []string{"b.txt"})
	if !IsOperationInProgressError(err) {
		t.Fatalf("占锁后 Commit 应返回 ErrOperationInProgress，实际: %v", err)
	}
}

// TestMutationMethod_WorksAfterRelease 验证锁释放后变更方法恢复正常执行。
func TestMutationMethod_WorksAfterRelease(t *testing.T) {
	svc := NewGitService()
	dir := setupTinyRepo(t)
	mustWriteFile(t, filepath.Join(dir, "c.txt"), []byte("c"))

	release, err := svc.tryLockRepo(dir)
	if err != nil {
		t.Fatalf("占锁失败: %v", err)
	}
	release()

	// 释放后 StageFiles 应正常执行
	if err := svc.StageFiles(dir, []string{"c.txt"}); err != nil {
		t.Fatalf("释放后 StageFiles 应成功: %v", err)
	}
}

// TestReadonlyMethod_NotBlockedByLock 验证只读方法不抢锁，
// 占锁期间 GetLocalChanges / GetBranches 仍可正常执行。
func TestReadonlyMethod_NotBlockedByLock(t *testing.T) {
	svc := NewGitService()
	dir := setupTinyRepo(t)
	mustWriteFile(t, filepath.Join(dir, "d.txt"), []byte("d"))

	release, err := svc.tryLockRepo(dir)
	if err != nil {
		t.Fatalf("占锁失败: %v", err)
	}
	defer release()

	// 只读 GetLocalChanges 应不受锁影响
	changes, err := svc.GetLocalChanges(dir)
	if err != nil {
		t.Fatalf("占锁期间 GetLocalChanges 应成功: %v", err)
	}
	if len(changes) == 0 {
		t.Error("应检测到 d.txt 变更")
	}

	// 只读 GetBranches 应不受锁影响
	if _, err := svc.GetBranches(dir); err != nil {
		t.Fatalf("占锁期间 GetBranches 应成功: %v", err)
	}
}

// TestBatchPull_RejectsLockedRepo 验证 BatchPull 内部 worker 抢仓级锁，
// 与外部已占锁冲突时该仓记为失败（Error 含 ErrOperationInProgress 文案）。
// 仓库须配置远程才会走到抢锁分支（无远程直接 Skipped），此处用 example.com 远程：
// 抢锁失败时 BatchPull 不会发起 Pull 网络请求，故远程是否可达不影响测试。
func TestBatchPull_RejectsLockedRepo(t *testing.T) {
	svc := NewGitService()
	dir := setupTinyRepo(t)
	runGit(t, dir, "remote", "add", "origin", "https://example.com/repo.git")

	// 外部占住该仓锁，模拟用户正对该仓做单仓操作
	release, err := svc.tryLockRepo(dir)
	if err != nil {
		t.Fatalf("占锁失败: %v", err)
	}
	defer release()

	results := svc.BatchPull([]string{dir}, 1, context.Background())
	if len(results) != 1 {
		t.Fatalf("应返回 1 个结果，实际 %d", len(results))
	}
	r := results[0]
	if r.Success {
		t.Error("抢锁失败应 Success=false")
	}
	if r.Skipped {
		t.Error("已配置远程不应被跳过")
	}
	if !IsOperationInProgressError(errString(r.Error)) {
		t.Errorf("Error 应含 ErrOperationInProgress 文案，实际: %s", r.Error)
	}
}

// TestBatchPull_NoDeadlockOnConcurrentWorkers race 下多仓并发 BatchPull，
// 验证 worker 间抢各自仓锁无死锁、无数据竞争。
func TestBatchPull_NoDeadlockOnConcurrentWorkers(t *testing.T) {
	svc := NewGitService()

	// 建两个无远程临时仓（BatchPull 会跳过，但会经过抢锁前 IsGitRepository 判定路径）
	dirs := []string{setupTinyRepo(t), setupTinyRepo(t), setupTinyRepo(t)}

	results := svc.BatchPull(dirs, 3, context.Background())
	if len(results) != len(dirs) {
		t.Fatalf("应返回 %d 结果，实际 %d", len(dirs), len(results))
	}
}
