# 网络访问：绑定地址快捷切换 + 带 token 链接复制

## Goal

设置页「网络访问」分区两个易用性增强：

1. **绑定地址快捷切换**：分段按钮「本地（127.0.0.1）／公网（0.0.0.0）」一键切换，替代手输地址；自定义地址输入保留（高级用法）。
2. **复制带 token 完整链接**：访问地址每项可一键复制 `http://<addr>:<port>/?token=<t>`（如 `http://127.0.0.1:36115/?token=b553...`），粘贴到任何浏览器即用。

## Requirements

### 快捷切换

* 分段控件（Element Plus el-radio-group 按钮态或 el-segmented，按现有设置页风格）两档：「本地 127.0.0.1」／「公网 0.0.0.0」，端口沿用当前 bindAddress 中的端口。
* 切到「公网」→ 走现有非回环风险确认 ElMessageBox（文案复用），确认后保存 `0.0.0.0:<port>`；取消则回弹原状态。
* 切到「本地」→ 直接保存 `127.0.0.1:<port>`（无需确认）。
* 手输自定义地址输入框保留，与分段控件双向联动（输入自定义值时分段控件置空/高亮「自定义」态；分段点击时回填输入框）。
* 保存链路复用现有 `SetWebServeConfig`（运行时重启已具备），无后端改动。

### 复制带 token 链接

* 「访问地址」展示区（GetWebServeConfig 返回的候选地址列表：127.0.0.1 与局域网 IP）每行加「复制链接」按钮：剪贴板写入 `http://<该地址>:<端口>/?token=<GetWebServeToken 取到的令牌>`。
* **永不生成 `0.0.0.0` 链接**（不可直连）；公网模式下列表自然只含实际 IP 候选（复用现有 webAccessUrls 逻辑，确认其已排除 0.0.0.0）。
* 复制成功轻提示（ElMessage success）；剪贴板 API 用现有 clipboard 封装（service/clipboard.go 对应前端已有复制先例，勘察后复用）。
* 链接含明文 token：按钮旁或复制提示中带一句安全提示（链接即凭据，勿外发）。

## Acceptance Criteria

* [ ] 分段切换本地/公网：地址回填、保存生效（服务按新地址重启）、公网走风险确认、取消回弹
* [ ] 自定义输入与分段控件联动正确
* [ ] 每个候选地址行可复制完整带 token 链接，粘贴浏览器可直接进入（含 cookie 种子）
* [ ] 任何情况下不产生 0.0.0.0 链接
* [ ] vitest 覆盖分段切换（含确认/取消/联动）与复制链接拼装（含 0.0.0.0 排除）
* [ ] `npm run build` + `npm test` 绿，前端覆盖率门禁 ≥70%

## Out of Scope

* 新增后端绑定方法（现有 GetWebServeConfig/GetWebServeToken/SetWebServeConfig 足够；若勘察发现候选地址/端口解析确缺字段再补，并走 wailsjs 三处同步）
* 二维码展示（手机扫码访问，留二期归档）

## Technical Notes

* 相关规范：frontend-visual-conventions.md（分段控件/提示风格）、design-tokens.md、e2e-testing.md（wails-mock-defaults.js 若需补形状）
* 勘察点：SettingsPanel.vue 现有网络访问分区结构、webAccessUrls 是否已排除 0.0.0.0、前端复制剪贴板的既有封装（useClipboard 或手写）
