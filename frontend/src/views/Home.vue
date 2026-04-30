<template>
  <div class="home">
    <el-container style="height: 100vh;">
      <!-- 顶部工具栏 -->
      <el-header style="background-color: #545c64; display: flex; align-items: center; padding: 0 20px;">
        <span style="color: white; font-size: 18px; font-weight: bold;">Git仓库管理工具</span>
        <el-divider direction="vertical" style="margin: 0 20px; border-color: #8c919a;" />
        <el-select
          v-model="selectedDirectoryId"
          placeholder="选择工作目录"
          style="width: 300px;"
          @change="onDirectoryChange"
        >
          <el-option
            v-for="dir in directories"
            :key="dir.id"
            :label="dir.name"
            :value="dir.id"
          />
        </el-select>
        <el-button
          type="primary"
          style="margin-left: 10px;"
          @click="showAddDirectoryDialog"
        >
          添加目录
        </el-button>
      </el-header>

      <!-- 主体内容 -->
      <el-container>
        <!-- 左侧文件树 -->
        <el-aside width="300px" style="border-right: 1px solid #e6e6e6; background-color: #f5f7fa;">
          <div style="padding: 10px;">
            <el-button-group style="margin-bottom: 10px;">
              <el-button size="small" @click="expandAll">全部展开</el-button>
              <el-button size="small" @click="collapseAll">全部收起</el-button>
            </el-button-group>
          </div>
          <el-tree
            v-if="selectedDirectoryId"
            ref="fileTreeRef"
            :props="treeProps"
            lazy
            :load="loadTreeNode"
            @node-click="onNodeClick"
            style="background: transparent;"
          >
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
          </el-tree>
          <el-empty v-else description="请先选择工作目录" :image-size="100" />
        </el-aside>

        <!-- 右侧操作面板 -->
        <el-main>
          <div v-if="selectedNode" style="padding: 20px;">
            <h2>{{ selectedNode.name }}</h2>
            <el-descriptions :column="2" border>
              <el-descriptions-item label="路径">{{ selectedNode.path }}</el-descriptions-item>
              <el-descriptions-item label="类型">{{ selectedNode.type === 'directory' ? '文件夹' : '文件' }}</el-descriptions-item>
            </el-descriptions>

            <el-divider />

            <div v-if="selectedNode.isGitRepo" style="margin-top: 20px;">
              <h3>Git信息</h3>
              <el-button type="primary" @click="pullRepo" :loading="gitLoading" style="margin-bottom: 10px;">
                拉取更新
              </el-button>
            </div>

            <div v-else-if="selectedNode.type === 'directory'" style="margin-top: 20px;">
              <h3>文件夹操作</h3>
              <el-button-group>
                <el-button @click="showCreateDirectoryDialog">新建文件夹</el-button>
                <el-button @click="showCreateFileDialog">新建文件</el-button>
              </el-button-group>
            </div>

            <div v-else-if="selectedNode.type === 'file'" style="margin-top: 20px;">
              <h3>文件操作</h3>
              <el-button-group>
                <el-button @click="previewFile">预览</el-button>
                <el-button @click="showRenameDialog">重命名</el-button>
                <el-button type="danger" @click="deleteFile">删除</el-button>
              </el-button-group>

              <div v-if="filePreview.content" style="margin-top: 20px;">
                <h4>文件内容</h4>
                <el-input
                  v-model="filePreview.content"
                  type="textarea"
                  :rows="10"
                  readonly
                  style="font-family: monospace;"
                />
              </div>
            </div>
          </div>
          <el-empty v-else description="请从左侧选择文件或文件夹" />
        </el-main>
      </el-container>
    </el-container>

    <!-- 添加目录对话框 -->
    <el-dialog
      v-model="addDirectoryDialogVisible"
      title="添加工作目录"
      width="500px"
    >
      <el-form :model="newDirectory" label-width="100px">
        <el-form-item label="目录名称">
          <el-input v-model="newDirectory.name" placeholder="例如: 我的工作空间" />
        </el-form-item>
        <el-form-item label="目录路径">
          <el-input v-model="newDirectory.path" placeholder="例如: C:\workspace" />
        </el-form-item>
        <el-form-item label="设为默认">
          <el-switch v-model="newDirectory.isDefault" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addDirectoryDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="addDirectory">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Folder,
  FolderOpened,
  Document,
  SuccessFilled
} from '@element-plus/icons-vue'
import { debug } from '../utils/debug'
import {
  GetDirectories, AddDirectory,
  GetFileTree, GetGitInfo,
  CreateDirectory, CreateFile, RenameFile, DeleteFile, PreviewFile,
  PullRepo
} from '../../wailsjs/go/main/App'

// 数据
const directories = ref([])
const selectedDirectoryId = ref('')
const fileTreeData = ref([])
const selectedNode = ref(null)
const fileTreeRef = ref()
const gitLoading = ref(false)

const addDirectoryDialogVisible = ref(false)
const newDirectory = ref({
  name: '',
  path: '',
  isDefault: false
})

const filePreview = ref({
  content: '',
  error: ''
})

const treeProps = {
  label: 'name',
  children: 'children',
  isLeaf: 'isLeaf'
}

