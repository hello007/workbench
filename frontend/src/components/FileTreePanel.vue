<template>
  <div class="file-tree-aside">
    <div class="tree-toolbar">
      <div class="tree-toolbar-row">
        <el-button-group>
          <el-button size="small" @click="refreshAll">刷新</el-button>
          <el-button size="small" @click="expandAll" :loading="expanding">全部展开</el-button>
          <el-button size="small" @click="collapseAll">全部收起</el-button>
          <!-- 聚合视图切换（双模式共用一个面板）：激活态用 primary 填充标识当前模式。
               浏览器通道隐藏入口：GetAggregatedRootNodes 已列入 RPCHiddenMethods，
               点击必失败；与 SettingsPanel/useIPBans 隐私入口「启动期隐藏而非点击报错」先例一致 -->
          <el-button
            v-if="!browserChannel"
            size="small"
            :type="aggregatedMode ? 'primary' : 'default'"
            data-action="toggle-aggregated-view"
            :title="aggregatedMode ? '当前为聚合视图，点击切回目录树' : '聚合视图：统一浏览全部收藏目录'"
            @click="toggleAggregatedMode"
          >{{ aggregatedMode ? '聚合视图' : '目录树' }}</el-button>
        </el-button-group>
        <!-- 工具栏扩展位：仓库筛选器等外部按钮通过具名 slot 注入 -->
        <slot name="toolbar-extra" />
      </div>
      <el-input
        ref="treeFilterInputRef"
        v-model="treeFilterKeyword"
        size="small"
        clearable
        :prefix-icon="Search"
        :placeholder="treeFilterPlaceholder"
        class="tree-filter-input"
        @input="onFilterInput"
        @clear="exitFilterMode"
        @keydown.esc.prevent="exitFilterMode"
      />
    </div>
    <div class="tree-content" @contextmenu.prevent="onBlankAreaContextMenu">
      <!-- 目录内按名筛选覆盖层（perf-baseline §15.6 截断配套）：激活时遮盖树视图。
           树用 v-show 保持挂载（v-if 卸载会丢展开态 + 重挂载后 locateNode 必 miss），
           覆盖层条件含 selectedDirectoryId 防无目录空态被顶替（聚合视图不依赖工作目录，同放行）；
           v-for 命中列表复用 truncateTreeNodes 限渲染上限 -->
      <div v-if="filterModeActive && (aggregatedMode || directoryStore.selectedDirectoryId)" class="tree-filter-overlay">
        <div class="tree-filter-summary">
          <!-- 聚合视图未点击目录时无作用域：不回退工作目录，显式引导避免误读为「工作目录内 0 命中」 -->
          <template v-if="aggregatedMode && !treeFilterScope">请先点击一个收藏目录，再按名筛选</template>
          <template v-else>在「{{ treeFilterScopeName }}」中命中 {{ filteredNodes.length }} 项</template>
        </div>
        <template v-if="filteredDisplayNodes.length > 0">
          <div
            v-for="item in filteredDisplayNodes"
            :key="item.path"
            class="filter-result-node"
            :class="{ 'truncation-hint-node': isTruncationHint(item) }"
            @click="onFilterResultClick(item)"
            @contextmenu.prevent.stop="onNodeContextMenu($event, item)"
          >
            <el-icon
              v-if="isTruncationHint(item)"
              color="#909399"
              style="margin-right: 5px;"
            >
              <MoreFilled />
            </el-icon>
            <el-icon
              v-else-if="item.type === 'directory'"
              color="#909399"
              style="margin-right: 5px;"
            >
              <Folder />
            </el-icon>
            <template v-else>
              <img
                v-if="getIconForFile(item.name)"
                :src="getIconForFile(item.name)"
                class="tree-node-file-icon"
                :alt="item.name"
              />
              <el-icon v-else color="#606266" style="margin-right: 5px;">
                <Document />
              </el-icon>
            </template>
            <span
              class="filter-result-name"
              :style="{ fontStyle: isTruncationHint(item) ? 'italic' : 'normal', color: isTruncationHint(item) ? '#909399' : '#606266' }"
            >{{ item.name }}</span>
          </div>
        </template>
        <!-- 聚合视图未点击目录时无作用域：summary 已给出引导，不再叠加「无匹配项」矛盾文案 -->
        <el-empty v-else-if="treeFilterScope || !aggregatedMode" description="无匹配项" :image-size="60" />
      </div>
      <el-tree
        v-show="(!filterModeActive || (aggregatedMode && !treeFilterScope)) && (aggregatedMode || !!directoryStore.selectedDirectoryId) && !(aggregatedMode && !aggregatedRootLoading && !aggregatedRootError && aggregatedRootCount === 0)"
        :key="treeKey"
        ref="fileTreeRef"
        :props="treeProps"
        node-key="path"
        lazy
        :expand-on-click-node="false"
        highlight-current
        :load="loadTreeNode"
        @node-click="onNodeClick"
        @node-contextmenu="onNodeContextMenu"
        @node-expand="closeContextMenu"
        @node-collapse="closeContextMenu"
        class="file-tree"
      >
        <template #default="{ node, data }">
          <span
            class="custom-tree-node"
            :class="{ 'truncation-hint-node': isTruncationHint(data), 'missing-node': data.missing }"
            :title="data.missing ? MISSING_FAVORITE_HINT : undefined"
          >
            <el-icon
              v-if="isTruncationHint(data)"
              color="#909399"
              style="margin-right: 5px;"
            >
              <MoreFilled />
            </el-icon>
            <el-icon
              v-else-if="data.type === 'directory'"
              :color="data.missing ? '#c0c4cc' : (node.expanded ? '#409EFF' : '#909399')"
              style="margin-right: 5px;"
            >
              <component :is="node.expanded ? FolderOpened : Folder" />
            </el-icon>
            <template v-else>
              <img
                v-if="getIconForFile(data.name)"
                :src="getIconForFile(data.name)"
                class="tree-node-file-icon"
                :alt="data.name"
              />
              <el-icon v-else color="#606266" style="margin-right: 5px;">
                <Document />
              </el-icon>
            </template>
            <span :style="{
              color: isTruncationHint(data)
                ? '#909399'
                : data.missing
                  ? '#c0c4cc'
                  : data.type === 'directory'
                    ? (node.expanded ? '#409EFF' : '#909399')
                    : '#606266',
              fontStyle: isTruncationHint(data) ? 'italic' : 'normal',
              textDecoration: data.missing ? 'line-through' : 'none'
            }">
              {{ node.label }}
            </span>
            <img
              v-if="data.isGitRepo"
              :src="data.hasRemote ? gitIcon : gitGrayIcon"
              class="tree-node-git-img"
              :alt="data.hasRemote ? 'Git 仓库' : 'Git 仓库（无远程）'"
              :title="data.hasRemote ? 'Git 仓库' : 'Git 仓库（未配置远程）'"
            />
          </span>
        </template>
      </el-tree>
      <el-empty v-if="!directoryStore.selectedDirectoryId && !aggregatedMode" description="请先选择工作目录" :image-size="100" />
      <!-- 聚合根层三态空态：加载中 / 加载失败 / 无目录收藏，避免加载瞬间与失败态误显示「暂无目录收藏」 -->
      <el-empty v-if="aggregatedMode && !filterModeActive && aggregatedRootLoading" description="正在加载收藏目录…" :image-size="100" />
      <el-empty v-else-if="aggregatedMode && !filterModeActive && aggregatedRootError" :description="aggregatedRootError" :image-size="100" />
      <el-empty v-else-if="aggregatedMode && !filterModeActive && aggregatedRootCount === 0" description="暂无目录收藏：在目录树上右键文件夹「添加到收藏」后，即可在此统一浏览" :image-size="100" />
    </div>

    <!-- 新建文件夹/文件对话框 -->
    <el-dialog
      v-model="createDialogVisible"
      :title="createType === 'directory' ? '新建文件夹' : '新建文件'"
      width="420px"
      append-to-body
    >
      <el-form label-width="80px">
        <el-form-item label="父文件夹">
          <el-input :model-value="createParentPath" disabled />
        </el-form-item>
        <el-form-item :label="createType === 'directory' ? '文件夹名' : '文件名'">
          <el-input
            ref="createInputRef"
            v-model="createName"
            :placeholder="createType === 'directory' ? '例如: src' : '例如: main.go'"
            :disabled="createLoading"
            @keyup.enter="handleCreate"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialogVisible = false" :disabled="createLoading">取消</el-button>
        <el-button type="primary" @click="handleCreate" :loading="createLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 重命名对话框 -->
    <el-dialog
      v-model="renameDialogVisible"
      title="重命名"
      width="420px"
      append-to-body
    >
      <el-form label-width="80px">
        <el-form-item label="当前名称">
          <el-input :model-value="renameNode?.name" disabled />
        </el-form-item>
        <el-form-item label="新名称">
          <el-input
            ref="renameInputRef"
            v-model="renameName"
            placeholder="请输入新名称"
            :disabled="renameLoading"
            @keyup.enter="handleRename"
            @keydown.esc="renameDialogVisible = false"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="renameDialogVisible = false" :disabled="renameLoading">取消</el-button>
        <el-button type="primary" @click="handleRename" :loading="renameLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 拷贝到对话框 -->
    <el-dialog
      v-model="copyToDialogVisible"
      title="拷贝到"
      width="480px"
      append-to-body
    >
      <el-form label-width="100px">
        <el-form-item label="原地址">
          <el-input
            v-model="copyToSourcePath"
            placeholder="请输入原文件或文件夹路径"
            :disabled="copyToLoading"
          />
        </el-form-item>
        <div class="swap-row">
          <el-button
            text
            size="small"
            :disabled="copyToLoading"
            @click="swapCopyToPaths"
          >
            <el-icon class="swap-icon"><Sort /></el-icon>
            互换
          </el-button>
        </div>
        <el-form-item label="目标地址">
          <el-input
            ref="copyToTargetInputRef"
            v-model="copyToTargetPath"
            placeholder="请输入目标文件夹路径"
            :disabled="copyToLoading"
            @keyup.enter="handleCopyTo"
          />
        </el-form-item>
        <el-form-item label="文件名">
          <el-input
            v-model="copyToTargetName"
            placeholder="默认与源同名（文件/文件夹均生效），可修改"
            :disabled="copyToLoading"
            clearable
            @keyup.enter="handleCopyTo"
          />
        </el-form-item>
        <el-form-item>
          <el-checkbox
            v-model="copyToWholeDir"
            :disabled="copyToLoading"
          >
            包含文件夹本身
          </el-checkbox>
        </el-form-item>
      </el-form>
      <div v-if="copyToPreview" class="copy-to-preview">
        <div class="copy-to-preview-label">拷贝效果预览</div>
        <div class="copy-to-preview-row">{{ copyToPreview.from }}</div>
        <div class="copy-to-preview-arrow">↓</div>
        <div class="copy-to-preview-row">{{ copyToPreview.to }}</div>
      </div>
      <template #footer>
        <el-button @click="copyToDialogVisible = false" :disabled="copyToLoading">取消</el-button>
        <el-button type="primary" @click="handleCopyTo" :loading="copyToLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 自定义右键菜单 -->
    <ul
      v-if="contextMenu.visible"
      class="context-menu"
      :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }"
      @click.stop
      @mousedown.stop
    >
      <!-- 聚合视图空白区：虚拟根不落盘、无明确新建落点，仅保留刷新与粘贴。
           粘贴目标 = 最后点击的目录节点（contextMenu.data 已由 onBlankAreaContextMenu 绑定）；
           无有效目标时隐藏粘贴项。工作目录语义的仓库筛选器/统计/看板项一并隐藏。 -->
      <template v-if="contextMenu.isBlankArea && aggregatedMode">
        <li class="context-menu-item" data-action="aggregated-blank-refresh" @click="onMenuCommand('refresh')">
          <el-icon><Refresh /></el-icon>刷新
        </li>
        <li v-if="contextMenu.data?.path" class="context-menu-item" data-action="aggregated-blank-paste" @click="onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
        </li>
      </template>
      <template v-else-if="contextMenu.isBlankArea">
        <li class="context-menu-item" @click="onMenuCommand('createFile')">
          <el-icon><DocumentAdd /></el-icon>新建文件
        </li>
        <li class="context-menu-item" @click="onMenuCommand('createDir')">
          <el-icon><FolderAdd /></el-icon>新建文件夹
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('openRepoFilter')">
          <el-icon><Search /></el-icon>仓库筛选器
        </li>
        <li v-if="blankAreaIsGitRepo" class="context-menu-item" @click="onMenuCommand('jumpStats')">
          <el-icon><TrendCharts /></el-icon>跳转仓库统计
        </li>
        <li v-if="blankAreaIsGitRepo" class="context-menu-item" @click="onMenuCommand('toggleDashboardPin')">
          <el-icon><DataBoard /></el-icon>{{ contextMenu.targetPinned ? '取消关注状态看板' : '加入状态看板' }}
        </li>
      </template>
      <!-- 聚合视图失效收藏节点：仅保留移除收藏与复制路径（其余文件操作对不存在路径无意义） -->
      <template v-else-if="contextMenu.data?.missing">
        <li class="context-menu-item" @click="onMenuCommand('removeFavorite')">
          <el-icon><StarFilled /></el-icon>移除收藏
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copyPath')">
          <el-icon><CopyDocument /></el-icon>复制路径
        </li>
      </template>
      <template v-else-if="contextMenu.data?.type === 'directory'">
        <li class="context-menu-item" @click="onMenuCommand('createFile')">
          <el-icon><DocumentAdd /></el-icon>新建文件
        </li>
        <li class="context-menu-item" @click="onMenuCommand('createDir')">
          <el-icon><FolderAdd /></el-icon>新建文件夹
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('rename')">
          <el-icon><Edit /></el-icon>重命名
          <span class="context-menu-shortcut">{{ settingsStore.shortcutRename }}</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('delete')">
          <el-icon><Delete /></el-icon>删除
          <span class="context-menu-shortcut">{{ settingsStore.shortcutDelete }}</span>
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('cut')">
          <el-icon><Scissor /></el-icon>剪切
          <span class="context-menu-shortcut">Ctrl+X</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copy')">
          <el-icon><CopyDocument /></el-icon>复制
          <span class="context-menu-shortcut">Ctrl+C</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
          <span class="context-menu-shortcut">Ctrl+V</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copyTo')">
          <el-icon><FolderAdd /></el-icon>拷贝到...
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item has-submenu">
          <el-icon><CopyDocument /></el-icon>复制路径
          <span class="context-menu-arrow">›</span>
          <ul class="context-submenu">
            <li class="context-menu-item" @click="onMenuCommand('copyPath')">复制绝对路径</li>
            <li class="context-menu-item" @click="onMenuCommand('copyRelativePath')">复制相对路径</li>
            <li class="context-menu-item" @click="onMenuCommand('copyRefPath')">文件引用路径</li>
            <li class="context-menu-item" @click="onMenuCommand('copyName')">复制文件名</li>
          </ul>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openExplorer')">
          <img :src="explorerIcon" class="context-menu-img-icon" alt="资源管理器" />在资源管理器中打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
          <img :src="vscodeIcon" class="context-menu-img-icon" alt="VSCode" />用 VSCode 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInWarp')">
          <img :src="warpIcon" class="context-menu-img-icon" alt="Warp" />用 Warp 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInObsidian')">
          <img :src="obsidianIcon" class="context-menu-img-icon" alt="Obsidian" />用 Obsidian 打开
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('refresh')">
          <el-icon><Refresh /></el-icon>刷新
          <span class="context-menu-shortcut">F5</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('pullRepos')">
          <el-icon><Refresh /></el-icon>更新仓库
        </li>
        <li class="context-menu-item" @click="onMenuCommand('contentSearch')">
          <el-icon><Search /></el-icon>在此目录中搜索
        </li>
        <li class="context-menu-divider" />
        <li v-if="isFavorited" class="context-menu-item" @click="onMenuCommand('removeFavorite')">
          <el-icon><StarFilled /></el-icon>取消收藏
        </li>
        <li v-else class="context-menu-item" @click="onMenuCommand('addFavorite')">
          <el-icon><Star /></el-icon>添加到收藏
        </li>
        <li v-if="aggregatedMode" class="context-menu-item" @click="onMenuCommand('setAsWorkDir')">
          <el-icon><FolderAdd /></el-icon>设为工作目录
        </li>
        <li v-else class="context-menu-item" @click="onMenuCommand('addAsWorkDir')">
          <el-icon><FolderAdd /></el-icon>添加为工作目录
        </li>
        <li class="context-menu-item" @click="onMenuCommand('addToAiChat')">
          <el-icon><ChatDotRound /></el-icon>添加到 AI 对话
        </li>
        <li v-if="contextMenu.data?.isGitRepo" class="context-menu-item" @click="onMenuCommand('jumpStats')">
          <el-icon><TrendCharts /></el-icon>跳转仓库统计
        </li>
        <li v-if="contextMenu.data?.isGitRepo" class="context-menu-item" @click="onMenuCommand('toggleDashboardPin')">
          <el-icon><DataBoard /></el-icon>{{ contextMenu.targetPinned ? '取消关注状态看板' : '加入状态看板' }}
        </li>
      </template>
      <template v-else>
        <li class="context-menu-item" @click="onMenuCommand('rename')">
          <el-icon><Edit /></el-icon>重命名
          <span class="context-menu-shortcut">{{ settingsStore.shortcutRename }}</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('delete')">
          <el-icon><Delete /></el-icon>删除
          <span class="context-menu-shortcut">{{ settingsStore.shortcutDelete }}</span>
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('cut')">
          <el-icon><Scissor /></el-icon>剪切
          <span class="context-menu-shortcut">Ctrl+X</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copy')">
          <el-icon><CopyDocument /></el-icon>复制
          <span class="context-menu-shortcut">Ctrl+C</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
          <span class="context-menu-shortcut">Ctrl+V</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copyTo')">
          <el-icon><FolderAdd /></el-icon>拷贝到...
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item has-submenu">
          <el-icon><CopyDocument /></el-icon>复制路径
          <span class="context-menu-arrow">›</span>
          <ul class="context-submenu">
            <li class="context-menu-item" @click="onMenuCommand('copyPath')">复制绝对路径</li>
            <li class="context-menu-item" @click="onMenuCommand('copyRelativePath')">复制相对路径</li>
            <li class="context-menu-item" @click="onMenuCommand('copyRefPath')">文件引用路径</li>
            <li class="context-menu-item" @click="onMenuCommand('copyName')">复制文件名</li>
          </ul>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openExplorer')">
          <img :src="explorerIcon" class="context-menu-img-icon" alt="资源管理器" />在资源管理器中打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
          <img :src="vscodeIcon" class="context-menu-img-icon" alt="VSCode" />用 VSCode 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInWarp')">
          <img :src="warpIcon" class="context-menu-img-icon" alt="Warp" />用 Warp 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInObsidian')">
          <img :src="obsidianIcon" class="context-menu-img-icon" alt="Obsidian" />用 Obsidian 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openWithDefaultApp')">
          <el-icon><Open /></el-icon>用默认程序打开
        </li>
        <li v-if="isPptxFile(contextMenu.data)" class="context-menu-item" @click="onMenuCommand('splitPptx')">
          <el-icon><Files /></el-icon>按页拆分
        </li>
        <!-- 查看历史（git 域）：--follow 追溯单文件演进史含重命名。仅文件节点
             （PRD 决策：目录入口不做，--follow 仅支持单一路径）；普通模式按工作目录
             判定仓库归属；聚合模式收藏根多为仓库且无廉价单文件归属判定，恒显示，
             非 git 目录点击后由后端 FindGitRoot 失败提示兜底 -->
        <li v-if="fileHistoryAvailable" class="context-menu-item" data-action="file-history" @click="onMenuCommand('fileHistory')">
          <el-icon><Clock /></el-icon>查看历史
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('refresh')">
          <el-icon><Refresh /></el-icon>刷新
          <span class="context-menu-shortcut">F5</span>
        </li>
        <li class="context-menu-divider" />
        <li v-if="isFavorited" class="context-menu-item" @click="onMenuCommand('removeFavorite')">
          <el-icon><StarFilled /></el-icon>取消收藏
        </li>
        <li v-else class="context-menu-item" @click="onMenuCommand('addFavorite')">
          <el-icon><Star /></el-icon>添加到收藏
        </li>
      </template>
    </ul>

    <!-- 文件历史弹窗（文件右键「查看历史」）：--follow 追溯单文件演进史含重命名 -->
    <FileHistoryDialog
      v-model="fileHistoryVisible"
      :repo-path="fileHistoryPath"
      :file-abs-path="fileHistoryPath"
    />
  </div>
