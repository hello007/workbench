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
	if !util.FileExists(s.configPath) {
		return settings, nil
	}
	err := util.LoadJSON(s.configPath, settings)
	if err != nil {
		return settings, nil
	}
	return settings, nil
}

func (s *SettingsService) Save(settings *model.AppSettings) error {
	return util.SaveJSON(s.configPath, settings)
}
