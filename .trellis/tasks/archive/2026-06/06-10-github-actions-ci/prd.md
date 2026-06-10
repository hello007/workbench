# GitHub Actions 自动打包发版流水线

## Goal

为 WorkBench（Wails v2 桌面应用）创建 GitHub Actions Release 流水线：推送 `v*` tag 时自动构建 Windows exe 并发布到 GitHub Release。

## Decisions

| 编号 | 决策 | 选择 | 理由 |
|------|------|------|------|
| D1 | Git Remote 策略 | GitHub 作为 Gitee 镜像，仅用于 CI/CD | 代码主仓库保持 Gitee 不变 |
| D2 | 构建平台 | 仅 Windows x64 | 用户明确需求，减少复杂度 |
| D3 | Release Notes | GitHub 自动生成（`generate_release_notes: true`） | 最简单，GitHub 按 PR/commit 自动分类 |
| D4 | CI 范围 | 仅 Release 流水线 | 先跑通发版流程，日常 CI 后续添加 |
| D5 | MVP 范围 | 最小可用 | 产物命名 workbench.exe，不含版本号后缀 |

## Requirements

1. 当推送 `v*` 格式的 tag（如 `v1.0.7`）时自动触发 GitHub Actions
2. 在 `windows-latest` runner 上安装构建环境（Go 1.24 + Node 20 + Wails CLI）
3. 执行 `wails build` 打包 Windows x64 exe
4. 通过 `-ldflags` 注入 `version`（从 tag 提取）和 `buildTime`（构建时间）
5. 自动创建 GitHub Release（tag 名称作为标题）
6. Release Notes 由 GitHub 自动生成（`generate_release_notes: true`）
7. 上传 `build/bin/workbench.exe` 至 Release Asset

## Acceptance Criteria

- [ ] 推送 `v*` tag 后 GitHub Actions 自动触发
- [ ] 流水线成功完成：Go + Node 依赖安装 → Wails CLI 安装 → `wails build`
- [ ] 产出的 exe 通过 `--version` 输出正确的版本号和构建时间
- [ ] GitHub Release 页面自动创建，标题为 tag 名称（如 `v1.0.7`）
- [ ] Release 页面包含自动生成的变更摘要
- [ ] `workbench.exe` 作为 Release Asset 可下载

## Definition of Done

- `.github/workflows/release.yml` 已创建
- README.md 或 docs/ 中补充发版流程说明
- 至少一次实际 tag 推送验证通过

## Technical Approach

### 环境依赖（研究结论）

| 依赖项 | 安装方式 | 说明 |
|--------|----------|------|
| Go 1.24 | `actions/setup-go@v5` | 匹配 go.mod |
| Node.js 20 | `actions/setup-node@v4` | LTS 版本，带 npm cache |
| Wails CLI | `go install .../wails@v2` | 锁定大版本号 |
| MinGW-w64 | 无需安装 | windows-latest 已预装 |
| WebView2 | 无需安装 | windows-latest 已预装 |

### Workflow 核心步骤

```
触发: push tag v*
  → checkout (fetch-depth: 0)
  → setup Go 1.24
  → setup Node 20 (npm cache)
  → npm ci (前端依赖)
  → go install Wails CLI
  → wails build -ldflags (注入版本号)
  → 验证 exe --version
  → softprops/action-gh-release@v2 (创建 Release + 上传 exe)
```

### 版本注入方式

从 tag 名称提取版本号（去掉 `v` 前缀），通过 `-ldflags "-X main.version=$VER -X main.buildTime=$BT"` 注入。PowerShell 语法：

```powershell
$VER = "${{ github.ref_name }}".TrimStart('v')
$BT = Get-Date -Format "yyyyMMdd-HHmmss"
wails build -ldflags "-X main.version=$VER -X main.buildTime=$BT"
```

## Out of Scope

- macOS / Linux 多平台构建
- 代码签名（Code Signing）
- 自动更新机制
- PR/push 触发的日常 CI（lint/test）
- 产物文件名包含版本号（如 `workbench-1.0.7.exe`）
- wails.json 版本号自动同步检查

## Research References

- [`research/wails-github-actions.md`](research/wails-github-actions.md) — Wails v2 在 GitHub Actions 上的完整构建指南

## Technical Notes

- **构建产物路径**: `build/bin/workbench.exe`（由 wails.json `outputfilename` 决定）
- **PowerShell 语法**: windows-latest 默认 shell 为 PowerShell，字符串操作用 `.TrimStart()` 而非 bash `${VAR#v}`
- **Wails CLI 版本**: 建议锁定 `@v2` 避免 `@latest` 未来版本不兼容
- **npm cache**: 通过 `actions/setup-node` 的 `cache: 'npm'` + `cache-dependency-path` 加速
- **package-lock.json**: 已确认存在，`npm ci` 可正常使用
