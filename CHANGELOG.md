# Changelog

## [Unreleased]

### Added
- 工作目录树右键菜单新增"更新仓库"操作（与文件树同名功能一致）

### Fixed
- 修复文件树右键菜单被内容面板覆盖的问题（移除 splitpanes 面板的 z-index 干扰）

## [1.0.2] - 2026-05-23

### Added
- 工作目录树右键菜单新增"在资源管理器中打开"、"用 VSCode 打开"、"用 Warp 打开"操作

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
