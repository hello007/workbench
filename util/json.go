package util

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LoadJSON 加载JSON文件
func LoadJSON(filePath string, v interface{}) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}

// SaveJSON 保存到JSON文件（原子写：同目录临时文件 → fsync → rename 原子替换目标）。
//
// 目标文件只在 rename 一步被替换，进程崩溃/写入中断不再产生半截 JSON（os.WriteFile
// 截断写的崩溃窗口已消除）；读取方（LoadJSON 精确路径读）也不会读到中间态。
//
// 临时文件用 os.CreateTemp（O_EXCL 排斥并发撞名），命名 .<目标名>.tmp-<随机>（点前缀
// 隐藏、带目标名自描述）；成功 rename 后该名已不存在，defer 删除为 no-op，仅失败路径
// 清理残留。崩溃残留的临时文件为隐藏点文件，不被任何加载方按精确路径读取，无害；
// 不做自动清扫（并发保存同文件时清扫他方在写临时文件会误删，风险大于收益）。
//
// fsync 口径：仅临时文件级 Sync（Windows FlushFileBuffers / Linux fsync），防进程崩溃
// 截断；目录级 fsync 不做——Windows 不支持，且防掉电持久化不在目标内。
//
// Windows 覆盖语义：os.Rename 走 MoveFileEx(MOVEFILE_REPLACE_EXISTING)，对已存在目标
// 为同卷原子替换（TestSaveJSON_OverwriteExisting 锚定）。
func SaveJSON(filePath string, v interface{}) error {
	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(filePath)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// 成功 rename 后目标名已不存在，此删除为 no-op；失败路径清理 temp 残留
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, filePath)
}

// FileExists 检查文件是否存在
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}
