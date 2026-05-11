# git-manager --version 支持设计

**日期：** 2026-05-11
**状态：** 已确认

## 需求

1. 打包后的 `git-manager.exe` 支持 `--version` 命令输出
2. 每次打包时自动生成 version（默认为打包时间戳）
3. `install.sh` 安装后可打印版本信息

## 输出格式

```
git-manager v1.0.0 (build 20260511-101500)
```

- 语义版本：从 `wails.json` 的 `productVersion` 读取
- 构建时间戳：打包时自动生成

## 方案：Go `-ldflags` 注入

构建时通过 `-ldflags "-X main.version=... -X main.buildTime=..."` 将版本变量注入二进制文件。

**选择理由：** Go 生态标准做法，零源文件污染，版本信息直接嵌入二进制。

## 文件变更

| 文件 | 变更 |
|------|------|
| `main.go` | 新增 version/buildTime 变量 + `--version` 参数解析 |
| `scripts/build.sh` | **新建**，自动注入版本的构建脚本 |
| `scripts/install.sh` | 无需修改，已适配 `--version` |

## 详细设计

### main.go

新增两个包级变量，供链接器注入：

```go
var (
    version   = "dev"
    buildTime = "unknown"
)
```

在 `main()` 入口添加 CLI 参数拦截：

```go
func main() {
    if len(os.Args) > 1 && os.Args[1] == "--version" {
        fmt.Printf("git-manager v%s (build %s)\n", version, buildTime)
        os.Exit(0)
    }
    // ... 原有 Wails 启动逻辑
}
```

### scripts/build.sh

- 从 `wails.json` 读取 `productVersion` 作为语义版本
- 自动生成构建时间戳
- 支持通过命令行参数手动指定版本：`./scripts/build.sh 2.0.0`
- 通过 `wails build -ldflags` 注入变量
