# Git Submodule 操作与状态显示研究

> 实测环境：git 2.41.0.windows.3，临时仓库 `/tmp/submod-demo`（superproject `parent` + submodule `libs/child` + 嵌套 `libs/child/nested/grandchild`）。

## 关键纠正

**PRD 假设「`git submodule status --porcelain`」在 git 2.41 不存在。** `git submodule status` 仅接受 `--cached` 与 `--recursive`，传 `--porcelain` 打印 usage 退出码非 0。结构化输出来源是 `git status --porcelain=2`（superproject 视角）。

## 1. `git submodule status` 输出格式

### 默认 human 格式

```
 e39deaf9c6eedc613c3cfc42756bfeb6acf84060 libs/child (v1.0-2-ge39deaf)   # 干净已初始化
+be955f5283fd271a01d6884282765528ad308731 libs/child (v2.0)              # SHA 与 index 不一致
-e39deaf9c6eedc613c3cfc42756bfeb6acf84060 libs/child                    # 未初始化（无 describe 括号）
```

### 字段解析

| 位置 | 字段 | 解析 |
|---|---|---|
| 第 1 字符 | 前导状态码 | ` ` / `-` / `+` / `U` |
| 第 2~41 | SHA（40 位） | submodule 当前 checkout 的提交 SHA |
| 空格后 | path | submodule 在 superproject 中的相对路径 |
| `(...)` | describe | `git describe` 结果；未初始化时无 |

### 前导状态码

| 码 | 含义 | 触发条件 |
|---|---|---|
| ` ` | SHA 与 superproject index 记录一致 | 正常态 |
| `-` | submodule 未初始化 | `.git/config` 无对应段或工作目录未检出 |
| `+` | checkout 的 SHA ≠ index 记录的 SHA | submodule 指针前移/落后 |
| `U` | 合并冲突 | superproject 合并中两分支记录不同 submodule SHA |

### 致命陷阱：不检测 dirty 工作区

在 submodule 内制造未提交修改后，`git submodule status` 前导码**仍为空格不变**。dirty 必须另取数据源：
- `git status --porcelain` → ` m <path>`（小写 m = 工作区 dirty，大写 `M` = SHA 指针变更）
- `git status --porcelain=2` → submodule 行 `S` 标志位 + Y 字段

`git status --porcelain=2` submodule 行实测：
```
1 .M S.M. 160000 160000 160000 e39deaf9c6... e39deaf9c6... libs/child
```
字段：`1`=普通行；`.M`=XY；`S.M.`=submodule 标志位（含 dirty）；`160000`=gitlink 模式 ×3（HEAD/index/worktree）；两 SHA；path。

**结论**：WorkBench 状态显示须双命令融合——`git submodule status`（SHA/init 态）+ `git status --porcelain=2`（dirty 标记），按 path 关联。

## 2. init / update / update --init --recursive / update --remote 语义

| 命令 | 语义 | detached HEAD | 递归 |
|---|---|---|---|
| `init` | `.gitmodules` 条目复制到本地 `.git/config`，不克隆不检出 | - | 否 |
| `update` | 按 superproject index 记录的 SHA 检出，默认 `--checkout` | **是** | 否 |
| `update --init --recursive` | init+update 一步 + 下探嵌套 | **是** | 是 |
| `update --remote` | 忽略 index SHA，拉 `.gitmodules` 的 branch 远程最新 | 依赖初始态+策略 | 否 |
| `update --merge` | 远程 tip 合并进当前分支，**保持分支** | 否 | 否 |
| `update --rebase` | 变基当前分支到远程 tip | 否 | 否 |

**detached HEAD 陷阱**：`update`/`update --init` 默认检出 detached，用户直接开发会丢提交。GUI 须显式提示 + 提供 `--merge`/`--rebase`/`--remote` 选项。

**`update --remote` 副作用**：superproject index SHA 与 submodule 实际 SHA 不一致，`git status` 出现 ` M libs/child`，须 superproject commit 指针变更才算完成升级。

## 3. add / deinit + 删除条目完整流程

### add 影响四处

`git submodule add -b master <url> <path>` 生成 `.gitmodules`（版本化）+ 写 `.git/config`（本地注册）+ `.git/modules/<name>`（submodule git 目录存储）+ 工作区检出。submodule `.git` 是**文件**：`gitdir: ../../.git/modules/libs/child`。

### 完整删除流程（三处+清理）

