package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
		cmd := exec.Command("cmd", "/c", "start", "", path)
		util.HideCommandWindow(cmd)
		return cmd.Start()
	}
	cmd := exec.Command("cmd", "/c", "start", "", "/select,", path)
	util.HideCommandWindow(cmd)
	return cmd.Start()
}

// OpenInVSCode 用 VSCode 打开文件或文件夹
func (s *FileOperationService) OpenInVSCode(path string) error {
	cmd := exec.Command("code", path)
	util.HideCommandWindow(cmd)
	return cmd.Start()
}

// OpenInWarp 用 Warp 终端打开
func (s *FileOperationService) OpenInWarp(path string) error {
	url := "file:///" + filepath.ToSlash(path)
	cmd := exec.Command("warp", url)
	util.HideCommandWindow(cmd)
	return cmd.Start()
}

// OpenWithDefaultApp 用系统默认程序打开文件
func (s *FileOperationService) OpenWithDefaultApp(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("不支持打开文件夹")
	}
	cmd := exec.Command("cmd", "/c", "start", "", path)
	util.HideCommandWindow(cmd)
	return cmd.Start()
}

// findAvailableName 查找可用路径，冲突时自动追加 (1), (2)...
func findAvailableName(targetPath string) string {
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return targetPath
	}

	ext := filepath.Ext(targetPath)
	nameWithoutExt := strings.TrimSuffix(filepath.Base(targetPath), ext)
	dir := filepath.Dir(targetPath)

	for i := 1; i < 1000; i++ {
		newName := fmt.Sprintf("%s(%d)%s", nameWithoutExt, i, ext)
		newPath := filepath.Join(dir, newName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}

	return targetPath
}

// CopyItem 复制文件或目录到目标文件夹，同名自动重命名
func (s *FileOperationService) CopyItem(sourcePath, targetDir string) (string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(targetDir, filepath.Base(sourcePath))
	targetPath = findAvailableName(targetPath)

	if info.IsDir() {
		return targetPath, util.CopyDir(sourcePath, targetPath)
	}
	return targetPath, util.CopyFile(sourcePath, targetPath)
}

// MoveItem 移动文件或目录到目标文件夹，同名自动重命名
func (s *FileOperationService) MoveItem(sourcePath, targetDir string) (string, error) {
	sourceDir := filepath.Dir(sourcePath)
	if sourceDir == targetDir {
		return "", fmt.Errorf("源路径与目标路径相同")
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(targetDir, filepath.Base(sourcePath))
	targetPath = findAvailableName(targetPath)

	err = os.Rename(sourcePath, targetPath)
	if err == nil {
		return targetPath, nil
	}

	// 跨盘移动降级为复制+删除
	if info.IsDir() {
		err = util.CopyDir(sourcePath, targetPath)
	} else {
		err = util.CopyFile(sourcePath, targetPath)
	}
	if err != nil {
		return "", err
	}
	return targetPath, os.RemoveAll(sourcePath)
}
