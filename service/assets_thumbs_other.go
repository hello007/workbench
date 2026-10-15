//go:build !windows

// 素材缩略图链路非 Windows 桩（linux-platform 契约：编译期字段独有走
// _windows/_other 双文件分叉）。PowerPoint COM 仅存在于 Windows（SRS §1.4.2），
// 非 Windows 平台缩略图/打开跳转整体降级（D4：前端提示条 + 入口隐藏，
// 后端防御返回 errThumbsUnavailable）。go-ole 仅被 _windows 文件引用，
// 本文件保证 GOOS=linux 编译零影响。

package service

import "context"

// unavailableThumbExporter 无 PowerPoint 能力桩：Available 恒 false 驱动
// 全链路降级（入队静默跳过、跳转返回 errThumbsUnavailable）。
type unavailableThumbExporter struct{}

// newThumbExporter 平台分叉构造点（无 tag 侧 NewAssetLibraryService 调用）。
// ownedPersistPath 与 Windows 侧同签名（自启 PID 持久化文件路径，D7）；桩无
// COM 会话恒不产生 owned 实例，参数忽略。
func newThumbExporter(_ string) ThumbExporter {
	return unavailableThumbExporter{}
}

func (unavailableThumbExporter) Available() bool {
	return false
}

func (unavailableThumbExporter) ExportPresentation(_ context.Context, _ thumbFileSpec, _ string, _ []thumbTier, _ func() *thumbJumpRequest) (int, error) {
	return 0, errThumbsUnavailable
}

func (unavailableThumbExporter) JumpOnly(jr *thumbJumpRequest) error {
	jr.Result <- errThumbsUnavailable
	return errThumbsUnavailable
}

func (unavailableThumbExporter) Discard() {}

func (unavailableThumbExporter) Shutdown() {}
