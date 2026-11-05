package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/util"
)

// writeSkill 写一个含 frontmatter 的 SKILL.md 到 dir/SKILL.md。
func writeSkill(t *testing.T, dir, frontmatter string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(frontmatter), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func TestParseSkillFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "SKILL.md")

	// 正常 frontmatter，description 带引号
	os.WriteFile(p, []byte("---\nname: drawio\ndescription: \"画图工具\"\n---\n# 正文\n"), 0644)
	name, desc, ok := parseSkillFrontmatter(p)
	if !ok || name != "drawio" || desc != "画图工具" {
		t.Fatalf("got name=%q desc=%q ok=%v, want drawio/画图工具/true", name, desc, ok)
	}

	// 无 frontmatter（首行非 ---）
	os.WriteFile(p, []byte("# 标题\n内容\n"), 0644)
	if _, _, ok := parseSkillFrontmatter(p); ok {
		t.Fatal("无 frontmatter 应返回 ok=false")
	}

	// name 为空
	os.WriteFile(p, []byte("---\ndescription: \"无 name\"\n---\n"), 0644)
	if _, _, ok := parseSkillFrontmatter(p); ok {
		t.Fatal("name 为空应返回 ok=false")
	}

	// 无结束标记
	os.WriteFile(p, []byte("---\nname: foo\ndescription: \"无结束\"\n"), 0644)
	if _, _, ok := parseSkillFrontmatter(p); ok {
		t.Fatal("无结束标记应返回 ok=false")
	}

	// 文件不存在
	if _, _, ok := parseSkillFrontmatter(filepath.Join(tmp, "nope.md")); ok {
		t.Fatal("文件不存在应返回 ok=false")
	}
}

func TestScanSkillsDir(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "skills")
	writeSkill(t, filepath.Join(skillsDir, "alpha"), "---\nname: alpha\ndescription: \"a\"\n---\n")
	writeSkill(t, filepath.Join(skillsDir, "beta"), "---\nname: beta\ndescription: \"b\"\n---\n")
	// 无 frontmatter 的应跳过
	writeSkill(t, filepath.Join(skillsDir, "bad"), "# 无 frontmatter\n")
	// 无 SKILL.md 的子目录应跳过
	os.MkdirAll(filepath.Join(skillsDir, "nodir"), 0755)

	seen := make(map[string]*model.SkillDescriptor)
	result := make([]*model.SkillDescriptor, 0)
	scanSkillsDir(skillsDir, "project", "/work", seen, &result)

	if len(result) != 2 {
		t.Fatalf("got %d skills, want 2（bad 与 nodir 应跳过）", len(result))
	}
	// 命名空间：项目级 /<name>，cwd=工作目录
	cmds := map[string]string{}
	for _, s := range result {
		if s.Source != "project" || s.Cwd != "/work" {
			t.Fatalf("source/cwd mismatch: %+v", s)
		}
		cmds[s.Name] = s.Command
	}
	if cmds["alpha"] != "/alpha" || cmds["beta"] != "/beta" {
		t.Fatalf("command mismatch: %v", cmds)
	}
}

func TestScanSkillsDir_PluginNamespace(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "skills")
	writeSkill(t, filepath.Join(skillsDir, "agree-slides"), "---\nname: agree-slides\ndescription: \"幻灯片\"\n---\n")

	seen := make(map[string]*model.SkillDescriptor)
	result := make([]*model.SkillDescriptor, 0)
	scanSkillsDir(skillsDir, "plugin", "/proj", seen, &result, "ab-office")

	if len(result) != 1 {
		t.Fatalf("got %d, want 1", len(result))
	}
	s := result[0]
	if s.Command != "/ab-office:agree-slides" {
		t.Fatalf("command=%q, want /ab-office:agree-slides", s.Command)
	}
	if s.Source != "plugin" || s.Plugin != "ab-office" || s.Cwd != "/proj" {
		t.Fatalf("descriptor mismatch: %+v", s)
	}
}

