package util

import (
	"net/url"
	"path/filepath"
	"strings"
)

// Linux 文件剪贴板的通用载荷格式：x-special/gnome-copied-files。
// 首行为动作（copy/cut），随后每行一个 file:// URI；
// Nautilus/PCManFM/Dolphin 等主流文件管理器通用，且天然携带剪切语义
// （对应 Windows CF_HDROP + Preferred DropEffect 的组合）。
// 纯逻辑（构造/解析）独立成无平台约束的文件，便于两侧平台共同编译测试；
// xclip/wl-copy 命令执行见 clipboard_other.go。

// clipboardFilesTarget 剪贴板目标格式名
const clipboardFilesTarget = "x-special/gnome-copied-files"

// buildClipboardFilesPayload 构造 x-special/gnome-copied-files 载荷：
// 首行动作（copy/cut），随后每行一个 file:// URI（路径经百分号编码，空格/中文安全）。
func buildClipboardFilesPayload(paths []string, isCut bool) string {
	action := "copy"
	if isCut {
		action = "cut"
	}
	lines := make([]string, 0, len(paths)+1)
	lines = append(lines, action)
	for _, p := range paths {
		lines = append(lines, pathToFileURI(p))
	}
	return strings.Join(lines, "\n")
}

// parseClipboardFilesPayload 解析 x-special/gnome-copied-files 载荷。
// ok=false 表示载荷缺失动作行或无有效文件内容，调用方按「剪贴板无文件」处理；
// isCut 由首行动作（cut）得出。
func parseClipboardFilesPayload(data string) (paths []string, isCut bool, ok bool) {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil, false, false
	}
	lines := strings.Split(data, "\n")
	action := strings.TrimSpace(lines[0])
	if action != "copy" && action != "cut" {
		return nil, false, false
	}
	isCut = action == "cut"
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p := fileURIToPath(line)
		if p == "" {
			continue
		}
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		return nil, false, false
	}
	return paths, isCut, true
}

// isASCIILetter 判断字节是否为 ASCII 字母（盘符判定用，不引 unicode 包）
func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// isWindowsDriveSegment 判断 slash 路径是否为 Windows 盘符形态开头（如 C:/a/b：
// 单个 ASCII 字母 + 冒号）。POSIX 路径第二字符为冒号的形态（/a:b）不会误判——
// 其首字符后不是「字母+冒号」结构。
func isWindowsDriveSegment(slashPath string) bool {
	return len(slashPath) >= 2 && slashPath[1] == ':' && isASCIILetter(slashPath[0])
}

// pathToFileURI 将本地路径编码为 file:// URI（\ 与 / 统一为 /，保留字符百分号编码）。
// net/url 对 Host 为空的 URL 序列化不自动补第三斜杠，Windows 盘符路径（C:/a/b）
// 会被产出为 file://C:/a/b（盘符落入 Host 位，解析侧无法还原）——故不以 / 开头的
// 路径前置 /，产出规范的 file:///C:/a/b；Linux 绝对路径（/ 开头）不受影响。
func pathToFileURI(p string) string {
	slash := filepath.ToSlash(p)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	u := url.URL{Scheme: "file", Path: slash}
	return u.String()
}

// fileURIToPath 将 file:// URI 解码为本地路径（百分号编码自动还原）。
// 兼容未加 scheme 的纯 slash 路径（部分文件管理器写入形态）；
// 兼容历史产出缺陷形态 file://C:/a/b（盘符被 net/url 解析进 Host 位，旧版本生成、
// 仍留存于用户剪贴板的数据）与规范形态 file:///C:/a/b（解析出的 /C:/a/b 剥前置斜杠）；
// 指向远程主机的 URI 无法映射本地路径，返回空串。
func fileURIToPath(s string) string {
	if strings.HasPrefix(s, "file://") {
		u, err := url.Parse(s)
		if err != nil {
			return ""
		}
		if u.Host != "" && u.Host != "localhost" {
			// file://C:/a/b 历史形态：盘符落在 Host 位，拼回路径还原
			if isWindowsDriveSegment(u.Host) {
				return filepath.FromSlash(u.Host + u.Path)
			}
			return ""
		}
		// 规范形态 file:///C:/a/b：u.Path 为 /C:/a/b，盘符路径剥前置斜杠还原
		// （仅首段为盘符形态时剥离，POSIX 路径 /tmp/... 不受影响）
		path := u.Path
		if len(path) > 0 && path[0] == '/' && isWindowsDriveSegment(path[1:]) {
			path = path[1:]
		}
		return filepath.FromSlash(path)
	}
	return filepath.FromSlash(s)
}
