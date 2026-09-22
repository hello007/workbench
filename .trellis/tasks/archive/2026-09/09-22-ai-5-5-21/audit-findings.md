# 审核发现清单（3 路并行审核，2026-09-22）

## 后端（2🔴 2🟠 8🟡）

| # | 位置 | 级别 | 问题 | 修复方向 |
|---|---|---|---|---|
| B1 | service/chat_service.go:787 | 🔴 | task.proc.Kill() 无 nil 防护：queued=false(L706) 到 proc 赋值(L751) 窗口内取消 → nil 接口调用 panic | `if task.proc != nil` 守卫 |
| B2 | service/chat_service.go:778 | 🔴 | close(task.queueCancel) double-close：两次快速 CancelChatTask → panic | 临界区内检查 task.canceled 已 true 直接返回，或 sync.Once |
| B3 | service/chat_service.go:706 | 🟠 | 串行保护绕过：queued=false 与 running=true 之间（含 spawn 全程）两标志均 false，守卫放行同会话第二轮 → 两 claude 并行同会话 | queued=false 同临界区提前置 running=true，Start 失败复位 |
| B4 | service/chat_service.go:610 | 🟠 | 持久化非原子：util.SaveJSON os.WriteFile 截断写，崩溃丢会话历史/索引（.bak 存的是已截断内容） | temp+os.Rename 原子替换，覆写前备份上一好版本 |
| B5 | service/chat_service.go:660 | 🟡 | taskID=UnixNano 无唯一性校验，同 tick 并发覆盖 map | 原子递增序号后缀或插入冲突重试 |
| B6 | service/chat_service.go:713 | 🟡 | WithTimeout cancel 正常完成/手动取消路径不调，定时器泄漏至超时 | pumpChatOutput Wait 后调 task.cancel() |
| B7 | service/chat_service.go:881 | 🟡 | 超时路径只杀根进程（CommandContext 默认 Kill），claude 子进程成孤儿；手动取消却整树杀 | 超时分支走 killProcessTree（设 cmd.Cancel 或 DeadlineExceeded 分支补整树杀） |
| B8 | service/chat_service.go:731 | 🟡 | Start/StdoutPipe 失败任务永久残留 s.tasks；已完成任务从不删除 → 内存无界增长 | 失败路径 delete；终态任务 pumpChatOutput emit done 后 delete（前端 restore 查无此任务分支已能处理终态） |
| B9 | service/chat_service.go:841 | 🟡 | scanner.Err() 忽略：超 4MB 单行或读错误静默终止，回复截断且 done 无 Error | Scan 退出后查 scanner.Err()，非 EOF 写 result.Error |
| B10 | service/chat_service.go:859 | 🟡 | result 事件 is_error/subtype/result 错误文本被丢弃：claude 侧失败用户只看到 exit status 1 或无提示 | is_error/result 字段透传进 result.Error（parseStreamLine 同包复用，需小扩展不破坏 ai_function 用法） |
| B11 | service/chat_config.go:227 | 🟡 | GetChatSettings/SaveChatSettings 不持 s.mu，与其余读写不一致，并发撕裂读静默回退默认 | 两方法加 s.mu |
| B12 | service/chat_service_test.go:798 | 🟡 | TestChatService_GetChatTaskState_Missing 注释宣称「排队不侵蚀超时预算」但函数体只测 nil/false | 删注释或补真实用例（补用例） |
| B13 | service/chat_service.go:444 | 🟡 | AddChatDirectory 去重键大小写敏感，前端 containsPath 已小写化——同路径不同大小写产生重复项 | 后端去重键 strings.ToLower(filepath.ToSlash(absPath)) |
| B14 | docs/API参考.md:339 | 🟡 | 事件 payload 描述不实：仅 done 是 ChatTaskRunResult 形状；queued={taskId}、started={taskId,chatSessionId}、output={taskId,chatSessionId,text} | 按事件逐一标注 |
| B15 | docs/功能说明.md:206 | 🟡 | 「复用 AI 功能全局并发槽位」不实——chat 独立池 chatMaxConcurrent=3 | 改「独立并发槽位（与 AI 功能互不挤占）」 |

## 前端（3🔴 3🟠 10🟡）

