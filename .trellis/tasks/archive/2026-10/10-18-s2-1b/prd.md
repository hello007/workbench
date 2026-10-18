# 素材管理缺陷修复：排除后缀规则归一未去重（冒烟 S2.1b）

## Goal

第十批真机冒烟（RPC 驱动 42/43 过）发现的唯一缺陷：`normalizeAssetExcludeRules`（service/assets_exclude.go）对 ExcludeExts 归一但**不去重**——输入 `tmp` / `.tmp` / `*.TMP` 三形态等价归一后回显 `[".tmp", ".tmp", ".tmp"]` 三条重复，前端索引设置弹窗呈现三条重复规则，用户困惑。功能无损（匹配幂等），纯数据卫生面。

## Requirements

* `normalizeAssetExcludeRules` 归一后对 excludeExts 去重（保序去重，首次出现位置保留）
* ExcludeDirs 同样去重（同值多形态目录模式如 `草稿*` 与 `草稿*` 重复输入；glob 模式归一仅 TrimSpace，同字符串去重）
* 去重发生在校验之后（坏模式剔除 → 归一 → 去重）
* 单测：三形态混合输入回显单条；重复目录模式回显单条

## Acceptance Criteria

* [ ] `UpdateAssetLibraryRules(id, [], ["tmp", ".tmp", "*.TMP"])` 后 `ListAssetLibraries` 回显 excludeExts == `[".tmp"]`
* [ ] 重复目录模式 `["archive", "archive"]` 回显 `["archive"]`
* [ ] 既有测试全绿（归一/防御剔除/匹配语义不回归）

## Out of Scope

* 已入库 libraries.json 的存量重复规则清洗（去重逻辑下次保存自动收敛，不迁移）

## Technical Notes

* 冒烟证据：`.trellis/tasks/archive/2026-10/10-18-srs-2-7-3/rpc_smoke.py` S2.1b
* 相关 spec：docs/spec/assets-management.md §12.1（坏规则双防线）
