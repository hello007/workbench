# 工作目录拖拽排序 实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 支持用户通过拖拽调整工作目录列表顺序，排序持久化到 JSON 配置文件。

**Architecture:** 后端新增 `Reorder` 方法按 id 列表重排目录数组并保存；前端用 `vuedraggable` 包裹现有列表，拖拽结束后调用后端持久化。

**Tech Stack:** Go / Vue 3 / vuedraggable / Element Plus / Wails

---

### Task 1: 后端 Reorder 方法 + 测试

**Files:**
- Modify: `service/directory.go`（新增 `Reorder` 方法）
- Modify: `service/directory_test.go`（新增排序测试）

**Step 1: 编写失败测试**

在 `service/directory_test.go` 末尾追加：

```go
// --- Reorder 测试 ---

func TestReorder_Success(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dir3 := t.TempDir()
	svc := createTestService(t)

	created1, _ := svc.Create("目录1", dir1, false)
	created2, _ := svc.Create("目录2", dir2, false)
	created3, _ := svc.Create("目录3", dir3, false)

	// 反序排列: 3, 1, 2
	ids := []string{created3.ID, created1.ID, created2.ID}
	err := svc.Reorder(ids)
	if err != nil {
		t.Fatalf("Reorder: got error %v", err)
	}

	dirs, _ := svc.Load()
	if len(dirs) != 3 {
		t.Fatalf("After reorder count: got %d, want 3", len(dirs))
	}
	if dirs[0].ID != created3.ID {
		t.Errorf("Position 0: got %q, want %q", dirs[0].ID, created3.ID)
	}
	if dirs[1].ID != created1.ID {
		t.Errorf("Position 1: got %q, want %q", dirs[1].ID, created1.ID)
	}
	if dirs[2].ID != created2.ID {
		t.Errorf("Position 2: got %q, want %q", dirs[2].ID, created2.ID)
	}
}

func TestReorder_CountMismatch(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	svc := createTestService(t)

	svc.Create("目录1", dir1, false)
	svc.Create("目录2", dir2, false)

	// 只传1个id
	err := svc.Reorder([]string{"single-id"})
	if err == nil {
		t.Fatal("Reorder count mismatch: expected error, got nil")
	}
}

func TestReorder_DuplicateID(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	svc := createTestService(t)

	created1, _ := svc.Create("目录1", dir1, false)
	svc.Create("目录2", dir2, false)

	// 重复id
	err := svc.Reorder([]string{created1.ID, created1.ID})
	if err == nil {
		t.Fatal("Reorder duplicate id: expected error, got nil")
	}
}

func TestReorder_UnknownID(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	svc := createTestService(t)

	created1, _ := svc.Create("目录1", dir1, false)
	created2, _ := svc.Create("目录2", dir2, false)

	err := svc.Reorder([]string{created1.ID, "nonexistent-id"})
	if err == nil {
		t.Fatal("Reorder unknown id: expected error, got nil")
	}
}
```

**Step 2: 运行测试验证失败**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && go test ./service/ -run TestReorder -v`
Expected: FAIL — `svc.Reorder undefined`

**Step 3: 实现 Reorder 方法**

在 `service/directory.go` 末尾追加：

```go
// Reorder 按给定 id 顺序重排目录
func (s *DirectoryService) Reorder(ids []string) error {
	directories, err := s.Load()
	if err != nil {
		return err
	}

	if len(ids) != len(directories) {
		return fmt.Errorf("排序 id 数量(%d)与实际目录数(%d)不一致", len(ids), len(directories))
	}

	// 构建查找表
	dirMap := make(map[string]*model.Directory, len(directories))
	for _, dir := range directories {
		dirMap[dir.ID] = dir
	}

	// 按新顺序排列，同时校验 id 有效且无重复
	reordered := make([]*model.Directory, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return fmt.Errorf("排序 id 重复: %s", id)
		}
		seen[id] = true
		dir, ok := dirMap[id]
		if !ok {
			return fmt.Errorf("工作目录不存在: %s", id)
		}
		reordered = append(reordered, dir)
	}

	return s.Save(reordered)
}
```

**Step 4: 运行测试验证通过**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && go test ./service/ -run TestReorder -v`
Expected: PASS

**Step 5: 提交**

```bash
git add service/directory.go service/directory_test.go
git commit -m "feat: DirectoryService.Reorder 按id列表重排目录顺序"
```