</template>

<script setup>
import { ref, computed, reactive, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Folder,
  FolderOpened,
  Document,
  FolderAdd,
  DocumentAdd,
  Edit,
  Delete,
  CopyDocument,
  Refresh,
  Open,
  Scissor,
  DocumentCopy,
  Star,
  StarFilled,
  Search,
  Sort,
  TrendCharts,
  DataBoard,
  ChatDotRound,
  MoreFilled,
  Files,
  Clock
} from '@element-plus/icons-vue'
import { debug } from '../utils/debug'
import { getIconForFile } from '../utils/fileIconMap'
import { truncateTreeNodes, TRUNCATION_HINT_TYPE } from '../utils/treeTruncate'
import { filterTreeNodesByName } from '../utils/treeFilter'
import { toRelativePath, toFileRefPath } from '../utils/relativePath'
import { useTreeState } from '../composables/useTreeState'
import { useFavoritesStore, useSettingsStore, useDirectoryStore, useWorkspaceStore, useUiStore, useAiChatStore } from '../store'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { isBrowserChannel } from '../transport'
import {
  GetFileTree,
  GetAggregatedRootNodes,
  InvalidateFileTreeCache,
  ClearAllFileTreeCache,
  CreateDirectory, CreateFile, RenameFile, DeleteFile,
  OpenInExplorer,
  OpenInVSCode,
  OpenInWarp,
  OpenWithDefaultApp,
  OpenInObsidian,
  OpenObsidianVaultManager,
  CopyObsidianVaultPath,
  AutoRegisterAndOpen,
  ScanAndPullRepos,
  IsDashboardPinned,
  AddDashboardPin,
  RemoveDashboardPin,
  SplitPptxPerPage
} from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import FileHistoryDialog from './FileHistoryDialog.vue'
import obsidianIcon from '../assets/icons/obsidian.png'
import explorerIcon from '../assets/icons/explorer.png'
import vscodeIcon from '../assets/icons/vscode.ico'
import warpIcon from '../assets/icons/warp.ico'
import gitIcon from '../assets/icons/git.png'
import gitGrayIcon from '../assets/icons/git-gray.png'

// ---- Props & Emits ----
// clipboard prop 已删（workspace 域批次6 迁 store，本组件零消费）。
// directories / selectedDirId 已迁 directory store，子组件直读。
const emit = defineEmits(['select', 'batchPull', 'copy', 'cut', 'paste', 'copyTo', 'contextmenu', 'delete', 'add-work-dir', 'set-as-work-dir', 'open-content-search', 'open-repo-filter'])

const { saveState, restoreState } = useTreeState()
const favoritesStore = useFavoritesStore()
const settingsStore = useSettingsStore()
const directoryStore = useDirectoryStore()
const workspaceStore = useWorkspaceStore()
const uiStore = useUiStore()
const aiChatStore = useAiChatStore()

// ---- Refs ----
const currentSelectedPath = ref('')
const fileTreeRef = ref()
const refreshCounter = ref(0)

// ---- 聚合视图模式（虚拟工作区：收藏目录平铺浏览）----
// 聚合模式下树根为虚拟节点（不落盘不持久化，仅 loadTreeNode level 0 分流时前端分流），
// 其子节点 = 收藏目录列表（后端 GetAggregatedRootNodes 一次返回存在性判定 + git 徽标），
// 后续展开/预览/编辑/文件操作全部复用既有绝对路径链路。
// 双下划线前缀保留命名空间：与真实目录路径天然不冲突，普通/聚合两模式树状态互不污染。
const AGGREGATED_TREE_STATE_KEY = '__aggregated__'
// el-tree 的 key 命名空间：聚合树与工作目录无关，不绑定 selectedDirectoryId，
// 避免切换工作目录时整棵聚合树重建重拉（展开态先丢后恢复）
const AGGREGATED_TREE_KEY = '__agg__'
// 失效收藏占位提示口径（tooltip / 点击提示 / locateNode 同源）：
// 不区分「已删除 / 无权限 / 网络盘不可达」——前端不掌握原因，断言「已移动」会把
// 临时断开的收藏误导为需要移除；也不暗示这是目录——文件类型收藏被删后同样是 Missing 占位
const MISSING_FAVORITE_HINT = '收藏项已失效或不可访问，可右键移除收藏'
const aggregatedMode = ref(false)
// 聚合根层节点数：驱动「无目录收藏」空态显示；树每次重建时重置
const aggregatedRootCount = ref(0)
// 聚合根层加载三态：加载中/失败不显示「暂无目录收藏」，避免误判收藏数据丢失
const aggregatedRootLoading = ref(false)
const aggregatedRootError = ref('')