func TestScanSkillsDir_Dedup(t *testing.T) {
	tmp := t.TempDir()
	skillsDir := filepath.Join(tmp, "skills")
	writeSkill(t, filepath.Join(skillsDir, "dup"), "---\nname: dup\ndescription: \"d\"\n---\n")

	seen := make(map[string]*model.SkillDescriptor)
	result := make([]*model.SkillDescriptor, 0)
	scanSkillsDir(skillsDir, "user", "", seen, &result)
	scanSkillsDir(skillsDir, "user", "", seen, &result) // 同 source+sourceDir，应去重

	if len(result) != 1 {
		t.Fatalf("got %d, want 1（同路径应去重）", len(result))
	}
}

func TestAggregatePluginInstalls(t *testing.T) {
	plugins := map[string][]installedPluginEntry{
		"ab-office@ab-internal-plugins": {
			{Scope: "project", ProjectPath: "D:/proj", InstallPath: "C:/cache/ab-office/0.3.0"},
		},
		"superpowers@superpowers-marketplace": {
			{Scope: "user", InstallPath: "C:/cache/superpowers/6.1.0"},
		},
		// 同一插件多个 local 安装，同 installPath，cwd 应取 project/local 的 projectPath
		"understand-anything@understand-anything": {
			{Scope: "local", ProjectPath: "D:/a", InstallPath: "C:/cache/ua/2.7.5"},
			{Scope: "local", ProjectPath: "D:/b", InstallPath: "C:/cache/ua/2.7.5"},
		},
	}
	installs := aggregatePluginInstalls(plugins)
	if len(installs) != 3 {
		t.Fatalf("got %d installs, want 3", len(installs))
	}
	if installs["C:/cache/ab-office/0.3.0"].plugin != "ab-office" {
		t.Fatal("ab-office plugin 名提取错误")
	}
	if installs["C:/cache/ab-office/0.3.0"].cwd != "D:/proj" {
		t.Fatalf("ab-office cwd=%q, want D:/proj", installs["C:/cache/ab-office/0.3.0"].cwd)
	}
	if installs["C:/cache/superpowers/6.1.0"].cwd != "" {
		t.Fatal("scope=user cwd 应为空")
	}
	// 同 installPath 多 local，cwd 取最后一个 project/local 的 projectPath（聚合覆盖）
	if installs["C:/cache/ua/2.7.5"].cwd != "D:/b" {
		t.Fatalf("ua cwd=%q, want D:/b", installs["C:/cache/ua/2.7.5"].cwd)
	}
}

// TestSkillDiscovery_Discover 三源集成：用户级 + 工作目录项目级 + 插件。
// 用 t.Setenv 隔离 os.UserHomeDir()，避免串扰真实用户目录。
func TestSkillDiscovery_Discover(t *testing.T) {
	tmp := t.TempDir()
	userHome := filepath.Join(tmp, "home")
	t.Setenv("USERPROFILE", userHome) // Windows os.UserHomeDir() 读 USERPROFILE
	t.Setenv("HOME", userHome)        // Linux/Mac（CI）读 HOME

	// 用户级 ~/.claude/skills/drawio
	writeSkill(t, filepath.Join(userHome, ".claude", "skills", "drawio"),
		"---\nname: drawio\ndescription: \"画图\"\n---\n")

	// 工作目录项目级 .claude/skills/release
	workDir := filepath.Join(tmp, "work")
	writeSkill(t, filepath.Join(workDir, ".claude", "skills", "release"),
		"---\nname: release\ndescription: \"发版\"\n---\n")

	// 插件 installPath/skills/agree-slides
	pluginInstall := filepath.Join(tmp, "plugin")
	writeSkill(t, filepath.Join(pluginInstall, "skills", "agree-slides"),
		"---\nname: agree-slides\ndescription: \"幻灯片\"\n---\n")
	// installed_plugins.json（真实结构：{ version, plugins: { "<plugin>@<marketplace>": [...] } }）
	pluginsFile := filepath.Join(userHome, ".claude", "plugins", "installed_plugins.json")
	util.SaveJSON(pluginsFile, struct {
		Version int                               `json:"version"`
		Plugins map[string][]installedPluginEntry `json:"plugins"`
	}{
		Version: 2,
		Plugins: map[string][]installedPluginEntry{
			"ab-office@ab-internal-plugins": {
				{Scope: "project", ProjectPath: workDir, InstallPath: pluginInstall},
			},
		},
	})

	// DirectoryService 装配工作目录
	dirSvc := NewDirectoryService(filepath.Join(tmp, "directories.json"))
	dirSvc.Save([]*model.Directory{{ID: "d1", Name: "work", Path: workDir}})

	svc := NewSkillDiscoveryService(dirSvc)
	result := svc.discover()

	byCmd := map[string]*model.SkillDescriptor{}
	for _, s := range result {
		byCmd[s.Command] = s
	}
	cases := []struct {
		cmd, source, cwd, plugin string
	}{
		{"/drawio", "user", "", ""},
		{"/release", "project", workDir, ""},
		{"/ab-office:agree-slides", "plugin", workDir, "ab-office"},
	}
	for _, c := range cases {
		s, ok := byCmd[c.cmd]
		if !ok {
			t.Fatalf("未发现 %s（result=%+v）", c.cmd, result)
		}
		if s.Source != c.source || s.Cwd != c.cwd || s.Plugin != c.plugin {
			t.Fatalf("%s 字段不符: source=%q cwd=%q plugin=%q, want %q/%q/%q",
				c.cmd, s.Source, s.Cwd, s.Plugin, c.source, c.cwd, c.plugin)
		}
	}
}

