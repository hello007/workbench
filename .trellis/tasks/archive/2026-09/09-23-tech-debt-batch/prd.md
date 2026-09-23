# 技术债清偿批次（Epic）

## Goal

一次批次清偿 4 项存量技术债：覆盖率门禁回全绿、格式化/lint 检查输出噪音清零、JSON 落盘鲁棒性收口。均为 2026-09-23 contributor-line-ranking 任务 check 阶段实测发现的存量问题 + 历史教训沉淀。

## 子任务（按序执行）

1. `09-23-model-coverage-80` — model 包覆盖率 76.5% < 80% 门禁补测回绿（P1）
2. `09-23-gofmt-cleanup` — gofmt 存量未格式化文件清零（P1）
3. `09-23-vet-clipboard-audit` — go vet clipboard_windows.go unsafe.Pointer 警告定性（P1）
4. `09-23-json-atomic-write` — data/*.json 原子写收口（P1，四项中唯一需 brainstorm 的）

## Acceptance Criteria

* [x] 4 个子任务全部按 trellis 流程完成并归档（model-coverage-80 / gofmt-cleanup / vet-clipboard-audit / json-atomic-write → archive/2026-09/）
* [x] 收口验证：`go test ./...` 全绿（6 包 -count=1）；`scripts/coverage-check.sh` 全门禁 PASS（model 97.1%/server 93.5%/service 79.8%/util 55.1%）；`gofmt -l` 无输出；`go vet` 零警告（按 docs/开发规范.md 项目规范命令 `-unsafeptr=false`，clipboard 5 处为 go#44836 已知误报已在子任务3定性留档）
* [x] 父任务最后归档

## Out of Scope

* B 档功能延伸任务（stats-file-line-dist / three-way-merge / error-report-export / desktop-e2e-cdp 各自独立任务）
* 前端格式化工具链变更
