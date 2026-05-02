## ADDED Requirements

### Requirement: Git仓库信息获取
系统应获取并显示Git仓库的基本信息。

#### Scenario: 成功获取仓库信息
- **WHEN** 用户选择一个Git仓库目录
- **THEN** 系统执行git命令获取仓库信息
- **AND** 系统返回以下信息：
  - 当前分支名称
  - 远程仓库列表（如 origin）
  - 远程仓库URL
  - 最新提交信息（可选）

#### Scenario: 获取无远程的仓库信息
- **WHEN** 用户选择一个没有配置远程的本地Git仓库
- **THEN** 系统返回基本信息（分支、本地提交）
- **AND** 系统标记远程信息为"未配置"

#### Scenario: 获取非Git目录信息
- **WHEN** 用户尝试获取非Git目录的仓库信息
- **THEN** 系统返回错误提示"不是Git仓库"

#### Scenario: Git命令执行失败
- **WHEN** Git命令执行失败（如Git未安装或仓库损坏）
- **THEN** 系统返回错误提示"无法获取仓库信息"
- **AND** 系统显示详细错误原因

### Requirement: Git仓库克隆
系统应允许用户克隆远程Git仓库到本地。

#### Scenario: 成功克隆仓库
- **WHEN** 用户在选中目录下输入有效的GitHub URL
- **AND** 目标路径不存在同名目录
- **THEN** 系统执行git clone命令
- **AND** 系统显示克隆进度（可选）
- **AND** 克隆完成后系统刷新文件树
- **AND** 系统显示成功提示

#### Scenario: 克隆到指定目录
- **WHEN** 用户在目录A中克隆仓库 https://github.com/user/repo.git
- **THEN** 系统在目录A下创建repo文件夹
- **AND** 系统将仓库内容克隆到 A/repo/ 路径

#### Scenario: 目标目录已存在（普通目录）
- **WHEN** 克隆目标位置已存在同名普通目录
- **THEN** 系统中止克隆操作
- **AND** 系统显示错误提示"目录已存在，无法克隆"

#### Scenario: 目标目录已存在（Git仓库）
- **WHEN** 克隆目标位置已存在同名Git仓库
- **THEN** 系统中止克隆操作
- **AND** 系统显示错误提示"Git仓库已存在"

#### Scenario: 克隆URL无效
- **WHEN** 用户输入的Git URL格式错误或不存在
- **THEN** 系统显示错误提示"无效的Git仓库URL"
- **AND** 系统不创建目录

#### Scenario: 克隆网络错误
- **WHEN** 克隆过程中网络连接失败
- **THEN** 系统显示错误提示"网络连接失败，请检查网络"
- **AND** 系统清理已创建的不完整目录

#### Scenario: 提取仓库名称
- **WHEN** 用户输入的URL为 https://github.com/user/project-name.git
- **THEN** 系统提取仓库名称为 "project-name"
- **AND** 系统在目标目录下创建 project-name 文件夹

#### Scenario: SSH URL克隆
- **WHEN** 用户输入SSH格式的URL（如 git@github.com:user/repo.git）
- **THEN** 系统执行克隆（依赖系统SSH密钥配置）
- **AND** 如果认证失败，系统显示错误提示"认证失败，请检查SSH密钥配置"

### Requirement: Git拉取更新
系统应允许用户从远程仓库拉取最新代码。

#### Scenario: 成功拉取更新
- **WHEN** 用户在Git仓库目录中点击"拉取更新"按钮
- **AND** 远程仓库有新提交
- **THEN** 系统执行git pull命令
- **AND** 系统显示拉取成功提示
- **AND** 系统刷新提交历史

#### Scenario: 拉取无更新
- **WHEN** 用户执行拉取操作
- **AND** 远程仓库没有新提交
- **THEN** 系统显示提示"已经是最新"

#### Scenario: 拉取冲突
- **WHEN** 拉取过程中产生代码冲突
- **THEN** 系统显示错误提示"拉取产生冲突，请手动解决"
- **AND** 系统不自动合并冲突

#### Scenario: 拉取网络错误
- **WHEN** 拉取过程中网络连接失败
- **THEN** 系统显示错误提示"网络连接失败"

#### Scenario: 拉取时本地有未提交更改
- **WHEN** 用户本地有未提交的更改
- **THEN** 系统执行git pull（Git会自动合并或提示冲突）
- **AND** 系统显示操作结果

#### Scenario: 无远程仓库
- **WHEN** 用户在无远程配置的仓库中执行拉取
- **THEN** 系统显示错误提示"未配置远程仓库"

### Requirement: Git提交历史查看
系统应允许用户查看Git提交历史，支持分页加载。

#### Scenario: 成功获取提交历史
- **WHEN** 用户查看Git仓库的提交历史
- **THEN** 系统返回最近10条提交记录（默认页大小10）
- **AND** 每条记录包含：
  - 提交hash（短格式，前7位）
  - 作者名称
  - 提交日期时间
  - 提交信息

#### Scenario: 分页加载提交历史
- **WHEN** 用户点击"下一页"
- **THEN** 系统加载下一页提交记录（跳过前10条，取接下来10条）
- **AND** 系统更新显示的提交列表

#### Scenario: 第一页提交历史
- **WHEN** 用户查看第1页，每页10条
- **THEN** 系统执行 git log --skip=0 -n=10

#### Scenario: 第二页提交历史
- **WHEN** 用户查看第2页，每页10条
- **THEN** 系统执行 git log --skip=10 -n=10

#### Scenario: 提交历史最后一页
- **WHEN** 用户浏览到最后一页
- **THEN** 系统显示剩余的提交记录（可能不足10条）
- **AND** "下一页"按钮禁用

