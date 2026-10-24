package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// DefaultWebTokenName 存量单 token 迁移与轮换编排生成记录的默认名称。
const DefaultWebTokenName = "默认"

// lastUsedFlushDelay lastUsedAt 变更的 debounce 落盘延迟：认证命中是高频路径
// （server requireToken 对静态资产每请求都走），逐请求写盘写放大不可接受；
// 内存更新后延迟批量 flush。重启丢弃最近数秒的 lastUsedAt 精度，可接受
// （非正确性数据，仅列表展示用途）。
const lastUsedFlushDelay = 5 * time.Second

// hashToken 计算明文 token 的 SHA-256 hex（落盘唯一形态；明文不持久化）。
func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// WebTokenService 浏览器访问通道的多 token 管理服务（GitHub PAT 式）。
//
// 存储 data/web_tokens.json（util.SaveJSON 原子写；损坏降级空列表 + slog.Warn，
// 对齐 SessionService 模式）。存量单 token 文件（data/web_token）首次加载时
// 迁移为首条记录（名称「默认」）；两文件均不存在（全新安装）时生成初始 token，
// 明文同步写 data/web_token（0600）保住既有「查看令牌」链路。
//
// Verify 作为 serve 认证链的校验函数直接注入 server.WebOptions.Verify 与
// WSHub——闭包持本服务引用，无状态快照，新建/撤销/过期在服务层即时生效，
// 无需任何「热轮换通知」（单 token 时代的 RotateToken 职责由此消解）。
type WebTokenService struct {
	mu         sync.Mutex
	path       string // data/web_tokens.json
	legacyPath string // data/web_token（存量单 token：迁移源 + 明文兼容显示文件）
	list       model.WebTokenList
	// bansPath 封禁记录文件（data/web_bans.json，由 path 同目录推导，构造后
	// 只读）与内存态（连接管控四期：tokenID 维度临时拒连状态机，懒加载与
	// 落盘同 mu 临界区）。撤销令牌联动清理对应记录。
	bansPath string
	bans     model.WebBanList
	loaded   bool
	nowFn      func() time.Time       // 时间源（测试注入驱动过期/防 sleep）
	genToken   func() (string, error) // 明文 token 生成（测试注入；默认 32 字节 crypto/rand hex）
	audit      *AuditLogger           // 审计出口（可 nil：单测不注审计时静默跳过）
	flushDelay time.Duration          // lastUsedAt debounce 延迟（测试收紧）
	flushTimer *time.Timer            // 待执行的 debounce flush（nil = 无在途）
	dirty      bool                   // lastUsedAt 有待落盘变更
}

// NewWebTokenService 构造多 token 管理服务。
//
// path 为多 token 记录文件（data/web_tokens.json）；legacyPath 为存量单 token
// 明文文件（data/web_token，迁移源 + 明文兼容显示文件）。不生成 32 字节随机
// 明文的生成逻辑不反向依赖 server 包（server.GenerateToken 等价实现内联，
// 保持 service → server 无耦合）。加载为懒式（ensureLoadedLocked），调用方
// 可经 EnsureLoaded 在启动期显式触发（推荐：存量迁移与初始引导在启动期完成，
// 明文兼容文件就绪不依赖首个认证请求）。
func NewWebTokenService(path, legacyPath string) *WebTokenService {
	return &WebTokenService{
		path:       path,
		legacyPath: legacyPath,
		bansPath:   filepath.Join(filepath.Dir(path), "web_bans.json"),
		nowFn:      time.Now,
		genToken: func() (string, error) {
			buf := make([]byte, 32)
			if _, err := rand.Read(buf); err != nil {
				return "", fmt.Errorf("生成随机令牌失败: %w", err)
			}
			return hex.EncodeToString(buf), nil
		},
		flushDelay: lastUsedFlushDelay,
	}
}

// SetAudit 注入审计出口（token 生命周期事件落盘）。
func (s *WebTokenService) SetAudit(a *AuditLogger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = a
}

// EnsureLoaded 显式触发加载/迁移/引导（幂等）。NewAppServices 启动期调用：
// 保证 data/web_token 明文兼容文件在首个认证请求前就绪（serve 控制台提示与
// 设置页「查看令牌」链路依赖该文件）。
func (s *WebTokenService) EnsureLoaded() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
}

