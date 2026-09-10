package model

import (
	"testing"
)

func TestNewDirectory(t *testing.T) {
	dir := NewDirectory("测试", "C:\\test", true)

	if dir.Name != "测试" {
		t.Errorf("期望名称为 '测试', 实际为 '%s'", dir.Name)
	}

	if !dir.IsDefault {
		t.Error("期望 IsDefault 为 true")
	}
}

func TestDirectoryValidate(t *testing.T) {
	dir := &Directory{Name: "", Path: ""}
	err := dir.Validate()
	if err == nil {
		t.Error("期望验证失败")
	}
}

func TestNewFileTreeNode(t *testing.T) {
	node := NewFileTreeNode("test.txt", "C:\\test.txt", "file")

	if node.Type != "file" {
		t.Errorf("期望类型为 'file', 实际为 '%s'", node.Type)
	}

	if !node.IsLeaf {
		t.Error("文件节点 IsLeaf 应为 true")
	}
}

func TestGitCommitShortHash(t *testing.T) {
	commit := &GitCommit{Hash: "abc1234567890"}
	shortHash := commit.ShortHash()

	if shortHash != "abc1234" {
		t.Errorf("期望短哈希为 'abc1234', 实际为 '%s'", shortHash)
	}
}

func TestNewPageResult(t *testing.T) {
	records := []int{1, 2, 3}
	result := NewPageResult(records, 25, 2, 10)

	if result.Total != 25 {
		t.Errorf("期望 Total 为 25, 实际为 %d", result.Total)
	}

	if result.Pages != 3 {
		t.Errorf("期望 Pages 为 3, 实际为 %d", result.Pages)
	}
}

// TestDirectoryValidate_Branches 覆盖 Validate 各分支：缺名、缺路径、两者齐备。
func TestDirectoryValidate_Branches(t *testing.T) {
	if err := (&Directory{Name: "有名字", Path: ""}).Validate(); err == nil {
		t.Error("缺路径应返回错误")
	}
	if err := (&Directory{Name: "", Path: "C:\\x"}).Validate(); err == nil {
		t.Error("缺名称应返回错误")
	}
	if err := (&Directory{Name: "名", Path: "C:\\x"}).Validate(); err != nil {
		t.Errorf("名称路径齐备应通过校验, got %v", err)
	}
}

// TestGitCommitShortHash_Short 哈希长度不足 7 时原样返回。
func TestGitCommitShortHash_Short(t *testing.T) {
	if got := (&GitCommit{Hash: "abc"}).ShortHash(); got != "abc" {
		t.Errorf("短哈希期望 abc, 实际 %s", got)
	}
	if got := (&GitCommit{Hash: ""}).ShortHash(); got != "" {
		t.Errorf("空哈希期望空串, 实际 %s", got)
	}
}

// TestNewPageResult_ExactDivide 总数整除页大小时不应多加一页。
func TestNewPageResult_ExactDivide(t *testing.T) {
	result := NewPageResult([]int{1}, 20, 1, 10)
	if result.Pages != 2 {
		t.Errorf("整除边界: 期望 Pages=2, 实际 %d", result.Pages)
	}
	result = NewPageResult([]int{1}, 10, 1, 10)
	if result.Pages != 1 {
		t.Errorf("正好一页: 期望 Pages=1, 实际 %d", result.Pages)
	}
}
