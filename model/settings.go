package model

// DefaultWebServeBindAddress 浏览器访问通道默认绑定地址（回环，仅本机浏览器可达）。
// 远程访问须显式改绑（设置页确认或 --listen 命令行覆盖），改绑非回环地址会向
// 局域网暴露整机文件、终端与 AI agent 能力，建议配合隧道使用。
const DefaultWebServeBindAddress = "127.0.0.1:36115"

// WebServeSettings 浏览器访问通道配置（settings.json 的 webServe 段）。
type WebServeSettings struct {
	Enabled     bool   `json:"enabled"`     // 桌面模式是否同开 HTTP 服务（默认 true）
	BindAddress string `json:"bindAddress"` // HTTP 监听地址 host:port（默认回环 127.0.0.1:36115）
	// TLS 是否启用 HTTPS/WSS（默认 false = 纯 HTTP，向后兼容老配置零值语义）。
	// TLS 是显式决策项，EnsureWebServeDefaults 不补默认值。
	TLS bool `json:"tls"`
	// TLSCertPath / TLSKeyPath 用户自带证书的 PEM 文件路径（tls=true 时生效）。
	// 双空 = 自签引导（首次自动生成并持久化 data/tls_cert.pem / data/tls_key.pem）；
	// 仅填其一为无效配置，启动时报 E_TLS_CERT_INVALID。
	TLSCertPath string `json:"tlsCertPath,omitempty"`
	TLSKeyPath  string `json:"tlsKeyPath,omitempty"`
}

// WebServeConfig 浏览器访问通道配置与运行状态（GetWebServeConfig 返回）。
// 在持久化配置之上附带派生信息：服务运行状态与候选访问地址（回环 + 本机局域网 IP）。
type WebServeConfig struct {
	Enabled     bool     `json:"enabled"`     // 是否启用（持久化配置）
	BindAddress string   `json:"bindAddress"` // 监听地址 host:port（持久化配置）
	Running     bool     `json:"running"`     // HTTP 服务当前是否运行中
	AccessUrls  []string `json:"accessUrls"`  // 候选访问地址（含回环与本机局域网 IPv4，同网段设备可达）
}

// 素材缩略图设置默认值与取值边界（素材管理面板内设置区，PRD D2；前端 settings
// store 的对应常量与此对齐）。宽度为 Export 目标像素（高按 16:9 自算，不在设置
// 面板暴露）；配额 MB 为单档位 PNG 缓存上限（LRU 淘汰依据）。
const (
	DefaultAssetThumbSmallWidth   = 320
	MinAssetThumbSmallWidth       = 120
	MaxAssetThumbSmallWidth       = 800
	DefaultAssetThumbHDWidth      = 1600
	MinAssetThumbHDWidth          = 480
	MaxAssetThumbHDWidth          = 3840
	DefaultAssetThumbSmallQuotaMB = 1024
	DefaultAssetThumbHDQuotaMB    = 1024
	MinAssetThumbQuotaMB          = 64
	MaxAssetThumbQuotaMB          = 8192
)

// AssetThumbsSettings 素材缩略图设置段（素材管理面板内设置区，D2 五项）。
// 段指针形态区分「老配置缺失（→ 默认）」与显式取值（对齐 WebServe 先例）。
type AssetThumbsSettings struct {
	Auto         bool `json:"auto"`         // 扫描完成后自动生成缩略图（默认 true）
	SmallWidth   int  `json:"smallWidth"`   // 画廊小图宽度 px（默认 320，clamp 120-800；高按 16:9 自算）
	HDWidth      int  `json:"hdWidth"`      // 详情高清宽度 px（默认 1600，clamp 480-3840）
	SmallQuotaMB int  `json:"smallQuotaMB"` // 小图缓存上限 MB（默认 1024，clamp 64-8192）
	HDQuotaMB    int  `json:"hdQuotaMB"`    // 高清缓存上限 MB（默认 1024，clamp 64-8192）
}

// DefaultAssetThumbsSettings 素材缩略图默认设置（老配置升级与损坏降级统一入口）。
func DefaultAssetThumbsSettings() AssetThumbsSettings {
	return AssetThumbsSettings{
		Auto:         true,
		SmallWidth:   DefaultAssetThumbSmallWidth,
		HDWidth:      DefaultAssetThumbHDWidth,
		SmallQuotaMB: DefaultAssetThumbSmallQuotaMB,
		HDQuotaMB:    DefaultAssetThumbHDQuotaMB,
	}
}

