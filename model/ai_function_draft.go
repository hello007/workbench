package model

// AiFunctionDraftRequest AI 功能自然语言生成请求。
// 前端填写 Description/Cwd/SkillCommand；App.GenerateAiFunctionDraft 入口在调用
// service 前补全 Skills（候选 skill 清单）与 ExistingIDs（已有功能 ID 清单）。
type AiFunctionDraftRequest struct {
	Description  string             `json:"description"`           // 自然语言需求描述（必填）
	Cwd          string             `json:"cwd"`                   // 工作目录（选填，用于 skill 发现与生成上下文）
	SkillCommand string             `json:"skillCommand"`          // 用户指定的 skill 命令（选填，含命名空间）
	Skills       []*SkillDescriptor `json:"skills,omitempty"`      // 候选 skill 清单（App 层补全：全局已发现 ∪ 指定目录）
	ExistingIDs  []string           `json:"existingIds,omitempty"` // 已有功能 ID 清单（App 层补全，生成时避让重名）
}

// AiFunctionDraftResult AI 功能生成结果（草稿不落盘）。
type AiFunctionDraftResult struct {
	Draft      *AiFunction `json:"draft"`                // 生成的配置草稿，前端预览后可填入配置表单
	Warnings   []string    `json:"warnings"`             // 可用性警告（不阻断，如 skill 未命中、ID 重命名）
	Renamed    bool        `json:"renamed"`              // ID 是否因与已有项冲突被自动重命名
	OriginalID string      `json:"originalId,omitempty"` // 重命名前的原 ID（Renamed=true 时有值）
}
