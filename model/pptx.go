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