```
# 1) deinit：清工作区 + .git/config
git submodule deinit -f <path>
# 2) git rm：清 superproject index gitlink + .gitmodules 条目
git rm -f <path>
# 3) .git/modules/<name> 残留，须手动删（git 不自动清）
rm -rf .git/modules/<name>
```

| 位置 | 清理方式 | 谁负责 |
|---|---|---|
| `.gitmodules` | `git rm -f` 自动移除 | git rm |
| `.git/config` | `git submodule deinit` | deinit |
| `.git/modules/<name>` | **手动 `rm -rf`** | 工具显式 |
| superproject index gitlink | `git rm -f` | git rm |
| 工作区目录 | deinit 清空 + git rm 删 | deinit + git rm |

**遗漏 `.git/modules/<name>`**：同名 submodule 重新 add 复用旧 git 目录，SHA/历史错乱。GUI 须显式执行第 3 步。

## 4. 同类工具呈现（documented 行为归纳，未联网核验）

| 工具 | 视图位置 | 列表字段 | 操作粒度 |
|---|---|---|---|
| Sourcetree | 左侧栏独立 Submodules 节点 | name/path/URL/commit | 全量 + 单个右键；add 走设置 |
| GitKraken | 左侧 Submodules 面板 | name/branch/commit SHA | 全量 + 单个；update 有 --recursive/--remote |
| VSCode 内置 | Source Control 面板，submodule 带 `S` 标记 | 仅 path+修改标记 | **无专用管理 UI**，粒度弱 |
| Fork | 独立 Submodules 视图 | name/path/branch/commit/URL | 全量 + 单个 init/update/sync；右键删除含清理 |

**共性**：列表含 path/name + 短 SHA + 跟踪 branch + URL；状态图标区分未初始化/干净/dirty/指针不同步；全量操作一级按钮，单个走右键；add/deinit 独立面板。WorkBench 做到 Sourcetree/Fork 级即优于 VSCode 内置。

## 5. 边界场景

| 场景 | 表现 | 修复 |
|---|---|---|
| 未初始化（`-`） | 无 describe 括号 | `git submodule update --init [<path>]` |
| detached HEAD | `update` 默认产物，`branch --show-current` 空 | `git -C <sub> checkout <branch>` 或 --merge/--rebase |
| dirty 工作区 | `submodule status` 不变，`status --porcelain=2` 有 `S` 标志 | 融合双命令 |
| 嵌套 submodule | `status` 默认不递归 | `--recursive` 或树形呈现 |
| SHA 不同步（`+`） | `submodule status` 前缀 +，`status --porcelain` ` M` | superproject commit 指针或 `update` 回退 |
| 冲突态（`U`） | 合并中两分支记录不同 submodule SHA | superproject 解决 |

## 6. 落地建议

1. **FindGitRoot 必须先修**（见 `findgitroot-fix-and-gogit-submodule.md`），否则 submodule 路径根定位错乱
2. **状态查询双命令融合**：`ListSubmodules` = `git submodule status` + `git status --porcelain=2`，只读不抢锁
3. **操作走 tryLockRepo**：init/update/add/deinit/remove 均抢锁；add/remove 加 `precheckMutation`
4. **detached HEAD 显式提示**：前端列表警告标记 + "切换跟踪分支"操作
5. **删除流程服务层显式清 `.git/modules/<name>`**
6. **`file://` 协议错误原样透传**（git 2.41 默认禁，CVE-2022-39253，用户环境配置，WorkBench 不处理）

## 数据契约建议

```go
type GitSubmodule struct {
    Path        string `json:"path"`
    Sha         string `json:"sha"`
    ShortSha    string `json:"shortSha"`
    Describe    string `json:"describe"`     // 可空
    Branch      string `json:"branch"`       // .gitmodules 的 branch，可空
    Url         string `json:"url"`
    Initialized bool   `json:"initialized"`  // - 前缀反之
    ShaMismatch bool   `json:"shaMismatch"`  // + 前缀
    Dirty       bool   `json:"dirty"`        // porcelain=2
    Conflict    bool   `json:"conflict"`     // U 前缀
}
```

## Caveats

1. `git submodule status --porcelain` 不存在——PRD 第 33 行假设须纠正
2. GUI 工具字段级细节未联网核验，实现期对照当前版本截图
3. 冲突态 `U` 未真实触发实测，格式据文档，建议补用例
4. 临时仓库 `C:/Users/liuyang/AppData/Local/Temp/submod-demo` 可复查，清理 `rm -rf`
