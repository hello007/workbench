package service

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// maxUpdateBinarySize 解压产物（workbench 二进制）大小上限 512MB。
// 不采用 release asset FileSize 作上限：tar.gz 解压后大小与压缩包大小无固定比例
// （压缩包 size 约束不了成员解压后体积），保守常量更可靠；正式产物实测 15MB 量级，
// 该上限仅作解压炸弹防御（超大成员写满磁盘），不影响正常更新。
const maxUpdateBinarySize = 512 << 20

// extractUpdateTarGz 解压 Linux 更新 tar.gz（updateAssetName() 产物，workbench-linux-<GOARCH>.tar.gz），
// 提取包内顶层 workbench
// 二进制（updateBinaryLinux）到 destDir，返回解出二进制的路径。
// 与 release.yml 打包布局对齐：tar 顶层为扁平的 workbench + README-linux.md，其余成员忽略。
// 防御性说明：成员名一律经 filepath.Base 剥离目录成分后与 updateBinaryLinux 精确比对，
// 含路径分隔符（"../evil"、"/abs/path"）或 "." / ".." 的成员名比对必不命中，天然免疫路径穿越；
// 逐项流式解析（tar.Reader）不整包载入内存；写入经 LimitReader 限制成员解压体积上限
// （多读 1 字节探测超限），超限删除半成品并报错，防解压炸弹写满磁盘。
// 资产虽来自自家 GitHub Release，解包侧仍按不信任输入处理（HTTPS 之上再加一层结构校验）。
// 编译说明：无 build tag 无条件编译（纯标准库、跨平台无害），沿用本文件同 buildUpdateSh 的
// 运行时 GOOS 分支惯例——仅 DownloadUpdate 的非 Windows 分支调用，Windows 下载路径不触及。
func extractUpdateTarGz(tarGzPath, destDir string) (string, error) {
	return extractUpdateTarGzWithLimit(tarGzPath, destDir, maxUpdateBinarySize)
}

// extractUpdateTarGzWithLimit extractUpdateTarGz 的上限参数化形态（maxBytes 由测试注入
// 超小值驱动截断分支；生产路径经 extractUpdateTarGz 固定传 maxUpdateBinarySize）。
func extractUpdateTarGzWithLimit(tarGzPath, destDir string, maxBytes int64) (string, error) {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return "", fmt.Errorf("打开更新包失败: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("读取 gzip 流失败: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("解析 tar 失败: %w", err)
		}
		// 仅提取普通文件成员（目录/符号链接等跳过）；Base 剥离目录成分防路径穿越（见函数注释）
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != updateBinaryLinux {
			continue
		}
		outPath := filepath.Join(destDir, updateBinaryLinux)
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			return "", fmt.Errorf("创建二进制文件失败: %w", err)
		}
		// LimitReader 上限多留 1 字节：Copy 后按写入量区分「恰好在限内」与「超限被截断」
		written, err := io.Copy(out, io.LimitReader(tr, maxBytes+1))
		if err != nil {
			out.Close()
			return "", fmt.Errorf("写入二进制文件失败: %w", err)
		}
		if written > maxBytes {
			out.Close()
			os.Remove(outPath)
			return "", fmt.Errorf("更新包内二进制超出大小上限 %dMB，疑似异常包", maxBytes>>20)
		}
		if err := out.Close(); err != nil {
			return "", fmt.Errorf("写入二进制文件失败: %w", err)
		}
		// 显式 0755：tar 头权限经 umask 落盘可能丢失执行位，更新脚本 mv 后需直接可执行
		if err := os.Chmod(outPath, 0755); err != nil {
			return "", fmt.Errorf("设置二进制执行权限失败: %w", err)
		}
		return outPath, nil
	}
	return "", fmt.Errorf("更新包内未找到 %s 二进制", updateBinaryLinux)
}