// TestSkillDiscovery_CacheAndRefresh 缓存命中返回同结果，Refresh 强制重扫。
func TestSkillDiscovery_CacheAndRefresh(t *testing.T) {
	tmp := t.TempDir()
	userHome := filepath.Join(tmp, "home")
	t.Setenv("USERPROFILE", userHome) // Windows
	t.Setenv("HOME", userHome)        // Linux/Mac（CI）
	writeSkill(t, filepath.Join(userHome, ".claude", "skills", "alpha"),
		"---\nname: alpha\ndescription: \"a\"\n---\n")

	dirSvc := NewDirectoryService(filepath.Join(tmp, "directories.json"))
	svc := NewSkillDiscoveryService(dirSvc)

	first := svc.GetCached()
	if len(first) != 1 || first[0].Command != "/alpha" {
		t.Fatalf("首次 GetCached 结果异常: %+v", first)
	}
	// 二次 GetCached：fingerprint 一致，返回缓存（同切片长度与内容）
	second := svc.GetCached()
	if len(second) != len(first) || second[0].Command != first[0].Command {
		t.Fatalf("二次 GetCached 应返回缓存，got %+v", second)
	}
	// Refresh 强制重扫
	third := svc.Refresh()
	if len(third) != 1 || third[0].Command != "/alpha" {
		t.Fatalf("Refresh 结果异常: %+v", third)
	}
}

// TestSkillDiscovery_ChatDirsScanned 对话目录（不在工作目录列表）的 .claude/skills
// 被发现：source=project、cwd=对话目录、command=/<name>；provider 经 setter 注入。
func TestSkillDiscovery_ChatDirsScanned(t *testing.T) {
	tmp := t.TempDir()
	userHome := filepath.Join(tmp, "home")
	t.Setenv("USERPROFILE", userHome) // Windows
	t.Setenv("HOME", userHome)        // Linux/Mac（CI）
	writeSkill(t, filepath.Join(userHome, ".claude", "skills", "user-skill"),
		"---\nname: user-skill\ndescription: \"u\"\n---\n")

	// 对话目录（不在工作目录列表）带一个项目级 skill
	chatDir := filepath.Join(tmp, "chat-proj")
	writeSkill(t, filepath.Join(chatDir, ".claude", "skills", "chat-skill"),
		"---\nname: chat-skill\ndescription: \"c\"\n---\n")

	dirSvc := NewDirectoryService(filepath.Join(tmp, "directories.json"))
	svc := NewSkillDiscoveryService(dirSvc)
	svc.SetChatDirsProvider(func() []string { return []string{chatDir} })

	byCmd := map[string]*model.SkillDescriptor{}
	for _, s := range svc.discover() {
		byCmd[s.Command] = s
	}
	s, ok := byCmd["/chat-skill"]
	if !ok {
		t.Fatalf("未发现 /chat-skill（对话目录应被扫描）: %+v", byCmd)
	}
	if s.Source != "project" || s.Cwd != chatDir {
		t.Fatalf("chat-skill 字段不符: source=%q cwd=%q, want project/%s", s.Source, s.Cwd, chatDir)
	}
	if _, ok := byCmd["/user-skill"]; !ok {
		t.Fatal("用户级 skill 不应受影响")
	}
}

