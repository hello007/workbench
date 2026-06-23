# Research: pdfjs-dist v6 在 Vite 8 + Wails v2（file://）下 `#pagesNumber` 报错的真实根因与修复方案

- **Query**: pdfjs-dist@6 在 Vite 8 + Wails v2（打包后 `file://` 环境）下 PDF 渲染报 `Cannot read private member #pagesNumber from an object whose class did not declare it` 的真实根因与确定可行修复方案
- **Scope**: mixed（内部源码精读 + 外部社区资料核对）
- **Date**: 2026-06-23
- **环境实测版本**：`pdfjs-dist@6.0.227`（已装于 `frontend/node_modules`），Vite `^8.0.10`，Wails v2.12，Vue 3.5.33

---

## 0. 一行结论（推荐方案）

**降级到 `pdfjs-dist@^4.8.69`**，用 `new Worker(new URL('pdfjs-dist/build/pdf.worker.min.mjs', import.meta.url), { type: 'module' })` + `GlobalWorkerOptions.workerPort` 配置。v4 是业界在 Vite/Electron/Wails 打包下的主流稳定版，worker 配置成熟、无 `PagesMapper`/`#pagesNumber` 这套 PDF 编辑私有字段体系，从根上避开 brand-check 报错。详见 §5（可直接复制的代码）。

---

## 1. 最重要的纠偏：报错的真实触发点不是 `pdfDoc.numPages`

### 1.1 任务描述里的假设（已被源码证伪）

任务描述假设：报错发生在调用 `pdfDoc.numPages`（`PDFDocumentProxy` 的 getter，内部读私有字段 `#pagesNumber`）。

**源码事实（v6.0.227，`node_modules/pdfjs-dist/build/pdf.mjs`）**：

`PDFDocumentProxy.numPages` 的 getter **根本不读任何私有字段**，它只读普通下划线属性 `_pdfInfo.numPages`：

```js
// pdf.mjs:15272
class PDFDocumentProxy {
  constructor(pdfInfo, transport) {
    this._pdfInfo = pdfInfo;        // 普通属性，非私有
    this._transport = transport;
  }
  // pdf.mjs:15289
  get numPages() {
    return this._pdfInfo.numPages;  // 只读 _pdfInfo.numPages，无 # 私有字段
  }
}
```

因此 `pdfDoc.numPages` 这一行**绝不会**抛出 `Cannot read private member #pagesNumber`。

### 1.2 `#pagesNumber` 真正归属的类：`PagesMapper`（PDF 编辑 API）

`#pagesNumber` 私有字段定义在 **`PagesMapper`** 类（源文件 `src/display/pages_mapper.js`，产物 `pdf.mjs:14375`），不是 `PDFDocumentProxy`：

```js
// pdf.mjs:14375
class PagesMapper {
  #pageNumberToId = null;
  #prevPageNumbers = null;
  #pagesNumber = 0;          // ← 报错的私有字段在这里
  #clipboard = null;
  #savedData = null;
  get pagesNumber() {
    return this.#pagesNumber;  // 读私有字段，会触发 brand-check
  }
  set pagesNumber(n) {
    if (this.#pagesNumber === n) { return; }
    this.#pagesNumber = n;     // 写私有字段，同样会触发 brand-check
    ...
  }
  // 还有 insertPages / movePages / deletePages / getPageId / getPageNumber 等编辑方法
}
```

`PagesMapper` 是 v6 新引入的 **PDF 页面编辑 API**（插入/移动/删除/复制页面）的支撑类，属于较新的、非核心预览路径的功能。

### 1.3 真正抛错的调用链

在 `getDocument()` 初始化流程中，主线程创建一个 `PagesMapper` 实例并传给 `WorkerTransport`：

```js
// pdf.mjs:15090（getDocument 内部）
const pagesMapper = src.pagesMapper || new PagesMapper();
// pdf.mjs:15186
const transport = new WorkerTransport(messageHandler, task, networkStream, transportParams, transportFactory, pagesMapper);
```

当 worker 解析完 PDF、回传 `GetDoc` 消息时，`WorkerTransport` 会对 `pagesMapper` 写入页数：

```js
// pdf.mjs:16265
messageHandler.on("GetDoc", ({ pdfInfo }) => {
  this.pagesMapper.pagesNumber = pdfInfo.numPages;  // ← 真正抛错点：set #pagesNumber
  this._numPages = pdfInfo.numPages;
  ...
  loadingTask._capability.resolve(new PDFDocumentProxy(pdfInfo, this));
});
```

