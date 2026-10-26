package main

import (
	"testing"

	"workbench/model"
)

// 连接管控六期测试：SetIPBanConfig「保存+生效」单点（持久化 + ApplyConfig 热更）。

// TestApp_SetIPBanConfig_SaveAndHotApply 保存链：入参经 normalize 持久化设置段，
// 并热更运行中服务（新阈值对后续 RecordFailure 即时生效，无需重启）。
func TestApp_SetIPBanConfig_SaveAndHotApply(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)
	ip := testAuthFailIP

	if err := app.SetIPBanConfig(3, 10, 5, 60); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	// 持久化断言：GetSettings（缓存已换新）含归一后段值
	got := app.GetSettings().IPBan
	if got == nil || got.Threshold != 3 || got.WindowMinutes != 10 ||
		got.DurationMinutes != 5 || got.MaxBanDurationMinutes != 60 {
		t.Fatalf("设置段应持久化为归一值, got %+v", got)
	}
	// 热更断言：新阈值 3 下 3 次失败即触发（默认阈值 10 需 10 次）
	for i := 0; i < 3; i++ {
		app.ipBanSvc.RecordFailure(ip)
	}
	bans := app.ListIPBans()
	if len(bans) != 1 || bans[0].Strike != 1 {
		t.Fatalf("热更后 3 次失败应触发封禁, got %+v", bans)
	}
}

// TestApp_SetIPBanConfig_ClampsInvalidInput 越界入参 clamp 收敛（前端已挡边界，
// 后端兜底手改/异常形态）：threshold 1→3、window 999→120、duration 999→60、
// 封顶 1→5→(低于基础时长抬到)60。
func TestApp_SetIPBanConfig_ClampsInvalidInput(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	if err := app.SetIPBanConfig(1, 999, 999, 1); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	got := app.GetSettings().IPBan
	if got.Threshold != model.MinIPBanThreshold ||
		got.WindowMinutes != model.MaxIPBanWindowMinutes ||
		got.DurationMinutes != model.MaxIPBanDurationMinutes ||
		got.MaxBanDurationMinutes != model.MaxIPBanDurationMinutes {
		t.Errorf("越界入参应 clamp 收敛, got %+v", got)
	}
}

// TestApp_SetIPBanConfig_PreservesOtherSections 指针段保留：预置 webServe 段后
// 调 SetIPBanConfig，webServe 段不被覆丢（合并写基底保留纪律）。
func TestApp_SetIPBanConfig_PreservesOtherSections(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	// 预置 webServe 段（显式 enabled=false 区分默认值形态）
	if err := app.SetWebServeConfig(false, "127.0.0.1:36115"); err != nil {
		t.Fatalf("SetWebServeConfig: %v", err)
	}
	if err := app.SetIPBanConfig(5, 10, 5, 60); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	cfg := app.GetSettings()
	if cfg.WebServe == nil || cfg.WebServe.Enabled {
		t.Errorf("webServe 段应保留（enabled=false）, got %+v", cfg.WebServe)
	}
	if cfg.IPBan == nil || cfg.IPBan.Threshold != 5 {
		t.Errorf("ipban 段应写入, got %+v", cfg.IPBan)
	}
}
