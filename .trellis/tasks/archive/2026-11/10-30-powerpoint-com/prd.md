# 探针：PowerPoint COM 多实例并行导出与整本导出可行性

## Goal

用真机探针回答两个决策：① 缩略图批量生成能否靠多实例并行提速；② `Presentation.Export` 整本一次导出能否替代逐页 `Slide.Export` 循环。结论只落 `research/`，不改生产代码、不作生产契约。

## 环境（2026-11-01 实测）

| 项 | 结果 |
|---|---|
| Microsoft PowerPoint | **未安装**（`C:\Program Files\Microsoft Office` 无 `POWERPNT.EXE`，`HKCR\PowerPoint.Application\CLSID` 指向的 CLSID 节点缺失） |
| WPS Office | 已安装，`%LOCALAPPDATA%\Kingsoft\WPS Office\{12.1.0.28802,12.1.0.29680}\office6\wpp.exe` |
| 结论适用范围 | 本机实测对象 = WPS COM 兼容层；**MS PowerPoint 三项结论未验证**，落 research 时显式标注 |

## 样例文件

| 用途 | 路径 | 页数 | 大小 |
|---|---|---|---|
| 主样例（真实业务，含图片/图表） | `D:\素材库\agree\afa\AFA6产品介绍_V2.0.pptx` | 116 | 39.2MB |
| 备选 | `D:\素材库\agree\运维\新核心运维平台培训手册-v1.pptx` | 75 | 40.4MB |
| 合成 fixture | 探针 `comMakeFixture` 自产（彩色矩形编码页号 + 可控隐藏页） | 6-8 | 小 |

## Requirements

- R1 探针独立可跑：`go run ./scripts/ppt-com-probe -only 9,10,11,12,13`，不依赖 workbench 运行时。
- R2 每项输出结构化结果：指标名 + 采样数组 + 中位数 + p95 + 判定结论；`-json <path>` 落文件，stdout 保留结论行。
- R3 环境不可用项记「未验证」及原因，禁推测填数。
- R4 不修改生产代码；改动限 `scripts/ppt-com-probe/` 与本任务 `research/`。
- R5 进程清理按 PID 精确执行；探针启动前检测到任何 Office 演示进程即拒跑（禁无差别 `taskkill /IM`）。

## Probe Design（已确认 2026-11-01）

### P9 多实例 CreateObject 行为（并行可行性前置门）

- **指标**：连续 3 次 `CreateObject("PowerPoint.Application")` 后的新增 PID 数；两 app 的 `Presentations` 集合交叉可见性（app2 打开文件后 app1 的 `Count` 是否变化）。
- **强制新建路径**（各测一次，失败也记录拒绝形态）：(a) 先 `exec` 起 `wpp.exe`/`POWERPNT.EXE` 再 CreateObject，比对 PID 是否为自己所起；(b) 两个独立 STA goroutine 并发 CreateObject；(c) 直调 `CoCreateInstance(CLSCTX_LOCAL_SERVER)`；(d) 启动参数 `/automation`、`-automation`（探测性，不假设两引擎支持）。
- **采样**：3 轮，每轮前清空 Office 进程。
- **判定**：新增 PID ≥2 且集合互不可见 ⇒ 「多进程可建」；否则记「单实例复用」+ 4 条路径结果。

### P10 两实例并发导出总耗时

- **场景 A**：同一文件两实例各导一半页（验证 MS 侧「已打开」限制；WPS 已记允许双开）。
- **场景 B**：两个不同文件（同源复制）两实例并发整本导。
- **指标**：墙钟总时长、加速比（串行/并发）、每页中位耗时、COM 错误率（失败页/总页）、错误形态（hr + desc）。
- **采样**：每场景 `-reps`（默认 3）轮，取中位数；首轮单列 cold。
- **判定**：加速比 ≥1.4 且错误率 0 ⇒ 值得并行；1.2-1.4 ⇒ 仅千页级库有意义；<1.2 ⇒ 不值得。
- **前置**：P9 不成立则记「未验证（前置不成立）」，仍跑 A/B 记录实际行为。

### P11 单页导出耗时分档

