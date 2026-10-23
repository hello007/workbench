# Memory Index

- [WorkBench 数据写入约束](workbench-data-write-constraint.md) — 手改产物 JSON 前仍须关闭 workbench.exe（整体覆盖模型不变）；崩溃截断损坏已由原子写消除（2026-09-24 收口）
- [Trellis 收尾入口](trellis-finish-work-entry.md) — 任务归档走 /trellis:finish-work，禁裸跑 task.py archive
