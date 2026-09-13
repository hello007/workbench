package model

// ExternalDiffRequest 描述一次外部 diff 打开的版本来源。
// 仅在后端内部流转（App 层构造、GitService 消费），不出现在 Wails 暴露方法签名中，
// 前端经 OpenInExternalDiff 平铺 string 参数传递。
type ExternalDiffRequest struct {
	// Mode diff 来源模式：workspace（工作区单文件）/ commit（提交中单文件）/ range（两提交间单文件）
	Mode string
	// File 仓库内相对路径（workspace/commit/range 均必填）
	File string
	// SHA commit 模式的提交 SHA
	SHA string
	// BaseSHA range 模式的基准提交 SHA（旧版本侧）
	BaseSHA string
	// HeadSHA range 模式的目标提交 SHA（新版本侧）
	HeadSHA string
}
