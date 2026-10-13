# Research: Go SQLite 驱动 + FTS5 中文全文检索方案

- **Query**: Go 生成 SQLite 驱动 + FTS5 中文全文检索方案（10 万级幻灯片页索引，关键词如「城商行」「获客」需命中，检索 P95 < 1s，Windows 为主、需兼容 Linux 交叉编译，禁 CGO 新依赖）
- **Scope**: mixed（本地实证基准 + 模块源码验证 + GitHub 生态调研）
- **Date**: 2026-10-13
- **验证环境**: Go 1.26.6 / Windows 11 amd64 / modernc.org/sqlite v1.60.1（内嵌 SQLite 3.53.4）

## 结论速览

**推荐方案 A：FTS5 + unicode61 分词器 + 应用层 CJK 按字切分 + phrase 查询 + bm25 列加权。**

三条决定性证据（全部本地实测）：

1. **unicode61 默认对中文完全不可用**：连续中文（无空格标点分隔）被切成「一个长 token」，`MATCH '城商行'` 与 `MATCH '"城商行"'` 均命中 0 行；应用层在每个 CJK 字符后插空格后，phrase 查询与 `LIKE '%关键词%'` 语义**精确等价**（实测命中数逐条一致：2001/2001）。
2. **trigram 无法命中 2 字词**：`MATCH '获客'` 恒返回 0 行且无报错（trigram 最小 3 字符）；列级 LIKE 回退全表扫，2 字词 p95 达 0.63–1.01s，踩线超标。
3. **方案 A 全查询类型 p95 ≤ 25ms（10 万行）**，是 1s 预算的 1/40；纯 Go 驱动交叉编译实测通过（linux/amd64、windows/arm64），零 CGO。

## 1. modernc.org/sqlite 驱动验证

### 1.1 FTS5 编译开关（源码确认）

| 项目 | 结论 | 证据 |
|---|---|---|
| FTS5 内置 | **是，默认开启** | `lib/sqlite.go` 编译开关包含 `SQLITE_ENABLE_FTS5`（同时含 FTS3/FTS4/RTREE/DBSTAT/MATH 等） |
| trigram tokenizer | **是** | SQLite ≥ 3.34.0 引入，v1.60.1 内嵌 3.53.4；基准实测 trigram 表创建与查询正常 |
| 内嵌 SQLite 版本 | 3.53.4（2026 年版） | `SQLITE_VERSION = "3.53.4"` |
| 最新版本 | v1.60.1（2026-09-29） | `go list -m -versions` 确认 |
| Go 版本要求 | **v1.60.0 起要求 Go ≥ 1.26** | CHANGELOG 明确说明；本项目 go 1.26.6 满足 |
| 自定义 FTS5 分词器注册 | **Go 层未暴露 API** | 模块全部 `.go` 文件无 tokenizer 注册入口（仅 C 层结构体）→ 自定义分词器路线在纯 Go 驱动上不可行，应用层预处理是现实路径 |

### 1.2 平台与交叉编译（实测）

- `lib/` 平台文件覆盖：`sqlite_windows.go`、`sqlite_linux_386/amd64/arm`、`sqlite_darwin_amd64/arm64`、`sqlite_freebsd_*` 全家桶。
- 实测 `CGO_ENABLED=0` 下 **`GOOS=linux GOARCH=amd64` 与 `GOOS=windows GOARCH=arm64` 交叉编译均通过**（用包含 sqlite 依赖的真实程序验证）。
- 符合项目约束：与 go-git 同类的纯 Go 依赖先例；Wails Windows/Linux 构建链无需 mingw/gcc。

### 1.3 版本注意事项（CHANGELOG 提炼）

| 注意事项 | 说明 |
|---|---|
| libc 版本对齐 | 升级 sqlite 版本时，`go.mod` 中 `modernc.org/libc` 必须与上游 pin 同版本，否则测试失败（上游 issue #177） |
| v1.60.1 性能修复 | 修复绑定上千 `?` 参数的多行 INSERT 二次方复杂度（批量导入场景直接受益） |
| v1.59 性能优化 | Linux 目标上 transpiled musl memcpy/memmove 等替换为原生 Go 实现，查询型负载 CPU 降低约 1/3 |
| 与 C 版性能差距 | 纯 Go 转译版比 C 编译版 SQLite 慢（官方 v1.59 起在包文档新增 Performance 章节说明）；对本地桌面检索场景（P95 < 1s 目标）实测完全够用 |
| 驱动使用 | 建议单写连接（`SetMaxOpenConns` 收敛）+ DSN `_pragma=journal_mode(WAL)`/`_pragma=busy_timeout(...)` 形式；与项目「手改 data 文件前关 workbench.exe」的整体覆盖模型规则兼容 |

