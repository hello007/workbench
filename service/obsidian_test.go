package service

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveObsidianVault_Directory 文件夹→自身
func TestResolveObsidianVault_Directory(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveObsidianVault(dir)
	if err != nil {
		t.Fatalf("resolveObsidianVault(dir) 出错: %v", err)
	}
	if got != dir {
		t.Errorf("文件夹应返回自身: 期望 %q, 实际 %q", dir, got)
	}
}

// TestResolveObsidianVault_File 文件→父目录
func TestResolveObsidianVault_File(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	got, err := resolveObsidianVault(file)
	if err != nil {
		t.Fatalf("resolveObsidianVault(file) 出错: %v", err)
	}
	if got != dir {
		t.Errorf("文件应返回父目录: 期望 %q, 实际 %q", dir, got)
	}
}

// TestResolveObsidianVault_NotFound 不存在路径→错误
func TestResolveObsidianVault_NotFound(t *testing.T) {
	_, err := resolveObsidianVault(filepath.Join(os.TempDir(), "definitely-not-exist-12345"))
	if err == nil {
		t.Fatal("不存在路径应返回错误")
	}
}

// TestEncodeObsidianPath 空格/中文/反斜杠编码
func TestEncodeObsidianPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // 关键断言片段
	}{
		{"反斜杠转正斜杠后编码", `C:\Users\test`, "C%3A%2FUsers%2Ftest"},
		{"空格编码为%20", `C:\my notes`, "my%20notes"},
		{"中文安全编码", `C:\Users\张三\笔记`, "%E5%BC%A0%E4%B8%89"},
		{"不含加号", `C:\a b\c`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := encodeObsidianPath(c.in)
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Errorf("encodeObsidianPath(%q) = %q, 应包含 %q", c.in, got, c.want)
			}
			// 编码结果不应出现原始空格或反斜杠
			if strings.ContainsAny(got, " \\") {
				t.Errorf("编码结果 %q 不应含空格或反斜杠", got)
			}
			// 空格应编码为 %20 而非 +
			if strings.Contains(got, "+") {
				t.Errorf("空格应编码为 %%20 而非 +: %q", got)
			}
		})
	}
}

// TestOpenInObsidian_NotFoundPath 路径不存在→友好错误（不启动外部进程）
func TestOpenInObsidian_NotFoundPath(t *testing.T) {
	svc := NewFileOperationService()
	err := svc.OpenInObsidian(filepath.Join(os.TempDir(), "definitely-not-exist-67890"), "")
	if err == nil {
		t.Fatal("不存在路径应返回错误")
	}
}

