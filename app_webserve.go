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
// 含已撤销/已过期记录供列表展示与审计可见性），每条附带在线连接数（连接管控
// 任务新增）：由连接明细缓存按 TokenID 聚合 join 而来，列表与计数单调用原子
// 一致，与桌面明细面板行数口径相同（同一缓存快照）。连接管控四期起附带封禁
// 状态 join（Banned/BannedUntil/BanReason，数据源 data/web_bans.json 封禁记录，
// 含已过期记录原样透出——「已过期」态由前端按 BannedUntil 判定展示，拒连判定
// 以服务层懒过期为准）。在线数与封禁状态不落盘——返回的 WebTokenStatus 为绑定
// 面包装形态，data/web_tokens.json 落盘仍为纯 WebTokenRecord（零污染）。
func (a *App) ListWebTokens() []model.WebTokenStatus {
	if a.webTokenSvc == nil {
		return []model.WebTokenStatus{}
	}
	records := a.webTokenSvc.List()
	online := a.onlineConnCountsByToken()
	bans := a.banStateByToken()
	statuses := make([]model.WebTokenStatus, 0, len(records))
	for _, rec := range records {
		st := model.WebTokenStatus{Record: rec, OnlineConns: online[rec.ID]}
		if ban, ok := bans[rec.ID]; ok {
			st.Banned = true
			st.BannedUntil = ban.ExpiresAt
			st.BanReason = ban.Reason
		}
		statuses = append(statuses, st)
	}
	return statuses
}

// banStateByToken 按令牌 ID 聚合封禁记录（ListWebTokens 封禁状态 join 数据源，
// 连接管控四期）。封禁列表量级为个人工具小量级，单次调用全量拉取后建 map 即可。
func (a *App) banStateByToken() map[string]model.WebBanRecord {
	bans := make(map[string]model.WebBanRecord)
	if a.webTokenSvc == nil {
		return bans
	}
	for _, ban := range a.webTokenSvc.ListBans() {
		bans[ban.TokenID] = ban
	}
	return bans
}

// rememberConnDetails 更新连接明细缓存（整体替换语义）。写入点两处：hub
// SetOnConnectionsChange 回调（web_serve.go startLocked，载荷为最新全量快照）
// 与 stopLocked 停机清零（hub.Close 收口期回调被抑制，须显式写空快照）。
func (a *App) rememberConnDetails(details []server.ConnDetail) {
	a.connDetailsMu.Lock()
	a.connDetails = details
	a.connDetailsMu.Unlock()
}

// cachedConnDetails 读取连接明细缓存副本（回调 goroutine 写 + 绑定调用读并发
// 安全；拷贝出锁后消费，不持锁聚合）。
func (a *App) cachedConnDetails() []server.ConnDetail {
	a.connDetailsMu.Lock()
	defer a.connDetailsMu.Unlock()
	out := make([]server.ConnDetail, len(a.connDetails))
	copy(out, a.connDetails)
	return out
}

// onlineConnCountsByToken 按令牌 ID 聚合在途连接数（ListWebTokens 在线数
// 数据源）。聚合在缓存拷贝出锁后进行；miss（令牌已撤销但连接未断）不进计数——
// 名册无该 ID 记录可 join，计数仅对现存记录生效（已撤销记录仍在名册，其在途
// 连接照常计入，与明细面板 TokenName 空串降级同款语义）。
func (a *App) onlineConnCountsByToken() map[string]int {
	counts := make(map[string]int)
	for _, d := range a.cachedConnDetails() {
		counts[d.TokenID]++
	}
	return counts
}

// KickConnection 踢出指定浏览器连接（桌面明细面板处置入口，连接管控任务新增）。
// 委托 webServeManager（停机/未启用返回 E_WEBSERVE_NOT_RUNNING，连接不存在返回
// E_CONNECTION_NOT_FOUND，均 warning 级引导）。踢出生效（hub 命中）后写审计
// conn_kick（对齐「动作生效后才写审计」纪律，miss/停机不写——桌面按钮与浏览器
// RPC 反射面同走本方法族，单咽喉全覆盖）。踢出语义 = 断开当前连接，对端同令牌可
// 重连（非封禁）；防重连由 RevokeWebToken 承担（撤销即时生效于新连接，不影响
// 已建立连接，两手段互补）。浏览器 UI 无此入口（同构暴露面不特判，认证由外层
// token 把关，与停服务/撤令牌同能力面）。
//
// 桌面路径 actorSource=desktop（本地信任模型无令牌上下文，操作者即桌面用户）；
// 浏览器 RPC 路径经 rpcAuditTarget 装饰 override 捕获调用者令牌 ID 后同走
// kickConnectionAs（连接管控三期 actor 归因）。
func (a *App) KickConnection(connID uint64) error {
	return a.kickConnectionAs(connID, service.AuditActorDesktop, "")
}

