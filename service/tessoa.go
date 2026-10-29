package service

import (
	"os"
	"os/exec"
)

// resolveTessoaExecutable 解析 tessoa（https://tessoa.cn/ ，跨平台文件管理器）
// 可执行文件路径，供「打开资源管理器」优先经 tessoa 打开；不可用返回空串由调用方
// 降级系统文件管理器。解析顺序：
//  1. 设置配置路径非空且文件存在：采用配置路径（覆盖免安装未进 PATH 的场景）；
//  2. 否则（含配置为空、配置路径已失效）：exec.LookPath("tessoa") 搜索 PATH
//     （Windows 自动尝试 .exe 等后缀）。
func resolveTessoaExecutable(configured string) string {
	if configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured
		}
	}
	if path, err := exec.LookPath("tessoa"); err == nil {
		return path
	}
	return ""
}