// normalize 零值回退默认，越界 clamp 收敛（Auto 为 bool 无 clamp 语义，显式
// false 被尊重；对齐 clampTerminalInt 模式）。
func (s *AssetThumbsSettings) normalize() {
	s.SmallWidth = clampAssetThumbInt(s.SmallWidth, MinAssetThumbSmallWidth, MaxAssetThumbSmallWidth, DefaultAssetThumbSmallWidth)
	s.HDWidth = clampAssetThumbInt(s.HDWidth, MinAssetThumbHDWidth, MaxAssetThumbHDWidth, DefaultAssetThumbHDWidth)
	s.SmallQuotaMB = clampAssetThumbInt(s.SmallQuotaMB, MinAssetThumbQuotaMB, MaxAssetThumbQuotaMB, DefaultAssetThumbSmallQuotaMB)
	s.HDQuotaMB = clampAssetThumbInt(s.HDQuotaMB, MinAssetThumbQuotaMB, MaxAssetThumbQuotaMB, DefaultAssetThumbHDQuotaMB)
}

// NormalizeAssetThumbsSettings 归一化入口：nil（老配置缺失/降级）→ 默认值；
// 非 nil → 拷贝 normalize 后返回，不改写入参（调用方可能传入共享的设置缓存
// 指针，原地改会与并发读者竞态）。
func NormalizeAssetThumbsSettings(s *AssetThumbsSettings) AssetThumbsSettings {
	if s == nil {
		return DefaultAssetThumbsSettings()
	}
	out := *s
	out.normalize()
	return out
}

// EnsureAssetsThumbsDefaults 补全素材缩略图段默认值并收敛越界值：
//   - 段缺失（nil，含老配置升级与配置文件损坏降级）→ 五项全默认；
//   - 段存在 → 零值回退默认、越界 clamp（Auto 显式 false 尊重用户关闭）。
//
// 在 SettingsService.Load 返回前统一调用，保证缩略图链路消费方拿到的段始终
// 完整可用。
func (s *AppSettings) EnsureAssetsThumbsDefaults() {
	if s.Assets == nil {
		def := DefaultAssetThumbsSettings()
		s.Assets = &def
		return
	}
	s.Assets.normalize()
}