// 方法
const loadDirectories = async () => {
  const dirs = await GetDirectories()
  directories.value = dirs || []

  // 自动选择默认目录
  const defaultDir = dirs.find(d => d.isDefault)
  if (defaultDir) {
    selectedDirectoryId.value = defaultDir.id
  } else if (dirs.length > 0) {
    selectedDirectoryId.value = dirs[0].id
  }
}

const onDirectoryChange = async () => {
  debug.log('Directory changed to:', selectedDirectoryId.value)

  // 强制树组件重新加载根节点
  if (fileTreeRef.value) {
    fileTreeRef.value.loadData()
  }
}

const loadTreeNode = async (node, resolve) => {
  debug.log('loadTreeNode called, node:', node)
  debug.log('node.level:', node?.level)
  debug.log('node.data:', node?.data)

  let path
  // 判断是否为根节点（level === 0 或者 node.data 为空）
  if (!node || node.level === 0 || !node.data) {
    // 根节点加载 - 获取当前选中的目录路径
    const dir = directories.value.find(d => d.id === selectedDirectoryId.value)
    if (!dir) {
      debug.log('No directory found for ID:', selectedDirectoryId.value)
      resolve([])
      return
    }
    path = dir.path
    debug.log('Loading root node for path:', path)
  } else {
    // 子节点加载
    path = node.data.path
    debug.log('Loading child nodes for path:', path)
  }

  try {
    const nodes = await GetFileTree(path)
    debug.log('Got nodes for path', path, ':', nodes)

    // 确保每个节点都有正确的 isLeaf 属性
    const processedNodes = (nodes || []).map(n => ({
      ...n,
      isLeaf: n.type === 'file' || !n.hasChildren
    }))

    debug.log('Processed nodes:', processedNodes)
    resolve(processedNodes)
  } catch (error) {
    console.error('Error in loadTreeNode:', error)
    ElMessage.error('加载节点失败: ' + (error.message || error))
    resolve([])
  }
}

const onNodeClick = async (data) => {
  selectedNode.value = data

  // 清空文件预览（切换节点时）
  filePreview.value = {
    content: '',
    error: ''
  }

  // 如果是Git仓库，获取Git信息
  if (data.isGitRepo) {
    const info = await GetGitInfo(data.path)
    Object.assign(selectedNode.value, info)
  }
}

const expandAll = () => {
  // TODO: 实现全部展开
  ElMessage.info('功能开发中')
}

const collapseAll = () => {
  // TODO: 实现全部收起
  ElMessage.info('功能开发中')
}

const showAddDirectoryDialog = () => {
  newDirectory.value = {
    name: '',
    path: '',
    isDefault: false
  }
  addDirectoryDialogVisible.value = true
}

const addDirectory = async () => {
  if (!newDirectory.value.name || !newDirectory.value.path) {
    ElMessage.error('请填写完整信息')
    return
  }

  const result = await AddDirectory(
    newDirectory.value.name,
    newDirectory.value.path,
    newDirectory.value.isDefault
  )

  if (result) {
    ElMessage.success('添加成功')
    addDirectoryDialogVisible.value = false
    await loadDirectories()
  } else {
    ElMessage.error('添加失败')
  }
}

const pullRepo = async () => {
  if (!selectedNode.value) return

  gitLoading.value = true
  const result = await PullRepo(selectedNode.value.path)
  gitLoading.value = false

  ElMessage.success(result || '拉取完成')
}

const showCreateDirectoryDialog = () => {
  ElMessage.info('功能开发中')
}

const showCreateFileDialog = () => {
  ElMessage.info('功能开发中')
}

const showRenameDialog = () => {
  ElMessage.info('功能开发中')
}

const deleteFile = async () => {
  if (!selectedNode.value) return

  try {
    await ElMessageBox.confirm('确定要删除吗？', '警告', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }

  const result = await DeleteFile(selectedNode.value.path)
  if (result) {
    ElMessage.success('删除成功')
    // 刷新文件树
    onDirectoryChange()
  } else {
    ElMessage.error('删除失败')
  }
}

const previewFile = async () => {
  if (!selectedNode.value) return

  const preview = await PreviewFile(selectedNode.value.path)
  filePreview.value = preview

  if (preview.error) {
    ElMessage.error('预览失败: ' + preview.error)
  } else if (preview.tooLarge) {
    ElMessage.warning('文件过大，无法预览')
  } else if (preview.isBinary) {
    ElMessage.warning('二进制文件，无法预览')
  }
}

// 生命周期
onMounted(async () => {
  await loadDirectories()
  // 在懒加载模式下，文件树会自动加载，不需要手动调用 loadFileTree
  debug.log('Directories loaded:', directories.value)
  debug.log('Selected directory ID:', selectedDirectoryId.value)
})
</script>

<style scoped>
.home {
  font-family: 'Microsoft YaHei', Arial, sans-serif;
}

.custom-tree-node {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 14px;
  padding-right: 8px;
}

.el-header {
  padding: 0 20px;
  box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}

.el-aside {
  overflow-y: auto;
}

.el-main {
  background-color: #fff;
  overflow-y: auto;
}
</style>
