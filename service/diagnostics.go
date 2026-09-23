package service

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 诊断信息导出：把应用版本/运行环境/日志文件打包为 zip 诊断包，由用户手动提交
// （桌面单用户无后端，「发送」实为导出）。纯函数实现，不注册服务（无依赖注入）。

// DiagnosticsInfo 诊断包元信息，App 层从应用全局状态平铺传入。
type DiagnosticsInfo struct {
	Version   string // 应用版本号（ldflags 注入，dev 模式为 "dev"）
	BuildTime string // 构建时间（ldflags 注入）
	GoVersion string // 编译用 Go 版本（runtime.Version）
	OSInfo    string // 运行平台（GOOS/GOARCH）
}

// BuildDiagnosticsZip 生成诊断包 zip 到 zipPath。
//
// 包结构：
//
//	diagnostics.txt     —— 版本/构建/平台/导出时间 + 收纳文件清单（含日志缺失说明）
//	logs/<name>...      —— logDir 下全部日志文件（app.log 与轮转备份，扁平收纳，保留原文件名）
//	session.json        —— 崩溃恢复 UI 快照（存在时收纳，供排查异常退出场景）
//
// 降级口径：logDir 不存在或为空 → 跳过 logs/ 并在 diagnostics.txt 注明；sessionPath
// 不存在 → 跳过。仅这两种缺失降级，zip 目标不可写等真实错误上抛。
func BuildDiagnosticsZip(zipPath string, info DiagnosticsInfo, logDir, sessionPath string) error {
	// 先收集条目再统一写入：日志文件清单要进 diagnostics.txt，须先扫后写
	type entry struct {
		name string // zip 内路径（/ 分隔）
		src  string // 本地源路径
	}
	var entries []entry
	var missing []string // 降级说明

	if files := listLogFiles(logDir); len(files) > 0 {
		for _, f := range files {
			entries = append(entries, entry{name: "logs/" + filepath.Base(f), src: f})
		}
	} else {
		missing = append(missing, "logs/（日志目录不存在或无日志文件："+logDir+"）")
	}
	if _, err := os.Stat(sessionPath); err == nil {
		entries = append(entries, entry{name: "session.json", src: sessionPath})
	} else {
		missing = append(missing, "session.json（不存在，未收纳）")
	}

	f, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("创建诊断包文件失败: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)

	// diagnostics.txt
	var b strings.Builder
	fmt.Fprintf(&b, "WorkBench 诊断信息\n")
	fmt.Fprintf(&b, "==================\n")
	fmt.Fprintf(&b, "版本: %s\n", info.Version)
	fmt.Fprintf(&b, "构建时间: %s\n", info.BuildTime)
	fmt.Fprintf(&b, "Go 版本: %s\n", info.GoVersion)
	fmt.Fprintf(&b, "运行平台: %s\n", info.OSInfo)
	fmt.Fprintf(&b, "导出时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "\n收纳文件:\n")
	if len(entries) == 0 {
		b.WriteString("  （无）\n")
	}
	for _, e := range entries {
		fmt.Fprintf(&b, "  %s\n", e.name)
	}
	for _, m := range missing {
		fmt.Fprintf(&b, "  [缺失] %s\n", m)
	}
	if err := writeZipEntry(zw, "diagnostics.txt", strings.NewReader(b.String())); err != nil {
		return err
	}

	// 日志与 session 快照
	for _, e := range entries {
		src, err := os.Open(e.src)
		if err != nil {
			return fmt.Errorf("打开 %s 失败: %w", e.src, err)
		}
		err = writeZipEntry(zw, e.name, src)
		src.Close()
		if err != nil {
			return err
		}
	}

	return zw.Close()
}

// listLogFiles 列出 logDir 下的日志文件（不递归），按文件名排序保证清单稳定。
// 目录不存在或为空返回 nil。
func listLogFiles(logDir string) []string {
	des, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}
	var files []string
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		files = append(files, filepath.Join(logDir, de.Name()))
	}
	sort.Strings(files)
	return files
}

// writeZipEntry 写入单个 zip 条目（ Deflate 压缩）。
func writeZipEntry(zw *zip.Writer, name string, r io.Reader) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("创建 zip 条目 %s 失败: %w", name, err)
	}
	if _, err := io.Copy(w, r); err != nil {
		return fmt.Errorf("写入 zip 条目 %s 失败: %w", name, err)
	}
	return nil
}

// ===== 异常退出标记（crash.flag）=====

// 异常退出标记用独立文件而非 session.json 字段：session.json 被前端 debounce
// 频繁 Save 覆盖写（SessionService.Save），crashed 标记混入会被正常快照保存冲掉。
// flag 生命周期：startup 检测旧 flag（存在=上次异常退出）→ 重建标记本次运行 →
// 正常 shutdown 删除。文件内容为标记时间戳文本，便于人工查看创建时机。

// DetectLastCrash 检测上次会话是否异常退出（flagPath 存在即为真）。
func DetectLastCrash(flagPath string) bool {
	_, err := os.Stat(flagPath)
	return err == nil
}

// MarkSessionStart 写入本次会话的运行中标记（启动时调用，内容为标记时间）。
// 写入失败仅返回错误由调用方记日志（flag 缺失只影响崩溃检测灵敏度，不阻塞启动）。
func MarkSessionStart(flagPath string) error {
	return os.WriteFile(flagPath, []byte(time.Now().Format(time.RFC3339)), 0644)
}

// ClearCrashFlag 清除异常退出标记（正常 shutdown 时调用；文件不存在静默成功）。
func ClearCrashFlag(flagPath string) error {
	err := os.Remove(flagPath)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
