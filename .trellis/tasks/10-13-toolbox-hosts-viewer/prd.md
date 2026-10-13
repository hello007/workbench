# 工具箱续作第三期：hosts 查看器（只读）

## Goal

工具箱新增「hosts 文件」只读查看器，解析展示本机 hosts（Windows `C:\Windows\System32\drivers\etc\hosts` / Linux `/etc/hosts`）行结构（IP / 主机名 / 行内注释），支持关键字过滤、注释显示切换、复制单行/复制 IP。提升本机域名解析排障能力（本地调试域名劫持、屏蔽项排查等高频场景）。沿用工具箱 tools 数组 + 独立 Dialog 组件模式。

## What I already know

- 先例任务：`10-12-toolbox-dev-utilities`（tools 数组 + 独立 Dialog 接入）、`10-13-toolbox-env-viewer`（EnvVarGroup.Error 单来源失败形态、util 平台分叉 `_windows`/`_other` + 无 tag 共享读逻辑）
- 平台分叉先例成熟：`util/env_windows.go` / `util/env_other.go`（路径常量平台化 + 读/解析逻辑无 tag 共享）
- 工具箱接入：`frontend/src/components/ToolboxPanel.vue` `tools` 数组 + `openXxx` handler + 独立 Dialog（append-to-body）
- wailsjs 三处同步 + 具名 string 类型 MISSING_EXPORT 坑已沉淀 spec
- 零新增依赖可行：纯文件读取 + 字符串解析

## Requirements

- 工具箱 `tools` 数组新增「hosts 文件」项，独立 Dialog 组件
- 后端读取本机 hosts：路径常量 `_windows`/`_other` 双文件平台分叉，读文件与解析逻辑无 tag 共享
- 解析行结构：IP（首个 token，含 IPv6）/ 主机名列表 / 行内注释（`#` 之后）；空行与纯注释行过滤，「显示注释」开关可切回展示
- 关键字过滤：匹配 IP 或主机名（大小写不敏感子串包含）
- 复制单行（原始行文本）/ 复制 IP
- 打开时读取 + 手动刷新按钮；不做自动轮询
- 文件不存在或无读权限：组级错误提示（单来源失败形态对齐 10-13 EnvVarGroup.Error 模式——单组数据带 Error 字段，非整体 exception）
- 纯只读：不做编辑/新增/删除（提权写链路留后续任务）

## Assumptions（实现取默认，未逐项确认）

- 无法解析的非空非注释行（如格式残缺）不静默丢弃：以灰色「未识别行」形态展示原始文本，避免用户以为行被吞
- hosts 文件按 UTF-8 读取（容忍 BOM）；注释乱码不做转码兜底
- IPv6 行主机名数可变（如含 scope 的 link-local），解析按「首个 token 为 IP、其余为主机名」通用规则，不硬编码 token 数
- 过滤作用于解析后条目；「显示注释」开启时纯注释行同样参与关键字过滤（匹配原文）

## Acceptance Criteria

- [ ] 解析正确：fixture 注入（含 IPv4/IPv6/行内注释/纯注释/空行/未识别行），禁依赖本机真实 hosts 内容
- [ ] 平台路径分叉：`_windows`/`_other` 双文件，读/解析逻辑无 tag 共享可注入路径测试
- [ ] 文件不存在/无权限：组级 Error 返回，前端组级 warning 行；文件存在但为空不报错
- [ ] 过滤（IP/主机名）、注释开关、复制单行/复制 IP 可用
- [ ] 打开时读取 + 手动刷新；无自动轮询
- [ ] wailsjs 三处同步（App.js/App.d.ts/models.ts）；新增具名 string 类型须手动补 models.ts 别名（MISSING_EXPORT 坑）
- [ ] 测试平台断言拆 `_windows_test`/`_linux_test`（如涉平台耦合断言），fixture 注入无环境外部状态依赖
- [ ] 覆盖率门禁不回退（util ≥40% + service ≥76% + 前端 ≥70%）

## Definition of Done

- 测试全绿 + 覆盖率门禁达标
- docs/功能说明.md、README.md 更新
- CLAUDE.md 关键规则表按需追加
- 新 spec 沉淀到 docs/spec/（如适用）

## Out of Scope

- hosts 编辑/新增/删除（提权链路）
- hosts 文件选择/自定义路径
- 多 hosts 合并视图（如 WSL、docker desktop 生成的片段文件）
- 自动轮询刷新

## Technical Notes

- 候选结构：`util/hosts.go`（解析逻辑 + 类型，无 tag）+ `util/hosts_windows.go`/`util/hosts_other.go`（路径常量）→ `service/hosts.go` 编排 → `app_hosts.go` 绑定 → `model/hosts.go` DTO
- 前端：`HostsDialog.vue` + ToolboxPanel 接入；组级 warning 行、弹窗尺寸分档（数据表类中小弹窗 720×480 档参考）、图标规则按 frontend-visual-conventions
- 先例：`.trellis/tasks/archive/2026-10/10-13-toolbox-env-viewer`、`10-12-toolbox-dev-utilities`
- spec：docs/spec/linux-platform.md、cross-layer-contracts.md、frontend-visual-conventions.md、test-stability.md、test-coverage-gate.md、process-management.md（工具箱接入模式）
