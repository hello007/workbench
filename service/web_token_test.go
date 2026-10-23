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

// newTestTokenService 构造测试用 WebTokenService：nowFn 固定时间源（过期判定
// 驱动不依赖真实时钟）、genToken 确定性生成（避免对随机源形态断言）、flushDelay
// 收紧（debounce 落盘可等待）。注意：tokens.json 缺失时服务会引导生成「默认」
// 记录（生产语义：全新安装必有初始 token），断言列表长度时须计入。
func newTestTokenService(t *testing.T, legacyPlain string) (*WebTokenService, string, func() time.Time) {
	t.Helper()
	dir := t.TempDir()
	tokensPath := filepath.Join(dir, "web_tokens.json")
	legacyPath := filepath.Join(dir, "web_token")
	if legacyPlain != "" {
		if err := os.WriteFile(legacyPath, []byte(legacyPlain+"\n"), 0o600); err != nil {
			t.Fatalf("写存量令牌文件: %v", err)
		}
	}
	now := time.Unix(1_700_000_000, 0)
	counter := 0
	svc := NewWebTokenService(tokensPath, legacyPath)
	svc.nowFn = func() time.Time { return now }
	svc.genToken = func() (string, error) {
		counter++
		return strings.Repeat(string(rune('a'+counter-1)), 8) + "-test-token", nil
	}
	svc.flushDelay = 30 * time.Millisecond
	return svc, dir, func() time.Time { return now }
}

// advanceNowFn 推进时间源（惰性过期判定驱动，不依赖 sleep 与真实时钟）。
func advanceNowFn(base func() time.Time, d time.Duration) func() time.Time {
	start := base()
	return func() time.Time { return start.Add(d) }
}

// TestWebTokenService_MigrateLegacy 存量单 token 迁移：data/web_token 明文存在
// 时首载建首条记录（名称「默认」，哈希为明文 SHA-256），旧文件保留不删（回滚
// 安全），Verify 对存量明文命中。
func TestWebTokenService_MigrateLegacy(t *testing.T) {
	svc, dir, _ := newTestTokenService(t, "legacy-plain-token")
	svc.EnsureLoaded()

	tokens := svc.List()
	if len(tokens) != 1 {
		t.Fatalf("迁移后应恰好一条记录, got %d", len(tokens))
	}
	rec := tokens[0]
	if rec.Name != DefaultWebTokenName {
		t.Errorf("迁移记录名应为「默认」, got %q", rec.Name)
	}
	if rec.Hash != hashToken("legacy-plain-token") {
		t.Errorf("迁移记录哈希应为明文 SHA-256, got %q", rec.Hash)
	}
	if rec.ID == "" || rec.CreatedAt == 0 {
		t.Errorf("迁移记录 ID/CreatedAt 应就绪: %+v", rec)
	}
	if id, ok := svc.Verify("legacy-plain-token"); !ok || id != rec.ID {
		t.Errorf("存量明文应认证命中, got id=%q ok=%v", id, ok)
	}
	// 旧文件保留不删（回滚安全）
	if data, err := os.ReadFile(filepath.Join(dir, "web_token")); err != nil {
		t.Errorf("存量文件应保留: %v", err)
	} else if strings.TrimSpace(string(data)) != "legacy-plain-token" {
		t.Errorf("存量文件内容不应被改写, got %q", string(data))
	}
	// 新文件已落盘且不含明文
	raw, err := os.ReadFile(svc.path)
	if err != nil {
		t.Fatalf("读多 token 文件: %v", err)
	}
	if strings.Contains(string(raw), "legacy-plain-token") {
		t.Error("多 token 文件不得包含明文 token")
	}
}

