package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"workbench/model"
	"workbench/util"
)

// 三向合并工具集成：冲突场景拉起外部合并工具（base/local/remote/merged 四文件）。
//
// 版本来源：git 冲突态下 index 中的 stage 编号——:1=共同祖先(base)、:2=当前分支
// (local/ours)、:3=合并目标(remote/theirs)。三者写临时文件；merged 恒为工作区
// 原文件绝对路径，用户在外部工具中编辑保存即写回工作区，随后经现有「标记已解决」
// （git add）链路衔接，无新增流程。
//
// 模板来源：预设名内置三向模板（MergeToolPresetTemplates，用户零配置）；
// custom 预设走 settings.DiffToolMergeArgs 自定义模板。两者均须包含
// {base}/{local}/{remote}/{merged} 四占位符。

// mergeToolPresetTemplates 内置三向合并参数模板（按 diff 工具预设名）。
// 参数顺序与各工具官方 CLI 文档及 git mergetools 内置配置一致：
//   - beyondcompare：git mergetools/bc 口径 `"$LOCAL" "$REMOTE" "$BASE" "$MERGED"`
//   - winmerge：WinMerge 手册三方形式，三栏左中右 = Local/Base/Remote，-o 输出 merged
//   - vscode：VSCode CLI `--merge <path1> <path2> <base> <result>`（git mergetools/vscode 口径）
//   - kdiff3：`kdiff3 <base> <mine> <theirs> -o <output>`
//   - meld：git mergetools/meld 口径 `meld "$LOCAL" "$BASE" "$REMOTE" --output="$MERGED"`
var mergeToolPresetTemplates = map[string]string{
	"beyondcompare": "{local} {remote} {base} {merged}",
	"winmerge":      "-e -u -wl -wr -dl Local -dm Base -dr Remote {local} {base} {remote} -o {merged}",
	"vscode":        "--wait --merge {remote} {local} {base} {merged}",
	"kdiff3":        "{base} {local} {remote} -o {merged}",
	"meld":          "{local} {base} {remote} --output={merged}",
}

// MergeToolPresetTemplate 返回预设名对应的三向合并模板；custom 或未知预设名
// 返回空串（调用方应回落到 settings.DiffToolMergeArgs）。
func MergeToolPresetTemplate(name string) string {
	return mergeToolPresetTemplates[name]
}

// isStageMissing 判断 `git show :N:file` 错误是否为「该 stage 不存在」语义：
//   - "path 'x' is in the index, but not at stage N"（文件在 index 但该 stage 无，
//     如非冲突文件的 :2/:3、add/add 冲突无 :1）
//   - "path 'x' does not exist (neither on disk nor in the index)"（文件完全不在 index）
//
// 仅缺失语义允许降级（base 空文件/不在冲突状态报错）；超时/锁冲突等真实错误上抛。
func isStageMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "is in the index, but not at stage") ||
		strings.Contains(msg, "does not exist (neither on disk nor in the index)")
}