## 2. 分词器对中文的实际行为（实测，非文档推断）

### 2.1 unicode61 默认行为：对中文不可用

对 1000 行含「全面服务城商行客户数字化转型需求」的未切分文本实测：

| 查询写法 | 命中 | 解释 |
|---|---|---|
| `MATCH '"城商行"'`（短语） | **0 行** | 连续中文被 unicode61 切为**一个长 token**「全面服务城商行客户数字化转型需求」，短语无法在 token 中间命中 |
| `MATCH '城商行'`（裸 token） | **0 行** | 同上，整串等于查「整段文本是否恰好等于城商行」 |
| `MATCH '"城商行*"'`（前缀） | **0 行** | 前缀仅匹配 token 开头，中间子串不命中 |
| `MATCH '"城 商 行"'`（按字短语） | **0 行** | 未切分文本没有按字 token 序列 |

**关键推论**：unicode61 路线必须配合应用层按字切分；「unicode61 自动按字切中文」是常见误传，实测不成立。

### 2.2 应用层按字切分 + phrase 查询：与 LIKE 语义精确等价

切分实现：遍历 rune，每个 CJK 字符（U+3400–9FFF、F900–FAFF、U+20000–2FA1F）后插入一个空格；ASCII 词保持原样（unicode61 天然按空格/标点切词，英文单词与大小写折叠不受影响）。查询侧用同一函数切分后以 phrase 包裹：`获客` → `MATCH '"获 客"'`。

10 万行实测语义对照：

| 对照组 | 方案 A 命中 | LIKE 命中 | 等价性 |
|---|---|---|---|
| 城商行（正文列） | 2001 | 2001 | 完全一致 |
| 获客（title+body） | 2376（全字段口径，多出 5 条来自 fname 列） | 2371 | 口径对齐后一致 |

**即：按字切分 + phrase = 精确子串匹配语义，且不依赖任何分词词典**（「城商行」这类领域词不会因词典缺失被切碎或漏切）。

### 2.3 trigram：2 字词是硬伤

10 万行实测：

| 查询 | 命中 | 耗时 p95 | 说明 |
|---|---|---|---|
| `MATCH '获客'`（2 字） | **0 行，无报错** | — | trigram 最小 3 字符，<3 字查询词静默返回空——**漏检风险最高的形态**（不报错，用户只是看不到结果） |
| `MATCH '城商行'`（3 字） | 2001 | **2.0ms** | trigram 索引命中最快 |
| 列级 `LIKE '%城商行%'`（3 字，走 L2 索引策略） | 2001 | 46.4ms | trigram 表的 LIKE 优化走索引 |
| 列级 `LIKE '%获客%'`（2 字，回退全扫） | 306（body 列） | **632.9ms** | 结果正确但退化 |
| 列级 `LIKE '%获客%'`（title+body） | 2371 | **1009.8ms** | **超出 1s 预算** |
| 表名隐藏列 `tg LIKE '%获客%'` / `'%城商行%'` | **0 行** | — | **坑**：FTS5 表名作隐藏列只支持 MATCH 语义，对 LIKE 一律空结果；trigram 表上必须写列级 LIKE |

## 3. 三方案对比（10 万行实测数据）

