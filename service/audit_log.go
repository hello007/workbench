package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// 审计事件名（JSONL 行的 event 字段取值）。auth_* 由 server 包认证点回调
// （server.AuthEvent.Event）透传至此落盘；token_* 由 WebTokenService 生命周期
// 操作直接产生。
const (
	AuditEventAuthSuccess  = "auth_success"  // 认证成功（HTTP requireToken / WS 握手认证点）
	AuditEventAuthFailure  = "auth_failure"  // 认证失败（同上两处认证点；封禁拒连形态 TokenID 非空）
	AuditEventTokenCreate  = "token_create"  // 新建 token
	AuditEventTokenRevoke  = "token_revoke"  // 撤销 token（含轮换编排的逐条撤旧）
	AuditEventTokenRotate  = "token_rotate"  // 轮换编排（新建 + 撤旧）完成（App 层编排事件）
	AuditEventTokenMigrate = "token_migrate" // 存量单 token 迁移为多 token 首条记录 / 初始引导
	AuditEventConnKick     = "conn_kick"     // 踢出在途 WS 连接（连接管控动作，App 层 hub 命中后落盘）
	AuditEventTokenBan     = "token_ban"     // 封禁令牌（连接管控四期：tokenID 维度临时拒连）
	AuditEventTokenUnban   = "token_unban"   // 解封令牌（删除封禁记录）
	AuditEventIPBan        = "ip_ban"        // 来源 IP 自动封禁（连接管控五期：认证失败达阈值，actorSource 恒 system）
	AuditEventIPUnban      = "ip_unban"      // 手动解封来源 IP（实际解除才写，no-op 不写）
)

// 操作者来源（actorSource 字段取值，连接管控三期新增；五期扩 system）：
// desktop 为本地桌面操作（无令牌上下文，本地信任模型，操作者即桌面用户）；
// rpc 为浏览器通道 /api/rpc 调用（actorTokenId 携带调用者令牌 ID，多令牌场景
// 可归因「谁踢的」）；system 为服务端自动触发（IP 认证失败达阈值自动封禁，
// 无人类操作者，actorTokenId 恒空）。
const (
	AuditActorDesktop = "desktop"
	AuditActorRPC     = "rpc"
	AuditActorSystem  = "system"
)

// webAuditMaxBytes 审计日志单文件大小上限：超过即轮转入多档链（档位深度见
// webAuditMaxRotationDepth），继续向新文件追加。5MB 对认证事件级频度足够（约
// 3 万行）。
//
// 连接管控十一期将十期「不做多份轮转链」转正为固定深度轮转链：5MB×2（约 6 万行）
// 在认证失败潮下数小时即轮转一轮，趋势面 TrendWindowDays 默认 7 天的回看窗口
// 名存实亡，故扩链深度以保留更长历史（数据源仍为审计 JSONL 单一来源，读侧
// readSnapshot 多档合并后 QueryIPBanTrend/QueryAuditLog 透明受益）。
const webAuditMaxBytes int64 = 5 * 1024 * 1024

// webAuditMaxRotationDepth 审计轮转链档位深度（连接管控十一期）：主文件超限时
// 链式下移历史档（.N-1→.N … .1→.2）再将主文件 rename 为 <path>.1，淘汰最旧档
// （静默，设计语义非异常——趋势面 earliestTs 自然前移收敛）。默认 6：历史合计
// 30MB + 主 5MB = 35MB，正常频度（认证事件每天几十~几百 KB）足以撑满趋势面默认
// 7 天回看窗口；可经 AuditSettings.depth（clamp 2–10）调整。轮转触发仍为纯大小
// 触发，不做时间触发与压缩归档（压缩持锁阻塞审计写、读侧解压开销不抵收益）。
const webAuditMaxRotationDepth = 6