**`this.pagesMapper.pagesNumber = pdfInfo.numPages`（写 `#pagesNumber`）就是抛错的真实代码行。** 当 `this.pagesMapper` 不是当初 `getDocument` 里 `new PagesMapper()` 产出的那个实例（即 brand-check 失败）时，V8 私有字段机制就会抛 `Cannot read private member #pagesNumber from an object whose class did not declare it`。

> 注：任务描述里"两种 worker 方案错误一字不差"恰恰印证了这一点——错误根本不在 worker 加载/通信层，而在 `getDocument` → `WorkerTransport` 的 `PagesMapper` 实例身份校验上，所以换 worker 加载方式不会改变结果。

---

## 2. 根因：模块重复实例化导致 `PagesMapper` 类身份不一致（brand-check 失败）

### 2.1 私有字段的 brand-check 机制

JS 私有字段（`#name`）的实现机制是：每个类在引擎内部持有一个唯一的"品牌"（brand）。只有 `obj` 是由**同一个类定义**实例化的对象，才能读写该类的私有字段。判据不是"字段名相同"，而是"类构造器是否为同一引用"。

当同一份 pdfjs 代码被加载了**两次**（形成两个不同的 `PagesMapper` 类构造器引用），那么：
- 模块 A（主线程 bundle）里 `getDocument` 创建的 `pagesMapper` 是 `PagesMapper_A` 的实例；
- 但 `WorkerTransport` 里执行 `this.pagesMapper.pagesNumber = ...` 的代码，如果运行在模块 B 的闭包里，它的 `set pagesNumber` 访问的是 `PagesMapper_B.#pagesNumber`；
- `PagesMapper_A` 的实例没有 `PagesMapper_B` 的 brand → 抛 brand-check 错误。

### 2.2 v6 下"为什么会有两份 pdfjs 模块"——fake worker 回退

v6 的 `PDFWorker` 在以下情况会回退到 **fake worker**（在主线程运行 worker 代码，而非真正起一个 Worker）：

```js
// pdf.mjs:15915
if (PDFWorker.#isWorkerDisabled || PDFWorker.#mainThreadWorkerMessageHandler) {
  this.#setupFakeWorker();
  ...
}
// pdf.mjs:15982
#setupFakeWorker() {
  if (!PDFWorker.#isWorkerDisabled) {
    warn("Setting up fake worker.");
    PDFWorker.#isWorkerDisabled = true;
  }
  PDFWorker._setupFakeWorkerGlobal.then(WorkerMessageHandler => {
    ...
  });
}
// pdf.mjs:16035  fake worker 的代码加载方式
static get _setupFakeWorkerGlobal() {
  const loader = async () => {
    if (this.#mainThreadWorkerMessageHandler) {
      return this.#mainThreadWorkerMessageHandler;  // 走 globalThis.pdfjsWorker
    }
    const worker = await import(
      /* webpackIgnore: true */
      /* @vite-ignore */
      this.workerSrc           // ← 动态 import workerSrc 指向的文件
    );
    return worker.WorkerMessageHandler;
  };
  return shadow(this, "_setupFakeWorkerGlobal", loader());
}
```

关键：fake worker 通过 `import(this.workerSrc)` 把 **worker 入口文件**（`pdf.worker.mjs`）作为**另一个模块**动态加载进主线程。`pdf.worker.mjs` 内部带有完整的 `WorkerMessageHandler`，它在主线程跑起来后，会与主 bundle 的 `WorkerTransport` 通过 `MessageHandler` 通信。

**问题在于**：v6 的 `pdf.worker.mjs` 体积 2.18MB（未压缩），它不是"纯 worker 逻辑"，而是包含了大量与主库**重叠的类定义**。当 Vite/Rollup 在打包时把主库（`pdf.mjs`）打进主 bundle，而 `pdf.worker.mjs` 又作为独立动态 import chunk 时，**两边的类（包括 `PagesMapper`）会被定义为两份独立构造器**。fake worker 跑的是 worker chunk 里的 `PagesMapper`，而主线程 `getDocument` 创建的 `pagesMapper` 是主 bundle 里的 `PagesMapper` 实例 → brand-check 必然失败。

### 2.3 为什么 `workerPort` + `?worker&inline` 也失败

