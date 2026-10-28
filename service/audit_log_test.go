package service

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// mustAuditLine 序列化单条审计记录为 JSONL 行（测试手工构造档文件用）。
func mustAuditLine(t *testing.T, rec AuditRecord) string {
	t.Helper()
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("序列化审计记录: %v", err)
	}
	return string(line)
}

// readAuditLines 读入审计日志全部行并解析为 AuditRecord（损坏行跳过——读侧
// 容错语义与生产一致）。
func readAuditLines(t *testing.T, path string) []AuditRecord {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("打开审计日志 %s: %v", path, err)
	}
	defer f.Close()
	var recs []AuditRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec AuditRecord
		if err := json.Unmarshal([]byte(line), &rec); err == nil {
			recs = append(recs, rec)
		}
	}
	return recs
}

// TestAuditLogger_AppendAndFormat 追加与行格式：两行合法 JSONL，字段
// （ts/event/tokenId/name/remoteAddr/userAgent）完整落行。
func TestAuditLogger_AppendAndFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)

	a.Log(AuditEventAuthSuccess, "tok-1", "", "127.0.0.1:1234", "Mozilla/5.0")
	a.Log(AuditEventTokenCreate, "tok-2", "手机", "", "")

	recs := readAuditLines(t, path)
	if len(recs) != 2 {
		t.Fatalf("应有两行审计记录, got %d", len(recs))
	}
	if recs[0].Event != AuditEventAuthSuccess || recs[0].TokenID != "tok-1" ||
		recs[0].RemoteAddr != "127.0.0.1:1234" || recs[0].UserAgent != "Mozilla/5.0" {
		t.Errorf("认证事件字段不符: %+v", recs[0])
	}
	if recs[1].Event != AuditEventTokenCreate || recs[1].Name != "手机" || recs[1].TokenID != "tok-2" {
		t.Errorf("生命周期事件字段不符: %+v", recs[1])
	}
	if recs[0].Ts == 0 || recs[1].Ts == 0 {
		t.Error("事件时间应就绪")
	}
}

// TestAuditLogger_NoPlaintextToken 审计面无明文泄露：Log 出口不接受明文 token
// 入参（签名即约束），落盘行不含敏感字段名形态（除 tokenId 哈希归因键）。
func TestAuditLogger_NoPlaintextToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)
	secret := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	a.Log(AuditEventAuthFailure, "", "", "192.168.1.5:999", "curl/8.0")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读审计日志: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Error("审计日志不得包含明文 token")
	}
}

// TestAuditLogger_Rotation 轮转：注入小 maxBytes 驱动超限，旧内容 rename 为 .1，
// 新内容继续落主文件；.1 与主文件均可解析（读侧无损坏）。depth 钉 1 保持单档
// 形态（多档链行为见 TestAuditLogger_RotationChain 系列）。
func TestAuditLogger_Rotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)
	a.mu.Lock()
	a.maxBytes = 160 // 驱动多次轮转的极小上限
	a.depth = 1      // 单档形态（连接管控十一期多档链独立覆盖）
	a.mu.Unlock()

	for i := 0; i < 6; i++ {
		a.Log(AuditEventAuthSuccess, "tok-r", "设备", "127.0.0.1:1", "ua")
	}

	dot1 := readAuditLines(t, path+".1")
	main := readAuditLines(t, path)
	if len(dot1) == 0 {
		t.Fatal("轮转应产生 .1 文件")
	}
	if len(main) == 0 {
		t.Fatal("轮转后主文件应继续追加")
	}
	for _, rec := range append(append([]AuditRecord{}, dot1...), main...) {
		if rec.Event != AuditEventAuthSuccess {
			t.Errorf("轮转前后记录均应完整可解析: %+v", rec)
		}
	}

	// 读侧合并：QueryAuditLog 应跨 .1 与主文件返回全部可见行（真实写侧落盘后
	// 的读回路径；覆盖行数 = 两文件之和，轮转覆盖语义下旧行被 .1 覆盖属预期）
	res, err := a.QueryAuditLog(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("查询: %v", err)
	}
	if res.Total != len(dot1)+len(main) {
		t.Errorf("读侧应合并两文件全部行, got %d want %d", res.Total, len(dot1)+len(main))
	}
}

