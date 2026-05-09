package service

import (
	"os"
	"os/exec"
	"path/filepath"

	"git-manager/model"
	"git-manager/util"
)

// FileOperationService 文件操作服务
type FileOperationService struct{}

// NewFileOperationService 创建服务
func NewFileOperationService() *FileOperationService {
	return &FileOperationService{}
}

// CreateDirectory 创建文件夹
func (s *FileOperationService) CreateDirectory(parentPath, name string) error {
	fullPath := filepath.Join(parentPath, name)

	if _, err := os.Stat(fullPath); err == nil {
		return os.ErrExist
	}

	return util.CreateDirectory(fullPath)
}

// CreateFile 创建文件
func (s *FileOperationService) CreateFile(parentPath, name, content string) error {
	fullPath := filepath.Join(parentPath, name)

	if _, err := os.Stat(fullPath); err == nil {
		return os.ErrExist
	}

	return util.CreateFile(fullPath, content)
}

// Rename 重命名
func (s *FileOperationService) Rename(oldPath, newName string) error {
	dir := filepath.Dir(oldPath)
	newPath := filepath.Join(dir, newName)

	if _, err := os.Stat(newPath); err == nil {
		return os.ErrExist
	}

	return util.RenamePath(oldPath, newPath)
}

// Delete 删除
func (s *FileOperationService) Delete(path string) error {
	return util.RemovePath(path)
}

// PreviewFile 预览文件
func (s *FileOperationService) PreviewFile(filePath string, maxSize int64) (*model.FilePreview, error) {
	preview := &model.FilePreview{
		Path: filePath,
		Name: filepath.Base(filePath),
	}

	info, err := os.Stat(filePath)
	if err != nil {
		preview.Error = err.Error()
		return preview, err
	}

	preview.Size = info.Size()

	if preview.Size > maxSize {
		preview.TooLarge = true
		return preview, nil
	}

	if !util.IsPreviewable(filePath) {
		data, _ := util.ReadFileSafe(filePath, 1024)
		for _, b := range data {
			if b == 0 {
				preview.IsBinary = true
				return preview, nil
			}
		}
	}

	data, err := util.ReadFileSafe(filePath, maxSize)
	if err != nil {
		preview.Error = err.Error()
		return preview, err
	}

	preview.Content = string(data)
	return preview, nil
}

// OpenInExplorer 在资源管理器中打开
func (s *FileOperationService) OpenInExplorer(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		cmd := exec.Command("explorer", path)
		util.HideCommandWindow(cmd)
		return cmd.Start()
	}
	cmd := exec.Command("explorer", "/select,", path)
	util.HideCommandWindow(cmd)
	return cmd.Start()
}

// OpenInVSCode 用 VSCode 打开文件或文件夹
func (s *FileOperationService) OpenInVSCode(path string) error {
	cmd := exec.Command("code", path)
	util.HideCommandWindow(cmd)
	return cmd.Start()
}
