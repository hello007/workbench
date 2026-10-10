//go:build !windows

package util

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"workbench/model"
)

// procNetEntry /proc/net/tcp（或 tcp6）单行解析结果。
type procNetEntry struct {
	LocalAddr string // 可读本地地址，不含端口
	Port      int
	Protocol  string // TCP（tcp6 与 tcp 同为基础协议 TCP）
	State     string // LISTEN / ESTABLISHED 等标准状态名
	Inode     string // socket inode，用于经 /proc/<pid>/fd 反查 PID
}

// linuxProcMeta Linux 单进程的元数据（进程名 / 内存 / 父进程 / 内核线程标志）。
type linuxProcMeta struct {
	Name     string
	MemoryKB int64
	PPid     int
	IsKernel bool // 内核线程（cmdline 为空）：无用户态实体，不可终止
}

// tcpStateNames /proc/net/tcp 的 st 字段（十六进制）到标准状态名映射。
// 取值来自内核 tcp_states 枚举（include/net/tcp_states.h）。
var tcpStateNames = map[string]string{
	"01": "ESTABLISHED",
	"02": "SYN_SENT",
	"03": "SYN_RECV",
	"04": "FIN_WAIT1",
	"05": "FIN_WAIT2",
	"06": "TIME_WAIT",
	"07": "CLOSE",
	"08": "CLOSE_WAIT",
	"09": "LAST_ACK",
	"0A": "LISTEN",
	"0B": "CLOSING",
	"0C": "NEW_SYN_RECV",
}

// parseProcNetTCP 解析 /proc/net/tcp（ipv6 为 true 时即 tcp6）内容为连接条目。
func parseProcNetTCP(content string, ipv6 bool) []procNetEntry {
	return parseProcNetConnections(content, ipv6, "TCP")
}

// parseProcNetUDP 解析 /proc/net/udp（ipv6 为 true 时即 udp6）内容为端口条目。
//
// 行格式与 tcp 完全一致（列布局相同），差异在协议标记与 st 列语义：UDP 无连接，
// st 恒为 07（CLOSE），照搬 TCP 状态名会让已绑定的 UDP 端口在界面上显示成「已关闭」
// 而误导用户，也与 Windows netstat 的 UDP 行（无 State 列）不一致。故状态留空——
// service 层按「状态无法识别」保守保留（与 Windows UDP 行同处理，见 filterListeningProcesses）。
func parseProcNetUDP(content string, ipv6 bool) []procNetEntry {
	entries := parseProcNetConnections(content, ipv6, "UDP")
	for i := range entries {
		entries[i].State = ""
	}
	return entries
}

// parseProcNetConnections 解析 /proc/net/{tcp,tcp6,udp,udp6} 内容为连接条目。
//
// 行格式（首字段为 "<sl>:" 序号才为数据行，表头行以此跳过）：
//
//	0: 0100007F:8CF2 00000000:0000 0A 00000000:00000000 00:00000000 00000000  0  0 22458 1 ...
//	   └本地地址:端口 └远端地址:端口 └状态 └...                              └inode(第 10 字段)
//
// 端口为十六进制（大端，直接解析）；地址为小端十六进制（见 formatProcHexAddr）。
// protocol 由调用方给定（tcp 系为 TCP，udp 系为 UDP）。
func parseProcNetConnections(content string, ipv6 bool, protocol string) []procNetEntry {
	var entries []procNetEntry
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || !strings.HasSuffix(fields[0], ":") {
			continue // 表头行 / 空行 / 畸形行
		}
		local, stateCode, inode := fields[1], fields[3], fields[9]
		colon := strings.LastIndex(local, ":")
		if colon < 0 {
			continue
		}
		port, err := strconv.ParseUint(local[colon+1:], 16, 32)
		if err != nil {
			continue
		}
		state := tcpStateNames[strings.ToUpper(stateCode)]
		if state == "" {
			state = strings.ToUpper(stateCode) // 未知状态码原样透出，不丢行
		}
		entries = append(entries, procNetEntry{
			LocalAddr: formatProcHexAddr(local[:colon], ipv6),
			Port:      int(port),
			Protocol:  protocol,
			State:     state,
			Inode:     inode,
		})
	}
	return entries
}