| 维度 | A：FTS5 unicode61 + 按字切分 | B：FTS5 trigram | C：裸 LIKE 全表扫 |
|---|---|---|---|
| 2 字词（获客） | **命中，p95 24.9ms** | 不命中（MATCH 恒空）/ LIKE 回退 p95 0.63–1.01s | 命中，p95 581.0ms（title+body） |
| 3 字词（城商行） | 命中，p95 11.6ms | 命中，p95 2.0ms | 命中，p95 557.8ms |
| AND/OR/NOT 组合 | 原生支持（p95 22.7ms） | 同左 | 手写 SQL 拼接，语义易错 |
| 限定字段查询 | 原生支持（p95 22.8ms） | 同左 | 需逐列 LIKE 累加耗时 |
| bm25 相关性加权 | **原生**（`bm25(fts, 4.0, 3.0, 1.0)`，Top20 p95 21.9ms） | 同左（Top20 p95 7.0ms） | 无 |
| snippet()/highlight() 高亮 | 原生（输出含切分空格，展示前去除 CJK 间单空格即可） | 原生（无空格问题） | 无 |
| 索引构建（10 万行） | 13.6s（含正文表双写） | **84.5s（慢 6 倍）** | 无索引 |
| 存储占用（10 万行库文件） | 267.2 MB | 296.9 MB | ~25 MB（仅正文） |
| 依赖成本 | 纯 Go 驱动一个（modernc.org/sqlite），切分函数约 15 行 | 同左 | 同左 |
| 上限外推 | 100 万行 phrase 查询线性外推约 200–250ms，仍达标 | 100 万行 2 字词回退不可用 | 100 万行全扫 5s+，不可用 |
| 主要风险 | snippet 输出带空格需后处理；索引约 10 倍于正文 | **2 字词静默漏检**；构建慢 | P95 踩线；无排序无高亮；100 万行不可扩展 |

## 4. 纯 Go 中文分词库现状（预分词路线的前提）

| 库 | 纯 Go | 维护状态 | Star | 结论 |
|---|---|---|---|---|
| yanyiwu/gojieba | **否（CGO：jieba.cpp/jieba.h + 内嵌 cppjieba）** | 活跃（pushed 2026-07） | 2646 | **违反禁 CGO 约束，排除** |
| wangbin/jiebago | 是 | **停更（最后 push 2020-01）** | 505 | 停更 6 年+，排除 |
| go-ego/gse | **是（无任何 .c/.h 文件，支持词典 go:embed）** | 活跃（pushed 2026-09-12） | 2844 | 预分词路线唯一现实选项 |

**预分词路线（gse + FTS5 unicode61）不推荐作为 MVP 方案的理由**：

1. **词典覆盖风险与本方案核心诉求相反**：素材领域词（城商行、获客、代发工资等）若不在词典中会被切碎，漏检形态与 trigram 2 字词同样隐蔽；按字切分方案对任意中文串的子串命中是确定性的，零词典依赖。
2. 启动加载词典数十 MB 级内存与初始化耗时，Wails 桌面常驻进程需评估。
3. 对「关键词命中单页幻灯片」场景，词级 vs 字级 phrase 的排序质量差异可先用 bm25 字段加权弥补，收益不成比例。

## 5. 推荐结论与落地要点

### 5.1 选型：方案 A（FTS5 + unicode61 + 应用层按字切分）

理由（按权重排序）：

1. **唯一同时满足「2 字词必命中」与「P95 << 1s」的方案**（获客类 2 字关键词是素材检索高频形态）。
2. **零 CGO、零词典**：一个纯 Go 依赖 + 15 行切分函数，交叉编译实测通过，与 Wails Windows/Linux 构建链完全兼容。
3. **语义确定性**：phrase 查询 = 精确子串匹配（实测与 LIKE 逐行等价），领域词不受词典影响。
4. **功能上限高**：bm25 四字段加权（文件名/标题/正文/演讲者备注）、snippet 高亮、布尔组合、前缀查询全部原生可用；未来可叠加 trigram 表优化 ≥3 字词（p95 2ms），两条路线共享同一套驱动与同步代码。

### 5.2 表结构建议

```sql
-- 主表：原始数据（未切分文本，供 UI 展示与导出）
CREATE TABLE ppt_pages(
  id INTEGER PRIMARY KEY,
  deck_id INTEGER NOT NULL,
  page_no INTEGER NOT NULL,
  file_name TEXT, title TEXT, body TEXT, notes TEXT
);
-- FTS 表：只存切分后文本，rowid 与主表 id 对齐（standalone 模式，MVP 推荐）
CREATE VIRTUAL TABLE ppt_pages_fts USING fts5(
  fname_t, title_t, body_t, notes_t
);
```

三种 FTS 表模式取舍：

| 模式 | 适用 | 取舍 |
|---|---|---|
| **standalone（独立 FTS 表，rowid 对齐主表）** | **MVP 推荐**：导入是批量事务场景，双写即可同步，无需触发器 | 切分文本存两份概念（FTS 内部一份），语义最简单可控 |
| external content（`content='ppt_pages', content_rowid='id'`） | 后续数据量涨、想省掉切分文本副本 | 索引省空间、snippet 从主表读原文；但内容列必须与索引 tokenization 一致（需主表另存 `*_t` 切分列），删除/更新必须配触发器或整表 rebuild（`INSERT INTO fts(fts) VALUES('rebuild')`），漏同步即静默漏检 |
| contentless（`content=''`） | 不推荐 | 不存原文，snippet() 不可用；3.43+ 的 `contentless_delete=1` 仍限制多，收益不足以抵复杂度 |

