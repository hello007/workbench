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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"workbench/model"
)

const (
	// GitHubReleaseAPI GitHub Releases API 地址
	GitHubReleaseAPI = "https://api.github.com/repos/hello007/workbench/releases/latest"
	// UpdateTempDir 临时下载目录名
	UpdateTempDir = "workbench-update"
	// PendingUpdateFile 待更新标记文件
	PendingUpdateFile = "pending-update.json"
	// updateAssetWindows Windows 平台更新资产名（与 .github/workflows/release.yml 上传产物名严格一致）
	updateAssetWindows = "workbench.exe"
	// updateAssetLinux Linux 平台更新资产名（tar.gz 压缩包，与 release.yml 打包产物名严格一致；
	// 不带版本号——版本由 Release tag 携带，固定资产名保证旧版本客户端按名匹配不失配，amd64 后缀预留 arm64 扩展）
	updateAssetLinux = "workbench-linux-amd64.tar.gz"
	// updateBinaryLinux Linux tar.gz 包内二进制名（扁平布局顶层成员，解包后落 updateDir）
	updateBinaryLinux = "workbench"
)

// UpdateService 更新服务
type UpdateService struct {
	sinkHolder // 事件出口持有器（update:download-progress 推送），SetContext 注入 + serve 模式经 SetEventSink 切换
	httpClient *http.Client
	cancelDL   context.CancelFunc // 用于取消下载
	mu         sync.Mutex         // 保护 cancelDL 字段
}

// updateAssetName 返回当前平台的更新资产名（CheckForUpdate 按平台匹配 GitHub Release 资产）
func updateAssetName() string {
	if runtime.GOOS == "windows" {
		return updateAssetWindows
	}
	return updateAssetLinux
}

// updateBinaryPath 返回更新目录内新版本二进制的路径：
// Windows 资产即二进制本体（workbench.exe 直接落地）；Linux 资产为 tar.gz，
// 下载后由 extractUpdateTarGz 解包出 workbench 二进制
func updateBinaryPath(updateDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(updateDir, updateAssetWindows)
	}
	return filepath.Join(updateDir, updateBinaryLinux)
}

