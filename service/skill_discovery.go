package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"workbench/model"
	"workbench/util"

	"gopkg.in/yaml.v3"
)

// skillFrontmatter SKILL.md frontmatter 解析目标结构，只取 name + description 两字段。
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// installedPluginEntry installed_plugins.json 中单个插件的单条安装记录。
// installed_plugins.json 的 key 格式为 "<plugin>@<marketplace>"，值为安装记录数组。
type installedPluginEntry struct {
	Scope       string `json:"scope"`       // user / project / local
	ProjectPath string `json:"projectPath"` // scope=project/local 时的绑定工作目录
	InstallPath string `json:"installPath"` // 插件实际安装路径（含 skills/ 子目录）
}

// pluginInstall 聚合后的单个插件安装实例：installPath 唯一对应一个 plugin 名与建议 cwd。
// 同一 installPath 可能被多个 entry 引用（如同一插件在不同工作目录 local 安装），
// 聚合时 cwd 优先取 project/local 的 projectPath，否则空（scope=user）。
type pluginInstall struct {
	plugin string
	cwd    string
}

// SkillDiscoveryService skill 自动发现服务。
//
// 扫描用户级 ~/.claude/skills + 所有已管理工作目录的 .claude/skills + 已安装插件的
// installPath/skills，解析每个 SKILL.md 的 frontmatter，去重后返回 SkillDescriptor 列表，
// 供配置对话框「导入 skill」入口回填 command/cwd/description/name。
//
// 缓存：按各源目录 mtime 摘要 fingerprint 比对，一致返回缓存，不一致重扫；不落盘
// （skill 发现只读无副作用，重启重扫成本可接受）。前端「刷新」按钮调 Refresh 强制重扫。
type SkillDiscoveryService struct {
	directorySvc *DirectoryService
	mu           sync.Mutex
	cache        []*model.SkillDescriptor
	fingerprint  string
}

// NewSkillDiscoveryService 创建 skill 发现服务，复用 DirectoryService 获取工作目录列表。
func NewSkillDiscoveryService(directorySvc *DirectoryService) *SkillDiscoveryService {
	return &SkillDiscoveryService{directorySvc: directorySvc}
}

// GetCached 返回已发现的 skill 列表（带 fingerprint 缓存）。
// fingerprint 一致则返回缓存，不一致则重扫并更新缓存。空结果也缓存（避免反复重扫）。
func (s *SkillDiscoveryService) GetCached() []*model.SkillDescriptor {
	s.mu.Lock()
	defer s.mu.Unlock()
	fp := s.computeFingerprint()
	if s.cache != nil && s.fingerprint == fp {
		return s.cache
	}
	s.cache = s.discover()
	s.fingerprint = fp
	return s.cache
}

// Refresh 强制重扫并返回最新列表（清除缓存后走 GetCached 重扫）。
func (s *SkillDiscoveryService) Refresh() []*model.SkillDescriptor {
	s.mu.Lock()
	s.cache = nil
	s.fingerprint = ""
	s.mu.Unlock()
	return s.GetCached()
}

// discover 执行全量扫描：用户级 + 工作目录项目级 + 已安装插件。
// 去重键 source + ":" + sourceDir，同路径不重复；同名不同来源都保留（source 不同）。
func (s *SkillDiscoveryService) discover() []*model.SkillDescriptor {
	seen := make(map[string]*model.SkillDescriptor)
	result := make([]*model.SkillDescriptor, 0)

	home, _ := os.UserHomeDir()

	// 1. 用户级 ~/.claude/skills/<name>/SKILL.md，cwd 空
	if home != "" {
		scanSkillsDir(filepath.Join(home, ".claude", "skills"), "user", "", seen, &result)
	}

	// 2. 工作目录 <dir>/.claude/skills/<name>/SKILL.md，cwd=工作目录
	dirs, _ := s.directorySvc.Load()
	for _, d := range dirs {
		if d.Path == "" {
			continue
		}
		scanSkillsDir(filepath.Join(d.Path, ".claude", "skills"), "project", d.Path, seen, &result)
	}

	// 3. 插件：读 installed_plugins.json，聚合每个 installPath 的 plugin 名与建议 cwd，扫描其 skills
	if home != "" {
		pluginsFile := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
		if util.FileExists(pluginsFile) {
			// installed_plugins.json 真实结构：{ "version": 2, "plugins": { "<plugin>@<marketplace>": [...] } }
			var pluginsDoc struct {
				Version int                               `json:"version"`
				Plugins map[string][]installedPluginEntry `json:"plugins"`
			}
			if err := util.LoadJSON(pluginsFile, &pluginsDoc); err == nil {
				installs := aggregatePluginInstalls(pluginsDoc.Plugins)
				for installPath, pi := range installs {
					scanSkillsDir(filepath.Join(installPath, "skills"), "plugin", pi.cwd, seen, &result, pi.plugin)
				}
			} else {
				println("skill discovery: parse installed_plugins.json failed:", err.Error())
			}
		}
	}

	return result
}