### 5.3 关键实现细节（坑清单）

| # | 要点 |
|---|---|
| 1 | 切分函数与查询构造必须走**同一个函数**（写入与查询 tokenization 不一致 = 静默漏检）；建议放 `util` 并配单测 |
| 2 | ASCII 词不切分（按字符类别判断：仅 CJK 表意文字区间后插空格），保证「AB产品」「Q3」等英文数字词整体可查、大小写折叠交给 unicode61 |
| 3 | phrase 查询依赖 token 位置信息，**必须保持默认 `detail=full`**，不能用 `detail=none/1` 省索引 |
| 4 | FTS 表上写列级条件，禁用表名隐藏列做 LIKE（实测对任何 pattern 返回 0 行，见 2.3） |
| 5 | snippet() 输出含切分空格（如 `[城 商 行]`），前端展示前用切分函数的逆操作合并 CJK 间单空格 |
| 6 | bm25 列权重顺序 = FTS 表列定义顺序，值越大权重越高，`ORDER BY bm25(...)` 升序即最优在前；建议文件名 4.0 / 标题 3.0 / 备注 2.0 / 正文 1.0（可调） |
| 7 | 索引同步走导入事务双写（与主表同事务 commit）；批量重建用 `INSERT INTO ppt_pages_fts(ppt_pages_fts) VALUES('rebuild')` |
| 8 | 驱动层：单写连接 + `journal_mode(WAL)` + `busy_timeout`；DB 文件放 data/ 目录，遵守「手改 data 文件前关闭 workbench.exe」项目规则 |
| 9 | 升级 modernc.org/sqlite 时同步对齐 `modernc.org/libc` pin 版本（上游硬约束） |
| 10 | 2 字词 phrase（获客）doclist 较大，p95 21–25ms 主要耗在此；若未来达 100 万行再叠加 trigram 表承接 ≥3 字词 |

## 6. 参考链接

- modernc.org/sqlite（含 CHANGELOG 与 Performance 章节）: https://pkg.go.dev/modernc.org/sqlite
- SQLite FTS5 官方文档（unicode61/trigram/external content/bm25）: https://www.sqlite.org/fts5.html （trigram 3.34.0 引入；external content 触发器与 rebuild 语义）
- trigram 分词器源码（case_sensitive/LIKE 优化策略，GitHub 镜像）: https://github.com/sqlite/sqlite/blob/master/ext/fts5/fts5_tokenize.c
- go-ego/gse（纯 Go 分词，活跃）: https://github.com/go-ego/gse
- yanyiwu/gojieba（CGO，活跃）: https://github.com/yanyiwu/gojieba
- wangbin/jiebago（纯 Go，停更 2020）: https://github.com/wangbin/jiebago

## 附：实测环境与原始数据

- 基准程序：临时 Go 模块（已清理），数据源 `rand.New(rand.NewSource(42))` 确定性生成，10 万行正文（每行 120–300 汉字）+ 2% 植入「城商行」（正文）/「获客」（标题）；文件名/标题/正文三列。
- 单连接 `SetMaxOpenConns(1)`，`journal_mode(OFF)+synchronous(OFF)`（构建计时口径），30 次采样取 p95。
- 方案 A 关键原始数据：phrase 城商行 p95 11.6ms / phrase 获客 p95 24.9ms / AND 组合 p95 22.7ms / bm25 Top20 p95 21.9ms / 构建 13.6s / 库文件 267.2MB。
- 方案 B 关键原始数据：MATCH 城商行 p95 2.0ms / bm25 Top20 p95 7.0ms / 列级 LIKE 获客 p95 632.9ms（body）、1009.8ms（title+body）/ 构建 84.5s / 库文件 296.9MB / 表名隐藏列 LIKE 恒 0 行。
- 方案 C 关键原始数据：LIKE 城商行 p95 557.8ms / LIKE 获客（title+body）p95 581.0ms。
- 交叉编译：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64` 与 `CGO_ENABLED=0 GOOS=windows GOARCH=arm64` 均 build 通过。
