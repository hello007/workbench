# 约束索引

按需读取约束，不要一次性加载全部。根据任务类型选择 1-3 个相关文档。

所有约束文档位于 `docs/agent-resources/conventions/` 目录。

> **规则 vs 剧本**：本文只索引**规则**（可判定违规、不含操作步骤）。需要该场景的**分步流程**时看 `docs/agent-resources/index-workflows.md`，两者不重复同一段正文。
> 主题的权威归属以 `docs/agent-resources/README.md` 的「主题 → 权威文件」表为准。

## 编码规范（Coding）

| 文档 | 场景 | 内容 |
|------|------|------|
| `docs/agent-resources/conventions/coding-vue-style.md` | 编写 Vue 3 组件 / 响应式处理 / 样式隔离 / SFC 结构 / 跨组件状态 | Composition API、`<script setup>`、ref/reactive、响应式避坑、样式隔离、Composables、Pinia |
| `docs/agent-resources/conventions/coding-api-contracts.md` | 前后端接口联调 / Wails 绑定契约 / 数据转换 | 绑定方法契约、数据结构同步、字段防御 |
| `docs/agent-resources/conventions/coding-api-integration.md` | 网络层与路由 / transport 双通道收口 | transport 统一收口、桌面/浏览器通道分流、路由懒加载 |
| `docs/agent-resources/conventions/coding-error-handling.md` | 前端错误捕获 / 异常降级 / 提示 | 全局 errorHandler、Promise catch、友好用户提示 |

## 流程规范（Process）— 规则，不是工作流；分步流程见 index-workflows.md

| 文档 | 场景 | 内容 |
|------|------|------|
| `docs/agent-resources/conventions/process-convention-files.md` | 新增 / 修改 / 删除约定文件 | 约定文件格式、frontmatter 规范、变更后同步派生物的要求 |
| `docs/agent-resources/conventions/process-build-quality.md` | 构建质量门禁 / 提交前检查 | 真实门禁清单（CI / vet / 覆盖率 / 依赖分档）、已知缺口（无 lint）现状 |

开发、评审、文档等流程类内容在 `docs/agent-resources/workflows/`，通过 `index-workflows.md` 索引。

## 项目约束（Project）

| 文档 | 场景 | 内容 |
|------|------|------|
| `docs/agent-resources/conventions/project-boundaries.md` | 调整目录分层 / 重构组件 / 新增模块 / 依赖流向 | Go 后端与 Vue 前端分层职责、依赖边界、各层特有约束 |

## 测试规范（Testing）

| 文档 | 场景 | 内容 |
|------|------|------|
| `docs/agent-resources/conventions/testing-strategy.md` | 编写测试 / 组件渲染验证 / 工具函数单测 | 测试分层、Vitest 配置、`@vue/test-utils`、测试数据隔离 |

> 命名规则：**前缀映射范畴**（`coding-` / `process-` / `project-` / `testing-`），后缀仅作描述。新增文件必须带前缀，详见 `docs/agent-resources/README.md`。
