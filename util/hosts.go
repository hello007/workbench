package util

import (
	"errors"
	"net/netip"
	"os"
	"strings"

	"workbench/model"
)

// hosts 行条目类别取值常量（普通 string，非具名 string 类型：service 层映射到
// model.HostsEntry.Kind 的 struct 字段位，具名类型徒增 wails models.ts 别名缺失的
// MISSING_EXPORT 风险，见 docs/spec/cross-layer-contracts.md）。
const (
	HostsKindEntry   = "entry"   // 主机映射行：首个 token 为合法 IP，其余 token 为主机名（可附行内注释）
	HostsKindComment = "comment" // 纯注释行（去首尾空白后以 # 开头）
	HostsKindBlank   = "blank"   // 空行或纯空白行
	HostsKindUnknown = "unknown" // 非空非注释但首个 token 不是合法 IP 的未识别行（不静默丢弃）
)

// HostsEntry hosts 文件单行解析条目。Raw 恒保留原始行文本（剥离行尾 \r），供「复制单行」。
type HostsEntry struct {
	Kind      string   // 行类别：HostsKindEntry / Comment / Blank / Unknown
	IP        string   // entry：地址原样 token（含 IPv6 带 zone 形态）；其余类别空串
	Hostnames []string // entry：主机名列表（IP 单独成行时为空列表）；其余类别为空
	Comment   string   // entry：行内注释（首个 # 之后文本，去首尾空白）；其余类别空串
	Raw       string   // 原始行文本
}

// hostsBOM UTF-8 BOM（Windows 记事本等编辑器保存 hosts 可能携带，容忍剥离）。
const hostsBOM = "\xEF\xBB\xBF"

// ReadHostsFile 读取并解析指定路径的 hosts 文件（路径注入点：生产传 HostsFilePath
// 平台常量，单测注入 fixture 路径，不依赖本机真实 hosts 内容——docs/spec/test-stability.md）。
//
// 读取失败包装为 AppError（用户可读消息 + 根因保留），由 service 层取 Message 写入
// 组级 Error 字段下发，不作为整体异常外抛。
func ReadHostsFile(path string) ([]HostsEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, model.WrapAppError(model.ErrCodeHostsReadFailed, hostsReadFailureMessage(err), err)
	}
	return ParseHosts(string(data)), nil
}

// hostsReadFailureMessage 读取失败的用户可读消息分类（纯函数，按根因错误注入可确定性单测，
// 规避「目录当文件读」的失败类别在 Windows/Linux 上的平台差异耦合断言）。
// 用 errors.Is 而非 os.IsNotExist：后者只对 *PathError 等固定类型解包，不认
// fmt.Errorf %w 包装链，会漏判。
func hostsReadFailureMessage(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "hosts 文件不存在"
	case errors.Is(err, os.ErrPermission):
		return "hosts 文件无读取权限"
	default:
		return "hosts 文件读取失败"
	}
}

// ParseHosts 解析 hosts 文件内容为行条目列表（纯函数，单测入口，两平台共享同测）。
//
// 解析规则（PRD 决策 2026-10-13）：
//   - 容忍 UTF-8 BOM；按 UTF-8 读取，注释乱码不做转码兜底
//   - 首个 token 经 netip.ParseAddr 判定为合法地址（含 IPv6 及带 zone 的 link-local，
//     net.ParseIP 不认 zone 形态故不用）→ entry 行，其余 token 为主机名，不硬编码 token 数
//   - 首个 # 之后为行内注释（IPv6 地址不含 #，首个 # 即注释起点）；去首尾空白后以 #
//     开头为纯注释行；空行 / 纯空白行为 blank 行
//   - 非空非注释且首个 token 非 IP → unknown 未识别行，保留原文不静默丢弃
//   - 文件以换行结尾时，末尾换行是终止符而非空行，不产生多余 blank 条目
func ParseHosts(content string) []HostsEntry {
	content = strings.TrimPrefix(content, hostsBOM)
	lines := strings.Split(content, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	entries := make([]HostsEntry, 0, len(lines))
	for _, line := range lines {
		entries = append(entries, parseHostsLine(strings.TrimSuffix(line, "\r")))
	}
	return entries
}

// parseHostsLine 解析单行为条目（CRLF 已在上层剥离）。
func parseHostsLine(raw string) HostsEntry {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "":
		return HostsEntry{Kind: HostsKindBlank, Raw: raw}
	case strings.HasPrefix(trimmed, "#"):
		return HostsEntry{Kind: HostsKindComment, Raw: raw}
	}
	body, comment := trimmed, ""
	if idx := strings.IndexByte(trimmed, '#'); idx >= 0 {
		body = strings.TrimSpace(trimmed[:idx])
		comment = strings.TrimSpace(trimmed[idx+1:])
	}
	fields := strings.Fields(body)
	// body 非空则 Fields 必非空（首字符既非空白也非 #），此分支防御未来解析规则
	// 变更引入 panic；真出现时按未识别行处理而非崩溃
	if len(fields) == 0 {
		return HostsEntry{Kind: HostsKindUnknown, Raw: raw}
	}
	// 仅做合法性判定，IP 展示保留文件中的原样 token（不做地址规范化改写）
	if _, err := netip.ParseAddr(fields[0]); err == nil {
		return HostsEntry{Kind: HostsKindEntry, IP: fields[0], Hostnames: fields[1:], Comment: comment, Raw: raw}
	}
	return HostsEntry{Kind: HostsKindUnknown, Raw: raw}
}
