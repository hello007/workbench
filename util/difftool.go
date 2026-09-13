package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 外部 diff 工具集成辅助：参数模板渲染与左右版本临时文件管理。
//
// 临时文件策略：每次打开外部 diff 经 os.MkdirTemp 创建原子唯一的子目录，
// 目录内分 left/right 两侧写入，文件保留原文件名以便外部工具按扩展名
// 做语法高亮。启动后不主动删除（工具可能立即 detach 或长时间持有文件），
// 由应用启动时统一清理上会话残留（CleanupDiffTempDir）。

// diffTempRootOverride 测试注入用根目录覆盖；非空时 DiffTempRoot 返回它，
// 避免单测读写真实共享临时目录（误删运行中应用的在用文件）。
var diffTempRootOverride string

// DiffTempRoot 返回外部 diff 临时文件根目录（系统临时目录下 workbench-diff）。
func DiffTempRoot() string {
	if diffTempRootOverride != "" {
		return diffTempRootOverride
	}
	return filepath.Join(os.TempDir(), "workbench-diff")
}

// CleanupDiffTempDir 清理外部 diff 临时文件根目录（上次会话残留）。
// 删除失败静默忽略：文件可能仍被外部 diff 工具占用（Windows 文件锁），
// 留待下次启动重试。
func CleanupDiffTempDir() {
	_ = os.RemoveAll(DiffTempRoot())
}

// CreateDiffTempDir 创建一次外部 diff 会话的临时目录，返回目录路径。
// os.MkdirTemp 保证并发/连续调用原子唯一，避免时间戳粒度撞名互覆。
func CreateDiffTempDir() (string, error) {
	dir, err := os.MkdirTemp(DiffTempRoot(), "")
	if err != nil {
		return "", fmt.Errorf("创建外部 diff 临时目录失败: %w", err)
	}
	return dir, nil
}

// WriteDiffTempFile 将一侧版本内容写入临时目录的 side 子目录（left/right），
// 保留原文件名，返回可直接传给外部 diff 工具的文件绝对路径。
func WriteDiffTempFile(tempDir, side, fileName, content string) (string, error) {
	if tempDir == "" {
		return "", fmt.Errorf("临时目录不能为空")
	}
	if fileName == "" {
		return "", fmt.Errorf("临时文件名不能为空")
	}
	dir := filepath.Join(tempDir, side)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	return path, nil
}

// RenderDiffArgsTemplate 将外部 diff 工具的参数模板按空白分词，
// 替换 {left} / {right} 占位符为实际文件路径，返回 exec.Command 参数切片。
// 占位符替换在分词后进行，含空格的路径整体作为单个参数传递，无需引号包裹。
// 模板须同时包含 {left} 与 {right}，否则视为配置无效。
func RenderDiffArgsTemplate(template, left, right string) ([]string, error) {
	tokens := strings.Fields(template)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("参数模板为空")
	}
	if !strings.Contains(template, "{left}") || !strings.Contains(template, "{right}") {
		return nil, fmt.Errorf("参数模板须包含 {left} 与 {right} 占位符")
	}
	args := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.ReplaceAll(token, "{left}", left)
		token = strings.ReplaceAll(token, "{right}", right)
		args = append(args, token)
	}
	return args, nil
}
