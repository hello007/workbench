## 1. 代码修改

- [x] 1.1 在 import 块中添加 `"log"` 包
- [x] 1.2 将 `println("Error:", err.Error())` 替换为 `log.Fatalf("Error: %v", err)`

## 2. 验证

- [x] 2.1 运行 `go build` 确认编译通过
