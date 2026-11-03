package util

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

// backupDigestLen 备份文件名中保留的内容摘要字符数（SHA-256 十六进制前 12 位）。
// 48 bit 摘要对「同目录下少数几个损坏留底文件」的碰撞概率可忽略，同时保持
// 文件名可读、可目视比对。
const backupDigestLen = 12

// BackupCorruptFile 将读取或解析失败的原文件内容留底到同目录备份文件，
// 备份名为 <文件路径>.bak.<内容 SHA-256 前 12 位 hex>。
//
// 以内容摘要而非时间戳命名：同一份损坏内容反复读取只保留一份备份（避免备份
// 随读取次数无上限增涨），不同内容各留一份；内容相同即使覆盖同名文件也不改变
// 留底结果，因此重复调用是幂等的。空 raw 同样正常留底（得到空文件的固定摘要）。
//
// 返回备份路径；写入失败返回错误，由调用方按 best effort 处理（备份失败不得
// 阻断损坏降级主流程）。
func BackupCorruptFile(filePath string, raw []byte) (string, error) {
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])[:backupDigestLen]
	bakPath := filePath + ".bak." + digest
	if err := os.WriteFile(bakPath, raw, 0o644); err != nil {
		return bakPath, err
	}
	return bakPath, nil
}
