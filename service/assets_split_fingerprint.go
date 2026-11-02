package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 拆分产物页级指纹（schema v10，任务 11-02-artifact-page-fingerprint）。
//
// 背景：旧产物清理原先按「记录主干 + 页序 + mtime > split_at + 2s」启发式判定
// 归属——用户自建的同主干同页序文件若 mtime 早于 split_at（复制、解压、手工改
// 时间）会被删除且不可恢复。指纹把归属从启发式变成可验证事实：清理时重算候选
// 文件 SHA-256，与记录中的基线对比，不同内容的文件无论 mtime 一律保留。
//
// 契约（ADR-lite §6/§8/§9）：
//   - 每页 {page, name, size, sha256}；name 只存 basename（目录由 output_dir 提供），
//     重链/目录迁移不重写指纹（产物文件名不变）；
//   - 失效方向恒为「漏清不误删」：指纹缺失/损坏时回落 mtime 守卫；有指纹但某页
//     无条目时该页保留（不回落，避免把用户文件按旧启发式删掉）；
//   - 指纹只由成功拆分写入，禁止自动回填（现有目录内容可能已被用户替换）。

// splitArtifactFingerprint 单页产物指纹（JSON 列 artifact_fingerprints 的元素）。
type splitArtifactFingerprint struct {
	Page   int    `json:"page"`
	Name   string `json:"name"`   // 拆分时产物文件名（basename）；重链/目录迁移不重写
	Size   int64  `json:"size"`   // 字节数，快速预筛
	SHA256 string `json:"sha256"` // 全文小写 hex
}

// encodeSplitFingerprints 指纹清单序列化为 JSON 列值（空清单写空串 = 无指纹）。
// 序列化失败按空串降级并 Warn（结构体只含基础类型，理论不可达）——失效方向为
// 回落 mtime 守卫，不阻断已成功的拆分。
func encodeSplitFingerprints(fps []splitArtifactFingerprint) string {
	if len(fps) == 0 {
		return ""
	}
	raw, err := json.Marshal(fps)
	if err != nil {
		Logger().Warn("序列化拆分产物指纹失败，按无指纹写入（清理回落 mtime 守卫）", "err", err)
		return ""
	}
	return string(raw)
}

// parseSplitFingerprintsJSON 反序列化指纹列。空串返回 nil（= 无指纹）；解析失败
// 同样返回 nil 并 Warn，不返回错误——保持查询宽容语义（读记录不因脏 JSON 失败，
// 清理侧回落 mtime 守卫即可）。
func parseSplitFingerprintsJSON(raw, relPath string) []splitArtifactFingerprint {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var fps []splitArtifactFingerprint
	if err := json.Unmarshal([]byte(raw), &fps); err != nil {
		Logger().Warn("拆分记录产物指纹解析失败，按无指纹处理（清理回落 mtime 守卫）",
			"sourceRelPath", relPath, "err", err)
		return nil
	}
	return fps
}

// buildSplitFingerprints 计算产物页级指纹（拆分成功后调用）：逐文件取 basename、
// 字节数与全文 SHA-256，按页序升序输出。paths 与 pages 一一对应（pages 由调用方
// 从产物名解析页序，解析失败回退结果下标 + 1）。任一文件 Stat/哈希失败返回错误，
// 由调用方降级为无指纹（不回滚已成功的拆分）。
func buildSplitFingerprints(paths []string, pages []int) ([]splitArtifactFingerprint, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if len(pages) != len(paths) {
		return nil, fmt.Errorf("产物页序清单与路径数量不一致（%d != %d）", len(pages), len(paths))
	}
	fps := make([]splitArtifactFingerprint, 0, len(paths))
	for i, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("读取拆分产物状态失败（%s）: %w", p, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("拆分产物不是常规文件（%s）", p)
		}
		sum, err := hashFileSHA256(p)
		if err != nil {
			return nil, fmt.Errorf("计算拆分产物哈希失败（%s）: %w", p, err)
		}
		fps = append(fps, splitArtifactFingerprint{
			Page:   pages[i],
			Name:   filepath.Base(p),
			Size:   info.Size(),
			SHA256: sum,
		})
	}
	sort.SliceStable(fps, func(i, j int) bool { return fps[i].Page < fps[j].Page })
	return fps, nil
}

// fingerprintsUsable 指纹整体是否可用于清理判定：记录存在、页数有效、指纹非空
// 且条目数与 page_count 一致。空串 / JSON 解析失败 / 条目数不符（视为损坏）一律
// false，调用方整体回落 mtime 守卫。
func fingerprintsUsable(record *assetSplitRecord) bool {
	return record != nil && record.pageCount > 0 &&
		len(record.fingerprints) > 0 && len(record.fingerprints) == record.pageCount
}

// splitFingerprintNameValid 指纹中的文件名是否为纯 basename（防脏数据路径穿越）：
// 拒绝空串、`.`/`..`、任何目录分隔符（`/` 与 `\`，跨平台双侧检查）与非 basename
// 形态。库内产物文件名恒为 basename，非法值只可能来自手工改库。
func splitFingerprintNameValid(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	return filepath.Base(name) == name
}

// fingerprintMatch 候选文件是否与指纹一致（size + 全文 SHA-256）：要求常规文件
// （符号链接/目录/设备一律不命中）。name 由调用方定位时保证，不在此重复比较。
func fingerprintMatch(path string, fp splitArtifactFingerprint) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if info.Size() != fp.Size {
		return false
	}
	sum, err := hashFileSHA256(path)
	if err != nil {
		return false
	}
	return strings.EqualFold(sum, fp.SHA256)
}

// fingerprintForPage 按页序查指纹；该页无条目返回 false（调用方按「不是本记录
// 产物，保留」处理，不回落 mtime——见 ADR §9）。
func fingerprintForPage(record *assetSplitRecord, page int) (splitArtifactFingerprint, bool) {
	if record == nil {
		return splitArtifactFingerprint{}, false
	}
	for _, fp := range record.fingerprints {
		if fp.Page == page {
			return fp, true
		}
	}
	return splitArtifactFingerprint{}, false
}

// fingerprintForName 按文件名查指纹（经 sameNameFold：仅 Windows 折叠大小写）。
// 无命中或多命中（大小写敏感文件系统上并存的大小写变体）视为不可判定返回 false
// ——调用方按用户文件保留，失效方向为漏清而非误删。
func fingerprintForName(record *assetSplitRecord, name string) (splitArtifactFingerprint, bool) {
	if record == nil {
		return splitArtifactFingerprint{}, false
	}
	var hit splitArtifactFingerprint
	hits := 0
	for _, fp := range record.fingerprints {
		if !sameNameFold(fp.Name, name) {
			continue
		}
		hit = fp
		hits++
	}
	if hits != 1 {
		return splitArtifactFingerprint{}, false
	}
	return hit, true
}