// kickConnectionAs 踢出动作统一实现（导出 KickConnection 与 RPC 装饰 override
// 的共用体，actor 归因由此透传入审计）。签名保持未导出——不入 Wails 绑定面与
// RPC 反射面。
func (a *App) kickConnectionAs(connID uint64, actorSource, actorTokenID string) error {
	if a.webServe == nil {
		return model.NewAppError(model.ErrCodeWebServeNotRunning,
			"浏览器访问服务未运行，无法踢出连接")
	}
	detail, err := a.webServe.KickConnection(connID)
	if err != nil {
		return err
	}
	a.auditConnKick(actorSource, actorTokenID, detail)
	return nil
}

// auditConnKick 踢出动作审计（conn_kick 事件行）。令牌名经名册映射补全（hub
// 明细快照的 TokenName 为 server 层原始形态恒空，webTokenNamedDetails 单点映射，
// miss 降级空串与明细面板同款语义）；webAuditSvc 未装配（旁路构造的测试 App）
// 静默跳过——审计为旁路观测面，不阻断踢出主链路。
func (a *App) auditConnKick(actorSource, actorTokenID string, detail server.ConnDetail) {
	if a.webAuditSvc == nil {
		return
	}
	named := a.webTokenNamedDetails([]server.ConnDetail{detail})
	a.webAuditSvc.LogConnKick(service.KickAudit{
		ActorSource:   actorSource,
		ActorTokenID:  actorTokenID,
		TargetTokenID: named[0].TokenID,
		TargetName:    named[0].TokenName,
		RemoteAddr:    named[0].IP,
		ConnID:        named[0].ID,
	})
}

// rpcAuditTarget 浏览器 RPC 面的 per-request 装饰 target（连接管控三期 actor
// 归因）：embed *App 提升全部导出方法（RPC 白名单与桌面绑定面形态一致，其余
// 方法零分叉），仅 override 带操作者归因的管控动作方法——KickConnection（三期）
// 与 BanWebToken/UnbanWebToken（四期）：闭包捕获本次请求的调用者令牌 ID（token
// 中间件注入 ctx，装配回调按请求构造本值），委托未导出 *As 变体完成 actor=rpc
// 归因审计。定义在 main 包（server 禁反向依赖 main）；未导出字段/方法不入任何
// 暴露面，导出面签名零变更（wailsjs 生成面不动）。
type rpcAuditTarget struct {
	*App
	actorTokenID string
}

func (t rpcAuditTarget) KickConnection(connID uint64) error {
	return t.kickConnectionAs(connID, service.AuditActorRPC, t.actorTokenID)
}

func (t rpcAuditTarget) BanWebToken(id string, expiresAt int64, reason string) error {
	return t.banWebTokenAs(id, expiresAt, reason, service.AuditActorRPC, t.actorTokenID)
}

func (t rpcAuditTarget) UnbanWebToken(id string) error {
	return t.unbanWebTokenAs(id, service.AuditActorRPC, t.actorTokenID)
}

// rpcTargetForActor 装配回调（server.WebOptions.RPCTargetForActor 消费形态）：
// 按调用者令牌 ID 构造 per-request RPC target。
func (a *App) rpcTargetForActor(actorTokenID string) any {
	return rpcAuditTarget{App: a, actorTokenID: actorTokenID}
}

