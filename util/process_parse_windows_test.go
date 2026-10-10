//go:build windows

package util

import (
	"testing"
)

// netstatFixture 真实 `netstat -ano` 输出形态（本机实测 421 行）：英文表头 +
// "Active Connections" 标题 + IPv4 TCP + IPv6 TCP（含 [::] 与链路本地 fe80::x%n）+
// UDP（无状态列，4 列）。
const netstatFixture = `
Active Connections

  Proto  Local Address          Foreign Address        State           PID
  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       1912
  TCP    127.0.0.1:36115        0.0.0.0:0              LISTENING       44336
  TCP    [::]:8080              [::]:0                 LISTENING       7712
  TCP    fe80::1%12:8081        fe80::2%12:0           ESTABLISHED     7712
  UDP    0.0.0.0:50555          *:*                                    44336
  UDP    [::]:5353              *:*                                    8888
`

func TestParseNetstatOutput_MixedRows(t *testing.T) {
	entries := parseNetstatOutput(netstatFixture)
	if len(entries) != 6 {
		t.Fatalf("条目数: got %d, want 6（表头与标题行须被跳过）: %+v", len(entries), entries)
	}
	cases := []struct {
		index    int
		protocol string
		local    string
		port     int
		state    string
		pid      int
	}{
		{0, "TCP", "0.0.0.0", 135, "LISTENING", 1912},
		{1, "TCP", "127.0.0.1", 36115, "LISTENING", 44336},
		// IPv6 全零地址 [::]：端口取最后一个冒号之后
		{2, "TCP", "[::]", 8080, "LISTENING", 7712},
		// 链路本地地址带作用域后缀 fe80::1%12：端口仍在最后一个冒号之后
		{3, "TCP", "fe80::1%12", 8081, "ESTABLISHED", 7712},
		// UDP 无 State 列（4 列），须按协议分支取列
		{4, "UDP", "0.0.0.0", 50555, "", 44336},
		{5, "UDP", "[::]", 5353, "", 8888},
	}
	for _, c := range cases {
		got := entries[c.index]
		if got.Protocol != c.protocol || got.LocalAddr != c.local || got.Port != c.port ||
			got.State != c.state || got.Pid != c.pid {
			t.Errorf("条目[%d]: got %+v, want %+v", c.index, got, c)
		}
	}
}

// TestParseNetstatOutput_LocalizedHeader 中文系统列表头形态（解析不得依赖列头文字）。
func TestParseNetstatOutput_LocalizedHeader(t *testing.T) {
	const localized = `
活动连接

  协议    本地地址              外部地址               状态             PID
  TCP    0.0.0.0:443            0.0.0.0:0              LISTENING       9999
`
	entries := parseNetstatOutput(localized)
	if len(entries) != 1 {
		t.Fatalf("中文表头下条目数: got %d, want 1: %+v", len(entries), entries)
	}
	if entries[0].Port != 443 || entries[0].Pid != 9999 {
		t.Errorf("中文表头下解析结果: got %+v", entries[0])
	}
}

func TestParseNetstatOutput_SkipsInvalidRows(t *testing.T) {
	const invalid = `
  Proto  Local Address          Foreign Address        State           PID
  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       notapid
  TCP    0.0.0.0                0.0.0.0:0              LISTENING       1912
  TCP6   [::1]:9000             [::]:0                 LISTENING       1234
  UDP    0.0.0.0:53             *:*                                    *
`
	entries := parseNetstatOutput(invalid)
	if len(entries) != 1 {
		t.Fatalf("非法行须跳过: got %d, want 1: %+v", len(entries), entries)
	}
	if entries[0].Protocol != "TCP" || entries[0].Port != 9000 || entries[0].Pid != 1234 {
		t.Errorf("TCPv6 须归并为 TCP: got %+v", entries[0])
	}
}

const tasklistFixture = `"System Idle Process","0","Services","0","8 K"
"System","4","Services","0","2,904 K"
"node.exe","44336","Console","1","105,464 K"
"workbench.exe","1234","Console","1","1,234,567 K"
INFO: No tasks are running which match the specified criteria.

`

