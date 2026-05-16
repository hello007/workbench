---
stepsCompleted:
  - step-01-validate-prerequisites
  - step-02-design-epics
  - step-03-create-stories
  - step-04-final-validation
status: 'complete'
completedAt: '2026-05-16'
inputDocuments:
  - '_bmad-output/planning-artifacts/prd.md'
  - '_bmad-output/planning-artifacts/architecture.md'
  - 'docs/project-context.md'
---

# git-manager - Epic Breakdown

## Overview

This document provides the complete epic and story breakdown for git-manager, decomposing the requirements from the PRD, UX Design if it exists, and Architecture requirements into implementable stories.

## Requirements Inventory

### Functional Requirements

FR1: 用户可以添加本地目录作为工作目录
FR2: 用户可以从列表中移除工作目录
FR3: 用户可以设置默认工作目录（应用启动时自动选中）
FR4: 系统可以将工作目录列表持久化到本地配置文件
FR5: 系统可以验证用户输入的路径是否有效
FR6: 用户可以展开工作目录查看其文件树结构
FR7: 系统按需懒加载文件树子节点（展开时才加载）
FR8: 系统可以自动检测目录是否为 Git 仓库并标识
FR9: 用户可以查看隐藏文件夹（以 `.` 开头的目录，`.git` 除外）
FR10: 用户可以全部展开或收起文件树
FR11: 用户可以选中文件树节点查看其信息
FR12: 用户可以在指定文件夹下创建新文件
FR13: 用户可以在指定文件夹下创建新子文件夹
FR14: 用户可以重命名文件或文件夹
FR15: 用户可以删除文件或文件夹
FR16: 用户可以预览文件内容（文本文件直接渲染，二进制文件提示不可预览）
FR17: 系统可以限制预览文件的大小（超出限制时提示）
FR18: 用户可以通过右键菜单对文件/文件夹执行操作（操作列表根据节点类型区分）
FR19: 用户可以复制文件名或文件完整路径
FR20: 用户可以剪切、复制、粘贴文件和文件夹
FR21: 用户可以查看 Git 仓库的基本信息（当前分支、远程地址、仓库状态）
FR22: 用户可以克隆远程仓库到本地指定路径（支持 HTTPS 和 SSH）
FR23: 用户可以对单个仓库执行 `git pull` 拉取更新
FR24: 用户可以批量拉取多个仓库的更新（系统递归扫描工作目录下的 Git 仓库）
FR25: 系统并行执行批量拉取，实时展示每个仓库的进度（成功/失败/跳过）
FR26: 用户可以查看批量更新的汇总结果（成功数、失败数、失败原因）
FR27: 用户可以分页查看仓库的提交历史
FR28: 用户可以在 VS Code 中打开文件或文件夹
FR29: 用户可以在系统文件资源管理器中打开指定目录
FR30: 用户可以用系统默认程序打开文件
FR31: 应用采用三栏布局（目录列表 + 文件树 + 内容面板）
FR32: 应用可以显示当前版本号
FR33: 应用的所有本地功能可在离线状态下使用（Git 网络操作除外）

### NonFunctional Requirements

NFR1: 应用冷启动时间 < 3 秒
NFR2: 文件树单级目录加载 < 1 秒（1000 个文件以内）
NFR3: 文件预览渲染 < 500 毫秒（1MB 以内文本文件）
NFR4: 单仓库 git pull 超时 30 秒
NFR5: 批量更新并发控制，最多 5 个仓库并行拉取
NFR6: 批量更新 50 个仓库 2 分钟内完成（网络正常）
NFR7: 内存占用空闲 < 150MB，峰值 < 300MB
NFR8: 所有用户输入路径必须规范化处理，防止路径遍历攻击
NFR9: 删除操作需用户二次确认，防止误删

### Additional Requirements

- **棕地项目约束**：所有功能已实现并投产，架构文档记录的是现有架构而非新建项目
- **三层架构边界**：app.go 调度层 → service 业务层 → util 工具层，service 禁止导入 Wails
- **双 Git 引擎**：go-git（读）+ exec.Cmd（写），同一功能内不混用
- **平台约束**：仅 Windows 10/11，exec.Cmd 必须使用 HideCommandWindow
- **前端约束**：纯 JS（无 TypeScript），无新 npm 依赖，Composition API + `<script setup>`
- **数据契约**：Go json 标签 ↔ 前端隐式定义，修改需双向同步
- **事件安全**：EventsOn 必须在 onBeforeUnmount 中 EventsOff
- **构建分发**：`wails build` → `buildAndInstall.sh`，单一二进制文件
- **Starter Template**：无 — 棕地项目，已在 Wails v2 框架上运行