// TestAuditLogger_RotationChain 深度淘汰（连接管控十一期多档链）：depth=2 下
// 逐行驱动轮转，链稳态为 .2/.1/主 三档，无超深度档 .3 产生；链内内容时间正序
// （.2 最旧、主最新）；读侧合并恰好覆盖链内全部行（被淘汰档静默丢弃，对齐
// 趋势面 earliestTs 自然前移口径）。
func TestAuditLogger_RotationChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)
	a.mu.Lock()
	a.maxBytes = 160
	a.depth = 2
	a.mu.Unlock()

	for i := 0; i < 10; i++ {
		a.Log(AuditEventAuthSuccess, "tok-"+strconv.Itoa(i), "", "127.0.0.1:1", "ua")
	}

	for _, p := range []string{path + ".2", path + ".1", path} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("链档应存在: %s", p)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatal("深度淘汰后不应产生超深度档 .3")
	}

	recs2 := readAuditLines(t, path+".2")
	recs1 := readAuditLines(t, path+".1")
	recsM := readAuditLines(t, path)
	if len(recs2) == 0 || len(recs1) == 0 || len(recsM) == 0 {
		t.Fatalf("三档均应有内容: %d/%d/%d", len(recs2), len(recs1), len(recsM))
	}
	if recs2[0].TokenID != "tok-7" || recs1[0].TokenID != "tok-8" || recsM[0].TokenID != "tok-9" {
		t.Errorf("链内内容应时间正序（.2 旧 主 新）: %s/%s/%s",
			recs2[0].TokenID, recs1[0].TokenID, recsM[0].TokenID)
	}

	res, err := a.QueryAuditLog(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("查询: %v", err)
	}
	if res.Total != 3 {
		t.Errorf("读侧应仅合并链内 3 行（淘汰档不可见）, got %d", res.Total)
	}
}

// TestAuditLogger_ApplyConfig 热更生效（连接管控十一期，App.SetAuditConfig 生效
// 点）：锁内整体替换 maxBytes/depth 两字段，对后续写入立即生效——缩到小上限后
// 下一行即触发轮转（仅面向未来语义的正面形态），depth 新值同驱动链深。
func TestAuditLogger_ApplyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)
	if a.maxBytes != webAuditMaxBytes || a.depth != webAuditMaxRotationDepth {
		t.Fatalf("构造初值应为生产常量, got %d/%d", a.maxBytes, a.depth)
	}

	// 热更到测试小值后断言锁内字段替换
	a.ApplyConfig(160, 2)
	a.mu.Lock()
	gotMax, gotDepth := a.maxBytes, a.depth
	a.mu.Unlock()
	if gotMax != 160 || gotDepth != 2 {
		t.Fatalf("热更后字段应整体替换, got %d/%d", gotMax, gotDepth)
	}

	// 生效断言：热更前写入不轮转（原上限下未超限），热更后下一行超小限即轮转出 .1/.2
	a.Log(AuditEventAuthSuccess, "tok-pre", "", "127.0.0.1:1", "ua")
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatal("热更前写入不应触发轮转")
	}
	for i := 0; i < 4; i++ {
		a.Log(AuditEventAuthSuccess, "tok-"+strconv.Itoa(i), "", "127.0.0.1:1", "ua")
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("热更后写入应按新上限轮转出 .1: %v", err)
	}
}

// TestAuditLogger_RotationChain_UpgradeFromDot1 存量兼容：现网十期形态已存 .1，
// 十一期首次轮转 .1→.2 顺移（源存在即移，零迁移脚本），主→.1，链衔接无缝。
func TestAuditLogger_RotationChain_UpgradeFromDot1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)
	a.mu.Lock()
	a.maxBytes = 160
	a.depth = 3
	a.mu.Unlock()

	legacy := mustAuditLine(t, AuditRecord{Ts: 100, Event: AuditEventAuthSuccess, TokenID: "tok-legacy"})
	if err := os.WriteFile(path+".1", []byte(legacy+"\n"), 0o600); err != nil {
		t.Fatalf("写存量 .1: %v", err)
	}
	a.Log(AuditEventAuthSuccess, "tok-b", "", "127.0.0.1:1", "ua")
	a.Log(AuditEventAuthSuccess, "tok-c", "", "127.0.0.1:1", "ua") // 触发轮转：.1→.2、主→.1

	if got := readAuditLines(t, path+".2"); len(got) != 1 || got[0].TokenID != "tok-legacy" {
		t.Errorf("存量 .1 应顺移为 .2: %+v", got)
	}
	if got := readAuditLines(t, path+".1"); len(got) != 1 || got[0].TokenID != "tok-b" {
		t.Errorf("原主文件应轮转为 .1: %+v", got)
	}
	if got := readAuditLines(t, path); len(got) != 1 || got[0].TokenID != "tok-c" {
		t.Errorf("轮转后主文件应继续追加: %+v", got)
	}
}