// OpenInExternalMerge 用外部三向合并工具打开冲突文件。
// exePath 为外部工具可执行文件路径；mergeTemplate 为三向参数模板（App 层已按
// 预设名/自定义解析）；file 为冲突文件相对仓库根路径。
// 未配置（exePath/模板为空或占位符缺失）返回 ErrCodeDiffToolNotConfigured，
// 进程启动失败返回 ErrCodeDiffToolLaunchFailed，前端按 code 分流提示。
func (s *GitService) OpenInExternalMerge(repoPath, exePath, mergeTemplate, file string) error {
	if strings.TrimSpace(exePath) == "" {
		return model.NewAppError(model.ErrCodeDiffToolNotConfigured, "未配置外部 diff 工具，请在设置中配置")
	}
	if strings.TrimSpace(mergeTemplate) == "" {
		return model.NewAppError(model.ErrCodeDiffToolNotConfigured, "未配置三向合并模板：预设工具自动内置，自定义工具请在设置中填写合并参数模板")
	}
	if file == "" {
		return fmt.Errorf("文件路径不能为空")
	}
	// 占位符校验提前到 git 操作前：配置无效属用户可自行修复项，快速失败
	for _, ph := range []string{"{base}", "{local}", "{remote}", "{merged}"} {
		if !strings.Contains(mergeTemplate, ph) {
			return model.NewAppError(model.ErrCodeDiffToolNotConfigured, "三向合并模板须包含 {base} {local} {remote} {merged} 占位符")
		}
	}

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	// stage 提取：:2(local)/:3(remote) 缺失视为该文件不在冲突状态（已被解决或
	// 未冲突）；:1(base) 缺失（add/add 双方新增冲突无共同祖先）降级为空文件。
	// stage 缺失判断用 isStageMissing（「is in the index, but not at stage N」
	// 文案），与 diff 场景 isRevPathMissing 的 rev-path 缺失口径不同，不复用
	baseContent, baseErr := s.gitShow(gitRoot, ":1:"+file)
	if baseErr != nil {
		if !isStageMissing(baseErr) {
			return fmt.Errorf("获取冲突 base 版本失败: %w", baseErr)
		}
		baseContent = ""
	}
	localContent, err := s.gitShow(gitRoot, ":2:"+file)
	if err != nil {
		if isStageMissing(err) {
			return fmt.Errorf("%s 不在冲突状态（无本地版本）", file)
		}
		return fmt.Errorf("获取冲突 local 版本失败: %w", err)
	}
	remoteContent, err := s.gitShow(gitRoot, ":3:"+file)
	if err != nil {
		if isStageMissing(err) {
			return fmt.Errorf("%s 不在冲突状态（无远端版本）", file)
		}
		return fmt.Errorf("获取冲突 remote 版本失败: %w", err)
	}

	// merged = 工作区原文件：工具直接编辑工作区文件，保存即写回（deleted/modify
	// 冲突下工作区文件可能不存在，三方工具无法呈现编辑目标，引导先恢复）
	mergedPath := filepath.Join(gitRoot, filepath.FromSlash(file))
	if _, statErr := os.Stat(mergedPath); statErr != nil {
		return fmt.Errorf("工作区文件不存在（delete/modify 冲突请先恢复文件再外部合并）: %w", statErr)
	}

	// base/local/remote 恒写临时文件（复用 diff 临时目录生命周期：启动后不删，
	// 下次应用启动 CleanupDiffTempDir 统一清理上会话残留）
	tempDir, err := util.CreateDiffTempDir()
	if err != nil {
		return err
	}
	name := diffTempFileName(file)
	basePath, err := util.WriteDiffTempFile(tempDir, "base", name, baseContent)
	if err != nil {
		return err
	}
	localPath, err := util.WriteDiffTempFile(tempDir, "local", name, localContent)
	if err != nil {
		return err
	}
	remotePath, err := util.WriteDiffTempFile(tempDir, "remote", name, remoteContent)
	if err != nil {
		return err
	}

	args, err := util.RenderMergeArgsTemplate(mergeTemplate, basePath, localPath, remotePath, mergedPath)
	if err != nil {
		return model.NewAppError(model.ErrCodeDiffToolNotConfigured, "三向合并参数模板无效: "+err.Error())
	}

	cmd := exec.Command(exePath, args...)
	util.HideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		return model.WrapAppError(model.ErrCodeDiffToolLaunchFailed,
			fmt.Sprintf("启动外部合并工具失败: %v", err), err)
	}

	// 异步等待进程退出（同 OpenInExternalDiff）：工具可能运行很久，不阻塞调用
	go func() {
		if waitErr := cmd.Wait(); waitErr != nil {
			Logger().Warn("external merge tool exited with error", "tool", exePath, "file", file, "err", waitErr)
		}
	}()

	return nil
}
