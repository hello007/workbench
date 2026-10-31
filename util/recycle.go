// util/recycle.go
package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MoveToRecycleBin 把文件移入操作系统回收站（可恢复的非永久删除）。
//
// 使用场景：素材拆分「就地替换」落点在全页拆分成功、逐页产物校验通过、标注
// 迁移完成后，把源 pptx 移入回收站（PRD D2：用户可恢复，误拆不丢原文件）。
//
// 跨平台实现按 build tag 拆分（docs/spec/linux-platform.md §2：同签名不同实现
// 用双文件）：
//   - util/recycle_windows.go：SHFileOperationW + FOF_ALLOWUNDO（回收站）；
//   - util/recycle_other.go：freedesktop Trash 规范（$XDG_DATA_HOME/Trash）。
//
// 公共前置校验在此完成（空路径 / 非绝对路径 / 目标不存在），平台函数只承担
// 真实移动动作——前置校验两平台同测，平台实现差异面收敛到最小。
func MoveToRecycleBin(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("无法移入回收站：文件路径为空")
	}
	// 仅接受绝对路径：相对路径在不同工作目录下含义漂移，且回收站 API 需要
	// 完整路径才能定位文件
	if !filepath.IsAbs(path) {
		return fmt.Errorf("无法移入回收站：路径须为绝对路径（%s）", path)
	}
	// Lstat 而非 Stat：符号链接本身也可移入回收站，不跟随目标
	if _, err := os.Lstat(path); err != nil {
		return fmt.Errorf("无法移入回收站：文件不存在或不可访问: %w", err)
	}
	return moveToRecycleBinPlatform(path)
}