// AuditRecord 审计日志单行（JSONL）。禁落 token 明文与完整 URL（?token= 泄露面，
// 对齐 server 层「不落完整 URL」既有约定）。
type AuditRecord struct {
	Ts         int64  `json:"ts"`                   // 事件时间（unix 秒）
	Event      string `json:"event"`                // 事件名，见 AuditEvent* 常量
	TokenID    string `json:"tokenId,omitempty"`    // 关联 token 记录 ID（认证失败时为空）
	Name       string `json:"name,omitempty"`       // token 名称（token 生命周期事件携带）
	RemoteAddr string `json:"remoteAddr,omitempty"` // 来源地址（r.RemoteAddr；直连形态不做 XFF 信任，PRD R2）
	UserAgent  string `json:"userAgent,omitempty"`  // 客户端 User-Agent
	ConnID     uint64 `json:"connId,omitempty"`     // 被踢连接 ID（conn_kick 事件携带；认证/生命周期事件为空不落字段）
	// 操作者归因（连接管控三期新增，仅 conn_kick 事件携带）：ActorSource 区分
	// 桌面/浏览器 RPC 来源，ActorTokenID 为 RPC 调用者令牌 ID（桌面为空——本地
	// 信任模型无令牌上下文）。与 TokenID（conn_kick 语义为被踢目标的令牌）正交，
	// 二者不可复用同一字段。
	ActorSource  string `json:"actorSource,omitempty"`
	ActorTokenID string `json:"actorTokenId,omitempty"`
	// 封禁参数（连接管控四期新增，仅 token_ban 事件携带）：Reason 封禁备注、
	// ExpiresAt 封禁到期 unix 秒（0=永久封禁，omitempty 恒不落键）。均为空/零值
	// 不落字段，保证既有事件行形态不变。
	Reason    string `json:"reason,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
	// IP 级封禁字段（连接管控五期新增，仅 ip_ban/ip_unban 事件携带）：IP 为
	// 来源地址（RemoteAddr 剥端口归一后的形态，与 RemoteAddr 字段互不复用——
	// 该字段语义为认证事件的原始地址含端口）；FailCount 为触发封禁时滑动窗口
	// 内认证失败计数；Strike 为累计触发次数（连接管控六期递增封禁，仅 ip_ban
	// 携带——ip_unban 无计数语义）。均零值不落字段，既有事件行形态不变。
	IP        string `json:"ip,omitempty"`
	FailCount int    `json:"failCount,omitempty"`
	Strike    int    `json:"strike,omitempty"`
}

// AuditLogger 认证与 token 生命周期审计日志（JSONL append-only，data/web_audit.log）。
//
// append 语义独立实现，不走 util.SaveJSON 原子写——原子写针对「整体覆盖型 JSON
// 快照」的半截文件问题，append-only 日志以 O_APPEND 追加单行（O_APPEND 下单次
// write 为原子 append），属 data-atomic-write 规范的「非 JSON 写入」豁免口径，
// 主数据文件截断写禁令不适用于本旁路日志。
//
// 写入为旁路：打开/写入失败仅 slog.Warn 降级，不阻塞认证与 token 管理主链路
// （审计是旁路观测面，非正确性依赖）。全部事件写入收敛单方法 Log：server 认证
// 回调（OnAuth）与 service token 生命周期操作都经此处落盘。
type AuditLogger struct {
	mu       sync.Mutex
	path     string
	maxBytes int64 // 单文件大小上限（测试注入小值驱动轮转；生产 webAuditMaxBytes）
	depth    int   // 轮转链档位深度（生产 webAuditMaxRotationDepth；AuditSettings.depth clamp 2–10 热更）
}

// NewAuditLogger 构造审计日志器（path 如 data/web_audit.log）。
func NewAuditLogger(path string) *AuditLogger {
	return &AuditLogger{path: path, maxBytes: webAuditMaxBytes, depth: webAuditMaxRotationDepth}
}

// ApplyConfig 热更轮转参数（连接管控十一期，App.SetAuditConfig「保存+生效」
// 生效点）：maxBytes 单文件字节上限、depth 链档位深度，mu 锁内整体替换（写路径
// rotateLocked 同锁，无撕裂配置）。仅面向未来：已超限主文件不立即轮转（下次写
// 入达限触发）；depth 缩小后残留超深度旧档不入读不清除（静默滞留，见 readSnapshot）。
func (a *AuditLogger) ApplyConfig(maxBytes int64, depth int) {
	a.mu.Lock()
	a.maxBytes = maxBytes
	a.depth = depth
	a.mu.Unlock()
}

// rotatedPath 第 i 档历史档路径（<path>.i，i≥1）。
func (a *AuditLogger) rotatedPath(i int) string {
	return a.path + "." + strconv.Itoa(i)
}

// Log 追加一条审计事件（单写锁串行，并发安全）。
func (a *AuditLogger) Log(event, tokenID, name, remoteAddr, userAgent string) {
	a.write(AuditRecord{
		Ts:         time.Now().Unix(),
		Event:      event,
		TokenID:    tokenID,
		Name:       name,
		RemoteAddr: remoteAddr,
		UserAgent:  userAgent,
	})
}

// KickAudit conn_kick 事件载荷（结构化参数，连接管控三期由位置参数收编）：操作者
// 归因（Actor/ActorTokenID）与被踢目标（TargetTokenID/TargetName/RemoteAddr/
// ConnID）两组字段，调用方按语义填充，避免多同名 string 位置参数错位。
type KickAudit struct {
	ActorSource   string // 操作者来源：AuditActorDesktop | AuditActorRPC
	ActorTokenID  string // 操作者令牌 ID（RPC 调用者；桌面为空）
	TargetTokenID string // 被踢连接的令牌 ID
	TargetName    string // 被踢连接的令牌名
	RemoteAddr    string // 被踢连接来源 IP（对端可重连，IP 足以对号入座）
	ConnID        uint64 // 被踢连接 ID
}

// LogConnKick 追加一条踢出连接审计事件（conn_kick，连接管控二期新增）：记录被踢
// 目标的令牌 ID/名称/来源地址/连接 ID 与操作者归因（actorSource/actorTokenId，
// 连接管控三期新增——多令牌场景可回答「谁踢的」）。可见性口径与 Log 一致——无
// 明文 token、无 URL；RemoteAddr 取连接明细快照的来源 IP。
// 写入时序对齐既有审计纪律：仅动作生效（hub 命中）后由调用方落盘，miss 不写。
func (a *AuditLogger) LogConnKick(k KickAudit) {
	a.write(AuditRecord{
		Ts:           time.Now().Unix(),
		Event:        AuditEventConnKick,
		TokenID:      k.TargetTokenID,
		Name:         k.TargetName,
		RemoteAddr:   k.RemoteAddr,
		ConnID:       k.ConnID,
		ActorSource:  k.ActorSource,
		ActorTokenID: k.ActorTokenID,
	})
}

// BanAudit token_ban/token_unban 事件载荷（结构化参数，对齐 KickAudit 形态防
// 多同名 string 位置参数错位）：操作者归因与封禁目标两组字段，token_unban 仅
// 消费归因与目标三字段（解封无备注/到期语义）。
type BanAudit struct {
	ActorSource   string // 操作者来源：AuditActorDesktop | AuditActorRPC
	ActorTokenID  string // 操作者令牌 ID（RPC 调用者；桌面为空）
	TargetTokenID string // 被封禁/解封的令牌记录 ID
	TargetName    string // 被封禁/解封的令牌名
	Reason        string // 封禁备注（token_ban 携带；未填为空串不落键）
	ExpiresAt     int64  // 封禁到期 unix 秒（0=永久，token_ban 携带）
}

// LogBan 追加一条封禁令牌审计事件（token_ban，连接管控四期）：记录封禁参数
// （到期时间/备注）与操作者归因。可见性口径与 Log 一致——无明文 token、无 URL。
// 写入时序对齐既有审计纪律：仅动作生效（含落盘成功）后由调用方落盘，失败不写。
func (a *AuditLogger) LogBan(b BanAudit) {
	a.write(AuditRecord{
		Ts:           time.Now().Unix(),
		Event:        AuditEventTokenBan,
		TokenID:      b.TargetTokenID,
		Name:         b.TargetName,
		ActorSource:  b.ActorSource,
		ActorTokenID: b.ActorTokenID,
		Reason:       b.Reason,
		ExpiresAt:    b.ExpiresAt,
	})
}

// LogUnban 追加一条解封令牌审计事件（token_unban，连接管控四期）。
func (a *AuditLogger) LogUnban(b BanAudit) {
	a.write(AuditRecord{
		Ts:           time.Now().Unix(),
		Event:        AuditEventTokenUnban,
		TokenID:      b.TargetTokenID,
		Name:         b.TargetName,
		ActorSource:  b.ActorSource,
		ActorTokenID: b.ActorTokenID,
	})
}

// IPBanAudit ip_ban/ip_unban 事件载荷（结构化参数，对齐 BanAudit 形态防多同名
// 位置参数错位）：ip_unban 仅消费 IP 与操作者归因三字段（解封无计数/到期/备注
// 语义）；ActorTokenID 为 RPC 调用者令牌记录 ID（桌面路径为空不落键，与
// ActorSource 正交互补——多令牌场景可回答「谁解封的」，对齐四期 token_unban）。
type IPBanAudit struct {
	IP           string // 封禁/解封的来源 IP（已归一形态）
	FailCount    int    // 触发封禁时滑动窗口内认证失败计数（ip_ban 携带）
	Strike       int    // 累计触发封禁次数（连接管控六期递增封禁，ip_ban 携带）
	ExpiresAt    int64  // 封禁到期 unix 秒（ip_ban 携带）
	Reason       string // 封禁原因文案（ip_ban 携带，含触发计数与档位时长描述）
	ActorSource  string // 操作者来源：AuditActorSystem（自动封禁）| AuditActorDesktop | AuditActorRPC（手动解封）
	ActorTokenID string // 操作者令牌记录 ID（RPC 解封调用者；桌面/自动触发为空不落键）
}

// LogIPBan 追加一条来源 IP 封禁审计事件（ip_ban，连接管控五期；七期复用单事件
// 扩手动封禁归因）：记录触发计数、到期时间与操作者归因——自动触发恒 system 且
// ActorTokenID 恒空（无人类操作者）；手动封禁（七期）actorSource 为 desktop/rpc、
// ActorTokenID 为 RPC 调用者令牌记录 ID（桌面为空不落键）。可见性口径与 Log
// 一致——无明文 token、无 URL。写入时序对齐既有审计纪律：仅动作生效（封禁
// 写入内存，手动封禁另含落盘成功）后由调用方落盘。
func (a *AuditLogger) LogIPBan(b IPBanAudit) {
	a.write(AuditRecord{
		Ts:           time.Now().Unix(),
		Event:        AuditEventIPBan,
		IP:           b.IP,
		FailCount:    b.FailCount,
		Strike:       b.Strike,
		ExpiresAt:    b.ExpiresAt,
		Reason:       b.Reason,
		ActorSource:  b.ActorSource,
		ActorTokenID: b.ActorTokenID,
	})
}

// LogIPUnban 追加一条手动解封来源 IP 审计事件（ip_unban，连接管控五期）：操作者
// 归因（ActorSource + ActorTokenID，RPC 路径携带调用者令牌记录 ID，桌面为空不落键）。
func (a *AuditLogger) LogIPUnban(b IPBanAudit) {
	a.write(AuditRecord{
		Ts:           time.Now().Unix(),
		Event:        AuditEventIPUnban,
		IP:           b.IP,
		ActorSource:  b.ActorSource,
		ActorTokenID: b.ActorTokenID,
	})
}

// write 序列化并追加一条审计记录（全部 Log* 出口的单点收口：行构造、轮转、
// 建目录、追加写均在此处，避免多出口实现漂移）。
func (a *AuditLogger) write(rec AuditRecord) {
	line, err := json.Marshal(rec)
	if err != nil {
		// 全为基本类型字段不会失败，防御保留
		slog.Error("marshal audit record failed", "event", rec.Event, "err", err)
		return
	}
	line = append(line, '\n')

	a.mu.Lock()
	defer a.mu.Unlock()
	a.rotateLocked(int64(len(line)))
	// 父目录不存在时自动创建（data/ 目录约定；裸目录创建失败由下方 open 兜底告警）
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		slog.Warn("create audit log dir failed", "path", a.path, "err", err)
		return
	}
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		slog.Warn("open audit log failed", "path", a.path, "err", err)
		return
	}
	if _, err := f.Write(line); err != nil {
		slog.Warn("write audit log failed", "path", a.path, "err", err)
	}
	_ = f.Close()
}

// rotateLocked 持锁轮转（连接管控十一期多档链）：当前文件追加本行后将超过上限时，
// 链式下移历史档（.N-1→.N … .1→.2，源档不存在跳过——链前端缺失不断链）再将主
// 文件 rename 为 <path>.1，淘汰最旧档。Go os.Rename 在 Windows 走 MoveFileEx
// REPLACE_EXISTING（覆盖已存在的目标档）。
//
// 任一环失败即告警停链：后续档保持现状、主文件继续追加（超限容忍，下条再试）——
// 链中断只影响该轮可达深度，不丢行不重排。失败实例之一即读写竞速的 Windows 形态：
// Go os.Open 句柄不透出 DELETE 共享语义，读侧（readSnapshot/QueryAuditLog 懒扫）
// 正打开某档时 rename 该档报 sharing violation，本轮停链、读侧关闭后下一行自愈。
// 损坏行不处理（读侧按行解析，坏行跳过属读方容错）。
func (a *AuditLogger) rotateLocked(incoming int64) {
	info, err := os.Stat(a.path)
	if err != nil || info.Size()+incoming <= a.maxBytes {
		return
	}
	for i := a.depth - 1; i >= 1; i-- {
		if _, err := os.Stat(a.rotatedPath(i)); err != nil {
			continue // 源档不存在（链未轮转满/前端缺失），跳过
		}
		if err := os.Rename(a.rotatedPath(i), a.rotatedPath(i+1)); err != nil {
			slog.Warn("rotate audit log failed", "path", a.rotatedPath(i), "err", err)
			return
		}
	}
	if err := os.Rename(a.path, a.path+".1"); err != nil {
		// 轮转失败不阻塞本行写入：继续向当前文件追加（超限容忍，下条再试轮转）
		slog.Warn("rotate audit log failed", "path", a.path, "err", err)
	}
}

// webAuditQueryDefaultLimit / webAuditQueryMaxLimit 读侧分页缺省与单次上限：
// 5MB 文件约 3 万行，全量过桥既浪费也无必要；上限钳制防一次拉全量。
const (
	webAuditQueryDefaultLimit = 100
	webAuditQueryMaxLimit     = 500
)

// AuditQueryOptions 审计历史查询条件（连接管控三期读侧新增，桌面 UI 消费）。
// 全部条件为与语义（逐项命中才保留）；零值项表示该维度不过滤。
type AuditQueryOptions struct {
	Events  []string // 事件类型白名单（空 = 全部事件）
	From    int64    // 起始时间（unix 秒，含；0 = 不限）
	To      int64    // 结束时间（unix 秒，含；0 = 不限）
	TokenID string   // 令牌 ID（命中 TokenID 或 ActorTokenID 任一即相关：同一令牌
	//  既可能是事件主体（token_*）、认证者（auth_*）也可能是踢出操作者
	// （conn_kick 的 actorTokenId），按「与该令牌相关」口径过滤）
	Offset int // 分页偏移（过滤后、时间倒序序列上的偏移）
	Limit  int // 页大小（<=0 取缺省，超上限钳制）
}

// AuditQueryResult 审计历史查询结果。Records 恒非 nil（时间倒序，最新在前），
// Total 为过滤后总数（分页窗口外的量），前端据此渲染分页器。
type AuditQueryResult struct {
	Total   int
	Records []AuditRecord
}

// QueryAuditLog 查询审计历史（读侧 API，连接管控三期新增）：合并读轮转链全部
// 档位（旧，<path>.N…<path>.1）与主文件（新），过滤后按时间倒序分页返回。
//
// 快照语义：打开-读-关闭，不持写锁——与写侧 O_APPEND 单行原子追加并发安全。
// 读写竞速的平台分叉（连接管控十一期多档链实测）：Windows 下读打开的档使写侧
// rename 该档失败（sharing violation，写侧停链降级自愈，见 rotateLocked），读侧
// 拿到的是轮转前一致快照；Unix 下 rename 不受阻，读期间发生轮转窗口时可能重复
// 或漏读边界单行。两者均属可接受偏差（本 API 为排查观测面，非正确性依赖，与
// 写侧旁路降级同款定位）。坏行（非法 JSON 或缺 event 字段）跳过——写侧不保证
// 行完整性（崩溃截断/外部手改），读方容错。
func (a *AuditLogger) QueryAuditLog(opts AuditQueryOptions) (AuditQueryResult, error) {
	all, err := a.readSnapshot()
	if err != nil {
		return AuditQueryResult{}, err
	}
	eventSet := make(map[string]bool, len(opts.Events))
	for _, e := range opts.Events {
		eventSet[e] = true
	}
	filtered := make([]AuditRecord, 0, len(all))
	for _, rec := range all {
		if len(eventSet) > 0 && !eventSet[rec.Event] {
			continue
		}
		if opts.From > 0 && rec.Ts < opts.From {
			continue
		}
		if opts.To > 0 && rec.Ts > opts.To {
			continue
		}
		if opts.TokenID != "" && rec.TokenID != opts.TokenID && rec.ActorTokenID != opts.TokenID {
			continue
		}
		filtered = append(filtered, rec)
	}

	result := AuditQueryResult{Total: len(filtered), Records: []AuditRecord{}}
	limit := opts.Limit
	if limit <= 0 {
		limit = webAuditQueryDefaultLimit
	}
	if limit > webAuditQueryMaxLimit {
		limit = webAuditQueryMaxLimit
	}
	start := opts.Offset
	if start < 0 {
		start = 0
	}
	if start >= len(filtered) {
		return result, nil
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	// 倒序取窗：filtered 为时间正序（文件读取序），倒序序列上第 j 项即
	// filtered[len-1-j]；按 start..end-1 顺追加即页内最新在前
	for j := start; j < end; j++ {
		result.Records = append(result.Records, filtered[len(filtered)-1-j])
	}
	return result, nil
}

// readSnapshot 依次读轮转链全部档位与主文件（时间正序拼接，最旧档在前），逐行
// 解析审计记录。档位列表按当前 depth 构造（path.N … path.1 + 主文件）；depth
// 经设置热更缩小后，残留的超深度旧档不入列表也不删除（静默滞留磁盘，容量口径
// 以新配置为准）。depth 在锁内快照后再开文件（读侧不持写锁，与热更竞态下取
// 新旧任一深度均合法）。文件不存在（首次使用/尚未轮转/链未满）跳过，非"不存在"
// 的打开错误上抛；坏行跳过；单文件读中断（Scanner 错误）按快照容错截断该文件
// 继续下一文件。
func (a *AuditLogger) readSnapshot() ([]AuditRecord, error) {
	a.mu.Lock()
	depth := a.depth
	a.mu.Unlock()
	paths := make([]string, 0, depth+1)
	for i := depth; i >= 1; i-- {
		paths = append(paths, a.rotatedPath(i))
	}
	paths = append(paths, a.path)
	var recs []AuditRecord
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			var rec AuditRecord
			if json.Unmarshal(line, &rec) != nil || rec.Event == "" {
				continue
			}
			recs = append(recs, rec)
		}
		_ = f.Close()
	}
	return recs, nil
}