// ensureLoadedLocked 首次加载（调用方须持 mu）：
//   - data/web_tokens.json 存在 → 加载；损坏降级空列表（对齐 SessionService
//     模式，损坏不阻塞启动；空列表下认证天然 fail closed）；
//   - 不存在 → 引导（bootstrapLocked）：存量迁移或全新生成。
func (s *WebTokenService) ensureLoadedLocked() {
	if s.loaded {
		return
	}
	s.loaded = true
	s.list = model.WebTokenList{Version: model.WebTokenListVersion}
	s.bans = model.WebBanList{Version: model.WebBanListVersion}
	if util.FileExists(s.path) {
		if err := util.LoadJSON(s.path, &s.list); err != nil {
			Logger().Warn("web tokens file corrupt, starting from empty list",
				"path", s.path, "err", err)
			s.list = model.WebTokenList{Version: model.WebTokenListVersion}
		}
	} else {
		s.bootstrapLocked()
	}
	s.loadBansLocked()
}

// loadBansLocked 加载封禁记录文件（调用方须持 mu；ensureLoadedLocked 专属）。
// 文件缺失（尚未使用过封禁功能）为常态空列表；损坏降级空列表不阻塞令牌链路
// （封禁为管控增强面，宁放行不可阻断认证；代价为重启后需重新下发封禁，可
// 接受——对齐 web_tokens.json 损坏降级模式）。
func (s *WebTokenService) loadBansLocked() {
	if !util.FileExists(s.bansPath) {
		return
	}
	if err := util.LoadJSON(s.bansPath, &s.bans); err != nil {
		Logger().Warn("web bans file corrupt, starting from empty list",
			"path", s.bansPath, "err", err)
		s.bans = model.WebBanList{Version: model.WebBanListVersion}
	}
}

// saveBansLocked 持久化封禁记录（调用方须持 mu；util.SaveJSON 原子写）。
func (s *WebTokenService) saveBansLocked() error {
	return util.SaveJSON(s.bansPath, s.bans)
}

// bootstrapLocked 多 token 文件不存在时的引导（调用方须持 mu，且仅在文件缺失
// 分支进入）：
//   - 存量单 token 文件（data/web_token）存在且非空 → 迁移：读明文 → SHA-256 →
//     建首条记录（名称「默认」）→ 保存新文件。旧文件保留不删（回滚安全：旧版本
//     二进制仍按单 token 语义读取该文件可继续工作）。
//   - 存量文件不存在/为空/读取失败 → 全新引导：生成初始 token，记录落新文件，
//     明文同步写 legacy 文件（0600）——既有「查看令牌」链路（GetWebServeToken）
//     与 serve 控制台「访问令牌见 data/web_token」提示继续可用。
func (s *WebTokenService) bootstrapLocked() {
	plain := ""
	if data, err := os.ReadFile(s.legacyPath); err == nil {
		plain = strings.TrimSpace(string(data))
	} else if !os.IsNotExist(err) {
		// 非缺失类读取错误（权限等）：按全新引导处理并留痕
		Logger().Warn("read legacy web token failed, bootstrapping fresh token",
			"path", s.legacyPath, "err", err)
	}
	if plain == "" {
		var err error
		if plain, err = s.genToken(); err != nil {
			// 随机源失败属致命环境问题：不产生任何记录（认证 fail closed），仅留痕
			Logger().Error("generate bootstrap web token failed", "err", err)
			return
		}
		s.writeLegacyCompatLocked(plain)
	}
	rec := s.appendTokenLocked(plain, DefaultWebTokenName, 0)
	if err := util.SaveJSON(s.path, s.list); err != nil {
		Logger().Error("save bootstrapped web tokens failed", "path", s.path, "err", err)
	}
	s.auditEventLocked(AuditEventTokenMigrate, rec.ID, rec.Name)
}

// writeLegacyCompatLocked 将明文写存量兼容文件（0600，语义对齐 server.SaveToken：
// 非 JSON token 写，豁免 util.SaveJSON 原子写口径——文件仅引导/轮换时低频写入）。
// 失败仅告警降级：兼容文件缺失只影响「查看令牌」展示链路，不影响认证。
func (s *WebTokenService) writeLegacyCompatLocked(plain string) {
	dir := filepath.Dir(s.legacyPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		Logger().Warn("create legacy token dir failed", "path", dir, "err", err)
		return
	}
	if err := os.WriteFile(s.legacyPath, []byte(plain+"\n"), 0o600); err != nil {
		Logger().Warn("write legacy token file failed", "path", s.legacyPath, "err", err)
	}
}

