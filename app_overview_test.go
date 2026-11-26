package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"workbench/server"
)

// TestServeRPC_GetOverviewSnapshotAvailableForBrowser 概览快照对浏览器通道可用：
// 未被 RPCHiddenMethods 屏蔽（三个数据源现有面板均已对浏览器开放，本任务不新增
// 敏感字段），且载荷结构保持「三域 + 档位回显」不含审计/攻击趋势字段（隐私红线）。
func TestServeRPC_GetOverviewSnapshotAvailableForBrowser(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)
	m := app.webServe
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	token, err := server.LoadOrCreateToken(serveTokenFile)
	if err != nil {
		t.Fatalf("读访问令牌: %v", err)
	}
	h := m.handler
	if h == nil {
		t.Fatal("前置：装配产物 handler 应就绪")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/rpc",
		strings.NewReader(`{"method":"GetOverviewSnapshot","args":["week",true]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("RPC 应 HTTP 200, got %d %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Ok   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析 RPC 响应失败: %v（原文 %s）", err, rec.Body.String())
	}
	if !resp.Ok {
		t.Fatalf("GetOverviewSnapshot 不应被浏览器通道屏蔽: %s", rec.Body.String())
	}

	// 顶层字段闭集：概览快照只含生成时间、档位回显与三个数据域，
	// 不得混入审计/攻击趋势（R7 隐私红线回归）。
	var top map[string]json.RawMessage
	if err := json.Unmarshal(resp.Data, &top); err != nil {
		t.Fatalf("解析快照载荷失败: %v", err)
	}
	keys := make([]string, 0, len(top))
	for k := range top {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"ai", "generatedAt", "range", "repos", "todos"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("快照顶层字段 = %v，期望 %v（不得包含审计/攻击趋势字段）", keys, want)
	}
	if !strings.Contains(string(top["range"]), `"week"`) {
		t.Errorf("range 应回显入参 week, got %s", top["range"])
	}
}