// 浏览器通道（启动期固定，非响应式）：聚合根查询在后端 RPC 面屏蔽，
// 入口隐藏而非点击报错（与 SettingsPanel / useIPBans 隐私入口先例一致）
const browserChannel = isBrowserChannel()

const treeKey = computed(() =>
  `${aggregatedMode.value ? AGGREGATED_TREE_KEY : directoryStore.selectedDirectoryId}_${refreshCounter.value}`
)

let treeReadyResolve = null
let treeReadyPromise = new Promise(r => { treeReadyResolve = r })

function resetTreeReady() {
  treeReadyPromise = new Promise(r => { treeReadyResolve = r })
}

// 等待树根就绪：树已有根节点直接通过；否则由 loadTreeNode 完成信号驱动
// （普通树根层/聚合根层加载结束时 resolve），超时兜底防懒加载异常导致流程永久挂起。
// 空收藏时聚合根层立即完成，不再空等数秒。
const waitTreeReady = async (timeout = 3000) => {
  if (fileTreeRef.value?.store?.root?.childNodes?.length > 0) return
  await Promise.race([
    treeReadyPromise,
    new Promise(resolve => setTimeout(resolve, timeout))
  ])
}

const treeProps = {
  label: 'name',
  children: 'children',
  isLeaf: 'isLeaf'
}

// ---- 右键菜单状态 ----
// targetPinned：右键打开菜单时异步查询 IsDashboardPinned 缓存的 pin 状态，
// 用于「加入/取消关注状态看板」菜单项文案切换。关闭菜单时不重置（onMenuCommand
// 先 closeContextMenu 再调 handleToggleDashboardPin，后者仍需读 targetPinned），
// 每次打开菜单时重置为 false 再按查询结果更新。
const contextMenu = reactive({
  visible: false,
  x: 0,
  y: 0,
  data: null,
  isBlankArea: false,
  targetPinned: false
})

// 路径比较归一：分隔符统一为反斜杠、去尾部分隔符、大小写不敏感（Windows 路径语义）。
// 去尾分隔符保证 D:\repo\ 与 D:\repo 视为同一路径，收藏判重与聚合子树归属共用同一口径。
const normalizeComparablePath = (p) => (p || '').replace(/\//g, '\\').replace(/\\+$/, '').toLowerCase()

const isFavorited = computed(() => {
  const favList = favoritesStore.favorites
  const path = contextMenu.data?.path
  if (!path) return false
  // 收藏路径可能带尾分隔符（D:\repo\），节点路径不带：按归一后比较，否则误显示「添加到收藏」
  const key = normalizeComparablePath(path)
  return favList.some(f => normalizeComparablePath(f.path) === key)
})

// 空白区右键时判断当前工作目录是否 git 仓库（空白区 data 无 isGitRepo 字段，回查工作目录）
const blankAreaIsGitRepo = computed(() => {
  const dir = directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)
  return dir?.isGitRepo || false
})

// ---- 新建对话框状态 ----
const createDialogVisible = ref(false)
const createType = ref('directory')
const createName = ref('')
const createLoading = ref(false)
const createParentData = ref(null)
const createParentPath = computed(() => createParentData.value?.path || '')

// ---- 重命名对话框状态 ----
const renameDialogVisible = ref(false)
const renameName = ref('')
const renameLoading = ref(false)
const createInputRef = ref()
const renameInputRef = ref()
const renameNode = ref(null)

// ---- 拷贝到对话框状态 ----
const copyToDialogVisible = ref(false)
const copyToSourcePath = ref('')
const copyToTargetPath = ref('')
const copyToWholeDir = ref(true)
const copyToLoading = ref(false)
// 自定义目标文件名（文件/文件夹均生效），空串 = 与源同名
const copyToTargetName = ref('')
// 源是否为文件（当前未参与显示逻辑，保留备用；右键入口由节点类型确定）
const copyToFileMode = ref(false)

const copyToTargetInputRef = ref()

// 文件名输入框默认值：跟随源路径最后一段
const defaultCopyToName = () => {
  const src = copyToSourcePath.value.trim().replaceAll('\\', '/')
  return src.split('/').pop() || ''
}

// 源路径变化时同步默认名（文件/目录均跟随）：仅当用户未自行改名
// （当前值为空或等于旧默认名）时跟随，避免覆盖用户已输入的自定义名（对话框内原地址输入框可编辑）
watch(copyToSourcePath, (newVal, oldVal) => {
  const oldSrc = (oldVal || '').trim().replaceAll('\\', '/')
  const oldDefault = oldSrc.split('/').pop() || ''
  if (!copyToTargetName.value || copyToTargetName.value === oldDefault) {
    copyToTargetName.value = defaultCopyToName()
  }
})

const copyToPreview = computed(() => {
  const src = copyToSourcePath.value.trim().replaceAll('\\', '/')
  const dst = copyToTargetPath.value.trim().replaceAll('\\', '/')
  if (!src || !dst) return null

  const srcName = src.split('/').pop() || ''
  const normalizedDst = dst.replace(/\/+$/, '')
  if (copyToWholeDir.value) {
    const finalName = copyToTargetName.value.trim() || srcName
    return { from: src, to: normalizedDst + '/' + finalName }
  }
  return { from: src + '/*', to: normalizedDst + '/*' }
})

// 互换原地址与目标地址；文件名重置为新源路径的默认值（源已变，旧自定义名不再适用）
const swapCopyToPaths = () => {
  const temp = copyToSourcePath.value
  copyToSourcePath.value = copyToTargetPath.value
  copyToTargetPath.value = temp
  copyToTargetName.value = defaultCopyToName()
}

// ---- 目录内按名筛选（截断优化配套，perf-baseline §15.6 盲区补齐）----
// 覆盖层模式：筛选激活时 tree-content 切换为扁平命中列表，不碰 el-tree 懒加载/
// 截断哨兵数据流。数据源 GetFileTree(作用域) 全量拉取（Go 侧缓存命中，10k ~35ms），
// 前端按名过滤不受 2000 截断限制。
const treeFilterKeyword = ref('')
const treeFilterInputRef = ref()
const filteredNodes = ref([])
// 作用域：筛选前最后点击的目录节点；无点击/点击为文件时回退工作目录根层
const lastClickedNodeData = ref(null)
let filterDebounceTimer = null
// 竞态守卫：连续输入多次触发 runFilter 并发，旧响应晚到须丢弃防结果回跳
let filterSeq = 0

const currentWorkDir = computed(() =>
  directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)
)

const filterModeActive = computed(() => treeFilterKeyword.value.trim().length > 0)

const treeFilterScope = computed(() => {
  const d = lastClickedNodeData.value
  if (d && d.type === 'directory') return d
  return null
})

const treeFilterScopeName = computed(() =>
  treeFilterScope.value?.name || currentWorkDir.value?.name || ''
)

const treeFilterPlaceholder = computed(() => {
  if (treeFilterScope.value) return `在「${treeFilterScope.value.name}」内按名筛选`
  // 聚合视图无「当前工作目录」可回退：未点击目录前引导先点击
  if (aggregatedMode.value) return '点击目录后可按名筛选'
  return '在当前目录按名筛选'
})

// 命中列表渲染上限：复用 truncateTreeNodes（2000 + 末尾哨兵），防命中过多渲染劣化
const filteredDisplayNodes = computed(() => {
  const scopePath = treeFilterScope.value?.path
    || (aggregatedMode.value ? '' : currentWorkDir.value?.path)
    || ''
  return truncateTreeNodes(filteredNodes.value, scopePath).nodes
})

const onFilterInput = () => {
  clearTimeout(filterDebounceTimer)
  if (!filterModeActive.value) {
    filteredNodes.value = []
    return
  }
  // 防抖 300ms：连续输入避免逐字符拉取
  filterDebounceTimer = setTimeout(runFilter, 300)
}

const runFilter = async () => {
  const kw = treeFilterKeyword.value.trim()
  if (!kw) return
  // 聚合视图作用域恒为点击的目录节点（无工作目录回退）——收藏目录间无公共祖先，
  // 未点击就筛选无从确定作用域，直接不拉取（placeholder 已引导先点击目录）
  const scopePath = aggregatedMode.value
    ? treeFilterScope.value?.path
    : (treeFilterScope.value?.path || currentWorkDir.value?.path)
  if (!scopePath) return
  const seq = ++filterSeq
  try {
    const nodes = await GetFileTree(scopePath)
    if (seq !== filterSeq) return
    filteredNodes.value = filterTreeNodesByName(nodes || [], kw)
  } catch (error) {
    if (seq !== filterSeq) return
    console.error('筛选失败:', error)
    ElMessage.error('筛选失败: ' + (error.message || error))
    filteredNodes.value = []
  }
}

const exitFilterMode = () => {
  clearTimeout(filterDebounceTimer)
  // 递增序号使在途 runFilter 响应失效：否则 ESC/切目录后旧作用域响应仍写回
  // filteredNodes（seq 相等不被丢弃），再次输入时防抖窗口内覆盖层闪现旧目录命中
  filterSeq++
  treeFilterKeyword.value = ''
  filteredNodes.value = []
}

const focusTreeFilter = () => {
  nextTick(() => treeFilterInputRef.value?.focus?.())
}

// 命中项点击：文件 emit select 预览（保持筛选态）；目录退出筛选回树定位展开；
// 哨兵（命中超限提示）纯提示不可交互
const onFilterResultClick = (item) => {
  if (isTruncationHint(item)) return
  if (item.type === 'directory') {
    exitFilterMode()
    locateNode(item.path)
    return
  }
  // 同步树选中态：F2/Delete 快捷键经 getCurrentNode 作用于树当前节点，
  // 不同步则「预览命中文件 A、快捷键删树上残留节点 B」错位
  const tree = fileTreeRef.value
  if (tree?.getNode?.(item.path)) {
    tree.setCurrentKey(item.path)
    currentSelectedPath.value = item.path
  } else {
    // 截断层外文件树中无对应节点：清当前键防快捷键命中旧节点（getCurrentNode 返回 null 早退）
    tree?.setCurrentKey?.(null)
  }
  emit('select', item)
}

// 作用域取筛选前最后点击的目录节点（onNodeClick 处记录）
watch(treeKey, () => {
  resetTreeReady()
  // 目录切换/整体刷新全量重建树，筛选态随之重置；lastClickedNodeData 一并清空，
  // 防新目录内筛选误用上一目录的点击节点作作用域（跨目录数据泄漏）
  exitFilterMode()
  lastClickedNodeData.value = null
})

// ---- 懒加载 ----
const loadTreeNode = async (node, resolve) => {
  debug.log('loadTreeNode called, node:', node)
  debug.log('node.level:', node?.level)
  debug.log('node.data:', node?.data)

  // 捕获本次加载对应的就绪信号：加载期间若树被重建（resetTreeReady），
  // 旧加载只释放旧 promise，不会提前唤醒新一代的等待
  const readyResolve = treeReadyResolve

  let path
  if (!node || node.level === 0 || !node.data) {
    // 聚合视图：level 0 返回收藏目录节点列表（虚拟根不落盘，不进 GetFileTree 链路）
    if (aggregatedMode.value) {
      await loadAggregatedRootNodes(resolve)
      return
    }
    const dir = directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)
    if (!dir) {
      debug.log('No directory found for ID:', directoryStore.selectedDirectoryId)
      resolve([])
      // 无目录分支也须释放就绪信号：否则 waitTreeReady 等满超时，
      // 模式切换在途标志迟迟不复位
      nextTick(() => readyResolve?.())
      return
    }
    path = dir.path
    debug.log('Loading root node for path:', path)
  } else {
    path = node.data.path
    debug.log('Loading child nodes for path:', path)
  }

  try {
    const nodes = await GetFileTree(path)
    debug.log('Got nodes for path', path, ':', nodes)

    const processedNodes = (nodes || []).map(n => ({
      ...n,
      isLeaf: n.type === 'file' || !n.hasChildren
    }))

    // 单层超限截断（perf-baseline §15.4）：el-tree 无虚拟滚动，全量渲染万级节点
    // 卡死级劣化（100k ≈ 49s / +3.14GB JSHeap），截断后渲染量与本层规模解耦；
    // 哨兵节点（truncation-hint）提示剩余数量并引导 Ctrl+P 文件搜索定位。
    const truncated = truncateTreeNodes(processedNodes, path)

    debug.log('Processed nodes:', truncated.nodes)
    resolve(truncated.nodes)

    if (!node || node.level === 0 || !node.data) {
      nextTick(() => readyResolve?.())
    }
  } catch (error) {
    console.error('Error in loadTreeNode:', error)
    ElMessage.error('加载节点失败: ' + (error.message || error))
    resolve([])
    // 根层加载失败同样释放就绪信号，防 waitTreeReady 空等超时
    if (!node || node.level === 0 || !node.data) {
      nextTick(() => readyResolve?.())
    }
  }
}

