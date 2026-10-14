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
	// webServe 段默认值补全（段缺失/地址为空 → 默认开启 + 回环地址），
	// 保证桌面 startup 与 --serve 地址解析拿到的配置始终完整可用
	settings.EnsureWebServeDefaults()
	// 终端外观三字段默认值补全与越界收敛（零值 → 默认，越界 clamp），
	// 保证前端 GetSettings 拿到的字号/字体/回滚配置始终可用
	settings.EnsureTerminalDefaults()
	// 素材缩略图段默认值补全与越界收敛（段缺失 → 五项全默认），
	// 保证缩略图链路消费方拿到的配置段始终完整可用
	settings.EnsureAssetsThumbsDefaults()

	s.mu.Lock()
	s.cached = settings
	s.mu.Unlock()
	return settings, nil
}

func (s *SettingsService) Save(settings *model.AppSettings) error {
	// 保存前补全各段默认值（对齐 Load 链路三处 Ensure）：缓存命中路径不再经过
	// Load 的补全链路，部分字段的保存入参若直接入缓存会破坏「Load 返回的配置段
	// 始终完整可用」不变式（如 webServe 段 nil 时 --serve 地址解析直接解引用
	// settings.WebServe.BindAddress 会 panic）。
	settings.EnsureWebServeDefaults()
	settings.EnsureTerminalDefaults()
	settings.EnsureAssetsThumbsDefaults()
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