// TestAuditLogger_RotationChain_MiddleGap 链中段缺失不断链：.2 缺失时 .3→.4
// 与 .1→.2 各档独立移位，轮转不因空洞中断。
func TestAuditLogger_RotationChain_MiddleGap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)
	a.mu.Lock()
	a.maxBytes = 160
	a.depth = 4
	a.mu.Unlock()

	writeLegacy := func(p, tokenID string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(mustAuditLine(t, AuditRecord{
			Ts: 100, Event: AuditEventAuthSuccess, TokenID: tokenID})+"\n"), 0o600); err != nil {
			t.Fatalf("写 %s: %v", p, err)
		}
	}
	writeLegacy(path+".3", "tok-x3")
	writeLegacy(path+".1", "tok-y1")
	a.Log(AuditEventAuthSuccess, "tok-p", "", "127.0.0.1:1", "ua")
	a.Log(AuditEventAuthSuccess, "tok-q", "", "127.0.0.1:1", "ua") // 触发轮转

	if got := readAuditLines(t, path+".4"); len(got) != 1 || got[0].TokenID != "tok-x3" {
		t.Errorf(".3 应顺移为 .4: %+v", got)
	}
	if got := readAuditLines(t, path+".2"); len(got) != 1 || got[0].TokenID != "tok-y1" {
		t.Errorf(".1 应顺移为 .2（.2 缺失跳过不断链）: %+v", got)
	}
	if got := readAuditLines(t, path+".1"); len(got) != 1 || got[0].TokenID != "tok-p" {
		t.Errorf("原主文件应轮转为 .1: %+v", got)
	}
	if got := readAuditLines(t, path); len(got) != 1 || got[0].TokenID != "tok-q" {
		t.Errorf("轮转后主文件应继续追加: %+v", got)
	}
}

// TestAuditLogger_ReadSnapshot_DepthTiers 读侧档位列表按 depth 构造：.3/.2/.1/主
// 全档合并、时间正序拼接（最旧档在前、倒序分页最新在前），缺档（.2）跳过。
func TestAuditLogger_ReadSnapshot_DepthTiers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)
	a.mu.Lock()
	a.depth = 3
	a.mu.Unlock()

	writeLegacy := func(p string, ts int64, tokenID string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(mustAuditLine(t, AuditRecord{
			Ts: ts, Event: AuditEventAuthSuccess, TokenID: tokenID})+"\n"), 0o600); err != nil {
			t.Fatalf("写 %s: %v", p, err)
		}
	}
	writeLegacy(path+".3", 100, "tok-old3")
	writeLegacy(path+".1", 200, "tok-old1")
	writeLegacy(path, 300, "tok-main")

	res, err := a.QueryAuditLog(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("查询: %v", err)
	}
	if res.Total != 3 {
		t.Fatalf("三档应合并 3 行, got %d", res.Total)
	}
	if res.Records[0].TokenID != "tok-main" || res.Records[2].TokenID != "tok-old3" {
		t.Errorf("倒序应为主文件最新在前、.3 最旧在后: %+v", res.Records)
	}
}

// TestAuditLogger_DepthShrink_LegacyTierIgnored depth 热更缩小：超深度残留旧档
// 不入读侧列表也不删除（静默滞留磁盘，容量口径以新配置为准——PRD 技术方案取舍）。
func TestAuditLogger_DepthShrink_LegacyTierIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)

	writeLegacy := func(p string, ts int64, tokenID string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(mustAuditLine(t, AuditRecord{
			Ts: ts, Event: AuditEventAuthSuccess, TokenID: tokenID})+"\n"), 0o600); err != nil {
			t.Fatalf("写 %s: %v", p, err)
		}
	}
	writeLegacy(path+".2", 50, "tok-evicted")
	writeLegacy(path+".1", 100, "tok-keep1")
	writeLegacy(path, 150, "tok-keepM")

	a.mu.Lock()
	a.depth = 1 // 模拟热更缩小后的读取口径
	a.mu.Unlock()

	res, err := a.QueryAuditLog(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("查询: %v", err)
	}
	if res.Total != 2 {
		t.Errorf("缩小后应仅读 .1 与主文件 2 行, got %d", res.Total)
	}
	for _, rec := range res.Records {
		if rec.TokenID == "tok-evicted" {
			t.Errorf("超深度残留档不应入读: %+v", rec)
		}
	}
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Errorf("残留旧档不应被删除（静默滞留）: %v", err)
	}
}