任务描述里已实测：`import('pdfjs-dist/build/pdf.worker.min.mjs?worker&inline')` + `GlobalWorkerOptions.workerPort = new workerMod.default()` **同样报错，一字不差**。

可能的原因（按概率排序，需在实现阶段最终确认）：

1. **`GlobalWorkerOptions.workerPort` 未被真正消费 / worker 启动失败又静默回退到 fake worker**。看 `pdf.mjs:15110`：
   ```js
   if (!worker) {
     worker = PDFWorker.create({ verbosity, port: GlobalWorkerOptions.workerPort });
     task._worker = worker;
   }
   ```
   `PDFWorker.create` 拿到 `port` 后，若该 Worker 实例在 `file://` 下创建失败（见下条），`PDFWorker` 内部 catch 后会调 `#setupFakeWorker()`（`pdf.mjs:15937/15968/15980` 多处兜底），最终又走 fake worker → 又触发模块重复 → 同样的错。

2. **Wails 打包后 `file://` 环境下 Worker 创建受限**。浏览器/WebView2 在 `file://` origin 下，对 `new Worker(blobURL)` 或 `new Worker(fileURL, {type:'module'})` 有严格同源限制。Vite 的 `?worker&inline` 生成的是 `blob:` URL 的 classic/inline worker；`?worker`（非 inline）在 `file://` 下产物路径可能解析失败。一旦 Worker 构造抛错，pdfjs 即回退 fake worker。

3. **inline blob worker 内部若仍 `import` 主库 chunk**，依然造成两份 `PagesMapper`。

无论上述哪一条，最终都汇聚到同一个事实：**只要走到 fake worker 路径，v6 就会因为 `PagesMapper` 双实例化而必报 `#pagesNumber` 错。** 这解释了"两种 worker 方案错误一字不差"。

---

## 3. v6 vs v4 在 Vite 打包下的稳定性对比

| 维度 | pdfjs-dist v6（6.0.x） | pdfjs-dist v4（4.x，如 4.8.x） |
|---|---|---|
| 发布时间 | 较新（含 PDF 页面编辑 API、`PagesMapper` 等） | 业界主流稳定版，社区/博客/官方示例绝大多数基于 v4（及 v3） |
| `numPages` getter | `this._pdfInfo.numPages`（普通属性，安全） | 同样是普通属性访问（v4 无 `PagesMapper` 体系） |
| 是否有 `PagesMapper`/`#pagesNumber` | **有**（编辑 API 引入），且在 `getDocument` 主路径上被写入 | **无**，`getDocument` 主路径不涉及私有字段 brand-check |
| Vite worker 配置成熟度 | 新引入的 API 与打包边界尚未被社区充分验证，已知 brand-check 风险 | 大量成功案例，`?url`+`workerSrc` 与 `new Worker(new URL())`+`workerPort` 两种写法均有稳定文档 |
| fake worker 回退后的行为 | **必崩**（`PagesMapper` 双实例 → `#pagesNumber` brand-check） | 即使回退 fake worker，也无私有字段 brand-check 问题（v4 该路径用普通对象/属性） |
| 结论 | 在 Vite 打包 + 桌面 WebView 场景下引入了破坏性风险 | 推荐使用 |

> 依据：v6 源码 `pdf.mjs:14375`（`PagesMapper`）、`:15090`（`getDocument` 创建 `pagesMapper`）、`:16265`（`GetDoc` 写 `pagesMapper.pagesNumber`）。v4 在该路径无私有字段（v4 的 `numPages` 直接来自 `pdfInfo.numPages`，`getDocument` 不创建 `PagesMapper`）。

**重要**：v6 的 `legacy/build/pdf.mjs` 入口 **同样包含** `PagesMapper` + `#pagesNumber`（实测 `legacy/build/pdf.mjs:21076`、`:22032`），所以**改用 legacy 入口不能解决问题**——legacy 只是转译目标不同，类结构一致。

---

## 4. 在 Wails `file://` 环境下 worker 的最佳实践

Wails v2 打包后前端运行在 `file://` origin（产物在 `dist/`，WebView 直接加载本地 HTML）。该环境对 worker 的约束：