// TestIsAncestorOrEqual 完整路径段匹配、大小写不敏感、归一化。
func TestIsAncestorOrEqual(t *testing.T) {
	cases := []struct {
		name   string
		parent string
		child  string
		want   bool
	}{
		{"相等", `C:\Vault`, `C:\Vault`, true},
		{"祖先是 vault 根", `C:\Vault`, `C:\Vault\note.md`, true},
		{"非完整段不匹配", `C:\Vault`, `C:\VaultChild\note.md`, false},
		{"大小写不敏感", `c:\vault`, `C:\VAULT\note.md`, true},
		{"正斜杠输入", `C:/Vault`, `C:/Vault/note.md`, true},
		{"不同盘符不匹配", `C:\Vault`, `D:\Vault\note.md`, false},
		{"末尾分隔符归一", `C:\Vault\`, `C:\Vault`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isAncestorOrEqual(c.parent, c.child); got != c.want {
				t.Errorf("isAncestorOrEqual(%q, %q) = %v, 期望 %v", c.parent, c.child, got, c.want)
			}
		})
	}
}

// TestFindVaultForPath_DeepestMatch 多个 vault 包含目标时取嵌套最深者。
func TestFindVaultForPath_DeepestMatch(t *testing.T) {
	// 用真实临时目录，确保 EvalSymlinks 可解析
	root := t.TempDir()
	vaultOuter := filepath.Join(root, "Vault")
	vaultInner := filepath.Join(root, "Vault", "Sub")
	note := filepath.Join(vaultInner, "note.md")
	if err := os.MkdirAll(vaultInner, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(note, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	vaults := map[string]VaultEntry{
		"outer": {Path: vaultOuter},
		"inner": {Path: vaultInner},
	}
	got, ok := findVaultForPath(vaults, note)
	if !ok {
		t.Fatal("期望命中 vault，实际未命中")
	}
	if want := resolvePath(vaultInner); got != want {
		t.Errorf("期望最具体匹配 inner vault %q, 实际 %q", want, got)
	}
}

// TestFindVaultForPath_NoMatch 目标不属于任何已注册 vault 时返回 ok=false。
func TestFindVaultForPath_NoMatch(t *testing.T) {
	vaults := map[string]VaultEntry{
		"v1": {Path: `C:\Vault`},
	}
	_, ok := findVaultForPath(vaults, `D:\Other\note.md`)
	if ok {
		t.Fatal("不匹配任何 vault 时应返回 ok=false")
	}
}

// TestLoadVaultsFromFile_Normal 正常 JSON 解析（未知顶层字段忽略）。
func TestLoadVaultsFromFile_Normal(t *testing.T) {
	content := `{"vaults":{"abc":{"path":"C:\\Vault","ts":1700000000000,"open":true}},"updateDisabled":true}`
	f := filepath.Join(t.TempDir(), "obsidian.json")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vaults, err := loadVaultsFromFile(f)
	if err != nil {
		t.Fatalf("loadVaultsFromFile 出错: %v", err)
	}
	if len(vaults) != 1 {
		t.Fatalf("期望 1 个 vault, 实际 %d", len(vaults))
	}
	v, ok := vaults["abc"]
	if !ok {
		t.Fatal("缺少 vault abc")
	}
	if v.Path != `C:\Vault` {
		t.Errorf("path 期望 C:\\Vault, 实际 %q", v.Path)
	}
	if !v.Open {
		t.Errorf("open 期望 true, 实际 false")
	}
}

// TestLoadVaultsFromFile_NotExist 文件不存在返回错误。
func TestLoadVaultsFromFile_NotExist(t *testing.T) {
	_, err := loadVaultsFromFile(filepath.Join(t.TempDir(), "no-such.json"))
	if err == nil {
		t.Fatal("文件不存在应返回错误")
	}
}

// TestLoadVaultsFromFile_EmptyPath 空路径（非 Windows / APPDATA 缺失）返回错误。
func TestLoadVaultsFromFile_EmptyPath(t *testing.T) {
	_, err := loadVaultsFromFile("")
	if err == nil {
		t.Fatal("空路径应返回错误")
	}
}

// TestLoadVaultsFromFile_UnknownFields 未知字段应被忽略，不报错。
func TestLoadVaultsFromFile_UnknownFields(t *testing.T) {
	content := `{"vaults":{"abc":{"path":"C:\\Vault","ts":1,"unknown":"x"}},"foo":"bar"}`
	f := filepath.Join(t.TempDir(), "obsidian.json")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vaults, err := loadVaultsFromFile(f)
	if err != nil {
		t.Fatalf("未知字段不应导致解析失败: %v", err)
	}
	if _, ok := vaults["abc"]; !ok {
		t.Fatal("应解析出 vault abc")
	}
}

// TestLoadVaultsFromFile_NilVaults vaults 字段缺失时返回空表而非 nil。
func TestLoadVaultsFromFile_NilVaults(t *testing.T) {
	content := `{"updateDisabled":true}`
	f := filepath.Join(t.TempDir(), "obsidian.json")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	vaults, err := loadVaultsFromFile(f)
	if err != nil {
		t.Fatalf("vaults 缺失不应报错: %v", err)
	}
	if vaults == nil {
		t.Fatal("vaults 缺失应返回空表而非 nil")
	}
	if len(vaults) != 0 {
		t.Errorf("期望空表, 实际 %d 项", len(vaults))
	}
}

// TestLoadVaultsFromFile_InvalidJSON 非法 JSON 返回错误。
func TestLoadVaultsFromFile_InvalidJSON(t *testing.T) {
	f := filepath.Join(t.TempDir(), "obsidian.json")
	if err := os.WriteFile(f, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := loadVaultsFromFile(f)
	if err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
}

// TestNewVaultID_Format 生成的 vault ID 为 16 位小写 hex。
func TestNewVaultID_Format(t *testing.T) {
	id := newVaultID(map[string]VaultEntry{})
	if len(id) != 16 {
		t.Fatalf("vault ID 长度应为 16, 实际 %d (%q)", len(id), id)
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Errorf("vault ID %q 应为合法 hex: %v", id, err)
	}
	if id != strings.ToLower(id) {
		t.Errorf("vault ID %q 应为小写 hex", id)
	}
}

// TestNewVaultID_NoConflict 不与现有 ID 冲突。
func TestNewVaultID_NoConflict(t *testing.T) {
	existing := map[string]VaultEntry{
		"439a9f093c243976": {Path: "C:\\Vault1"},
		"52bb7de88c00ed4f": {Path: "C:\\Vault2"},
	}
	for i := 0; i < 20; i++ {
		id := newVaultID(existing)
		if _, dup := existing[id]; dup {
			t.Fatalf("生成的 ID %q 与现有 ID 冲突", id)
		}
	}
}

// TestLoadFullConfig_Normal 正常解析且保留未知顶层字段。
func TestLoadFullConfig_Normal(t *testing.T) {
	content := `{"vaults":{"abc":{"path":"C:\\Vault","ts":1700000000000,"open":true}},"updateDisabled":true}`
	f := filepath.Join(t.TempDir(), "obsidian.json")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFullConfig(f)
	if err != nil {
		t.Fatalf("loadFullConfig 出错: %v", err)
	}
	if len(cfg.Vaults) != 1 {
		t.Fatalf("期望 1 个 vault, 实际 %d", len(cfg.Vaults))
	}
	if cfg.Vaults["abc"].Path != `C:\Vault` {
		t.Errorf("path 期望 C:\\Vault, 实际 %q", cfg.Vaults["abc"].Path)
	}
	// 未知顶层字段应保留在 Rest
	if _, ok := cfg.Rest["updateDisabled"]; !ok {
		t.Fatal("未知顶层字段 updateDisabled 应保留在 Rest")
	}
}

// TestLoadFullConfig_NotExist 文件不存在返回错误。
func TestLoadFullConfig_NotExist(t *testing.T) {
	_, err := loadFullConfig(filepath.Join(t.TempDir(), "no-such.json"))
	if err == nil {
		t.Fatal("文件不存在应返回错误")
	}
}

// TestLoadFullConfig_EmptyPath 空路径返回错误。
func TestLoadFullConfig_EmptyPath(t *testing.T) {
	_, err := loadFullConfig("")
	if err == nil {
		t.Fatal("空路径应返回错误")
	}
}

// TestLoadFullConfig_InvalidJSON 非法 JSON 返回错误。
func TestLoadFullConfig_InvalidJSON(t *testing.T) {
	f := filepath.Join(t.TempDir(), "obsidian.json")
	if err := os.WriteFile(f, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := loadFullConfig(f)
	if err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
}

// TestLoadFullConfig_NilVaults vaults 缺失时返回空表而非 nil。
func TestLoadFullConfig_NilVaults(t *testing.T) {
	content := `{"updateDisabled":true}`
	f := filepath.Join(t.TempDir(), "obsidian.json")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFullConfig(f)
	if err != nil {
		t.Fatalf("vaults 缺失不应报错: %v", err)
	}
	if cfg.Vaults == nil {
		t.Fatal("vaults 缺失应返回空表而非 nil")
	}
	if len(cfg.Vaults) != 0 {
		t.Errorf("期望空表, 实际 %d 项", len(cfg.Vaults))
	}
}

// TestLoadFullConfig_PreserveUnknownFields 未知顶层字段在回写后仍保留。
func TestLoadFullConfig_PreserveUnknownFields(t *testing.T) {
	content := `{"vaults":{"abc":{"path":"C:\\Vault","ts":1}},"updateDisabled":true,"foo":42}`
	f := filepath.Join(t.TempDir(), "obsidian.json")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFullConfig(f)
	if err != nil {
		t.Fatalf("loadFullConfig 出错: %v", err)
	}
	// 追加新 vault 后回写
	cfg.Vaults["newid"] = VaultEntry{Path: `D:\NewVault`, Ts: 2}
	if err := atomicWriteConfig(f, cfg); err != nil {
		t.Fatalf("atomicWriteConfig 出错: %v", err)
	}
	// 重新读取，验证未知字段保留
	cfg2, err := loadFullConfig(f)
	if err != nil {
		t.Fatalf("回写后重新读取出错: %v", err)
	}
	if _, ok := cfg2.Rest["updateDisabled"]; !ok {
		t.Error("回写后 updateDisabled 应保留")
	}
	if _, ok := cfg2.Rest["foo"]; !ok {
		t.Error("回写后 foo 应保留")
	}
	if len(cfg2.Vaults) != 2 {
		t.Errorf("回写后应有 2 个 vault, 实际 %d", len(cfg2.Vaults))
	}
	if _, ok := cfg2.Vaults["newid"]; !ok {
		t.Error("回写后新 vault 应存在")
	}
}

// TestAtomicWriteConfig_ContentCorrect 写入后文件内容正确且可解析。
func TestAtomicWriteConfig_ContentCorrect(t *testing.T) {
	f := filepath.Join(t.TempDir(), "obsidian.json")
	origContent := `{"vaults":{"abc":{"path":"C:\\Vault","ts":1}},"updateDisabled":true}`
	if err := os.WriteFile(f, []byte(origContent), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFullConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Vaults["deadbeefdeadbeef"] = VaultEntry{Path: `D:\New`, Ts: 99}
	if err := atomicWriteConfig(f, cfg); err != nil {
		t.Fatalf("atomicWriteConfig 出错: %v", err)
	}
	// 读回验证
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	var check map[string]json.RawMessage
	if err := json.Unmarshal(b, &check); err != nil {
		t.Fatalf("写入内容应为合法 JSON: %v", err)
	}
	if _, ok := check["updateDisabled"]; !ok {
		t.Error("写入后应保留 updateDisabled")
	}
	if _, ok := check["vaults"]; !ok {
		t.Error("写入后应包含 vaults")
	}
}

// TestAtomicWriteConfig_TmpCleanup 原子写后临时文件已清理。
func TestAtomicWriteConfig_TmpCleanup(t *testing.T) {
	f := filepath.Join(t.TempDir(), "obsidian.json")
	if err := os.WriteFile(f, []byte(`{"vaults":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFullConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteConfig(f, cfg); err != nil {
		t.Fatalf("atomicWriteConfig 出错: %v", err)
	}
	// 临时文件应已被 rename 移走，不存在
	matches, _ := filepath.Glob(f + ".tmp.*")
	if len(matches) != 0 {
		t.Errorf("原子写后不应残留临时文件, 实际 %v", matches)
	}
}

// TestBackupConfig_BackupCreated 备份文件生成且内容与原文件一致。
func TestBackupConfig_BackupCreated(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "obsidian.json")
	origContent := `{"vaults":{"abc":{"path":"C:\\Vault","ts":1}},"updateDisabled":true}`
	if err := os.WriteFile(f, []byte(origContent), 0644); err != nil {
		t.Fatal(err)
	}
	bak, err := backupConfig(f)
	if err != nil {
		t.Fatalf("backupConfig 出错: %v", err)
	}
	if bak == "" {
		t.Fatal("备份路径不应为空")
	}
	if _, err := os.Stat(bak); err != nil {
		t.Fatalf("备份文件应存在: %v", err)
	}
	bakContent, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("读取备份文件失败: %v", err)
	}
	if string(bakContent) != origContent {
		t.Errorf("备份内容应与原文件一致, 原始 %q, 备份 %q", origContent, string(bakContent))
	}
}

// TestBackupConfig_NotExist 原文件不存在时备份返回错误。
func TestBackupConfig_NotExist(t *testing.T) {
	_, err := backupConfig(filepath.Join(t.TempDir(), "no-such.json"))
	if err == nil {
		t.Fatal("原文件不存在应返回错误")
	}
}

// TestIsObsidianRunning_NoPanic 进程检测不 panic（依赖系统状态，不强断言结果）。
func TestIsObsidianRunning_NoPanic(t *testing.T) {
	// 仅验证调用不 panic；实际结果依赖本机是否运行 Obsidian，不强断言。
	_ = isObsidianRunning()
}
