# 规范沉淀（docs/spec/）

> 本目录承接原 `.trellis/spec/` 的项目规范沉淀职能（该目录已于 2026-09-07 废弃移除）。

## 体系用途

存放本项目的**详细规范文档**：跨层契约、接口约定、签名同步规则等需要完整上下文（触发条件、签名对照、错误矩阵、正反示例）才能表达的内容。

## 两级分流规则

| 内容级别 | 沉淀位置 | 判断标准 |
|---|---|---|
| 详细契约 / 主题文档 | `docs/spec/<topic>.md` | 需要多节结构（触发条件、契约、测试要求等）才能说清 |
| 一两句级关键规则 | 项目根 `CLAUDE.md`「规范沉淀规则 → 关键规则」清单 | 每个 session 必知、一两句话可完整表达，附指向本目录详细文档的链接 |

执行 trellis 工作流 Phase 3.3（`trellis-update-spec`）时按上述规则分流写入，**禁止写入 `.trellis/spec/`**（该目录已废弃）。

## 文件索引

|文档|说明|
|---|---|
|[cross-layer-contracts.md](cross-layer-contracts.md)|Wails 绑定同步契约：App 方法签名变更须手动同步 `frontend/wailsjs/` 三处（App.js / App.d.ts / models.ts）；文件预览/保存的编码契约（UTF-8 / GBK）|
|[test-coverage-gate.md](test-coverage-gate.md)|测试覆盖率分层门禁：后端 model/server ≥80% + service ≥76% 基线 + util ≥40% 排除 pty_windows.go + 主包不设门禁；前端 ≥70% 硬失败 exclude wailsjs；CI 脚本 scripts/coverage-check.sh 契约|

## 新增文档约定

- 命名：`<topic>.md`，topic 用英文小写连字符（如 `cross-layer-contracts.md`）
- 每个文档聚焦一个主题，标题下加一段摘要说明适用范围
- 新增后须同步更新本 README 的文件索引表，并在 `CLAUDE.md`「关键规则」清单补充对应的一句话入口（如有必要）

---

**迁移记录：** 2026-09-07 自 `.trellis/spec/backend/cross-layer-contracts.md` 经 `git mv` 迁入（保留 git 历史）；原 `.trellis/spec/` 下空模板与通用 guides 一并删除。

**最后更新：** 2026-09-10
