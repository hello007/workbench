# 「拷贝到」支持自定义目标文件名

## Goal

「拷贝到」功能（FileTreePanel 右键菜单 + ToolboxPanel 工具箱）目前只能指定目标目录，落盘文件名固定等于源文件名。本任务为其增加自定义目标文件名能力：默认与原名一致，可修改。附带改进：拷贝后自动移除目标文件继承的只读属性。

## Requirements

* **覆盖两处入口**：FileTreePanel 右键「拷贝到」+ ToolboxPanel 工具箱「拷贝到」
* 对话框增加目标文件名输入框，默认值 = 源路径最后一段，可修改
  * FileTreePanel：文件与目录（含文件夹本身）均支持自定义目标名，输入框始终显示
  * ToolboxPanel：源路径手填无类型信息，输入框始终显示，默认取源路径最后一段
* 拷贝效果预览实时反映自定义文件名
* **「互换」交互**：互换原/目标地址时，文件名输入框重置为新源路径最后一段（默认值），用户可再改
* **后端 API**：扩展现有 `App.CopyTo` / `FileOperationService.CopyTo` 签名，新增第 4 参 `targetName string`；空串 = 取源名（原行为），非空 = 自定义名；Go 侧空串兜底，旧语义零破坏
* **目录源自定义名语义**：文件源、或目录源 + `copyWholeDir=true`（包含文件夹本身）时 targetName 生效，目标以 targetName 命名落盘（冲突沿用 `findAvailableName` 自动加 (1)）；目录源 + `copyWholeDir=false`（仅拷贝目录内容）时无「目录名」概念，targetName 被忽略且不提示
* **冲突策略**：沿用 `findAvailableName` 自动加 (1)、(2)，与现状一致
* **只读属性处理**：在 `util.CopyFile` / `util.CopyDir` 层统一生效，所有复制路径（「拷贝到」、右键复制粘贴 CopyItem）拷出的目标一律去只读。现状 `util.CopyFile` 以 `info.Mode()` 创建目标文件（util/file.go:118），源只读则目标也只读；改为拷贝完成后对目标 `os.Chmod` 去除只读位（保留其余权限位），`CopyDir` 递归同理（MkdirAll 同样传了 info.Mode()，目录只读位一并去除）
* 前端校验：自定义名含 `/` `\` `:` `*` `?` `"` `<` `>` `|` 等非法字符或为 `.`/`..` 时警告提示，禁止提交
* 前端校验：自定义名含 `/` `\` `:` `*` `?` `"` `<` `>` `|` 等非法字符时警告提示，禁止提交

## Acceptance Criteria

* [ ] FileTreePanel 右键文件 → 拷贝到 → 对话框出现文件名输入框，默认原文件名
* [ ] FileTreePanel 右键文件夹 → 拷贝到 → 同样显示文件名输入框（默认源目录名，勾选「包含文件夹本身」时自定义目录名生效）
* [ ] ToolboxPanel 拷贝到 → 始终显示文件名输入框，默认取源路径最后一段

* [ ] 修改文件名后预览路径随之变化
* [ ] 修改名后拷贝，目标文件名为修改后的名字
* [ ] 目录源 + 包含文件夹本身 + 改名 → 目标目录以自定义名落盘，冲突自动加 (1)
* [ ] 目录源 + 仅拷贝内容 + 改名 → targetName 被忽略，内容按原语义拷入目标目录
* [ ] 源文件只读 → 拷贝后目标文件可写（只读属性已移除）
* [ ] 源目录内含只读文件 → 递归拷贝后目标内对应文件可写
* [ ] 不修改文件名拷贝，行为与现状完全一致
* [ ] 自定义名含非法字符时给出提示且不执行拷贝
* [ ] 同名冲突自动加 (1)，与现状一致

## Definition of Done

* 后端 `go test ./...` 通过
* 前端 `npm test`（Vitest）通过，Home.spec.js 中 CopyTo 相关用例更新
* wailsjs 绑定重新生成（`wails generate module` 或 dev/build 触发）
* docs/功能说明.md 更新「拷贝到」说明（自定义文件名 + 只读移除）
* README.md 确认是否需要更新

## Technical Approach

1. **后端 service 层**：`FileOperationService.CopyTo` 加 `targetName` 参数；文件源且 targetName 非空时，拼目标路径用 targetName 替代 `filepath.Base(sourcePath)`，再走 `CopyItem` 同款 `findAvailableName`（需抽公共路径或新增内部方法）；目录源 + copyWholeDir=true + targetName 非空时同款逻辑（targetName 拼目标路径 + findAvailableName）；目录源 + copyWholeDir=false 时 targetName 忽略且不提示
2. **后端 app 层**：`App.CopyTo` 透传 targetName
3. **util 层**：`CopyFile` 拷完 `os.Chmod(dst, info.Mode().Perm()&^0444)`；`CopyDir` 对目录与递归文件同样处理
4. **前端 FileTreePanel**：新增 `copyToTargetName` ref；showCopyToDialog 初始化默认名（文件/目录均取源路径最后一段）；preview computed 用自定义名；swapCopyToPaths 重置文件名；handleCopyTo 校验非法字符后随 emit 传 targetName
5. **前端 ToolboxPanel**：同款输入框（始终显示）+ 默认值跟随源路径；handleCopyTo 传 targetName，toast 展示后端返回串
6. **Home.vue**：copyTo 事件处理透传 targetName 到 `CopyTo` 调用

## Decision (ADR-lite)

**Context**: 目标文件名固定等于源名，无法在拷贝时重命名；需选择 API 演进方式与各边界行为
**Decision**:
* 扩展现有 `CopyTo` 签名加 `targetName`（空串兜底），而非新增方法
* 文件与目录（含文件夹本身，即 copyWholeDir=true）均支持自定义名；目录仅拷内容（copyWholeDir=false）时忽略且不提示
* 只读移除下沉 `util.CopyFile`/`CopyDir` 统一生效
* 冲突沿用自动加 (1)
**Consequences**: wailsjs 绑定需重新生成，前端两处调用与 Home.spec.js 同步改；复制粘贴（CopyItem）行为变化——只读源拷出不再只读，属预期改进

## Out of Scope

* 移动（MoveItem）同类能力
* 拷贝覆盖确认对话框（沿用自动重命名）

## Technical Notes

* 相关文件：
  - frontend/src/components/FileTreePanel.vue（对话框 UI :129-178 + preview computed :457-468 + swapCopyToPaths :471 + handleCopyTo :1001）
  - frontend/src/components/ToolboxPanel.vue（独立对话框 :32-73 + handleCopyTo :124）
  - frontend/src/views/Home.vue（copyTo 事件处理 → 后端调用）
  - app.go:911 CopyTo（Wails 绑定）
  - service/fileoperation.go:516 CopyTo / :468 CopyItem / :447 findAvailableName
  - util/file.go:106 CopyFile / :129 CopyDir
* 签名变更需重新生成 wailsjs 绑定（frontend/wailsjs/go/main/App.js:61）
* 前端测试：frontend/src/views/__tests__/Home.spec.js 有 CopyTo 相关 mock