#### Scenario: 提交总数统计
- **WHEN** 系统加载提交历史
- **THEN** 系统同时返回总提交数量
- **AND** 系统计算总页数
- **AND** 界面显示"当前页/总页数"

#### Scenario: 自定义页大小
- **WHEN** 用户选择每页显示20条（如果提供该选项）
- **THEN** 系统按新的页大小加载数据
- **AND** 系统重新计算总页数

#### Scenario: 空仓库
- **WHEN** Git仓库没有任何提交
- **THEN** 系统返回空提交列表
- **AND** 系统显示提示"暂无提交记录"

### Requirement: Git命令执行
系统应通过ProcessBuilder执行系统Git命令。

#### Scenario: 执行简单Git命令
- **WHEN** 系统需要执行 git status
- **THEN** 系统使用ProcessBuilder构造命令
- **AND** 系统设置工作目录为指定Git仓库路径
- **AND** 系统执行命令并解析输出

#### Scenario: 执行带参数的Git命令
- **WHEN** 系统需要执行 git log --oneline -10
- **THEN** 系统构造完整的命令参数数组
- **AND** 系统执行命令并解析输出

#### Scenario: Git命令超时
- **WHEN** Git命令执行时间超过预期（如克隆大型仓库）
- **THEN** 系统设置合理的超时时间（如30分钟）
- **AND** 如果超时，系统终止进程并返回错误提示

#### Scenario: Git命令返回非零退出码
- **WHEN** Git命令执行失败，返回非零退出码
- **THEN** 系统捕获错误流（stderr）
- **AND** 系统解析错误信息并返回给用户

### Requirement: Git认证
系统应依赖系统Git的认证配置。

#### Scenario: HTTPS克隆使用凭据助手
- **WHEN** 用户克隆HTTPS URL
- **AND** 系统Git配置了凭据助手（如Windows凭据管理器）
- **THEN** Git自动使用保存的凭据
- **AND** 工具不处理认证逻辑

#### Scenario: SSH克隆使用密钥
- **WHEN** 用户克隆SSH URL（如 git@github.com:user/repo.git）
- **AND** 系统配置了SSH密钥
- **THEN** Git自动使用SSH密钥认证
- **AND** 工具不处理密钥逻辑

#### Scenario: 认证失败
- **WHEN** Git认证失败（无凭据或密钥）
- **THEN** 系统显示Git返回的错误信息
- **AND** 系统提示用户配置Git认证

### Requirement: Git仓库状态检测
系统应检测目录是否为Git仓库。

#### Scenario: 检测标准Git仓库
- **WHEN** 目录包含.git子文件夹
- **THEN** 系统判定该目录为Git仓库
- **AND** 系统在文件树中标记该目录

#### Scenario: 检测裸仓库
- **WHEN** 目录本身是Git裸仓库（bare repository）
- **THEN** 系统判定该目录为Git仓库
- **AND** 系统标记该目录为Git裸仓库（可选）

#### Scenario: 检测Git子模块
- **WHEN** 目录包含.git文件（而不是文件夹， Git子模块特性）
- **THEN** 系统判定该目录为Git仓库（子模块）
- **AND** 系统可以标记为Git子模块（可选）

#### Scenario: 检测非Git目录
- **WHEN** 目录不包含.git文件夹或文件
- **THEN** 系统判定该目录不是Git仓库
- **AND** 系统不显示Git相关操作

### Requirement: Git操作错误处理
系统应提供清晰的Git操作错误提示。

#### Scenario: Git未安装
- **WHEN** 系统执行Git命令时发现Git未安装
- **THEN** 系统返回错误提示"系统未安装Git，请先安装Git"

#### Scenario: 仓库损坏
- **WHEN** Git仓库损坏或处于异常状态
- **THEN** 系统返回错误提示"Git仓库损坏"

#### Scenario: 分支不存在
- **WHEN** 用户操作的分支不存在
- **THEN** 系统返回错误提示"分支不存在"

#### Scenario: 远程仓库不存在
- **WHEN** 用户拉取时远程仓库已被删除
- **THEN** 系统返回错误提示"远程仓库不存在"

### Requirement: Git操作并发控制
系统应避免多个Git操作同时执行导致冲突。

#### Scenario: 克隆中禁止其他操作
- **WHEN** 用户正在克隆仓库
- **THEN** 系统禁用克隆和拉取按钮
- **AND** 系统显示操作进行中提示
- **AND** 克隆完成后恢复按钮状态

#### Scenario: 拉取中禁止重复拉取
- **WHEN** 用户正在拉取更新
- **AND** 用户再次点击拉取按钮
- **THEN** 系统忽略重复请求
- **AND** 系统显示"正在拉取中"提示

### Requirement: Git信息面板展示
系统应在右侧面板清晰展示Git信息。

#### Scenario: Git仓库信息面板布局
- **WHEN** 用户选中Git仓库目录
- **THEN** 右侧面板显示：
  - 仓库路径
  - 分支信息
  - 远程仓库信息
  - 拉取按钮
  - 提交历史列表

#### Scenario: 非Git目录面板
- **WHEN** 用户选中非Git目录
- **THEN** 右侧面板不显示Git相关信息
- **AND** 面板显示克隆新仓库的表单

#### Scenario: 提交历史显示格式
- **WHEN** 系统显示提交历史
- **THEN** 每条提交显示为：
  - Hash（短格式，如 abc1234）
  - 提交信息（如 修复登录bug）
  - 作者和日期（如 刘阳 2025-04-28 10:30）