// NewUpdateService 创建更新服务
func NewUpdateService() *UpdateService {
	return &UpdateService{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetContext 设置 Wails 上下文并注入事件出口（用于推送下载进度事件）。
// ctx 为 nil（单测等无 Wails 上下文场景）时事件推送静默跳过（防护集中在 EventSink）。
func (s *UpdateService) SetContext(ctx context.Context) {
	s.SetEventSink(NewWailsEventSink(ctx))
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

	// 按平台查找更新资产（Windows: workbench.exe；Linux: workbench-linux-amd64.tar.gz）
	for _, asset := range release.Assets {
		if asset.Name == updateAssetName() {
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

	// 下载目标按平台资产名落地（Windows: workbench.exe；Linux: workbench-linux-amd64.tar.gz）
	targetFile := filepath.Join(updateDir, updateAssetName())

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

			// 推送进度（经事件出口，sink 未注入或非 Wails 上下文时静默跳过）
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

			s.emitCurrent("update:download-progress", model.DownloadProgress{
				TotalBytes: total,
				Downloaded: downloaded,
				Percent:    percent,
				Speed:      speed,
			})
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("下载中断: %w", err)
		}
	}

	// Linux 资产为 tar.gz 压缩包：下载完成后解包出二进制到 updateDir，后续 ApplyUpdate /
	// CheckPendingUpdate 经 updateBinaryPath 取解包产物走替换流程（Windows 资产即二进制
	// 本体，下载路径即最终路径，无需解包）
	if runtime.GOOS != "windows" {
		if _, err := extractUpdateTarGz(targetFile, updateDir); err != nil {
			return fmt.Errorf("解压更新包失败: %w", err)
		}
	}

	// 写入待更新标记文件
	if err := s.writePendingUpdate(); err != nil {
		return fmt.Errorf("写入更新标记失败: %w", err)
	}

	// 推送完成事件
	s.emitCurrent("update:download-progress", model.DownloadProgress{
		TotalBytes: total,
		Downloaded: downloaded,
		Percent:    100,
		Completed:  true,
	})

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

// shellQuote POSIX shell 单引号包裹：路径内单引号按 POSIX 规则转义为 '\''
// （结束引号、转义引号、重开引号），其余字符（含 " / $ / 反引号 / $() ）在单引号内
// 均失去特殊含义，杜绝双引号直插时路径破坏脚本或以当前用户身份执行任意命令的注入面。
// 与 terminal.go buildPosixCdCommand 同款惯用法；仅 Linux 分支 .sh 脚本使用，
// Windows .bat（cmd 无单引号引用语义）不在其责。
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// buildUpdateSh 生成 Linux 更新 shell 脚本内容（buildUpdateBat 的 POSIX 等价实现）：
// kill -0 轮询等待旧进程退出（最多 10 秒，超时 kill -9 强杀）、mv 替换二进制、
// 清理更新标记与临时目录、nohup 后台启动新版本、脚本自删。
// 与 .bat 的差异：LF 换行（CR 会导致 shebang 解析失败）、路径经 shellQuote 单引号包裹
// 防空格与 shell 元字符注入。
// 替换失败（如安装目录只读）即以非零退出中止、不做任何清理：清理分支会连带删除
// updateDir 内的新二进制与脚本自身，若照常执行将出现「旧版本静默继续 + 更新包被删 +
// 用户误以为已更新」的假更新；此处保留 pending 标记与 updateDir 作为下次启动
// CheckPendingUpdate 重试素材，退出码非零仅依赖 stderr 日志诊断。
// 脚本自身位于 updateDir 内，rm -rf 时已连带删除，末行自删兜底脚本被移出临时目录的场景
// （与 .bat 的 rd /S /Q + del %~f0 行为一致）。
func buildUpdateSh(pid int, newExe, currentExe, pendingFile, updateDir string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("echo 正在更新 WorkBench...\n\n")
	b.WriteString("PID=" + strconv.Itoa(pid) + "\n")
	b.WriteString("WAIT=0\n")
	b.WriteString("# 等待当前进程退出（最多 10 秒，超时强杀）\n")
	b.WriteString("while kill -0 \"$PID\" 2>/dev/null; do\n")
	b.WriteString("    if [ \"$WAIT\" -ge 10 ]; then\n")
	b.WriteString("        kill -9 \"$PID\" 2>/dev/null\n")
	b.WriteString("        break\n")
	b.WriteString("    fi\n")
	b.WriteString("    WAIT=$((WAIT + 1))\n")
	b.WriteString("    sleep 1\n")
	b.WriteString("done\n\n")
	b.WriteString("# 替换二进制（失败即中止，不清理不重启，保留现场待下次启动重试）\n")
	b.WriteString("mv -f " + shellQuote(newExe) + " " + shellQuote(currentExe) +
		" || { echo '替换二进制失败（检查安装目录写入权限）' >&2; exit 1; }\n\n")
	b.WriteString("# 清理更新标记和临时目录\n")
	b.WriteString("rm -f " + shellQuote(pendingFile) + "\n")
	b.WriteString("rm -rf " + shellQuote(updateDir) + "\n\n")
	b.WriteString("# 启动新版本（nohup 后台运行，脱离本脚本会话）\n")
	b.WriteString("nohup " + shellQuote(currentExe) + " >/dev/null 2>&1 &\n\n")
	b.WriteString("# 删除脚本自身\n")
	b.WriteString("rm -f -- \"$0\"\n")
	return b.String()
}

// buildApplySh 生成 Linux 启动时应用更新的 shell 脚本内容（buildApplyBat 的 POSIX 等价实现）：
// mv 替换二进制、清理更新标记与临时目录、nohup 后台启动新版本、脚本自删。
// LF 换行；路径经 shellQuote 单引号包裹防空格与 shell 元字符注入。
// 替换失败分支语义（与 update.sh 的差别：本脚本由 CheckPendingUpdate 在启动早期启动，
// app.go 已因命中 pending 标记 os.Exit(0)）：若失败后不拉起任何进程，用户再次点击图标
// 将无限重复「启动→检测 pending→退出→失败」，应用永久无法打开。故失败分支清除 pending
// 标记（不再自动重试）、保留 updateDir 内新二进制（可诊断，待下次下载覆盖）、拉起未被
// 覆盖的旧版本保证应用可用，再以非零退出留诊断痕迹。
func buildApplySh(newExe, currentExe, pendingFile, updateDir string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("echo 正在应用更新...\n\n")
	b.WriteString("if mv -f " + shellQuote(newExe) + " " + shellQuote(currentExe) + "; then\n")
	b.WriteString("    rm -f " + shellQuote(pendingFile) + "\n")
	b.WriteString("    rm -rf " + shellQuote(updateDir) + "\n")
	b.WriteString("    nohup " + shellQuote(currentExe) + " >/dev/null 2>&1 &\n")
	b.WriteString("else\n")
	b.WriteString("    echo '替换二进制失败（检查安装目录写入权限），本次更新已跳过' >&2\n")
	b.WriteString("    rm -f " + shellQuote(pendingFile) + "\n")
	b.WriteString("    nohup " + shellQuote(currentExe) + " >/dev/null 2>&1 &\n")
	b.WriteString("    exit 1\n")
	b.WriteString("fi\n\n")
	b.WriteString("# 删除脚本自身\n")
	b.WriteString("rm -f -- \"$0\"\n")
	return b.String()
}

// ApplyUpdate 执行更新替换并重启应用
func (s *UpdateService) ApplyUpdate() error {
	updateDir := filepath.Join(os.TempDir(), UpdateTempDir)
	newExe := updateBinaryPath(updateDir)

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

	// 按平台生成更新脚本：Windows 生成 .bat（CRLF），Linux 生成 .sh（LF）
	scriptName, scriptContent := "update.bat", buildUpdateBat(os.Getpid(), newExe, currentExe, pendingFile, updateDir)
	if runtime.GOOS != "windows" {
		scriptName, scriptContent = "update.sh", buildUpdateSh(os.Getpid(), newExe, currentExe, pendingFile, updateDir)
	}
	scriptPath := filepath.Join(updateDir, scriptName)
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return fmt.Errorf("创建更新脚本失败: %w", err)
	}
	// Linux 下落盘受 umask 影响可能丢失执行位，显式 chmod 0755 保证可执行
	if runtime.GOOS != "windows" {
		if err := os.Chmod(scriptPath, 0755); err != nil {
			return fmt.Errorf("设置更新脚本执行权限失败: %w", err)
		}
	}

	// 执行更新脚本（独立进程，不需要等待）：Windows 经 cmd /C 运行 .bat，Linux 经 sh 运行 .sh
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", scriptPath)
	} else {
		cmd = exec.Command("sh", scriptPath)
	}
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

	newExe := updateBinaryPath(updateDir)
	if _, err := os.Stat(newExe); os.IsNotExist(err) {
		// 标记文件在但新版本二进制不存在，清理后返回
		os.RemoveAll(updateDir)
		return false, nil
	}

	// 执行替换
	currentExe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("获取当前程序路径失败: %w", err)
	}

	// 按平台生成应用更新脚本：Windows 生成 .bat（CRLF），Linux 生成 .sh（LF）
	scriptName, scriptContent := "apply-update.bat", buildApplyBat(newExe, currentExe, pendingFile, updateDir)
	if runtime.GOOS != "windows" {
		scriptName, scriptContent = "apply-update.sh", buildApplySh(newExe, currentExe, pendingFile, updateDir)
	}
	scriptPath := filepath.Join(updateDir, scriptName)
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return false, fmt.Errorf("创建更新脚本失败: %w", err)
	}
	// Linux 下落盘受 umask 影响可能丢失执行位，显式 chmod 0755 保证可执行
	if runtime.GOOS != "windows" {
		if err := os.Chmod(scriptPath, 0755); err != nil {
			return false, fmt.Errorf("设置更新脚本执行权限失败: %w", err)
		}
	}

	// 执行更新脚本（独立进程）：Windows 经 cmd /C 运行 .bat，Linux 经 sh 运行 .sh
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", scriptPath)
	} else {
		cmd = exec.Command("sh", scriptPath)
	}
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
