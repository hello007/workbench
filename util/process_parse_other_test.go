//go:build !windows

package util

import (
	"testing"

	"workbench/model"
)

// procNetTCPFixture 真实 /proc/net/tcp 内容形态：表头行 + 监听行 + 已建立连接行。
const procNetTCPFixture = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:8CF2 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 22458 1 0000000000000000 100 0 0 10 0
   1: 00000000:8CF3 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 22459 1 0000000000000000 100 0 0 10 0
   2: 0100007F:C000 0100007F:D431 01 00000000:00000000 00:00000000 00000000  1000        0 22460 1 0000000000000000 100 0 0 10 0
`

func TestParseProcNetTCP_IPv4(t *testing.T) {
	entries := parseProcNetTCP(procNetTCPFixture, false)
	if len(entries) != 3 {
		t.Fatalf("条目数: got %d, want 3（表头行须跳过）: %+v", len(entries), entries)
	}
	cases := []struct {
		index int
		local string
		port  int
		state string
		inode string
	}{
		// 0100007F 小端 → 127.0.0.1；8CF2 → 36082
		{0, "127.0.0.1", 36082, "LISTEN", "22458"},
		// 00000000 → 0.0.0.0（监听所有地址）
		{1, "0.0.0.0", 36083, "LISTEN", "22459"},
		// C000 → 49152，st=01 为 ESTABLISHED
		{2, "127.0.0.1", 49152, "ESTABLISHED", "22460"},
	}
	for _, c := range cases {
		got := entries[c.index]
		if got.LocalAddr != c.local || got.Port != c.port || got.State != c.state ||
			got.Inode != c.inode || got.Protocol != "TCP" {
			t.Errorf("条目[%d]: got %+v, want %+v", c.index, got, c)
		}
	}
}

func TestParseProcNetTCP_IPv6(t *testing.T) {
	const tcp6 = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 22461 1 0000000000000000 100 0 0 10 0
   1: 0000000000000000FFFF00000100007F:2328 0000000000000000FFFF00000100007F:0000 0A 00000000:00000000 00:00000000 00000000     0        0 22462 1 0000000000000000 100 0 0 10 0
`
	entries := parseProcNetTCP(tcp6, true)
	if len(entries) != 2 {
		t.Fatalf("条目数: got %d, want 2: %+v", len(entries), entries)
	}
	if entries[0].LocalAddr != "0000:0000:0000:0000:0000:0000:0000:0000" {
		t.Errorf("IPv6 全零地址: got %q", entries[0].LocalAddr)
	}
	if entries[0].Port != 8080 || entries[0].State != "LISTEN" || entries[0].Inode != "22461" {
		t.Errorf("IPv6 条目: got %+v", entries[0])
	}
	// IPv4 映射地址（::ffff:127.0.0.1）：末 4 字节 0100007F 小端 → 7f00:0001
	if entries[1].LocalAddr != "0000:0000:0000:0000:0000:ffff:7f00:0001" {
		t.Errorf("IPv4 映射地址: got %q", entries[1].LocalAddr)
	}
	if entries[1].Port != 9000 {
		t.Errorf("端口 0x2328: got %d, want 9000", entries[1].Port)
	}
}

// procNetUDPFixture 真实 /proc/net/udp 内容形态：列布局与 tcp 一致，st 恒为 07（CLOSE）。
const procNetUDPFixture = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:14E9 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 22470 2 0000000000000000 0
   1: 0100007F:8CF2 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 22471 2 0000000000000000 0
`

func TestParseProcNetUDP_IPv4(t *testing.T) {
	entries := parseProcNetUDP(procNetUDPFixture, false)
	if len(entries) != 2 {
		t.Fatalf("条目数: got %d, want 2: %+v", len(entries), entries)
	}
	if entries[0].Protocol != "UDP" {
		t.Errorf("协议须标记为 UDP: got %q", entries[0].Protocol)
	}
	if entries[0].Port != 5353 || entries[0].LocalAddr != "0.0.0.0" || entries[0].Inode != "22470" {
		t.Errorf("UDP 条目: got %+v", entries[0])
	}
	// UDP 无连接：st=07（CLOSE）不具 TCP 语义，状态留空由 service 层保守保留
	if entries[0].State != "" || entries[1].State != "" {
		t.Errorf("UDP 状态须留空（照搬 TCP 的 CLOSE 会误导用户）: got %q, %q", entries[0].State, entries[1].State)
	}
}

func TestParseProcNetUDP_IPv6(t *testing.T) {
	const udp6 = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:14EA 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 22472 2 0000000000000000 0
`
	entries := parseProcNetUDP(udp6, true)
	if len(entries) != 1 {
		t.Fatalf("条目数: got %d, want 1: %+v", len(entries), entries)
	}
	if entries[0].Port != 5354 || entries[0].Protocol != "UDP" || entries[0].Inode != "22472" {
		t.Errorf("udp6 条目: got %+v", entries[0])
	}
	if entries[0].LocalAddr != "0000:0000:0000:0000:0000:0000:0000:0000" {
		t.Errorf("udp6 地址: got %q", entries[0].LocalAddr)
	}
}

