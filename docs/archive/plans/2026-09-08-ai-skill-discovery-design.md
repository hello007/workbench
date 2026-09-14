# Skill 自动发现与一键导入设计

**日期**：2026-09-08
**优先级**：P1
**状态**：待评审

## 1. 概述

新增 skill 自动发现能力：扫描 WorkBench 已管理工作目录与用户级目录下的 `.claude/skills/`，解析 `SKILL.md` frontmatter，在配置对话框内提供「从已发现 skill 导入」入口，自动填充 `command` / `cwd`，消除手记命名空间与目录的负担。

## 2. 背景与痛点

新增功能项时（`AiFunctionConfigDialog.vue` 第 39-43 行），`command`（斜杠命令，含插件命名空间如 `/ab-office:agree-slides`）与 `cwd`（skill 所在项目根）需手填：

| 痛点 | 说明 |
| --- | --- |
| 命名空间靠记忆 | 插件 skill 命令是 `/<plugin>:<skill>`，用户级是 `/<skill>`，混淆即触发失败 |
| 目录靠记忆 | skill 所在项目根决定项目级 skill 发现与 env 生效，填错静默失败 |
| 散落不可见 | skills 分布在多个项目 `.claude/skills/` 与 `~/.claude/skills/`，无统一视图 |
| 重复配置 | 同一 skill 在多个项目重复发现，无去重 |

设计文档 [[2026-09-08-ai-skill-menu-design]] 第 4.4 节原计划「skills 全迁用户级」已作废（插件 skill 不可迁移），散落现状需用发现机制兜底。

## 3. 需求总结

1. 扫描用户级 `~/.claude/skills/` 与所有已管理工作目录的 `.claude/skills/`
2. 解析每个 skill 的 `SKILL.md` frontmatter，提取 `name` / `description`
3. 配置对话框新增「导入 skill」入口，列表展示已发现 skill
4. 选中后自动填充 `command` / `cwd` / `description` / `name`，用户微调其余字段后保存
5. 同名 skill 多来源时标注来源（用户级 / 项目级 / 插件），去重展示

## 4. 设计

### 4.1 发现范围与来源分类

| 来源 | 扫描路径 | command 拼接 |
| --- | --- | --- |
| 用户级 | `~/.claude/skills/<name>/SKILL.md` | `/<name>` |
| 项目级 | `<工作目录>/.claude/skills/<name>/SKILL.md` | `/<name>`，`cwd` = 工作目录 |
| 插件 | `<工作目录>/.claude/plugins/<plugin>/skills/<name>/SKILL.md` | `/<plugin>:<name>`，`cwd` = 工作目录 |

> **实施前需确认**：插件 skill 的实际目录结构（marketplace 机制）。提交 `af79a07` 提到「bmad、OpenSpec 相关 skills 移到根目录按需加载」，实施前需抓样确认插件 skill 路径与命名空间拼接规则。

### 4.2 数据模型

`model/skill_discovery.go` 新增：

```go
// SkillDescriptor 已发现的 skill 元信息
type SkillDescriptor struct {
    Name        string `json:"name"`        // SKILL.md frontmatter name
    Description string `json:"description"` // frontmatter description
    Command     string `json:"command"`     // 拼接好的斜杠命令
    Cwd         string `json:"cwd"`         // 触发用工作目录（用户级为空）
    Source      string `json:"source"`      // user / project / plugin
    SourceDir   string `json:"sourceDir"`   // SKILL.md 所在目录绝对路径
    Plugin      string `json:"plugin"`      // 插件来源时为插件名，否则空
}
```

### 4.3 发现服务

`service/skill_discovery.go` 新增 `SkillDiscoveryService`：

- 输入：工作目录列表（复用 `DirectoryService.Load()`）
- 遍历每个工作目录的 `.claude/skills/` 与 `.claude/plugins/*/skills/`
- 加上用户级 `~/.claude/skills/`
- 每个含 `SKILL.md` 的子目录解析 frontmatter（YAML 头部）
- 去重键：`source + sourceDir`（同路径不重复）；同名不同来源都保留，标注来源区分

frontmatter 解析复用现有 YAML 依赖（若无，引入 `gopkg.in/yaml.v3`，go.mod 已有间接依赖可确认）。

### 4.4 前端导入入口

`AiFunctionConfigDialog.vue` 基础字段区顶部新增「导入 skill」按钮：

1. 点击调 `GetDiscoveredSkills()` 拉取列表
2. 弹 `el-dialog` 展示表格：名称 | 描述 | 来源 | 命令 | 目录
3. 支持名称/描述模糊搜索过滤
4. 选中一行后关闭对话框，将 `command` / `cwd` / `description` / `name`（作为功能项 name 初值）写入 `editing`
5. 用户继续微调 `id`、`completion`、`params` 等后保存

> 已在编辑的功能项也可点「导入」覆盖 command/cwd，便于修正填错的配置。

### 4.5 缓存与刷新

发现结果按工作目录 mtime 缓存（复用 `GetRepoFilterList` 的 mtime 缓存模式），二次打开近乎瞬时。导入对话框内提供「刷新」按钮强制重扫。

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `model/skill_discovery.go` | 新增 | `SkillDescriptor` |
| `service/skill_discovery.go` | 新增 | `SkillDiscoveryService`，扫描 + frontmatter 解析 + 去重 |
| `app.go` | 新增绑定 | `GetDiscoveredSkills()` 方法 |
| `frontend/src/components/AiFunctionConfigDialog.vue` | 修改 | 顶部「导入 skill」按钮 + 导入对话框 |
| `frontend/src/components/__tests__/AiFunctionConfigDialog.spec.js` | 修改 | 导入流程、字段回填测试 |
| `frontend/wailsjs/` | 同步 | `App.js` / `App.d.ts` 增 `GetDiscoveredSkills` 绑定 |

## 6. 验收标准

1. 导入对话框列出用户级 + 所有工作目录的 skill，含名称/描述/来源/命令/目录
2. 选中后 `command` / `cwd` / `description` 正确回填
3. 插件 skill 命名空间拼接正确（待 4.1 确认后验证）
4. 同名多来源 skill 均展示，来源标注区分
5. 模糊搜索过滤生效
6. mtime 缓存生效，二次打开瞬时

## 7. 待确认点

1. 插件 skill 目录结构与命名空间规则（4.1 节），实施前抓样确认
2. 是否扫描工作目录的 `.claude/skills/` 子目录嵌套（如按主题分子目录）？Claude Code skill 发现是否支持嵌套？建议先按一级子目录实现，与 CLI 发现规则对齐
3. frontmatter 解析失败（无 YAML 头或格式错误）的 skill 如何处理？建议跳过并在日志记录，不阻断发现

## 8. 关联

- 来源：[[2026-09-08-ai-skill-menu-design]] 第 4.4 节 skills 收拢作废后的替代方案
- 配套：[[2026-09-08-ai-config-form-design]]（导入后用表单补全 params 等）