// List 返回全部 token 记录（按创建时间升序）。记录只含哈希不含明文；返回
// 切片为内部数据拷贝，调用方修改不影响服务状态。空列表返回空切片（非 nil，
// Wails 序列化为 [] 而非 null）。
func (s *WebTokenService) List() []model.WebTokenRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make([]model.WebTokenRecord, len(s.list.Tokens))
	copy(out, s.list.Tokens)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// Create 新建 token：生成随机明文（仅本次返回，不落盘），SHA-256 哈希入记录。
// name 为设备/用途名（去首尾空白后不得为空）；expiresAt 为过期 unix 秒
// （0 = 永不过期，非 0 须晚于当前时间）。审计 token_create。
func (s *WebTokenService) Create(name string, expiresAt int64) (string, model.WebTokenRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", model.WebTokenRecord{}, model.NewAppError(model.ErrCodeTokenNameRequired,
			"请填写令牌名称（用于标识设备或用途）")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	if expiresAt != 0 && expiresAt <= s.nowFn().Unix() {
		return "", model.WebTokenRecord{}, model.NewAppError(model.ErrCodeTokenExpiryInvalid,
			"有效期必须晚于当前时间")
	}
	plain, err := s.genToken()
	if err != nil {
		return "", model.WebTokenRecord{}, fmt.Errorf("生成访问令牌失败: %w", err)
	}
	rec := s.appendTokenLocked(plain, name, expiresAt)
	if err := util.SaveJSON(s.path, s.list); err != nil {
		// 落盘失败回滚内存追加，保持内存与磁盘一致（磁盘仍为旧集合）
		s.removeTokenLocked(rec.ID)
		return "", model.WebTokenRecord{}, fmt.Errorf("持久化令牌记录失败: %w", err)
	}
	s.auditEventLocked(AuditEventTokenCreate, rec.ID, rec.Name)
	return plain, rec, nil
}

// Revoke 撤销指定 token（立即生效：后续认证全部拒绝）。已撤销的重复撤销为
// no-op 成功（幂等：前端重复点击与并发撤销不报错）；记录不存在（列表过期）
// 返回 E_TOKEN_NOT_FOUND。记录保留不删除（审计与列表可见性，界面标记已撤销）。
func (s *WebTokenService) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	for i := range s.list.Tokens {
		if s.list.Tokens[i].ID != id {
			continue
		}
		if s.list.Tokens[i].Revoked {
			return nil
		}
		s.list.Tokens[i].Revoked = true
		if err := util.SaveJSON(s.path, s.list); err != nil {
			s.list.Tokens[i].Revoked = false
			return fmt.Errorf("持久化撤销状态失败: %w", err)
		}
		// 撤销生效后联动清理其封禁记录（撤销即凭据失效，封禁失去拒连对象）
		s.cleanupBanLocked(id)
		s.auditEventLocked(AuditEventTokenRevoke, id, s.list.Tokens[i].Name)
		return nil
	}
	return model.NewAppError(model.ErrCodeTokenNotFound, "令牌不存在或已被删除，请刷新列表")
}

// RevokeAllExcept 撤销除 exceptID 外的全部未撤销记录（轮换编排的撤旧步骤），
// 返回本次实际撤销的记录数。落盘失败整体回滚（撤销不生效），错误上抛。
func (s *WebTokenService) RevokeAllExcept(exceptID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	prev := append([]model.WebTokenRecord(nil), s.list.Tokens...)
	changed := 0
	// 审计事件在落盘成功后统一补发（对齐 Revoke 的「先持久化成功再审计」顺序）：
	// 落盘失败整体回滚时，未生效的撤销不得在审计面留痕
	revoked := make([]model.WebTokenRecord, 0, len(s.list.Tokens))
	for i := range s.list.Tokens {
		rec := &s.list.Tokens[i]
		if rec.ID == exceptID || rec.Revoked {
			continue
		}
		rec.Revoked = true
		changed++
		revoked = append(revoked, *rec)
	}
	if changed == 0 {
		return 0, nil
	}
	if err := util.SaveJSON(s.path, s.list); err != nil {
		s.list.Tokens = prev
		return 0, fmt.Errorf("持久化撤销状态失败: %w", err)
	}
	for _, rec := range revoked {
		// 撤销生效后联动清理其封禁记录（对齐 Revoke 单条撤销的联动语义）
		s.cleanupBanLocked(rec.ID)
		s.auditEventLocked(AuditEventTokenRevoke, rec.ID, rec.Name)
	}
	return changed, nil
}