### UX Design Requirements

无独立 UX 设计文档。棕地项目 UI 已实现并验证，三栏布局、文件树、右键菜单、预览面板、剪贴板操作等均已投产。

### FR Coverage Map

FR1  → Epic 1 - 添加工作目录
FR2  → Epic 1 - 移除工作目录
FR3  → Epic 1 - 设置默认工作目录
FR4  → Epic 1 - 工作目录持久化
FR5  → Epic 1 - 路径有效性验证
FR6  → Epic 2 - 展开文件树
FR7  → Epic 2 - 懒加载子节点
FR8  → Epic 2 - Git 仓库检测
FR9  → Epic 2 - 显示隐藏文件夹
FR10 → Epic 2 - 全部展开/收起
FR11 → Epic 2 - 选中节点查看信息
FR12 → Epic 3 - 创建新文件
FR13 → Epic 3 - 创建新子文件夹
FR14 → Epic 3 - 重命名
FR15 → Epic 3 - 删除
FR16 → Epic 3 - 预览文件内容
FR17 → Epic 3 - 限制预览文件大小
FR18 → Epic 3 - 右键菜单操作
FR19 → Epic 3 - 复制文件名/路径
FR20 → Epic 3 - 剪切/复制/粘贴
FR21 → Epic 4 - Git 仓库基本信息
FR22 → Epic 4 - 克隆远程仓库
FR23 → Epic 4 - 单仓库 git pull
FR24 → Epic 4 - 批量拉取
FR25 → Epic 4 - 并行执行+实时进度
FR26 → Epic 4 - 批量汇总结果
FR27 → Epic 4 - 分页提交历史
FR28 → Epic 5 - VS Code 打开
FR29 → Epic 5 - 资源管理器打开
FR30 → Epic 5 - 系统默认程序打开
FR31 → Epic 1 - 三栏布局
FR32 → Epic 1 - 版本号显示
FR33 → Epic 1 - 离线支持

## Epic List

### Epic 1: 应用框架与工作目录管理
用户可以启动应用，看到三栏布局界面，管理工作目录列表（添加、删除、设为默认），应用显示版本号，所有本地功能离线可用。
**FRs covered:** FR1, FR2, FR3, FR4, FR5, FR31, FR32, FR33

### Epic 2: 文件树浏览
用户可以展开工作目录查看文件树，系统按需懒加载子节点，自动检测 Git 仓库，显示隐藏文件夹（`.git` 除外），支持全部展开/收起，选中节点查看信息。
**FRs covered:** FR6, FR7, FR8, FR9, FR10, FR11

### Epic 3: 文件操作与剪贴板
用户可以通过右键菜单创建文件/文件夹、重命名、删除、预览文件内容，复制文件名/路径，以及剪切/复制/粘贴文件和文件夹。
**FRs covered:** FR12, FR13, FR14, FR15, FR16, FR17, FR18, FR19, FR20

### Epic 4: Git 仓库管理
用户可以查看 Git 仓库信息，克隆远程仓库，单仓库/批量拉取更新（并行执行、实时进度），查看批量汇总结果，分页浏览提交历史。
**FRs covered:** FR21, FR22, FR23, FR24, FR25, FR26, FR27

### Epic 5: 外部工具集成
用户可以在 VS Code 中打开文件/文件夹，在系统资源管理器中打开目录，用系统默认程序打开文件。
**FRs covered:** FR28, FR29, FR30

## Epic 1: 应用框架与工作目录管理

### Story 1.1: 三栏布局应用框架

As a 开发者,
I want 应用采用三栏布局（目录列表 + 文件树 + 内容面板）,
So that 我可以在同一界面内管理工作目录、浏览文件和查看内容。

**Acceptance Criteria:**

**Given** 应用启动完成
**When** 主界面加载完毕
**Then** 界面呈现三栏布局：左侧目录列表、中间文件树、右侧内容面板