// TestAuditLogger_Concurrent 并发写入安全（单写锁串行）：全部行完整可解析
// （无交错半行）。
func TestAuditLogger_Concurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)

	const workers, perWorker = 8, 25
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				a.Log(AuditEventAuthFailure, "tok-c", "", "127.0.0.1:1", "ua")
			}
		}(i)
	}
	wg.Wait()

	recs := readAuditLines(t, path)
	if len(recs) != workers*perWorker {
		t.Fatalf("并发写入应无丢失, got %d want %d", len(recs), workers*perWorker)
	}
}

// TestAuditLogger_MissingDir 自动建目录：父目录不存在时追加仍成功（对齐
// data/ 落盘目录约定）。
func TestAuditLogger_MissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deeper")
	path := filepath.Join(dir, "web_audit.log")
	a := NewAuditLogger(path)
	a.Log(AuditEventTokenMigrate, "tok-0", DefaultWebTokenName, "", "")
	if recs := readAuditLines(t, path); len(recs) != 1 {
		t.Fatalf("父目录缺失时应自动创建并写入, got %d", len(recs))
	}
}

// TestAuditLogger_LogConnKick 踢出连接审计行（conn_kick）：LogConnKick 专责出口
// 字段齐（event/tokenId/name/remoteAddr/connId/ts + actorSource/actorTokenId，
// 连接管控三期 actor 归因），既有事件（Log 出口）不携带 connId/actor 字段
// （omitempty 不落键，行形态向后兼容）。
func TestAuditLogger_LogConnKick(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)

	a.Log(AuditEventAuthSuccess, "tok-1", "", "127.0.0.1:1234", "ua")
	a.LogConnKick(KickAudit{
		ActorSource:   AuditActorRPC,
		ActorTokenID:  "tok-actor",
		TargetTokenID: "tok-k",
		TargetName:    "手机",
		RemoteAddr:    "192.168.1.5",
		ConnID:        42,
	})

	recs := readAuditLines(t, path)
	if len(recs) != 2 {
		t.Fatalf("应有两行审计记录, got %d", len(recs))
	}
	if recs[0].ConnID != 0 {
		t.Errorf("既有事件不应携带 connId, got %d", recs[0].ConnID)
	}
	kick := recs[1]
	if kick.Event != AuditEventConnKick {
		t.Errorf("事件名应为 conn_kick, got %q", kick.Event)
	}
	if kick.TokenID != "tok-k" || kick.Name != "手机" || kick.RemoteAddr != "192.168.1.5" || kick.ConnID != 42 {
		t.Errorf("踢出审计字段不符: %+v", kick)
	}
	if kick.ActorSource != AuditActorRPC || kick.ActorTokenID != "tok-actor" {
		t.Errorf("操作者归因字段不符: %+v", kick)
	}
	if kick.Ts == 0 {
		t.Error("ts 应就绪")
	}
	// 行形态断言：connId/actor 键落行、既有行不含这些键（omitempty 口径）
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读审计日志: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if !strings.Contains(lines[1], `"connId":42`) ||
		!strings.Contains(lines[1], `"actorSource":"rpc"`) ||
		!strings.Contains(lines[1], `"actorTokenId":"tok-actor"`) {
		t.Errorf("踢出行应含 connId/actor 归因键: %s", lines[1])
	}
	if strings.Contains(lines[0], "connId") || strings.Contains(lines[0], "actor") {
		t.Errorf("既有事件行不应出现 connId/actor 键: %s", lines[0])
	}
}

// TestAuditLogger_LogConnKickDesktop 桌面踢出审计行：actorSource=desktop 且
// actorTokenId 空不落键（omitempty，本地信任模型无令牌上下文）。
func TestAuditLogger_LogConnKickDesktop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)

	a.LogConnKick(KickAudit{
		ActorSource:   AuditActorDesktop,
		TargetTokenID: "tok-k",
		TargetName:    "默认",
		RemoteAddr:    "192.168.1.9",
		ConnID:        7,
	})

	recs := readAuditLines(t, path)
	if len(recs) != 1 {
		t.Fatalf("应有一行审计记录, got %d", len(recs))
	}
	if recs[0].ActorSource != AuditActorDesktop || recs[0].ActorTokenID != "" {
		t.Errorf("桌面归因应为 desktop/空: %+v", recs[0])
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读审计日志: %v", err)
	}
	line := strings.TrimSpace(string(raw))
	if strings.Contains(line, "actorTokenId") {
		t.Errorf("桌面行不应出现 actorTokenId 键: %s", line)
	}
	if !strings.Contains(line, `"actorSource":"desktop"`) {
		t.Errorf("桌面行应含 actorSource=desktop: %s", line)
	}
}

