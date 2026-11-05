---
title: Vue 3 组件设计与编码规范
scope: [vue, coding-style]
targets: [vue, store, composable]
---

# Vue 3 组件设计与编码规范

本文定义 Vue 3 前端工程的组件设计、组合式 API、响应式处理、样式隔离与状态管理规则（适用于 `frontend/src/`）。

> 本仓前端为 **JavaScript（非 TypeScript）**：SFC 不带 `lang="ts"`，类型语义用运行时校验与 JSDoc 注释表达。

## 1. 单文件组件（SFC）排布规范

- **代码块顺序**：统一按 `<script setup>` -> `<template>` -> `<style scoped>` 排布。
- **命名规范**：
  - 组件文件名使用 PascalCase（大驼峰），如 `UserProfileCard.vue`、`NavHeader.vue`。
  - 基础通用组件前缀为 `Base` 或 `App`，如 `BaseButton.vue`。
  - 单例组件以 `The` 前缀，如 `TheSidebar.vue`。

## 2. 组合式 API 与 Script Setup

- **统一使用 `<script setup>`**：禁止使用 Options API（选项式）。
- **Props 与 Emits 声明**：使用运行时声明并配套默认值，禁止裸用未声明的 props：

  ```js
  const props = defineProps({
    userId: { type: String, required: true },
    showDetail: { type: Boolean, default: false },
  });

  const emit = defineEmits(['update:status', 'close']);
  ```

## 3. 响应式最佳实践与避坑

- **基础与对象声明**：
  - 基础数据类型必须使用 `ref()`。
  - 复杂对象或状态聚合体使用 `reactive()` 或 `ref()`。
  - 复杂第三方实例（如 ECharts 实例、CodeMirror/xterm 编辑器实例）必须使用 `shallowRef()` 或 `markRaw()`，严禁深度代理导致内存泄露与性能卡顿。
- **严禁直接解构**：
  - 对 `reactive` 或 Props 进行解构会直接丢失响应性！如需解构必须使用 `toRefs()` 或 `toRef()`。
- **计算属性（computed）**：
  - 必须为无副作用的只读纯函数，严禁在 `computed` 内部修改其他状态或发起异步请求。
- **监听器（watch / watchEffect）**：
  - 需要对比新旧值或进行深度防抖时使用 `watch(source, callback, options)`；
  - 自动追踪依赖的 `watchEffect()` 仅在明确不需要旧值时使用。

## 4. 样式规范

- **样式隔离策略**：
  - 业务页面组件声明 `<style scoped>` 避免样式全局污染；需覆盖 Element Plus 组件样式时统一使用 `:deep(.target-class)` 语法。
  - 主题与设计令牌优先引用全局 CSS 变量（`app.css` / `style.css` 中的 `--*` 令牌），禁止在组件内硬编码主题色值。

## 5. 跨组件状态共享（Composables 与 Provide / Inject）

- 跨多个组件共享逻辑与状态时，优先编写可复用的组合式函数（`frontend/src/composables/`）或利用 `provide()` / `inject()` 依赖注入：

```js
import { ref, readonly } from 'vue';

// 模块级响应式状态（轻量全局/局部共享）
const globalCount = ref(0);

export function useGlobalCounter() {
  const increment = () => {
    globalCount.value += 1;
  };

  return {
    count: readonly(globalCount),
    increment,
  };
}
```

## 6. Pinia 全局状态管理规范

> 本仓已显式依赖 `pinia`（store 目录：`frontend/src/store/`）。

### Setup Stores 语法糖

统一使用类似组合式 API 的 Setup Store 函数签名：

```js
import { defineStore } from 'pinia';
import { ref, computed } from 'vue';

export const useUserStore = defineStore('user', () => {
  // state: ref()
  const token = ref('');

  // getters: computed()
  const isAuthenticated = computed(() => Boolean(token.value));

  // actions: function()
  function setToken(val) {
    token.value = val;
  }

  return { token, isAuthenticated, setToken };
});
```

### 状态组织

- **单一职责**：按业务域拆分（如现有 `workspace.js`、`settings.js`、`ui.js`、`aiChat.js`），禁止创建上帝 Store。
- **敏感凭证**：任何 Token、密码不得明文持久化到 localStorage。
