package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"workbench/model"
)

const (
	// GitHubReleaseAPI GitHub Releases API 地址
	GitHubReleaseAPI = "https://api.github.com/repos/hello007/workbench/releases/latest"
	// UpdateTempDir 临时下载目录名
	UpdateTempDir = "workbench-update"
	// PendingUpdateFile 待更新标记文件
	PendingUpdateFile = "pending-update.json"
)

// UpdateService 更新服务
type UpdateService struct {
	ctx        context.Context
	httpClient *http.Client
	cancelDL   context.CancelFunc // 用于取消下载
	mu         sync.Mutex         // 保护 cancelDL 字段
}

// NewUpdateService 创建更新服务
func NewUpdateService() *UpdateService {
	return &UpdateService{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetContext 设置 Wails 上下文（用于 EventsEmit）
func (s *UpdateService) SetContext(ctx context.Context) {
	s.ctx = ctx
}

// CheckForUpdate 检查是否有新版本
func (s *UpdateService) CheckForUpdate(currentVersion string) (*model.UpdateInfo, error) {
	resp, err := s.httpClient.Get(GitHubReleaseAPI)
	if err != nil {
		return nil, fmt.Errorf("无法连接更新服务器: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("更新服务器返回错误: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取更新信息失败: %w", err)
	}

	var release struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}

	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("解析更新信息失败: %w", err)
	}

	// 去掉 tag_name 的 v 前缀
	latestVer := strings.TrimPrefix(release.TagName, "v")

	info := &model.UpdateInfo{
		CurrentVer:   currentVersion,
		LatestVer:    latestVer,
		ReleaseNotes: release.Body,
		PublishedAt:  release.PublishedAt,
	}

	// 查找 workbench.exe 资产
	for _, asset := range release.Assets {
		if asset.Name == "workbench.exe" {
			info.DownloadURL = asset.BrowserDownloadURL
			info.FileSize = asset.Size
			break
		}
	}

	if info.DownloadURL == "" {
		return nil, fmt.Errorf("未找到可下载的更新文件")
	}

	// 比较版本号
	info.HasUpdate = CompareVersions(latestVer, currentVersion) > 0

	return info, nil
}

// DownloadUpdate 下载新版本，通过 Wails Events 推送进度
func (s *UpdateService) DownloadUpdate(downloadURL string) error {
	// 创建临时目录
	tempDir := os.TempDir()
	updateDir := filepath.Join(tempDir, UpdateTempDir)
	if err := os.MkdirAll(updateDir, 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}

	targetFile := filepath.Join(updateDir, "workbench.exe")

	// 创建可取消的请求上下文
	dlCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancelDL = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancelDL = nil
		s.mu.Unlock()
	}()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败: %w", err)
	}

	// 使用独立的 HTTP 客户端，不使用带 30s 超时的 s.httpClient，
	// 避免大文件下载被整体超时中断
	downloadClient := &http.Client{}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}

	// 创建目标文件
	out, err := os.Create(targetFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer out.Close()

	total := resp.ContentLength
	var downloaded int64
	startTime := time.Now()
	buf := make([]byte, 32*1024) // 32KB 缓冲

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			written, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("写入临时文件失败: %w", writeErr)
			}
			downloaded += int64(written)

			// 推送进度
			if s.ctx != nil {
				percent := float64(0)
				if total > 0 {
					percent = float64(downloaded) / float64(total) * 100
				}

				elapsed := time.Since(startTime).Seconds()
				var speed string
				if elapsed > 0 {
					bytesPerSec := float64(downloaded) / elapsed
					speed = formatSpeed(bytesPerSec)
				}

				runtime.EventsEmit(s.ctx, "update:download-progress", model.DownloadProgress{
					TotalBytes: total,
					Downloaded: downloaded,
					Percent:    percent,
					Speed:      speed,
				})
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("下载中断: %w", err)
		}
	}

	// 写入待更新标记文件
	if err := s.writePendingUpdate(); err != nil {
		return fmt.Errorf("写入更新标记失败: %w", err)
	}

	// 推送完成事件
	if s.ctx != nil {
		runtime.EventsEmit(s.ctx, "update:download-progress", model.DownloadProgress{
			TotalBytes: total,
			Downloaded: downloaded,
			Percent:    100,
			Completed:  true,
		})
	}

	return nil
}

