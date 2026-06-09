# Changelog

## [Unreleased]

### Added
- 文件预览编辑 — 预览文本文件时可就地编辑，保存/取消，切换文件时自动检查未保存修改
- 自定义快捷键 — 设置面板管理快捷键，录制新组合键，右键菜单显示提示，单个/全部重置
- 内容搜索 — 基于 ripgrep 的文件内容搜索，支持扩展名过滤、排除规则、指定目录搜索
- 文件树右键菜单新增"更新仓库"操作（与文件树同名功能一致）

### Fixed
- 修复快捷键录制无法捕获按键的问题（绑定 keydown、聚焦容器、阻止冒泡）
- 修复 useShortcuts 解构 DEFAULTS 导致的 ReferenceError
- 修复搜索结果面板滚动条位置和内容搜索状态清理

## [1.0.3] - 2026-05-24

### Added
- 工作目录树右键菜单新增"在资源管理器中打开"、"用 VSCode 打开"、"用 Warp 打开"操作

### Fixed
- 修复文件树右键菜单被内容面板覆盖的问题（移除 splitpanes 面板的 z-index 干扰）

## [1.0.0] - 2026-04-30

### Added
- File tree styling with icons and color themes
- Git repository information display component
- Commit history viewer with timeline layout
- Search and filter functionality for commits
- Pagination for commit history (20 per page)
- Copy-to-clipboard for commit SHA and remote URLs
- Collapse all button for file tree
- Refresh buttons for Git info and commit history
- Simple in-memory cache mechanism (5-min expiration)

### Changed
- Improved visual hierarchy with color-coded nodes
- Enhanced hover effects on tree nodes
- Better error handling and user feedback

### Fixed
- Removed expand all button as requested
- Fixed Wails binding import paths for components