// TestSkillDiscovery_ChatDirDedupWithWorkDir 同一路径既是工作目录又是对话目录只扫一次；
// Windows 大小写 / \\ vs / 分隔符变体也只一条（normalizeDirKey 规范化兜底）。
func TestSkillDiscovery_ChatDirDedupWithWorkDir(t *testing.T) {
	tmp := t.TempDir()
	userHome := filepath.Join(tmp, "home")
	t.Setenv("USERPROFILE", userHome)
	t.Setenv("HOME", userHome)

	workDir := filepath.Join(tmp, "both")
	writeSkill(t, filepath.Join(workDir, ".claude", "skills", "rel"),
		"---\nname: rel\ndescription: \"r\"\n---\n")

	dirSvc := NewDirectoryService(filepath.Join(tmp, "directories.json"))
	dirSvc.Save([]*model.Directory{{ID: "d1", Name: "both", Path: workDir}})

	svc := NewSkillDiscoveryService(dirSvc)
	// 变体：小写盘符 + 正斜杠（Windows 路径比较大小写不敏感）
	variant := strings.ToLower(filepath.ToSlash(workDir))
	svc.SetChatDirsProvider(func() []string { return []string{variant} })

	count := 0
	for _, s := range svc.discover() {
		if s.Command == "/rel" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("/rel 出现 %d 次, want 1（重合路径应去重）", count)
	}
}

// TestSkillDiscovery_ChatDirsFingerprint 对话目录纳入指纹：新增带 skill 的对话目录后
// GetCached 触发重扫发现新 skill；provider 返回 nil/空列表时不扫、不出错。
func TestSkillDiscovery_ChatDirsFingerprint(t *testing.T) {
	tmp := t.TempDir()
	userHome := filepath.Join(tmp, "home")
	t.Setenv("USERPROFILE", userHome)
	t.Setenv("HOME", userHome)

	dirSvc := NewDirectoryService(filepath.Join(tmp, "directories.json"))
	chatDir := filepath.Join(tmp, "chat")
	chatPaths := []string{} // 可变清单，模拟对话目录动态增减
	svc := NewSkillDiscoveryService(dirSvc)
	svc.SetChatDirsProvider(func() []string { return chatPaths })

	// 空清单 + 目录下尚无 skill：空结果
	if got := svc.GetCached(); len(got) != 0 {
		t.Fatalf("空清单应返回空列表, got %+v", got)
	}
	// 新增对话目录及其 skill（目录 mtime 变化触发指纹更新）
	writeSkill(t, filepath.Join(chatDir, ".claude", "skills", "later"),
		"---\nname: later\ndescription: \"l\"\n---\n")
	chatPaths = append(chatPaths, chatDir)
	got := svc.GetCached()
	if len(got) != 1 || got[0].Command != "/later" {
		t.Fatalf("新增对话目录后应重扫发现 /later, got %+v", got)
	}
	// provider 返回 nil：不扫、不出错，回退为仅用户级（此处为空）
	svc.SetChatDirsProvider(nil)
	if got := svc.GetCached(); len(got) != 0 {
		t.Fatalf("nil provider 应跳过对话目录, got %+v", got)
	}
}

// TestModTimeStr_NotExists 不存在路径返回 "0"。
func TestModTimeStr_NotExists(t *testing.T) {
	if got := modTimeStr(filepath.Join(t.TempDir(), "missing")); got != "0" {
		t.Errorf("不存在路径应返回 0, got %s", got)
	}
}

// TestModTimeStr_Exists 存在路径返回非零 mtime 格式串。
func TestModTimeStr_Exists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := modTimeStr(p)
	if got == "0" || len(got) == 0 {
		t.Errorf("存在路径应返回时间戳, got %s", got)
	}
}
