# Journal - liuyang (Part 4)

> Continuation from `journal-3.md` (archived at ~2000 lines)
> Started: 2026-10-29

---



## Session 169: 素材文件夹树全展/全收与双击切换

**Date**: 2026-10-29
**Task**: 素材文件夹树全展/全收与双击切换
**Branch**: `master`

### Summary

FolderTree 头部新增全部展开/全部收起按钮（loading/busy 禁用，全收含根）+ 双击目录名切换展开收起（单击选中过滤不变）；核心纪律 syncExpandedKeysFromStore 单点回写防 reload 展开面漂移（store 直赋 expanded 不触发事件，default-expanded-keys 仅新建 Node 生效）；宿主零改动；单测 24/24 全量 2249 绿；文档同步 README/功能说明 14.9/spec §8.7/CLAUDE.md 关键规则行（超限已削至 245）；trellis-check 六项过

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `af1f7e0` | (see git log) |
| `5b2332f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
