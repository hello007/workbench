# 外部 diff 工具审核修复

## Goal

修复 09-13-diff 交付后的双路代码审核发现的 1 bug + 10 risk + 1 nit。全部为边界加固与错误语义收紧，不改功能面。

## Requirements（审核发现 → 修复）

### 后端（539069f）

1. **[bug/安全] SHA 入参校验**：`OpenInExternalDiff` 的 sha/baseSha/headSha 未校验即拼入 `git show` 参数，`-` 开头值被 git 当选项（`--output=` 任意写文件），方法暴露给前端 → commit/range 分支入参先过 `^[0-9a-fA-F]{7,64}$` 校验，非法返回普通错误
2. **git show 失败语义区分**：`resolveExternalDiffSides` 中 gitShow 失败一律吞为空左侧——超时/锁/对象损坏被伪装成「新增文件」假 diff → 仅错误信息含 "does not exist"（缺失语义）才降级空内容，否则上抛
3. **os.Stat 收紧**：workspace 右侧 `os.Stat` 任何失败都按「已删除」降级 → 仅 `errors.Is(err, fs.ErrNotExist)` 降级
4. **读设置失败错误码**：`app_external.go` 读设置 IO 失败映射为 `NOT_CONFIGURED` 误导排障 → 改普通 `fmt.Errorf`
5. **临时目录原子唯一**：UnixNano 目录名 Windows 粒度 ~0.5ms 可撞名互覆 → 改 `os.MkdirTemp(DiffTempRoot(), "")`
6. **util 测试根注入**：测试读写真实 `%TEMP%\workbench-diff` 会删掉运行中应用的在用文件 → 根目录可注入，测试指向 `t.TempDir()`

### 前端（87f6db9）

7. **加载失败残留清空路径**：`loadSettings` 失败被吞，store 留默认 `''`，此后 `onSettingsChange`/`onGpuChange` 全量覆盖写静默清空磁盘 diffTool 配置 → 两处全量写改合并写（GetSettings 为基底覆盖面板管理字段），diffTool/themeMode 键不再由这两处覆盖（各走 saveDiffTool/saveTheme 合并写）
8. **range 模式 header 按钮**：`props.file` 空点击静默 no-op → disabled 追加 `mode === 'range'`
9. **预设切换覆盖保护**：已有非默认自定义值时先 `ElMessageBox.confirm`，取消恢复旧选中；模板改 `:model-value` 手动赋值
10. **保存失败提示**：`onDiffToolChange`/`onDiffToolPresetChange` 补 try/catch + ElMessage.error（对齐 onThemeChange）
11. **loadDiff 并发守卫**：请求序号，过期响应丢弃（含 isBinaryFile/left/right/fileGroups 全部状态写入与 loading 复位）
12. **diffToolConfigured 补 args 校验**：与后端对称——args 非空且含 {left}{right} 占位符

## Acceptance Criteria

* [ ] 后端：非法 SHA 拒绝用例、临时目录唯一性用例、注入根隔离用例；go test 全绿
* [ ] 前端：range header disabled、预设取消恢复、保存失败提示、竞态守卫、configured args 用例；896+ 全绿
* [ ] 受影响既有用例同步更新（合并写后 diffTool 断言、E2E SaveSettings 字段断言）
* [ ] npm run build / E2E 全量 / 覆盖率门禁零回归

## Out of Scope

* hasParent 瞬时失败误判 root commit（本地 rev-parse 失败面极窄，避免波及 GetCommitFileDiff，注释说明即可）