// ---- 聚合视图根层加载 ----
// 把全部收藏路径交给后端聚合绑定一次判定：目录 -> 正常节点（含 git 徽标）、
// 不存在 -> Missing 占位（前端标灰 + 右键移除收藏）、文件类 -> 后端跳过不进聚合树。
// 收藏 alias 覆盖节点显示名（后端仅知路径，取路径末段兜底）。
// 路径比较归一走 normalizeComparablePath（定义在右键菜单区），避免收藏数据尾部斜杠导致子树归属失配。

// 聚合根集合：祖先已被收藏的项不再作为独立根（与收藏顺序无关，同路径只保留首个）。
// el-tree node-key=path，同一路径在根层与子层同时注册会互相覆盖 nodesMap
// （展开态/定位/刷新错位）；父收藏展开后仍可浏览到该目录，聚合语义不损失。
// 该集合同时是 findAggregatedRootFor 的匹配面——二者必须同源，否则被剔除的子收藏
// 仍会被最长前缀命中，定位/刷新落到树上不存在的一级节点。
const aggregatedFavoriteRoots = () => {
  const unique = []
  const seen = new Set()
  for (const f of favoritesStore.favorites) {
    const key = normalizeComparablePath(f.path)
    if (!key || seen.has(key)) continue
    seen.add(key)
    unique.push({ fav: f, key })
  }
  return unique
    .filter(x => !unique.some(y => y.key !== x.key && x.key.startsWith(y.key + '\\')))
    .map(x => x.fav)
}

const loadAggregatedRootNodes = async (resolve) => {
  // 同 loadTreeNode：捕获本次加载对应的就绪信号，防旧加载唤醒新一代等待
  const readyResolve = treeReadyResolve
  aggregatedRootLoading.value = true
  aggregatedRootError.value = ''
  try {
    const keptFavs = aggregatedFavoriteRoots()
    const nodes = await GetAggregatedRootNodes(keptFavs.map(f => f.path))
    const aliasByPath = new Map(keptFavs.map(f => [f.path, f.alias]))
    const processedNodes = (nodes || []).map(n => {
      const alias = aliasByPath.get(n.path)
      return {
        ...n,
        name: alias && alias.trim() ? alias : n.name,
        // 失效占位节点不可展开（磁盘上无子节点可加载），按叶子处理防展开请求打空
        isLeaf: n.type === 'file' || !!n.missing || !n.hasChildren
      }
    })
    // 收藏上限 100 理论不触发截断，统一走截断链路保持行为一致（哨兵 key 用聚合命名空间）
    const truncated = truncateTreeNodes(processedNodes, AGGREGATED_TREE_STATE_KEY)
    aggregatedRootCount.value = processedNodes.length
    resolve(truncated.nodes)
  } catch (error) {
    console.error('Error in loadAggregatedRootNodes:', error)
    // 失败态与「无收藏」区分：空态由 aggregatedRootError 分支接管，避免误判收藏丢失
    aggregatedRootError.value = '加载收藏目录失败，请点击工具栏「刷新」重试'
    ElMessage.error('加载收藏目录失败: ' + (error.message || error))
    aggregatedRootCount.value = 0
    resolve([])
  } finally {
    aggregatedRootLoading.value = false
    nextTick(() => readyResolve?.())
  }
}

// ---- 节点点击 ----
// 截断哨兵节点：不选中、不触发预览/展开；点击聚焦工具栏筛选框
// （2000 名外文件定位入口，文案见 utils/treeTruncate.js）
const isTruncationHint = (data) => data?.type === TRUNCATION_HINT_TYPE

const onNodeClick = (data, node) => {
  if (isTruncationHint(data)) {
    // el-tree highlight-current 在本回调前已 setCurrentNode，哨兵被高亮为当前节点；
    // 清高亮保持「哨兵不选中」不变式（视觉 + F2/Delete 读取的 getCurrentNode 双保险）
    fileTreeRef.value?.setCurrentKey?.(null)
    focusTreeFilter()
    return
  }
  // 失效收藏节点：磁盘上不可访问（不存在/无权限/网络盘断开），预览/展开均无意义，
  // 前端不区分原因，统一提示后引导右键移除
  if (data.missing) {
    fileTreeRef.value?.setCurrentKey?.(null)
    ElMessage.info(`该${MISSING_FAVORITE_HINT}`)
    return
  }
  const clickedPath = data.path.replace(/\\/g, '/')
  const prevPath = currentSelectedPath.value.replace(/\\/g, '/')
  const wasSelected = prevPath === clickedPath // 点击前是否已选中

  currentSelectedPath.value = data.path
  // 记录最后点击节点供筛选作用域判定（目录筛选作用于该目录，文件/其他回退根层）
  lastClickedNodeData.value = data
  emit('select', data)

  if (data.isLeaf || data.type === 'file') return

  const isAncestor = prevPath.length > clickedPath.length
    && prevPath.startsWith(clickedPath + '/')

  if (isAncestor) return

  // 已展开未选中 → 仅选中，不收起
  // 已展开已选中 → 收起
  // 未展开 → 展开
  if (node.expanded) {
    if (wasSelected) {
      node.collapse()
    }
  } else {
    node.expand()
  }
}

// ---- 聚合视图：定位目标路径所属的收藏根（最长前缀，兼容 \ / 分隔符与大小写差异）----
// 目标可能位于任一收藏根子树下，也可能即收藏根本身（返回该根自身）。
// 无命中返回空串（目标不在任何收藏范围内）。
const findAggregatedRootFor = (nodePath) => {
  const target = normalizeComparablePath(nodePath)
  let best = ''
  // 只在「去重后的根集合」内匹配：被剔除的嵌套收藏在树中不是一级节点
  for (const f of aggregatedFavoriteRoots()) {
    const root = normalizeComparablePath(f.path)
    // 仅子树前缀命中（含根自身），排除仅共享前缀串的兄弟目录（/a/b vs /a/bc）
    if (root && (target === root || target.startsWith(root + '\\')) && root.length > best.length) {
      best = f.path
    }
  }
  return best
}

// 收藏根判定：判定依赖与 findAggregatedRootFor 同源（同一匹配面 + 同一路径归一），
// 禁另写前缀比较，否则「被剔除的嵌套收藏」或分隔符写法差异会让判定与定位分叉。
const isAggregatedRoot = (nodePath) => {
  const root = findAggregatedRootFor(nodePath)
  return !!root && normalizeComparablePath(root) === normalizeComparablePath(nodePath)
}

// 相对路径基准：聚合视图以节点所属收藏根为基准（无命中回退节点自身，避免把
// 收藏范围外路径错切到工作目录下）；普通树沿用当前工作目录，语义不变。
// 「复制相对路径」与「文件引用路径」共用此基准，避免两者口径分裂。
const relativePathBase = (nodePath) => {
  if (aggregatedMode.value) return findAggregatedRootFor(nodePath) || nodePath
  return directoryStore.currentDirPath
}