func TestParseTasklistCSV_Fixture(t *testing.T) {
	entries := parseTasklistCSV(tasklistFixture)
	if len(entries) != 4 {
		t.Fatalf("条目数: got %d, want 4（INFO 行与空行须跳过）: %+v", len(entries), entries)
	}
	if got := entries[44336]; got.Name != "node.exe" || got.MemoryKB != 105464 {
		t.Errorf("44336: got %+v, want node.exe / 105464（千分位逗号须剔除）", got)
	}
	if got := entries[4]; got.Name != "System" || got.MemoryKB != 2904 {
		t.Errorf("4: got %+v, want System / 2904", got)
	}
	if got := entries[0]; got.Name != "System Idle Process" || got.MemoryKB != 8 {
		t.Errorf("0: got %+v, want System Idle Process / 8", got)
	}
	if got := entries[1234]; got.MemoryKB != 1234567 {
		t.Errorf("1234 内存: got %d, want 1234567", got.MemoryKB)
	}
}

func TestParseTasklistCSV_UnreadableMemory(t *testing.T) {
	const content = `"MsMpEng.exe","8888","Services","0","N/A"`
	entries := parseTasklistCSV(content)
	if got := entries[8888]; got.Name != "MsMpEng.exe" || got.MemoryKB != 0 {
		t.Errorf("内存不可读须降级为 0 且保留进程名: got %+v", got)
	}
}

func TestIsSystemProcessName(t *testing.T) {
	systemNames := []string{"System", "system idle process", "csrss.exe", "LSASS.EXE", "services.exe", "wininit.exe"}
	for _, name := range systemNames {
		if !isSystemProcessName(name) {
			t.Errorf("%q 应判定为系统关键进程", name)
		}
	}
	normalNames := []string{"node.exe", "workbench.exe", "java.exe", "", "MySystem.exe"}
	for _, name := range normalNames {
		if isSystemProcessName(name) {
			t.Errorf("%q 不应判定为系统关键进程", name)
		}
	}
}

func TestMergeWindowsProcessInfos(t *testing.T) {
	netstat := []netstatEntry{
		{Protocol: "TCP", LocalAddr: "127.0.0.1", Port: 36115, State: "LISTENING", Pid: 44336},
		{Protocol: "TCP", LocalAddr: "0.0.0.0", Port: 80, State: "LISTENING", Pid: 9999},
		{Protocol: "UDP", LocalAddr: "0.0.0.0", Port: 50555, State: "", Pid: 44336},
	}
	tasklist := map[int]tasklistEntry{
		4:     {Name: "System", MemoryKB: 2904},
		1234:  {Name: "workbench.exe", MemoryKB: 1234567},
		44336: {Name: "node.exe", MemoryKB: 105464},
	}
	infos := mergeWindowsProcessInfos(netstat, tasklist, map[int]bool{1234: true})
	if len(infos) != 4 {
		t.Fatalf("条目数: got %d, want 4（含不监听端口的进程与端口表独有 PID）", len(infos))
	}
	// 结果按 PID 升序
	wantPids := []int{4, 1234, 9999, 44336}
	for i, want := range wantPids {
		if infos[i].Pid != want {
			t.Fatalf("条目[%d] PID: got %d, want %d（结果须按 PID 升序）", i, infos[i].Pid, want)
		}
	}
	if !infos[0].IsSystem {
		t.Error("PID 4（System）须标记 IsSystem")
	}
	if !infos[1].IsSelf || infos[0].IsSelf {
		t.Error("IsSelf 须仅命中注入的自身 PID 集合")
	}
	if infos[1].Ports != nil {
		t.Error("不监听端口的进程 Ports 须为空（nil）")
	}
	if len(infos[2].Ports) != 1 || infos[2].Ports[0].Port != 80 || infos[2].Name != "" {
		t.Errorf("tasklist 中缺失的 PID 须补为无名条目并挂上端口: got %+v", infos[2])
	}
	if len(infos[3].Ports) != 2 || infos[3].Ports[0].Port != 36115 || infos[3].Ports[1].Protocol != "UDP" {
		t.Errorf("多端口须按枚举顺序全部挂接: got %+v", infos[3].Ports)
	}
}
