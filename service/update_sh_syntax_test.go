//go:build linux

package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestUpdateShScripts_ValidPosixSyntax 生成的 .sh 更新脚本必须是合法 POSIX sh。
// sh -n 仅做语法解析不执行任何命令，可拦截未来改动引入的 bashism（[[ ]]、((++)) 等
// 在 dash 下为语法错误）。运行方以 `sh <script>` 执行（Debian/Ubuntu 为 dash），
// 与脚本 shebang #!/bin/sh 一致。Windows 无 /bin/sh，故限 linux 构建。
func TestUpdateShScripts_ValidPosixSyntax(t *testing.T) {
	newExe := "/tmp/workbench-update/workbench.exe"
	currentExe := "/opt/workbench/workbench"
	pendingFile := "/tmp/workbench-update/pending_update.json"
	updateDir := "/tmp/workbench-update"

	scripts := map[string]string{
		"update.sh":       buildUpdateSh(1234, newExe, currentExe, pendingFile, updateDir),
		"apply-update.sh": buildApplySh(newExe, currentExe, pendingFile, updateDir),
	}
	for name, content := range scripts {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			t.Fatalf("写入 %s 失败: %v", name, err)
		}
		cmd := exec.Command("sh", "-n", path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s 不是合法 POSIX sh 脚本: %v\nsh -n 输出: %s", name, err, out)
		}
	}
}