// ---- 沿父路径向上回溯，找到第一个已展开的祖先节点 ----
// 聚合视图按收藏根回溯（收藏根是树一级节点、在 nodesMap 中存在，可本身作为回溯终点）；
// 普通树按工作目录回溯（根不在 nodesMap，回溯到根返回 null 由 store.root 兜底）。
const findExpandedAncestor = (nodePath, store) => {
  let rootPath
  if (aggregatedMode.value) {
    rootPath = findAggregatedRootFor(nodePath)
    if (!rootPath) return null
  } else {
    const dir = directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)
    if (!dir) return null
    rootPath = dir.path
  }

  const sep = rootPath.includes('\\') ? '\\' : '/'
  const normalizedRoot = sep === '\\' ? rootPath.replace(/\//g, '\\') : rootPath.replace(/\\/g, '/')
  const normalizedTarget = sep === '\\' ? nodePath.replace(/\//g, '\\') : nodePath.replace(/\\/g, '/')

  if (!normalizedTarget.startsWith(normalizedRoot)) return null

  let segments = normalizedTarget.split(sep)
  while (segments.length > 1) {
    segments = segments.slice(0, -1)
    const parentPath = segments.join(sep)
    if (parentPath.length < normalizedRoot.length) return null
    const parent = store.nodesMap[parentPath]
    if (parent && parent.expanded === true) return parent
    // 回溯到收藏根：无论展开与否都可作为刷新目标（expand 会展开并加载最新子节点）；
    // 普通树工作目录根不在 nodesMap（树中无该节点），维持返回 null
    if (parentPath === normalizedRoot) {
      return aggregatedMode.value ? (store.nodesMap[parentPath] || null) : null
    }
  }
  return null
}

// ---- 刷新节点（保留子树展开状态）----
// 局部刷新指定节点：触发 el-tree loadData 重建子节点，并在重建后恢复子树展开状态。
// 解决两个问题：
//   1. 工作目录根下操作不刷新 —— 根路径在 nodesMap 中不存在，此处改用 store.root 兜底；
//   2. 拷贝/粘贴后目标收起 —— loadData 会 childNodes=[] 重建子节点，丢失子树展开，
//      此处刷新前记录子树 expandedPaths，重建后逐层恢复。
// 命中目标后 expand() 会展开目标并加载最新子节点（拷贝到未展开目标时随之展开，符合"看到结果"预期）。
const refreshNode = async (nodePath, options = {}) => {
  if (!fileTreeRef.value || !nodePath) return

  // 筛选态下覆盖层挡住树视图，文件操作后刷新先退出筛选，
  // 防覆盖层命中列表与刷新后树数据脱节（过期结果误导）。
  // 覆盖层右键触发的重命名/删除传 keepFilter 保筛选语境（命中列表由
  // 调用方重跑 runFilter 刷新，见 refreshAfterFilterOp）；树语境无参
  // 调用维持退筛选现行为
  if (!options.keepFilter) {
    exitFilterMode()
  }

  // 聚合视图刷新收藏根本身：根行徽标（isGitRepo/hasRemote）来自后端根层加载时的一次
  // 探测，refreshNode 只重载子层会保留旧徽标快照（在收藏根上 git init / git remote add
  // 后刷新看不到变化）；改走整层重建以重新查询根节点徽标。收藏根的子目录刷新不受影响。
  // 必须先清后端 git 徽标缓存，否则整层重建命中的仍是旧 isGitRepo/hasRemote 结论
  // （GetRootNodesForPaths 复用 isGitRepoDir/hasRemote 缓存，不引入 TTL）。
  if (aggregatedMode.value && isAggregatedRoot(nodePath)) {
    try {
      await InvalidateFileTreeCache(nodePath)
    } catch (error) {
      console.error('Error clearing file tree cache:', error)
    }
    await refreshAggregatedRoot()
    return
  }

  const store = fileTreeRef.value.store
  const dir = directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)

  // 规范化路径分隔符（对齐工作目录根的分隔符，避免 \ / 混用导致 nodesMap 查询失败）；
  // 聚合视图无工作目录根语义，分隔符取目标所属收藏根（无命中则取目标自身）
  const sepRef = aggregatedMode.value ? (findAggregatedRootFor(nodePath) || nodePath) : (dir && dir.path)
  const sep = sepRef && sepRef.includes('\\') ? '\\' : '/'
  const normalize = (p) => sep === '\\' ? (p || '').replace(/\//g, '\\') : (p || '').replace(/\\/g, '/')
  const normalizedPath = normalize(nodePath)
  // 工作目录根 store.root 兜底仅普通树有意义（聚合树根为虚拟节点，收藏根本身在 nodesMap 可直接命中）
  const normalizedRoot = !aggregatedMode.value && dir ? normalize(dir.path) : ''

  // 定位目标节点：nodesMap 命中 -> 工作目录根(store.root) -> 已展开祖先兜底
  let target = store.nodesMap[normalizedPath]
  if (!target && normalizedRoot && normalizedPath === normalizedRoot) {
    target = store.root
  }
  if (!target) {
    target = findExpandedAncestor(nodePath, store)
  }
  if (!target) return

  // 目标是文件时刷新其所在目录：文件作为叶子节点无子节点可重载，对其 expand() 无效，
  // 故上溯到父节点（文件位于工作目录根下时父节点即 store.root）。
  // 对未命中而走 findExpandedAncestor 的路径，target 已是目录，不会进入此分支。
  if (target.data && target.data.type === 'file' && target.parent) {
    target = target.parent
  }

  // 清除后端该路径缓存：避免缓存未失效时 el-tree 重载拿到陈旧数据。
  // InvalidateFileTreeCache 纯清缓存（不返回数据），后续 target.expand 触发 loadTreeNode -> GetFileTree
  // 命中 miss 后实扫回写最新数据。UI 更新走 el-tree loadData 机制以保留子树展开状态恢复逻辑。
  // target 为 store.root 时 data.path 可能为空，回退到 normalizedPath（此时即工作目录根路径）。
  const refreshPath = (target.data && target.data.path) ? target.data.path : normalizedPath
  if (refreshPath) {
    try {
      await InvalidateFileTreeCache(refreshPath)
    } catch (error) {
      console.error('Error clearing file tree cache:', error)
    }
  }

  // 刷新前记录子树展开状态（loadData 重建 childNodes 会丢失子树展开）
  const expandedSubPaths = getExpandedPathsOf(target)

  // 触发重新加载
  target.loaded = false
  target.loading = false
  target.expand()

  // 等待子节点重建完成（el-tree loadData 异步）
  await waitForNodeLoaded(target, 3000)
  await nextTick()

  // 恢复子树展开状态
  await restoreExpandedPaths(expandedSubPaths)
}

// ---- 筛选语境文件操作后的刷新（覆盖层右键重命名/删除，保语境连续）----
// 三步链路，顺序敏感：
//   1. 显式清父目录后端缓存：不依赖 refreshNode 内 target 查找——截断层外文件
//      树中无节点，refreshNode 可能 target miss 静默早退不清缓存，重跑筛选会
//      命中陈旧数据；显式失效同时兜底 mtime 同 tick 边界（filetree_cache.go
//      mtime 差量判定在删除与紧随查询同 tick 时误判未变）
//   2. refreshNode(keepFilter)：树保持挂载照常刷新（退筛选后即见新数据），
//      不 await（树刷新与筛选重拉并行，2000 名外场景其 target miss 早退无副作用）
//   3. 重跑 runFilter：保筛选语境连续，命中列表反映操作后最新状态
//      （删除项消失；重命名后仅新名仍含关键词时保留）
const refreshAfterFilterOp = async (parentPath) => {
  if (!parentPath) return
  try {
    await InvalidateFileTreeCache(parentPath)
  } catch (error) {
    console.error('Error clearing file tree cache:', error)
  }
  refreshNode(parentPath, { keepFilter: true })
  await runFilter()
}

// ---- 文件操作后的统一刷新出口（defineExpose 供父组件调用）----
// 按 filterModeActive 分流，调用方（组件内 handleCreate/handleRename/handleDeleteAt、
// Home.vue 的 paste/copyTo 完成回调）零语境判断：
//   筛选激活（覆盖层右键等入口）→ refreshAfterFilterOp 保语境（命中列表重拉，不退筛选）
//   树语境                    → refreshNode 现状（含 exitFilterMode，树语境无感）
// 注意与右键菜单「刷新」项（onMenuCommand 'refresh'）语义不同：后者在筛选语境下
// 刻意退筛选回树（用户要看树的真实状态）。
const refreshAfterFileOp = (parentPath) => {
  if (filterModeActive.value) {
    return refreshAfterFilterOp(parentPath)
  }
  // 聚合视图下父目录不属于任何收藏根 = 操作对象是收藏根本身（重命名/删除）：
  // 收藏根没有可定位的树内祖先，refreshNode 会静默早退，改为整层重建
  // （收藏记录仍指向旧路径，重建后按失效占位兜底，提示用户移除）
  if (aggregatedMode.value && !findAggregatedRootFor(parentPath)) {
    return refreshAggregatedRoot()
  }
  return refreshNode(parentPath)
}

// ---- 全部刷新 ----
// 前置清除全部后端树缓存，再触发 el-tree 整体重建（treeKey 变更 -> 逐节点重拉 GetFileTree）。
// 不清缓存则二次重建命中陈旧缓存，拿不到最新数据。
const refreshAll = async () => {
  try {
    await ClearAllFileTreeCache()
  } catch (error) {
    console.error('Error clearing file tree cache:', error)
  }
  refreshCounter.value++
  ElMessage.success('文件树已刷新')
}

// ---- 双模式切换编排（目录树 ⇄ 聚合视图）----
// 三步时序，顺序敏感：
//   1. 翻转前保存当前模式树状态（显式传入旧模式，避免 await 期间模式已翻转导致 key 串写）
//   2. 翻转 aggregatedMode -> treeKey 变化 -> el-tree 整体重建（懒加载重拉对应模式根节点）
//   3. 等新树根节点就绪后恢复目标模式树状态（key 同样显式固化）
// restoreState=false 跳过第 3 步：「设为工作目录」链路退出聚合模式后马上要切换工作目录，
// 恢复旧树状态属浪费（onDirectorySelect 会保存/恢复目标目录状态）。
// 代次守卫：在途切换期间再点切换（树尚未重建完成）时跳过保存并让上一次切换流程早退，
// 防止把未完成的树状态写入目标模式 key。
let modeSwitchSeq = 0
let modeSwitchInFlight = false

const switchTreeMode = async (targetMode, options = {}) => {
  if (aggregatedMode.value === targetMode) return
  const seq = ++modeSwitchSeq
  if (!modeSwitchInFlight) {
    saveCurrentState(directoryStore.currentDirPath, aggregatedMode.value)
  }
  modeSwitchInFlight = true
  aggregatedMode.value = targetMode
  aggregatedRootCount.value = 0
  aggregatedRootError.value = ''
  // treeKey 变化的 watch 也会退筛选/清作用域，此处显式执行不依赖 watch 时序
  exitFilterMode()
  lastClickedNodeData.value = null
  try {
    await nextTick()
    if (seq !== modeSwitchSeq) return
    await waitTreeReady()
    if (seq !== modeSwitchSeq) return
  } finally {
    // 异常同样复位在途标志：否则后续切换永久跳过保存
    if (seq === modeSwitchSeq) modeSwitchInFlight = false
  }
  if (options.restoreState !== false) {
    await restoreTreeState(directoryStore.currentDirPath, targetMode)
  }
}

const toggleAggregatedMode = () => switchTreeMode(!aggregatedMode.value)

// 退出聚合视图（供 Home.vue 选择工作目录统一入口前置调用，defineExpose 暴露）
const exitAggregatedMode = (options) => switchTreeMode(false, options)

// ---- 聚合视图内收藏增删后重建根层 ----
// 聚合根层 = 收藏列表快照，收藏变化后旧节点不会自行消失/出现（refreshNode 只刷新
// 节点子层），须整体重建根层；保存/恢复展开态，避免已展开目录因重建收起。
const refreshAggregatedRoot = async () => {
  if (!aggregatedMode.value) return
  // 模式切换在途时树状态不可信：放弃本次重建，最终状态由切换流程负责
  if (modeSwitchInFlight) return
  const seq = ++modeSwitchSeq
  saveCurrentState(directoryStore.currentDirPath, true)
  // 重建期间先清根层态，避免「空树 + 上一次空态文案/旧计数」同帧并存
  aggregatedRootCount.value = 0
  aggregatedRootError.value = ''
  refreshCounter.value++
  await nextTick()
  await waitTreeReady()
  // 期间用户切回目录树或触发新一轮重建：放弃恢复，防聚合展开态落到普通树
  if (seq !== modeSwitchSeq || !aggregatedMode.value) return
  await restoreTreeState(directoryStore.currentDirPath, true)
}

// ---- 聚合视图空白区「刷新」：清全部树/徽标缓存 + 整层重建 ----
// 与节点右键刷新（refreshNode）刻意区分：节点刷新只失效目标路径或其子层（收藏根分支
// 走 InvalidateFileTreeCache 定向失效），而空白区刷新语义是「整树刷新」，对齐目录树的
// refreshAll：先清全部树/徽标缓存再重建根层。否则 GetRootNodesForPaths 复用的
// isGitRepoDir/hasRemote 缓存无 TTL，根行徽标仍命中原有旧结论（收藏根上
// git init / git remote add 后点空白区刷新看不到徽标变化）。
// 清缓存失败只记录错误、不阻断重建（与 refreshAll 同口径）。
const refreshAggregatedRootWithCache = async () => {
  try {
    await ClearAllFileTreeCache()
  } catch (error) {
    console.error('Error clearing file tree cache:', error)
  }
  await refreshAggregatedRoot()
}

// ---- 收藏外部变更 → 重建聚合根层 ----
// 覆盖不经过本组件 handleAdd/handleRemove 的入口：命令面板取消收藏、配置导入后
// loadFavorites、收藏别名修改（store.updateAlias 成功后重载列表）。本组件显式调用
// refreshAggregatedRoot 与 watch 可能双触发，重建幂等（整层重拉 + 展开态恢复）。
// 100ms 去抖合并批量变化（配置导入一次改多条）；代次守卫丢弃「调度后已切换模式 /
// 已有新一轮重建」的过期调度，与既有 modeSwitchSeq 重建链一致。
let favoritesWatchTimer = null

watch(
  () => favoritesStore.favorites,
  () => {
    if (!aggregatedMode.value) return
    const seq = modeSwitchSeq
    clearTimeout(favoritesWatchTimer)
    favoritesWatchTimer = setTimeout(() => {
      favoritesWatchTimer = null
      if (seq !== modeSwitchSeq || !aggregatedMode.value) return
      refreshAggregatedRoot()
    }, 100)
  },
  { deep: true }
)

// ---- 全部展开 ----
const expanding = ref(false)

const expandAll = async () => {
  if (!fileTreeRef.value) return

  expanding.value = true
  try {
    const expandNode = (node) => {
      return new Promise(resolve => {
        if (node.isLeaf) {
          resolve()
          return
        }
        node.expand(() => {
          Promise.all(node.childNodes.map(child => expandNode(child))).then(resolve)
        })
      })
    }

    const root = fileTreeRef.value.store.root
    await Promise.all(root.childNodes.map(child => expandNode(child)))
    ElMessage.success('已全部展开')
  } catch (error) {
    ElMessage.error('展开失败: ' + (error.message || String(error)))
  } finally {
    expanding.value = false
  }
}

// ---- 全部收起 ----
const collapseAll = () => {
  if (fileTreeRef.value) {
    const allNodes = fileTreeRef.value.store.nodesMap
    Object.keys(allNodes).forEach(key => {
      const node = allNodes[key]
      if (node.expanded) {
        node.expanded = false
      }
    })
    ElMessage.success('已全部收起')
  }
}

// ---- 右键菜单 ----
const onNodeContextMenu = (event, data) => {
  event.preventDefault()
  event.stopPropagation() // 恢复 stopPropagation()，防止事件冒泡

  // 截断哨兵节点无文件语义（新建/重命名等命令不适用），不弹菜单
  if (isTruncationHint(data)) return

  // 通知父组件关闭另一个组件的菜单
  emit('contextmenu')

  // 先设置菜单位置
  let x = event.clientX
  let y = event.clientY

  contextMenu.x = x
  contextMenu.y = y
  contextMenu.data = data
  contextMenu.isBlankArea = false
  contextMenu.targetPinned = false
  contextMenu.visible = true

  // git 仓库才查 pin 状态（用于右键「加入/取消关注状态看板」文案切换）
  // 守卫用 path 而非对象引用：空白区 data 每次新建对象 === 失效，节点/空白区统一用 path 更一致
  if (data.isGitRepo) {
    const targetPath = data.path
    IsDashboardPinned(targetPath).then(pinned => {
      // 菜单可能已关闭或切换目标，仅当目标仍是当前 data 时生效
      if (contextMenu.data?.path === targetPath) {
        contextMenu.targetPinned = pinned
      }
    }).catch(() => { /* 查询失败默认未 pin，不阻塞菜单 */ })
  }

  // 等菜单渲染完成后测量实际高度并调整位置
  nextTick(() => {
    const menuElement = document.querySelector('.context-menu')
    if (menuElement) {
      const rect = menuElement.getBoundingClientRect()
      const menuWidth = rect.width
      const menuHeight = rect.height

      // 重新检查并调整边界
      let adjustedX = x
      let adjustedY = y

      // 检查右侧边界
      if (adjustedX + menuWidth > window.innerWidth) {
        adjustedX = window.innerWidth - menuWidth - 5
      }

      // 检查底部边界
      if (adjustedY + menuHeight > window.innerHeight) {
        adjustedY = window.innerHeight - menuHeight - 5
      }

      // 检查左侧边界
      if (adjustedX < 5) {
        adjustedX = 5
      }

      // 检查顶部边界
      if (adjustedY < 5) {
        adjustedY = 5
      }

      // 如果位置有变化，更新菜单位置
      if (adjustedX !== x || adjustedY !== y) {
        contextMenu.x = adjustedX
        contextMenu.y = adjustedY
      }
    }
  })
}

const closeContextMenu = () => {
  contextMenu.visible = false
  contextMenu.isBlankArea = false
}

const onGlobalClick = () => {
  closeContextMenu()
}

// ---- 空白区域右键菜单 ----
const onBlankAreaContextMenu = (event) => {
  event.stopPropagation()

  const dir = directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)
  // 聚合视图无工作目录语义：虚拟根下不依赖当前工作目录，无工作目录时仍可刷新
  if (!aggregatedMode.value && !dir) return

  emit('contextmenu')

  let x = event.clientX
  let y = event.clientY

  contextMenu.x = x
  contextMenu.y = y
  if (aggregatedMode.value) {
    // 聚合虚拟根不落盘，不伪装成某个工作目录节点：空白区本身没有落点。
    // 粘贴目标取最后点击的目录节点（缺失收藏/截断哨兵不可作为落点），
    // 无有效目标时留空 path，模板据此隐藏粘贴项、仅保留刷新。
    const last = lastClickedNodeData.value
    const pasteTarget = last && last.type === 'directory' && !last.missing && !isTruncationHint(last)
      ? last
      : null
    contextMenu.data = pasteTarget
      ? { path: pasteTarget.path, name: pasteTarget.name, type: 'directory' }
      : { path: '', name: '', type: 'directory' }
  } else {
    contextMenu.data = { path: dir.path, name: dir.name, type: 'directory' }
  }
  contextMenu.isBlankArea = true
  contextMenu.targetPinned = false
  contextMenu.visible = true

  // 当前工作目录为 git 仓库才查 pin 状态（用于右键「加入/取消关注状态看板」文案切换）
  // 守卫用 path 而非对象引用：空白区 data 每次新建对象 === 失效
  // 聚合视图菜单不含仓库统计/看板项，跳过该查询
  if (!aggregatedMode.value && blankAreaIsGitRepo.value) {
    const targetPath = dir.path
    IsDashboardPinned(targetPath).then(pinned => {
      // 菜单可能已关闭或切换目标，仅当目标仍是当前 data 时生效
      if (contextMenu.data?.path === targetPath) {
        contextMenu.targetPinned = pinned
      }
    }).catch(() => { /* 查询失败默认未 pin，不阻塞菜单 */ })
  }

  nextTick(() => {
    const menuElement = document.querySelector('.context-menu')
    if (menuElement) {
      const rect = menuElement.getBoundingClientRect()
      let adjustedX = x
      let adjustedY = y
      if (adjustedX + rect.width > window.innerWidth) adjustedX = window.innerWidth - rect.width - 5
      if (adjustedY + rect.height > window.innerHeight) adjustedY = window.innerHeight - rect.height - 5
      if (adjustedX < 5) adjustedX = 5
      if (adjustedY < 5) adjustedY = 5
      if (adjustedX !== x || adjustedY !== y) {
        contextMenu.x = adjustedX
        contextMenu.y = adjustedY
      }
    }
  })
}