// QueryAuditLog 查询审计历史（连接管控三期，桌面审计历史弹窗数据源）：委托
// AuditLogger 读侧（过滤 + 时间倒序分页，坏行跳过，快照语义）。
//
// **隐私红线——本方法不进浏览器通道**：桌面绑定面与 RPC 白名单同构，读侧若
// 同构可达，浏览器通道可直调获取全谱 IP/令牌明细，破「他人 IP/token 明细仅走
// 桌面出口」红线。装配层经 server.WebOptions.RPCHiddenMethods 对翻译层屏蔽
// （对浏览器呈现「方法不存在」），前端设置页入口在浏览器模式隐藏。桌面 Wails
// 绑定不经翻译层，不受屏蔽影响。审计服务未装配（旁路构造的测试 App）返回
// 空结果（审计为旁路观测面，不报错阻断）。
func (a *App) QueryAuditLog(opts service.AuditQueryOptions) (service.AuditQueryResult, error) {
	if a.webAuditSvc == nil {
		return service.AuditQueryResult{Records: []service.AuditRecord{}}, nil
	}
	result, err := a.webAuditSvc.QueryAuditLog(opts)
	if err != nil {
		return service.AuditQueryResult{}, err
	}
	if result.Records == nil {
		result.Records = []service.AuditRecord{}
	}
	return result, nil
}

// webTokenNamedDetails 为连接明细快照补全令牌名（web_serve.go startLocked 桌面
// 明细回调专用）。tokenID→名称映射在装配层完成：server 包不知令牌名册（分层
// 红线），回调闭包持 App 引用查本方法。每次回调重建 ID→Name map——连接变化为
// 低频事件、名册量小，无缓存必要。映射 miss（令牌已撤销但连接未断、webTokenSvc
// 未装配）TokenName 留空串，前端 fallback「已撤销令牌」展示，不阻断明细呈现。
func (a *App) webTokenNamedDetails(details []server.ConnDetail) []server.ConnDetail {
	named := make([]server.ConnDetail, len(details))
	nameByID := make(map[string]string)
	if a.webTokenSvc != nil {
		for _, rec := range a.webTokenSvc.List() {
			nameByID[rec.ID] = rec.Name
		}
	}
	for i, d := range details {
		d.TokenName = nameByID[d.TokenID]
		named[i] = d
	}
	return named
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

// BanWebToken 封禁指定令牌（连接管控四期，桌面路径）：tokenID 维度临时拒连
// ——新连接（HTTP/WS 两认证点）401 携带含解封时间的专用消息，已建立连接不受
// 影响（仅拒新连，对齐撤销热生效语义）；「同时断开在途连接」联动踢出为前端
// 编排（按 tokenID 过滤连接明细逐个 KickConnection），后端零额外参数。
// expiresAt 为到期 unix 秒（0 = 永久封禁须手动解封）；重复封禁 = 刷新封禁参数。
// 桌面路径 actorSource=desktop（本地信任模型无令牌上下文）；浏览器 RPC 路径经
// rpcAuditTarget 装饰 override 捕获调用者令牌 ID 后同走 banWebTokenAs（actor
// 归因，对齐 kickConnectionAs 三期先例）。
func (a *App) BanWebToken(id string, expiresAt int64, reason string) error {
	return a.banWebTokenAs(id, expiresAt, reason, service.AuditActorDesktop, "")
}

// banWebTokenAs 封禁动作统一实现（导出 BanWebToken 与 RPC 装饰 override 的共用
// 体，actor 归因由此透传入审计）。签名保持未导出——不入 Wails 绑定面与 RPC
// 反射面。
func (a *App) banWebTokenAs(id string, expiresAt int64, reason, actorSource, actorTokenID string) error {
	if a.webTokenSvc == nil {
		return fmt.Errorf("令牌服务未装配")
	}
	return a.webTokenSvc.Ban(id, expiresAt, reason, actorSource, actorTokenID)
}

// UnbanWebToken 解封指定令牌（删除封禁记录，新连接立即可达；未封禁 no-op 成功，
// 不存在返回 E_TOKEN_NOT_FOUND）。桌面路径 actorSource=desktop。
func (a *App) UnbanWebToken(id string) error {
	return a.unbanWebTokenAs(id, service.AuditActorDesktop, "")
}

// unbanWebTokenAs 解封动作统一实现（对齐 banWebTokenAs，未导出不入暴露面）。
func (a *App) unbanWebTokenAs(id, actorSource, actorTokenID string) error {
	if a.webTokenSvc == nil {
		return fmt.Errorf("令牌服务未装配")
	}
	return a.webTokenSvc.Unban(id, actorSource, actorTokenID)
}
