# data JSON 落盘原子写契约

## 统一原子写入口

全部 `data/` 目录 JSON 持久化必须经 `util.SaveJSON`（`util/json.go`）：

```
MarshalIndent → MkdirAll → os.CreateTemp(同目录, ".<目标名>.tmp-*") → Write → Sync → Close → os.Rename 替换目标（defer 清理 temp 残留）
```

* **禁新增** `os.WriteFile` 直写 data JSON（截断写，崩溃产生半截文件）；禁再自建 temp+rename 实现（历史上 `saveChatJSON`/`atomicWriteConfig` 各建一套，2026-09 已收敛删除）。
* 目标文件只在 rename 一步被替换：进程崩溃不产生半截 JSON，读取方（`LoadJSON` 精确路径读）不会读到中间态。
* fsync 口径为临时文件级 Sync；目录级 fsync 不做（Windows 不支持，防掉电持久化不在目标内）。
* 崩溃残留的 `.<目标名>.tmp-*` 为隐藏点文件，无加载方按精确路径消费，无害；**不做自动清扫**（并发保存同文件时清扫他方在写临时文件会误删）。
* 目标文件权限为 CreateTemp 的 0600（原 0644）：应用数据属主私有，收紧合理；Windows 下无实质影响。

## Windows 覆盖语义

`os.Rename` 在 Windows 走 `MoveFileEx(MOVEFILE_REPLACE_EXISTING)`，对已存在目标为同卷原子替换（Go 标准库自 1.5 起行为）。测试锚定：`util/json_test.go` `TestSaveJSON_OverwriteExisting`；中断残留无害锚定：`TestSaveJSON_StaleTempResidueDoesNotBreakTarget`。

## 领域包装层（保留的合法特例）

|包装|位置|额外语义|
|---|---|---|
|`saveChatJSON`|service/chat_service.go|覆写前把上一好版本复制为 `<目标>.bak`（写侧保险，无加载方消费）|
|`atomicWriteConfig`|service/obsidian_vault.go|领域入口名，直接委托 `util.SaveJSON`|

包装层只允许叠加语义（备份等），原子核心必须委托 `util.SaveJSON`。

## 边界：原子写不解决整体覆盖丢数据

`RepoMetaService` 等服务的整体覆盖模型（内存态全量序列化覆盖文件）意味着：应用运行期间外部手改 data/*.json，任意一次应用保存都会以内存旧快照覆盖手改内容——last-writer-wins 与写入原子性正交。手改 data 文件前仍须关闭 workbench.exe。

## 新增落盘点核对清单

1. data JSON 持久化 → 只用 `util.SaveJSON`（或叠加语义的领域包装）
2. 非 JSON 写入（脚本/标记/token/导出文件/备份 .bak）不强制原子写，但主数据文件禁止截断写
3. 新增 temp 文件命名沿用 `.<目标名>.tmp-*` 模式（chat_service_test 的 glob 断言依赖该模式）
