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
