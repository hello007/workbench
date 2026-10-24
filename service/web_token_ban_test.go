package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"workbench/model"
)

// requireAppErr 断言 err 为带指定错误码的 AppError（封禁域校验拒绝断言复用）。
func requireAppErr(t *testing.T, err error, code string) *model.AppError {
	t.Helper()
	if err == nil {
		t.Fatalf("应返回 %s 错误, got nil", code)
	}
	appErr, ok := err.(*model.AppError)
	if !ok || appErr.Code != code {
		t.Fatalf("应返回 %s, got %v", code, err)
	}
	return appErr
}

// banRecordByID 从封禁记录列表按 tokenID 取记录（miss 返回 false）。
func banRecordByID(bans []model.WebBanRecord, tokenID string) (model.WebBanRecord, bool) {
	for _, b := range bans {
		if b.TokenID == tokenID {
			return b, true
		}
	}
	return model.WebBanRecord{}, false
}

// TestWebTokenService_BanAndRejectMessage 封禁与拒绝消息：永久封禁返回含
// 「永久封禁」的专用消息；带到期时间返回含「至 YYYY-MM-DD HH:MM 自动解封」；
// 未封禁令牌返回空串（放行）。
func TestWebTokenService_BanAndRejectMessage(t *testing.T) {
	svc, _, now := newTestTokenService(t, "")
	_, recA, err := svc.Create("可疑设备", 0)
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	_, recB, err := svc.Create("正常设备", 0)
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}

	// 永久封禁
	if err := svc.Ban(recA.ID, 0, "异常行为", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban 永久: %v", err)
	}
	msg := svc.BanRejectMessage(recA.ID)
	if msg == "" {
		t.Fatal("永久封禁应返回非空拒绝消息")
	}
	if !strings.Contains(msg, "永久封禁") {
		t.Errorf("永久封禁消息应含「永久封禁」, got %q", msg)
	}

	// 带到期时间封禁：消息含解封时间（本地时区，精确到分钟）
	expires := now().Add(2 * time.Hour)
	if err := svc.Ban(recB.ID, expires.Unix(), "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban 定时: %v", err)
	}
	msg = svc.BanRejectMessage(recB.ID)
	if !strings.Contains(msg, "至 "+expires.Format(banExpiryMessageFormat)+" 自动解封") {
		t.Errorf("定时封禁消息应含解封时间 %q, got %q", expires.Format(banExpiryMessageFormat), msg)
	}

	// 未封禁令牌与空 tokenID 均放行
	if msg := svc.BanRejectMessage("no-such-id"); msg != "" {
		t.Errorf("未封禁令牌应返回空串, got %q", msg)
	}
	if msg := svc.BanRejectMessage(""); msg != "" {
		t.Errorf("空 tokenID 应返回空串, got %q", msg)
	}

	// 封禁记录字段齐备
	bans := svc.ListBans()
	if len(bans) != 2 {
		t.Fatalf("应恰有 2 条封禁记录, got %d", len(bans))
	}
	banA, ok := banRecordByID(bans, recA.ID)
	if !ok {
		t.Fatal("recA 封禁记录应存在")
	}
	if banA.TokenName != "可疑设备" || banA.ExpiresAt != 0 || banA.Reason != "异常行为" {
		t.Errorf("封禁记录字段不符: %+v", banA)
	}
	if banA.BannedAt != now().Unix() {
		t.Errorf("BannedAt 应为封禁时刻 %d, got %d", now().Unix(), banA.BannedAt)
	}
	if banA.ActorSource != AuditActorDesktop {
		t.Errorf("ActorSource 应为 desktop, got %q", banA.ActorSource)
	}
}

