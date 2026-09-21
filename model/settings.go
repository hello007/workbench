package model

// DefaultWebServeBindAddress 浏览器访问通道默认绑定地址（回环，仅本机浏览器可达）。
// 远程访问须显式改绑（设置页确认或 --listen 命令行覆盖），改绑非回环地址会向
// 局域网暴露整机文件、终端与 AI agent 能力，建议配合隧道使用。
const DefaultWebServeBindAddress = "127.0.0.1:36115"

// WebServeSettings 浏览器访问通道配置（settings.json 的 webServe 段）。
type WebServeSettings struct {
	Enabled     bool   `json:"enabled"`     // 桌面模式是否同开 HTTP 服务（默认 true）
	BindAddress string `json:"bindAddress"` // HTTP 监听地址 host:port（默认回环 127.0.0.1:36115）
}

// WebServeConfig 浏览器访问通道配置与运行状态（GetWebServeConfig 返回）。
// 在持久化配置之上附带派生信息：服务运行状态与候选访问地址（回环 + 本机局域网 IP）。
type WebServeConfig struct {
	Enabled     bool     `json:"enabled"`     // 是否启用（持久化配置）
	BindAddress string   `json:"bindAddress"` // 监听地址 host:port（持久化配置）
	Running     bool     `json:"running"`     // HTTP 服务当前是否运行中
	AccessUrls  []string `json:"accessUrls"`  // 候选访问地址（含回环与本机局域网 IPv4，同网段设备可达）
}

// AppSettings 应用设置
type AppSettings struct {
	GpuDisabled        bool     `json:"gpuDisabled"`
	DefaultShell       string   `json:"defaultShell"`       // 默认 Shell 类型：powershell/cmd/gitbash/wsl
	GitBashPath        string   `json:"gitBashPath"`        // Git Bash 自定义路径
	WslDistro          string   `json:"wslDistro"`          // WSL 发行版名称
	SearchExcludeDirs  []string `json:"searchExcludeDirs"`  // 搜索排除目录
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
	// WebServe 浏览器访问通道配置段。指针形态区分「段缺失（老配置升级，视为未
	// 配置 → 默认开启）」与「显式 enabled:false（用户主动关闭）」——Go bool 零值
	// 为 false，非指针无法表达「默认 true」语义。加载后经 EnsureWebServeDefaults 补全。
	WebServe *WebServeSettings `json:"webServe,omitempty"`
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
}