// ===== 封禁状态机（连接管控四期）=====

// Ban 封禁指定令牌：tokenID 维度临时拒连——verify 放行后由 BanCheck 组合判定
// 拒绝（D4 决策，verify 保持凭据有效性单一语义），新连接（HTTP requireToken /
// WS 握手两认证点）401 携带专用消息，已建立连接不受影响（对齐撤销热生效语义，
// 仅拒新连；可选联动踢出为前端编排，见 docs/spec/browser-channel.md §2.3）。
//
// 幂等语义对齐 Revoke 先例的扩展形态：重复封禁 = 刷新封禁参数（BannedAt/
// ExpiresAt/Reason/归因整体覆盖为本次值，改时长/改永久均经此路径，记录恒唯一）。
// expiresAt 为到期 unix 秒（0 = 永久封禁须手动解封；非 0 须晚于当前时间，对齐
// Create 校验）。令牌不存在返回 E_TOKEN_NOT_FOUND；已撤销令牌拒绝（撤销即凭据
// 失效，封禁无拒连对象，E_TOKEN_REVOKED 引导刷新列表）。落盘失败回滚内存（对齐
// Create/Revoke 先例）；动作生效（含落盘成功）后才写 token_ban 审计行（失败不写，
// 对齐审计时序纪律）。
func (s *WebTokenService) Ban(id string, expiresAt int64, reason, actorSource, actorTokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	var target *model.WebTokenRecord
	for i := range s.list.Tokens {
		if s.list.Tokens[i].ID == id {
			target = &s.list.Tokens[i]
			break
		}
	}
	if target == nil {
		return model.NewAppError(model.ErrCodeTokenNotFound, "令牌不存在或已被删除，请刷新列表")
	}
	if target.Revoked {
		return model.NewAppError(model.ErrCodeTokenRevoked, "令牌已撤销（凭据已失效，无需封禁），请刷新列表")
	}
	if expiresAt != 0 && expiresAt <= s.nowFn().Unix() {
		return model.NewAppError(model.ErrCodeTokenExpiryInvalid, "封禁到期时间必须晚于当前时间")
	}
	ban := model.WebBanRecord{
		TokenID:      id,
		TokenName:    target.Name,
		BannedAt:     s.nowFn().Unix(),
		ExpiresAt:    expiresAt,
		Reason:       strings.TrimSpace(reason),
		ActorSource:  actorSource,
		ActorTokenID: actorTokenID,
	}
	prev := s.bans.Bans
	replaced := false
	for i := range s.bans.Bans {
		if s.bans.Bans[i].TokenID == id {
			s.bans.Bans[i] = ban
			replaced = true
			break
		}
	}
	if !replaced {
		s.bans.Bans = append(s.bans.Bans, ban)
	}
	if err := s.saveBansLocked(); err != nil {
		s.bans.Bans = prev
		return fmt.Errorf("持久化封禁记录失败: %w", err)
	}
	s.auditBanLocked(ban, actorSource, actorTokenID)
	return nil
}

