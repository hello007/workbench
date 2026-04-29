# Enhance UI Git History - 开发计划

**变更编号:** enhance-ui-git-history
**创建日期:** 2025-04-29
**预计工期:** 3-4 天
**执行方式:** 配合 @superpowers:executing-plans 技能自动执行

---

## 目录

- [1. 概述](#1-概述)
- [2. 整体架构](#2-整体架构)
- [3. Phase 1: 后端开发](#3-phase-1-后端开发)
- [4. Phase 2: 前端核心组件开发](#4-phase-2-前端核心组件开发)
- [5. Phase 3: 业务逻辑集成](#5-phase-3-业务逻辑集成)
- [6. Phase 4: 测试和验证](#6-phase-4-测试和验证)
- [7. 关键检查点](#7-关键检查点)
- [8. 风险和缓解措施](#8-风险和缓解措施)

---

## 1. 概述

### 1.1 变更目标

基于 `openspec/changes/enhance-ui-git-history/proposal.md` 定义的三个优化点：

1. **左侧树样式美化** - 优化文件树的视觉效果，包括节点图标、颜色主题、展开/折叠动画、悬停效果
2. **Git 仓库地址信息展示** - 在右侧区域显示 Git 仓库的详细信息（远程地址、分支、提交信息）
3. **Git 历史提交查看功能** - 支持查看提交列表、提交详情、搜索过滤和分页加载

### 1.2 技术栈

- **后端:** Go 1.21+, Wails v2, go-git v5
- **前端:** Vue.js 3, Element Plus, Element Plus Icons
- **构建:** Wails CLI
- **测试:** Go testing, 手动功能测试

### 1.3 项目结构

```
git-manager/
├── main.go                          # Wails 绑定方法
├── model/                           # 数据模型
│   └── commit.go                    # Commit 和 GitRemoteInfo 结构体
├── service/                         # 业务逻辑层（可选）
├── util/                            # 工具类（可选）
└── frontend/
    ├── src/
    │   ├── views/
    │   │   └── Home.vue             # 主页面（需修改）
    │   ├── components/
    │   │   ├── GitInfo.vue          # Git 信息组件（新建）
    │   │   └── CommitHistory.vue    # 提交历史组件（新建）
    │   └── utils/
    │       ├── gitCache.js          # 缓存机制（新建）
    │       ├── errorHandler.js      # 错误处理（新建）
    │       ├── retry.js             # 重试机制（新建）
    │       └── confirmation.js      # 确认对话框（新建）
    └── wailsjs/                     # 自动生成的绑定
```

---

## 2. 整体架构

### 2.1 执行策略：线性分层执行

采用**线性分层执行**方案，按技术分层先后后前：

```
Phase 1: 后端开发（关键步骤 A - 超详细）
  ├── 依赖管理和环境准备
  ├── 数据模型定义
  ├── Git 操作实现
  ├── 单元测试
  └── Wails 绑定生成

Phase 2: 前端核心组件开发（关键步骤 B - 超详细）
  ├── 文件树样式增强
  ├── Git 信息展示组件
  ├── 提交历史组件
  └── 组件集成到 Home.vue

Phase 3: 业务逻辑集成（中等详细）
  ├── 状态管理和数据流
  ├── 交互逻辑实现
  └── 错误处理和用户反馈

Phase 4: 测试和验证（中等详细）
  ├── 功能测试
  ├── 性能测试
  ├── 跨平台测试
  └── 构建和部署
```

### 2.2 预估时间

- **Phase 1**: 0.5-1 天（后端开发）
- **Phase 2**: 1.5-2 天（前端核心组件）
- **Phase 3**: 0.5 天（业务逻辑集成）
- **Phase 4**: 0.5 天（测试验证）
- **总计：3-4 天**

### 2.3 关键检查点

在每个 Phase 结束后设置检查点：

1. **Phase 1 完成后** - 后端方法可调用，测试通过
2. **Phase 2 完成后** - UI 组件可交互，视觉效果符合预期
3. **Phase 3 完成后** - 完整功能流程可用
4. **Phase 4 完成后** - 所有测试通过，可发布

---

## 3. Phase 1: 后端开发（关键步骤 A - 超详细）

### 3.1 依赖管理和环境准备

#### 目标
添加必要的 Go 依赖，确保开发环境就绪

#### 任务清单

**1. 检查当前 Go 版本**
```bash
go version  # 确保 >= 1.21
```

**2. 添加 go-git 依赖**
```bash
cd git-manager
go get github.com/go-git/go-git/v5
go get github.com/go-git/go-git/v5/plumbing
go get github.com/go-git/go-git/v5/plumbing/object
go mod tidy
go mod verify
```

**依赖说明：**
- `go-git/v5` - Git 核心操作库
- `plumbing` - Git 对象引用
- `plumbing/object` - Git 对象（提交、树等）

---

### 3.2 数据模型定义

#### 目标
定义 Commit 数据结构，支持前后端数据传输

#### 文件路径
`git-manager/model/commit.go`

#### 完整代码

```go
package model

// Commit 表示一个 Git 提交记录
type Commit struct {
    SHA       string   `json:"sha"`        // 完整的 40 位 SHA
    ShortSHA  string   `json:"shortSha"`   // 前 8 位 SHA，用于显示
    Message   string   `json:"message"`    // 提交消息
    Author    string   `json:"author"`     // 作者名称
    Email     string   `json:"email"`      // 作者邮箱
    Timestamp int64    `json:"timestamp"`  // Unix 时间戳
    DateTime  string   `json:"dateTime"`   // 格式化的时间字符串
    Files     []string `json:"files"`      // 变更的文件路径列表
}

// GitRemoteInfo 表示 Git 远程仓库信息
type GitRemoteInfo struct {
    RemoteURL  string `json:"remoteUrl"`  // 远程仓库地址
    Branch     string `json:"branch"`     // 当前分支名称
    IsDetached bool   `json:"isDetached"` // 是否处于分离头指针状态
}
```

#### 测试文件

**文件路径：** `git-manager/model/commit_test.go`

```go
package model

import "testing"

func TestCommit_Structure(t *testing.T) {
    commit := Commit{
        SHA:       "abc123def456",
        ShortSHA:  "abc123de",
        Message:   "Test commit",
        Author:    "Test Author",
        Email:     "test@example.com",
        Timestamp: 1234567890,
    }

    if commit.SHA != "abc123def456" {
        t.Errorf("Expected SHA to be abc123def456, got %s", commit.SHA)
    }
}
```

---

### 3.3 Git 操作实现

#### 3.3.1 GetGitRemoteURL 方法

**文件路径：** `git-manager/main.go`

```go
// GetGitRemoteURL 获取 Git 仓库的远程地址和当前分支信息
func GetGitRemoteURL(path string) (*model.GitRemoteInfo, error) {
    repo, err := git.PlainOpen(path)
    if err != nil {
        return nil, fmt.Errorf("无法打开 Git 仓库: %w", err)
    }

    remote, err := repo.Remote("origin")
    if err != nil {
        return &model.GitRemoteInfo{}, nil
    }

    remoteURL := ""
    if len(remote.Config().URLs) > 0 {
        remoteURL = remote.Config().URLs[0]
    }

    head, err := repo.Head()
    if err != nil {
        return nil, fmt.Errorf("无法获取 HEAD 引用: %w", err)
    }

    branchName := head.Name().Short()
    isDetached := !head.Name().IsBranch()

    return &model.GitRemoteInfo{
        RemoteURL:  remoteURL,
        Branch:     branchName,
        IsDetached: isDetached,
    }, nil
}
```

#### 3.3.2 GetCommitHistory 方法

**文件路径：** `git-manager/main.go`

```go
// GetCommitHistory 获取 Git 仓库的提交历史
func GetCommitHistory(path string, limit int, offset int) ([]model.Commit, error) {
    repo, err := git.PlainOpen(path)
    if err != nil {
        return nil, fmt.Errorf("无法打开 Git 仓库: %w", err)
    }

    commitIter, err := repo.Log(&git.LogOptions{
        Order: git.LogOrderCommitterTime,
    })
    if err != nil {
        return nil, fmt.Errorf("无法获取提交历史: %w", err)
    }
    defer commitIter.Close()

    // 跳过 offset 个提交
    for i := 0; i < offset; i++ {
        _, err := commitIter.Next()
        if err != nil {
            break
        }
    }

    var commits []model.Commit
    for i := 0; i < limit; i++ {
        commitObj, err := commitIter.Next()
        if err != nil {
            break
        }

        commit := model.Commit{
            SHA:       commitObj.Hash.String(),
            ShortSHA:  commitObj.Hash.String()[:8],
            Message:   commitObj.Message,
            Author:    commitObj.Author.Name,
            Email:     commitObj.Author.Email,
            Timestamp: commitObj.Author.When.Unix(),
            DateTime:  commitObj.Author.When.Format("2006-01-02 15:04:05"),
        }

        files := getCommitFiles(repo, commitObj)
        commit.Files = files

        commits = append(commits, commit)
    }

    return commits, nil
}

// getCommitFiles 获取提交中变更的文件列表
func getCommitFiles(repo *git.Repository, commit *object.Commit) []string {
    var files []string

    currentTree, err := commit.Tree()
    if err != nil {
        return files
    }

    parentCommit, err := commit.Parent(0)
    if err != nil {
        return getTreeFiles(currentTree)
    }

    parentTree, err := parentCommit.Tree()
    if err != nil {
        return files
    }

    patch, err := currentTree.Patch(parentTree)
    if err != nil {
        return files
    }

    for _, patchObj := range patch.FilePatches() {
        from, to := patchObj.Files()
        if from != nil {
            files = append(files, from.Path())
        } else if to != nil {
            files = append(files, to.Path())
        }
    }

    return files
}

func getTreeFiles(tree *object.Tree) []string {
    var files []string
    tree.Files().ForEach(func(file *object.File) error {
        files = append(files, file.Name)
        return nil
    })
    return files
}
```

---

### 3.4 单元测试

**文件路径：** `git-manager/main_test.go`

```go
package main

import (
    "os"
    "path/filepath"
    "testing"
)

var testRepoPath string

func TestMain(m *testing.M) {
    testRepoPath = filepath.Join(os.TempDir(), "test-git-repo")
    os.MkdirAll(testRepoPath, 0755)

    code := m.Run()
    os.RemoveAll(testRepoPath)
    os.Exit(code)
}

func TestGetGitRemoteURL(t *testing.T) {
    info, err := GetGitRemoteURL(testRepoPath)
    if err != nil {
        t.Fatalf("GetGitRemoteURL failed: %v", err)
    }
    if info == nil {
        t.Fatal("Expected GitRemoteInfo, got nil")
    }
}

func TestGetCommitHistory(t *testing.T) {
    commits, err := GetCommitHistory(testRepoPath, 10, 0)
    if err != nil {
        t.Fatalf("GetCommitHistory failed: %v", err)
    }
    if len(commits) > 10 {
        t.Errorf("Expected max 10 commits, got %d", len(commits))
    }
}
```

**运行测试：**
```bash
cd git-manager
go test ./...
go test -cover ./...
```

---

### 3.5 Wails 绑定生成

```bash
cd git-manager
wails generate module
```

**验证生成的文件：**
```bash
ls -la wailsjs/go/main/App.js
ls -la wailsjs/go/main/App.d.ts
```

---

### Phase 1 检查点

**完成标准：**
- [ ] go-git 依赖已添加
- [ ] `model/commit.go` 已创建
- [ ] `GetGitRemoteURL` 方法已实现
- [ ] `GetCommitHistory` 方法已实现
- [ ] 单元测试全部通过
- [ ] Wails 绑定已生成

**测试命令：**
```bash
cd git-manager
go test ./...
wails dev
```

---

## 4. Phase 2: 前端核心组件开发（关键步骤 B - 超详细）

### 4.1 文件树样式增强

#### 4.1.1 安装 Element Plus Icons

```bash
cd git-manager/frontend
npm install @element-plus/icons-vue
```

#### 4.1.2 修改文件树模板

**文件路径：** `git-manager/frontend/src/views/Home.vue`

**导入图标：**
```javascript
import {
  Folder,
  FolderOpened,
  Document,
  SuccessFilled
} from '@element-plus/icons-vue'
```

**修改模板：**
```vue
<template #default="{ node, data }">
  <span class="custom-tree-node">
    <el-icon
      v-if="data.type === 'directory'"
      :color="node.expanded ? '#409EFF' : '#909399'"
      style="margin-right: 5px;"
    >
      <component :is="node.expanded ? FolderOpened : Folder" />
    </el-icon>
    <el-icon v-else color="#606266" style="margin-right: 5px;">
      <Document />
    </el-icon>
    <span :style="{
      color: data.type === 'directory'
        ? (node.expanded ? '#409EFF' : '#909399')
        : '#606266'
    }">
      {{ node.label }}
    </span>
    <el-icon v-if="data.isGitRepo" color="#67C23A" style="margin-left: 5px;">
      <SuccessFilled />
    </el-icon>
  </span>
</template>
```

#### 4.1.3 添加样式

**文件路径：** `git-manager/frontend/src/views/Home.vue` - `<style scoped>`

```css
.el-tree-node__content {
  transition: background-color 0.2s ease;
  border-radius: 4px;
  margin: 2px 0;
}

.el-tree-node__content:hover {
  background-color: #F5F7FA !important;
}

.is-current > .el-tree-node__content {
  background-color: #E6F7FF !important;
  font-weight: 500;
}

.custom-tree-node {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 14px;
  padding-right: 8px;
}

.el-tree-node__children {
  transition: all 0.3s ease;
  overflow: hidden;
}
```

---

### 4.2 Git 信息展示组件

#### 4.2.1 创建 GitInfo.vue

**文件路径：** `git-manager/frontend/src/components/GitInfo.vue`

**完整组件代码：**
```vue
<template>
  <el-card
    v-if="gitInfo"
    class="git-info-card"
    shadow="hover"
  >
    <template #header>
      <div class="card-header">
        <span>Git 仓库信息</span>
        <el-button
          :icon="Refresh"
          circle
          size="small"
          @click="handleRefresh"
          :loading="loading"
        />
      </div>
    </template>

    <el-descriptions
      :column="1"
      border
      size="small"
      v-loading="loading"
    >
      <el-descriptions-item label="远程地址">
        <div v-if="gitInfo.remoteUrl">
          <el-link
            v-if="isHttpUrl(gitInfo.remoteUrl)"
            :href="gitInfo.remoteUrl"
            target="_blank"
            type="primary"
          >
            {{ gitInfo.remoteUrl }}
          </el-link>
          <div v-else class="url-with-copy">
            <span class="url-text">{{ gitInfo.remoteUrl }}</span>
            <el-button
              :icon="DocumentCopy"
              size="small"
              text
              @click="copyToClipboard(gitInfo.remoteUrl)"
            />
          </div>
        </div>
        <el-text v-else type="info">未配置远程地址</el-text>
      </el-descriptions-item>

      <el-descriptions-item label="当前分支">
        <el-tag
          v-if="!gitInfo.isDetached"
          :type="getBranchTagType(gitInfo.branch)"
        >
          {{ gitInfo.branch }}
        </el-tag>
        <el-tag v-else type="danger">分离头指针</el-tag>
      </el-descriptions-item>

      <el-descriptions-item label="最新提交">
        <div class="sha-with-copy">
          <el-text class="sha-text">{{ latestCommit?.shortSha || 'N/A' }}</el-text>
          <el-button
            :icon="DocumentCopy"
            size="small"
            text
            @click="copyToClipboard(latestCommit?.sha || '')"
          />
        </div>
      </el-descriptions-item>

      <el-descriptions-item label="提交时间">
        {{ formatTime(latestCommit?.timestamp) }}
      </el-descriptions-item>

      <el-descriptions-item label="提交消息">
        <el-text class="commit-message" :line-clamp="3">
          {{ latestCommit?.message || 'N/A' }}
        </el-text>
      </el-descriptions-item>
    </el-descriptions>
  </el-card>
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, DocumentCopy } from '@element-plus/icons-vue'
import { GetGitRemoteURL } from '../../../wailsjs/go/main/App'

const props = defineProps({
  repoPath: { type: String, required: true },
  latestCommit: { type: Object, default: null }
})

const gitInfo = ref(null)
const loading = ref(false)

const loadGitInfo = async () => {
  loading.value = true
  try {
    const info = await GetGitRemoteURL(props.repoPath)
    gitInfo.value = info
  } catch (error) {
    ElMessage.error('加载 Git 信息失败: ' + error)
  } finally {
    loading.value = false
  }
}

const handleRefresh = () => {
  loadGitInfo()
}

const isHttpUrl = (url) => {
  return url && (url.startsWith('http://') || url.startsWith('https://'))
}

const getBranchTagType = (branch) => {
  if (branch === 'main' || branch === 'master') return 'primary'
  return 'success'
}

const copyToClipboard = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch (error) {
    ElMessage.error('复制失败')
  }
}

const formatTime = (timestamp) => {
  if (!timestamp) return 'N/A'
  const now = Date.now()
  const diff = now - timestamp * 1000
  const days = Math.floor(diff / (1000 * 60 * 60 * 24))
  if (days === 0) return '今天'
  if (days === 1) return '昨天'
  if (days < 7) return `${days} 天前`
  if (days < 30) return `${Math.floor(days / 7)} 周前`
  const date = new Date(timestamp * 1000)
  return date.toLocaleDateString('zh-CN')
}

loadGitInfo()

defineExpose({ loadGitInfo })
</script>

<style scoped>
.git-info-card { margin-bottom: 20px; }
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 500;
}
.url-with-copy {
  display: flex;
  align-items: center;
  gap: 8px;
}
.url-text {
  font-family: monospace;
  font-size: 13px;
  color: #606266;
}
.sha-with-copy {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sha-text {
  font-family: monospace;
  font-size: 13px;
  color: #409EFF;
  cursor: pointer;
}
.commit-message {
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
```

---

### 4.3 提交历史组件

#### 4.3.1 创建 CommitHistory.vue

**文件路径：** `git-manager/frontend/src/components/CommitHistory.vue`

**完整组件代码：**
```vue
<template>
  <el-card class="commit-history-card" shadow="hover">
    <template #header>
      <div class="card-header">
        <span>提交历史</span>
        <div class="header-actions">
          <el-input
            v-model="searchKeyword"
            placeholder="搜索提交..."
            prefix-icon="Search"
            size="small"
            style="width: 200px; margin-right: 10px;"
            clearable
            @input="handleSearch"
          />
          <el-button
            :icon="Refresh"
            circle
            size="small"
            @click="handleRefresh"
            :loading="loading"
          />
        </div>
      </div>
    </template>

    <div v-loading="loading" class="timeline-container">
      <el-timeline v-if="filteredCommits.length > 0">
        <el-timeline-item
          v-for="commit in filteredCommits"
          :key="commit.sha"
          :timestamp="formatTime(commit.timestamp)"
          placement="top"
          @click="toggleCommitDetail(commit.sha)"
          class="commit-item"
        >
          <el-card class="commit-card" shadow="hover">
            <div class="commit-header">
              <div class="commit-sha">
                <el-text
                  type="primary"
                  class="sha-text"
                  @click.stop="copyToClipboard(commit.sha)"
                >
                  {{ commit.shortSha }}
                </el-text>
                <el-tag size="small" type="info" style="margin-left: 10px;">
                  {{ commit.files.length }} 个文件
                </el-tag>
              </div>
              <el-button
                :icon="expandedCommits.has(commit.sha) ? ArrowUp : ArrowDown"
                size="small"
                text
                @click.stop="toggleCommitDetail(commit.sha)"
              />
            </div>

            <el-text class="commit-message">{{ commit.message }}</el-text>
            <div class="commit-meta">
              <el-icon><User /></el-icon>
              <el-text size="small">{{ commit.author }}</el-text>
              <el-divider direction="vertical" />
              <el-text size="small" type="info">
                {{ formatTime(commit.timestamp) }}
              </el-text>
            </div>

            <el-collapse-transition>
              <div v-show="expandedCommits.has(commit.sha)" class="commit-detail">
                <el-divider />
                <el-descriptions :column="1" size="small" border>
                  <el-descriptions-item label="完整 SHA">
                    <div class="sha-full">
                      <el-text class="sha-text">{{ commit.sha }}</el-text>
                      <el-button
                        :icon="DocumentCopy"
                        size="small"
                        text
                        @click.stop="copyToClipboard(commit.sha)"
                      />
                    </div>
                  </el-descriptions-item>
                  <el-descriptions-item label="作者邮箱">
                    {{ commit.email }}
                  </el-descriptions-item>
                  <el-descriptions-item label="提交时间">
                    {{ commit.dateTime }}
                  </el-descriptions-item>
                </el-descriptions>

                <div class="files-section">
                  <el-text size="small" strong>变更文件：</el-text>
                  <el-tag
                    v-for="(file, index) in commit.files"
                    :key="index"
                    size="small"
                    style="margin: 5px 5px 0 0;"
                  >
                    {{ file }}
                  </el-tag>
                </div>
              </div>
            </el-collapse-transition>
          </el-card>
        </el-timeline-item>
      </el-timeline>

      <el-empty
        v-else-if="!loading && commits.length === 0"
        description="暂无提交记录"
      />

      <el-empty
        v-else-if="!loading && filteredCommits.length === 0"
        description="未找到匹配的提交"
      />

      <div
        v-if="!loading && commits.length > 0 && hasMore"
        class="load-more"
      >
        <el-button
          type="primary"
          @click="loadMore"
          :loading="loadingMore"
          plain
          style="width: 100%;"
        >
          加载更多 ({{ commits.length }} / {{ totalCount }})
        </el-button>
      </div>
    </div>
  </el-card>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import {
  Refresh, DocumentCopy, ArrowUp, ArrowDown,
  User, Search
} from '@element-plus/icons-vue'
import { GetCommitHistory } from '../../../wailsjs/go/main/App'

const props = defineProps({
  repoPath: { type: String, required: true }
})

const PAGE_SIZE = 20

const commits = ref([])
const expandedCommits = ref(new Set())
const loading = ref(false)
const loadingMore = ref(false)
const searchKeyword = ref('')
const hasMore = ref(false)
const totalCount = ref(0)

const filteredCommits = computed(() => {
  if (!searchKeyword.value) return commits.value

  const keyword = searchKeyword.value.toLowerCase()
  return commits.value.filter(commit =>
    commit.message.toLowerCase().includes(keyword) ||
    commit.author.toLowerCase().includes(keyword) ||
    commit.sha.toLowerCase().includes(keyword)
  )
})

const loadCommits = async (reset = true) => {
  if (reset) {
    loading.value = true
    commits.value = []
  } else {
    loadingMore.value = true
  }

  try {
    const offset = reset ? 0 : commits.value.length
    const newCommits = await GetCommitHistory(props.repoPath, PAGE_SIZE, offset)

    if (reset) {
      commits.value = newCommits
    } else {
      commits.value.push(...newCommits)
    }

    hasMore.value = newCommits.length === PAGE_SIZE
    totalCount.value = commits.value.length
  } catch (error) {
    ElMessage.error('加载提交历史失败: ' + error)
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}

const loadMore = () => {
  loadCommits(false)
}

const handleRefresh = () => {
  expandedCommits.value.clear()
  loadCommits(true)
}

const handleSearch = () => {
  // 搜索由 computed 属性自动处理
}

const toggleCommitDetail = (sha) => {
  if (expandedCommits.value.has(sha)) {
    expandedCommits.value.delete(sha)
  } else {
    expandedCommits.value.add(sha)
  }
}

const copyToClipboard = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch (error) {
    ElMessage.error('复制失败')
  }
}

const formatTime = (timestamp) => {
  if (!timestamp) return 'N/A'
  const now = Date.now()
  const diff = now - timestamp * 1000
  const minutes = Math.floor(diff / (1000 * 60))
  const hours = Math.floor(diff / (1000 * 60 * 60))
  const days = Math.floor(diff / (1000 * 60 * 60 * 24))

  if (minutes < 60) return `${minutes} 分钟前`
  if (hours < 24) return `${hours} 小时前`
  if (days < 30) return `${days} 天前`
  const date = new Date(timestamp * 1000)
  return date.toLocaleDateString('zh-CN')
}

onMounted(() => {
  loadCommits(true)
})

defineExpose({ loadCommits, handleRefresh })
</script>

<style scoped>
.commit-history-card { height: 100%; }
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 500;
}
.header-actions {
  display: flex;
  align-items: center;
}
.timeline-container {
  max-height: 600px;
  overflow-y: auto;
}
.commit-item { cursor: pointer; }
.commit-card { margin-bottom: 10px; }
.commit-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.commit-sha {
  display: flex;
  align-items: center;
}
.sha-text {
  font-family: monospace;
  font-size: 13px;
  cursor: pointer;
}
.sha-text:hover { text-decoration: underline; }
.sha-full {
  display: flex;
  align-items: center;
  gap: 10px;
}
.commit-message {
  display: block;
  margin: 10px 0;
  font-size: 14px;
  line-height: 1.5;
  color: #303133;
}
.commit-meta {
  display: flex;
  align-items: center;
  gap: 5px;
  color: #909399;
}
.commit-detail { margin-top: 10px; }
.files-section { margin-top: 15px; }
.load-more {
  margin-top: 20px;
  text-align: center;
}
.timeline-container::-webkit-scrollbar { width: 6px; }
.timeline-container::-webkit-scrollbar-thumb {
  background-color: #dcdfe6;
  border-radius: 3px;
}
</style>
```

---

### 4.4 组件集成到 Home.vue

**文件路径：** `git-manager/frontend/src/views/Home.vue`

**导入组件：**
```javascript
import GitInfo from '../components/GitInfo.vue'
import CommitHistory from '../components/CommitHistory.vue'
```

**添加响应式数据：**
```javascript
const latestCommit = ref(null)
const commits = ref([])
```

**修改模板：**
```vue
<el-main>
  <div v-if="selectedNode" style="padding: 20px;">
    <h2>{{ selectedNode.name }}</h2>
    <el-descriptions :column="2" border>
      <el-descriptions-item label="路径">{{ selectedNode.path }}</el-descriptions-item>
      <el-descriptions-item label="类型">
        {{ selectedNode.type === 'directory' ? '文件夹' : '文件' }}
      </el-descriptions-item>
    </el-descriptions>

    <el-divider />

    <!-- Git 信息组件 -->
    <GitInfo
      v-if="selectedNode.isGitRepo"
      :repo-path="selectedNode.path"
      :latest-commit="latestCommit"
    />

    <!-- 提交历史组件 -->
    <CommitHistory
      v-if="selectedNode.isGitRepo"
      :repo-path="selectedNode.path"
    />

    <!-- 原有内容保持不变 -->
  </div>
</el-main>
```

---

### Phase 2 检查点

**完成标准：**
- [ ] Element Plus Icons 已安装
- [ ] 文件树添加了图标和颜色主题
- [ ] GitInfo.vue 组件已创建并正常工作
- [ ] CommitHistory.vue 组件已创建并正常工作
- [ ] 组件已集成到 Home.vue
- [ ] 可以在 Git 仓库节点查看信息和历史
- [ ] 复制功能正常工作
- [ ] 搜索和过滤功能正常工作
- [ ] 分页加载功能正常工作

**测试步骤：**
```bash
cd git-manager
wails dev
```

---

## 5. Phase 3: 业务逻辑集成（中等详细）

### 5.1 状态管理和数据流

#### 5.1.1 实现缓存机制

**文件路径：** `git-manager/frontend/src/utils/gitCache.js`

```javascript
class GitCache {
  constructor() {
    this.cache = new Map()
    this.maxAge = 5 * 60 * 1000 // 5 分钟
  }

  set(key, value) {
    this.cache.set(key, {
      value,
      timestamp: Date.now()
    })
  }

  get(key) {
    const item = this.cache.get(key)
    if (!item) return null

    if (Date.now() - item.timestamp > this.maxAge) {
      this.cache.delete(key)
      return null
    }

    return item.value
  }

  clear() {
    this.cache.clear()
  }

  delete(key) {
    this.cache.delete(key)
  }
}

export const gitCache = new GitCache()

export const getCacheKey = (type, path) => {
  return `${type}:${path}`
}
```

---

### 5.2 交互逻辑实现

#### 5.2.1 加载状态管理

**修改 Home.vue 的 onNodeClick：**
```javascript
const onNodeClick = async (data) => {
  selectedNode.value = data
  filePreview.value = { content: '', error: '' }

  if (data.isGitRepo) {
    gitLoadingStates.value.info = true
    gitLoadingStates.value.history = true

    try {
      const [info, history] = await Promise.all([
        GetGitInfo(data.path),
        GetCommitHistory(data.path, 20, 0)
      ])

      Object.assign(selectedNode.value, info)
      commits.value = history

      if (history.length > 0) {
        latestCommit.value = history[0]
      }
    } catch (error) {
      ElMessage.error('加载 Git 数据失败: ' + error)
    } finally {
      gitLoadingStates.value.info = false
      gitLoadingStates.value.history = false
    }
  } else {
    latestCommit.value = null
    commits.value = []
  }
}
```

---

### 5.3 错误处理和用户反馈

#### 5.3.1 全局错误处理

**文件路径：** `git-manager/frontend/src/utils/errorHandler.js`

```javascript
import { ElMessage } from 'element-plus'

export const ErrorTypes = {
  NETWORK: 'network',
  PERMISSION: 'permission',
  NOT_FOUND: 'not_found',
  GIT_ERROR: 'git_error',
  UNKNOWN: 'unknown'
}

export const handleError = (error, context = '') => {
  console.error(`[${context}] Error:`, error)

  let errorType = ErrorTypes.UNKNOWN
  let message = '操作失败'

  if (error.message) {
    const msg = error.message.toLowerCase()

    if (msg.includes('network') || msg.includes('fetch')) {
      errorType = ErrorTypes.NETWORK
      message = '网络错误，请检查连接'
    } else if (msg.includes('permission') || msg.includes('denied')) {
      errorType = ErrorTypes.PERMISSION
      message = '权限不足，无法访问'
    } else if (msg.includes('not found') || msg.includes('no such')) {
      errorType = ErrorTypes.NOT_FOUND
      message = '未找到指定的资源'
    } else if (msg.includes('git')) {
      errorType = ErrorTypes.GIT_ERROR
      message = 'Git 操作失败'
    }
  }

  ElMessage.error({
    message: context ? `${context}: ${message}` : message,
    duration: 5000,
    showClose: true
  })

  return { errorType, message }
}

export const withErrorHandling = async (fn, context = '') => {
  try {
    return await fn()
  } catch (error) {
    handleError(error, context)
    throw error
  }
}
```

#### 5.3.2 重试机制

**文件路径：** `git-manager/frontend/src/utils/retry.js`

```javascript
export const retryOperation = async (
  operation,
  maxRetries = 3,
  delay = 1000
) => {
  for (let i = 0; i < maxRetries; i++) {
    try {
      return await operation()
    } catch (error) {
      if (i === maxRetries - 1) throw error
      await new Promise(resolve =>
        setTimeout(resolve, delay * Math.pow(2, i))
      )
    }
  }
}

export const withRetryAndErrorHandling = async (
  operation,
  context = '',
  maxRetries = 2
) => {
  try {
    return await retryOperation(operation, maxRetries)
  } catch (error) {
    const { handleError } = await import('./errorHandler')
    handleError(error, context)
    throw error
  }
}
```

---

### Phase 3 检查点

**完成标准：**
- [ ] 缓存机制已实现并集成
- [ ] 加载状态管理优化完成
- [ ] 全局错误处理已实现
- [ ] 重试机制已实现
- [ ] 性能优化措施已实施

---

## 6. Phase 4: 测试和验证（中等详细）

### 6.1 功能测试

#### 6.1.1 功能测试清单

创建 `git-manager/TEST_CHECKLIST.md`：

```markdown
# 功能测试清单

## 1. 文件树功能
- [ ] 可以添加/切换工作目录
- [ ] 文件树正确加载文件和文件夹
- [ ] 文件夹图标正确显示（展开/未展开）
- [ ] Git 仓库标识正确显示
- [ ] 节点悬停效果正常
- [ ] 展开/折叠动画流畅
- [ ] 全部收起按钮正常工作

## 2. Git 信息展示
- [ ] 远程仓库地址正确显示
- [ ] HTTP/HTTPS 地址可点击
- [ ] SSH 地址可复制
- [ ] 当前分支名称正确
- [ ] 分离头指针状态正确标识
- [ ] 最新提交信息正确显示
- [ ] 刷新按钮正常工作

## 3. 提交历史
- [ ] 提交列表正确显示
- [ ] 按时间倒序排列
- [ ] 初始加载 20 条
- [ ] 点击可展开/折叠详情
- [ ] 完整 SHA 显示正确
- [ ] 变更文件列表显示
- [ ] "加载更多"正常工作
- [ ] 搜索功能正常
- [ ] 复制功能正常

## 4. 性能测试
- [ ] 小型仓库（< 100 提交）- 快速
- [ ] 中型仓库（100-1000 提交）- 流畅
- [ ] 大型仓库（> 10000 提交）- 不会卡顿

## 5. 错误处理
- [ ] 网络错误有提示
- [ ] 权限错误有提示
- [ ] 无效仓库有提示
```

---

### 6.2 性能测试

#### 6.2.1 创建性能基准测试

**文件路径：** `git-manager/backend_benchmark_test.go`

```go
package main

import (
    "testing"
)

func BenchmarkGetCommitHistory(b *testing.B) {
    repoPath := "/path/to/large/repo"

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := GetCommitHistory(repoPath, 20, 0)
        if err != nil {
            b.Fatalf("GetCommitHistory failed: %v", err)
        }
    }
}

func BenchmarkGetGitRemoteURL(b *testing.B) {
    repoPath := "/path/to/test/repo"

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := GetGitRemoteURL(repoPath)
        if err != nil {
            b.Fatalf("GetGitRemoteURL failed: %v", err)
        }
    }
}
```

**运行测试：**
```bash
cd git-manager
go test -bench=. -benchmem
```

**性能标准：**
- `GetCommitHistory`: < 100ms (20条记录)
- `GetGitRemoteURL`: < 50ms
- 前端渲染: < 500ms (20条提交)

---

### 6.3 跨平台测试

#### 6.3.1 创建跨平台测试脚本

**文件路径：** `git-manager/scripts/test-cross-platform.sh`

```bash
#!/bin/bash

echo "开始跨平台测试..."

PLATFORM=$(uname -s)
echo "当前平台: $PLATFORM"

case "$PLATFORM" in
  Linux*)
    echo "运行 Linux 测试..."
    go test ./...
    ;;

  Darwin*)
    echo "运行 macOS 测试..."
    go test ./...
    ;;

  MINGW*|MSYS*|CYGWIN*)
    echo "运行 Windows 测试..."
    go test ./...
    ;;

  *)
    echo "未知平台: $PLATFORM"
    exit 1
    ;;
esac

echo "跨平台测试完成！"
```

---

### 6.4 构建和部署

#### 6.4.1 构建生产版本

```bash
cd git-manager

# 清理之前的构建
wails build -clean

# 构建 Windows 版本
wails build -platform windows/amd64

# 构建 macOS 版本（如果需要）
wails build -platform darwin/amd64 -output git-manager-macos

# 构建 Linux 版本（如果需要）
wails build -platform linux/amd64 -output git-manager-linux
```

#### 6.4.2 验证构建产物

```bash
# 检查文件
ls -lh build/bin/git-manager.exe

# 测试运行
./build/bin/git-manager.exe

# 验证功能
# - 添加工作目录
# - 浏览文件树
# - 查看 Git 信息
# - 查看提交历史
```

#### 6.4.3 创建发布包

```bash
# 创建发布目录
mkdir -p release/git-manager-v1.0.0

# 复制文件
cp build/bin/git-manager.exe release/git-manager-v1.0.0/

# 创建 README
cat > release/git-manager-v1.0.0/README.md << 'EOF'
# Git Manager v1.0.0

## 安装

双击 git-manager.exe 启动应用

## 功能

- Git 仓库管理
- 文件浏览
- 提交历史查看

## 系统要求

- Windows 10 或更高版本
- WebView2 运行时
EOF

# 打包
cd release
zip -r git-manager-v1.0.0-windows.zip git-manager-v1.0.0/
```

#### 6.4.4 版本标签和发布

```bash
cd git-manager

# 创建版本标签
git tag -a v1.0.0 -m "Release version 1.0.0"

# 推送标签
git push origin v1.0.0

# 创建 GitHub Release
gh release create v1.0.0 \
  --title "Git Manager v1.0.0" \
  --notes "首个稳定版本，包含完整的 Git 仓库管理功能" \
  release/git-manager-v1.0.0-windows.zip
```

---

### Phase 4 检查点

**完成标准：**
- [ ] 所有功能测试通过
- [ ] 性能测试达标
- [ ] 跨平台测试通过（至少 Windows）
- [ ] 生产版本构建成功
- [ ] 构建产物可正常运行
- [ ] 发布包已创建
- [ ] 版本标签已创建

---

## 7. 关键检查点

### 7.1 Phase 检查点汇总

| Phase | 检查点 | 验证标准 |
|-------|--------|---------|
| **Phase 1** | 后端 API 可用 | 单元测试通过，Wails 绑定生成成功 |
| **Phase 2** | UI 组件完成 | 组件可交互，视觉效果符合预期 |
| **Phase 3** | 功能集成完成 | 完整功能流程可用，性能优化完成 |
| **Phase 4** | 测试和验证 | 所有测试通过，可发布 |

### 7.2 最终验证清单

```bash
# 运行所有测试
cd git-manager
go test ./...

# 构建生产版本
wails build

# 测试构建产物
./build/bin/git-manager.exe
```

**手动验证：**
- [ ] 可以添加工作目录
- [ ] 文件树样式美观，图标正确
- [ ] 点击 Git 仓库节点显示信息
- [ ] 提交历史正确显示
- [ ] 搜索和分页功能正常
- [ ] 复制功能正常
- [ ] 性能良好，无卡顿

---

## 8. 风险和缓解措施

### 8.1 已识别风险

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 大型仓库性能问题 | UI 卡顿 | 分页加载（每页 20 条） |
| go-git 库兼容性问题 | 某些仓库无法读取 | 错误处理和用户提示 |
| 跨平台路径问题 | 路径解析错误 | 使用 filepath 包处理 |
| WebView2 缺失 | Windows 无法运行 | 检测并提供安装指引 |

### 8.2 回滚策略

- 如果新功能导致严重问题，通过 Git revert 快速回滚
- 在发布前打上 tag，便于快速回滚
- 改动主要集中在前端 UI 和新增 Go 方法，不影响现有功能

---

## 附录

### A. 相关文档

- `openspec/changes/enhance-ui-git-history/proposal.md` - 变更提案
- `openspec/changes/enhance-ui-git-history/design.md` - 技术设计
- `openspec/changes/enhance-ui-git-history/specs/` - 功能规格
- `openspec/changes/enhance-ui-git-history/tasks.md` - 任务清单

### B. 依赖安装速查

```bash
# Go 依赖
cd git-manager
go get github.com/go-git/go-git/v5
go mod tidy

# 前端依赖
cd frontend
npm install @element-plus/icons-vue

# Wails 绑定生成
wails generate module
```

### C. 测试命令速查

```bash
# 后端测试
cd git-manager
go test ./...
go test -bench=. -benchmem

# 前端开发
wails dev

# 生产构建
wails build
```

---

**文档版本:** 1.0
**最后更新:** 2025-04-29
**状态:** 待审核
