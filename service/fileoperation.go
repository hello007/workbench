package service

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"workbench/model"
	"workbench/util"
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
	preview.Kind = detectPreviewKind(filePath)

	// 按 kind 分流：只有 text 需要把全文读成 string（受 maxSize 限制）。
	// image/pdf/office/unsupported 不在此读取内容、不判 tooLarge——
	//   image/office 由前端走 ReadFileBytes（50MB 上限）取 base64；
	//   pdf 走 iframe + AssetServer Range 流式（无大小限制）；
	//   unsupported 由前端降级提示。
	// maxSize（1MB）本意仅针对 text 读全内容，不能用于误伤其他类型。
	if preview.Kind != model.KindText {
		return preview, nil
	}

	// text：超过上限则标记过大，不再读全内容
	if preview.Size > maxSize {
		preview.TooLarge = true
		return preview, nil
	}

	data, err := util.ReadFileSafe(filePath, maxSize)
	if err != nil {
		preview.Error = err.Error()
		return preview, err
	}

	preview.Content = string(data)
	return preview, nil
}

// ReadFileBytes 读取文件原始字节（base64），供前端构造 Blob 预览图片/PDF/Office
func (s *FileOperationService) ReadFileBytes(filePath string, maxSize int64) (*model.FileBytes, error) {
	result := &model.FileBytes{
		Path: filePath,
		Name: filepath.Base(filePath),
		Kind: detectPreviewKind(filePath),
	}

	info, err := os.Stat(filePath)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	result.Size = info.Size()

	if info.Size() > maxSize {
		result.TooLarge = true
		return result, nil
	}

	data, err := util.ReadFileSafe(filePath, maxSize)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	result.Base64 = base64.StdEncoding.EncodeToString(data)
	return result, nil
}

// detectPreviewKind 根据扩展名识别预览类型，供前端选择渲染器
func detectPreviewKind(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".bmp", ".gif", ".webp", ".svg", ".ico", ".tif", ".tiff", ".heic", ".heif", ".avif":
		return model.KindImage
	case ".pdf":
		return model.KindPDF
	case ".doc", ".docx", ".docm", ".dot", ".dotx",
		".ppt", ".pptx", ".pptm", ".pps", ".ppsx",
		".xls", ".xlsx", ".xlsm", ".xlsb", ".csv",
		".odt", ".odp", ".ods", ".rtf":
		return model.KindOffice
	}
	if util.IsPreviewable(filename) {
		return model.KindText
	}
	return model.KindUnsupported
}

// SaveFile 保存文件内容（原子写入：先写临时文件再 rename）
func (s *FileOperationService) SaveFile(filePath string, content string) error {
	// 校验路径存在且为普通文件
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("文件不存在: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("不能保存目录")
	}

	// 大小限制（与预览一致：1MB）
	const maxSize = 1024 * 1024
	if int64(len(content)) > maxSize {
		return fmt.Errorf("内容超过1MB限制")
	}

	// 原子写入：先写临时文件再 rename
	dir := filepath.Dir(filePath)
	tmpFile, err := os.CreateTemp(dir, ".workbench-save-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, err = tmpFile.WriteString(content)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	err = os.Rename(tmpPath, filePath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("替换文件失败: %w", err)
	}

	return nil
}

// OpenInExplorer 在资源管理器中打开
func (s *FileOperationService) OpenInExplorer(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return exec.Command("explorer", path).Start()
	}
	return exec.Command("explorer", "/select,"+path).Start()
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

// resolveObsidianVault 解析 Obsidian vault 目录：文件夹→自身，文件→父目录。
// 纯函数，便于单测；路径不存在时返回错误。
func resolveObsidianVault(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return path, nil
	}
	return filepath.Dir(path), nil
}

// encodeObsidianPath 将绝对路径编码为 obsidian:// URI 的 path 参数值。
// 反斜杠→正斜杠 → QueryEscape → '+' 替换为 '%20'（与官方示例一致，中文/空格安全）。
func encodeObsidianPath(p string) string {
	escaped := url.QueryEscape(filepath.ToSlash(p))
	return strings.ReplaceAll(escaped, "+", "%20")
}

// OpenInObsidian 用 Obsidian 打开指定路径对应的 vault。
// vault 解析：文件夹→自身，文件→父目录。
// 调用策略：obsidianPath 非空且文件存在 → 直接用该 exe 启动（优先）；
// 否则走系统协议方案（注册表预检 + cmd /c start "" obsidian://open?path=...）。
// 未检测到 Obsidian 时返回友好错误，由前端引导用户配置。
func (s *FileOperationService) OpenInObsidian(path, obsidianPath string) error {
	vaultPath, err := resolveObsidianVault(path)
	if err != nil {
		return fmt.Errorf("路径不存在或无法访问: %w", err)
	}
	uri := "obsidian://open?path=" + encodeObsidianPath(vaultPath)

	// 策略一：用户配置了 Obsidian 可执行文件路径且存在，直接用该 exe 启动
	if strings.TrimSpace(obsidianPath) != "" {
		if _, statErr := os.Stat(obsidianPath); statErr == nil {
			cmd := exec.Command(obsidianPath, uri)
			util.HideCommandWindow(cmd)
			return cmd.Start()
		}
	}

	// 策略二：系统协议方案——预检 obsidian:// 是否注册，未注册则提示
	if !isObsidianProtocolRegistered() {
		return fmt.Errorf("未检测到 Obsidian，请在【设置 → 通用 → 外部应用】中配置 Obsidian 程序路径，或安装 Obsidian 并至少运行一次")
	}
	cmd := exec.Command("cmd", "/c", "start", "", uri)
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

// CopyTo 将源路径拷贝到目标路径，支持整体拷贝或仅拷贝目录内容
func (s *FileOperationService) CopyTo(sourcePath, targetPath string, copyWholeDir bool) (string, error) {
	// 校验源路径
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("原地址不存在: %s", sourcePath)
	}

	// 防止将父目录拷贝到子目录导致无限递归
	cleanSource := filepath.Clean(sourcePath)
	cleanTarget := filepath.Clean(targetPath)
	if cleanSource == cleanTarget {
		return "", fmt.Errorf("原地址与目标地址相同")
	}
	if strings.HasPrefix(strings.ToLower(cleanTarget), strings.ToLower(cleanSource)+string(os.PathSeparator)) {
		return "", fmt.Errorf("不能将父目录拷贝到其子目录中")
	}

	// 校验目标路径
	targetInfo, err := os.Stat(targetPath)
	if err == nil && !targetInfo.IsDir() {
		return "", fmt.Errorf("目标地址不是文件夹: %s", targetPath)
	}
	if err != nil {
		// 目标目录不存在，自动创建
		if mkErr := os.MkdirAll(targetPath, 0755); mkErr != nil {
			return "", mkErr
		}
	}

	// 执行拷贝
	if !sourceInfo.IsDir() || copyWholeDir {
		return s.CopyItem(sourcePath, targetPath)
	}

	// copyWholeDir=false 且源是文件夹：逐项拷贝目录内容
	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return "", err
	}

	var lastResult string
	for _, entry := range entries {
		entryPath := filepath.Join(sourcePath, entry.Name())
		result, err := s.CopyItem(entryPath, targetPath)
		if err != nil {
			return "", err
		}
		lastResult = result
	}
	return lastResult, nil
}
