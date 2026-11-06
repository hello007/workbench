package service

import (
	"sync"

	"workbench/model"
	"workbench/util"
)

// SettingsService 应用设置服务（data/settings.json）。
//
// 缓存语义（素材缩略图 PR3 起）：Load 首次读盘后写入内存缓存，后续 Load 命中
// 直接返回缓存；Save 落盘成功后更新缓存——缩略图读路径高频消费设置（画廊逐
// 卡片 GetAssetThumb 都要解析分辨率档位），不能每次读盘。
//
// 语义变化登记：设置改为进程内缓存后，运行期手改 settings.json 文件不再即时
// 生效（须重启应用；本就要求改 data 文件前关闭 workbench.exe，与 docs/spec/
// data-atomic-write.md 整体覆盖模型口径一致）。
//
// 并发口径：Load 返回共享指针，消费方只读禁改（原地修改会与并发读者竞态——
// 需要改动的调用点先复制再改，见 app_webserve.go SetWebServeConfig）；写路径
// 唯一入口 Save（整体替换，成功后整体换新缓存指针）。mu 仅保护缓存指针的读写
// 与替换，不保护指针指向内容的字段（依赖「消费方只读 + Save 整体换指针」纪律）。
type SettingsService struct {
	configPath string

	mu     sync.Mutex
	cached *model.AppSettings // 内存缓存：Load 命中直接返回，Save 成功后更新
	// assetsThumbMigrated 本次首次 Load 是否发生 small 档默认宽度迁移
	// （320 → 640，2026-11）：迁移只改内存配置，磁盘上的旧分辨率 PNG 仍然存在，
	// 须由启动装配消费本标记触发一次清档重建，否则 GetAssetThumb 继续返回旧图。
	// 消费即清零（一次性）。
	assetsThumbMigrated bool
}

func NewSettingsService(configPath string) *SettingsService {
	return &SettingsService{configPath: configPath}
}

func (s *SettingsService) Load() (*model.AppSettings, error) {
	s.mu.Lock()
	if s.cached != nil {
		settings := s.cached
		s.mu.Unlock()
		return settings, nil
	}
	s.mu.Unlock()

	settings := &model.AppSettings{}
	if util.FileExists(s.configPath) {
		if err := util.LoadJSON(s.configPath, settings); err != nil {
			// 配置损坏降级：吞错回退空设置（各字段走空值语义），不阻塞启动
			settings = &model.AppSettings{}
		}
	}
	// 迁移前原始 small 档宽度（Ensure 前快照）：用于判定本次加载是否命中存量
	// 默认值迁移（320 → 640），供启动装配触发旧分辨率缓存重建
	rawSmallWidth := 0
	if settings.Assets != nil {
		rawSmallWidth = settings.Assets.SmallWidth
	}
	// 各设置段默认值补全与越界收敛（Ensure 串联单点 EnsureAllDefaults，新增设置
	// 段在 model.AppSettings.EnsureAllDefaults 一处加行），保证「Load 返回的配置段
	// 始终完整可用」不变式：桌面 startup、--serve 地址解析、前端 GetSettings 与
	// 各服务装配拿到的配置段始终完整可用
	settings.EnsureAllDefaults()

	// 存量 small 档默认宽度迁移结果写回磁盘：Ensure 已把旧默认 320 升级为 640，
	// 若不写回，每次启动都会重新判定迁移并触发一次全量清档重建。写回失败仅告警：
	// 本次仍内存生效；下次启动再迁移一遍，重复一次重导（幂等可自愈）。
	migrated := rawSmallWidth == model.LegacyAssetThumbSmallWidth &&
		settings.Assets != nil && settings.Assets.SmallWidth == model.DefaultAssetThumbSmallWidth
	if migrated {
		if err := util.SaveJSON(s.configPath, settings); err != nil {
			Logger().Warn("设置迁移写回失败", "path", s.configPath, "err", err)
		}
	}

	s.mu.Lock()
	s.cached = settings
	s.assetsThumbMigrated = migrated
	s.mu.Unlock()
	return settings, nil
}

// ConsumeAssetsThumbMigration 一次性消费 small 档默认宽度迁移标记：返回本次首次
// Load 是否发生 320 → 640 迁移，消费后清零。启动装配据此调用
// ApplyThumbsSettingsChange（prev/cur 宽度差异触发清档重导），保证旧 320 PNG
// 被新分辨率产物替换。
func (s *SettingsService) ConsumeAssetsThumbMigration() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	migrated := s.assetsThumbMigrated
	s.assetsThumbMigrated = false
	return migrated
}

func (s *SettingsService) Save(settings *model.AppSettings) error {
	// 保存前补全各段默认值（与 Load 同一 Ensure 串联单点 EnsureAllDefaults）：
	// 缓存命中路径不再经过 Load 的补全链路，部分字段的保存入参若直接入缓存会
	// 破坏「Load 返回的配置段始终完整可用」不变式（如 webServe 段 nil 时 --serve
	// 地址解析直接解引用 settings.WebServe.BindAddress 会 panic）。
	settings.EnsureAllDefaults()
	// 锁跨「落盘 + 缓存换新」全程持有：两并发 Save（桌面 + 浏览器通道）若只锁
	// 缓存赋值，磁盘写入序与缓存换新序可相反（缓存=A 而磁盘=B，脱钩持续到下次
	// 保存/重启）。写路径低频，全程持锁无性能顾虑。
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := util.SaveJSON(s.configPath, settings); err != nil {
		return err
	}
	s.cached = settings
	return nil
}