| # | 位置 | 级别 | 问题 | 修复方向 |
|---|---|---|---|---|
| F1 | frontend/src/components/AiChatPanel.vue:630 | 🔴 | onInputEnter 未查 isComposing：中文 IME 选词回车把拼音串发给 claude | `if (e.isComposing \|\| e.keyCode === 229) return` |
| F2 | frontend/src/store/aiChat.js:244 | 🔴 | 乐观态时序错：RunChat 进程启动成功才返回 taskId，queued/started 事件先于 resolve 到达被丢弃 → status 永停 queued；cancelChatTask 误判运行中为排队态清 chatTask → done(canceled) 对号失败、回复丢失 | RunChat resolve 即置 status:'running'（resolve 即已启动）；queued 态不再作为本地乐观态 |
| F3 | frontend/src/components/AiChatPanel.vue:167 | 🔴 | .chat-msg-md v-html 链接无点击拦截：linkify 裸 <a> 点击即 webview 导航走整个应用（FilePreviewRenderer.vue:906 称「崩溃根因」有现成方案） | 容器 click capture：closest('a') → preventDefault + BrowserOpenURL |
| F4 | frontend/src/store/aiChat.js:138 | 🟠 | loadChatSessions await 后不校验目录已切换：快速连点 A→B，A 迟到响应覆盖列表并自动选中 A 会话 | await 后 `if (selectedChatDirectoryId.value !== dirId) return` |
| F5 | frontend/src/components/AiChatPanel.vue:394 | 🟠 | 拖拽排序失败无回滚：localDirs 永久停留未持久化顺序 | catch 内 `localDirs.value = [...aiChatStore.chatDirectories]` |
| F6 | frontend/src/components/AiChatPanel.vue:579 | 🟠 | handleDeleteSession 无 confirmDiscardInFlight：删在途会话进程继续跑，完成后复活孤儿消息文件 | 删除前有在途任务先确认并取消 |
| F7 | AiChatPanel.vue:412 | 🟡 | 确认弹窗承诺「取消任务并切换」但 cancelChatTask 失败仍切换 | 取消失败中止切换（return false）或提示任务仍在运行 |
| F8 | AiChatPanel.vue:437 | 🟡 | 移除确认文案称「会话与消息记录不受影响」但重加同路径生成新 id，旧会话永久不可达 | 文案如实说明（移除目录后历史会话暂不可达） |
| F9 | AiChatPanel.vue:621 | 🟡 | 流式每增量强制 scrollTop=scrollHeight，上翻被拽回 | 距底超阈值暂停自动跟随 |
| F10 | AiChatPanel.vue:647 | 🟡 | 无会话首发路径 chatInFlight 检查与置乐观态之间隔 await createChatSession，双击双会话双发 | 本地 sending 标志或 chatInFlight 复查 |
| F11 | AiChatPanel.vue:792 | 🟡 | onMounted 先 await 加载后注册监听，await 期间卸载则 off 闭包 null 泄漏 | 先注册后加载，或卸载标志守卫 |
| F12 | aiChat.js:173 | 🟡 | selectChatSession 快速连切竞态：迟到响应覆盖新选中会话消息 | await 后比对 selectedChatSessionId |
| F13 | aiChat.js:258 | 🟡 | slice(0,20) UTF-16 码元截断，emoji 拦腰截断非法半代理 | Array.from(prompt) 后截取 |
| F14 | AiChatPanel.spec.js:297 | 🟡 | 大量用例 wrapper.vm.$.setupState 直捅内部状态，实现级断言 | 高价值用例改 DOM 触发（点击/键盘），余下保留可接受 |
| F15 | frontend/e2e/ai-chat.spec.js:54 | 🟡 | mock 事件派发时序与真实相反（resolve 后 setTimeout 派发；真实先 emit 后返回），恰好绕开 F2 缺陷 | mock 改先派发 queued/started 再 resolve；F2 修后补生产时序断言 |

## 跨层（0🔴 0🟠 3🟡）

19 方法签名、models.ts、mock 形状、错误码三处、事件 payload 解构、RPC 暴露、applySink 装配逐项核对一致。仅文档/注释 3 项（已并入 B13/B14/B15）。
