package service

import (
	"workbench/model"
	"workbench/util"
)

type SettingsService struct {
	configPath string
}

func NewSettingsService(configPath string) *SettingsService {
	return &SettingsService{configPath: configPath}
}

func (s *SettingsService) Load() (*model.AppSettings, error) {
	settings := &model.AppSettings{}
	if util.FileExists(s.configPath) {
		if err := util.LoadJSON(s.configPath, settings); err != nil {
			// 配置损坏降级：吞错回退空设置（各字段走空值语义），不阻塞启动
			settings = &model.AppSettings{}
		}
	}
	// webServe 段默认值补全（段缺失/地址为空 → 默认开启 + 回环地址），
	// 保证桌面 startup 与 --serve 地址解析拿到的配置始终完整可用
	settings.EnsureWebServeDefaults()
	return settings, nil
}

func (s *SettingsService) Save(settings *model.AppSettings) error {
	return util.SaveJSON(s.configPath, settings)
}
