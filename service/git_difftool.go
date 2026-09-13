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

// 外部 diff 工具集成：GitService 编排 git 版本内容获取与外部工具进程启动。
//
// 职责边界：App 层负责读取设置并平铺传参；本文件负责按 mode 解析左右版本
// 内容、落临时文件、渲染参数模板并启动工具进程。错误经 AppError 结构化
// 返回，前端按 code 分流（见 docs/spec/logging-and-errors.md）。

// OpenInExternalDiff 用外部 diff 工具打开文件的两个版本对比。
//   - workspace：左侧 HEAD 版本（未跟踪/无 HEAD 版本时为空文件），右侧工作区原文件
//     （原文件已删除时降级为空文件临时路径）
//   - commit：左侧父提交版本（root commit 对比空树，即空文件），右侧提交版本
//   - range：左侧 BaseSHA 版本，右侧 HeadSHA 版本
//
// exePath / argsTemplate 来自应用设置；未配置或模板缺占位符返回
// ErrCodeDiffToolNotConfigured，进程启动失败返回 ErrCodeDiffToolLaunchFailed。
func (s *GitService) OpenInExternalDiff(repoPath, exePath, argsTemplate string, req model.ExternalDiffRequest) error {
	if strings.TrimSpace(exePath) == "" {
		return model.NewAppError(model.ErrCodeDiffToolNotConfigured, "未配置外部 diff 工具，请在设置中配置")
	}
	// 占位符校验提前到 git 操作前：配置无效属用户可自行修复项，快速失败
	if !strings.Contains(argsTemplate, "{left}") || !strings.Contains(argsTemplate, "{right}") {
		return model.NewAppError(model.ErrCodeDiffToolNotConfigured, "外部 diff 工具参数模板须包含 {left} 与 {right} 占位符")
	}
	if req.File == "" {
		return fmt.Errorf("文件路径不能为空")
	}

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	leftContent, rightSource, needTempRight, err := s.resolveExternalDiffSides(gitRoot, req)
	if err != nil {
		return err
	}

	// 左侧恒写临时文件；右侧为仓库原文件时直接传路径（用户可在工具中编辑保存），
	// 为内容时（commit/range 或工作区删除降级）同样写临时文件
	tempDir, err := util.CreateDiffTempDir()
	if err != nil {
		return err
	}
	name := diffTempFileName(req.File)
	leftPath, err := util.WriteDiffTempFile(tempDir, "left", name, leftContent)
	if err != nil {
		return err
	}
	rightPath := rightSource
	if needTempRight {
		rightPath, err = util.WriteDiffTempFile(tempDir, "right", name, rightSource)
		if err != nil {
			return err
		}
	}

	args, err := util.RenderDiffArgsTemplate(argsTemplate, leftPath, rightPath)
	if err != nil {
		return model.NewAppError(model.ErrCodeDiffToolNotConfigured, "外部 diff 工具参数模板无效: "+err.Error())
	}

	cmd := exec.Command(exePath, args...)
	util.HideCommandWindow(cmd)
	if err := cmd.Start(); err != nil {
		return model.WrapAppError(model.ErrCodeDiffToolLaunchFailed,
			fmt.Sprintf("启动外部 diff 工具失败: %v", err), err)
	}

	// 异步等待进程退出：工具可能运行很久，不阻塞调用；异常退出仅记日志
	go func() {
		if waitErr := cmd.Wait(); waitErr != nil {
			Logger().Warn("external diff tool exited with error", "tool", exePath, "file", req.File, "err", waitErr)
		}
	}()

	return nil
}

// resolveExternalDiffSides 按 mode 解析左右版本内容来源。
// 返回左侧内容（写临时文件）与右侧路径；needTempRight 表示右侧是否需要写临时文件
// （false 时 rightPath 为仓库内原文件绝对路径）。
func (s *GitService) resolveExternalDiffSides(gitRoot string, req model.ExternalDiffRequest) (leftContent, rightPath string, needTempRight bool, err error) {
	switch req.Mode {
	case "workspace":
		// 左侧 HEAD 版本；未跟踪文件无 HEAD 版本，git show 失败按空内容处理
		left, err := s.gitShow(gitRoot, "HEAD:"+req.File)
		if err != nil {
			left = ""
		}
		abs := filepath.Join(gitRoot, filepath.FromSlash(req.File))
		if _, statErr := os.Stat(abs); statErr != nil {
			// 工作区文件已删除（deleted 状态）：右侧降级为空内容临时文件
			return left, "", true, nil
		}
		return left, abs, false, nil
	case "commit":
		if req.SHA == "" {
			return "", "", false, fmt.Errorf("提交 SHA 不能为空")
		}
		left := ""
		if s.hasParent(gitRoot, req.SHA) {
			left, err = s.gitShow(gitRoot, req.SHA+"^:"+req.File)
			if err != nil {
				// 父提交中不存在该文件（本提交新增）：左侧为空内容
				left = ""
			}
		}
		right, err := s.gitShow(gitRoot, req.SHA+":"+req.File)
		if err != nil {
			return "", "", false, fmt.Errorf("获取提交版本内容失败: %w", err)
		}
		return left, right, true, nil
	case "range":
		if req.BaseSHA == "" || req.HeadSHA == "" {
			return "", "", false, fmt.Errorf("提交 SHA 不能为空")
		}
		left, err := s.gitShow(gitRoot, req.BaseSHA+":"+req.File)
		if err != nil {
			// 基准提交中不存在该文件（区间内新增）：左侧为空内容
			left = ""
		}
		right, err := s.gitShow(gitRoot, req.HeadSHA+":"+req.File)
		if err != nil {
			return "", "", false, fmt.Errorf("获取版本内容失败: %w", err)
		}
		return left, right, true, nil
	default:
		return "", "", false, fmt.Errorf("不支持的 diff 模式: %s", req.Mode)
	}
}

// gitShow 读取指定 rev:path 的文件内容（如 HEAD:main.go）。
func (s *GitService) gitShow(gitRoot, revPath string) (string, error) {
	return s.gitCmd.Execute(gitRoot, "show", revPath)
}

// diffTempFileName 取仓库相对路径的文件名部分，兼容 git 的 / 分隔与 Windows 的 \ 分隔。
func diffTempFileName(file string) string {
	if idx := strings.LastIndexAny(file, `/\`); idx >= 0 {
		return file[idx+1:]
	}
	return file
}