---

### Task 2: App 绑定方法

**Files:**
- Modify: `app.go`（新增 `ReorderDirectories` 方法）

**Step 1: 添加绑定方法**

在 `app.go` 的 `GetDefaultDirectory` 方法之后追加：

```go
// ReorderDirectories 重排工作目录顺序
func (a *App) ReorderDirectories(ids []string) bool {
	err := a.directorySvc.Reorder(ids)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}
```

**Step 2: 编译验证**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && go build ./...`
Expected: 编译成功（忽略 `pattern all:frontend/dist` 警告）

**Step 3: 提交**

```bash
git add app.go
git commit -m "feat: App.ReorderDirectories 绑定方法"
```

---

### Task 3: 安装 vuedraggable 依赖

**Files:**
- Modify: `frontend/package.json`（新增依赖）

**Step 1: 安装**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager/frontend && npm install vuedraggable@next`

**Step 2: 验证安装**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager/frontend && npm ls vuedraggable`
Expected: 显示版本号

**Step 3: 提交**

```bash
git add frontend/package.json frontend/package-lock.json
git commit -m "chore: 安装 vuedraggable 依赖"
```

---

### Task 4: 前端拖拽集成

**Files:**
- Modify: `frontend/src/components/DirectoryTree.vue`

**Step 1: 替换模板中的目录列表**

将 `<div class="dir-list">` 内的 `v-for` 列表替换为 `draggable` 组件：

```html
    <!-- 目录列表 -->
    <div class="dir-list">
      <draggable
        :list="directories"
        item-key="id"
        :animation="200"
        ghost-class="dir-item--ghost"
        @end="onDragEnd"
      >
        <template #item="{ element: dir }">
          <div
            class="dir-item"
            :class="{ 'dir-item--active': dir.id === selectedId }"
            @click="handleSelect(dir.id)"
            @contextmenu.prevent="onContextMenu($event, dir)"
          >
            <div class="dir-info">
              <div class="dir-row">
                <el-icon class="dir-item-icon" color="#909399">
                  <Folder />
                </el-icon>
                <span class="dir-item-name" :title="dir.name">{{ dir.name }}</span>
                <el-icon v-if="dir.isDefault" class="dir-item-star" color="#e6a23c">
                  <Star />
                </el-icon>
              </div>
              <div class="dir-path" :title="dir.path">{{ dir.path }}</div>
            </div>
          </div>
        </template>
      </draggable>
      <el-empty
        v-if="!directories || directories.length === 0"
        description="暂无工作目录"
        :image-size="80"
      />
    </div>
```

**Step 2: 添加 script 引入和处理函数**

在 `<script setup>` 中：

添加 import：
```javascript
import draggable from 'vuedraggable'
import { ReorderDirectories } from '../../wailsjs/go/main/App'
```

添加拖拽处理函数（在 `handleDelete` 函数之后）：
```javascript
// --- 拖拽排序 ---
const onDragEnd = async () => {
  const ids = directories.value.map(d => d.id)
  try {
    const result = await ReorderDirectories(ids)
    if (!result) {
      ElMessage.error('排序保存失败')
      emit('change')
    }
  } catch (error) {
    ElMessage.error('排序保存失败')
    emit('change')
  }
}
```

注意：`directories` 是 props，vuedraggable 的 `:list` 绑定会直接修改 props 数组顺序。由于父组件 `Home.vue` 持有 `directories` 的 ref，拖拽后数组顺序已更新，调用 `ReorderDirectories` 持久化即可。

**Step 3: 添加拖拽样式**

在 `<style scoped>` 中追加：

```css
.dir-item--ghost {
  opacity: 0.5;
  background: #c8e6c9;
}
```

**Step 4: 启动 wails dev 手动验证**

Run: `wails dev`

验证：
1. 拖拽目录项到新位置
2. 松手后刷新页面，顺序保持不变
3. 拖拽过程中有绿色半透明占位符

**Step 5: 提交**

```bash
git add frontend/src/components/DirectoryTree.vue
git commit -m "feat: DirectoryTree 集成 vuedraggable 拖拽排序"
```

---

### Task 5: 运行全量测试

**Step 1: 后端测试**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && go test ./model/ ./service/ -v`
Expected: 全部 PASS

**Step 2: 前端编译检查**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager/frontend && npm run build`
Expected: 构建成功
