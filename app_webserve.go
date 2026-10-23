package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"workbench/model"
	"workbench/server"
	"workbench/service"
)

// ===== 浏览器访问域 =====
//
// 桌面设置页「网络访问」分区与浏览器通道运行期变更的绑定方法：
//   - GetWebServeConfig：配置 + 运行状态 + 候选访问地址（设置页展示）；
//   - SetWebServeConfig：持久化并使运行时生效（改地址/开关平滑重启服务）；
//   - GetWebServeToken / RegenerateWebToken：访问令牌查看与轮换（多 token
//     语义下的「默认」令牌编排，见 RegenerateWebToken 注释）；
//   - ListWebTokens / CreateWebToken / RevokeWebToken：多 token 管理三方法
//     （前端 token 管理 UI 另行派发，本批仅后端能力与绑定）。
//
// 令牌生效语义：HTTP 认证与 WS 握手校验经 verify 闭包直读 WebTokenService
// （无状态快照），新建/撤销/过期即时生效（不断监听、不断在途连接）；已建立的
// WS 连接不在数据面校验令牌，保持有效至断开重连，重连须用有效令牌。

// GetWebServeConfig 获取浏览器访问通道配置与运行状态。
// 配置段经 EnsureWebServeDefaults 补全默认值（段缺失 → 默认开启 + 回环地址）；
// AccessUrls 为候选访问地址（回环 + 本机局域网 IPv4，同网段设备可达），TLS
// 开启时 scheme 为 https://（与自签证书 SAN 同源覆盖）。TLS 开关与证书路径
// 经 GetSettings（整体读写 AppSettings）透传前端，本方法零额外字段。
func (a *App) GetWebServeConfig() *model.WebServeConfig {
	settings := a.GetSettings()
	cfg := &model.WebServeConfig{
		Enabled:     settings.WebServe.Enabled,
		BindAddress: settings.WebServe.BindAddress,
		AccessUrls:  webAccessUrls(settings.WebServe.BindAddress, settings.WebServe.TLS),
	}
	if a.webServe != nil {
		cfg.Running = a.webServe.Running()
	}
	return cfg
}

// SetWebServeConfig 保存浏览器访问通道配置并使运行时生效。
//
// 生效语义：enabled=true 时若服务未运行则以新地址启动、地址或 TLS 配置变化则
// 平滑重启（优雅停旧实例 → 新配置重启；同地址同 TLS 配置为 no-op 不断在途
// 连接）；enabled=false 时停机（保留配置供下次开启）。TLS 开关与证书路径不由
// 本方法入参表达（前端经 SaveSettings 整体保存该两字段），此处以磁盘现有配置
// 为基底保留——本方法只覆写 enabled/bindAddress，避免覆掉 TLS 设置。
// 空地址回退默认回环地址；地址格式须为 host:port。改绑非回环地址不在此阻止
// （桌面设置页已弹风险确认，--serve 场景由 --listen 显式表达），仅记录告警日志。
func (a *App) SetWebServeConfig(enabled bool, bindAddress string) error {
	addr := strings.TrimSpace(bindAddress)
	if addr == "" {
		addr = model.DefaultWebServeBindAddress
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("绑定地址格式无效（须为 host:port，如 127.0.0.1:36115）: %w", err)
	}

	// 持久化（以磁盘现有设置为基底合并写，避免覆盖其他字段）。GetSettings 经
	// settingsSvc 缓存命中返回共享指针（service/settings.go 并发口径：消费方
	// 只读禁改），此处复制后替换 webServe 段，防原地改与并发读者竞态。
	base := a.GetSettings()
	settings := *base
	webServe := &model.WebServeSettings{Enabled: enabled, BindAddress: addr}
	// TLS 设置（开关 + 证书路径）保留磁盘现值：本方法入参不表达 TLS，
	// 重建段时若不带会把它覆成零值（用户保存地址即丢 TLS 配置）
	if base.WebServe != nil {
		webServe.TLS = base.WebServe.TLS
		webServe.TLSCertPath = base.WebServe.TLSCertPath
		webServe.TLSKeyPath = base.WebServe.TLSKeyPath
	}
	settings.WebServe = webServe
	// Assets 段同为指针共享，而 Save 入口会对入参执行 EnsureAssetsThumbsDefaults
	// （normalize 含无条件字段赋值）——一并复制为私有段，防同值写与并发读者竞态
	if base.Assets != nil {
		assets := *base.Assets
		settings.Assets = &assets
	}
	if err := a.settingsSvc.Save(&settings); err != nil {
		return fmt.Errorf("保存设置失败: %w", err)
	}

	// 运行时生效（webServe 为 nil 时仅落盘，下次启动按配置生效）：
	// 开启 → 启动/平滑重启到新地址与新 TLS 配置；关闭 → 优雅停机
	if a.webServe != nil {
		if enabled {
			tlsCfg := &WebServeTLSConfig{
				Enabled:  webServe.TLS,
				CertPath: webServe.TLSCertPath,
				KeyPath:  webServe.TLSKeyPath,
			}
			if err := a.webServe.Start(addr, tlsCfg); err != nil {
				return fmt.Errorf("重启浏览器访问服务失败: %w", err)
			}
		} else if err := a.webServe.Stop(); err != nil {
			return fmt.Errorf("停止浏览器访问服务失败: %w", err)
		}
	}

	if enabled && !isLoopbackBind(addr) {
		// 非回环绑定：整机文件/终端/AI agent 能力向局域网暴露，记告警留痕
		slog.Warn("浏览器访问服务绑定到非回环地址，本机文件/终端/agent 能力将对局域网可达", "addr", addr)
	}
	return nil
}

