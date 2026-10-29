# el-tree 展开态：可达展开面过滤与真实 EP 单测纪律

> 主题：素材管理文件夹树（FolderTree.vue）的展开/收起状态管理；适用任何经
> `default-expanded-keys` 维护展开面的 el-tree 组件。
> 来源：2026-10-29「子文件夹点击/双击不收起」bugfix（任务 10-29-bug）。

## 1. 真实 EP 的 default-expanded-keys 回放语义（根因）

EP 2.14.5（locked）真实 el-tree 对 `default-expanded-keys` prop 有 **watch 回放**：

```js
// tree.vue：prop 变化即回放
watch(() => props.defaultExpandedKeys, (newVal) => {
  store.setDefaultExpandedKeys(newVal ?? [])
})
// tree-store.mjs：逐 key expand，autoExpandParent 默认 true
setDefaultExpandedKeys(keys) {
  keys.forEach((key) => {
    const node = this.getNode(key)
    if (node) node.expand(null, this.autoExpandParent)  // ← ep 默认 true
  })
}
// node.mjs：expandParent=true 时祖先链强制展开
expand(callback, expandParent) {
  const done = () => {
    if (expandParent) {
      let parent = this.parent
      while (parent && parent.level > 0) parent.expanded = true  // ← 翻回点
    }
    this.expanded = true
    ...
  }
}
```

**自闭环死锁**：收起目录 X → sync 收紧 expandedKeys（X 移除但其展开中的子孙仍在）
→ prop 变化触发回放 → expand(子孙, true) 的祖先链循环把 X 强制翻回展开态 →
双击收起永远失效。

## 2. 修复单点：可达展开面过滤（syncExpandedKeysFromStore）

`syncExpandedKeysFromStore` 不再收集全部 `expanded=true` 的 key，改收集
「根到该节点路径上每个父节点均展开」的**可达展开面**：

- 收起 X → X 子孙因祖先链断被一并剔除 → 回放无 expand 落 X 祖先链 → X 保持收起
- 全收只剩 `['']`（根展开时含根；根收起为空数组）、全展全部保留、reload 回放面正确
- 三条写路径（双击 toggle / 全展 / 全收）统一收口本单点，禁绕过

## 3. 单测纪律：el-tree 相关断言禁自造桩（假绿教训）

- 事故形态：`FolderTree.spec.js` 曾用自造 `ElTreeStub`（递归渲染插槽 + reactive
  nodesMap），桩注释声称「已挂载 Node 不响应该 prop 变化」——与真实 EP 相反
  （真实有 watch 回放 + 祖先链展开）。桩下 28 用例全绿、真机必挂。
- **禁令**：凡断言展开/收起/回放行为的用例，必须 mount **真实 EP el-tree**
  （`global.plugins: [ElementPlus]`，仅 stub el-dialog/el-input/el-dropdown 等
  无关件）。桩只允许用于展示/上报契约面（事件 emit、徽标计数、菜单禁用态）。
- 取证技巧（复现时的确定性手段，二次排查可复用）：
  - 事件 trigger 必须打在 `@dblclick` 绑定面元素（`.folder-node`），打在父容器
    `.el-tree-node__content` 上冒泡路径不经过子节点、handler 不执行
  - jsdom 无布局：`offsetParent` 恒 null，可见性判定用「自身到根链无
    `display:none`」（v-show 直接落在元素 style）
  - Vue reactive set trap 有 `value = toRaw(value)`：外层 Proxy spy 包装 reactive
    对象再写回集合会被剥壳静默失效——拦截「直接赋值」用底层 plain 对象
    `Object.defineProperty`（reactive proxy 的 Reflect.set 落 target 时触发其
    accessor，栈内可见写入者）；拦截方法调用用 prototype hook + `this.__v_raw`
    身份过滤
  - EP 构造参数 `ref(new TreeStore())` 深 reactive：测试补丁经
    `store.nodesMap[key]['__v_raw']` 取底层对象

## 4. 相关契约

- 既有「store 直赋不触发 node-expand/node-collapse 事件，操作后必须显式重算
  expandedKeys」纪律不变（防 reload 回放旧展开面漂移），本主题是其修正升级。
- `default-expanded-keys` 含根 `''` 与否：根展开时含（回放 expand 根幂等），
  根收起时为空数组（reload 后仅剩根行）。
