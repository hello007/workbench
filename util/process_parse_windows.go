//go:build windows

package util

import (
	"encoding/csv"
	"sort"
	"strconv"
	"strings"

	"workbench/model"
)

// netstatEntry `netstat -ano` 单行解析结果（端口→PID 主通道）。
type netstatEntry struct {
	Protocol  string // TCP / UDP（TCPv6、UDPv6 归并为基础协议名）
	LocalAddr string // 本地地址，不含端口
	Port      int
	State     string // LISTENING / ESTABLISHED / ...；UDP 行无状态列，为空串
	Pid       int
}

// tasklistEntry `tasklist /FO CSV /NH` 单行解析结果（进程名 / 内存补通道）。
type tasklistEntry struct {
	Name     string
	MemoryKB int64
}

// parseNetstatOutput 解析 `netstat -ano` 输出为端口条目（实测 421 行 / 0.075s）。
//
// 三条硬约束（research/process-enumeration.md §2.5）：
//  1. 按 strings.Fields 拆分后的**列数**分支解析（TCP 5 列 / UDP 4 列），绝不依赖
//     列头文字——系统 UI 语言本地化会改变列头文案（中文系统为「协议/本地地址/...」）；
//     列头行与 "Active Connections" 标题行因列数不符被自然跳过。
//  2. 本地地址端口取**最后一个冒号之后**，兼容 IPv6 的 "[::]:8080" 与 "fe80::1%12:8080"。
//  3. UDP 行无 State 列（4 列），须按协议分支取列，不能统一取倒数第二列。
func parseNetstatOutput(output string) []netstatEntry {
	var entries []netstatEntry
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		proto := netstatBaseProtocol(fields[0])
		if proto == "" {
			continue
		}
		var local, state, pidField string
		switch {
		case proto == "TCP" && len(fields) == 5:
			local, state, pidField = fields[1], fields[3], fields[4]
		case proto == "UDP" && len(fields) == 4:
			local, state, pidField = fields[1], "", fields[3]
		default:
			continue // 表头行、标题行、畸形行
		}
		pid, err := strconv.Atoi(pidField)
		if err != nil || pid <= 0 {
			continue
		}
		colon := strings.LastIndex(local, ":")
		if colon < 0 {
			continue
		}
		port, err := strconv.Atoi(local[colon+1:])
		if err != nil {
			continue
		}
		entries = append(entries, netstatEntry{
			Protocol:  proto,
			LocalAddr: local[:colon],
			Port:      port,
			State:     strings.ToUpper(state),
			Pid:       pid,
		})
	}
	return entries
}

// netstatBaseProtocol 归并 netstat 协议列为基础协议名，非协议行（表头等）返回空串。
// 协议取值随系统版本为 TCP / TCPv6 / TCP6 / UDP / UDPv6 / UDP6，不受系统 UI 语言
// 本地化影响（实测为英文缩写）。
func netstatBaseProtocol(field string) string {
	switch strings.ToUpper(field) {
	case "TCP", "TCPV6", "TCP6":
		return "TCP"
	case "UDP", "UDPV6", "UDP6":
		return "UDP"
	}
	return ""
}

// parseTasklistCSV 解析 `tasklist /FO CSV /NH` 输出为 PID→进程条目映射（实测 494 行 / ~1.0s）。
//
// 必须用 encoding/csv 而非 Split(",")：内存字段含千分位逗号（实测 "2,904 K"），
// 按逗号切分会把内存值切断错位。字段数不足 5 的行为非法行（"INFO:" 提示行、空行）跳过。
func parseTasklistCSV(output string) map[int]tasklistEntry {
	result := make(map[int]tasklistEntry)
	reader := csv.NewReader(strings.NewReader(output))
	reader.FieldsPerRecord = -1 // 各版本字段数可变，由下方按字段数判定，不交由 csv 报长度错
	reader.LazyQuotes = true    // 个别版本可能输出裸引号，容错解析而非整表失败
	records, err := reader.ReadAll()
	if err != nil && len(records) == 0 {
		return result
	}
	for _, rec := range records {
		if len(rec) < 5 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(rec[1]))
		if err != nil || pid < 0 {
			continue
		}
		result[pid] = tasklistEntry{
			Name:     strings.TrimSpace(rec[0]),
			MemoryKB: parseTasklistMemoryKB(rec[4]),
		}
	}
	return result
}

