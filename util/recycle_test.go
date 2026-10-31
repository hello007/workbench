// util/recycle_test.go
package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMoveToRecycleBin_Prechecks 公共前置校验：空路径 / 相对路径 / 不存在路径
// 三类拒绝。真实回收站调用交集成/手测（CI 不移动真实文件，见
// docs/spec/test-stability.md「禁隐式依赖环境外部状态」）。
func TestMoveToRecycleBin_Prechecks(t *testing.T) {
	t.Run("空路径拒绝", func(t *testing.T) {
		err := MoveToRecycleBin("   ")
		if err == nil || !strings.Contains(err.Error(), "路径为空") {
			t.Errorf("空路径应拒绝并提示路径为空，实际：%v", err)
		}
	})

	t.Run("相对路径拒绝", func(t *testing.T) {
		err := MoveToRecycleBin(filepath.Join("相对目录", "a.pptx"))
		if err == nil || !strings.Contains(err.Error(), "绝对路径") {
			t.Errorf("相对路径应拒绝并提示须为绝对路径，实际：%v", err)
		}
	})

	t.Run("不存在路径拒绝", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "不存在.pptx")
		err := MoveToRecycleBin(missing)
		if err == nil || !strings.Contains(err.Error(), "不存在或不可访问") {
			t.Errorf("不存在路径应拒绝并提示不可访问，实际：%v", err)
		}
		if _, statErr := os.Lstat(missing); !os.IsNotExist(statErr) {
			t.Errorf("前置校验不得创建文件，实际 stat err=%v", statErr)
		}
	})
}
