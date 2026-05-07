## ADDED Requirements

### Requirement: 显示 Git 仓库远程地址
系统 SHALL 在右侧区域显示 Git 仓库的远程地址（origin URL），以便用户了解仓库的远程位置。

#### Scenario: 显示 HTTP/HTTPS 远程地址
- **WHEN** 用户点击 Git 仓库节点
- **THEN** 系统应在右侧区域显示远程仓库地址
- **AND** 如果远程地址是 HTTP/HTTPS 格式（如 https://github.com/user/repo.git）
- **AND** 地址应显示为可点击的链接

#### Scenario: 显示 SSH 远程地址
- **WHEN** 用户点击 Git 仓库节点
- **THEN** 系统应显示远程仓库地址
- **AND** 如果远程地址是 SSH 格式（如 git@github.com:user/repo.git）
- **AND** 地址应显示为纯文本（不可点击）

#### Scenario: 没有配置远程地址
- **WHEN** 用户点击 Git 仓库节点
- **AND** 该仓库未配置远程地址
- **THEN** 系统应显示"未配置远程地址"或类似提示信息
- **AND** 提示信息应使用灰色或较淡的颜色

### Requirement: 显示 Git 仓库当前分支
系统 SHALL 在右侧区域显示 Git 仓库的当前分支名称，以便用户了解当前工作分支。

#### Scenario: 显示主分支名称
- **WHEN** 用户点击 Git 仓库节点
- **AND** 当前分支是 main 或 master
- **THEN** 系统应显示分支名称（如 "main" 或 "master"）
- **AND** 分支名称应使用标签（Tag）组件展示，颜色为主题蓝色

#### Scenario: 显示功能分支名称
- **WHEN** 用户点击 Git 仓库节点
- **AND** 当前分支是功能分支（如 feature/new-ui）
- **THEN** 系统应显示完整的分支名称
- **AND** 分支名称应使用标签组件展示，颜色为绿色或橙色

#### Scenario: 显示分离头指针状态
- **WHEN** 用户点击 Git 仓库节点
- **AND** 仓库处于分离头指针（HEAD detached）状态
- **THEN** 系统应显示"分离头指针"或类似警告信息
- **AND** 提示信息应使用红色或警告颜色

### Requirement: 显示 Git 仓库最新提交信息
系统 SHALL 在右侧区域显示最新提交的关键信息，包括 SHA、作者、时间和消息。

#### Scenario: 显示最新提交 SHA
- **WHEN** 用户点击 Git 仓库节点
- **THEN** 系统应显示最新提交的完整 SHA（40 位十六进制字符串）
- **AND** SHA 应显示为等宽字体（monospace）
- **AND** SHA 应支持点击复制功能

#### Scenario: 显示最新提交作者
- **WHEN** 用户点击 Git 仓库节点
- **THEN** 系统应显示最新提交的作者名称和邮箱
- **AND** 格式应为"名称 <邮箱>"（如 "张三 <zhangsan@example.com>"）

#### Scenario: 显示最新提交时间
- **WHEN** 用户点击 Git 仓库节点
- **THEN** 系统应显示最新提交的时间
- **AND** 时间格式应为相对时间（如"2 小时前"、"3 天前"）
- **AND** 如果时间超过 30 天，应显示绝对日期（如"2025-03-15"）

#### Scenario: 显示最新提交消息
- **WHEN** 用户点击 Git 仓库节点
- **THEN** 系统应显示最新提交的完整消息
- **AND** 如果消息是多行的，应显示所有行
- **AND** 消息应保持原始格式和换行

### Requirement: Git 信息使用结构化布局展示
系统 SHALL 使用结构化的布局组件（如 el-descriptions）展示 Git 信息，以确保信息清晰易读。

#### Scenario: 使用键值对布局展示
- **WHEN** 用户查看 Git 信息区域
- **THEN** 系统应使用键值对布局展示信息
- **AND** 每个信息项应包含标签（Label）和内容（Value）
- **AND** 标签应显示在左侧，内容显示在右侧

#### Scenario: 支持复制远程地址
- **WHEN** 用户点击远程地址字段
- **AND** 远程地址是 HTTP/HTTPS 格式
- **THEN** 系统应在浏览器中打开该地址
- **AND** 如果是 SSH 格式，系统应复制地址到剪贴板

#### Scenario: 支持复制提交 SHA
- **WHEN** 用户点击提交 SHA 字段
- **THEN** 系统应复制完整的 SHA 到剪贴板
- **AND** 系统应显示"已复制"提示信息

### Requirement: Git 信息区域使用卡片式布局
系统 SHALL 将 Git 信息展示在独立的卡片组件中，以与其他内容区域区分。

#### Scenario: Git 信息卡片标题
- **WHEN** 用户查看 Git 信息区域
- **THEN** 系统应显示卡片标题，如"Git 仓库信息"或"Git Information"
- **AND** 标题应清晰明确，字体稍大

#### Scenario: Git 信息卡片样式
- **WHEN** 用户查看 Git 信息区域
- **THEN** 卡片应具有轻微的阴影和圆角
- **AND** 卡片背景色应为白色或浅灰色
- **AND** 卡片应有适当的内边距（padding）

#### Scenario: Git 信息仅在 Git 仓库中显示
- **WHEN** 用户点击非 Git 仓库的文件夹或文件节点
- **THEN** 系统不应显示 Git 信息卡片
- **AND** 右侧区域应显示其他相关内容（如文件操作或文件夹操作）

### Requirement: Git 信息加载状态反馈
系统 SHALL 在加载 Git 信息时提供加载状态反馈，以改善用户体验。

#### Scenario: 显示加载中状态
- **WHEN** 用户点击 Git 仓库节点
- **AND** Git 信息正在加载中
- **THEN** 系统应在 Git 信息区域显示加载指示器（如 loading spinner 或骨架屏）
- **AND** 加载指示器应居中显示

#### Scenario: 加载失败显示错误信息
- **WHEN** 系统尝试加载 Git 信息
- **AND** 加载过程失败（如网络错误、权限问题）
- **THEN** 系统应显示错误提示信息
- **AND** 错误信息应说明失败原因（如"无法读取 Git 信息"）
- **AND** 错误信息应提供重试按钮

### Requirement: Git 信息支持刷新操作
系统 SHALL 允许用户手动刷新 Git 信息，以获取最新的仓库状态。

#### Scenario: 提供刷新按钮
- **WHEN** 用户查看 Git 信息区域
- **THEN** 系统应在卡片右上角显示刷新按钮
- **AND** 刷新按钮应使用刷新图标（如 el-icon-refresh）

#### Scenario: 点击刷新按钮更新信息
- **WHEN** 用户点击刷新按钮
- **THEN** 系统应重新从后端获取最新的 Git 信息
- **AND** 刷新过程中应显示加载状态
- **AND** 刷新完成后应更新显示的信息
