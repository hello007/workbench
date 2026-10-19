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
