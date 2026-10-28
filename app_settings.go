package main

import (
	"workbench/model"
)

// ===== 设置域 =====

// GetSettings 获取应用设置
//
// Load 返回的对象恒已补全默认值（service/settings.go：缓存 miss 分支 Ensure 后
// 才写入缓存，Save 成功换缓存前同样 Ensure——「Load 返回的配置段始终完整可用」
// 是 SettingsService 的不变式），故正常路径不再对返回对象做 Ensure：EnsureXxx
// 含无条件字段赋值，作用于缓存命中的共享指针时与并发读者构成 data race（写值
// 虽等于已发布值，仍是无同步的并发写读）。settingsSvc 异常降级分支（就地构造的
// 私有新对象）经 EnsureAllDefaults 单点补全全部设置段（webServe 段 / 终端外观
// 三字段 / 素材缩略图段 / IP 封禁段 / 审计留存段），覆盖语义与 Load 正常路径
// 一致，新增设置段在该单点加行后本分支自动覆盖。
func (a *App) GetSettings() *model.AppSettings {
	settings, err := a.settingsSvc.Load()
	if err != nil {
		settings = &model.AppSettings{}
		// settingsSvc 异常降级分支：私有新对象补全全部设置段默认值（Ensure
		// 串联单点，与 Load/Save 同源），保证前端拿到的配置段始终完整
		settings.EnsureAllDefaults()
	}
	return settings
}

// SaveSettings 保存应用设置
func (a *App) SaveSettings(settings *model.AppSettings) error {
	// prev 取保存前内存态（缓存命中即真实前值）。已登记口径：Load 在「缓存 miss
	// 且磁盘读失败」时静默降级为空设置（无错误信号可判），此刻 prev.Assets 退化
	// 为默认值，存量非默认宽度会被误判「分辨率变更」触发清档重导——触发前提是
	// settings.json 损坏，概率极窄且后果仅为一次全量重导（自愈），接受不设防。
	prev, _ := a.settingsSvc.Load()
	settings.EnsureAssetsThumbsDefaults()
	if err := a.settingsSvc.Save(settings); err != nil {
		return err
	}
	// 素材缩略图设置变更应用（D2）：分辨率清档重入队 / 上限触发惰性淘汰 / 开关
	// 影响下次扫描补缺入队。异步，不阻塞保存返回。
	a.assetsSvc.ApplyThumbsSettingsChange(
		model.NormalizeAssetThumbsSettings(prev.Assets),
		*settings.Assets,
	)
	return nil
}
