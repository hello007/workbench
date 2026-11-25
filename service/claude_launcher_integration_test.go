//go:build integration

package service

// claude 启动链路端到端集成测试（真调 claude CLI，需网络与登录态）：
// 验证 resolveClaudeCommand 绕开 .cmd shim 的 cmd.exe 转发后，
// --json-schema 的 JSON 参数不再被破坏（真机缺陷：报「--json-schema is not valid JSON」exit 1）
// 且 stdin prompt 生效（Windows 命令行 32K 上限规避）。

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestIntegration_ClaudeLaunchWithJsonSchema(t *testing.T) {
	name, prefix := resolveClaudeCommand()
	if name == "claude" && len(prefix) == 0 {
		t.Skip("claude 未解析出真实可执行（未安装或非 .cmd shim 形态），跳过真调")
	}

	// 本机 claude 的凭据/网关常配在用户级 %USERPROFILE%/.claude/settings.json 的 env 段，
	// exec.Command 只继承父进程环境拿不到该段，会实测报「Not logged in · Please run /login」。
	// 此处显式合并注入（settings env 覆盖父进程同名 key）。仅输出 key 名与计数，绝不打印 env 值。
	_, settingsEnv := readClaudeUserSettings()
	if len(settingsEnv) > 0 {
		t.Logf("已注入 %d 项 claude settings.json env（key: %s）",
			len(settingsEnv), strings.Join(sortedEnvKeys(settingsEnv), ", "))
	}
	if !claudeCredentialPresent(settingsEnv) {
		t.Skip("本机未配置 claude 凭据（settings.json env 与父进程环境均无 ANTHROPIC_AUTH_TOKEN/ANTHROPIC_API_KEY），跳过真调")
	}

	args := append(append([]string{}, prefix...), "-p", "--json-schema",
		`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`,
		"--output-format", "stream-json", "--verbose", "--model", "haiku",
		"--permission-mode", "default", "--disallowedTools", "AskUserQuestion")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = mergeClaudeEnv(os.Environ(), settingsEnv)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	go func() {
		_, _ = io.WriteString(stdin, "只回复JSON：ok 为 true")
		_ = stdin.Close()
	}()
	data, _ := io.ReadAll(stdout)
	out := string(data)
	if err := cmd.Wait(); err != nil {
		// 失败时输出末尾含根因（如 --json-schema is not valid JSON）
		t.Fatalf("claude 退出非 0: %v\n输出末尾: %s", err, tailForSmoke(out, 500))
	}
	if strings.Contains(out, "not valid JSON") {
		t.Fatalf("--json-schema 仍被破坏\n输出末尾: %s", tailForSmoke(out, 500))
	}
	if !strings.Contains(out, `"structured_output"`) {
		t.Fatalf("应有 structured_output 事件\n输出末尾: %s", tailForSmoke(out, 500))
	}
}

// claudeCredentialPresent 判定认证凭据是否存在：settings env 优先，其次父进程环境；
// 只判非空，绝不取值或输出。
func claudeCredentialPresent(settingsEnv map[string]string) bool {
	for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"} {
		if strings.TrimSpace(settingsEnv[key]) != "" {
			return true
		}
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

// sortedEnvKeys 返回排序后的 key 名（仅用于日志输出，避免打印敏感值）。
func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func tailForSmoke(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
