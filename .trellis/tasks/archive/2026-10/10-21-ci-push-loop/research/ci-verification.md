# CI 验证记录（镜像仓 hello007/workbench）

> 采集时间：2026-10-21。GitHub API 公开端点（runs/jobs/steps 结论公开；日志正文需认证未采集）。

## run#13（82dfe27，push 后首跑）

- 整体：completed success（7 分钟）
- test job：**全绿**，关键首跑项：
  - 「前端依赖分档一致性检查」（ci.yml 步骤 2.5）线上首跑 ✅
  - 「后端单元测试」（goleak 硬门禁 Linux 载体）转绿 ✅——run 11/12 旧红点收口
  - E2E 测试 / 前端单元测试 / 后端集成测试 ✅
- security job：**failure**（「Go 漏洞扫描」govulncheck 红，后续 npm 扫描 skip）→ 触发本轮修复

## 修复 commit 57f17d6

- go.mod toolchain go1.26.6 → go1.26.10（11 个标准库可达漏洞唯一修法）
- golang.org/x/net v0.59.0 → v0.60.0（4 漏洞）
- govulncheck 本地复跑 exit 0（可达 11→0 清零；残留 GO-2026-5932 Module 级停维护，接受风险登记 docs/spec/security-scan.md §5.3）
- Windows + WSL Linux 双侧 go test 全量回归绿

## run#14（57f17d6，修复验证跑）

- 整体：completed success（4 分钟）
- test job：success
- security job：failure——「Go 漏洞扫描」转绿 ✅，但「前端漏洞扫描」**首跑暴露新红**（run#13 时因 Go 扫描红被 skip，两扫描串行分层暴露）
- 本地复现：npm audit 6 漏洞（3 high：markdown-it DoS ×2、vite 传递 launch-editor ×3、brace-expansion；3 low：KaTeX/mermaid 链路）

## run#15（5cd6b65，npm audit fix 验证跑）

- 整体：completed success（5 分钟）
- test job：success
- security job：**success**——「Go 漏洞扫描」+「前端漏洞扫描」两步实际执行且均过 ✅
- 修复内容：package-lock.json 单文件（markdown-it 14.3.3、vite 8.3.4、brace-expansion 5.0.12、dompurify 3.4.17 + rolldown 家族随动；package.json 声明零改动）
- 残留：KaTeX 链路 2 low（归 10-21-mermaid-12 专项，audit-level=high 门禁下不阻塞）

## run#16（12e7299，规程文档 commit 验证跑）

- 整体：completed success（4 分钟），test + security 双 job 绿

## 终态（2026-10-21）

- 本地 master = gitee origin = GitHub 镜像仓 HEAD = `12e7299`
- 远端 CI run#15（功能收口跑）与 run#16（终态跑）均全 job 绿
- 规程沉淀：docs/贡献指南.md 5.3 节（同步机制/触发时机/验证方法/红点处置/责任口）
