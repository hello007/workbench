package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"

	"workbench/model"
	"workbench/util"
)

// 哨兵错误：供 app 层翻译为状态码字符串供前端分流。
var (
	// ErrObsidianNotInstalled 未检测到 Obsidian（未配置 exe 且 obsidian:// 协议未注册）。
	ErrObsidianNotInstalled = errors.New("未检测到 Obsidian")
	// ErrVaultNotRegistered 目标路径不属于任何已注册 Obsidian vault（将触发 Vault not found）。
	ErrVaultNotRegistered = errors.New("目标路径未注册为 Obsidian vault")
	// ErrObsidianRunning Obsidian 正在运行，自动注册需先关闭后重试（规避运行时回写覆盖）。
	ErrObsidianRunning = errors.New("Obsidian 正在运行")
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

	// 按 kind 分流：
	//   image/pdf/office 不在此读取内容、不判 tooLarge——
	//     image/office 由前端走 ReadFileBytes（50MB 上限）取 base64；
	//     pdf 走 iframe + AssetServer Range 流式（无大小限制）。
	//   text 与 unsupported 统一走编码检测（读字节 -> DetectTextEncoding），
	//     解决 unsupported 直接放弃、text 的 GBK 文件乱码两类问题。
	// maxSize（1MB）本意针对文本读全内容，不能用于误伤 image/pdf/office。
	if preview.Kind != model.KindText && preview.Kind != model.KindUnsupported {
		return preview, nil
	}

	// text/unsupported：超过上限则标记过大，不再读全内容
	if preview.Size > maxSize {
		preview.TooLarge = true
		return preview, nil
	}

	data, err := util.ReadFileSafe(filePath, maxSize)
	if err != nil {
		preview.Error = err.Error()
		return preview, err
	}

	// 编码检测：ok=true -> 降级为 text 显示（Content=转码内容，Encoding=来源）；
	// ok=false（含 NUL 或非 UTF-8 且 GBK 解码失败）-> IsBinary=true，Kind 保留原值，Content 空。
	enc, content, ok := util.DetectTextEncoding(data)
	if !ok {
		preview.IsBinary = true
		return preview, nil
	}
	preview.Encoding = enc
	preview.Content = content
	preview.Kind = model.KindText
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

// SaveFile 保存文件内容（原子写入：先写临时文件再 rename）。
// encoding 指定按原文件编码写入：encoding="gbk" -> GBK 编码后写入；
// 其余（utf-8/空）-> 直接写入 UTF-8 字节。保留 1MB 限制与原子写。
func (s *FileOperationService) SaveFile(filePath string, content string, encoding string) error {
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

	// 按原文件编码转码为字节：GBK -> GBK 编码；其余（utf-8/空）-> 原样 UTF-8 字节
	var data []byte
	if encoding == "gbk" {
		encoded, encErr := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(content))
		if encErr != nil {
			return fmt.Errorf("GBK 编码失败: %w", encErr)
		}
		data = encoded
	} else {
		data = []byte(content)
	}

	// 原子写入：先写临时文件再 rename
	dir := filepath.Dir(filePath)
	tmpFile, err := os.CreateTemp(dir, ".workbench-save-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, err = tmpFile.Write(data)
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

// launchObsidianURI 用指定 obsidian:// URI 启动 Obsidian。
// useExe=true 用配置的 exe 启动（优先）；否则走 cmd /c start "" uri 协议方案（尊重默认协议处理器）。
// 复用 HideCommandWindow 隐藏子控制台窗口，与 OpenIn* 系列同构。
func launchObsidianURI(uri, obsidianPath string, useExe bool) error {
	if useExe {
		cmd := exec.Command(obsidianPath, uri)
		util.HideCommandWindow(cmd)
		return cmd.Start()
	}
	cmd := exec.Command("cmd", "/c", "start", "", uri)
	util.HideCommandWindow(cmd)
	return cmd.Start()
}

// OpenInObsidian 用 Obsidian 打开指定路径对应的 vault。
// vault 解析：文件夹->自身，文件->父目录。
// 预检流程：resolveObsidianVault -> 协议/exe 检测 -> 归属判断 -> 发 URI。
//   - 未检测到 Obsidian（未配置 exe 且协议未注册）-> ErrObsidianNotInstalled
//   - obsidian.json 读取失败 -> 降级（直接发 URI，记日志，不阻塞，不比现状更差）
//   - 路径不属于任何已注册 vault -> ErrVaultNotRegistered
//   - 命中某 vault -> 正常发 URI
//
// 注意：用户配置了 exe 且存在时跳过协议预检，但归属判断仍要做（exe 启动同样会 Vault not found）。
func (s *FileOperationService) OpenInObsidian(path, obsidianPath string) error {
	vaultPath, err := resolveObsidianVault(path)
	if err != nil {
		return fmt.Errorf("路径不存在或无法访问: %w", err)
	}
	uri := "obsidian://open?path=" + encodeObsidianPath(vaultPath)

	// 判断 Obsidian 是否可用 + 决定启动方式
	useExe := false
	if strings.TrimSpace(obsidianPath) != "" {
		if _, statErr := os.Stat(obsidianPath); statErr == nil {
			useExe = true
		}
	}
	if !useExe && !isObsidianProtocolRegistered() {
		return ErrObsidianNotInstalled
	}

	// 归属判断（无论 exe 还是协议，目标未注册 vault 都会触发 Vault not found，故统一做）
	vaults, loadErr := loadObsidianVaults()
	if loadErr != nil {
		// 降级：读不到 obsidian.json，直接尽力打开，记日志不阻塞
		println("警告: 读取 Obsidian vault 注册表失败，降级为直接打开:", loadErr.Error())
		return launchObsidianURI(uri, obsidianPath, useExe)
	}
	if _, ok := findVaultForPath(vaults, vaultPath); !ok {
		return ErrVaultNotRegistered
	}
	return launchObsidianURI(uri, obsidianPath, useExe)
}

// OpenObsidianVaultManager 打开 Obsidian 仓库管理器（obsidian://choose-vault），
// 供用户手动将目录「打开文件夹作为仓库」添加为 vault。
// 复用 exe 优先 / cmd /c start "" uri 模式；未检测到 Obsidian 时返回 ErrObsidianNotInstalled。
func (s *FileOperationService) OpenObsidianVaultManager(obsidianPath string) error {
	const uri = "obsidian://choose-vault"
	useExe := false
	if strings.TrimSpace(obsidianPath) != "" {
		if _, statErr := os.Stat(obsidianPath); statErr == nil {
			useExe = true
		}
	}
	if !useExe && !isObsidianProtocolRegistered() {
		return ErrObsidianNotInstalled
	}
	return launchObsidianURI(uri, obsidianPath, useExe)
}

// CopyObsidianVaultPath 将路径对应的 vault 路径文本复制到系统剪贴板（CF_UNICODETEXT）。
// vault 解析：文件夹->自身，文件->父目录。供「打开仓库管理器」前复制路径，便于用户在 Obsidian 路径栏粘贴。
func (s *FileOperationService) CopyObsidianVaultPath(path string) error {
	vaultPath, err := resolveObsidianVault(path)
	if err != nil {
		return fmt.Errorf("路径不存在或无法访问: %w", err)
	}
	return util.WriteClipboardText(vaultPath)
}

// AutoRegisterAndOpen 自动将路径对应目录注册为 Obsidian vault 并打开。
// 流程：resolveObsidianVault -> Obsidian 可用性检测 -> 进程检测 -> 读配置去重 -> 备份 -> 追加条目 -> 原子写 -> 发 URI。
// 返回哨兵错误：
//   - ErrObsidianNotInstalled: 未检测到 Obsidian（未配置 exe 且协议未注册）
//   - ErrObsidianRunning: Obsidian 正在运行（需用户关闭后重试，规避运行时回写覆盖）
//   - 其他: 读取/写入/打开失败
//
// 约束：不创建 .obsidian、不建 <vaultID>.json 窗口缓存、不改 open 字段、不 taskkill。
// 复用现有 resolveObsidianVault/resolvePath/encodeObsidianPath/launchObsidianURI。
func (s *FileOperationService) AutoRegisterAndOpen(path, obsidianPath string) error {
	vaultPath, err := resolveObsidianVault(path)
	if err != nil {
		return fmt.Errorf("路径不存在或无法访问: %w", err)
	}

	// Obsidian 可用性 + 启动方式（与 OpenInObsidian 同构）
	useExe := false
	if strings.TrimSpace(obsidianPath) != "" {
		if _, statErr := os.Stat(obsidianPath); statErr == nil {
			useExe = true
		}
	}
	if !useExe && !isObsidianProtocolRegistered() {
		return ErrObsidianNotInstalled
	}

	// 进程检测：运行中不写入（Obsidian 运行时持有内存缓存，回写会覆盖手动修改）
	if isObsidianRunning() {
		return ErrObsidianRunning
	}

	// 读完整配置（保留未知顶层字段，回写不丢失 updateDisabled 等）
	cfgPath := obsidianConfigPath()
	cfg, err := loadFullConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("读取 Obsidian 配置失败: %w", err)
	}

	// 去重：按 resolvePath 已注册则直接发 URI 打开（幂等，不重复写入）
	resolvedTarget := resolvePath(vaultPath)
	for _, v := range cfg.Vaults {
		if resolvePath(v.Path) == resolvedTarget {
			uri := "obsidian://open?path=" + encodeObsidianPath(vaultPath)
			return launchObsidianURI(uri, obsidianPath, useExe)
		}
	}

	// 备份原配置（异常时可手动恢复）
	if _, err := backupConfig(cfgPath); err != nil {
		return fmt.Errorf("备份配置失败: %w", err)
	}

	// 追加新 vault 条目（不设 Open，不创建窗口缓存）
	id := newVaultID(cfg.Vaults)
	cfg.Vaults[id] = VaultEntry{
		Path: vaultPath,
		Ts:   time.Now().UnixMilli(),
	}

	// 原子写（同分区临时文件 + Rename，避免半写损坏）
	if err := atomicWriteConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}

	// 发 URI 打开（Obsidian 未运行，启动时读取新 vault 列表）
	uri := "obsidian://open?path=" + encodeObsidianPath(vaultPath)
	return launchObsidianURI(uri, obsidianPath, useExe)
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