// formatProcHexAddr 把 /proc/net/tcp{,6} 的十六进制地址转换为可读地址。
//
// IPv4 为 4 字节、按主机字节序（小端）打印（"0100007F" → "127.0.0.1"）；
// IPv6 为 16 字节、按每 4 字节一个 32 位字以主机字节序打印（共 4 个字，32 个
// 十六进制字符），每个字内反转字节后拆为两个 16 位组输出（IPv6 展示惯例）：
// "0000000000000000FFFF00000100007F" → "0000:0000:0000:0000:0000:ffff:7f00:0001"。
// 解析失败（位数不符）时原样返回十六进制串，不丢条目。
func formatProcHexAddr(hexAddr string, ipv6 bool) string {
	raw, err := hex.DecodeString(hexAddr)
	if err != nil {
		return hexAddr
	}
	if !ipv6 {
		if len(raw) != 4 {
			return hexAddr
		}
		return fmt.Sprintf("%d.%d.%d.%d", raw[3], raw[2], raw[1], raw[0])
	}
	if len(raw) != 16 {
		return hexAddr
	}
	groups := make([]string, 0, 8)
	for i := 0; i < 16; i += 4 {
		groups = append(groups, fmt.Sprintf("%02x%02x", raw[i+3], raw[i+2]))
		groups = append(groups, fmt.Sprintf("%02x%02x", raw[i+1], raw[i]))
	}
	return strings.Join(groups, ":")
}

// parseProcStatusRSSKB 从 /proc/<pid>/status 内容取 VmRSS（常驻内存，KB）。
// 字段缺失或不可读（内核线程无 VmRSS）返回 0。
func parseProcStatusRSSKB(content string) int64 {
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "VmRSS" {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return 0
		}
		kb, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return 0
		}
		return kb
	}
	return 0
}

// parseProcStatPPid 从 /proc/<pid>/stat 内容取父进程 PID。
//
// comm 字段可含空格与括号（如 "(sshd)"、"(kworker/0:1)"），故取**最后一个** ')'
// 之后按空格切分：其后布局为 state ppid ...，即第 2 个字段为 PPid。
func parseProcStatPPid(content string) int {
	idx := strings.LastIndex(content, ")")
	if idx < 0 {
		return 0
	}
	fields := strings.Fields(content[idx+1:])
	if len(fields) < 2 {
		return 0
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0
	}
	return ppid
}

// parseSocketInode 从 /proc/<pid>/fd/<n> 的 readlink 结果提取 socket inode。
// 链接形如 "socket:[22458]"；非 socket 链接（普通文件 / 管道 / anon_inode）返回 ok=false。
func parseSocketInode(link string) (string, bool) {
	rest, ok := strings.CutPrefix(link, "socket:[")
	if !ok {
		return "", false
	}
	inode, ok := strings.CutSuffix(rest, "]")
	if !ok || inode == "" {
		return "", false
	}
	return inode, true
}

// mergeLinuxProcessInfos 合并端口映射与进程元数据为进程列表（纯函数，fixture 单测入口）。
//
// 以元数据表为基线（覆盖不监听端口的进程，进程名检索须命中），端口按 PID 挂接；
// 仅在端口表中出现的 PID（进程已退出或权限差异）补为无名条目，避免端口检索漏结果。
// 结果按 PID 升序。
//
// selfPIDs 由调用方注入（本应用自身 + 父进程链），保持本函数为纯函数。
func mergeLinuxProcessInfos(portsByPid map[int][]model.ProcessPort, metaByPid map[int]linuxProcMeta, selfPIDs map[int]bool) []model.ProcessInfo {
	pids := make([]int, 0, len(metaByPid)+len(portsByPid))
	for pid := range metaByPid {
		pids = append(pids, pid)
	}
	for pid := range portsByPid {
		if _, ok := metaByPid[pid]; !ok {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)

	infos := make([]model.ProcessInfo, 0, len(pids))
	for _, pid := range pids {
		meta := metaByPid[pid] // 缺失时为零值：无名、0 内存、未知父进程
		infos = append(infos, model.ProcessInfo{
			Pid:      pid,
			Name:     meta.Name,
			Ports:    portsByPid[pid],
			MemoryKB: meta.MemoryKB,
			IsSelf:   selfPIDs[pid],
			IsSystem: isLinuxSystemProcess(pid, meta),
		})
	}
	return infos
}

// isLinuxSystemProcess 判定是否系统关键进程：PID 1（init/systemd）、父进程为 2 的
// 内核线程，或元数据标记为内核线程（/proc/<pid>/cmdline 为空，无用户态实体）。
func isLinuxSystemProcess(pid int, meta linuxProcMeta) bool {
	return pid == 1 || meta.IsKernel || (meta.PPid == 2)
}
