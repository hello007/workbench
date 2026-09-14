# 取消收藏功能设计

## 1. 问题描述

当前只有"添加到收藏"操作入口，没有"取消收藏"的交互入口。后端 `RemoveFavorite` 接口和 composable 的 `removeFavorite` 方法已实现，仅缺前端入口。

## 2. 设计方案

### 2.1 文件树右键菜单 — 智能切换

- 从 `useFavorites` 解构出 `favorites` 和 `removeFavorite`
- 新增 computed `isFavorited`：检查 `contextMenu.data?.path` 是否在 `favorites` 列表中
- 右键菜单中用 `v-if/v-else` 切换显示"添加到收藏"或"取消收藏"（互斥，不同时出现）
- 取消收藏图标用 `<StarFilled />` 配合文字"取消收藏"
- 取消操作无需确认弹窗（操作可逆，可再次右键添加回来）

菜单项模板：
```html
<li v-if="isFavorited" @click="onMenuCommand('removeFavorite')">
  <el-icon><StarFilled /></el-icon>取消收藏
</li>
<li v-else @click="onMenuCommand('addFavorite')">
  <el-icon><Star /></el-icon>添加到收藏
</li>
```

onMenuCommand 新增 case：
```js
case 'removeFavorite':
  handleRemoveFavorite(data)
  break
```

handler 实现：
```js
const handleRemoveFavorite = async (node) => {
  const err = await removeFavorite(node.path)
  if (err) {
    ElMessage.warning(err)
  } else {
    ElMessage.success('已取消收藏')
  }
}
```

### 2.2 Command Palette 收藏列表 — 行内删除按钮

- 每个收藏项右侧增加一个关闭图标按钮
- 点击图标时调用 `removeFavorite`，刷新列表
- 用 `@click.stop` 阻止事件冒泡（避免触发 selectFavorite 跳转）

模板结构：
```html
<div class="result-item-right">
  <el-icon class="remove-fav-btn" @click.stop="handleRemoveFav(item)">
    <Close />
  </el-icon>
</div>
```

handleRemoveFav 实现：
```js
async function handleRemoveFav(item) {
  const err = await RemoveFavorite(item.path)
  if (!err) {
    favoriteResults.value = favoriteResults.value.filter(f => f.path !== item.path)
  }
}
```

样式：删除按钮默认隐藏，hover 收藏项时显示。

### 2.3 数据联动

`useFavorites` composable 内部 `favorites` 是模块级 ref。`removeFavorite` 成功后调用 `loadFavorites()` 刷新列表。FileTreePanel 中的 `isFavorited` computed 自动响应变化。

CommandPalette 需要额外处理列表移除（因其使用独立的 `favoriteResults` 数组）。

## 3. 影响范围

| 文件 | 改动内容 |
|------|----------|
| `frontend/src/components/FileTreePanel.vue` | 解构 `removeFavorite` + `favorites`，新增 `isFavorited` computed，菜单 v-if 切换，handler |
| `frontend/src/components/CommandPalette.vue` | 收藏项增加删除按钮，导入 Close 图标，新增 handleRemoveFav，hover 样式 |

## 4. 测试要点

- 右键已收藏目录 → 显示"取消收藏"而非"添加到收藏"
- 点击"取消收藏" → 成功提示，再次右键显示"添加到收藏"
- Command Palette 收藏列表 → hover 显示删除按钮，点击删除后项目消失
- 删除按钮点击不触发收藏跳转（事件冒泡阻止）