### Story 1.2: 工作目录添加与移除

As a 开发者,
I want 添加本地目录到工作目录列表，或从中移除,
So that 我可以管理常用的工作目录集合。

**Acceptance Criteria:**

**Given** 用户在目录列表面板中
**When** 用户输入目录名称和路径并确认添加
**Then** 系统验证路径有效性（NFR8: 路径规范化），有效则添加到列表
**And** 无效路径提示错误信息

**Given** 工作目录列表中存在目录
**When** 用户选择移除某个目录
**Then** 目录从列表中移除，持久化配置同步更新（FR4）

### Story 1.3: 默认工作目录与持久化

As a 开发者,
I want 设置默认工作目录，且目录列表持久化保存,
So that 每次启动应用时自动选中常用目录，无需重复配置。

**Acceptance Criteria:**

**Given** 工作目录列表中存在多个目录
**When** 用户将某个目录设为默认
**Then** 该目录标记为默认，下次启动时自动选中

**Given** 用户添加、移除或修改目录
**When** 操作完成
**Then** 变更持久化到 `data/directories.json`（FR4）
**And** 应用重启后列表恢复一致

### Story 1.4: 版本号显示与离线支持

As a 开发者,
I want 应用显示当前版本号，且所有本地功能离线可用,
So that 我知道当前版本，且在无网络环境下仍可使用本地功能。

**Acceptance Criteria:**

**Given** 应用启动完成
**When** 用户查看应用界面
**Then** 界面显示当前版本号（待实现）

**Given** 应用在无网络环境下运行
**When** 用户执行本地操作（文件浏览、文件操作、提交历史查看）
**Then** 所有本地功能正常工作（FR33）
**And** Git 网络操作（clone/pull）失败时给出明确提示，不影响其他功能

## Epic 2: 文件树浏览

### Story 2.1: 文件树懒加载

As a 开发者,
I want 展开工作目录查看文件树，系统按需懒加载子节点,
So that 我可以高效浏览目录结构，不会因一次性加载全部内容而卡顿。

**Acceptance Criteria:**

**Given** 用户选中了一个工作目录
**When** 用户点击目录节点展开
**Then** 系统加载并显示该目录下的直接子项（FR6, FR7）
**And** 单级目录加载时间 < 1 秒（NFR2）
**And** 子节点按需加载，未展开的目录不预读取

### Story 2.2: Git 仓库自动检测

As a 开发者,
I want 系统自动检测目录是否为 Git 仓库并标识,
So that 我可以一眼识别哪些目录是 Git 项目。

**Acceptance Criteria:**

**Given** 文件树加载完成
**When** 某个目录包含 `.git` 子目录
**Then** 该目录节点显示 Git 仓库标识（FR8）
**And** 检测结果缓存，避免重复检测

### Story 2.3: 隐藏文件夹显示

As a 开发者,
I want 查看隐藏文件夹（如 `.claude`、`.vscode`）,
So that 我可以访问和操作配置目录。

**Acceptance Criteria:**

**Given** 文件树加载完成
**When** 目录下存在以 `.` 开头的子目录
**Then** 这些隐藏目录可见并可在文件树中操作（FR9）
**And** `.git` 目录始终不显示

### Story 2.4: 全部展开/收起与节点选中

As a 开发者,
I want 全部展开或收起文件树，以及选中节点查看信息,
So that 我可以快速定位文件或概览目录结构。

**Acceptance Criteria:**

**Given** 文件树已加载
**When** 用户点击"全部展开"按钮
**Then** 文件树递归展开所有节点（FR10，待实现）
**And** 当前该按钮已隐藏，功能尚未实现

**Given** 文件树已加载
**When** 用户点击"全部收起"按钮
**Then** 文件树收起至根节点（FR10）

**Given** 文件树已加载
**When** 用户点击某个文件或文件夹节点
**Then** 节点高亮选中，右侧面板展示节点信息（FR11）

## Epic 3: 文件操作与剪贴板

### Story 3.1: 创建文件和文件夹

As a 开发者,
I want 在指定文件夹下创建新文件或子文件夹,
So that 我可以直接在文件树中快速创建所需资源。

**Acceptance Criteria:**