- **三段拆分**：`Presentations.Open`、单页 `Slide.Export`、`Close`。
- **三档**：small 320×180 PNG / hd 1600×900 PNG / BMP 原始（不传宽高）。
- **采样**：真实样例抽首/中/尾共 ≥10 页，每页每档 `-reps` 次取中位，报 median/p95/min/max；记每档字节数与 IHDR 尺寸。
- **判定**：small ≥ hd×60% ⇒ 双档导出不划算（倾向 hd 一次 + Go 派生）；small ≤ hd×30% ⇒ 双档可接受；BMP 快于 hd PNG ≥30% ⇒ BMP 路线进候选。

### P12 Go 侧后处理耗时

- **三段拆分**：`png.Decode`、缩放到 small（stdlib `draw.NearestNeighbor`）、`png.Encode`。
- **采样**：同批 10 张 hd 图各 5 次，报 median/p95 + 峰值堆增量。
- **判定**：单页 decode+scale+encode ≤30ms ⇒ Go 侧可承担；且 < COM small 单页耗时 50% ⇒ 「只导 hd、small 由 Go 派生」胜出。
- **说明**：`golang.org/x/image/draw.CatmullRom` 对比需新增依赖，本轮不做；结论标注「最近邻口径」。

### P13 `Presentation.Export` 整本导出

- **指标**：产物目录形态与逐页文件名（大小写/补零/扩展名）；页序映射（文件名序 vs `sldIdLst` 序 vs 页内容标识）；隐藏页是否导出；整本 vs 逐页循环耗时（small/hd）；传宽度是否生效（IHDR）。
- **页序识别**：合成 fixture 每页铺满唯一颜色矩形（RGB 编码页号），导出后读 PNG 中心像素反解页号；真实样例另跑一次只验命名与数量。
- **采样**：耗时 3 轮中位；命名与映射各跑 2 个不同页数 fixture 确认规则稳定。
- **判定**：整本 ≤ 逐页循环×0.8 且映射确定可逆 且 隐藏页行为与逐页一致 ⇒ 可作替代候选；任一条不满足 ⇒ 仅作参考。

## Acceptance Criteria

- [x] P9-P13 各产出结论行，含采样数与中位数
- [x] `-json` 输出可解析，含环境头/样例文件/每项原始采样
- [x] 结论落 `.trellis/tasks/10-30-powerpoint-com/research/com-parallel-export-probe.md`
- [x] MS PowerPoint 相关结论显式标「未验证（本机未安装）」
- [x] 探针可重复运行，运行前后无残留 Office 进程（实测 tasklist 计数 0）

## Definition of Done

- `go vet ./scripts/ppt-com-probe` 通过、`go run ... -only 9` 可跑
- research 文件含原始数据表 + 判读口径 + 局限说明
- 生产代码零改动、生产 `go.mod` 零新增依赖

## Out of Scope

- 改造 `service/assets_thumbs*.go` 或引入并行链路
- 把探针结论写成 spec 契约（留后续正式任务）
- CI 接入、MS Office 机器实测（本机无）
- `golang.org/x/image` 高质量重采样对比

## Technical Notes

- 现有探针 P1-P8 不动；新项 P9-P13 续编号。
- 复用：`withComSession`、`officeServerPids`/`diffPids`、`comMakeFixture`、`openPresentation`、`pngSize`、`pngInkRatio`。
- 新增：`requireCleanOfficeEnv`（有残留即拒跑）、`newComApp`（分离创建与清理）、`exportPages`（一次 Open 多页导出计时）、`probeRecorder`（结构化结果收集）。
- `quitSoon()` 现有实现无差别杀全部 Office 进程，新项**不得调用**。
- 隐藏页设置：`Slide.SlideShowTransition.Hidden = msoTrue`。
- 判定口径来源：`docs/spec/assets-thumbs-com.md`（单 STA 串行、进程归属、绝对路径、瞬态重试）。

## Decision (ADR-lite)

**Context**：缩略图批量生成是长尾耗时，单 STA 串行；业界常见做法是多进程并行，但 Office COM 以 multi-use 复用为主。
**Decision**：先探针实测多进程可得性与并发收益，再决定是否改生产链路；整本导出只作为候选，需先过「耗时 ×0.8 + 映射可逆 + 隐藏页一致」三条。
**Consequences**：结论只对 WPS 成立；MS Office 需另机复跑同一探针。