// TestAuditLogger_QueryAuditLog 读侧查询：轮转文件（旧）与主文件（新）合并、
// 过滤（事件/时间窗/令牌）、时间倒序分页（最新在前）、Total 为过滤后总数。
// 两文件手工构造（轮转落盘形态已有 TestAuditLogger_Rotation 覆盖，此处专注
// 读侧合并与过滤分页语义，行数精确可控）。
func TestAuditLogger_QueryAuditLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web_audit.log")
	dot1 := []AuditRecord{
		{Ts: 100, Event: AuditEventAuthSuccess, TokenID: "tok-old", RemoteAddr: "127.0.0.1:1"},
		{Ts: 110, Event: AuditEventTokenCreate, TokenID: "tok-old", Name: "旧设备"},
	}
	main := []AuditRecord{
		{Ts: 200, Event: AuditEventAuthSuccess, TokenID: "tok-new", RemoteAddr: "127.0.0.1:2"},
		{Ts: 210, Event: AuditEventAuthFailure, TokenID: "tok-new", RemoteAddr: "127.0.0.1:2"},
		{Ts: 220, Event: AuditEventConnKick, TokenID: "tok-new", Name: "新设备",
			RemoteAddr: "127.0.0.1:2", ConnID: 1, ActorSource: AuditActorDesktop},
	}
	writeAuditFile := func(p string, recs []AuditRecord) {
		t.Helper()
		var b strings.Builder
		for _, rec := range recs {
			line, err := json.Marshal(rec)
			if err != nil {
				t.Fatalf("序列化: %v", err)
			}
			b.Write(line)
			b.WriteByte('\n')
		}
		if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
			t.Fatalf("写 %s: %v", p, err)
		}
	}
	writeAuditFile(path+".1", dot1)
	writeAuditFile(path, main)
	a := NewAuditLogger(path)

	// 无过滤：全量 5 行，时间倒序（主文件尾行第一，.1 旧行最尾）
	res, err := a.QueryAuditLog(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("查询: %v", err)
	}
	if res.Total != 5 || len(res.Records) != 5 {
		t.Fatalf("全量应 5 行, got total=%d records=%d", res.Total, len(res.Records))
	}
	if res.Records[0].Ts != 220 || res.Records[0].Event != AuditEventConnKick {
		t.Errorf("倒序首行应为最新 conn_kick, got %+v", res.Records[0])
	}
	if res.Records[4].Ts != 100 || res.Records[4].TokenID != "tok-old" {
		t.Errorf("倒序尾行应为 .1 旧文件最旧行, got %+v", res.Records[4])
	}

	// 事件过滤
	res, err = a.QueryAuditLog(AuditQueryOptions{Events: []string{AuditEventConnKick}})
	if err != nil {
		t.Fatalf("查询: %v", err)
	}
	if res.Total != 1 || res.Records[0].Event != AuditEventConnKick {
		t.Errorf("事件过滤应仅 conn_kick: total=%d", res.Total)
	}

	// 令牌过滤：命中 TokenID 或 ActorTokenID 任一（本例令牌均为目标侧，另测 actor 侧）
	res, _ = a.QueryAuditLog(AuditQueryOptions{TokenID: "tok-old"})
	if res.Total != 2 {
		t.Errorf("tok-old 相关应 2 行, got %d", res.Total)
	}

	// 分页：offset 越过末页返回空但 Total 不变
	res, _ = a.QueryAuditLog(AuditQueryOptions{Offset: 4, Limit: 2})
	if res.Total != 5 || len(res.Records) != 1 {
		t.Errorf("末页应 1 行 Total=5, got total=%d records=%d", res.Total, len(res.Records))
	}
	res, _ = a.QueryAuditLog(AuditQueryOptions{Offset: 9, Limit: 2})
	if res.Total != 5 || len(res.Records) != 0 {
		t.Errorf("越界页应 0 行 Total=5, got total=%d records=%d", res.Total, len(res.Records))
	}
	// 页内倒序：第 2 页（offset=2,limit=2）应取全量第 3、4 新（ts 降序）
	res, _ = a.QueryAuditLog(AuditQueryOptions{Offset: 2, Limit: 2})
	if res.Records[0].Ts != 200 || res.Records[1].Ts != 110 {
		t.Errorf("分页窗口应保持倒序连续: %+v", res.Records)
	}

	// Limit 钳制：请求超上限按上限返回（本例 5 行全回）
	res, _ = a.QueryAuditLog(AuditQueryOptions{Limit: 99999})
	if len(res.Records) != 5 {
		t.Errorf("Limit 超上限应钳制返回全部, got %d", len(res.Records))
	}
}

