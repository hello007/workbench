package util

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"workbench/model"
)

// git 无法创建 index.lock 的 stderr 特征（本机 git 实测文案）：
//
//	fatal: Unable to create '<repo>/.git/index.lock': File exists.
//
// 提取组 1 为锁文件路径（git 输出的正斜杠风格原样保留展示）。
// git 输出被本地化（LC_MESSAGES）时英文特征失效，此时识别降级为既有裸错误
// 透传（不误判，仅损失引导能力）。
var gitLockPathRe = regexp.MustCompile(`Unable to create '([^']*index\.lock)': File exists`)

// parseGitLockError 判定 git stderr 是否为 index.lock 被占用（残留或他进程持有），
// 命中返回携带 E_GIT_LOCKED 的 AppError（message 含锁路径与人工处理引导），
// 未命中返回 nil 由调用方走既有错误路径。
//
// 返回 AppError 后经 Wails ErrorFormatter（main.go）与 serve 通道
// rpcFailureFromCall（server/rpc.go）双通道透传 {code, message}，
// 前端按 warning 分流展示引导，两通道无需各自识别。
//
// 禁自动删除锁文件：锁存在不能断定 stale（同仓可能有用户手动 git 进程合法持锁），
// 误删会破坏在途写操作。
func parseGitLockError(stderr string) *model.AppError {
	if !strings.Contains(stderr, "index.lock") {
		return nil
	}
	m := gitLockPathRe.FindStringSubmatch(stderr)
	if m == nil {
		return nil
	}
	return model.WrapAppError(model.ErrCodeGitLocked,
		fmt.Sprintf("Git 锁文件被占用：%s。若确认没有其他 Git 进程正在操作该仓库（如编辑器打开的 git commit），可手动删除该文件后重试。", m[1]),
		errors.New(stderr))
}

// findIndexLockResidue 探测 dir 所属仓库的 index.lock 残留，返回存在中的锁文件路径。
// dir 为空、非仓库、锁不存在、任一探测环节失败均返回空串（静默降级，不阻塞取消主流程）。
//
// resolveUpward 为 true 时 dir 可为仓库子目录（向上定位仓库根，run() 通用场景）；
// 为 false 时 dir 即仓库根本体（git clone 目标场景：目标可能尚未建成任何仓库形态，
// 禁止向上——目标父目录本身是仓库时会误报父仓的锁）。
//
// .git 覆盖两形态：目录（常规仓库）与文件（worktree/submodule，内容形如
// "gitdir: <path>"，锁在指向的真实 gitdir 下），后者解析 gitdir 定位（相对路径
// 以仓库根为基准）。
func findIndexLockResidue(dir string, resolveUpward bool) string {
	if dir == "" {
		return ""
	}
	root := dir
	if resolveUpward {
		found, err := FindGitRoot(dir)
		if err != nil {
			return ""
		}
		root = found
	}
	lockPath := repoIndexLockPath(root)
	if lockPath == "" {
		return ""
	}
	if _, err := os.Stat(lockPath); err != nil {
		return ""
	}
	return lockPath
}

// repoIndexLockPath 返回 repoDir 仓库的 index.lock 路径，非仓库形态返回空串。
func repoIndexLockPath(repoDir string) string {
	gitEntry := filepath.Join(repoDir, ".git")
	info, err := os.Stat(gitEntry)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return filepath.Join(gitEntry, "index.lock")
	}
	// worktree/submodule：.git 是文件，内容形如 "gitdir: <path>"，解析指针定位真实 gitdir
	data, err := os.ReadFile(gitEntry)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	gitDir, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return ""
	}
	gitDir = strings.TrimSpace(gitDir)
	if gitDir == "" {
		// 畸形指针（仅 "gitdir:" 无路径）：提前返回空串，防 Join 出相对路径
		// "index.lock" 被 os.Stat 相对进程 CWD 误解析
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoDir, gitDir)
	}
	return filepath.Join(gitDir, "index.lock")
}

// warnIndexLockResidue 取消/超时终止 git 子进程后探测锁残留并告警。仅日志可观测，
// 不改变调用方错误返回值——锁残留不能断定 stale，禁自动删除（同仓可能有用户手动
// git 进程合法持锁，误删破坏在途写操作）。
func warnIndexLockResidue(lockPath string) {
	if lockPath == "" {
		return
	}
	slog.Warn("git 操作终止后检测到 index.lock 残留，该仓库后续写操作将持续失败，请人工确认无其他 git 进程后删除该文件",
		"path", lockPath)
}