**Given** 用户右键点击了一个文件夹节点
**When** 选择"新建文件"并输入文件名
**Then** 在该目录下创建指定名称的空文件（FR12）

**Given** 用户右键点击了一个文件夹节点
**When** 选择"新建文件夹"并输入名称
**Then** 在该目录下创建指定名称的子文件夹（FR13）

### Story 3.2: 重命名和删除

As a 开发者,
I want 重命名或删除文件和文件夹,
So that 我可以在文件树中直接管理文件。

**Acceptance Criteria:**

**Given** 用户右键点击了一个文件或文件夹
**When** 选择"重命名"并输入新名称
**Then** 该文件/文件夹更新为新名称（FR14）

**Given** 用户右键点击了一个文件或文件夹
**When** 选择"删除"
**Then** 弹出确认对话框，用户确认后执行删除（FR15）
**And** 删除操作需二次确认（NFR9）

### Story 3.3: 文件预览

As a 开发者,
I want 点击文件预览其内容,
So that 我无需离开应用即可查看文件。

**Acceptance Criteria:**

**Given** 用户点击了一个文本文件
**When** 文件大小 ≤ 1MB
**Then** 右侧面板直接渲染文件内容（FR16）
**And** 预览渲染时间 < 500ms（NFR3）

**Given** 用户点击了一个文件
**When** 文件大小 > 1MB
**Then** 提示"文件过大，无法预览"（FR17）

**Given** 用户点击了一个二进制文件（图片、压缩包等）
**When** 系统判断文件不可预览
**Then** 提示"该文件类型不支持预览"（FR16）

### Story 3.4: 右键菜单系统

As a 开发者,
I want 通过右键菜单对文件/文件夹执行操作,
So that 我可以快速访问所有文件操作。

**Acceptance Criteria:**

**Given** 用户右键点击了文件树中的节点
**When** 菜单弹出
**Then** 根据节点类型（文件/文件夹/Git 仓库）显示不同的操作列表（FR18）

**Given** 右键菜单显示
**When** 节点是文件夹
**Then** 可用操作包含：新建文件、新建文件夹、重命名、删除、在资源管理器打开、在 VS Code 打开、复制路径等

**Given** 右键菜单显示
**When** 节点是文件
**Then** 可用操作包含：重命名、删除、预览、复制路径、在 VS Code 打开、用默认程序打开等

### Story 3.5: 复制文件名和路径

As a 开发者,
I want 复制文件名或文件完整路径到剪贴板,
So that 我可以在其他工具中快速引用文件。

**Acceptance Criteria:**

**Given** 用户右键点击了一个节点
**When** 选择"复制文件名"
**Then** 文件名（不含路径）复制到系统剪贴板（FR19）

**Given** 用户右键点击了一个节点
**When** 选择"复制完整路径"
**Then** 文件完整路径复制到系统剪贴板（FR19）

### Story 3.6: 剪切、复制、粘贴

As a 开发者,
I want 通过剪贴板剪切、复制、粘贴文件和文件夹,
So that 我可以在文件树中移动和复制文件资源。

**Acceptance Criteria:**

**Given** 用户右键点击了一个文件或文件夹
**When** 选择"复制"或"剪切"
**Then** 源路径记录到剪贴板，标注操作类型（FR20）

**Given** 用户已复制或剪切了一个文件/文件夹
**When** 右键点击目标文件夹并选择"粘贴"
**Then** 文件/文件夹被复制或移动到目标目录（FR20）
**And** 粘贴时路径规范化处理（NFR8）

## Epic 4: Git 仓库管理

### Story 4.1: Git 仓库信息查看

As a 开发者,
I want 查看 Git 仓库的基本信息,
So that 我可以快速了解仓库当前状态。

**Acceptance Criteria:**

**Given** 用户选中了一个 Git 仓库目录
**When** 系统检测到该目录为 Git 仓库
**Then** 显示当前分支名、远程地址（origin URL）、仓库状态（FR21）
**And** 使用 go-git 读取仓库信息（架构约束：读操作用 go-git）

### Story 4.2: 克隆远程仓库

As a 开发者,
I want 克隆远程仓库到本地指定路径,
So that 我可以快速获取项目代码。

**Acceptance Criteria:**

