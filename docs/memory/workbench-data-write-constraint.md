---
name: workbench-data-write-constraint
description: 手改 WorkBench data/*.json 前仍须关闭 workbench.exe（整体覆盖模型不变）；崩溃截断风险已由原子写消除（2026-09-24 收口）
metadata: 
  node_type: memory
  type: feedback
  originSessionId: cea5dfcf-ec7f-4076-ab1e-4d5d0fc4bddf
---

直接写 `D:\Program Files\WorkBench\data\` 下任意配置文件（repo_meta.json / favorites.json / directories.json 等）前，仍必须先确认 WorkBench 进程已退出。

**Why**：根因是 `RepoMetaService`（`service/repo_meta.go`）的**整体覆盖模型**——运行中的应用内存持有旧快照，UI 任何触发保存的操作都会用内存数据全量覆盖文件，last-writer-wins 与写入原子性正交。2026-09-24 技术债批次（09-23-json-atomic-write）已把全部 data JSON 落盘收口为原子写（`util.SaveJSON`：CreateTemp 同目录临时文件 → fsync → os.Rename 原子替换），**消除了崩溃/写入中断把文件截断成半截 JSON 的损坏形态**；但应用运行期间的手改内容仍会被内存旧快照覆盖——该约束不变。

**How to apply**：手改 data/*.json 前用 `tasklist | grep -qi workbench` 校验，无进程才写；写入前先 `cp` 备份（`.bak-日期`）。注意产物数据路径 `D:\Program Files\WorkBench\data\` 与源码 `git-manager/data/` 是两份，用户日常用前者，别写位置。