// GetWebServeToken 读取当前访问令牌（存量单 token 明文兼容文件链路）。
//
// 多 token 语义下明文不再落多 token 记录（仅创建时一次性返回），本方法改为
// 读取 data/web_token 明文兼容文件：初始引导与轮换编排会把当前「默认」令牌
// 明文同步写该文件（0600），保证设置页「查看令牌」与 serve 控制台提示在
// token 管理 UI（另行派发）落地前继续可用。用户自建的命名 token 明文不进
// 该文件（只经 CreateWebToken 返回值一次性可见）。
func (a *App) GetWebServeToken() (string, error) {
	if a.webTokenSvc == nil {
		// 未装配多 token 服务（测试零值构造等）：退回存量单 token 语义
		return server.LoadOrCreateToken(serveTokenFile)
	}
	// 显式触发加载：存量迁移/初始引导在启动期（NewAppServices）已完成，此处
	// 兜底覆盖「服务被旁路构造」的形态；引导会写明文兼容文件
	a.webTokenSvc.EnsureLoaded()
	data, err := os.ReadFile(serveTokenFile)
	if err != nil {
		return "", fmt.Errorf("读取令牌文件失败: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("令牌文件为空（可能已损坏），请重新生成令牌")
	}
	return token, nil
}

// RegenerateWebToken 重新生成访问令牌（多 token 语义下的轮换编排）：
// 新建「默认」token → 撤销全部旧记录 → 新明文同步写存量兼容文件 → 返回新明文。
// 前端 API 签名与既有语义（返回新明文、旧令牌立即失效）不变。
//
// 全程持 webTokenMu 互斥——并发轮换（浏览器多标签页同时触发）若不加锁，
// 多次「新建 + 撤旧」编排可交错出中间态（goroutine 1 的旧令牌被 goroutine 2
// 后建的新令牌撤掉前又把 2 的新令牌视为旧记录，最终存活集合不确定）。互斥后
// 编排串行，最后一个完成的调用决定最终态（唯一存活记录 = 其新令牌）。
//
// 生效语义：HTTP 与 WS 握手校验经 verify 闭包直读 WebTokenService（无状态
// 快照），撤旧建新落盘（内存先行）后即时生效，无需任何热轮换通知；已建立的
// WS 连接不在数据面校验令牌，保持有效至断开重连（见文件头注释的热生效语义）。
// 旧令牌对后续全部 HTTP 与 WS 握手请求立即失效。
func (a *App) RegenerateWebToken() (string, error) {
	a.webTokenMu.Lock()
	defer a.webTokenMu.Unlock()

	if a.webTokenSvc == nil {
		return "", fmt.Errorf("令牌服务未装配")
	}
	plain, rec, err := a.webTokenSvc.Create(service.DefaultWebTokenName, 0)
	if err != nil {
		return "", fmt.Errorf("生成访问令牌失败: %w", err)
	}
	if _, err := a.webTokenSvc.RevokeAllExcept(rec.ID); err != nil {
		return "", fmt.Errorf("撤销旧令牌失败: %w", err)
	}
	// 审计轮换编排完成（创建/逐条撤销明细已由服务层各自落盘 token_create/token_revoke）
	if a.webAuditSvc != nil {
		a.webAuditSvc.Log(service.AuditEventTokenRotate, rec.ID, rec.Name, "", "")
	}
	// 新明文同步写存量兼容文件（0600）：「查看令牌」链路与轮换后的有效令牌一致
	if err := server.SaveToken(serveTokenFile, plain); err != nil {
		return "", fmt.Errorf("持久化访问令牌失败: %w", err)
	}
	slog.Info("web serve token rotated", "tokenId", rec.ID)
	return plain, nil
}

// ListWebTokens 列出全部访问令牌记录（按创建时间升序；只含哈希不含明文，
// 含已撤销/已过期记录供列表展示与审计可见性）。
func (a *App) ListWebTokens() []model.WebTokenRecord {
	if a.webTokenSvc == nil {
		return []model.WebTokenRecord{}
	}
	return a.webTokenSvc.List()
}

// CreateWebToken 新建访问令牌：明文经返回值一次性下发（不落盘），记录只含
// 哈希。name 空 / expiresAt 过去的校验拒绝以 AppError 错误码下发（前端按码
// 分流提示级别）。Wails 绑定形态为单结构体返回（Wails v2.16 仅支持 1~2 个
// 返回值，明文与记录由 model.WebTokenCreated 承载）。
func (a *App) CreateWebToken(name string, expiresAt int64) (*model.WebTokenCreated, error) {
	if a.webTokenSvc == nil {
		return nil, fmt.Errorf("令牌服务未装配")
	}
	plain, rec, err := a.webTokenSvc.Create(name, expiresAt)
	if err != nil {
		return nil, err
	}
	return &model.WebTokenCreated{PlainText: plain, Record: rec}, nil
}

// RevokeWebToken 撤销指定令牌（幂等：重复撤销 no-op 成功；不存在返回
// E_TOKEN_NOT_FOUND，warning 级引导刷新列表）。
func (a *App) RevokeWebToken(id string) error {
	if a.webTokenSvc == nil {
		return fmt.Errorf("令牌服务未装配")
	}
	return a.webTokenSvc.Revoke(id)
}