**Given** 用户点击"克隆仓库"并输入 HTTPS 或 SSH 地址
**When** 选择本地目标路径并确认
**Then** 系统执行 `git clone` 将仓库克隆到指定路径（FR22）
**And** 使用 exec.Cmd 执行克隆命令（架构约束：写操作用 exec.Cmd）
**And** 克隆完成后新仓库自动出现在文件树中

**Given** 目标路径已存在同名仓库
**When** 用户尝试克隆
**Then** 提示"Git 仓库已存在"

### Story 4.3: 单仓库拉取

As a 开发者,
I want 对单个仓库执行 `git pull` 拉取更新,
So that 我可以将远程最新代码同步到本地。

**Acceptance Criteria:**

**Given** 用户选中了一个 Git 仓库
**When** 点击"拉取更新"
**Then** 系统执行 `git pull` 并返回结果（FR23）
**And** 单仓库拉取超时 30 秒（NFR4）

**Given** 拉取过程中发生错误
**When** 超时或网络异常
**Then** 返回错误信息，不影响其他功能

### Story 4.4: 批量并行拉取与实时进度

As a 开发者,
I want 批量拉取多个仓库的更新，实时看到每个仓库的进度,
So that 我可以一键同步所有仓库，高效掌握更新状态。

**Acceptance Criteria:**

**Given** 用户点击"批量更新"按钮
**When** 系统递归扫描工作目录下的 Git 仓库（FR24）
**Then** 识别所有 Git 仓库并准备批量拉取

**Given** 扫描发现多个 Git 仓库
**When** 批量拉取开始执行
**Then** 最多 5 个仓库并行拉取（NFR5）
**And** 实时展示每个仓库的进度：成功/失败/跳过（FR25）
**And** 通过 Wails 事件系统推送进度更新（架构约束：safeEmit）

### Story 4.5: 批量更新汇总结果

As a 开发者,
I want 查看批量更新的汇总结果,
So that 我可以了解整体更新情况并处理失败项。

**Acceptance Criteria:**

**Given** 批量拉取全部完成
**When** 用户查看汇总面板
**Then** 显示成功数、失败数、失败原因（FR26）
**And** 50 个仓库 2 分钟内完成（NFR6，网络正常）

**Given** 汇总结果中存在失败项
**When** 用户点击失败项
**Then** 展示具体错误信息，便于排查

### Story 4.6: 分页提交历史

As a 开发者,
I want 分页查看仓库的提交历史,
So that 我可以了解项目的开发记录。

**Acceptance Criteria:**

**Given** 用户选中了一个 Git 仓库
**When** 打开提交历史面板
**Then** 分页显示提交记录，每页包含 SHA、作者、时间、提交信息（FR27）
**And** 使用 go-git 读取提交日志（架构约束：读操作用 go-git）

**Given** 提交历史已加载
**When** 用户点击"下一页"或"上一页"
**Then** 加载对应页的提交记录，无需重新读取全部

## Epic 5: 外部工具集成

### Story 5.1: 在 VS Code 中打开

As a 开发者,
I want 在 VS Code 中打开文件或文件夹,
So that 我可以快速进入编码环境。

**Acceptance Criteria:**

**Given** 用户右键点击了一个文件或文件夹
**When** 选择"在 VS Code 中打开"
**Then** 系统调用 `code` 命令打开对应路径（FR28）
**And** exec.Cmd 必须使用 HideCommandWindow（架构约束）

### Story 5.2: 在资源管理器中打开

As a 开发者,
I want 在系统文件资源管理器中打开指定目录,
So that 我可以使用 Windows 原生资源管理器操作文件。

**Acceptance Criteria:**

**Given** 用户右键点击了一个文件夹
**When** 选择"在资源管理器中打开"
**Then** 系统调用 `explorer` 命令打开对应目录（FR29）

### Story 5.3: 用系统默认程序打开文件

As a 开发者,
I want 用系统默认程序打开文件,
So that 我可以快速查看或编辑文件。

**Acceptance Criteria:**

**Given** 用户右键点击了一个文件
**When** 选择"用默认程序打开"
**Then** 系统调用 `cmd /c start` 以 Windows 文件关联打开文件（FR30）
**And** exec.Cmd 必须使用 HideCommandWindow（架构约束）
