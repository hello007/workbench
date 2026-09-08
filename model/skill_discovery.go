package model

// SkillDescriptor 已发现的 skill 元信息，供配置对话框「导入 skill」入口展示与回填。
//
// 扫描三类来源（见 service.SkillDiscoveryService）：
//   - user：用户级 ~/.claude/skills/<name>，command 为 /<name>，cwd 为空（任意目录生效）
//   - project：工作目录 .claude/skills/<name>，command 为 /<name>，cwd 为该工作目录
//   - plugin：已安装插件 installPath/skills/<name>，command 为 /<plugin>:<name>，
//     cwd 按 scope：user 为空，project/local 为绑定的 projectPath
//
// 导入只回填初值，cwd 为空时用户在表单补填（前端必填校验拦截空值）。
type SkillDescriptor struct {
	Name        string `json:"name"`        // SKILL.md frontmatter name
	Description string `json:"description"` // frontmatter description，无此字段则为空
	Command     string `json:"command"`     // 拼接好的斜杠命令：用户级/项目级 /<name>，插件 /<plugin>:<name>
	Cwd         string `json:"cwd"`         // 触发用工作目录；用户级与 scope=user 插件为空，由用户在表单补填
	Source      string `json:"source"`      // user / project / plugin
	SourceDir   string `json:"sourceDir"`   // SKILL.md 所在目录绝对路径
	Plugin      string `json:"plugin"`      // 插件来源时为插件名（installed_plugins.json key 中 @ 前部分），否则空
}
