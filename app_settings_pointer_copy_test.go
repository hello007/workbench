package main

import (
	"testing"

	"workbench/model"
)

// 设置写入口指针段私有拷贝（model.AppSettings.PrivateCopy 单点化）回归：
// SetWebServeConfig 曾漏拷 IPBan/Audit 两段、SetIPBanConfig 曾漏拷 Audit 段——
// 共享缓存指针经 Save 入口 Ensure 的 normalize 无条件赋值与并发读者构成无同步
// 写读竞态。断言口径：Save 换新缓存后，GetSettings 返回的段指针须与调用前缓存
// 的段指针不同址（漏拷形态下两指针相同）且内容保留。

// TestApp_SetIPBanConfig_PrivateCopiesAuditSegment SetIPBanConfig 补拷 Audit 段
// 独立性（PrivateCopy 收口的遗留之一）：预置 Audit 段后调 SetIPBanConfig，
// 新缓存中 Audit 段与基底缓存段指针不同址且内容保留。
func TestApp_SetIPBanConfig_PrivateCopiesAuditSegment(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	if err := app.SetAuditConfig(8, 4); err != nil {
		t.Fatalf("SetAuditConfig: %v", err)
	}
	baseAudit := app.GetSettings().Audit
	if baseAudit == nil || baseAudit.MaxBytesMB != 8 || baseAudit.Depth != 4 {
		t.Fatalf("预置 Audit 段失败, got %+v", baseAudit)
	}

	if err := app.SetIPBanConfig(5, 10, 5, 60, 3, 7, nil); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	gotAudit := app.GetSettings().Audit
	if gotAudit == baseAudit {
		t.Error("Audit 段应经 PrivateCopy 私有化（与基底缓存段指针不同址）")
	}
	if gotAudit == nil || gotAudit.MaxBytesMB != 8 || gotAudit.Depth != 4 {
		t.Errorf("Audit 段内容应保留, got %+v", gotAudit)
	}
}

// TestApp_SetWebServeConfig_PrivateCopiesIPBanAndAuditSegments SetWebServeConfig
// 补拷 IPBan/Audit 两段独立性（PrivateCopy 收口的遗留之一）+ TLS 三字段保留
// 回归：预置 TLS 与 ipban/audit 段后调 SetWebServeConfig，两段与新缓存段指针
// 不同址且内容保留，TLS 三字段保留磁盘现值（本方法只覆写 enabled/bindAddress）。
func TestApp_SetWebServeConfig_PrivateCopiesIPBanAndAuditSegments(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	// 预置基底设置（TLS 三字段 + ipban/audit 段；保存对象为测试局部私有构造，
	// 不引入额外的共享指针形态）
	base := model.AppSettings{}
	base.EnsureAllDefaults()
	base.WebServe.TLS = true
	base.WebServe.TLSCertPath = "cert.pem"
	base.WebServe.TLSKeyPath = "key.pem"
	base.IPBan = &model.IPBanSettings{Threshold: 20}
	base.Audit = &model.AuditSettings{MaxBytesMB: 8, Depth: 4}
	if err := app.settingsSvc.Save(&base); err != nil {
		t.Fatalf("预置基底设置: %v", err)
	}
	baseIPBan := app.GetSettings().IPBan
	baseAudit := app.GetSettings().Audit

	if err := app.SetWebServeConfig(false, "127.0.0.1:36115"); err != nil {
		t.Fatalf("SetWebServeConfig: %v", err)
	}

	cfg := app.GetSettings()
	if cfg.IPBan == baseIPBan {
		t.Error("ipban 段应经 PrivateCopy 私有化（与基底缓存段指针不同址）")
	}
	if cfg.Audit == baseAudit {
		t.Error("audit 段应经 PrivateCopy 私有化（与基底缓存段指针不同址）")
	}
	if cfg.IPBan == nil || cfg.IPBan.Threshold != 20 {
		t.Errorf("ipban 段内容应保留, got %+v", cfg.IPBan)
	}
	if cfg.Audit == nil || cfg.Audit.MaxBytesMB != 8 || cfg.Audit.Depth != 4 {
		t.Errorf("audit 段内容应保留, got %+v", cfg.Audit)
	}
	// TLS 三字段保留回归（既有行为，防改写破坏）：重建 webServe 段时
	// 若丢 TLS 字段，用户保存地址即丢 TLS 配置
	if cfg.WebServe == nil || !cfg.WebServe.TLS ||
		cfg.WebServe.TLSCertPath != "cert.pem" || cfg.WebServe.TLSKeyPath != "key.pem" {
		t.Errorf("TLS 三字段应保留磁盘现值, got %+v", cfg.WebServe)
	}
	if cfg.WebServe.Enabled {
		t.Errorf("webServe 段应按入参写为关闭态, got %+v", cfg.WebServe)
	}
}