// CancelDownload 取消下载
func (s *UpdateService) CancelDownload() {
	s.mu.Lock()
	if s.cancelDL != nil {
		s.cancelDL()
		s.cancelDL = nil
	}
	s.mu.Unlock()
	// 清理临时文件
	updateDir := filepath.Join(os.TempDir(), UpdateTempDir)
	os.RemoveAll(updateDir)
}

// buildUpdateBat 生成更新批处理脚本内容（用户确认重启时使用）
func buildUpdateBat(pid int, newExe, currentExe, pendingFile, updateDir string) string {
	var b strings.Builder
	b.WriteString("@echo off\r\n")
	b.WriteString("echo 正在更新 WorkBench...\r\n")
	b.WriteString("setlocal enabledelayedexpansion\r\n\r\n")
	b.WriteString(":: 等待当前进程退出（最多 10 秒）\r\n")
	b.WriteString("set \"PID=" + strconv.Itoa(pid) + "\"\r\n")
	b.WriteString("set \"WAIT=0\"\r\n")
	b.WriteString(":wait_loop\r\n")
	b.WriteString("tasklist /FI \"PID eq %PID%\" 2>nul | find \"%PID%\" >nul\r\n")
	b.WriteString("if !errorlevel! equ 0 (\r\n")
	b.WriteString("    set /a WAIT+=1\r\n")
	b.WriteString("    if !WAIT! geq 10 (\r\n")
	b.WriteString("        taskkill /F /PID %PID% 2>nul\r\n")
	b.WriteString("    ) else (\r\n")
	b.WriteString("        timeout /t 1 /nobreak >nul\r\n")
	b.WriteString("        goto wait_loop\r\n")
	b.WriteString("    )\r\n")
	b.WriteString(")\r\n\r\n")
	b.WriteString(":: 替换 exe\r\n")
	b.WriteString("move /Y \"" + newExe + "\" \"" + currentExe + "\"\r\n\r\n")
	b.WriteString(":: 清理更新标记和临时目录\r\n")
	b.WriteString("del /Q \"" + pendingFile + "\" 2>nul\r\n")
	b.WriteString("rd /S /Q \"" + updateDir + "\" 2>nul\r\n\r\n")
	b.WriteString(":: 启动新版本\r\n")
	b.WriteString("start \"\" \"" + currentExe + "\"\r\n\r\n")
	b.WriteString(":: 删除批处理脚本自身\r\n")
	b.WriteString("(goto) 2>nul & del \"%~f0\"\r\n")
	return b.String()
}

// buildApplyBat 生成启动时应用更新的批处理脚本内容
func buildApplyBat(newExe, currentExe, pendingFile, updateDir string) string {
	var b strings.Builder
	b.WriteString("@echo off\r\n")
	b.WriteString("echo 正在应用更新...\r\n")
	b.WriteString("setlocal\r\n\r\n")
	b.WriteString(":: 替换 exe\r\n")
	b.WriteString("move /Y \"" + newExe + "\" \"" + currentExe + "\"\r\n\r\n")
	b.WriteString(":: 清理\r\n")
	b.WriteString("del /Q \"" + pendingFile + "\" 2>nul\r\n")
	b.WriteString("rd /S /Q \"" + updateDir + "\" 2>nul\r\n\r\n")
	b.WriteString(":: 启动新版本\r\n")
	b.WriteString("start \"\" \"" + currentExe + "\"\r\n\r\n")
	b.WriteString(":: 删除批处理脚本自身\r\n")
	b.WriteString("(goto) 2>nul & del \"%~f0\"\r\n")
	return b.String()
}