// Unban 解封指定令牌：删除封禁记录，新连接立即可达。未封禁为 no-op 成功（幂等：
// 解封动作可重复触发不报错）；令牌不存在返回 E_TOKEN_NOT_FOUND（解封目标以令牌
// 名册为准——封禁记录随撤销联动清理，不存在「名册外孤儿封禁可解」形态）。落盘
// 失败回滚内存；生效后写 token_unban 审计行（未封禁 no-op 未改变状态不写审计）。
func (s *WebTokenService) Unban(id string, actorSource, actorTokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	name := ""
	found := false
	for i := range s.list.Tokens {
		if s.list.Tokens[i].ID == id {
			name = s.list.Tokens[i].Name
			found = true
			break
		}
	}
	if !found {
		return model.NewAppError(model.ErrCodeTokenNotFound, "令牌不存在或已被删除，请刷新列表")
	}
	idx := -1
	for i := range s.bans.Bans {
		if s.bans.Bans[i].TokenID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	prev := s.bans.Bans
	s.bans.Bans = append(s.bans.Bans[:idx], s.bans.Bans[idx+1:]...)
	if err := s.saveBansLocked(); err != nil {
		s.bans.Bans = prev
		return fmt.Errorf("持久化解封变更失败: %w", err)
	}
	s.auditUnbanLocked(id, name, actorSource, actorTokenID)
	return nil
}

// banExpiryMessageFormat 封禁拒绝消息的解封时间格式（精确到分钟：拒连提示面向
// 人读，秒级精度无决策价值；时区取本地）。
const banExpiryMessageFormat = "2006-01-02 15:04"

// BanRejectMessage 封禁拒绝消息（server 两认证点 BanCheck 的生产实现）：令牌
// 命中生效中的封禁记录时返回专用拒绝消息（含解封时间或「永久封禁」，与普通 401
// 「请携带访问令牌」可区分），未封禁返回空串（放行）。tokenID 为空串直接放行
// （verify 未命中不会走到封禁判定，防御保留）。
//
// 懒过期判定：ExpiresAt 非 0 且不晚于当前时间视为封禁已失效（放行），记录不
// 删除（列表保留「已过期」态可见性，禁后台扫描）。线程安全与 Verify 同锁：
// serve 认证高频路径下仅线性扫描小量级封禁列表，无可观开销。
func (s *WebTokenService) BanRejectMessage(tokenID string) string {
	if tokenID == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	for _, ban := range s.bans.Bans {
		if ban.TokenID != tokenID {
			continue
		}
		if ban.ExpiresAt != 0 && ban.ExpiresAt <= s.nowFn().Unix() {
			return ""
		}
		if ban.ExpiresAt == 0 {
			return "该访问令牌已被封禁（永久封禁），请联系管理员解封"
		}
		return "该访问令牌已被封禁，至 " + time.Unix(ban.ExpiresAt, 0).Format(banExpiryMessageFormat) + " 自动解封"
	}
	return ""
}

// ListBans 返回全部封禁记录（含已过期——列表「已过期」态展示数据源，拒连
// 判定的懒过期语义以 BanRejectMessage 为准）。返回切片为内部数据拷贝，调用方
// 修改不影响服务状态；空列表返回空切片（非 nil）。
func (s *WebTokenService) ListBans() []model.WebBanRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make([]model.WebBanRecord, len(s.bans.Bans))
	copy(out, s.bans.Bans)
	return out
}

// cleanupBanLocked 联动清理指定令牌的封禁记录（撤销令牌生效后调用：撤销即凭据
// 失效，封禁失去拒连对象，残留只会污染列表）。落盘失败回滚内存并告警降级，不
// 回滚已生效的撤销（撤销为主动作且已落盘；残留封禁记录对已撤销令牌无拒连危害
// ——verify 对撤销令牌先行拒绝，封禁判定不可达）。
func (s *WebTokenService) cleanupBanLocked(tokenID string) {
	idx := -1
	for i := range s.bans.Bans {
		if s.bans.Bans[i].TokenID == tokenID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	prev := s.bans.Bans
	s.bans.Bans = append(s.bans.Bans[:idx], s.bans.Bans[idx+1:]...)
	if err := s.saveBansLocked(); err != nil {
		s.bans.Bans = prev
		Logger().Warn("cleanup ban record after revoke failed",
			"path", s.bansPath, "tokenId", tokenID, "err", err)
	}
}

// auditBanLocked 发 token_ban 审计行（调用方须持 mu；audit 为 nil 时静默跳过）。
// 含操作者归因与封禁参数，经 BanAudit 结构化参数防位置参数错位（对齐 KickAudit）。
func (s *WebTokenService) auditBanLocked(ban model.WebBanRecord, actorSource, actorTokenID string) {
	if s.audit == nil {
		return
	}
	s.audit.LogBan(BanAudit{
		ActorSource:   actorSource,
		ActorTokenID:  actorTokenID,
		TargetTokenID: ban.TokenID,
		TargetName:    ban.TokenName,
		Reason:        ban.Reason,
		ExpiresAt:     ban.ExpiresAt,
	})
}

// auditUnbanLocked 发 token_unban 审计行（调用方须持 mu；解封无备注/到期语义）。
func (s *WebTokenService) auditUnbanLocked(tokenID, tokenName, actorSource, actorTokenID string) {
	if s.audit == nil {
		return
	}
	s.audit.LogUnban(BanAudit{
		ActorSource:   actorSource,
		ActorTokenID:  actorTokenID,
		TargetTokenID: tokenID,
		TargetName:    tokenName,
	})
}

// Verify 校验候选明文凭据：SHA-256 后与全部记录哈希恒定时间比对（subtle，
// 逐记录比较防时序侧信道）；命中且未被撤销、未过期（ExpiresAt==0 或晚于当前
// 时间，惰性失效无后台清理）则返回记录 ID。
//
// 遍历不因命中提前退出：单次校验耗时与记录数线性相关且与命中位置无关（记录
// 数为个人工具小量级，全遍历的开销可忽略）。命中且有效时更新内存 lastUsedAt
// 并调度 debounce 落盘（不逐请求写盘——requireToken 对静态资产每请求都走）。
// 过期/撤销的哈希命中不单独发审计事件：server 认证点对整体失败已落
// auth_failure，避免双计。
func (s *WebTokenService) Verify(candidate string) (string, bool) {
	if candidate == "" {
		return "", false
	}
	hash := hashToken(candidate)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	now := s.nowFn().Unix()
	found := ""
	for i := range s.list.Tokens {
		rec := &s.list.Tokens[i]
		if subtle.ConstantTimeCompare([]byte(hash), []byte(rec.Hash)) != 1 {
			continue
		}
		if rec.Revoked || (rec.ExpiresAt != 0 && rec.ExpiresAt <= now) {
			continue
		}
		found = rec.ID
		rec.LastUsedAt = now
		s.dirty = true
	}
	if found == "" {
		return "", false
	}
	s.scheduleFlushLocked()
	return found, true
}

// FlushNow 立即落盘待写的 lastUsedAt 变更（App.shutdown 收口调用，减少重启
// 丢弃的精度损失；测试亦用于确定性断言）。
func (s *WebTokenService) FlushNow() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.flushTimer != nil {
		s.flushTimer.Stop()
		s.flushTimer = nil
	}
	s.flushLocked()
}

