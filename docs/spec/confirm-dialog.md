# 确认型弹框消费契约（ElMessageBox.confirm）

> 沉淀来源：任务 10-20-ep-2-14-5-e2e（EP 2.14.5 升级确认型弹框行为全仓回归排查，2026-10-20）。
> 逐处核对表见 `.trellis/tasks/10-20-ep-2-14-5-e2e/research/ep-confirm-consumers.md`（15 处 A 类 + B 类 E2E 断言面）。

## 1. 背景定性

`element-plus` 声明为 `^2.13.7`（caret 范围），lock 实际安装随依赖更新漂移（41c4b1a 后为 2.14.5）。**message-box 行为差异点（2.13.7 → 2.14）**：

| 差异点 | 2.13.7 行为 | 2.14+ 行为 |
|---|---|---|
| 关闭动画期间重开 | 第二个 confirm 异常（不可见/异常 reject） | 正常显示，链路可重复 |
| 关闭后实例 | 移除 | **驻留 DOM**（`display:none` 不移除） |

实例驻留导致：同链路弹第二次 confirm 时，裸 `.el-message-box` 选择器命中多元素，Playwright strict mode 失败。这是 E2E 断言面唯一受升级影响的点——组件代码本身无需改动的语义未变。

**排查方法论（EP 再升级回归可复用）**：组件权威行为以代码分支为准；vitest 面 ElMessageBox 被 mock 屏蔽（断言与代码一致即无漂移）；真受影响面 = E2E 可达的「同链路可能弹第二次 confirm」消费点 + 裸选择器断言。

## 2. 三形态分类学

排查全仓 24 组件 48 处 `ElMessageBox.confirm` 后归纳的消费链三形态。**EP 升级仅前两形态有暴露面**：

| 形态 | 结构 | E2E 选择器要求 | 已守护先例 |
|---|---|---|---|
| **递归恒弹** | catch 判确认型错误码 → confirm → 确认后重调 → 仍失败仍是确认型 → 再弹（组件内闭环，非 handleError 分流） | 必须 `.first()`/`.last()`（DOM 追加序 = 弹出序） | AssetExportDialog.runExport（`E_ASSETS_EXPORT_TARGET_EXISTS`，assets-export.spec.js） |
| **顺序两段 confirm** | 第一段关闭后立即弹第二段（背靠背，如 Obsidian 注册链「自动注册 → 信任提示」、删除会话「取消在途 → 删除确认」、清理升级确认） | 同上，或 `:visible` + `filter({ hasText })` | assets-backup-rules.spec.js（`:visible`+filter）；Obsidian 三处 vitest times(2) 守护 |
| **单次确认** | 确认后动作单次执行，失败走 handleError，无同链路第二框 | 裸 `.el-message-box` 安全（单实例） | 大多数删除/终止/重链确认（15 处 A 类中 10 处） |

判别方法：读组件代码，看 confirm 之后的动作路径是否存在「再到达 confirm 调用点」的可能（递归重调 or 顺序第二段）。**不能凭「在 catch 里」判定为递归形态**——实测多处理由是状态驱动确认或探测失败豁免分支，confirm 本身不在错误路径上（如 LocalChanges 推送无上游确认）。

## 3. vitest mock 面

- **ElMessageBox mock 是各 spec 文件级 `vi.mock('element-plus', ...)` 工厂**，`src/test/setup.js` 只全局 mock Wails 绑定、**不 mock element-plus**。
- 效果：EP 运行时 confirm 被完整替换为 `vi.fn()`，EP 版本升级行为漂移被 mock 屏蔽——vitest 断言以组件代码分支为准，不受 EP 版本影响。
- 两段链守护范式（对齐 AiChatPanel.spec.js 删除会话用例）：
  - 确认到底：confirm `mockResolvedValueOnce` 两次 → 断言 `toHaveBeenCalledTimes(2)` + 两段入参语义（title/confirmButtonText/distinguishCancelAndClose/dangerouslyUseHTMLString 等）+ 终态调用
  - 第二段取消：首段 resolve 次段 `mockRejectedValueOnce('cancel')` → 断言终态调用不发生 + **静默语义**（第二段 cancel 被内层 catch 吞掉后不得冒泡到外层 cancel 分支——断言外层分支副作用方法 not-called，如复制路径/打开仓库管理器）
- 同构组件（如 Obsidian 三处 `handleOpenObsidian`）**无共享单点，须各补守护**——一处独立改坏他处不知；三处断言面须拉齐（任一处缺入参断言即守护缺口）。

## 4. E2E 断言规则

1. 递归/顺序两段链：`.first()` 取首框、`.last()` 取新框（DOM 追加序 = 弹出序）；或 `:visible` + `filter({ hasText: ... })` 按内容过滤。
2. 单次链：裸选择器即可，勿过度适配。
3. 新增用例先判形态再选范式；「恒拒 override」mock 形态（描述符 `__error__`/`__code__`）是驱动确认型错误链的常规构造。
4. Wails 方法调用参数断言照常（`toHaveBeenNthCalledWith` 驱动 overwrite/force 等重调参数演进断言）。

## 5. 关联

- 前端依赖分档钳制与升级 SOP（EP 升级必走 3.4 专项，防 caret 漂移再发）：[frontend-dep-upgrade.md](frontend-dep-upgrade.md)
- 错误码分流契约（确认型码 = WARNING_CODES 集合）：[logging-and-errors.md](logging-and-errors.md)
- 素材域确认型消费点上下文（重链/导出/空目录删除）：[assets-management.md](assets-management.md)
- E2E 通用契约：[e2e-testing.md](e2e-testing.md)