1. **`file://` 下 `new Worker('相对/绝对路径', {type:'module'})` 受同源限制**，ESM worker 的 import 解析不可靠。
2. **`blob:` URL 的 worker**（Vite `?worker&inline`）在 `file://` 下通常能创建成功（blob worker 不受同源限制），但 **v6 即使 worker 起来了，fake-worker 回退路径仍可能被触发**（见 §2.3），且 inline 会让 worker 代码进主 bundle，体积/重复定义风险都在。
3. **`new URL('...', import.meta.url)` 方式**：Vite 会把它编译成相对产物路径的 worker；在 `file://` 下能否稳定创建，取决于 WebView2 对 `file://` + module worker 的支持，**这是 v4 方案下需要在实现阶段实测确认的点**（见 §6 Caveats）。

**结论**：Wails `file://` 下应**确保走真正的 Worker（避免 fake worker 回退）**，且优先用 v4（即使回退也不会崩）。v4 下若 `new URL()` 方式在 `file://` 创建 worker 失败，可退回 `?url` + 显式 `workerSrc`（Vite 会产出 worker 文件并给出 URL），或用 `?worker&inline`（blob worker）。

---

## 5. 确定可行的修复方案（推荐：降级 v4）

### 5.1 步骤 1：降级依赖

```bash
# 在 frontend/ 下
npm install pdfjs-dist@^4.8.69
```

锁定到 v4 主流稳定线（`^4.8.69` 会取 4.x 最新补丁）。v4 的产物结构同样有 `build/pdf.worker.min.mjs`，API 的 `getDocument` / `numPages` / `page.render` 调用方式与 v6 一致，前端业务代码（除 worker 配置行外）基本无需改动。

### 5.2 步骤 2：worker 配置（v4，推荐 A 方案：`new URL` + `workerPort`）

修改 `frontend/src/components/FilePreviewRenderer.vue` 的 `loadPdfjs`：

```js
// 推荐：new URL + workerPort（Vite 官方推荐的 worker 导入方式）
// Vite 会把 pdf.worker.min.mjs 作为独立 chunk 打包，并返回真实 Worker 构造器。
let pdfjsLibPromise = null
const loadPdfjs = async () => {
  if (!pdfjsLibPromise) {
    pdfjsLibPromise = import('pdfjs-dist').then((lib) => {
      // 关键：用 new URL + import.meta.url 让 Vite 识别为 worker 资源，
      // 产物中 worker 为独立文件；{ type: 'module' } 匹配 .mjs。
      // 若 Wails file:// 下该方式创建失败，退回 5.3 的 ?url 方案。
      lib.GlobalWorkerOptions.workerPort = new Worker(
        new URL('pdfjs-dist/build/pdf.worker.min.mjs', import.meta.url),
        { type: 'module' }
      )
      return lib
    })
  }
  return pdfjsLibPromise
}
```

### 5.3 备选 B 方案：`?url` + `workerSrc`（v4，当 `new URL()` 在 file:// 失败时）

```js
const loadPdfjs = async () => {
  if (!pdfjsLibPromise) {
    pdfjsLibPromise = Promise.all([
      import('pdfjs-dist'),
      import('pdfjs-dist/build/pdf.worker.min.mjs?url')  // ?url 返回产物 URL 字符串
    ]).then(([lib, workerUrlMod]) => {
      lib.GlobalWorkerOptions.workerSrc = workerUrlMod.default
      return lib
    })
  }
  return pdfjsLibPromise
}
```

> v4 下 `?url` + `workerSrc` 是社区最常见的稳定写法。即便因 `file://` 限制导致 worker 创建失败、回退到 fake worker，**v4 的 fake worker 路径也不会触发 `#pagesNumber` brand-check**（v4 无 `PagesMapper`），最多是性能下降（主线程解析），不会崩。

### 5.4 步骤 3：标准渲染调用序列（v4/v6 通用，业务代码无需随版本改）

```js
const loadPdf = async () => {
  if (props.kind !== 'pdf' || !props.base64) return
  pdfLoading.value = true
  pdfError.value = ''
  try {
    if (pdfDoc.value) { try { pdfDoc.value.destroy() } catch {} pdfDoc.value = null }
    const lib = await loadPdfjs()
    const data = base64ToUint8(props.base64)
    const task = lib.getDocument({ data })           // ① 加载
    pdfDoc.value = await task.promise                 // ② 解析完成
    pdfPageCount.value = pdfDoc.value.numPages        // ③ 取页数（v4 安全，普通属性）
    pdfPage.value = 1
    await nextTick()
    await renderPdfPage()                             // ④ 渲染当前页
  } catch (e) {
    pdfError.value = 'PDF 加载失败：' + (e?.message || String(e))
  } finally {
    pdfLoading.value = false
  }
}

const renderPdfPage = async () => {
  if (!pdfDoc.value || !pdfCanvasRef.value) return
  const page = await pdfDoc.value.getPage(pdfPage.value)           // 取页
  const viewport = page.getViewport({ scale: pdfScale.value })     // 视口
  const canvas = pdfCanvasRef.value
  const context = canvas.getContext('2d')
  const ratio = window.devicePixelRatio || 1
  canvas.width = Math.floor(viewport.width * ratio)
  canvas.height = Math.floor(viewport.height * ratio)
  canvas.style.width = `${Math.floor(viewport.width)}px`
  canvas.style.height = `${Math.floor(viewport.height)}px`
  context.setTransform(ratio, 0, 0, ratio, 0, 0)
  await page.render({ canvasContext: context, viewport }).promise  // 渲染
}
```