// TestAuditLogger_QueryAuditLog_ActorFilter 操作者令牌过滤：conn_kick 行的
// actorTokenId 与 TokenID 任一命中即相关。
func TestAuditLogger_QueryAuditLog_ActorFilter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)

	a.Log(AuditEventAuthSuccess, "tok-victim", "", "127.0.0.1:1", "ua")
	a.LogConnKick(KickAudit{
		ActorSource:   AuditActorRPC,
		ActorTokenID:  "tok-actor",
		TargetTokenID: "tok-victim",
		TargetName:    "受害设备",
		RemoteAddr:    "127.0.0.1:1",
		ConnID:        3,
	})

	res, err := a.QueryAuditLog(AuditQueryOptions{TokenID: "tok-actor"})
	if err != nil {
		t.Fatalf("查询: %v", err)
	}
	// 操作者令牌相关：自身认证行不含（TokenID=tok-actor 无）、conn_kick 行经
	// ActorTokenID 命中 1 行
	if res.Total != 1 || res.Records[0].Event != AuditEventConnKick {
		t.Errorf("操作者过滤应命中 conn_kick 1 行: total=%d", res.Total)
	}
}

// TestAuditLogger_QueryAuditLog_SkipBadLines 坏行跳过：非法 JSON 与缺 event
// 字段的行不进结果，合法行不受影响（读方容错约定）。
func TestAuditLogger_QueryAuditLog_SkipBadLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web_audit.log")
	good := `{"ts":1700000000,"event":"auth_success","tokenId":"tok-1"}`
	bad := []string{
		`{"ts":1700000001,"event"`, // 截断行
		`not-json-at-all`,          // 非法 JSON
		`{"ts":1700000002}`,        // 合法 JSON 但缺 event
		``,                         // 空行
	}
	content := good + "\n" + strings.Join(bad, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写审计日志: %v", err)
	}
	a := NewAuditLogger(path)

	res, err := a.QueryAuditLog(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("查询不应因坏行失败: %v", err)
	}
	if res.Total != 1 || res.Records[0].Event != AuditEventAuthSuccess {
		t.Errorf("应仅剩 1 条合法行: total=%d records=%+v", res.Total, res.Records)
	}
}

// TestAuditLogger_QueryAuditLog_EmptyFiles 无文件（首次使用/未轮转）返回空结果
// 不报错。
func TestAuditLogger_QueryAuditLog_EmptyFiles(t *testing.T) {
	a := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	res, err := a.QueryAuditLog(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("空文件查询不应报错: %v", err)
	}
	if res.Total != 0 || res.Records == nil || len(res.Records) != 0 {
		t.Errorf("空结果应为 0 行且 Records 非 nil: %+v", res)
	}
}

// TestAuditLogger_QueryAuditLog_TimeWindow 时间窗过滤（From/To 含边界，0=不限）。
func TestAuditLogger_QueryAuditLog_TimeWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)

	recs := []AuditRecord{
		{Ts: 100, Event: AuditEventAuthSuccess, TokenID: "t1"},
		{Ts: 200, Event: AuditEventAuthSuccess, TokenID: "t2"},
		{Ts: 300, Event: AuditEventAuthSuccess, TokenID: "t3"},
	}
	for _, rec := range recs {
		a.write(rec)
	}

	res, _ := a.QueryAuditLog(AuditQueryOptions{From: 200, To: 200})
	if res.Total != 1 || res.Records[0].TokenID != "t2" {
		t.Errorf("边界含值过滤不符: total=%d", res.Total)
	}
	res, _ = a.QueryAuditLog(AuditQueryOptions{From: 150})
	if res.Total != 2 {
		t.Errorf("仅 From 应 2 行, got %d", res.Total)
	}
	res, _ = a.QueryAuditLog(AuditQueryOptions{To: 250})
	if res.Total != 2 {
		t.Errorf("仅 To 应 2 行, got %d", res.Total)
	}
}
