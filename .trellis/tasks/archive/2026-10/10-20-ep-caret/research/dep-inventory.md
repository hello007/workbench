# 前端依赖盘点（2026-10-20）

> 盘点脚本：node 解析 frontend/package.json + package-lock.json 比对声明范围 vs lock 实际版本。

## 总览

- 39 个顶层依赖：dependencies 32（含 xlsx URL tarball）+ devDependencies 7
- 声明格式：**全部 caret（^）**，仅 xlsx 为 CDN tarball URL（天然 exact）
- lockfileVersion 3
- **lock 无一越界**：全部落在声明范围内（caret 范围内重解析，npm 设计内行为）
- 漂移根源定性：caret 全覆盖下，任意 `npm install` 依赖操作可将范围内依赖拉到最新 minor，lock 随操作漂移

## 分档定案（39 项）

### exact（2 项）——UI 组件库核心，事故源

| 依赖 | 新声明 | lock 现状 |
|---|---|---|
| element-plus | `2.14.5` | 2.14.5 |
| @element-plus/icons-vue | `2.3.2` | 2.3.2 |

### tilde（28 项）——运行时 UI 面，锁 minor 放 patch

声明值按 lock 现状 minor 重写（原 caret 声明值的 lock 已漂走 minor，`~<原声明值>` 会使 lock 越档）：

| 依赖 | 新声明 | lock 现状 | 原 caret 声明 |
|---|---|---|---|
| @codemirror/commands | ~6.11.0 | 6.11.0 | ^6.10.3 |
| @codemirror/lang-css | ~6.3.1 | 6.3.1 | ^6.3.0 |
| @codemirror/lang-html | ~6.4.12 | 6.4.12 | ^6.4.9 |
| @codemirror/lang-javascript | ~6.2.5 | 6.2.5 | ^6.2.2 |
| @codemirror/lang-json | ~6.0.2 | 6.0.2 | ^6.0.2 |
| @codemirror/lang-markdown | ~6.5.2 | 6.5.2 | ^6.5.0 |
| @codemirror/lang-sql | ~6.10.0 | 6.10.0 | ^6.10.0 |
| @codemirror/language | ~6.12.4 | 6.12.4 | ^6.12.3 |
| @codemirror/state | ~6.7.4 | 6.7.4 | ^6.6.0 |
| @codemirror/view | ~6.43.11 | 6.43.11 | ^6.43.1 |
| codemirror | ~6.0.2 | 6.0.2 | ^6.0.2 |
| dayjs | ~1.11.23 | 1.11.23 | ^1.11.23 |
| docx-preview | ~0.3.7 | 0.3.7 | ^0.3.7 |
| echarts | ~6.1.0 | 6.1.0 | ^6.1.0 |
| highlight.js | ~11.12.0 | 11.12.0 | ^11.11.1 |
| js-yaml | ~4.3.2 | 4.3.2 | ^4.3.0 |
| markdown-it | ~14.3.2 | 14.3.2 | ^14.2.0 |
| mermaid | ~11.17.2 | 11.17.2 | ^11.16.0 |
| pinia | ~4.0.3 | 4.0.3 | ^4.0.3 |
| splitpanes | ~4.1.2 | 4.1.2 | ^4.0.4 |
| vue | ~3.5.42 | 3.5.42 | ^3.5.33 |
| vue-draggable-plus | ~0.6.1 | 0.6.1 | ^0.6.1 |
| vue-echarts | ~8.3.0 | 8.3.0 | ^8.3.0 |
| vue-router | ~4.6.4 | 4.6.4 | ^4.6.4 |
| @vueuse/core | ~12.8.2 | 12.8.2 | ^12.0.0 |
| @xterm/addon-fit | ~0.11.0 | 0.11.0 | ^0.11.0 |
| @xterm/addon-web-links | ~0.12.0 | 0.12.0 | ^0.12.0 |
| @xterm/xterm | ~6.0.0 | 6.0.0 | ^6.0.0 |

注：0.x 包（docx-preview、xterm 三件）tilde 语义 = minor+patch 全锁（npm 0.x 特例），风险更低。

### caret（8 项）——测试/构建工具链，维持现状

| 依赖 | 声明 | lock 现状 |
|---|---|---|
| @playwright/test | ^1.63.0 | 1.63.0 |
| @vitejs/plugin-vue | ^6.0.6 | 6.0.9 |
| @vitest/coverage-v8 | ^4.1.5 | 4.1.11 |
| @vue/test-utils | ^2.4.9 | 2.5.0 |
| jsdom | ^29.1.0 | 29.1.1 |
| vite | ^8.0.10 | 8.3.0 |
| vitest | ^4.1.5 | 4.1.11 |
| @pinia/testing | ^2.0.1 | 2.0.1 |

@pinia/testing 归工具链档（测试辅助库，非运行时 UI 面）。

### url（1 项）

xlsx：CDN tarball 固定 0.20.3，天然 exact，不动（已知无补丁漏洞接受项，见 docs/spec/security-scan.md）。

## 漂移显著项（治理前现状）

| 依赖 | 声明 | lock | 漂移幅度 |
|---|---|---|---|
| element-plus | ^2.13.7 | 2.14.5 | minor（事故源） |
| vite | ^8.0.10 | 8.3.0 | minor×3 |
| @vueuse/core | ^12.0.0 | 12.8.2 | minor×8 |
| @codemirror/state | ^6.6.0 | 6.7.4 | minor |
| highlight.js | ^11.11.1 | 11.12.0 | minor |
| markdown-it | ^14.2.0 | 14.3.2 | minor |
| mermaid | ^11.16.0 | 11.17.2 | minor |
| splitpanes | ^4.0.4 | 4.1.2 | minor |
| vue | ^3.5.33 | 3.5.42 | patch |

## 确认弹框回归哨兵用例清单（E2E，frontend/e2e/）

ElMessageBox 消费 spec（grep message-box 命中 5 文件）：

- assets-export.spec.js（递归恒弹形态，EP 暴露面最大）
- assets-backup-rules.spec.js（顺序两段形态，`:visible`+filter 范式）
- assets-relink.spec.js
- file-tree.spec.js
- submodule.spec.js

## CI 现状（.github/workflows/ci.yml）

- test job：串行 18 步（checkout → dist 占位 → Go → Node → apt → wails → E2E → vitest）
- security job：continue-on-error，govulncheck + npm audit
- 一致性检查插入点：test job「检出代码」后早期 step（脚本纯读 package.json + package-lock.json 两文件，零依赖零安装，快速失败）