// TestWebTokenService_BanRefreshParams 重复封禁 = 刷新封禁参数（改时长/改永久
// 幂等路径）：记录恒唯一，BannedAt/ExpiresAt/Reason/归因整体覆盖为本次值。
func TestWebTokenService_BanRefreshParams(t *testing.T) {
	svc, _, now := newTestTokenService(t, "")
	_, rec, err := svc.Create("手机", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Ban(rec.ID, 0, "第一次", AuditActorDesktop, ""); err != nil {
		t.Fatalf("首次封禁: %v", err)
	}
	// 第二次封禁带新参数（永久 → 定时、换 reason、换归因）
	expires := now().Add(time.Hour).Unix()
	if err := svc.Ban(rec.ID, expires, "第二次", AuditActorRPC, "actor-1"); err != nil {
		t.Fatalf("重复封禁: %v", err)
	}

	bans := svc.ListBans()
	if len(bans) != 1 {
		t.Fatalf("重复封禁记录应恒唯一, got %d", len(bans))
	}
	ban := bans[0]
	if ban.ExpiresAt != expires || ban.Reason != "第二次" || ban.ActorSource != AuditActorRPC || ban.ActorTokenID != "actor-1" {
		t.Errorf("重复封禁应整体刷新参数: %+v", ban)
	}
	if ban.BannedAt != now().Unix() {
		t.Errorf("BannedAt 应刷新为本次封禁时刻, got %d", ban.BannedAt)
	}
	if svc.BanRejectMessage(rec.ID) == "" {
		t.Error("刷新后封禁应仍生效")
	}
}

// TestWebTokenService_BanValidation 封禁入参校验：令牌不存在 E_TOKEN_NOT_FOUND、
// 已撤销 E_TOKEN_REVOKED、到期时间过去 E_TOKEN_EXPIRY_INVALID（对齐 Create 校验）。
func TestWebTokenService_BanValidation(t *testing.T) {
	svc, _, now := newTestTokenService(t, "")
	_, rec, err := svc.Create("pad", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	requireAppErr(t, svc.Ban("no-such-id", 0, "", AuditActorDesktop, ""), model.ErrCodeTokenNotFound)

	if err := svc.Revoke(rec.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	requireAppErr(t, svc.Ban(rec.ID, 0, "", AuditActorDesktop, ""), model.ErrCodeTokenRevoked)

	// 重新建一条未撤销令牌验证过去时间拒绝
	_, rec2, err := svc.Create("pad2", 0)
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	past := now().Add(-time.Hour).Unix()
	requireAppErr(t, svc.Ban(rec2.ID, past, "", AuditActorDesktop, ""), model.ErrCodeTokenExpiryInvalid)
	if len(svc.ListBans()) != 0 {
		t.Error("校验拒绝时不应产生封禁记录")
	}
}

// TestWebTokenService_BanLazyExpiry 封禁懒过期：到期前拒绝、到期后放行（消息
// 空串），记录不删除（列表保留「已过期」态可见性）；时间经 nowFn 注入推进，
// 不依赖 sleep 与真实时钟。
func TestWebTokenService_BanLazyExpiry(t *testing.T) {
	svc, _, now := newTestTokenService(t, "")
	_, rec, err := svc.Create("临时设备", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Ban(rec.ID, now().Add(time.Hour).Unix(), "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban: %v", err)
	}
	if svc.BanRejectMessage(rec.ID) == "" {
		t.Fatal("到期前封禁应生效")
	}

	// 推进时间源跨到期点
	svc.mu.Lock()
	svc.nowFn = advanceNowFn(now, 2*time.Hour)
	svc.mu.Unlock()
	if msg := svc.BanRejectMessage(rec.ID); msg != "" {
		t.Errorf("到期后应放行（空串）, got %q", msg)
	}
	// 记录保留不删（「已过期」态可见性）
	bans := svc.ListBans()
	if len(bans) != 1 || bans[0].TokenID != rec.ID {
		t.Errorf("过期封禁记录应保留, got %+v", bans)
	}
	// 到期后可重新封禁（视为新一次封禁动作刷新参数）
	if err := svc.Ban(rec.ID, 0, "再次封禁", AuditActorDesktop, ""); err != nil {
		t.Fatalf("过期后重新封禁: %v", err)
	}
	if svc.BanRejectMessage(rec.ID) == "" {
		t.Error("重新封禁后应再次生效")
	}
}

// TestWebTokenService_Unban 解封语义：封禁后解封放行且记录删除；未封禁 no-op
// 成功（幂等）；令牌不存在 E_TOKEN_NOT_FOUND。
func TestWebTokenService_Unban(t *testing.T) {
	svc, _, _ := newTestTokenService(t, "")
	_, rec, err := svc.Create("手机", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 未封禁解封 no-op 成功
	if err := svc.Unban(rec.ID, AuditActorDesktop, ""); err != nil {
		t.Fatalf("未封禁解封应 no-op 成功, got %v", err)
	}

	if err := svc.Ban(rec.ID, 0, "异常", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban: %v", err)
	}
	if err := svc.Unban(rec.ID, AuditActorDesktop, ""); err != nil {
		t.Fatalf("Unban: %v", err)
	}
	if msg := svc.BanRejectMessage(rec.ID); msg != "" {
		t.Errorf("解封后应放行, got %q", msg)
	}
	if len(svc.ListBans()) != 0 {
		t.Error("解封应删除封禁记录")
	}
	// 重复解封仍 no-op 成功
	if err := svc.Unban(rec.ID, AuditActorDesktop, ""); err != nil {
		t.Errorf("重复解封应幂等 no-op, got %v", err)
	}
	// 不存在令牌
	requireAppErr(t, svc.Unban("no-such-id", AuditActorDesktop, ""), model.ErrCodeTokenNotFound)
}

// TestWebTokenService_RevokeCleansBan 撤销令牌联动清理封禁记录：Revoke 单条与
// RevokeAllExcept 批量两路径均清理；封禁已撤销令牌被拒（E_TOKEN_REVOKED）。
func TestWebTokenService_RevokeCleansBan(t *testing.T) {
	svc, _, _ := newTestTokenService(t, "")
	_, recA, err := svc.Create("设备A", 0)
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	_, recB, err := svc.Create("设备B", 0)
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	_, recKeep, err := svc.Create("保留设备", 0)
	if err != nil {
		t.Fatalf("Create keep: %v", err)
	}
	for _, id := range []string{recA.ID, recB.ID, recKeep.ID} {
		if err := svc.Ban(id, 0, "", AuditActorDesktop, ""); err != nil {
			t.Fatalf("Ban %s: %v", id, err)
		}
	}

	// 单条撤销联动清理
	if err := svc.Revoke(recA.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, ok := banRecordByID(svc.ListBans(), recA.ID); ok {
		t.Error("撤销后应联动清理封禁记录（Revoke 路径）")
	}
	// 批量撤旧联动清理（轮换编排路径）
	if _, err := svc.RevokeAllExcept(recKeep.ID); err != nil {
		t.Fatalf("RevokeAllExcept: %v", err)
	}
	bans := svc.ListBans()
	if len(bans) != 1 || bans[0].TokenID != recKeep.ID {
		t.Errorf("批量撤销后应仅保留未撤销令牌的封禁记录, got %+v", bans)
	}
	// 封禁已撤销令牌拒绝
	requireAppErr(t, svc.Ban(recB.ID, 0, "", AuditActorDesktop, ""), model.ErrCodeTokenRevoked)
}

// TestWebTokenService_BansPersistAcrossRestart 封禁持久化：同目录重建服务实例
// （重启形态）封禁持续生效。
func TestWebTokenService_BansPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	tokensPath := filepath.Join(dir, "web_tokens.json")
	legacyPath := filepath.Join(dir, "web_token")

	buildSvc := func() *WebTokenService {
		svc := NewWebTokenService(tokensPath, legacyPath)
		base := time.Unix(1_700_000_000, 0)
		svc.nowFn = func() time.Time { return base }
		return svc
	}
	svc := buildSvc()
	svc.EnsureLoaded()
	plain, rec, err := svc.Create("手机", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	expires := time.Unix(1_700_000_000, 0).Add(24 * time.Hour).Unix()
	if err := svc.Ban(rec.ID, expires, "异常", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban: %v", err)
	}

	// 重启：新实例同目录加载，封禁持续生效
	restarted := buildSvc()
	restarted.EnsureLoaded()
	if msg := restarted.BanRejectMessage(rec.ID); msg == "" {
		t.Fatal("重启后封禁应持续生效")
	} else if !strings.Contains(msg, "自动解封") {
		t.Errorf("定时封禁消息形态不符: %q", msg)
	}
	if id, ok := restarted.Verify(plain); !ok || id != rec.ID {
		t.Errorf("封禁不影响 verify 凭据判定（组合判定分离）, id=%q ok=%v", id, ok)
	}
	// 落盘文件不含明文 token
	raw, err := os.ReadFile(filepath.Join(dir, "web_bans.json"))
	if err != nil {
		t.Fatalf("读封禁文件: %v", err)
	}
	if strings.Contains(string(raw), plain) {
		t.Error("封禁文件不得包含明文 token")
	}
	var list model.WebBanList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("封禁文件应为合法 JSON: %v", err)
	}
	if list.Version != model.WebBanListVersion || len(list.Bans) != 1 || list.Bans[0].ExpiresAt != expires {
		t.Errorf("封禁文件形态不符: version=%d bans=%+v", list.Version, list.Bans)
	}
}

// TestWebTokenService_BansCorruptDowngrade 封禁文件损坏降级：空列表 + 告警，
// 不阻塞令牌链路（认证照常、封禁功能可重新下发）。
func TestWebTokenService_BansCorruptDowngrade(t *testing.T) {
	dir := t.TempDir()
	tokensPath := filepath.Join(dir, "web_tokens.json")
	legacyPath := filepath.Join(dir, "web_token")
	bansPath := filepath.Join(dir, "web_bans.json")
	if err := os.WriteFile(bansPath, []byte("{not-valid-json"), 0o600); err != nil {
		t.Fatalf("写损坏封禁文件: %v", err)
	}

	svc := NewWebTokenService(tokensPath, legacyPath)
	svc.nowFn = func() time.Time { return time.Unix(1_700_000_000, 0) }
	svc.EnsureLoaded()

	// 令牌链路不受影响：引导 + 创建 + 认证照常
	_, rec, err := svc.Create("手机", 0)
	if err != nil {
		t.Fatalf("损坏降级后 Create 不应受阻: %v", err)
	}
	if svc.BanRejectMessage(rec.ID) != "" {
		t.Error("降级空列表下不应有封禁命中")
	}
	// 封禁功能可重新下发并落盘（损坏文件被新列表覆盖）
	if err := svc.Ban(rec.ID, 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("降级后重新封禁: %v", err)
	}
	if svc.BanRejectMessage(rec.ID) == "" {
		t.Error("重新封禁应生效")
	}
}

// TestWebTokenService_BanSaveFailureRollback 封禁落盘失败回滚内存（对齐
// Create/Revoke 先例）：bansPath 父路径指向已存在文件（MkdirAll 报 ENOTDIR，
// 跨平台确定性失败），内存不产生记录、不写审计行。
func TestWebTokenService_BanSaveFailureRollback(t *testing.T) {
	svc, _, _ := newTestTokenService(t, "")
	audit := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	svc.SetAudit(audit)
	_, rec, err := svc.Create("手机", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 落盘失败构造：bansPath 父路径为已存在文件（ENOTDIR 确定性失败）
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备阻塞文件: %v", err)
	}
	svc.mu.Lock()
	svc.bansPath = filepath.Join(blocker, "web_bans.json")
	svc.mu.Unlock()

	if err := svc.Ban(rec.ID, 0, "异常", AuditActorDesktop, ""); err == nil {
		t.Fatal("落盘失败应报错")
	}
	// 内存回滚：无封禁记录、消息为空
	if len(svc.ListBans()) != 0 {
		t.Errorf("落盘失败应回滚内存记录, got %+v", svc.ListBans())
	}
	if svc.BanRejectMessage(rec.ID) != "" {
		t.Error("回滚后不应有封禁命中")
	}
	// 失败不写审计（动作未生效）
	for _, line := range readAuditLines(t, audit.path) {
		if line.Event == AuditEventTokenBan {
			t.Fatalf("落盘失败不应写 token_ban 审计, got %+v", line)
		}
	}

	// 解封落盘失败同样回滚（记录保留）
	svc.mu.Lock()
	svc.bansPath = filepath.Join(t.TempDir(), "web_bans.json")
	svc.mu.Unlock()
	if err := svc.Ban(rec.ID, 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("恢复可写后封禁: %v", err)
	}
	svc.mu.Lock()
	svc.bansPath = filepath.Join(blocker, "web_bans.json")
	svc.mu.Unlock()
	if err := svc.Unban(rec.ID, AuditActorDesktop, ""); err == nil {
		t.Fatal("解封落盘失败应报错")
	}
	if len(svc.ListBans()) != 1 {
		t.Errorf("解封落盘失败应回滚（记录保留）, got %d", len(svc.ListBans()))
	}
}

// TestWebTokenService_BanAuditTrail 封禁审计行：token_ban 含封禁参数与操作者
// 归因（desktop 路径 ActorSource=desktop 且 ActorTokenID 不落键）；token_unban
// 无 reason/expiresAt 字段；生效后才写（上例已覆盖失败不写）。
func TestWebTokenService_BanAuditTrail(t *testing.T) {
	svc, _, now := newTestTokenService(t, "")
	audit := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	svc.SetAudit(audit)
	_, rec, err := svc.Create("手机", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	expires := now().Add(time.Hour).Unix()
	if err := svc.Ban(rec.ID, expires, "异常行为", AuditActorRPC, "actor-9"); err != nil {
		t.Fatalf("Ban: %v", err)
	}
	if err := svc.Unban(rec.ID, AuditActorDesktop, ""); err != nil {
		t.Fatalf("Unban: %v", err)
	}

	var banRow, unbanRow *AuditRecord
	for _, line := range readAuditLines(t, audit.path) {
		switch line.Event {
		case AuditEventTokenBan:
			ban := line
			banRow = &ban
		case AuditEventTokenUnban:
			unban := line
			unbanRow = &unban
		}
	}
	if banRow == nil || unbanRow == nil {
		t.Fatalf("应各有一条 token_ban/token_unban 审计行, got ban=%v unban=%v", banRow, unbanRow)
	}
	if banRow.TokenID != rec.ID || banRow.Name != "手机" {
		t.Errorf("token_ban 目标字段不符: %+v", banRow)
	}
	if banRow.Reason != "异常行为" || banRow.ExpiresAt != expires {
		t.Errorf("token_ban 封禁参数不符: %+v", banRow)
	}
	if banRow.ActorSource != AuditActorRPC || banRow.ActorTokenID != "actor-9" {
		t.Errorf("token_ban 操作者归因不符: %+v", banRow)
	}
	if unbanRow.TokenID != rec.ID || unbanRow.Name != "手机" {
		t.Errorf("token_unban 目标字段不符: %+v", unbanRow)
	}
	if unbanRow.ActorSource != AuditActorDesktop {
		t.Errorf("token_unban ActorSource 应为 desktop, got %q", unbanRow.ActorSource)
	}
	if unbanRow.Reason != "" || unbanRow.ExpiresAt != 0 {
		t.Errorf("token_unban 不应携带 reason/expiresAt: %+v", unbanRow)
	}
}

// TestWebTokenService_BanConcurrent 封禁状态机并发安全（-race）：并发封禁/解封/
// 拒绝消息查询/撤销交错下无数据竞态，最终态一致（同一令牌至多一条封禁记录）。
func TestWebTokenService_BanConcurrent(t *testing.T) {
	svc, _, now := newTestTokenService(t, "")
	_, rec, err := svc.Create("手机", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				switch (i + j) % 4 {
				case 0:
					_ = svc.Ban(rec.ID, now().Add(time.Hour).Unix(), "c", AuditActorDesktop, "")
				case 1:
					_ = svc.Unban(rec.ID, AuditActorDesktop, "")
				case 2:
					_ = svc.BanRejectMessage(rec.ID)
				case 3:
					_ = len(svc.ListBans())
				}
			}
		}(i)
	}
	wg.Wait()
	// 最终态一致：至多一条记录，且 List 与 RejectMessage 判定同源
	bans := svc.ListBans()
	if len(bans) > 1 {
		t.Errorf("并发封禁后记录应恒唯一, got %d", len(bans))
	}
	if len(bans) == 1 && svc.BanRejectMessage(rec.ID) == "" {
		t.Error("封禁记录存在且未过期时消息不应为空")
	}
}
