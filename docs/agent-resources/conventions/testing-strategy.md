---
title: 前端测试策略与规范
scope: [testing, quality, frontend]
targets: [test]
---

# 前端测试策略与规范

> **权威声明**：本文件约束 `docs/agent-resources/` 体系内的通用前端测试规则；本仓覆盖率门禁阈值等专项基线以仓根 AGENTS.md「关键规则」与 `docs/测试策略.md` 为准，两者冲突时以后者为准。

## 1. 测试分层与工具选型

| 测试层次 | 关注重点 | 选型工具 |
|---|---|---|
| **单元测试 (Unit)** | 工具函数 (`frontend/src/utils/`)、业务逻辑 (composables)、store 状态 | Vitest |
| **组件测试 (Component)** | 组件渲染、Props 传入响应、Emits 事件派发 | `@vue/test-utils` + Vitest |
| **端到端测试 (E2E)** | 核心业务流程主链路 | Playwright |

> 本仓已显式声明 `@vue/test-utils`、`@vitest/coverage-v8`、`@playwright/test`，组件挂载测试与覆盖率命令均可用。

## 2. 测试用例编写规范

### 命名与组织

- 测试文件与被测文件同名，使用 `.spec.js` 后缀（本仓惯例，非 `.test.js`），放置在同级 `__tests__/` 目录。
- 新增 E2E spec 目录必须同步 `vitest.config.js` 的 `test.exclude`，避免单测误跑 E2E 文件。

### 示例（Composable 逻辑测试）

```js
import { describe, it, expect } from 'vitest';
import { useCounter } from '../useCounter';

describe('useCounter', () => {
  it('递增后计数为 1', () => {
    const { count, increment } = useCounter(0);
    increment();
    expect(count.value).toBe(1);
  });
});
```

### 示例（Vue 组件挂载测试）

```js
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import BaseButton from '../BaseButton.vue';

describe('BaseButton.vue', () => {
  it('点击后派发 click 事件', async () => {
    const wrapper = mount(BaseButton, {
      props: { text: '提交' },
    });
    await wrapper.trigger('click');
    expect(wrapper.emitted()).toHaveProperty('click');
  });
});
```

## 3. 测试数据与 Mock 规范

- Wails 后端调用的 mock 数据以 `frontend/src/test/wails-mock-defaults.js` 为单一数据源，禁止在各测试文件内复制粘贴大块 mock JSON。
- mock 数据形状必须对齐后端 model 的真实序列化形状（字段名、嵌套结构），禁止凭前端想象构造。
- 外部副作用（网络请求、剪贴板、时间）必须在测试中通过 `vi.mock()` 或注入隔离，禁止在单测中触碰真实环境。
- 含中文数据的往返逻辑（编码/解码）单测必须使用中文测试数据。

## 4. 运行与验证

在 `frontend/` 目录下执行：

```bash
npm test            # 交互式运行测试（本地 watch 模式）
npm run test:coverage   # 生成覆盖率报告
npm run e2e         # Playwright E2E（首次先 npm run e2e:install）
```
