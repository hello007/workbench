package main

import (
	"os"
	"strings"
	"testing"

	"workbench/model"
	"workbench/service"
)

// 连接管控十一期测试：SetAuditConfig「保存+生效」单点（持久化 + AuditLogger
// ApplyConfig 热更）。

// TestApp_SetAuditConfig_SaveAndHotApply 保存链：入参经 normalize 持久化设置段，
// 并热更 AuditLogger 轮转参数（maxBytesMB→字节换算在 App 层完成、depth 直传）。
// 热更生效走行为断言（AuditLogger 字段为 service 包未导出，main 包测试不可锁内
// 直读）：热更为 clamp 下限 1MB + depth 2 后，写 3 个大单行审计事件即触发两轮
// 轮转——maxBytes 生效（远小于默认 5MB 即轮转）且 depth 生效（链可达 .2 档，
// clamp 下限 2 保证第三行必产生 .2；字段直读形态由 service 层
// TestAuditLogger_ApplyConfig 覆盖）。
func TestApp_SetAuditConfig_SaveAndHotApply(t *testing.T) {
	app, dataDir := newWebServeTestAppDir(t)

	if err := app.SetAuditConfig(1, 2); err != nil {
		t.Fatalf("SetAuditConfig: %v", err)
	}
	// 持久化断言：GetSettings（缓存已换新）含归一后段值
	got := app.GetSettings().Audit
	if got == nil || got.MaxBytesMB != 1 || got.Depth != 2 {
		t.Fatalf("设置段应持久化为归一值, got %+v", got)
	}

	// 热更生效断言：3 个 ~600KB 单行（userAgent 携带）驱动 1MB 上限两轮轮转
	bigUA := strings.Repeat("u", 600*1024)
	for i := 0; i < 3; i++ {
		app.webAuditSvc.Log(service.AuditEventAuthSuccess, "tok-"+string(rune('a'+i)), "", "127.0.0.1:1", bigUA)
	}
	for _, name := range []string{"web_audit.log.2", "web_audit.log.1", "web_audit.log"} {
		if _, err := os.Stat(dataDir + string(os.PathSeparator) + name); err != nil {
			t.Fatalf("热更后轮转链应产生 %s: %v", name, err)
		}
	}
}

// TestApp_SetAuditConfig_ClampsInvalidInput 越界入参 clamp 收敛（前端已挡边界，
// 后端兜底手改/异常形态）：零值回默认（0/0 → 5/6）、越上限收敛（99/99 → 20/10）、
// 越下限收敛（负值 → 1/2）。
func TestApp_SetAuditConfig_ClampsInvalidInput(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	if err := app.SetAuditConfig(0, 0); err != nil {
		t.Fatalf("SetAuditConfig: %v", err)
	}
	got := app.GetSettings().Audit
	if got.MaxBytesMB != model.DefaultAuditMaxBytesMB || got.Depth != model.DefaultAuditDepth {
		t.Errorf("零值入参应回默认, got %+v", got)
	}

	if err := app.SetAuditConfig(99, 99); err != nil {
		t.Fatalf("SetAuditConfig: %v", err)
	}
	got = app.GetSettings().Audit
	if got.MaxBytesMB != model.MaxAuditMaxBytesMB || got.Depth != model.MaxAuditDepth {
		t.Errorf("越上限入参应收敛, got %+v", got)
	}

	if err := app.SetAuditConfig(-3, 1); err != nil {
		t.Fatalf("SetAuditConfig: %v", err)
	}
	got = app.GetSettings().Audit
	if got.MaxBytesMB != model.MinAuditMaxBytesMB || got.Depth != model.MinAuditDepth {
		t.Errorf("越下限入参应收敛, got %+v", got)
	}
}

// TestApp_SetAuditConfig_PreservesOtherSections 指针段保留：预置 webServe/ipban
// 段后调 SetAuditConfig，两段不被覆丢（合并写基底保留纪律——本方法对基底指针段
// 全部私有拷贝，替代 SetIPBanConfig 时代漏拷后段的既有形态）。
func TestApp_SetAuditConfig_PreservesOtherSections(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	// 预置 webServe 段（显式 enabled=false 区分默认值形态）与 ipban 段（threshold=5）
	if err := app.SetWebServeConfig(false, "127.0.0.1:36115"); err != nil {
		t.Fatalf("SetWebServeConfig: %v", err)
	}
	if err := app.SetIPBanConfig(5, 10, 5, 60, 3, 7, nil); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	if err := app.SetAuditConfig(8, 4); err != nil {
		t.Fatalf("SetAuditConfig: %v", err)
	}
	cfg := app.GetSettings()
	if cfg.WebServe == nil || cfg.WebServe.Enabled {
		t.Errorf("webServe 段应保留（enabled=false）, got %+v", cfg.WebServe)
	}
	if cfg.IPBan == nil || cfg.IPBan.Threshold != 5 {
		t.Errorf("ipban 段应保留, got %+v", cfg.IPBan)
	}
	if cfg.Audit == nil || cfg.Audit.MaxBytesMB != 8 || cfg.Audit.Depth != 4 {
		t.Errorf("audit 段应写入, got %+v", cfg.Audit)
	}
}
