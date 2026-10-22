package model

// 分支对比域数据结构（分支对比面板 batch 1，纯只读）。
//
// 约定：字段一律用普通 string + 常量表表达枚举语义，不用具名 string 类型——
// 规避 Wails 不为函数签名中的具名 string 生成 models.ts 别名导致
// npm run build MISSING_EXPORT（见 docs/spec/cross-layer-contracts.md）。
// json tag 用 camelCase；可选字段（Body/PatchID/ParentHashes/OldPath）带 omitempty。

// BranchCompareCommit 分支对比中的单个提交（对齐表行的一侧）。
type BranchCompareCommit struct {
	Hash         string   `json:"hash"`                   // 完整 40 位 SHA
	ShortHash    string   `json:"shortHash"`              // 前 8 位 SHA，用于显示
	Author       string   `json:"author"`                 // 作者名称
	Email        string   `json:"email"`                  // 作者邮箱
	Date         int64    `json:"date"`                   // 作者时间 Unix 秒（%ct committer 时间）
	Subject      string   `json:"subject"`                // 提交标题（%s 单行）
	Body         string   `json:"body,omitempty"`         // 提交正文（%b，可为空）
	ParentHashes []string `json:"parentHashes,omitempty"` // 父提交 SHA 列表（root 提交为空）
	PatchID      string   `json:"patchId,omitempty"`      // git patch-id --stable 计算值；管道失败降级时为空串
	BranchName   string   `json:"branchName"`             // 所属对比侧分支名（branchA / branchB 原样）
}

// BranchCompareRow 对齐表的一行：配对提交同行显示，单侧独有提交只占一列。
type BranchCompareRow struct {
	Key       string               `json:"key"`       // 行唯一键：配对行 <hashA>..<hashB>，独有行即该侧提交 SHA
	A         *BranchCompareCommit `json:"a"`         // A 侧提交；b-only 行为 nil
	B         *BranchCompareCommit `json:"b"`         // B 侧提交；a-only 行为 nil
	Side      string               `json:"side"`      // 行归属，取值见 RowSide 系列常量
	MatchedBy string               `json:"matchedBy"` // 配对方式，取值见 RowMatchedBy 系列常量
}

// RowSide 行归属常量（Side 字段取值）。
const (
	RowSideMatched = "matched" // 双侧配对成功
	RowSideAOnly   = "a-only"  // 仅 A 侧独有
	RowSideBOnly   = "b-only"  // 仅 B 侧独有
)

// RowMatchedBy 配对方式常量（MatchedBy 字段取值），优先级 hash > patch-id > subject。
const (
	RowMatchedByHash    = "hash"     // 完整 SHA 精确相等
	RowMatchedByPatchID = "patch-id" // patch-id 相等（疑似 cherry-pick 等价提交）
	RowMatchedBySubject = "subject"  // 归一化标题相等（剥离 cherry-pick 后缀 + 忽略大小写）
	RowMatchedByNone    = "none"     // 未配对（a-only / b-only 行）
)

// BranchCompareStats 对比统计条数据（R4）。
type BranchCompareStats struct {
	AOnly        int `json:"aOnly"`        // A 独有提交数
	BOnly        int `json:"bOnly"`        // B 独有提交数
	Matched      int `json:"matched"`      // 配对成功提交数（含三种匹配方式）
	PatchMatched int `json:"patchMatched"` // 其中 patch-id 匹配数（疑似拣选数）
	CommonBelow  int `json:"commonBelow"`  // merge-base 之下共同历史提交数
}

// BranchCompareAlignment 分支对齐结果（GetCommitAlignment 返回值）。
type BranchCompareAlignment struct {
	BranchA            string             `json:"branchA"`            // 对比侧 A 分支名
	BranchB            string             `json:"branchB"`            // 对比侧 B 分支名
	MergeBaseHash      string             `json:"mergeBaseHash"`      // 分叉点提交 SHA；无共同祖先（unrelated histories）时为空串
	CommonHistoryCount int                `json:"commonHistoryCount"` // merge-base 之下共同历史提交数；merge-base 缺失时为 0
	Rows               []BranchCompareRow `json:"rows"`               // 对齐表行，按 max(a.Date, b.Date) 降序
	Stats              BranchCompareStats `json:"stats"`              // 统计条
	Truncated          bool               `json:"truncated"`          // 任一侧触及 --max-count=2000 上限被截断（前端明示，不静默）
}

// BranchDiffFile 分支差异文件树行（R5）：两侧 ls-tree 并集 + diff 状态着色。
type BranchDiffFile struct {
	Path      string `json:"path"`              // 文件相对仓库根路径（renamed 行为新路径）
	OldPath   string `json:"oldPath,omitempty"` // renamed 时的旧路径，其余状态为空
	Status    string `json:"status"`            // 变更状态，取值见 BranchDiffStatus 系列常量
	Additions int    `json:"additions"`         // 新增行数（numstat；二进制为 0）
	Deletions int    `json:"deletions"`         // 删除行数（numstat；二进制为 0）
	Binary    bool   `json:"binary"`            // 二进制文件（numstat 行数位为 "-"）
}

// BranchDiffStatus 文件状态常量（BranchDiffFile.Status 取值，对应 git diff --name-status 字母）。
const (
	BranchDiffStatusAdded      = "added"      // A：新增
	BranchDiffStatusDeleted    = "deleted"    // D：删除
	BranchDiffStatusRenamed    = "renamed"    // R：重命名
	BranchDiffStatusCopied     = "copied"     // C：复制
	BranchDiffStatusTypechange = "typechange" // T：类型变更（文件↔符号链接等）
	BranchDiffStatusModified   = "modified"   // M：内容修改
	BranchDiffStatusUnchanged  = "unchanged"  // 两侧 ls-tree 并集中不在 diff 集的路径（两侧存在且内容一致）
)