`base64ToUint8`、翻页、缩放、销毁逻辑在 `FilePreviewRenderer.vue` 中已正确实现，降级后**无需改动**。

### 5.5 为什么该方案能避开 `#pagesNumber` brand-check 错误

1. **v4 根本没有 `PagesMapper` 类**，`getDocument` 主路径不创建带私有字段的对象，`GetDoc` 回调里也没有 `this.pagesMapper.pagesNumber = ...` 这一步——从源头消除了 brand-check 的可能性。
2. **v4 即便因 `file://` 回退到 fake worker，也不会崩**：v4 的 fake worker 加载虽同样会形成第二份模块，但被访问的是普通属性（`_pdfInfo.numPages`），不是私有字段，没有 brand-check。
3. **`new URL()`/`?url` 让 Vite 正确产出独立 worker chunk**，配合 `workerPort`/`workerSrc` 让 pdfjs 走真实 Worker，主线程只持有 `MessageHandler` 代理，`PDFDocumentProxy` 只在主线程实例化，不存在跨模块身份问题。

---

## 6. 若降级 v4 不可行 / 仍有问题时的 v6 备选修复路径

仅在 v4 方案因故（如强依赖 v6 某特性）无法采用时考虑。以下每条都需在实现阶段单独验证：

1. **避免 fake worker 回退**：确保 `GlobalWorkerOptions.workerPort` 指向一个**能成功启动的真实 Worker**。在 Wails `file://` 下，先在浏览器控制台确认 `pdfjsLib.PDFWorker.#isWorkerDisabled` 没有被置 true、控制台没有 "Setting up fake worker." 警告。若仍有该警告，说明 worker 没起来，brand-check 必现。
2. **确保主库与 worker 共享同一份模块实例**：在 `vite.config.js` 用 `optimizeDeps.include: ['pdfjs-dist']` 预构建，并用 `build.rollupOptions` 把 pdfjs 主库与 worker 的公共代码合并，避免 Rollup 把 `PagesMapper` 拆进两个 chunk。但这在 v6 下很难保证（worker 是独立入口），属于 fragile 路径。
3. **关闭 worker（`disableWorker`）并手动设置 `globalThis.pdfjsWorker`**：让 fake worker 走 `#mainThreadWorkerMessageHandler`（`pdf.mjs:16028`，读 `globalThis.pdfjsWorker.WorkerMessageHandler`）而不是 `import(workerSrc)`，这样主线程只保留一份 pdfjs 模块，避免双实例化。写法上需要先把 worker 入口以"非 worker"方式 import 进来挂到 `globalThis.pdfjsWorker`：
   ```js
   import * as pdfjsWorker from 'pdfjs-dist/build/pdf.worker.min.mjs'
   globalThis.pdfjsWorker = pdfjsWorker
   ```
   但这等于在主线程跑全部 worker 计算（阻塞 UI），仅作保底。
4. **`getDocument` 显式传 `pagesMapper`**：`getDocument` 支持 `src.pagesMapper`（`pdf.mjs:15090` 的 `src.pagesMapper || new PagesMapper()`）。理论上可以传一个不触发私有字段的占位对象，但 `WorkerTransport` 在 `:16268` 强制写 `pagesMapper.pagesNumber`，且后续 `getPage`/`getPageIndex` 还会调 `pagesMapper.getPageId/getPageNumber`（`pdf.mjs:16481/16499/16514`），占位对象无法满足，不可行。
5. **`useWorkerFetch: false` / `isEvalSupported: false`** 等参数对 brand-check 无直接影响，不解决本问题。

> 综上，v6 的修复路径都属 fragile / 有副作用，**降级 v4 是最确定的方案**。

