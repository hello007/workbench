# 交付前回归验证报告

> 任务：11-02-release-regression-verify（P0，只验证不改代码）
> 执行时间：2026-11-02 16:50–17:05
> 代码基线：`master` @ `97b7ea5`，工作区干净
> 环境：Windows 11 / Go 1.26.10 / Node v24.14.1 / npm 11.11.0 / Wails v2.16.0

## 一、结论摘要

| # | 验证项 | 命令 | 结果 | 耗时 |
|---|---|---|---|---|
| 1 | 后端全量测试 | `go test -count=1 ./...` | **通过**（6 包全 ok） | 335s |
| 2 | 后端覆盖率门禁 | `bash scripts/coverage-check.sh` | **通过**（fail=0） | 376s |
| 3 | 前端单元测试 | `cd frontend && npm test` | **通过**（103 文件 / 2275 用例） | 50s |
| 4 | 前端 E2E | `cd frontend && npm run e2e` | **失败**：122 通过 / 2 失败 | 88s |
| 5 | 桌面构建 | `wails build` | **通过**，产物 44,448,768 字节 | 43s |

**总体判定：可构建、可运行；E2E 2 例失败为测试断言过期（测试债务），非本轮产品缺陷。**

## 二、逐项明细

### 2.1 后端全量测试

```
ok  	workbench	54.874s
ok  	workbench/model	0.767s
?   	workbench/scripts/ppt-com-probe	[no test files]
ok  	workbench/server	2.012s
ok  	workbench/service	317.317s
ok  	workbench/util	22.862s
ok  	workbench/util/testutil	4.061s
```

说明：显式加 `-count=1` 强制真实执行，规避 Go 测试缓存导致的假绿。

### 2.2 覆盖率门禁

```
PASS  model: 98.2% >= 80%
PASS  server: 90.5% >= 80%
PASS  service: 81.2% >= 76%
PASS  util(排除pty_windows.go): 62.2% >= 40%
```

### 2.3 前端单元测试

```
Test Files  103 passed (103)
Tests       2275 passed (2275)
Duration    48.26s
```

非阻塞告警：Vite 提示 `vitest.config.js` 以 CommonJS 方式加载 ESM 语法，未来主版本需改 `.mjs` 或 `"type": "module"`。当前不影响执行。

### 2.4 E2E（失败项）

全量：`122 passed / 2 failed (1.4m)`。单文件复跑确认为**稳定复现**，非偶发：

```
npx playwright test e2e/assets-split.spec.js
2 failed / 1 passed (13.7s)
```

失败 1 —— `e2e/assets-split.spec.js:57`

```
Locator: locator('.el-dialog').filter({ hasText: '拆分 PPT' }).locator('.el-radio.is-checked')
Expected substring: "素材库缓存目录"
Received string:    "就地替换（删除源文件，推荐）"
```

```js
55 |     const radios = dialog.locator('.el-radio')
56 |     await expect(radios).toHaveCount(3)
57 |     await expect(dialog.locator('.el-radio.is-checked')).toContainText('素材库缓存目录')
58 |     // 默认落点为缓存目录说明（非源目录警示）
59 |     await expect(dialog.locator('.split-cache-hint')).toBeVisible()
60 |     await expect(dialog.locator('.split-source-warning')).toHaveCount(0)
```

失败 2 —— `e2e/assets-split.spec.js:157`，同一根因：

```
Expected substring: "素材库缓存目录"
Received string:    "就地替换（删除源文件，推荐）"
```

#### 根因判定

| 事实 | 证据 |
|---|---|
| 前端弹窗默认落点已改为 `replace` | `frontend/src/components/AssetsPanel.vue:1526` `const splitTarget = ref('replace')`；`:1543` 重开时同样复位 `'replace'` |
| 该变更属本轮提交 | `798fc2f fix(assets): 拆分记录随目录迁移，修复产物二次拆分`（`git log -S"splitTarget.value = 'replace'"` 唯一命中） |
| 单元测试与规范已同步 | `AssetsPanel.spec.js:3164/3182`「落点默认就地替换」；`docs/spec/pptx-split.md:98/103`「素材面板前端弹窗默认 `replace`」 |
| 仅 E2E 未同步 | `assets-split.spec.js` 最后改动于 `b1e2391`，早于 `798fc2f` |

**结论：与本轮改动相关，但属 E2E 断言未随默认值同步的测试债务；产品行为符合规范与单测口径。** 另注意用例内注释（`// 落点 radio 默认「素材库缓存目录（推荐）」`、`// 重开：恢复缓存目录默认落点`）与用例名「重开弹窗恢复默认落点」的描述也需一并更新。

#### 最小修复建议（本任务不执行）

修改 `frontend/e2e/assets-split.spec.js` 两处，使其对齐 `replace` 默认：

1. `:57` 默认断言改为「就地替换（删除源文件，推荐）」，同时 `.split-replace-warning` 可见、`.split-cache-hint` 计数 0；随后显式点选「素材库缓存目录」再提交，保留原 `target='cache'` 主链断言与后续参数校验。
2. `:157` 与 `:170` 两处默认断言改为「就地替换」、`.split-replace-warning` 可见；「取消后重开」断言改为「回到就地替换默认且强确认未勾选、提交禁用」，删除对 `.split-replace-warning` 计数 0 的断言。
3. 同步修正两处用例内注释与用例名中的「缓存目录默认落点」措辞。

验证命令：`cd frontend && npx playwright test e2e/assets-split.spec.js`。

### 2.5 桌面构建

```
Built 'D:\...\build\bin\workbench.exe' in 43.042s
-rwxr-xr-x 44448768 11月  2 17:03 build/bin/workbench.exe
```

绑定生成阶段存在 Wails 已知噪声输出 `Not found: time.Time`，构建仍成功，与本次改动无关。

## 三、真机冒烟建议清单（未执行）

| # | 场景 | 关注点 |
|---|---|---|
| 1 | 拆分三落点：`source` / `replace` / `cache` | 产物位置、索引收录、源文件是否入回收站 |
| 2 | 改名后重链再重拆 | `artifact_stem` 锚定是否命中旧页、旧页是否按预期清理 |
| 3 | 落点切换（产物容器目录被移动） | `output_dir` 重算与 `splitOutDirUnderPrefix` 前缀判定 |
| 4 | 备份 / 还原 | 版本过新拒绝、空库拒绝、还原后 `-wal/-shm` 清理 |
| 5 | 删除素材库 | 关联记录与磁盘产物清理 |
| 6 | 目录重命名 / 移动 / 前缀替换 | `split_records` 随迁移是否与 `files.path` 同事务一致 |

## 四、遗留与建议

1. **需修复（测试债务，建议单开子任务）**：`assets-split.spec.js` 两处默认落点断言过期，导致 E2E 整体红；修复面仅限该 spec 文件，不涉及产品代码。
2. **可选（非阻塞）**：`vitest.config.js` 的 CJS/ESM 加载告警，可在后续工具链升级中一并处理。
3. 其余四项（后端测试、覆盖率门禁、前端单测、桌面构建）本轮无需处理。
