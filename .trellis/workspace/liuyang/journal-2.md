# Journal - liuyang (Part 2)

> Continuation from `journal-1.md` (archived at ~2000 lines)
> Started: 2026-09-13

---



## Session 57: 外部 diff 工具集成

**Date**: 2026-09-13
**Task**: 外部 diff 工具集成
**Branch**: `master`

### Summary

落地路线图「差异工具集成」：设置面板新增外部 diff 工具配置（四预设模板+路径+{left}{right}参数模板），FileDiffDialog 三场景（工作区/提交/区间）一键外部工具打开，未配置置灰引导、启动失败 AppError 分流；后端 GitService.OpenInExternalDiff（git show 取版本内容落临时文件、模板拼命令、隐藏窗口启动），startup 清理上会话临时残留；修正路线图漂移（E2E 三流程改六流程、全局错误处理勾选）。附带修复：SettingsPanel 全量覆盖写未带新字段会清空 diff 配置。测试：后端单测+集成、前端 896 全绿（+18）、E2E 36（+4）、覆盖率门禁双端达标。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `539069f` | (see git log) |
| `87f6db9` | (see git log) |
| `e046ef4` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 58: 外部 diff 工具审核修复

**Date**: 2026-09-13
**Task**: 外部 diff 工具审核修复
**Branch**: `master`

### Summary

双路代码审核发现 1 bug + 10 risk + 1 nit 全部修复：SHA 入参正则校验（防 git 选项注入任意写文件）、git show 仅缺失语义降级（防超时/损坏伪装成新增假 diff）、os.Stat 仅 ErrNotExist 降级、读设置失败改普通 error、临时目录 os.MkdirTemp 原子唯一、util 测试根注入；前端通用保存改合并写消除「加载失败后通用保存清空 diff 配置」残留路径、预设切换加确认防误覆盖、保存失败补提示、range 模式 header 按钮禁用、loadDiff 序号守卫防过期响应覆盖、diffToolConfigured 补 args 校验。测试：前端 903 全绿（+7）、E2E 36、后端全绿、门禁零回归。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `4454bbc` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 59: 仓库列表配置导入导出

**Date**: 2026-09-13
**Task**: 仓库列表配置导入导出
**Branch**: `master`

### Summary

WorkBench 新增仓库列表配置（工作目录+收藏夹）JSON 导出导入：manifest v1 结构、RepoConfigService 聚合两数据源（Export/PreviewImport/ApplyImport）、冲突三决策（跳过/覆盖/另存为新项）预览-汇总链路、E_REPO_CONFIG_* 错误码双侧同步、DirectoryTree 工具栏入口、新错误码与 base64 UTF-8 解码契约沉淀 spec；后端 18 单测 + 前端覆盖率门禁 + E2E 6 用例全绿，trellis-check 修 saveAsNew 路径规范化与收藏分组兜底

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `11ab5b6` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
