package service

import "workbench/util"

// CopyToSystemClipboard 写入系统剪贴板（复制模式）
func (s *FileOperationService) CopyToSystemClipboard(paths []string) error {
	return util.WriteClipboardFiles(paths, false)
}

// CutToSystemClipboard 写入系统剪贴板（剪切模式）
func (s *FileOperationService) CutToSystemClipboard(paths []string) error {
	return util.WriteClipboardFiles(paths, true)
}

// ReadFromSystemClipboard 读取系统剪贴板文件列表
func (s *FileOperationService) ReadFromSystemClipboard() ([]string, bool, error) {
	return util.ReadClipboardFiles()
}
