package service

import (
	"encoding/json"
	"strings"
	"testing"

	"workbench/model"
)

// === parseStreamLine structured_output 提取测试（方案 C）===

// TestParseStreamLine_StructuredOutput result 事件携带 structured_output 字段时提取为 json.RawMessage。
// 实测 claude --json-schema 在 result 事件回 structured_output，与自由文本 result 解耦。
func TestParseStreamLine_StructuredOutput(t *testing.T) {
	line := `{"type":"result","subtype":"success","session_id":"s4","result":"done","structured_output":{"candidates":[{"type":"feat","description":"add login"}]}}`
	_, isResult, _, _, structuredOutput := parseStreamLine(line)
	if !isResult {
		t.Error("result 事件应标记终态")
	}
	if len(structuredOutput) == 0 {
		t.Fatal("应提取 structured_output 字段")
	}
	if !strings.Contains(string(structuredOutput), "feat") {
		t.Errorf("structured_output 应含 candidates 内容，got %s", structuredOutput)
	}
	// 验证提取的是合法 JSON
	var parsed map[string]any
	if err := json.Unmarshal(structuredOutput, &parsed); err != nil {
		t.Errorf("structured_output 应为合法 JSON: %v", err)
	}
}

// TestParseStreamLine_NoStructuredOutput result 事件无 structured_output 字段时返回 nil（未配 --json-schema）。
func TestParseStreamLine_NoStructuredOutput(t *testing.T) {
	line := `{"type":"result","subtype":"success","session_id":"s5","result":"done"}`
	_, isResult, _, _, structuredOutput := parseStreamLine(line)
	if !isResult {
		t.Error("result 事件应标记终态")
	}
	if structuredOutput != nil && len(structuredOutput) > 0 {
		t.Errorf("无 structured_output 字段应返回空，got %s", structuredOutput)
	}
}

// === buildClaudeArgs --json-schema 注入测试 ===

// TestBuildClaudeArgs_JsonSchema OutputSchema 非空时追加 --json-schema <schema>。
func TestBuildClaudeArgs_JsonSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"candidates":{"type":"array"}}}`)
	fn := &model.AiFunction{ID: "f1", OutputSchema: schema}
	args := buildClaudeArgs(fn, "p", "")
	found := false
	for i, a := range args {
		if a == "--json-schema" && i+1 < len(args) && args[i+1] == string(schema) {
			found = true
		}
	}
	if !found {
		t.Errorf("OutputSchema 非空应追加 --json-schema，got %v", args)
	}
}

// TestBuildClaudeArgs_NoJsonSchema OutputSchema 为空时不加 --json-schema（兼容现有 skill 零回归）。
func TestBuildClaudeArgs_NoJsonSchema(t *testing.T) {
	fn := &model.AiFunction{ID: "f1"} // OutputSchema nil
	args := buildClaudeArgs(fn, "p", "")
	for _, a := range args {
		if a == "--json-schema" {
			t.Errorf("OutputSchema 为空不应加 --json-schema，got %v", args)
		}
	}
}

// === defaultAiFunctions 新增 skill 测试 ===

// TestDefaultAiFunctions_IncludesCommitMessage defaultAiFunctions 含 commit-message skill 配置项 + OutputSchema。
func TestDefaultAiFunctions_IncludesCommitMessage(t *testing.T) {
	funcs := defaultAiFunctions()
	var cm *model.AiFunction
	for _, f := range funcs {
		if f.ID == "commit-message" {
			cm = f
		}
	}
	if cm == nil {
		t.Fatal("defaultAiFunctions 应含 commit-message skill")
	}
	if len(cm.OutputSchema) == 0 {
		t.Error("commit-message 应配 OutputSchema（候选数组 schema）")
	}
	if cm.Params == nil || cm.Params.Type != "form" {
		t.Error("commit-message 应 form 类型参数")
	}
	if cm.Params.PromptTemplate == "" {
		t.Error("commit-message 应有 PromptTemplate")
	}
	if !strings.Contains(cm.Params.PromptTemplate, "{{diff}}") {
		t.Error("commit-message PromptTemplate 应含 {{diff}} 占位符")
	}
	// OutputSchema 应为合法 JSON schema
	var schema map[string]any
	if err := json.Unmarshal(cm.OutputSchema, &schema); err != nil {
		t.Errorf("commit-message OutputSchema 应为合法 JSON: %v", err)
	}
}

// TestDefaultAiFunctions_IncludesCodeReview defaultAiFunctions 含 code-review skill 配置项 + OutputSchema。
func TestDefaultAiFunctions_IncludesCodeReview(t *testing.T) {
	funcs := defaultAiFunctions()
	var cr *model.AiFunction
	for _, f := range funcs {
		if f.ID == "code-review" {
			cr = f
		}
	}
	if cr == nil {
		t.Fatal("defaultAiFunctions 应含 code-review skill")
	}
	if len(cr.OutputSchema) == 0 {
		t.Error("code-review 应配 OutputSchema（问题清单 schema）")
	}
	if cr.Params == nil || cr.Params.Type != "form" {
		t.Error("code-review 应 form 类型参数")
	}
	if !strings.Contains(cr.Params.PromptTemplate, "{{diff}}") {
		t.Error("code-review PromptTemplate 应含 {{diff}} 占位符")
	}
	// OutputSchema 应含 issues 数组 + severity/category enum 约束
	schemaStr := string(cr.OutputSchema)
	if !strings.Contains(schemaStr, "issues") {
		t.Error("code-review OutputSchema 应含 issues 数组")
	}
	if !strings.Contains(schemaStr, "critical") || !strings.Contains(schemaStr, "warning") {
		t.Error("code-review OutputSchema 应含 severity enum（critical/warning/info）")
	}
}