// TestWebTokenService_BootstrapFresh 全新安装引导：两文件均不存在时生成初始
// token，明文同步写存量兼容文件（「查看令牌」链路依赖），记录落多 token 文件。
func TestWebTokenService_BootstrapFresh(t *testing.T) {
	svc, dir, _ := newTestTokenService(t, "")
	svc.EnsureLoaded()

	tokens := svc.List()
	if len(tokens) != 1 {
		t.Fatalf("引导后应恰好一条记录, got %d", len(tokens))
	}
	if tokens[0].Name != DefaultWebTokenName {
		t.Errorf("引导记录名应为「默认」, got %q", tokens[0].Name)
	}
	// 明文兼容文件就绪且内容可认证命中
	data, err := os.ReadFile(filepath.Join(dir, "web_token"))
	if err != nil {
		t.Fatalf("兼容文件应已写入: %v", err)
	}
	plain := strings.TrimSpace(string(data))
	if id, ok := svc.Verify(plain); !ok || id != tokens[0].ID {
		t.Errorf("兼容文件明文应认证命中, got id=%q ok=%v", id, ok)
	}
}

// TestWebTokenService_CreateAndVerify 创建与校验：明文 64 位 hex 风格由真实
// genToken 保证（此处断言注入形态），记录只含哈希、文件不含明文；Verify 命中
// 返回记录 ID，错误候选拒绝。
func TestWebTokenService_CreateAndVerify(t *testing.T) {
	svc, _, _ := newTestTokenService(t, "")
	plain, rec, err := svc.Create("手机", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Name != "手机" || rec.ExpiresAt != 0 || rec.Revoked || rec.LastUsedAt != 0 {
		t.Errorf("新建记录字段不符: %+v", rec)
	}
	if rec.Hash == plain || strings.Contains(rec.Hash, plain) {
		t.Errorf("记录不得含明文: hash=%q", rec.Hash)
	}
	if rec.Hash != hashToken(plain) {
		t.Errorf("记录哈希应为明文 SHA-256")
	}
	if id, ok := svc.Verify(plain); !ok || id != rec.ID {
		t.Errorf("Verify 应命中, got id=%q ok=%v", id, ok)
	}
	if _, ok := svc.Verify("wrong-token"); ok {
		t.Error("错误候选不应命中")
	}
	// 文件不含明文
	raw, _ := os.ReadFile(svc.path)
	if strings.Contains(string(raw), plain) {
		t.Error("落盘文件不得包含明文 token")
	}
}

// TestWebTokenService_CreateValidation Create 入参校验：名称空与过期时间过去
// 均以 AppError 错误码拒绝（前端按码分流）。
func TestWebTokenService_CreateValidation(t *testing.T) {
	svc, _, now := newTestTokenService(t, "")

	_, _, err := svc.Create("   ", 0)
	if err == nil {
		t.Fatal("空名称应拒绝")
	}
	appErr, ok := err.(*model.AppError)
	if !ok || appErr.Code != model.ErrCodeTokenNameRequired {
		t.Errorf("空名称应返回 E_TOKEN_NAME_REQUIRED, got %v", err)
	}

	past := now().Add(-time.Hour).Unix()
	_, _, err = svc.Create("x", past)
	if err == nil {
		t.Fatal("过期时间过去应拒绝")
	}
	appErr, ok = err.(*model.AppError)
	if !ok || appErr.Code != model.ErrCodeTokenExpiryInvalid {
		t.Errorf("过去过期应返回 E_TOKEN_EXPIRY_INVALID, got %v", err)
	}

	// 未来时间合法
	if _, _, err := svc.Create("x", now().Add(24*time.Hour).Unix()); err != nil {
		t.Errorf("未来过期时间应合法: %v", err)
	}
}

// TestWebTokenService_Revoke 撤销语义：撤销后认证拒绝；重复撤销幂等 no-op；
// 不存在返回 E_TOKEN_NOT_FOUND；撤销记录保留在列表（审计与展示可见性）。
func TestWebTokenService_Revoke(t *testing.T) {
	svc, _, _ := newTestTokenService(t, "")
	plain, rec, err := svc.Create("pad", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Revoke(rec.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, ok := svc.Verify(plain); ok {
		t.Error("撤销后认证应拒绝")
	}
	if err := svc.Revoke(rec.ID); err != nil {
		t.Errorf("重复撤销应幂等 no-op, got %v", err)
	}
	err = svc.Revoke("no-such-id")
	appErr, ok := err.(*model.AppError)
	if !ok || appErr.Code != model.ErrCodeTokenNotFound {
		t.Errorf("不存在 ID 应返回 E_TOKEN_NOT_FOUND, got %v", err)
	}

	tokens := svc.List()
	if len(tokens) != 2 {
		t.Fatalf("撤销后列表应含引导记录与被撤记录共 2 条, got %d", len(tokens))
	}
	for _, rec := range tokens {
		if rec.Name != "pad" {
			continue
		}
		if !rec.Revoked {
			t.Errorf("被撤记录应标记已撤销: %+v", rec)
		}
	}
}

// TestWebTokenService_ExpiryLazyReject 惰性过期：过期前命中、过期后拒绝且记录
// 不自动删除（保留列表可见性）；时间经 nowFn 注入推进，不依赖 sleep。
func TestWebTokenService_ExpiryLazyReject(t *testing.T) {
	svc, _, now := newTestTokenService(t, "")
	plain, _, err := svc.Create("临时设备", now().Add(time.Hour).Unix())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok := svc.Verify(plain); !ok {
		t.Fatal("过期前应命中")
	}

	// 推进时间源跨过期点
	svc.mu.Lock()
	svc.nowFn = advanceNowFn(now, 2*time.Hour)
	svc.mu.Unlock()
	if _, ok := svc.Verify(plain); ok {
		t.Error("过期后认证应拒绝")
	}
	tokens := svc.List()
	if len(tokens) != 2 {
		t.Fatalf("列表应含引导记录与过期记录共 2 条, got %d", len(tokens))
	}
	for _, rec := range tokens {
		if rec.Name != "临时设备" {
			continue
		}
		if rec.Revoked {
			t.Errorf("过期记录不应标记撤销（惰性失效非撤销）: %+v", rec)
		}
	}
}

// TestWebTokenService_LastUsedDebounceFlush lastUsedAt debounce 落盘：命中即
// 更新内存（最后使用时间 = 命中时刻），窗口内不写盘；flush 后落盘值一致。
// 轮询带截止时间（对齐 test-stability 的等待范式，不 sleep 硬等）。
func TestWebTokenService_LastUsedDebounceFlush(t *testing.T) {
	svc, _, now := newTestTokenService(t, "")
	plain, rec, err := svc.Create("pad", 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	findByID := func(list model.WebTokenList) *model.WebTokenRecord {
		for i := range list.Tokens {
			if list.Tokens[i].ID == rec.ID {
				return &list.Tokens[i]
			}
		}
		return nil
	}

	if _, ok := svc.Verify(plain); !ok {
		t.Fatal("Verify 应命中")
	}
	// 内存态立即更新（列表含引导「默认」记录，按 ID 定位被测记录）
	svc.mu.Lock()
	inMem := findByID(svc.list)
	if inMem == nil {
		svc.mu.Unlock()
		t.Fatal("内存列表应含被测记录")
	}
	if got := inMem.LastUsedAt; got != now().Unix() {
		svc.mu.Unlock()
		t.Fatalf("内存 lastUsedAt 应为命中时刻 %d, got %d", now().Unix(), got)
	}
	// 落盘前磁盘态 lastUsedAt 恒为 0（debounce 未 flush）
	raw, _ := os.ReadFile(svc.path)
	var onDisk model.WebTokenList
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		svc.mu.Unlock()
		t.Fatalf("解析落盘文件: %v", err)
	}
	onDiskRec := findByID(onDisk)
	if onDiskRec == nil || onDiskRec.LastUsedAt != 0 {
		svc.mu.Unlock()
		t.Fatalf("debounce 窗口内磁盘 lastUsedAt 应为 0, got %+v", onDiskRec)
	}
	svc.mu.Unlock()

	// FlushNow 确定性落盘
	svc.FlushNow()
	raw, _ = os.ReadFile(svc.path)
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("解析落盘文件: %v", err)
	}
	onDiskRec = findByID(onDisk)
	if onDiskRec == nil || onDiskRec.LastUsedAt != now().Unix() {
		t.Errorf("flush 后磁盘 lastUsedAt 应为 %d, got %+v", now().Unix(), onDiskRec)
	}
}

// TestWebTokenService_CorruptJSONDegrade 损坏 JSON 降级：加载失败降级空列表
// 不报错（对齐 SessionService 模式），认证 fail closed，后续 Create 可重建。
func TestWebTokenService_CorruptJSONDegrade(t *testing.T) {
	svc, _, _ := newTestTokenService(t, "")
	if err := os.WriteFile(svc.path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("写损坏文件: %v", err)
	}

	tokens := svc.List()
	if len(tokens) != 0 {
		t.Fatalf("损坏文件应降级空列表, got %d", len(tokens))
	}
	if _, ok := svc.Verify("anything"); ok {
		t.Error("空列表应拒绝一切认证")
	}
	plain, rec, err := svc.Create("重建", 0)
	if err != nil {
		t.Fatalf("损坏后 Create 应可重建: %v", err)
	}
	if _, ok := svc.Verify(plain); !ok || rec.Name != "重建" {
		t.Error("重建记录应可命中")
	}
}

// TestWebTokenService_RevokeAllExcept 轮换编排撤旧步骤：除存活 ID 外全部撤销，
// 幸存记录命中、其余拒绝；再次调用幂等返回 0。
func TestWebTokenService_RevokeAllExcept(t *testing.T) {
	svc, _, _ := newTestTokenService(t, "")
	p1, _, _ := svc.Create("a", 0)
	p2, r2, _ := svc.Create("b", 0)
	p3, _, _ := svc.Create("c", 0)

	n, err := svc.RevokeAllExcept(r2.ID)
	if err != nil || n != 3 {
		// 引导「默认」记录 + a + c 一并撤销（轮换撤旧语义：除幸存者外全撤）
		t.Fatalf("应撤销 3 条, got n=%d err=%v", n, err)
	}
	if _, ok := svc.Verify(p2); !ok {
		t.Error("幸存记录应命中")
	}
	if _, ok := svc.Verify(p1); ok {
		t.Error("a 应已撤销")
	}
	if _, ok := svc.Verify(p3); ok {
		t.Error("c 应已撤销")
	}
	if n, err := svc.RevokeAllExcept(r2.ID); err != nil || n != 0 {
		t.Errorf("重复撤旧应幂等返回 0, got n=%d err=%v", n, err)
	}
}

// TestWebTokenService_PersistAcrossInstances 落盘复用：同一文件的新实例加载
// 既有记录（撤销/过期状态随文件迁移），不再走迁移/引导。
func TestWebTokenService_PersistAcrossInstances(t *testing.T) {
	svc, _, _ := newTestTokenService(t, "")
	plain, rec, _ := svc.Create("设备A", 0)
	svc.FlushNow()

	dir := filepath.Dir(svc.path)
	svc2 := NewWebTokenService(svc.path, filepath.Join(dir, "web_token"))
	svc2.nowFn = svc.nowFn
	svc2.genToken = svc.genToken
	tokens := svc2.List()
	found := false
	for _, tk := range tokens {
		if tk.ID == rec.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("新实例应加载既有记录（含引导记录）, got %+v", tokens)
	}
	if _, ok := svc2.Verify(plain); !ok {
		t.Error("新实例 Verify 应命中既有记录")
	}
}

// TestWebTokenService_ConcurrentVerify 并发 Verify 与 Create 竞态安全（-race 面）。
func TestWebTokenService_ConcurrentVerify(t *testing.T) {
	svc, _, _ := newTestTokenService(t, "")
	plain, _, _ := svc.Create("并发", 0)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				svc.Verify(plain)
				if i%2 == 0 {
					svc.List()
				}
			}
		}(i)
	}
	wg.Wait()
}