// scheduleFlushLocked 调度一次 debounce flush（调用方须持 mu）：已有在途定时器
// 时不重复调度（窗口内的多次命中合并为一次落盘）。
func (s *WebTokenService) scheduleFlushLocked() {
	if s.flushTimer != nil {
		return
	}
	s.flushTimer = time.AfterFunc(s.flushDelay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.flushTimer = nil
		s.flushLocked()
	})
}

// flushLocked 落盘待写变更（调用方须持 mu）。仅在有脏数据时写。
func (s *WebTokenService) flushLocked() {
	if !s.dirty {
		return
	}
	s.dirty = false
	if err := util.SaveJSON(s.path, s.list); err != nil {
		Logger().Warn("flush web token lastUsedAt failed", "path", s.path, "err", err)
	}
}

// appendTokenLocked 追加一条记录（调用方须持 mu）：生成短随机 ID（8 位 hex，
// 碰撞重试），SHA-256 哈希落盘。
func (s *WebTokenService) appendTokenLocked(plain, name string, expiresAt int64) model.WebTokenRecord {
	rec := model.WebTokenRecord{
		ID:        s.newIDLocked(),
		Name:      name,
		Hash:      hashToken(plain),
		CreatedAt: s.nowFn().Unix(),
		ExpiresAt: expiresAt,
	}
	s.list.Tokens = append(s.list.Tokens, rec)
	return rec
}

// removeTokenLocked 按 ID 移除记录（Create 落盘失败回滚用）。
func (s *WebTokenService) removeTokenLocked(id string) {
	for i := range s.list.Tokens {
		if s.list.Tokens[i].ID == id {
			s.list.Tokens = append(s.list.Tokens[:i], s.list.Tokens[i+1:]...)
			return
		}
	}
}

// newIDLocked 生成 8 位 hex 短 ID（4 随机字节），与现有记录查重重试。随机源
// 失败时退化为时间纳秒低 32 位 hex（唯一性优先，仅极端环境降级）。
func (s *WebTokenService) newIDLocked() string {
	for {
		buf := make([]byte, 4)
		if _, err := rand.Read(buf); err != nil {
			return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
		}
		id := hex.EncodeToString(buf)
		conflict := false
		for i := range s.list.Tokens {
			if s.list.Tokens[i].ID == id {
				conflict = true
				break
			}
		}
		if !conflict {
			return id
		}
	}
}

// auditEventLocked 发一条 token 生命周期审计事件（调用方须持 mu；audit 为 nil
// 时静默跳过）。remoteAddr/userAgent 仅认证事件携带，生命周期事件留空。
func (s *WebTokenService) auditEventLocked(event, tokenID, name string) {
	if s.audit == nil {
		return
	}
	s.audit.Log(event, tokenID, name, "", "")
}