func TestParseProcNetTCP_SkipsMalformed(t *testing.T) {
	const malformed = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 22458 1
   1: 0100007F:ZZZZ 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 22459 1
   2: 0100007F:8CF2 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 22460 1
`
	entries := parseProcNetTCP(malformed, false)
	if len(entries) != 1 || entries[0].Inode != "22460" {
		t.Fatalf("畸形行须跳过，仅保留可解析行: got %+v", entries)
	}
}

func TestFormatProcHexAddr_InvalidKeepsRaw(t *testing.T) {
	if got := formatProcHexAddr("zzzz", false); got != "zzzz" {
		t.Errorf("非十六进制须原样返回: got %q", got)
	}
	if got := formatProcHexAddr("0100007F00", false); got != "0100007F00" {
		t.Errorf("位数不符须原样返回: got %q", got)
	}
}

func TestParseProcStatusRSSKB(t *testing.T) {
	const status = "Name:\tnode\nUmask:\t0022\nVmSize:\t  912384 kB\nVmRSS:\t  105464 kB\nRssAnon:\t   91234 kB\n"
	if got := parseProcStatusRSSKB(status); got != 105464 {
		t.Errorf("VmRSS: got %d, want 105464", got)
	}
	if got := parseProcStatusRSSKB("Name:\tkthread\nVmSize:\t 0 kB\n"); got != 0 {
		t.Errorf("无 VmRSS 须返回 0: got %d", got)
	}
}

func TestParseProcStatPPid(t *testing.T) {
	cases := []struct {
		content string
		want    int
	}{
		{"44336 (node) S 1234 44336 44336 0 -1 4194304 0 0 0 0 0 0", 1234},
		// comm 含空格与斜杠，须按最后一个 ')' 之后切分
		{"77 (kworker/0:1-events) I 2 0 0 0 -1 69238872 0 0 0 0 0 0", 2},
		{"1 (systemd) S 0 1 1 0 -1 4194304 0 0 0 0 0 0", 0},
		// 无括号包裹的 comm：判为不可解析，返 0
		{"44336 node S 1234", 0},
		// 字段不足：返 0
		{"44336 (node) S", 0},
	}
	for _, c := range cases {
		if got := parseProcStatPPid(c.content); got != c.want {
			t.Errorf("parseProcStatPPid(%q): got %d, want %d", c.content, got, c.want)
		}
	}
}

func TestParseSocketInode(t *testing.T) {
	if inode, ok := parseSocketInode("socket:[22458]"); !ok || inode != "22458" {
		t.Errorf("socket 链接: got %q, %v", inode, ok)
	}
	for _, link := range []string{"/dev/null", "pipe:[1234]", "anon_inode:[eventfd]", "socket:[]", ""} {
		if _, ok := parseSocketInode(link); ok {
			t.Errorf("%q 不应识别为 socket 链接", link)
		}
	}
}

func TestIsLinuxSystemProcess(t *testing.T) {
	cases := []struct {
		pid  int
		meta linuxProcMeta
		want bool
	}{
		// init 进程
		{1, linuxProcMeta{Name: "systemd"}, true},
		// 内核线程（父为 kthreadd，PID 2）
		{77, linuxProcMeta{PPid: 2}, true},
		// cmdline 为空的内核线程
		{88, linuxProcMeta{IsKernel: true}, true},
		{44336, linuxProcMeta{Name: "node", PPid: 1}, false},
	}
	for _, c := range cases {
		if got := isLinuxSystemProcess(c.pid, c.meta); got != c.want {
			t.Errorf("isLinuxSystemProcess(%d, %+v): got %v, want %v", c.pid, c.meta, got, c.want)
		}
	}
}

func TestMergeLinuxProcessInfos(t *testing.T) {
	ports := map[int][]model.ProcessPort{
		44336: {{Port: 8080, Protocol: "TCP", State: "LISTEN", LocalAddr: "0.0.0.0"}},
		9999:  {{Port: 5353, Protocol: "TCP", State: "LISTEN", LocalAddr: "0.0.0.0"}},
	}
	meta := map[int]linuxProcMeta{
		1:     {Name: "systemd", PPid: 0},
		1234:  {Name: "node", MemoryKB: 1024, PPid: 1},
		44336: {Name: "nginx", MemoryKB: 2048, PPid: 1},
	}
	infos := mergeLinuxProcessInfos(ports, meta, map[int]bool{1234: true})
	if len(infos) != 4 {
		t.Fatalf("条目数: got %d, want 4（含元数据表外但持有端口的 PID）", len(infos))
	}
	wantPids := []int{1, 1234, 9999, 44336}
	for i, want := range wantPids {
		if infos[i].Pid != want {
			t.Fatalf("条目[%d] PID: got %d, want %d（须按 PID 升序）", i, infos[i].Pid, want)
		}
	}
	if !infos[0].IsSystem {
		t.Error("PID 1 须标记 IsSystem")
	}
	if !infos[1].IsSelf || infos[0].IsSelf {
		t.Error("IsSelf 须仅命中注入的自身 PID 集合")
	}
	if infos[2].Name != "" || len(infos[2].Ports) != 1 {
		t.Errorf("元数据缺失的 PID 须补为无名条目并挂上端口: got %+v", infos[2])
	}
	if infos[3].Name != "nginx" || infos[3].MemoryKB != 2048 || len(infos[3].Ports) != 1 {
		t.Errorf("端口与元数据须按 PID 合并: got %+v", infos[3])
	}
}
