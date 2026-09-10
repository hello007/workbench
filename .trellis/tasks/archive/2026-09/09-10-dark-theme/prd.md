# 暗色主题支持

## Goal

为 WorkBench 实现暗色主题:系统主题自动跟随、手动切换亮/暗、配置持久化。一次到位覆盖工作目录树/文件树/导航中心/终端(xterm)/AI 功能面板/markdown 预览全场景,与现有 CSS 变量体系整合。

## What I already know

### 现有 CSS 变量体系(`frontend/src/style.css`,356 行)

`:root` 已定义完整变量集:主色调/辅助色/背景色(三档)/侧边栏(已是深色 `#1e2a3a`)/右键菜单/边框/文字(四档)/阴影/间距/圆角。组件用 `var()` 引用则暗色仅需覆盖变量集。

### Element Plus 暗色接入

`frontend/src/main.js` 标准接入 `element-plus/dist/index.css`。暗色追加 `element-plus/theme-chalk/dark/css-vars.css` + `<html>` 加 class `dark`。

### 持久化范式

`frontend/src/store/settings.js` Pinia setup store,`GetSettings`/`SaveSettings` 读写后端 `settings.json`(通用 JSON,加字段不改后端方法签名)。

## Decision (ADR-lite)

**Context**: 暗色覆盖范围与侧边栏处理需定 MVP 边界。

**Decision**: 一次到位——MVP 全覆盖含终端 xterm + markdown/mermaid 暗色变体;暗色模式侧边栏更深统一(`#0f1620` 级),保持与暗色背景层次。亮色模式侧边栏深色现状保留。

**Consequences**: 工作量增(xterm theme 切换 + markdown dark 容器样式),但避免暗色残缺体验;后续无补丁轮次。

## Requirements

1. 主题模式三态:system(系统跟随)/ light / dark,默认 system
2. `main.js` 追加 `import 'element-plus/theme-chalk/dark/css-vars.css'`
3. `style.css` 增 `html.dark :root { ... }` 覆盖全部变量为暗色值(含侧边栏更深)
4. settings store 加 `themeMode` + `resolvedTheme` computed(system 模式经 `matchMedia('(prefers-color-scheme: dark)')` 解析并监听变化)
5. `App.vue` watch `resolvedTheme` → toggle `document.documentElement.classList` 的 `dark` class,无刷新生效
6. 设置面板「通用」加主题单选(system/light/dark),保存即生效 + 持久化
7. 终端 xterm 主题随 `resolvedTheme` 切换(dark theme object)
8. markdown/mermaid 预览容器加 `.dark` 变体样式
9. settings.json schema 兼容(新字段不破坏旧配置,缺省回 system)

## Acceptance Criteria

* [ ] system 模式下系统主题切换,应用自动跟随(无刷新)
* [ ] 手动切 light/dark 立即生效,Element Plus 组件 + 自定义 CSS 变量同步
* [ ] 主题配置持久化,重启保留
* [ ] 终端 xterm 在暗色下主题正确(背景/文字/光标色)
* [ ] markdown/mermaid 预览暗色下无亮色残留
* [ ] 侧边栏暗色更深统一,与背景层次清晰
* [ ] 旧 settings.json(无 themeMode)加载不报错,默认 system
* [ ] 前端测试覆盖主题切换逻辑(store + resolvedTheme + class toggle)

## Definition of Done

* 前端测试覆盖率 ≥70%(exclude wailsjs)硬门禁过
* 暗色各区域视觉无残缺
* settings.json 向后兼容
* 文档更新:功能说明加主题项、路线图补勾选

## Out of Scope

* 主题色自定义(仅三态,不开放调色)
* 第三方主题包导入

## Technical Approach

1. `main.js`:`import 'element-plus/theme-chalk/dark/css-vars.css'`
2. `style.css`:增 `html.dark :root { --bg-primary: #1d1e1f; --bg-secondary: #252526; ... --sidebar-bg: #0f1620; ... }`(暗色全套变量)
3. settings store:加 `themeMode` ref('system') + `resolvedTheme` computed;`matchMedia` 监听器(系统模式);load/save 合并到现有 GetSettings/SaveSettings
4. `App.vue`:`watch(() => settings.resolvedTheme, ...)` → `document.documentElement.classList.toggle('dark', isDark)`
5. 设置面板「通用」:el-radio-group(system/light/dark)→ settings.themeMode + save
6. 终端:useTerminal composable 内 xterm `setOption('theme', isDark ? DARK_THEME : LIGHT_THEME)` watch resolvedTheme
7. markdown 预览:容器加 `.dark` 类(或依赖 `html.dark` 级联),mermaid 主题参数随暗色切 init 主题

## Technical Notes

* `frontend/src/style.css:2` `:root` 变量集(覆盖点)
* `frontend/src/main.js:4` ElementPlus 接入
* `frontend/src/store/settings.js` GetSettings/SaveSettings 范式(通用 JSON,加字段不触发跨层同步)
* `frontend/src/store/ui.js` 终端三件套(visible/height/dir)所在 store,主题切换接入点
* 侧边栏亮色已深色 `--sidebar-bg: #1e2a3a`
* 跨层契约参考:[docs/spec/cross-layer-contracts.md](../../../docs/spec/cross-layer-contracts.md)(themeMode 走现有通用 settings 则不触发;若新增后端方法须同步 wailsjs 三处)
