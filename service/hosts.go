package service

import (
	"workbench/model"
	"workbench/util"
)

// HostsService hosts 文件查看服务（工具箱·hosts 文件）：本机 hosts 只读读取 + 行解析编排。
type HostsService struct {
	path string // hosts 文件路径（生产为 util.HostsFilePath 平台常量；单测注入 fixture 路径）
}

// NewHostsService 创建 hosts 查看服务（生产路径：本机 hosts 平台路径）。
func NewHostsService() *HostsService {
	return &HostsService{path: util.HostsFilePath}
}

// GetHosts 读取并解析本机 hosts 文件（纯只读，无编辑能力）。
//
// 失败形态：读取失败不作为整体异常返回——用户可读错误消息写入 HostsView.Error 字段
// （对齐 EnvVarGroup.Error 单来源失败形态），前端组级 warning 行就地提示，仅 slog.Warn
// 留痕，不产生面向用户的整体错误码。
func (s *HostsService) GetHosts() (*model.HostsView, error) {
	entries, err := util.ReadHostsFile(s.path)
	if err != nil {
		Logger().Warn("hosts read failed", "path", s.path, "err", err)
		return &model.HostsView{
			Path: s.path,
			// sourceFailureMessage 复用 env.go 的分组 Error 提炼：AppError 取用户可读
			// Message（不含错误码前缀），裸错误拼兜底前缀——同款单来源失败形态语义
			Entries: []model.HostsEntry{},
			Error:   sourceFailureMessage(err),
		}, nil
	}
	return &model.HostsView{
		Path:    s.path,
		Entries: buildHostsEntries(entries),
	}, nil
}

// buildHostsEntries util 解析条目转展示 DTO：字段一一对应透传（Kind/IP/Hostnames/
// Comment/Raw），Hostnames 补非 nil 使前端拿到空数组而非 null。
func buildHostsEntries(entries []util.HostsEntry) []model.HostsEntry {
	infos := make([]model.HostsEntry, 0, len(entries))
	for _, e := range entries {
		hostnames := e.Hostnames
		if hostnames == nil {
			hostnames = []string{}
		}
		infos = append(infos, model.HostsEntry{
			Kind:      e.Kind,
			IP:        e.IP,
			Hostnames: hostnames,
			Comment:   e.Comment,
			Raw:       e.Raw,
		})
	}
	return infos
}
