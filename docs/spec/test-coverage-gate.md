# 测试覆盖率分层门禁

> 本文档规定 WorkBench 测试覆盖率的分层阈值、排除规则与 CI 门禁脚本契约。
> 最后更新：2026-09-10 · 来源任务：09-10-test-coverage

## 1. 适用范围

- 后端 Go 四包 + workbench 主包的 `go test -cover` 分层门禁
- 前端 Vitest v8 coverage thresholds 硬失败门禁
- CI 门禁脚本 `scripts/coverage-check.sh` 的阈值契约

## 2. 后端分层阈值

| 包 | 阈值 | 现状基线 | 说明 |
|---|---|---|---|
| model | ≥80% | 100% | 业务核心，必须达标 |
| server | ≥80% | 91% | 业务核心，必须达标 |
| service | ≥76% | 76.1% | 含 `RunStage` / `pumpOutput` / `killProcessTree`（claude 子进程）、`terminal` pty 读写、`clipboard` 系统剪贴板、`update` 网络下载 + bat 启动、外部进程启动、`searchWithRipgrep`（ripgrep 未装）等系统调用核心，本质难单测，76% 为实际可达上限 |
| util | ≥40%（排除 `pty_windows.go`） | 42.1% | `pty_windows.go` 含伪终端系统调用，按文件名排除计算 |
| workbench 主包 | 不设门禁 | 26.9% | `app.go` + `app_*.go` 共 14 文件均为转发 service 的薄包装，service 已测则 app 层补测重复验证价值低 |

**关键约束**：
- service 阈值定为 76% **非 80%**，是工程现实妥协。强行补测到 80% 须给系统调用核心写无意义 mock，违反「不为覆盖率写无意义测试」原则。
- util 排除 `pty_windows.go` 后计算，脚本按文件名 `index(file, "pty_windows.go") > 0` 跳过。
- workbench 主包不设门禁，`app_*.go` 后续新增方法无测试保护（可接受，薄包装）。

## 3. 前端阈值

`frontend/vitest.config.js` 配置：

```js
test: {
  coverage: {
    provider: 'v8',
    reporter: ['text', 'html'],
    exclude: ['wailsjs/**'],
    thresholds: {
      lines: 70,
      branches: 70,
      functions: 70,
      statements: 70
    }
  }
}
```

| 指标 | 阈值 | 现状基线 |
|---|---|---|
| lines | ≥70% | 81.67% |
| branches | ≥70% | 70.26% |
| functions | ≥70% | 73.75% |
| statements | ≥70% | 78.84% |

**关键约束**：
- `exclude: ['wailsjs/**']` 必须排除 Wails 生成的运行时绑定（`wailsjs/go`、`wailsjs/runtime` 非业务代码，原 0% 覆盖率会拖低整体）。
- 硬失败：低于阈值 `npm run test:coverage` exit 非 0。
- 设阈值前须先补测到 ≥70%，否则 CI 红。

## 4. CI 门禁脚本契约

### 后端 `scripts/coverage-check.sh`

**触发**：CI 或本地 `bash scripts/coverage-check.sh`

**契约**：
1. 跑 `go test ./... -coverprofile=coverage.out` 生成合并 profile
2. `go tool cover -func=coverage.out` 按包解析百分比
3. 阈值校验：model ≥80% / server ≥80% / service ≥76% / util ≥40%（排除 `pty_windows.go`）
4. workbench 主包不检查
5. 超阈值 `fail=1`，末尾 `exit $fail`
6. 打印各包实际值与目标
7. 临时 `coverage.out` 末尾清理

**Windows bash 可运行**（项目 Shell 为 bash）。

### 前端门禁

`npm run test:coverage`（= `vitest --coverage`），低于 thresholds exit 非 0。

## 5. 测试模式约定

### 后端

- 测试文件命名：`*_test.go`，与被测文件同目录同 `package`
- 文件 IO 隔离：`t.TempDir()` 创建临时目录，不污染工作区
- Mock 用现有测试工具函数，不另造轮子
- 每条用例覆盖明确行为或边界，**不为覆盖率写无意义测试**

### 前端

- 测试文件命名：`__tests__/*.spec.js`，与被测组件同目录
- 框架：Vitest + Vue Test Utils
- 测试环境：`jsdom`，setup 文件 `src/test/setup.js`
- Wails 绑定 mock 约定见 `src/test/setup.js`，复用现有方式
- Element Plus 组件用 stub
- 每条用例覆盖明确行为或边界

## 6. Wrong vs Correct

### Wrong：为覆盖率写无意义测试

```js
// 错误：只调函数不断言，纯凑覆盖率
it('runs', () => {
  const wrapper = mount(Component)
  wrapper.vm.someMethod() // 无 expect
})
```

### Correct：覆盖明确行为 + 断言

```js
// 正确：断言行为结果
it('someMethod 状态变更后 emit update', async () => {
  const wrapper = mount(Component)
  await wrapper.vm.someMethod()
  expect(wrapper.emitted('update')).toBeTruthy()
  expect(wrapper.emitted('update')[0]).toEqual([expectedPayload])
})
```

### Wrong：强测系统调用核心

```go
// 错误：给 pty 系统调用写大量 mock 硬拉覆盖率到 80%
func TestPtyRead(t *testing.T) {
    // 大量 mock 系统调用，测试脆弱且无意义
}
```

### Correct：接受基线 + 排除难测文件

```bash
# 正确：pty_windows.go 按文件名排除，util 阈值 40% 测可测部分
# service 接受 76% 基线，阈值设 76% 防回退
```

## 7. 维护规则

- 新增 service 系统调用核心方法（pty / 子进程 / 网络下载 / 外部进程）：不强行补测，service 阈值保持 76%，但须在 PR 说明未测原因
- 新增前端组件：须补 spec 到整体 ≥70%，否则 thresholds 硬失败阻断
- util 新增系统调用文件：脚本按文件名排除规则可扩展（`exclude` 判断逻辑）
- 阈值调整须同步更新本文档 + `docs/测试策略.md` 基线 + `CLAUDE.md` 关键规则