// aggregatePluginInstalls 将 installed_plugins.json 按 installPath 聚合为 pluginInstall。
// 一个 installPath 唯一对应一个 plugin 名（路径含 plugin 名）；cwd 优先取 project/local 的 projectPath。
func aggregatePluginInstalls(plugins map[string][]installedPluginEntry) map[string]*pluginInstall {
	installs := make(map[string]*pluginInstall)
	for key, entries := range plugins {
		pluginName := key
		if i := strings.Index(key, "@"); i > 0 {
			pluginName = key[:i]
		}
		for _, e := range entries {
			if e.InstallPath == "" {
				continue
			}
			pi, ok := installs[e.InstallPath]
			if !ok {
				pi = &pluginInstall{plugin: pluginName}
				installs[e.InstallPath] = pi
			}
			// cwd 优先取 project/local 的 projectPath（绑定工作目录）；scope=user 保持空
			if (e.Scope == "project" || e.Scope == "local") && e.ProjectPath != "" {
				pi.cwd = e.ProjectPath
			}
		}
	}
	return installs
}

// scanSkillsDir 扫描单个 skills 根目录下的一级子目录，每个含 SKILL.md 的子目录解析 frontmatter 后加入结果。
//
// source: user/project/plugin；cwd: 建议 cwd（用户级空、项目级工作目录、插件按 scope）；
// pluginName: 插件来源时为插件名，否则空（用于 command 命名空间拼接 /<plugin>:<name>）。
// 去重键 source + ":" + sourceDir；frontmatter 解析失败或 name 为空跳过并日志，不阻断发现。
func scanSkillsDir(skillsDir, source, cwd string, seen map[string]*model.SkillDescriptor, result *[]*model.SkillDescriptor, pluginName ...string) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return
	}
	plugin := ""
	if len(pluginName) > 0 {
		plugin = pluginName[0]
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillMd := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		if !util.FileExists(skillMd) {
			continue
		}
		name, desc, ok := parseSkillFrontmatter(skillMd)
		if !ok {
			println("skill discovery: skip, frontmatter parse failed:", skillMd)
			continue
		}
		sourceDir := filepath.Join(skillsDir, e.Name())
		dedupKey := source + ":" + sourceDir
		if _, exists := seen[dedupKey]; exists {
			continue
		}
		// command 拼接：用户级/项目级 /<name>，插件 /<plugin>:<name>
		command := "/" + name
		if source == "plugin" && plugin != "" {
			command = "/" + plugin + ":" + name
		}
		sd := &model.SkillDescriptor{
			Name:        name,
			Description: desc,
			Command:     command,
			Cwd:         cwd,
			Source:      source,
			SourceDir:   sourceDir,
			Plugin:      plugin,
		}
		seen[dedupKey] = sd
		*result = append(*result, sd)
	}
}

// parseSkillFrontmatter 解析 SKILL.md 的 YAML frontmatter（首尾独占一行的 --- 包裹），取 name + description。
// 无 YAML 头、未找到结束标记、解析失败、name 为空均返回 ok=false。
func parseSkillFrontmatter(path string) (name, description string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return
	}
	// 找结束标记（独占一行的 ---）
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return
	}
	yamlContent := strings.Join(lines[1:endIdx], "\n")
	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return
	}
	name = strings.TrimSpace(fm.Name)
	if name == "" {
		return
	}
	return name, strings.TrimSpace(fm.Description), true
}

// computeFingerprint 计算各源 mtime 摘要，用于缓存比对。
// 输入：用户级 skills 目录 mtime + installed_plugins.json mtime + 各工作目录 .claude/skills mtime（按 Path 排序）。
// 任一变化则 fingerprint 变化，触发重扫。SKILL.md 内容改不影响目录 mtime，由前端「刷新」按钮兜底。
func (s *SkillDiscoveryService) computeFingerprint() string {
	var sb strings.Builder
	home, _ := os.UserHomeDir()
	if home != "" {
		sb.WriteString(modTimeStr(filepath.Join(home, ".claude", "skills")))
		sb.WriteString("|")
		sb.WriteString(modTimeStr(filepath.Join(home, ".claude", "plugins", "installed_plugins.json")))
		sb.WriteString("|")
	}
	dirs, _ := s.directorySvc.Load()
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Path < dirs[j].Path })
	for _, d := range dirs {
		if d.Path == "" {
			continue
		}
		sb.WriteString(d.Path)
		sb.WriteString(":")
		sb.WriteString(modTimeStr(filepath.Join(d.Path, ".claude", "skills")))
		sb.WriteString(";")
	}
	return sb.String()
}

// modTimeStr 返回路径 mtime 的格式化字符串，路径不存在返回 "0"。
func modTimeStr(p string) string {
	info, err := os.Stat(p)
	if err != nil {
		return "0"
	}
	return info.ModTime().Format("20060102150405.000000000")
}
