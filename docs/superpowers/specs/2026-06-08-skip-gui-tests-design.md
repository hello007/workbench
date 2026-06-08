# 跳过会拉起外部 GUI 的后端测试

**日期**：2026-06-08
**作者**：刘阳
**状态**：待评审
**关联代码**：`service/fileoperation_test.go`、`service/fileoperation.go`

## 1. 背景

`go test ./...` 当前每次运行都会触发以下副作用：

- 弹出 Windows 资源管理器窗口（指向临时目录或选中临时文件）
- 启动 VSCode 并新建一个无意义的临时目录窗口

根因在 `service/fileoperation_test.go` 的 7 个用例直接调用 `FileOperationService.OpenInExplorer` / `OpenInVSCode`，而二者实现是真实的 `exec.Command("explorer", ...).Start()` 和 `exec.Command("code", ...).Start()`（见 `service/fileoperation.go:101-119`），没有抽象层可拦截。

测试本身价值有限：仅断言 `cmd.Start()` 不报错，并不能验证「外部程序确实正确打开了目标」，但代价（频繁打断专注、污染桌面状态）很高。

## 2. 目标 / 非目标

**目标**

- `go test ./...` 不再弹出任何资源管理器或 VSCode 窗口
- 保留用例骨架与原断言代码，便于将来手动验证或改造为 mock
- 改动范围严格控制在 `service/fileoperation_test.go`

**非目标**

- 不重构 `OpenInExplorer` / `OpenInVSCode` 实现（不引入 `commandRunner` 注入）
- 不引入环境变量开关（如 `RUN_GUI_TESTS=1`）
- 不改动前端测试（`FileTreePanel.spec.js` 已用 `vi.fn()` 做了 mock，无副作用）
- 不调整其它 `_test.go` 文件

## 3. 处理清单

`service/fileoperation_test.go` 中 7 个相关用例的处理对照：

| 行号 | 用例 | 当前行为 | 处理 | 理由 |
|---|---|---|---|---|
| L9 | `TestOpenInExplorer_Directory` | 弹资源管理器（临时目录） | `t.Skip` | 真实弹窗 |
| L19 | `TestOpenInExplorer_File` | 弹资源管理器并选中文件 | `t.Skip` | 真实弹窗 |
| L32 | `TestOpenInExplorer_NotFound` | `os.Stat` 失败先返回，不弹窗 | **保留** | 无副作用，覆盖错误分支 |
| L41 | `TestOpenInExplorer_EmptyPath` | `os.Stat` 失败先返回，不弹窗 | **保留** | 无副作用，覆盖错误分支 |
| L178 | `TestOpenInVSCode_Directory` | 启动 VSCode 打开临时目录 | `t.Skip` | 真实启动 VSCode |
| L188 | `TestOpenInVSCode_File` | 启动 VSCode 打开临时文件 | `t.Skip` | 真实启动 VSCode |
| L201 | `TestOpenInVSCode_InvalidCommand` | `exec.Command("code", "")` 仍会启动 VSCode 默认窗口 | `t.Skip` | 真实启动 VSCode |

合计：跳过 5 条，保留 2 条，无删除。

## 4. 实现细节

在 5 个目标用例函数体的首行插入：

```go
t.Skip("会启动外部 GUI 进程（资源管理器/VSCode），默认跳过；如需手动验证请删除此行")
```

原有断言代码在 `t.Skip` 之后保留，编译器仍可校验语法，删除 `t.Skip` 即可恢复。

示例（以 `TestOpenInExplorer_Directory` 为例）：

```go
func TestOpenInExplorer_Directory(t *testing.T) {
	t.Skip("会启动外部 GUI 进程（资源管理器/VSCode），默认跳过；如需手动验证请删除此行")
	dir := t.TempDir()
	svc := NewFileOperationService()

	err := svc.OpenInExplorer(dir)
	if err != nil {
		t.Fatalf("OpenInExplorer(directory) failed: %v", err)
	}
}
```

## 5. 取舍说明

| 备选方案 | 未采纳原因 |
|---|---|
| 整段块注释 `/* ... */` | 跳过的可见性不如 `t.Skip`：CI 看不到 SKIP 计数，恢复时容易漏 |
| 直接删除 5 个用例 | 想做手动冒烟时还得重写，没必要 |
| `commandRunner` 接口 + mock | 改动过大，超出本次范围；如未来要做行为级断言再做 |
| `RUN_GUI_TESTS=1` 环境变量门控 | YAGNI——想跑临时删 `t.Skip` 即可，无需维护开关 |

## 6. 验证方式

落地后执行：

```bash
go test ./service/... -run "OpenInExplorer|OpenInVSCode" -v
```

**预期**

- 5 条 `--- SKIP: TestOpenIn...`，跳过原因输出注释中的提示文本
- 2 条 `--- PASS`（`_NotFound`、`_EmptyPath`）
- 测试期间无任何资源管理器窗口或 VSCode 窗口弹出
- 整套 `go test ./...` 退出码 0

## 7. 影响面

- **覆盖率**：仅放弃「`cmd.Start()` 不报错」这一弱断言，`os.Stat` 错误分支仍有覆盖
- **CI**：无变化，本就是本地全量测试
- **回滚**：删除 5 处 `t.Skip(...)` 一行即可恢复

## 8. 后续工作（不在本次范围）

如果将来想真正验证 `OpenInExplorer` / `OpenInVSCode` 的行为，建议另起需求引入 `commandRunner` 接口，将 `exec.Command` 的构造与启动解耦，测试用 fake runner 断言「传入了 `explorer` + 期望参数」，避免再次依赖真实 GUI。
