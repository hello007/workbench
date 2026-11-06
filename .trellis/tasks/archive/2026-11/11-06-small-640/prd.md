# 提升素材缩略图 small 档分辨率至 640

## Goal

素材管理画廊当前使用 320×180 的 small 缩略图，而画廊卡片宽度可调至 420 px（内容区约 404 px），源图被放大 1.26 倍导致发糊，用户必须点开高清图才能看清内容。将 small 档默认宽度提升至 640 px，使画廊在最大卡宽下仍为降采样显示，减少对 hd 按需生成的依赖。

## Requirements

- small 档默认宽度 320 → 640（`DefaultAssetThumbSmallWidth`），clamp 区间 120–800 不变。
- 前端 `THUMB_SETTINGS_DEFAULTS.smallWidth` 同步为 640，保证配置缺失时的兜底一致。
- **存量配置一次性迁移**：`EnsureAssetsThumbsDefaults` 中，若 `Assets.SmallWidth` 恰等于旧默认值 320，升级为 640；其他自定义值原样保留。
- 迁移后旧 320 缩略图按 640 重建（复用既有 `ApplyThumbsSettingsChange` 清档链路，不新增清档逻辑）。
- 设置界面文案明确 small 档用途（画廊展示）与 hd 档用途（全屏预览），避免用户误以为两档可互相替代。
- 不新增设置段，不改 `AppSettings.EnsureAllDefaults` / `PrivateCopy` 结构。

## Acceptance Criteria

- [ ] 新用户首次启动，设置的 smallWidth 为 640。
- [ ] 存量配置 `smallWidth=320` 的用户升级后实际生效 640（读取设置即发生迁移）。
- [ ] 存量配置为 480/720 等非 320 值时不被迁移覆盖。
- [ ] `smallWidth=0`（未保存）仍走默认值 640。
- [ ] 迁移后 small 档缩略图按 640 重建，旧 320 PNG 被清理（经既有清档链）。
- [ ] 卡宽 420 时画廊缩略图无放大模糊。
- [ ] 单元测试覆盖：默认值、normalize 边界、迁移命中/不命中、幂等（二次加载不再变化）。

## Definition of Done

- 单元测试新增/更新并通过。
- `go vet ./...` 通过；前端 `npm run build` 通过。
- `docs/spec/assets-thumbs-com.md` 的默认值与迁移说明同步。
- README 或设置界面说明同步（如涉及用户可见行为）。
- 不改变 hd 档行为、配额结构与缓存键格式。

## Technical Approach

1. `model/settings.go`：
   - `DefaultAssetThumbSmallWidth` 由 320 改为 640。
   - 新增导出常量 `LegacyAssetThumbSmallWidth = 320`，供 service 层判定迁移。
   - `EnsureAssetsThumbsDefaults` 在 `Assets != nil` 分支内、`normalize()` 之前插入迁移：值等于旧默认 320 时升级为 640。
2. `service/settings.go`：
   - `Load` 在 `EnsureAllDefaults` 前快照原始 `SmallWidth`，迁移命中时**写回磁盘**（否则每次启动都会重复迁移并清档重建）。
   - 新增一次性标记 `assetsThumbMigrated` 与 `ConsumeAssetsThumbMigration()`，供启动装配触发清档。
3. `app.go`（桌面 startup）与 `main.go`（--serve 无 Wails startup）各自消费迁移标记，调用 `ApplyThumbsSettingsChange(legacy, cur)`，经既有 diff 触发 small 档清空并重新入队。
4. `frontend/src/components/AssetsPanel.vue`：`THUMB_SETTINGS_DEFAULTS.smallWidth` 改 640；小图/高清分辨率 label 增加用途 tooltip。
5. 测试：`model/settings_test.go` 增加迁移命中/不命中/幂等用例；`service/settings_test.go` 增加 Load 迁移写回、标记一次性、非旧值不迁移用例；前端 `AssetsPanel.spec.js` 的 mock 与断言同步 640。

## Decision (ADR-lite)

**Context**：存量用户 `settings.json` 已显式保存 `smallWidth: 320`，仅修改默认值不会对其生效；而 320 是旧版本默认值，非用户主动选择的典型值。

**Decision**：采用一次性迁移——配置值恰等于旧默认 320 时自动升级到 640，其他值保持不动。

**Consequences**：
- 老用户升级后立即获得清晰画廊，无需手动操作。
- 极少数主动把宽度设为 320 的用户会被改变；因 320 即旧默认值，判定为可接受。
- 未来若再次调整默认值，需同步更新 `legacyAssetThumbSmallWidth` 迁移链，避免二次迁移误伤。

## Out of Scope

- hd 档分辨率、配额与批量补缺策略不改。
- 不处理 hd 缺失 309 页与 COM 瞬态失败问题。
- 不改卡片宽度滑块范围（220–420）。
- 不改存储结构、缓存键格式与事件协议。

## Technical Notes

- 涉及文件：`model/settings.go`、`model/settings_test.go`、`frontend/src/components/AssetsPanel.vue`、`docs/spec/assets-thumbs-com.md`。
- 设置段默认值单点：`AppSettings.EnsureAllDefaults` → `EnsureAssetsThumbsDefaults`，三处消费（SettingsService Load/Save、App.GetSettings 降级）。
- 清档链：`ApplyThumbsSettingsChange` → `diffThumbsSettings` → `resetThumbTier` → `reenqueueThumbsAfterTierReset`，受 Auto 开关与 COM 可用性短路。
- 实测数据：本机库 `004912a8d4346dd2`，1281 页；small 全量 1281 张，hd 972 张（76%）；small 均值 33.5 KB/张。
- 体积预估：640 px 单张约 100 KB，1281 页约 128 MB，1024 MB 配额充足。