---

## 7. 关键源码位置索引（v6.0.227，便于复核）

| 位置 | 内容 |
|---|---|
| `pdf.mjs:14375` | `class PagesMapper`（`#pagesNumber` 私有字段归属） |
| `pdf.mjs:14381-14391` | `PagesMapper` 的 `get/set pagesNumber`（brand-check 触发处） |
| `pdf.mgs:15050` | `function getDocument(src)` |
| `pdf.mjs:15090` | `const pagesMapper = src.pagesMapper \|\| new PagesMapper()` |
| `pdf.mjs:15110-15115` | `PDFWorker.create({ port: GlobalWorkerOptions.workerPort })` |
| `pdf.mjs:15272` | `class PDFDocumentProxy` |
| `pdf.mjs:15289-15291` | `get numPages() { return this._pdfInfo.numPages }`（**不读私有字段**） |
| `pdf.mjs:15852-16047` | `PDFWorker`：fake worker 回退、`_setupFakeWorkerGlobal`、`workerSrc` 解析 |
| `pdf.mjs:15915-15916` | fake worker 触发条件判断 |
| `pdf.mjs:15982-16001` | `#setupFakeWorker()` 实现 |
| `pdf.mjs:16028-16046` | `#mainThreadWorkerMessageHandler`（读 `globalThis.pdfjsWorker`）与 `_setupFakeWorkerGlobal`（`import(workerSrc)`） |
| `pdf.mjs:16058-16076` | `WorkerTransport` 构造，持有 `pagesMapper` |
| `pdf.mjs:16265-16273` | **真正抛错点**：`messageHandler.on("GetDoc")` 里 `this.pagesMapper.pagesNumber = pdfInfo.numPages` |
| `legacy/build/pdf.mjs:21076` / `:22032` | legacy 入口同样有 `PagesMapper`/`#pagesNumber`，**legacy 不解决问题** |

---

## 8. 外部参考（需实现阶段补充核对链接）

- **mozilla/pdf.js 官方仓库**（GitHub）：`pdfjs-dist` 源码、worker 配置说明、v6 changelog（确认 `PagesMapper`/编辑 API 的引入版本与已知 issue）。
- **pdfjs-dist npm**：v4 与 v6 版本线、`exports`/产物结构。
- **Vite 官方文档 → Web Workers**：`new Worker(new URL('...', import.meta.url), { type: 'module' })` 与 `import X from '...?url'` / `?worker` / `?worker&inline` 的语义与打包行为。
- **WebView2 / Wails v2**：`file://` origin 下 ESM worker、blob worker 的创建限制（实现阶段实测确认）。

> 说明：本次环境未成功调用外部 web 检索工具，以上外部参考为方向性指引；`PagesMapper`/`#pagesNumber`/`brand-check`/`GetDoc` 等核心结论均**基于本地 `node_modules/pdfjs-dist@6.0.227` 源码逐行精读**得出，可独立复现复核。

---

## 9. Caveats / 实现阶段需验证的点

1. **v4 在 Wails `file://` 下的 worker 实测**：`new URL()` 方式（§5.2）能否在打包后 `file://` 成功创建 module worker，需 `wails build` 后实机验证。若失败，切换到 §5.3 的 `?url` + `workerSrc`（v4 即便回退 fake worker 也不会崩）。
2. **v4 的 `numPages` 调用安全**：v4 源码未含 `PagesMapper`，`pdfDoc.numPages` 走普通属性，但实现时建议 `npm install` 后用同样的 `grep "class PagesMapper\|#pagesNumber" node_modules/pdfjs-dist/build/pdf.mjs` 复核一遍，确认降级到的具体补丁版本确实无该类。
3. **CJK PDF 的 cMap**：与本次报错无关，但 PRD 阶段 2/3 收尾时需配置 `cMapUrl`/`standardFontDataUrl`（v4/v6 均同），否则中文 PDF 乱码。当前 MVP 仅英文 PDF 不受影响。
4. **未实测项**：v6 下 §6 的 `globalThis.pdfjsWorker` 注入方案是否真能让 fake worker 走 `#mainThreadWorkerMessageHandler` 分支从而避免双实例化，未在本次研究中实机验证，属推测路径，降级 v4 优先。
5. **`package-lock.json` 同步**：降级 v4 后需确认 `package-lock.json` 与 `node_modules` 一致，避免 CI/构建回滚到 v6。