// clampAssetThumbInt 零值回退默认，越界收敛到 [min, max]。
func clampAssetThumbInt(v, min, max, def int) int {
	if v == 0 {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// 终端外观默认值与取值边界（设置页控件与工具栏 A-/A+ 快捷调节共用同一边界；
// 前端 settings store 的 TERMINAL_APPEARANCE_DEFAULTS 等常量与此对齐）
const (
	DefaultTerminalFontSize   = 14
	MinTerminalFontSize       = 10
	MaxTerminalFontSize       = 24
	DefaultTerminalScrollback = 1000
	MinTerminalScrollback     = 1000
	MaxTerminalScrollback     = 10000
)

// AppSettings 应用设置
type AppSettings struct {
	GpuDisabled            bool     `json:"gpuDisabled"`
	DefaultShell           string   `json:"defaultShell"`           // 默认 Shell 类型：powershell/cmd/gitbash/wsl
	GitBashPath            string   `json:"gitBashPath"`            // Git Bash 自定义路径
	WslDistro              string   `json:"wslDistro"`              // WSL 发行版名称
	TerminalFontSize       int      `json:"terminalFontSize"`       // 终端字号（pt），默认 14，范围 10-24
	TerminalFontFamily     string   `json:"terminalFontFamily"`     // 终端字体族，空值走默认 Cascadia Code 字体栈
	TerminalScrollback     int      `json:"terminalScrollback"`     // 终端回滚行数，默认 1000，范围 1000-10000
	SearchExcludeDirs      []string `json:"searchExcludeDirs"`      // 搜索排除目录
	SearchExcludeFiles     []string `json:"searchExcludeFiles"`     // 搜索排除文件模式
	ShortcutCommandPalette string   `json:"shortcutCommandPalette"` // 命令面板快捷键，默认 "Ctrl+P"
	ShortcutToggleTerminal string   `json:"shortcutToggleTerminal"` // 切换终端快捷键，默认 "Ctrl+`"
	ShortcutRename         string   `json:"shortcutRename"`         // 重命名快捷键，默认 "F2"
	ShortcutDelete         string   `json:"shortcutDelete"`         // 删除快捷键，默认 "Delete"
	ObsidianPath           string   `json:"obsidianPath"`           // Obsidian 可执行文件自定义路径，留空表示未配置
	ThemeMode              string   `json:"themeMode"`              // 主题模式：system(跟随系统)/light/dark，空值按 system 处理
	DiffToolName           string   `json:"diffToolName"`           // 外部 diff 工具预设名：beyondcompare/winmerge/vscode/custom
	DiffToolPath           string   `json:"diffToolPath"`           // 外部 diff 工具可执行文件路径，留空表示未配置
	DiffToolArgs           string   `json:"diffToolArgs"`           // 外部 diff 工具参数模板，支持 {left} {right} 占位符
	DiffToolMergeArgs      string   `json:"diffToolMergeArgs"`      // 三向合并参数模板（custom 预设用），支持 {base} {local} {remote} {merged} 占位符；预设名非 custom 时由后端内置模板接管，本字段不消费
	// WebServe 浏览器访问通道配置段。指针形态区分「段缺失（老配置升级，视为未
	// 配置 → 默认开启）」与「显式 enabled:false（用户主动关闭）」——Go bool 零值
	// 为 false，非指针无法表达「默认 true」语义。加载后经 EnsureWebServeDefaults 补全。
	WebServe *WebServeSettings `json:"webServe,omitempty"`
	// Assets 素材缩略图设置段（素材管理面板设置区 D2 五项）。指针形态区分
	// 「老配置缺失 → 默认」与显式取值（对齐 WebServe 先例），加载后经
	// EnsureAssetsThumbsDefaults 补全。
	Assets *AssetThumbsSettings `json:"assets,omitempty"`
}

// EnsureWebServeDefaults 补全 webServe 段默认值：
//   - 段缺失（nil，含老配置升级与配置文件损坏降级）→ 默认开启 + 回环默认地址；
//   - 段存在但地址为空 → 仅补默认地址，enabled 尊重用户显式取值。
//
// 在 SettingsService.Load 返回前统一调用，保证所有消费方（桌面 startup、
// --serve 地址解析、前端 GetSettings）拿到的段始终完整可用。
func (s *AppSettings) EnsureWebServeDefaults() {
	if s.WebServe == nil {
		s.WebServe = &WebServeSettings{Enabled: true, BindAddress: DefaultWebServeBindAddress}
		return
	}
	if s.WebServe.BindAddress == "" {
		s.WebServe.BindAddress = DefaultWebServeBindAddress
	}
	// TLS 段零值即向后兼容默认（tls=false = 纯 HTTP；证书路径双空 = 自签引导），
	// 刻意不补默认值：TLS 是显式决策项，老配置升级必须保持原纯 HTTP 行为不变。
}

// EnsureTerminalDefaults 补全终端外观默认值并收敛越界值：
//   - 字号/回滚行数零值（老配置升级、字段缺失、损坏降级）→ 补默认（14 / 1000）；
//   - 手改配置文件越界值 → clamp 收敛到合法区间（字号 10-24、回滚 1000-10000），
//     防 xterm 拿到异常配置（字号过小不可读、过大破坏布局，scrollback 超限放大内存）；
//   - TerminalFontFamily 空串即默认 Cascadia Code 字体栈，不强制填充。
//
// 在 SettingsService.Load 返回前统一调用，保证前端 GetSettings 拿到的三字段始终可用。
func (s *AppSettings) EnsureTerminalDefaults() {
	s.TerminalFontSize = clampTerminalInt(s.TerminalFontSize, MinTerminalFontSize, MaxTerminalFontSize, DefaultTerminalFontSize)
	s.TerminalScrollback = clampTerminalInt(s.TerminalScrollback, MinTerminalScrollback, MaxTerminalScrollback, DefaultTerminalScrollback)
}

// clampTerminalInt 零值回退默认，越界收敛到 [min, max]。
func clampTerminalInt(v, min, max, def int) int {
	if v == 0 {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