const onGlobalContextMenu = () => {
  closeContextMenu()
}

// ---- 菜单命令分发 ----
const onMenuCommand = (command) => {
  const data = contextMenu.data
  // closeContextMenu 会复位 isBlankArea，先固化本次菜单语境供 refresh 分支分流
  const wasBlankArea = contextMenu.isBlankArea
  closeContextMenu()
  if (!data) return

  switch (command) {
    case 'createFile':
      showCreateAt(data, 'file')
      break
    case 'createDir':
      showCreateAt(data, 'directory')
      break
    case 'openRepoFilter':
      emit('open-repo-filter')
      break
    case 'rename':
      showRenameAt(data)
      break
    case 'delete':
      handleDeleteAt(data)
      break
    case 'cut':
      emit('cut', data)
      break
    case 'copy':
      emit('copy', data)
      break
    case 'paste':
      emit('paste', data)
      break
    case 'copyTo':
      showCopyToDialog(data)
      break
    case 'copyPath':
      copyToClipboard(data.path.replaceAll('\\', '/'), '路径')
      break
    case 'copyRelativePath':
      copyToClipboard(toRelativePath(data.path, relativePathBase(data.path)), '相对路径')
      break
    case 'copyRefPath':
      copyToClipboard(toFileRefPath(data.path, relativePathBase(data.path)), '文件引用路径')
      break
    case 'copyName':
      copyToClipboard(data.name, data.type === 'directory' ? '文件夹名' : '文件名')
      break
    case 'openExplorer':
      handleOpenExplorer(data.path)
      break
    case 'openInVSCode':
      handleOpenInVSCode(data.path)
      break
    case 'openInWarp':
      handleOpenInWarp(data.path)
      break
    case 'openInObsidian':
      handleOpenObsidian(data.path)
      break
    case 'openWithDefaultApp':
      handleOpenWithDefaultApp(data.path)
      break
    case 'refresh':
      // 聚合视图空白区「刷新」= 清全部树/徽标缓存 + 重建整个根层（虚拟根无节点可定位，
      // refreshNode 会静默早退）；节点右键刷新仍走 refreshNode（收藏根本身由 refreshNode
      // 内部先定向失效缓存再整层重建，见 ⑬）
      if (aggregatedMode.value && wasBlankArea) {
        refreshAggregatedRootWithCache()
        break
      }
      // 筛选语境下「刷新」= 退筛选回树并重载该节点（refreshNode 内 exitFilterMode）：
      // 用户要看树的真实状态，与重命名/删除的保语境刷新（refreshAfterFilterOp）语义不同
      refreshNode(data.path)
      break
    case 'pullRepos':
      handleBatchPull(data)
      break
    case 'addFavorite':
      handleAddFavorite(data)
      break
    case 'removeFavorite':
      handleRemoveFavorite(data)
      break
    case 'addAsWorkDir':
      handleAddAsWorkDir(data)
      break
    case 'setAsWorkDir':
      handleSetAsWorkDir(data)
      break
    case 'addToAiChat':
      handleAddToAiChat(data)
      break
    case 'contentSearch': {
      // 内容搜索沿用「当前工作目录 + 相对路径」口径，聚合视图刻意不改用收藏根：
      // 后端 service/content_search.go 以 filepath.Join(workDir, subDir) 解析入参，
      // 若传收藏根相对路径会被重新拼到工作目录下；跨工作目录的收藏根在当前绑定
      // （无 ctx、无根参数）下无法限定作用域，属已知限制。
      const currentWorkDir = directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)
      if (currentWorkDir && data.path.startsWith(currentWorkDir.path)) {
        const relPath = data.path.slice(currentWorkDir.path.length).replace(/^[\\\/]/, '')
        emit('open-content-search', relPath)
      } else {
        emit('open-content-search', '')
      }
      break
    }
    case 'jumpStats':
      handleJumpStats(data)
      break
    case 'toggleDashboardPin':
      handleToggleDashboardPin(data)
      break
    case 'splitPptx':
      handleSplitPptx(data)
      break
    case 'fileHistory':
      openFileHistory(data)
      break
  }
}

// ---- 文件历史弹窗 ----
// fileAbsPath 传节点绝对路径：后端 GetFileCommitHistory 以其定位仓库根并换算
// 仓库相对路径，repoPath 同值透传（内嵌 diff 弹窗经 FindGitRoot 消化，容忍文件路径）
const fileHistoryVisible = ref(false)
const fileHistoryPath = ref('')

const openFileHistory = (data) => {
  fileHistoryPath.value = data.path
  fileHistoryVisible.value = true
}

// 文件分支「查看历史」显示条件：仅文件节点（目录入口不做，--follow 仅支持单一路径）；
// git 归属为近似判定：普通模式按当前工作目录 isGitRepo（复用 blankAreaIsGitRepo）；
// 聚合模式恒显示（见菜单项处注释）
const fileHistoryAvailable = computed(() => {
  if (contextMenu.data?.type !== 'file') return false
  if (aggregatedMode.value) return true
  return blankAreaIsGitRepo.value
})

// 跳转仓库统计：设选中节点为右键目录并切统计面板，
// StatsView 读 selectedNode.path 加载统计（先设节点后切面板触发 watch 链）。
const handleJumpStats = (data) => {
  workspaceStore.selectedNode = { path: data.path, name: data.name, type: data.type, isGitRepo: true }
  uiStore.activePanel = 'stats'
}

// 加入/取消关注状态看板：基于右键时查询的 targetPinned 状态切换。
// pin 列表后端持久化（data/dashboard_pinned.json，路径规范化去重），看板开页或刷新时拉取最新状态。
const handleToggleDashboardPin = async (data) => {
  try {
    if (contextMenu.targetPinned) {
      await RemoveDashboardPin(data.path)
      ElMessage.success('已取消关注状态看板')
    } else {
      await AddDashboardPin(data.path)
      ElMessage.success('已加入状态看板')
    }
  } catch (e) {
    handleError('状态看板操作失败：', e)
  }
}

// ---- 新建文件/文件夹 ----
const showCreateAt = (data, type) => {
  createParentData.value = data
  createType.value = type
  createName.value = ''
  createDialogVisible.value = true
  setTimeout(() => {
    const input = createInputRef.value?.input
    if (input) {
      input.focus()
    }
  }, 100)
}

const handleCreate = async () => {
  if (!createName.value.trim()) {
    ElMessage.warning(createType.value === 'directory' ? '请输入文件夹名称' : '请输入文件名称')
    return
  }
  if (!createParentData.value) return

  createLoading.value = true
  try {
    let result
    if (createType.value === 'directory') {
      result = await CreateDirectory(createParentData.value.path, createName.value.trim())
    } else {
      result = await CreateFile(createParentData.value.path, createName.value.trim(), '')
    }
    if (result) {
      ElMessage.success(createType.value === 'directory' ? '文件夹创建成功' : '文件创建成功')
      createDialogVisible.value = false
      // 统一刷新出口：筛选语境保语境刷新命中列表（新建产物命中关键词时出现）；树语境现状
      refreshAfterFileOp(createParentData.value.path)
    } else {
      ElMessage.error('创建失败')
    }
  } catch (error) {
    ElMessage.error('创建失败: ' + (error.message || String(error)))
  } finally {
    createLoading.value = false
  }
}

// ---- 重命名 ----
const showRenameAt = (data) => {
  renameNode.value = data
  renameName.value = data.name
  renameDialogVisible.value = true
  setTimeout(() => {
    const input = renameInputRef.value?.input
    if (input) {
      input.focus()
      input.select()
    }
  }, 100)
}

const handleRename = async () => {
  if (!renameName.value.trim()) {
    ElMessage.warning('请输入名称')
    return
  }
  if (!renameNode.value) return

  renameLoading.value = true
  try {
    const result = await RenameFile(renameNode.value.path, renameName.value.trim())
    if (result) {
      ElMessage.success('重命名成功')
      renameDialogVisible.value = false
      const targetPath = renameNode.value.path
      let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
      if (!parentPath) {
        parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
      }
      // 统一刷新出口：筛选语境（覆盖层右键入口）保语境刷新命中列表；树语境维持退筛选现状
      await refreshAfterFileOp(parentPath)
    } else {
      ElMessage.error('重命名失败')
    }
  } catch (error) {
    ElMessage.error('重命名失败: ' + (error.message || String(error)))
  } finally {
    renameLoading.value = false
  }
}

