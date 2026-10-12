package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"workbench/model"
	"workbench/util"
)

// 来源取值常量。用**普通 string** 而非具名类型：wails generate 不为 `type X string`
// 生成 models.ts 类型别名，App.d.ts 引用后 npm run build 会报 MISSING_EXPORT
// （vitest 走 esbuild 不报类型错，极易漏，见 docs/spec/cross-layer-contracts.md）。
const (
	EnvSourceSystem  = "system"  // 系统级（Windows 注册表 HKLM；Linux 无此来源）
	EnvSourceUser    = "user"    // 用户级（Windows 注册表 HKCU；Linux 无此来源）
	EnvSourceProcess = "process" // 进程级（os.Environ，全平台可用）
)

// envSourceSpec 单个来源的枚举描述：来源标识 + 展示名 + 枚举入口。
type envSourceSpec struct {
	source    string
	name      string
	enumerate func(util.EnvSourceProvider) ([]util.EnvEntry, error)
}

// envSourceSpecs 来源枚举清单，顺序即返回分组顺序（固定 system → user → process）。
// Linux 下 SystemEnv/UserEnv 返回 util.ErrEnvSourceUnsupported，由 ListEnvVars 静默
// 跳过对应分组实现「UI 仅显示进程级」，故清单本身无需按 GOOS 分叉。
var envSourceSpecs = []envSourceSpec{
	{source: EnvSourceSystem, name: "系统环境变量", enumerate: func(p util.EnvSourceProvider) ([]util.EnvEntry, error) { return p.SystemEnv() }},
	{source: EnvSourceUser, name: "用户环境变量", enumerate: func(p util.EnvSourceProvider) ([]util.EnvEntry, error) { return p.UserEnv() }},
	{source: EnvSourceProcess, name: "进程环境变量", enumerate: func(p util.EnvSourceProvider) ([]util.EnvEntry, error) { return p.ProcessEnv() }},
}

// utilEnvSourceProvider 生产实现，转发到 util 包平台分叉函数
// （Windows 读注册表区分系统/用户级 + os.Environ 进程级；Linux 仅进程级）。
type utilEnvSourceProvider struct{}

func (utilEnvSourceProvider) SystemEnv() ([]util.EnvEntry, error) { return util.SystemEnv() }

func (utilEnvSourceProvider) UserEnv() ([]util.EnvEntry, error) { return util.UserEnv() }

func (utilEnvSourceProvider) ProcessEnv() ([]util.EnvEntry, error) { return util.ProcessEnv() }

// EnvService 环境变量查看服务（工具箱·环境变量）：多来源只读枚举 + 脱敏标记 + 组内排序。
type EnvService struct {
	provider util.EnvSourceProvider
}

// NewEnvService 创建环境变量查看服务（生产路径：真实 util 平台分叉实现）。
func NewEnvService() *EnvService {
	return &EnvService{provider: utilEnvSourceProvider{}}
}

// ListEnvVars 枚举环境变量并按来源分组返回（纯只读，无编辑能力）。
//
// 编排规则：
//   - 按固定来源顺序逐源枚举，单来源失败不拖垮整体——用户可读错误消息写入该分组
//     的 Error 字段（前端展示该来源不可用），同时 slog.Warn 留痕；
//   - 平台不支持的来源（Linux 系统/用户级）按 unsupported 哨兵错误静默跳过分组，
//     不计失败也不产生 Error 分组；
//   - 仅当全部枚举到的来源都真实失败时才返回整体 error（E_ENV_ENUM_FAILED，
//     前端引导重试）。
func (s *EnvService) ListEnvVars() ([]model.EnvVarGroup, error) {
	groups := make([]model.EnvVarGroup, 0, len(envSourceSpecs))
	successCount := 0
	var failures []error
	for _, spec := range envSourceSpecs {
		entries, err := spec.enumerate(s.provider)
		switch {
		case errors.Is(err, util.ErrEnvSourceUnsupported):
			// 平台无此来源（Linux 系统/用户级）：静默跳过分组，debug 留痕即可
			Logger().Debug("env source unsupported, skip group", "source", spec.source)
		case err != nil:
			failures = append(failures, err)
			Logger().Warn("env source enumerate failed", "source", spec.source, "err", err)
			groups = append(groups, model.EnvVarGroup{
				Source: spec.source,
				Name:   spec.name,
				Error:  sourceFailureMessage(err),
			})
		default:
			successCount++
			groups = append(groups, model.EnvVarGroup{
				Source: spec.source,
				Name:   spec.name,
				Vars:   buildEnvVarInfos(entries),
			})
		}
	}
	if successCount == 0 && len(failures) > 0 {
		// 全部来源真实失败（unsupported 跳过的来源不在 groups 亦不计失败）
		return nil, model.WrapAppError(model.ErrCodeEnvEnumFailed,
			"环境变量枚举失败，请重试", errors.Join(failures...))
	}
	return groups, nil
}

// sourceFailureMessage 提炼单来源失败的用户可读消息，写入 model.EnvVarGroup.Error：
// util 层已包 AppError 的取 Message（避免把 "[E_ENV_ENUM_FAILED]" 错误码前缀透给
// 展示层），裸错误拼兜底前缀。
func sourceFailureMessage(err error) string {
	var appErr *model.AppError
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return fmt.Sprintf("读取失败：%v", err)
}

// buildEnvVarInfos 条目转展示信息：Sensitive 由 util.IsSensitiveEnvName 判定
// （后端只打标记返回真实值，默认打码与点击临时显值由前端处理），组内按名称
// 大小写不敏感升序（决策 2026-10-13）。
func buildEnvVarInfos(entries []util.EnvEntry) []model.EnvVarInfo {
	infos := make([]model.EnvVarInfo, 0, len(entries))
	for _, entry := range entries {
		infos = append(infos, model.EnvVarInfo{
			Name:      entry.Name,
			Value:     entry.Value,
			Sensitive: util.IsSensitiveEnvName(entry.Name),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return strings.ToLower(infos[i].Name) < strings.ToLower(infos[j].Name)
	})
	return infos
}
