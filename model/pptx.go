// model/pptx.go
package model

// PptxSplitResult PPT 按页拆分结果（工具箱·PPT 拆分）。
//
// 每页一个独立 .pptx，输出到源文件同级同名文件夹；错误（含取消）时返回零值
// 结果，PageCount = len(Files)。
type PptxSplitResult struct {
	OutputDir string   `json:"outputDir"` // 输出文件夹绝对路径（源文件同级、以源文件名命名）
	Files     []string `json:"files"`     // 生成的单页文件绝对路径列表，按页序升序
	PageCount int      `json:"pageCount"` // 拆出的页数（= len(Files)）
}

// PptxBatchItem 文件夹批量拆分中单个文件的结果（工具箱·PPT 拆分）。
//
// Error 非空即该文件失败（用户可读原因），此时 OutputDir/PageCount 为零值；
// 成功时 Error 为空。
type PptxBatchItem struct {
	SrcPath   string `json:"srcPath"`   // 源文件绝对路径
	OutputDir string `json:"outputDir"` // 输出文件夹绝对路径（失败时为空）
	PageCount int    `json:"pageCount"` // 拆出的页数（失败时为 0）
	Error     string `json:"error"`     // 失败原因（用户可读消息），成功为空
}

// PptxBatchResult 文件夹批量拆分汇总结果（工具箱·PPT 拆分）。
//
// Items 按扫描（字典序）顺序逐文件排列；整批取消时 Items 仅含已完成文件，
// 并伴随取消错误一起返回。
type PptxBatchResult struct {
	DirPath string          `json:"dirPath"` // 扫描根目录绝对路径
	Total   int             `json:"total"`   // 扫描到的待拆分文件总数
	Items   []PptxBatchItem `json:"items"`   // 逐文件结果（按扫描顺序）
}

// PptxSubsetResult PPT 多页子集导出结果（素材购物栏合并导出的单文件子集能力）。
//
// 将源演示文稿中选定的页集合导出为独立 .pptx（保留页集合导出）；错误（含取消）
// 时返回零值结果。Pages 为归一化后的实际导出页号（去重升序），与 PageCount 一致。
type PptxSubsetResult struct {
	OutputPath string `json:"outputPath"` // 输出文件绝对路径
	Pages      []int  `json:"pages"`      // 实际导出的页号（1 起展示页序，去重升序）
	PageCount  int    `json:"pageCount"`  // 导出页数（= len(Pages)）
}

// PptxMergeItem 合并导出的单个页项（素材购物栏条目的后端形态）。
//
// SrcPath 为源文件绝对路径；PageNum 为 1 起展示页序（sldIdLst 三跳映射后的页序，
// 非 slide 文件名序）。Items 按购物栏顺序排列，输出页序与其一致。
type PptxMergeItem struct {
	SrcPath string `json:"srcPath"`
	PageNum int    `json:"pageNum"`
}

// PptxMergedPage 合并产物中的单个页明细（按输出页序排列）。
type PptxMergedPage struct {
	SrcPath    string `json:"srcPath"`    // 来源文件绝对路径
	SrcPageNum int    `json:"srcPageNum"` // 源展示页序（1 起）
	OutPageNum int    `json:"outPageNum"` // 产物页序（1 起，= 数组下标 + 1）
}

// PptxMergeSkipped 合并导出中被跳过的页项及原因。
//
// 单个源文件打不开/损坏或页号越界只跳过该项继续导出（结果中报告）；基底文件
// （清单第一项）打不开为整体失败，不产生本类型。
type PptxMergeSkipped struct {
	SrcPath string `json:"srcPath"`
	PageNum int    `json:"pageNum"`
	Reason  string `json:"reason"`
}

// PptxMergeResult 多源页合并导出结果（素材购物栏「合并导出」）。
//
// Pages 按购物栏顺序排列（产物 sldIdLst 顺序与其一致）；SkippedItems 为跳过项
// 明细；SldSzMixed 表示各源 p:sldSz 尺寸不一致（16:9/4:3 混排不自动缩放，前端
// 据此提示）。错误（含取消）时返回零值结果。
type PptxMergeResult struct {
	OutputPath   string             `json:"outputPath"`   // 输出文件绝对路径
	Pages        []PptxMergedPage   `json:"pages"`        // 页明细（购物栏顺序）
	PageCount    int                `json:"pageCount"`    // 合并页数（= len(Pages)）
	SkippedItems []PptxMergeSkipped `json:"skippedItems"` // 跳过项（源缺失/页越界/重复项）
	SldSzMixed   bool               `json:"sldSzMixed"`   // 各源幻灯片尺寸不一致
}