// ---- 删除 ----
const handleDeleteAt = async (data) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除 "${data.name}" 吗？此操作不可撤销。`,
      '警告',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
  } catch {
    return
  }

  try {
    const targetPath = data.path
    const result = await DeleteFile(targetPath)
    if (result) {
      ElMessage.success('删除成功')
      emit('delete', data)
      let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
      if (!parentPath) {
        parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
      }
      // 统一刷新出口：筛选语境（覆盖层右键入口）保语境刷新命中列表；树语境维持退筛选现状
      await refreshAfterFileOp(parentPath)
    } else {
      ElMessage.error('删除失败')
    }
  } catch (error) {
    ElMessage.error('删除失败: ' + (error.message || String(error)))
  }
}

// ---- 拷贝到对话框 ----
const showCopyToDialog = (data) => {
  copyToSourcePath.value = data.path.replaceAll('\\', '/')
  copyToTargetPath.value = ''
  copyToWholeDir.value = data.type === 'directory'
  copyToFileMode.value = data.type === 'file'
  // 文件/目录均初始化默认名（源路径最后一段），用户可修改
  copyToTargetName.value = defaultCopyToName()
  copyToLoading.value = false
  copyToDialogVisible.value = true
  setTimeout(() => {
    const input = copyToTargetInputRef.value?.input
    if (input) {
      input.focus()
    }
  }, 100)
}

// 文件名非法字符校验（与 Windows 资源管理器一致 + 路径分隔符）
const ILLEGAL_NAME_RE = /[\\/:*?"<>|]/

const handleCopyTo = () => {
  if (!copyToSourcePath.value.trim()) {
    ElMessage.warning('请输入原地址')
    return
  }
  if (!copyToTargetPath.value.trim()) {
    ElMessage.warning('请输入目标地址')
    return
  }
  const targetName = copyToTargetName.value.trim()
  if (targetName) {
    if (ILLEGAL_NAME_RE.test(targetName) || targetName === '.' || targetName === '..') {
      ElMessage.warning('文件名含非法字符（\\ / : * ? " < > |）')
      return
    }
  }

  emit('copyTo', {
    sourcePath: copyToSourcePath.value,
    targetPath: copyToTargetPath.value,
    targetName,
    copyWholeDir: copyToWholeDir.value
  })
}

// ---- 复制到剪贴板 ----
const copyToClipboard = async (text, label) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch {
    ElMessage.error('复制失败')
  }
}

// ---- 外部工具 ----
const handleOpenExplorer = async (path) => {
  try {
    const result = await OpenInExplorer(path)
    if (!result) {
      ElMessage.error('打开资源管理器失败')
    }
  } catch (error) {
    ElMessage.error('打开资源管理器失败: ' + (error.message || String(error)))
  }
}

const handleOpenInVSCode = async (path) => {
  try {
    const result = await OpenInVSCode(path)
    if (!result) {
      ElMessage.error('打开 VSCode 失败，请确认已安装 VSCode 并将 code 命令加入 PATH')
    }
  } catch (error) {
    ElMessage.error('打开 VSCode 失败: ' + (error.message || String(error)))
  }
}

const handleOpenInWarp = async (path) => {
  try {
    const result = await OpenInWarp(path)
    if (!result) {
      ElMessage.error('打开 Warp 失败，请确认已安装 Warp 终端')
    }
  } catch (error) {
    ElMessage.error('打开 Warp 失败: ' + (error.message || String(error)))
  }
}

const handleOpenObsidian = async (path) => {
  try {
    const status = await OpenInObsidian(path)
    if (status === 'not-installed') {
      ElMessage.error('未检测到 Obsidian，请在【设置 → 通用 → 外部应用】中配置 Obsidian 程序路径，或安装 Obsidian 并至少运行一次')
    } else if (status === 'not-registered') {
      try {
        await ElMessageBox.confirm('该目录未注册为 Obsidian vault。', '提示', {
          confirmButtonText: '自动注册并打开',
          cancelButtonText: '打开仓库管理器',
          distinguishCancelAndClose: true,
          type: 'info'
        })
        // 用户点「自动注册并打开」-> 二次确认（预告信任提示 + 备份 + 运行中需关闭）
        try {
          await ElMessageBox.confirm(
            '即将把该目录注册为 Obsidian vault 并打开。<br>• 首次打开会弹出信任提示，请选择「Trust author and enable plugins」<br>• Obsidian 配置已自动备份<br>• 若 Obsidian 正在运行，需先关闭后重试',
            '确认自动注册',
            { confirmButtonText: '继续', cancelButtonText: '取消', type: 'warning', dangerouslyUseHTMLString: true }
          )
          const regStatus = await AutoRegisterAndOpen(path)
          if (regStatus === '') {
            ElMessage.success('已注册为 Obsidian vault 并打开，首次会弹信任提示请确认')
          } else if (regStatus === 'running') {
            ElMessage.warning('Obsidian 正在运行，请关闭所有 Obsidian 窗口后重试')
          } else if (regStatus === 'not-installed') {
            ElMessage.error('未检测到 Obsidian，请在【设置 -> 通用 -> 外部应用】中配置')
          } else {
            ElMessage.error('自动注册失败，请重试或手动添加')
          }
        } catch {
          // 二次确认取消，不处理
        }
      } catch (action) {
        if (action === 'cancel') {
          // 「打开仓库管理器」：复制路径 + 提示 + 跳转 choose-vault
          const copied = await CopyObsidianVaultPath(path)
          if (copied) ElMessage.success('已复制目录路径到剪贴板')
          else ElMessage.warning('复制路径失败')
          const opened = await OpenObsidianVaultManager()
          if (opened) ElMessage.info('已打开 Obsidian 仓库管理器，请将该目录添加为 vault')
          else ElMessage.error('打开 Obsidian 仓库管理器失败')
        }
        // action === 'close' -> 关闭（X），不处理
      }
    }
  } catch (error) {
    ElMessage.error('打开 Obsidian 失败: ' + (error.message || String(error)))
  }
}

const handleOpenWithDefaultApp = async (path) => {
  try {
    const result = await OpenWithDefaultApp(path)
    if (!result) {
      ElMessage.error('打开文件失败')
    }
  } catch (error) {
    ElMessage.error('打开文件失败: ' + (error.message || String(error)))
  }
}

// ---- PPT 按页拆分（文件右键菜单项，仅 .pptx 文件显示，大小写不敏感）----
// 哨兵节点不弹菜单（onNodeContextMenu early return），本菜单项天然不触达哨兵
const isPptxFile = (data) =>
  data?.type === 'file' && typeof data.name === 'string' && data.name.toLowerCase().endsWith('.pptx')

// 确认弹窗预告输出目录与覆盖行为；拆分为长操作，Promise 完成前不持 UI 状态，
// 成功/失败经 message 反馈；进度展示由工具箱 PptSplitPanel 负责（本组件不监听
// pptx-split:* 事件，避免多组件共听面；两入口拆分事件载荷含 srcPath 可区分归属）
const handleSplitPptx = async (data) => {
  const baseName = data.name.replace(/\.pptx$/i, '')
  try {
    await ElMessageBox.confirm(
      `将在 PPT 同级目录生成「${baseName}」文件夹，每页一个独立 PPT（命名如 ${baseName}-01.pptx）。已存在的同名输出文件将被覆盖。是否继续？`,
      '按页拆分 PPT',
      { confirmButtonText: '开始拆分', cancelButtonText: '取消', type: 'info' }
    )
  } catch {
    return
  }
  try {
    const result = await SplitPptxPerPage(data.path)
    ElMessage.success(`拆分完成：${result.pageCount} 页已输出到 ${result.outputDir}`)
  } catch (error) {
    handleError('PPT 拆分失败: ', error)
  }
}

// ---- 批量拉取 ----
const handleBatchPull = (data) => {
  emit('batchPull', data)
}

// ---- 添加到收藏 ----
const handleAddFavorite = async (node) => {
  const err = await favoritesStore.addFavorite(node.path, '', '默认')
  if (err) {
    ElMessage.warning(err)
  } else {
    ElMessage.success('已添加到收藏')
    // 聚合视图内右键新增收藏：收藏列表已变，重建聚合根层让新目录出现
    await refreshAggregatedRoot()
  }
}

// ---- 取消收藏 ----
const handleRemoveFavorite = async (node) => {
  const err = await favoritesStore.removeFavorite(node.path)
  if (err) {
    ElMessage.warning(err)
  } else {
    ElMessage.success('已取消收藏')
    // 聚合视图内（含失效占位节点）移除收藏：重建聚合根层让节点消失
    await refreshAggregatedRoot()
  }
}

// ---- 设为工作目录（聚合视图右键收藏目录）----
// 仅 emit 事件，编排交 Home.vue：添加（若缺失）+ 切换工作目录 + 退出聚合模式 + 定位，
// 复用 onDirectorySelect 的既有保存/恢复链；本组件暴露 exitAggregatedMode/locateNode。
const handleSetAsWorkDir = (node) => {
  emit('set-as-work-dir', { path: node.path, name: node.name })
}

// ---- 添加为工作目录 ----
const handleAddAsWorkDir = (node) => {
  emit('add-work-dir', { path: node.path, name: node.name })
}

// ---- 添加到 AI 对话 ----
// 仅目录节点菜单含此项：把该子目录加入 AI 对话侧栏常用目录。
// 已存在时 info 提示不重复添加（后端同路径幂等兜底）；成功仅提示不强制
// 切换面板（对齐「添加到收藏」交互：不中断当前工作流）。
const handleAddToAiChat = async (node) => {
  if (aiChatStore.containsPath(node.path)) {
    ElMessage.info('该目录已在 AI 对话列表中')
    return
  }
  try {
    const item = await aiChatStore.addChatDirectory(node.path, '')
    ElMessage.success(item ? `已添加到 AI 对话「${item.displayName}」` : '已添加到 AI 对话')
  } catch (error) {
    handleError('添加失败: ', error)
  }
}

// ---- 树状态记忆 ----
function getExpandedPaths() {
  const tree = fileTreeRef.value
  if (!tree) return []
  const store = tree.store
  const paths = []
  function walk(node) {
    if (node.expanded && node.data && node.data.path) {
      paths.push(node.data.path)
    }
    if (node.childNodes) {
      node.childNodes.forEach(walk)
    }
  }
  walk(store.root)
  return paths
}

// 收集指定节点子树的展开路径（用于刷新后恢复子树展开状态）
function getExpandedPathsOf(node) {
  const paths = []
  function walk(n) {
    if (!n) return
    if (n.expanded && n.data && n.data.path) {
      paths.push(n.data.path)
    }
    if (n.childNodes) {
      n.childNodes.forEach(walk)
    }
  }
  walk(node)
  return paths
}

// 按记录的路径恢复展开状态（depth 分组逐层展开 + 等待懒加载）
async function restoreExpandedPaths(paths) {
  if (!paths || paths.length === 0) return
  const tree = fileTreeRef.value
  if (!tree || typeof tree.getNode !== 'function') return

  const depthGroups = new Map()
  for (const path of paths) {
    const depth = path.split(/[\\/]/).length
    if (!depthGroups.has(depth)) depthGroups.set(depth, [])
    depthGroups.get(depth).push(path)
  }

  const sortedDepths = [...depthGroups.keys()].sort((a, b) => a - b)
  for (const depth of sortedDepths) {
    const pending = []
    for (const path of depthGroups.get(depth)) {
      const node = tree.getNode(path)
      if (!node || node.expanded) continue
      node.expand()
      if (!node.loaded) pending.push(waitForNodeLoaded(node, 2000))
    }
    if (pending.length > 0) await Promise.all(pending)
  }
}

// 树状态持久化 key 按模式分流：聚合视图树与工作目录无关，固定写虚拟 key
// （AGGREGATED_TREE_STATE_KEY），普通树沿用目录路径 key，两模式展开状态互不污染。
// mode 可选：模式切换/重建链路显式传入「当前实际树模式」，避免 await 期间模式翻转
// 导致 key 串写；Home.vue 的切目录调用点不传，按当前模式分流。
function currentStateKey(dirPath, mode = aggregatedMode.value) {
  return mode ? AGGREGATED_TREE_STATE_KEY : dirPath
}

function saveCurrentState(dirPath, mode) {
  const treeEl = document.querySelector('.tree-content')
  saveState(currentStateKey(dirPath, mode), {
    expandedPaths: getExpandedPaths(),
    scrollTop: treeEl ? treeEl.scrollTop : 0,
    selectedPath: null
  })
}

async function restoreTreeState(dirPath, mode) {
  const state = restoreState(currentStateKey(dirPath, mode))
  if (state.expandedPaths.length === 0) return

  const tree = fileTreeRef.value
  if (!tree) return

  // 根层可能确定为空（空收藏/无工作目录）：就绪后直接返回，不再空等轮询超时
  await waitTreeReady()
  if (!tree.store.root || tree.store.root.childNodes.length === 0) return

  const depthGroups = new Map()
  for (const path of state.expandedPaths) {
    const depth = path.split(/[\\/]/).length
    if (!depthGroups.has(depth)) depthGroups.set(depth, [])
    depthGroups.get(depth).push(path)
  }

  const sortedDepths = [...depthGroups.keys()].sort((a, b) => a - b)

  for (const depth of sortedDepths) {
    const paths = depthGroups.get(depth)
    const pending = []

    for (const path of paths) {
      const node = tree.getNode(path)
      if (!node || node.expanded) continue
      node.expand()
      if (!node.loaded) pending.push(waitForNodeLoaded(node, 2000))
    }

    if (pending.length > 0) await Promise.all(pending)
  }

  if (state.scrollTop > 0) {
    await nextTick()
    const treeEl = document.querySelector('.tree-content')
    if (treeEl) treeEl.scrollTop = state.scrollTop
  }
}

function waitUntil(condition, timeout = 2000) {
  return new Promise(resolve => {
    if (condition()) { resolve(); return }
    const start = Date.now()
    const check = () => {
      if (condition() || Date.now() - start > timeout) {
        resolve()
      } else {
        setTimeout(check, 16)
      }
    }
    check()
  })
}

function waitForNodeLoaded(node, timeout = 2000) {
  return new Promise(resolve => {
    if (node.loaded) { resolve(); return }
    const start = Date.now()
    const check = () => {
      if (node.loaded || Date.now() - start > timeout) {
        resolve()
      } else {
        setTimeout(check, 16)
      }
    }
    check()
  })
}

async function locateNode(targetPath) {
  await treeReadyPromise
  const tree = fileTreeRef.value
  if (!tree) return

  // 聚合视图：目标按最长前缀收藏根回溯（收藏根是树一级节点）；普通树按工作目录回溯
  let rootPath
  if (aggregatedMode.value) {
    rootPath = findAggregatedRootFor(targetPath)
    if (!rootPath) {
      ElMessage.info('该路径不在任何收藏目录范围内（聚合视图仅展示收藏目录）')
      return
    }
  } else {
    const dir = directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)
    if (!dir) return
    rootPath = dir.path
  }

  const sep = rootPath.includes('\\') ? '\\' : '/'
  const normalizedTarget = sep === '\\' ? targetPath.replace(/\//g, '\\') : targetPath.replace(/\\/g, '/')
  const normalizedRoot = sep === '\\' ? rootPath.replace(/\//g, '\\') : rootPath.replace(/\\/g, '/')

  if (!normalizedTarget.startsWith(normalizedRoot)) return

  const relative = normalizedTarget.slice(normalizedRoot.length).replace(/^[\\/]/, '')
  if (!relative) {
    // 目标即根路径：普通树的工作目录根不在树中（维持静默返回）；聚合视图的收藏根
    // 是树一级节点，可直接选中并滚动定位
    if (aggregatedMode.value) {
      const rootNode = tree.getNode(normalizedTarget) || tree.getNode(targetPath)
      if (rootNode) {
        // 失效收藏不可选中：否则快捷键（F2/Delete）会作用于不存在的路径
        if (rootNode.data?.missing) {
          ElMessage.info(`该${MISSING_FAVORITE_HINT}`)
          return
        }
        tree.setCurrentKey(rootNode.data.path)
        await new Promise(r => setTimeout(r, 50))
        const treeContainer = document.querySelector('.tree-content')
        const nodeEl = treeContainer?.querySelector('.el-tree-node.is-current')
        if (nodeEl && treeContainer) {
          const containerRect = treeContainer.getBoundingClientRect()
          const nodeRect = nodeEl.getBoundingClientRect()
          const offset = nodeRect.top - containerRect.top - containerRect.height / 2
          treeContainer.scrollBy({ top: offset, behavior: 'smooth' })
        }
      }
    }
    return
  }

  const segments = relative.split(/[\\/]/)
  let currentPath = normalizedRoot

  for (let i = 0; i < segments.length; i++) {
    currentPath += sep + segments[i]
    const node = tree.getNode(currentPath)
    if (!node) break

    if (!node.expanded && !node.isLeaf) {
      node.expand()
      if (!node.loaded) {
        await waitForNodeLoaded(node, 3000)
      }
      await nextTick()
    }
  }

  await nextTick()
  const finalNode = tree.getNode(normalizedTarget) || tree.getNode(targetPath)
  if (finalNode) {
    tree.setCurrentKey(finalNode.data.path)
    await new Promise(r => setTimeout(r, 50))
    const treeContainer = document.querySelector('.tree-content')
    const nodeEl = treeContainer?.querySelector('.el-tree-node.is-current')
    if (nodeEl && treeContainer) {
      const containerRect = treeContainer.getBoundingClientRect()
      const nodeRect = nodeEl.getBoundingClientRect()
      const offset = nodeRect.top - containerRect.top - containerRect.height / 2
      treeContainer.scrollBy({ top: offset, behavior: 'smooth' })
    }
  } else {
    // 大目录截断后目标可能落在未渲染的前 2000 项之外（perf-baseline §15.6），
    // 路径已删除同样 miss；静默失败会让命令面板/收藏跳转「点了没反应」，显式提示
    ElMessage.info('未能定位该文件：路径不存在，或位于大目录未渲染部分（可用 Ctrl+P 搜索文件内容或经终端访问）')
  }
}

// ---- 键盘快捷键入口：作用于当前高亮节点 ----
// 哨兵节点过滤：el-tree 高亮点击（highlight-current）在组件 onNodeClick 之前已
// setCurrentNode，F2/Delete 经 getCurrentNode 可能拿到哨兵（合成 path 无文件语义），
// 须在此拦截防对哨兵弹重命名/删除对话框。getCurrentNode 返回的是 data 对象
// （组件层包装 return currentNode.data，非内部 Node），isTruncationHint 直接收它
// 不可操作节点：截断哨兵与聚合视图失效收藏占位（路径在磁盘上不存在）
const isUnusableNode = (node) => isTruncationHint(node) || !!node?.data?.missing

const triggerRenameCurrent = () => {
  const node = fileTreeRef.value?.getCurrentNode()
  if (node && !isUnusableNode(node)) showRenameAt(node)
}

const triggerDeleteCurrent = () => {
  const node = fileTreeRef.value?.getCurrentNode()
  if (node && !isUnusableNode(node)) handleDeleteAt(node)
}

// ---- 暴露方法 ----
defineExpose({
  refreshNode,
  refreshAfterFileOp,
  expandAll,
  collapseAll,
  showRenameAt,
  showCreateAt,
  handleDeleteAt,
  showCopyToDialog,
  triggerRenameCurrent,
  triggerDeleteCurrent,
  setCopyToLoading: (val) => { copyToLoading.value = val },
  closeCopyToDialog: () => { copyToDialogVisible.value = false },
  closeMenu: () => { contextMenu.visible = false },
  saveCurrentState,
  restoreTreeState,
  locateNode,
  exitAggregatedMode,
  // 供 Home.vue「选择工作目录 = 退出聚合」判定：函数形态与 exitAggregatedMode 同口径
  isAggregatedMode: () => aggregatedMode.value
})

// ---- 生命周期 ----
onMounted(() => {
  document.addEventListener('mousedown', onGlobalClick)
  document.addEventListener('contextmenu', onGlobalContextMenu)
  favoritesStore.loadFavorites()
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onGlobalClick)
  document.removeEventListener('contextmenu', onGlobalContextMenu)
  // 防抖在途时组件卸载后 runFilter 仍会拉取/写 ref/弹错误提示
  clearTimeout(filterDebounceTimer)
  // 收藏变化去抖在途时同样清理：卸载后不得再触发聚合根重建
  clearTimeout(favoritesWatchTimer)
})
</script>

<style scoped>
.file-tree-aside {
  display: flex;
  flex-direction: column;
  height: 100%;
  border-right: 1px solid var(--border-color);
  background-color: var(--bg-primary);
  overflow: hidden;
}

.tree-toolbar {
  flex-shrink: 0;
  padding: var(--spacing-md) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}

/* 工具栏首行：按钮组 + 扩展 slot；筛选输入框独立成行（面板窄，同行动线挤压） */
.tree-toolbar-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-sm);
}

.tree-filter-input {
  width: 100%;
}

.tree-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

/* 目录内按名筛选覆盖层（perf-baseline §15.6 截断配套）：
   激活时整体替换树视图，命中列表与树节点行高/间距对齐 */
.tree-filter-overlay {
  padding: var(--spacing-sm) var(--spacing-xs);
}

.tree-filter-summary {
  font-size: 12px;
  color: var(--text-secondary, #909399);
  padding: var(--spacing-sm) var(--spacing-md);
  letter-spacing: -0.01em;
}

.filter-result-node {
  display: flex;
  align-items: center;
  font-size: 13px;
  padding: var(--spacing-sm) var(--spacing-md);
  margin: var(--spacing-sm) var(--spacing-xs);
  border-radius: var(--radius-sm);
  cursor: pointer;
  user-select: none;
  transition: background-color var(--transition-normal);
}

.filter-result-node:hover {
  background-color: var(--bg-tertiary);
  box-shadow: var(--shadow-sm);
}

.filter-result-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 覆盖层内截断哨兵（命中超限提示）：非可交互项，「点击按名筛选」文案引导的是
   树视图哨兵行为，覆盖层语境下点击为空操作，视觉上去除可点击暗示 */
.tree-filter-overlay .truncation-hint-node {
  cursor: default;
}

.tree-filter-overlay .truncation-hint-node:hover {
  background-color: transparent;
  box-shadow: none;
}

.file-tree {
  background: transparent;
}

.el-tree-node__content {
  transition: all var(--transition-normal);
  border-radius: var(--radius-sm);
  margin: var(--spacing-sm) var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
}
.el-tree-node__content:hover {
  background-color: var(--bg-tertiary) !important;
  box-shadow: var(--shadow-sm);
}
.is-current > .el-tree-node__content {
  background-color: rgba(64, 158, 255, 0.1) !important;
  font-weight: 500;
  box-shadow: var(--shadow-md);
  border-left: 3px solid var(--primary-color);
}
.custom-tree-node {
  flex: 1;
  display: flex;
  align-items: center;
  font-size: 13px;
  cursor: default;
  user-select: none;
}
/* 截断哨兵节点（单层超限提示，见 utils/treeTruncate.js）：弱化展示；
   点击聚焦工具栏筛选框（onNodeClick 哨兵分支），快捷键路径由
   triggerRenameCurrent/triggerDeleteCurrent 双重过滤防误弹对话框 */
.truncation-hint-node {
  opacity: 0.75;
}
.el-tree-node__children {
  transition: all 0.3s ease;
  overflow: hidden;
}
.el-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

/* 文件树节点 git 仓库标记 img（替代原 SuccessFilled，保留 margin-left:5px 对齐） */
.tree-node-git-img {
  width: 14px;
  height: 14px;
  margin-left: 5px;
  vertical-align: middle;
  object-fit: contain;
}

/* 文件树节点文件类型图标（按后缀映射，与 Document/el-icon 视觉对齐） */
.tree-node-file-icon {
  width: 14px;
  height: 14px;
  margin-right: 5px;
  vertical-align: middle;
  object-fit: contain;
}

/* 互换按钮行 */
.swap-row {
  display: flex;
  justify-content: center;
  margin: -8px 0 0;
}

.swap-row .el-button {
  color: var(--text-tertiary, #909399);
  font-size: 12px;
}

.swap-row .el-button:hover {
  color: var(--primary-color, #409eff);
}

.swap-row .el-button:hover .swap-icon {
  transform: rotate(180deg);
}

.swap-icon {
  transition: transform 0.3s ease;
}

</style>