// parseTasklistMemoryKB 解析 tasklist 内存列为 KB 整数（形如 "2,904 K"）。
// 千分位逗号须先剔除；不可读值（"N/A"）等解析失败返回 0（列表展示降级，不报错）。
func parseTasklistMemoryKB(field string) int64 {
	value := strings.TrimSpace(field)
	for _, suffix := range []string{"K", "k", "B", "b"} {
		value = strings.TrimSpace(strings.TrimSuffix(value, suffix))
	}
	value = strings.ReplaceAll(value, ",", "")
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// windowsSystemProcessNames 系统关键进程名黑名单（去 .exe 后缀的小写名）。
//
// 强杀这些进程后果严重：csrss / smss / lsass / wininit / winlogon / services
// 被终止会立即蓝屏或强制重启（research/process-enumeration.md §5.2）。保护策略为
// 「允许尝试但强确认警告」而非硬禁——用户可能确实需要结束 svchost 排查问题，
// 且多数情况 taskkill 会直接返回拒绝访问。
var windowsSystemProcessNames = map[string]bool{
	"system":              true,
	"system idle process": true,
	"secure system":       true,
	"csrss":               true,
	"smss":                true,
	"lsass":               true,
	"wininit":             true,
	"winlogon":            true,
	"services":            true,
}

// isSystemProcessName 判定进程名是否属 Windows 系统关键进程（空名为未知进程，判否）。
func isSystemProcessName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	lower = strings.TrimSuffix(lower, ".exe")
	return windowsSystemProcessNames[lower]
}

// mergeWindowsProcessInfos 合并 netstat 端口条目与 tasklist 进程条目为进程列表。
//
// 以 tasklist 全量进程为基线：不监听端口的进程（如后台工具）在进程名关键字检索时
// 必须可命中，只按 netstat 建列表会漏。netstat 中有而 tasklist 中无的 PID（权限差异
// 或枚举时序差，进程已退出）补为无名条目，避免端口检索漏结果。
// 结果按 PID 升序（稳定可预期，不随枚举顺序跳动）。
//
// selfPIDs 由调用方注入（本应用自身 + 父进程链），保持本函数为纯函数可 fixture 单测。
func mergeWindowsProcessInfos(netstat []netstatEntry, tasklist map[int]tasklistEntry, selfPIDs map[int]bool) []model.ProcessInfo {
	portsByPid := make(map[int][]model.ProcessPort, len(netstat))
	for _, entry := range netstat {
		portsByPid[entry.Pid] = append(portsByPid[entry.Pid], model.ProcessPort{
			Port:      entry.Port,
			Protocol:  entry.Protocol,
			State:     entry.State,
			LocalAddr: entry.LocalAddr,
		})
	}
	pids := make([]int, 0, len(tasklist)+len(portsByPid))
	for pid := range tasklist {
		pids = append(pids, pid)
	}
	for pid := range portsByPid {
		if _, ok := tasklist[pid]; !ok {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)

	infos := make([]model.ProcessInfo, 0, len(pids))
	for _, pid := range pids {
		var name string
		var memory int64
		if entry, ok := tasklist[pid]; ok {
			name, memory = entry.Name, entry.MemoryKB
		}
		infos = append(infos, model.ProcessInfo{
			Pid:      pid,
			Name:     name,
			Ports:    portsByPid[pid],
			MemoryKB: memory,
			IsSelf:   selfPIDs[pid],
			IsSystem: pid == 0 || isSystemProcessName(name),
		})
	}
	return infos
}