// ApplyUpdate 执行更新替换并重启应用
func (s *UpdateService) ApplyUpdate() error {
	updateDir := filepath.Join(os.TempDir(), UpdateTempDir)
	newExe := filepath.Join(updateDir, "workbench.exe")

	// 检查新版本文件是否存在
	if _, err := os.Stat(newExe); os.IsNotExist(err) {
		return fmt.Errorf("更新文件不存在，请重新下载")
	}

	// 获取当前可执行文件路径
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前程序路径失败: %w", err)
	}

	pendingFile := filepath.Join(updateDir, PendingUpdateFile)
	batContent := buildUpdateBat(os.Getpid(), newExe, currentExe, pendingFile, updateDir)

	batPath := filepath.Join(updateDir, "update.bat")
	if err := os.WriteFile(batPath, []byte(batContent), 0755); err != nil {
		return fmt.Errorf("创建更新脚本失败: %w", err)
	}

	// 执行批处理脚本（独立进程，不需要等待）
	cmd := exec.Command("cmd", "/C", batPath)
	cmd.SysProcAttr = hideWindow()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新脚本失败: %w", err)
	}

	return nil
}

// CheckPendingUpdate 检查是否有待应用的更新（启动时调用）
// 如果有待更新文件，执行替换后启动新版本并退出当前进程
// 返回值: hasPending 表示是否检测到并启动了待更新操作, err 表示错误
func (s *UpdateService) CheckPendingUpdate() (bool, error) {
	updateDir := filepath.Join(os.TempDir(), UpdateTempDir)
	pendingFile := filepath.Join(updateDir, PendingUpdateFile)

	// 检查标记文件是否存在
	if _, err := os.Stat(pendingFile); os.IsNotExist(err) {
		return false, nil // 没有待更新
	}

	newExe := filepath.Join(updateDir, "workbench.exe")
	if _, err := os.Stat(newExe); os.IsNotExist(err) {
		// 标记文件在但 exe 不存在，清理后返回
		os.RemoveAll(updateDir)
		return false, nil
	}

	// 执行替换
	currentExe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("获取当前程序路径失败: %w", err)
	}

	batContent := buildApplyBat(newExe, currentExe, pendingFile, updateDir)

	batPath := filepath.Join(updateDir, "apply-update.bat")
	if err := os.WriteFile(batPath, []byte(batContent), 0755); err != nil {
		return false, fmt.Errorf("创建更新脚本失败: %w", err)
	}

	cmd := exec.Command("cmd", "/C", batPath)
	cmd.SysProcAttr = hideWindow()
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("启动更新脚本失败: %w", err)
	}

	return true, nil
}

// writePendingUpdate 写入待更新标记文件
func (s *UpdateService) writePendingUpdate() error {
	updateDir := filepath.Join(os.TempDir(), UpdateTempDir)
	if err := os.MkdirAll(updateDir, 0755); err != nil {
		return err
	}

	pendingFile := filepath.Join(updateDir, PendingUpdateFile)
	data := map[string]string{
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	}
	content, _ := json.Marshal(data)
	return os.WriteFile(pendingFile, content, 0644)
}

// hideWindow 返回 SysProcAttr 用于隐藏批处理脚本窗口
func hideWindow() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// CompareVersions 比较两个语义化版本号
// 返回值: 1 表示 v1 > v2, -1 表示 v1 < v2, 0 表示相等
func CompareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}

		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}

	return 0
}

// formatSpeed 格式化下载速度
func formatSpeed(bytesPerSec float64) string {
	if bytesPerSec < 1024 {
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
	if bytesPerSec < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/1024)
	}
	return fmt.Sprintf("%.1f MB/s", bytesPerSec/(1024*1024))
}
