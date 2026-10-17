package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"workbench/model"
)

// 文件丢失与重链（SRS §2.6.5，素材管理第九批 PR2）：lost 态记录的手动处置面。
//
// 三入口：
//   - ClearLostAssetFiles「清理丢失记录」：真删除全部 lost 记录（等效 lost 引入
//     前的扫描删除语义，元数据级联清除 + 缩略图记账/PNG 收口）；
//   - RelocateAssetFile「手动重链」：lost（或任意状态）记录指向库内新位置，哈希
//     校验相等直接改路径复活（内容同旧零恢复成本），不等需 force 确认（按新文件
//     重解析，标注保留），记录无哈希（加密/权限跳过）跳过校验重链；
//   - PreviewAssetPathPrefixReplace / ReplaceAssetPathPrefix「根目录前缀替换」：
//     库内子目录重组的批量重链——单事务批量改路径后逐条探测新位置三态收敛
//     （匹配复活 / 已变重解析 / 缺失保持 lost），并自动触发增量扫描全量收敛。
//
// 锁形态：RelocateAssetFile 与 ReplaceAssetPathPrefix 走 tryLockScan 与扫描互斥
// （两者都写 files 表并可能触发单文件重解析，与扫描的差分写面互斥防半态交错；
// 对齐 DeleteLibrary 复用扫描锁先例）。重解析/探测期间的连接安全性由扫描锁保证
// ——DeleteLibrary 以同把锁互斥，连接不会被并发关闭（runLibraryScan 同款模型）。
// ClearLostAssetFiles 走短 withStore 会话（纯删除秒级完成，不与扫描抢锁；并发
// 扫描下两个窗口均无危害：反向——已删行被扫描再标 lost，UPDATE 空命中；正向——
// 收集与事务删除之间行被并发扫描复活 lost→ok，由事务内资格复验跳过不误删，
// 见 clearLostFiles）。

// ClearLostAssetFiles 清理当前库全部 lost 记录：单事务删除（FTS 行显式清除 +
// thumb_cache 显式删除 + files 行删除，slides/file_tags/slide_tags/slide_meta
// 随外键级联），事务提交后清理缩略图记账 dirty 与 PNG 文件（文件系统操作不可
// 回滚，db 先行、文件后删）。返回实际删除的记录数——事务内逐条复验 lost 资格，
// 收集与事务之间被并发扫描复活的行不计入（无 lost 记录幂等返回 0）。
func (s *AssetLibraryService) ClearLostAssetFiles(libraryID string) (int64, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return 0, err
	}
	var deletedIDs []int64
	var removed int64
	// 短会话：收集 + 单事务删除一气呵成（lost 记录量级 = 用户未处置的丢失文件
	// 数，桌面场景千级以内，SQLite 毫秒级完成，无需分批摊薄 s.mu 持有时长）
	err := s.withStore(libraryID, func(store *assetsStore) error {
		ids, n, err := store.clearLostFiles()
		deletedIDs, removed = ids, n
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("清理丢失记录失败: %w", err)
	}
	thumbsDir := s.assetThumbsDir(libraryID)
	for _, fileID := range deletedIDs {
		// 记账 dirty 同步失效（防 flush 复活已删行）；PNG 文件删除失败仅告警
		// 不中断（残留孤儿文件无害，删除结果已提交）
		s.thumbLedger.invalidateFile(libraryID, fileID)
		removeThumbFilesForFile(thumbsDir, fileID)
	}
	if removed > 0 {
		Logger().Info("清理丢失记录完成", "libraryId", libraryID, "removed", removed)
	}
	return removed, nil
}

// RelocateAssetFile 手动重链（D6 必校验哈希 + 无自动匹配）：把文件记录重定向到
// 库内新位置的现存文件。
//   - 越界遏制：newAbsPath 须位于库根目录内（IsAbs + Rel 无 `..` 逃逸，对齐
//     RetryAssetThumb 先例），越界/不存在返回 E_ASSETS_RELINK_OUT_OF_LIB 或可读
//     错误（目标文件由前端文件对话框选择，浏览器通道可达本方法，校验不省）；
//   - 哈希校验（记录 hash 非空时必校验）：相等 → 改 path + 刷新 mtime/size +
//     status=ok 直接复活（内容同旧，解析/缩略图/FTS 全有效；processing 残留行
//     除外——必重处理自愈不变式，落重解析补齐半态）；不等且 force=false
//     → E_ASSETS_RELINK_MISMATCH 确认型拒绝（前端确认后 force=true 重调）；不等
//     且 force=true → 改 path + 单文件重解析（复用 processScanFile：失效缩略图、
//     按分类落终态，标注按位置语义保留）；
//   - 记录 hash 为空（加密/权限跳过文件无哈希）：跳过校验直接重链 + 重解析，
//     返回值 SkippedHashCheck=true 供前端提示「未校验内容」。
//
// 串行保护：tryLockScan 与扫描互斥（重链写 files 表且可能重解析，防与扫描差分
// 写面交错半态）；锁内复验库存在（getLibrary 与拿锁的窗口内删库防孤儿 db，对齐
// runLibraryScan）。取消注册不设（单文件操作秒级完成，无中途取消语义）。
func (s *AssetLibraryService) RelocateAssetFile(libraryID string, fileID int64, newAbsPath string, force bool) (model.AssetRelocateResult, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetRelocateResult{}, err
	}
	if fileID < 1 {
		return model.AssetRelocateResult{}, fmt.Errorf("文件 ID 非法（fileId=%d）", fileID)
	}
	newAbsPath = strings.TrimSpace(newAbsPath)
	if newAbsPath == "" {
		return model.AssetRelocateResult{}, errors.New("新位置路径不能为空")
	}
	newAbs, err := filepath.Abs(newAbsPath)
	if err != nil {
		return model.AssetRelocateResult{}, fmt.Errorf("新位置路径无效: %w", err)
	}
	// 越界遏制（重链库外拒绝）：目标必须在库根目录内，且为已存在的文件
	//（重链语义 = 指向现存文件；不存在时应走前缀替换或等待文件回归）
	newRel, err := assetRelPathWithinRoot(lib.RootPath, newAbs)
	if err != nil {
		return model.AssetRelocateResult{}, err
	}
	info, err := os.Stat(newAbs)
	if err != nil {
		return model.AssetRelocateResult{}, fmt.Errorf("目标文件不存在或无法访问: %w", err)
	}
	if info.IsDir() {
		return model.AssetRelocateResult{}, errors.New("目标路径是目录，须选择 pptx 文件")
	}

	// 串行保护：与扫描互斥后复验库存在，锁内取连接（连接关闭保护与扫描同模型）
	release, err := s.tryLockScan(libraryID)
	if err != nil {
		return model.AssetRelocateResult{}, err
	}
	defer release()
	lib, err = s.getLibrary(libraryID)
	if err != nil {
		return model.AssetRelocateResult{}, err
	}
	store, err := s.storeForScan(libraryID)
	if err != nil {
		return model.AssetRelocateResult{}, err
	}
	rec, err := store.fileRecordByID(fileID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.AssetRelocateResult{}, fmt.Errorf("素材文件不存在或已被删除（fileId=%d），请刷新列表", fileID)
	}
	if err != nil {
		return model.AssetRelocateResult{}, fmt.Errorf("读取文件记录失败: %w", err)
	}
	// 目标路径已被其他记录占用（该路径已有自己的索引行）：重链会造成两条记录
	// 指向同一文件/同一路径的 UNIQUE 冲突，拒绝并给出可读错误（该场景应改为
	// 清理冗余记录或走前缀替换）
	occupied, err := store.pathOwnedByOther(newRel, fileID)
	if err != nil {
		return model.AssetRelocateResult{}, fmt.Errorf("检查目标路径失败: %w", err)
	}
	if occupied {
		return model.AssetRelocateResult{}, fmt.Errorf(
			"目标路径「%s」已有索引记录，不能重链到已入库的路径", newRel)
	}

	result := model.AssetRelocateResult{FileID: fileID, OldPath: rec.path, NewPath: newRel}
	now := time.Now().Format("2006-01-02 15:04:05")

	if rec.hash != "" {
		newHash, err := hashFileSHA256(newAbs)
		if err != nil {
			return result, fmt.Errorf("读取目标文件内容失败: %w", err)
		}
		if newHash != rec.hash {
			if !force {
				// 确认型拒绝（warning 码，前端确认后 force=true 重调；消息说明
				// 强制重链的后果——按新文件重新解析，已有标注保留）
				return result, model.NewAppError(model.ErrCodeAssetsRelinkMismatch,
					"目标文件内容与原记录不一致（哈希校验未通过，可能是不同文件或已被修改）。"+
						"强制重链将按新文件重新解析，已有标签与标注保留。")
			}
		} else {
			result.HashMatched = true
		}
	} else {
		result.SkippedHashCheck = true
	}

	if result.HashMatched && rec.status != model.AssetFileStatusProcessing {
		// 哈希相等：直接改 path + 刷新 mtime/size + 复活 ok（零恢复成本）。
		// processing 残留行排除（「processing 必重处理」扫描自愈不变式，与扫描
		// 增量判定同口径）：上次解析中断的半态行即使内容同旧（哈希相等）也不得
		// 直翻 ok——slides/FTS 可能只落了半份数据，直复活后增量扫描 unchanged
		// 跳过永不纠正，落下方重解析补齐
		if err := store.relocateFileOK(fileID, newRel, info.Size(), info.ModTime().UnixNano(), now); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// 并发窗口：记录已被「清理丢失记录」删除（ClearLostAssetFiles 不持
				// 扫描锁）——UPDATE 空命中零操作，如实报错防成功虚报
				return result, errors.New("记录已被清理，请刷新后重试")
			}
			return result, fmt.Errorf("更新文件记录失败: %w", err)
		}
		Logger().Info("素材文件重链完成（哈希匹配）", "libraryId", libraryID,
			"fileId", fileID, "oldPath", rec.path, "newPath", newRel)
		return result, nil
	}

	// 哈希不等（force 确认）或记录无哈希：改 path + 标记 processing + 单文件
	// 重解析（复用扫描链路：失效缩略图、哈希现算、按解析分类落终态）
	if err := store.relocateFileProcessing(fileID, newRel, now); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// 并发窗口：记录已被「清理丢失记录」删除——如实报错防成功虚报，
			// 且不再进入随后的单文件重解析（幽灵行落 slides 外键违例）
			return result, errors.New("记录已被清理，请刷新后重试")
		}
		return result, fmt.Errorf("更新文件记录失败: %w", err)
	}
	e := pptxDiskEntry{rel: newRel, abs: newAbs, size: info.Size(), mtimeNs: info.ModTime().UnixNano()}
	if _, err := s.processScanFile(store, lib, e, rec, true); err != nil {
		return result, err
	}
	result.Reparsed = true
	Logger().Info("素材文件重链完成（重解析）", "libraryId", libraryID,
		"fileId", fileID, "oldPath", rec.path, "newPath", newRel, "skippedHashCheck", result.SkippedHashCheck)
	return result, nil
}

// assetRelPathWithinRoot 越界遏制单点（重链与前缀替换共用）：absPath 必须为
// 绝对路径且位于 root 内，返回库内相对路径（正斜杠口径，与 files.path 一致）。
// 越界返回 E_ASSETS_RELINK_OUT_OF_LIB（warning，前端引导在库内重新选择）。
func assetRelPathWithinRoot(root, absPath string) (string, error) {
	if !filepath.IsAbs(absPath) {
		return "", model.NewAppError(model.ErrCodeAssetsRelinkOutsideLib,
			"目标路径须为绝对路径且位于素材根目录内")
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", model.NewAppError(model.ErrCodeAssetsRelinkOutsideLib,
			"目标文件位于素材根目录之外，请在库内选择")
	}
	return filepath.ToSlash(rel), nil
}

// normalizeAssetPathPrefix 前缀归一（前缀替换单点）：修剪空白与首尾斜杠、统一
// 正斜杠口径（用户可能按 Windows 习惯输入反斜杠）；拒绝空串与相对逃逸形态
// （. / .. / ../ 开头）。返回可直接与 files.path 比对的库内相对前缀。
func normalizeAssetPathPrefix(raw string) (string, error) {
	p := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	p = strings.Trim(p, "/")
	if p == "" {
		return "", errors.New("路径前缀不能为空（填写库内相对目录，如「2023方案」）")
	}
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("路径前缀非法（%q）：须为素材库内的相对目录", raw)
	}
	return p, nil
}

// PreviewAssetPathPrefixReplace 前缀替换命中数预览（弹窗实时预览，防抖驱动）。
// 命中口径与 ReplaceAssetPathPrefix 同一存储层构造（目录边界语义：path 恰等于
// 旧前缀，或以「旧前缀/」开头，不误伤同头缀目录），新旧前缀均经归一化校验
// （非法前缀返回参数错误，与执行同口径——预览即暴露拼写问题）。
func (s *AssetLibraryService) PreviewAssetPathPrefixReplace(libraryID, oldPrefix, newPrefix string) (int, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return 0, err
	}
	oldNorm, err := normalizeAssetPathPrefix(oldPrefix)
	if err != nil {
		return 0, err
	}
	if _, err := normalizeAssetPathPrefix(newPrefix); err != nil {
		return 0, err
	}
	var n int
	err = s.withStore(libraryID, func(store *assetsStore) error {
		var e error
		n, e = store.countFilesByPathPrefix(oldNorm)
		return e
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// ReplaceAssetPathPrefix 根目录路径前缀批量替换（D7）：旧前缀命中非空校验 →
// 新前缀拼接后逐条越界遏制（任一越界整体拒绝，写面前置零半态）→ 目标路径
// UNIQUE 冲突预检 → 单事务批量 UPDATE path → 事务后逐条探测新位置三态收敛 →
// 自动触发增量扫描全量收敛。
//
// 三态收敛口径（对既有增量语义复用 processScanFile）：
//   - 新位置存在且 mtime+size 与记录一致 → status=ok（内容未变，解析/缩略图
//     仍有效；ok 行保持 ok 仅计路径修正，lost 行计入 Revived；processing 残留
//     行除外——必重处理自愈不变式，落重解析补齐半态）；
//   - 存在但已变 → 复用单文件重解析链路（失效缩略图 + 按分类落终态）；
//   - 不存在 → status=lost（含替换前为 ok 的行——索引现指向的路径无文件，
//     如实标记丢失）。
//
// 锁形态：替换与探测段持 tryLockScan（写 files + 单文件重解析，与扫描互斥）；
// 扫描段在锁释放后经 StartScan 自取锁执行（若与并发扫描冲突返回忙错误，替换
// 本身已提交，前端刷新即可）。
func (s *AssetLibraryService) ReplaceAssetPathPrefix(libraryID, oldPrefix, newPrefix string) (model.AssetPrefixReplaceResult, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetPrefixReplaceResult{}, err
	}
	oldNorm, err := normalizeAssetPathPrefix(oldPrefix)
	if err != nil {
		return model.AssetPrefixReplaceResult{}, err
	}
	newNorm, err := normalizeAssetPathPrefix(newPrefix)
	if err != nil {
		return model.AssetPrefixReplaceResult{}, err
	}
	if oldNorm == newNorm {
		return model.AssetPrefixReplaceResult{}, errors.New("新前缀与旧前缀相同，无需替换")
	}

	result := model.AssetPrefixReplaceResult{
		LibraryID: libraryID,
		OldPrefix: oldNorm,
		NewPrefix: newNorm,
	}
	// 替换与探测段持扫描锁（闭包内自取自放——自动扫描段必须在其后执行，
	// StartScan 以同把锁互斥，锁未放则自撞 E_ASSETS_SCAN_IN_PROGRESS）
	err = func() error {
		release, err := s.tryLockScan(libraryID)
		if err != nil {
			return err
		}
		defer release()
		// 锁内复验库存在（getLibrary 与拿锁窗口内删库防孤儿 db，对齐 runLibraryScan）
		lib, err = s.getLibrary(libraryID)
		if err != nil {
			return err
		}
		store, err := s.storeForScan(libraryID)
		if err != nil {
			return err
		}
		records, err := store.listFilesByPathPrefix(oldNorm)
		if err != nil {
			return fmt.Errorf("读取命中记录失败: %w", err)
		}
		if len(records) == 0 {
			return model.NewAppError(model.ErrCodeAssetsPrefixNoMatch,
				fmt.Sprintf("旧前缀「%s」没有命中的索引记录，请核对前缀拼写", oldNorm))
		}

		// 预检（写面前置，任一失败整体拒绝零半态）：新路径逐条越界遏制 +
		// 与未替换记录的 UNIQUE 冲突检测。替换集自身记录不进冲突表——其旧路径
		// 在本事务内全部腾空；嵌套前缀形态（新前缀位于旧前缀子树内，如
		// 旧目录 → 旧目录/归档）中本集记录的新路径可能撞上另一条本集记录的旧
		// 路径，该路径由对方让出，属合法链式腾位非真冲突（真冲突 = 撞未替换
		// 记录的路径，仍拒）
		renames := make([]assetPathRename, 0, len(records))
		existingPaths := make(map[string]int64)
		allPaths, err := store.listFileRecords()
		if err != nil {
			return fmt.Errorf("读取文件索引失败: %w", err)
		}
		matched := make(map[int64]bool, len(records))
		for _, rec := range records {
			matched[rec.id] = true
		}
		for _, rec := range allPaths {
			if matched[rec.id] {
				continue
			}
			existingPaths[rec.path] = rec.id
		}
		for _, rec := range records {
			// 后缀（含前导 /）原样保留：old/a.pptx → new/a.pptx；
			// 恰等于前缀的行（目录行形态，理论不出现）后缀为空串
			suffix := strings.TrimPrefix(strings.TrimPrefix(rec.path, oldNorm), "/")
			newRel := newNorm
			if suffix != "" {
				newRel = newNorm + "/" + suffix
			}
			if _, err := assetRelPathWithinRoot(lib.RootPath, filepath.Join(lib.RootPath, filepath.FromSlash(newRel))); err != nil {
				return fmt.Errorf("新路径「%s」越界: %w", newRel, err)
			}
			if _, ok := existingPaths[newRel]; ok {
				return fmt.Errorf("新路径「%s」与现有索引记录冲突，请调整新前缀后重试", newRel)
			}
			renames = append(renames, assetPathRename{fileID: rec.id, oldPath: rec.path, newRel: newRel})
		}
		// 执行序：旧路径长者先落库（嵌套前缀的链式腾位——碰撞链中占位方的旧
		// 路径 = 被占位方的新路径 = 其旧路径 + 中段子路径，恒长于后者，故长度
		// 降序即安全序：深层记录先让出目标槽位，浅层记录后落位），防事务内
		// 瞬态 UNIQUE 撞击整体回滚
		sort.SliceStable(renames, func(i, j int) bool {
			return len(renames[i].oldPath) > len(renames[j].oldPath)
		})

		// 单事务批量改路径（预检后写入，任一失败整体回滚）
		now := time.Now().Format("2006-01-02 15:04:05")
		if err := store.replaceFilePaths(renames, now); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// 并发窗口：命中集行在收集与事务之间被「清理丢失记录」删除
				//（ClearLostAssetFiles 不持扫描锁）——事务已整体回滚，如实报错
				// 防 Replaced 虚报，探测段不再进入
				return errors.New("命中记录已被清理，请刷新后重试")
			}
			return fmt.Errorf("批量更新路径失败: %w", err)
		}
		result.Replaced = int64(len(renames))

		// 事务后逐条探测新位置三态收敛（替换的记录按原记录快照判定：
		// id/size/mtime 为旧路径时代的事实，新位置 stat 与之比对）。
		// 配对不变式：renames 已按旧路径长度降序重排（嵌套腾位安全序，见上），
		// 与 records 原序（path 升序，listFilesByPathPrefix）不同序——必须经
		// recByID 按 fileID 取记录快照遍历 renames，禁按下标 zip 配对
		// （records[i] ↔ renames[i] 在混合长度命中集下配错记录：重解析/FTS/
		// lost 标记写到别的记录行。TestAssetPrefixReplace_MixedLengthPairing
		// 守护——「归档」UTF-8 字节序巧合曾掩蔽该错位）
		recByID := make(map[int64]assetFileRecord, len(records))
		for _, rec := range records {
			recByID[rec.id] = rec
		}
		for _, rn := range renames {
			rec, ok := recByID[rn.fileID]
			if !ok {
				// 结构性不可达（renames 与 records 同源于本轮命中集）；防御未来
				// 来源变更退化为零值静默错写——零值 rec 的 size/mtime/status 全零
				// 会让下方探测三态判定全错
				return fmt.Errorf("内部错误：替换记录快照缺失（fileId=%d）", rn.fileID)
			}
			newRel := rn.newRel
			newAbs := filepath.Join(lib.RootPath, filepath.FromSlash(newRel))
			info, statErr := os.Stat(newAbs)
			if statErr != nil {
				// 新位置不存在：保持/标记 lost（含替换前 ok 的行——如实标记）
				if _, err := store.markFileLost(rec.id, now); err != nil {
					return fmt.Errorf("标记丢失记录失败（%s）: %w", newRel, err)
				}
				result.StillLost++
				continue
			}
			size := info.Size()
			mtimeNs := info.ModTime().UnixNano()
			// processing 残留行排除（「processing 必重处理」扫描自愈不变式）：
			// mtime+size+hasSlides 恰好满足也不得直翻 ok，落重解析补齐半态
			if size == rec.size && mtimeNs == rec.mtimeNs && rec.status != model.AssetFileStatusProcessing {
				// 匹配：仅曾成功解析（有幻灯片产物）的行翻状态恢复——无产物行
				// （failed/encrypted/denied 跳过类或 0 页空演示文稿）走下方重解析
				// 按实际内容重新分类，防坏文件直复活为 ok（资格门槛单点
				// fileHasSlides，与扫描 lost 复活共用）
				hasSlides, err := store.fileHasSlides(rec.id)
				if err != nil {
					return fmt.Errorf("检查解析产物失败（%s）: %w", newRel, err)
				}
				if hasSlides {
					if err := store.reviveLostFile(rec.id, now); err != nil {
						return fmt.Errorf("更新记录状态失败（%s）: %w", newRel, err)
					}
					if rec.status == model.AssetFileStatusLost {
						result.Revived++
					}
					continue
				}
				// 无产物：不 continue，走下方重解析
			}
			// 已变：复用单文件重解析链路（markProcessing + 失效缩略图 + 按分类
			// 落终态；标注按位置语义保留）
			e := pptxDiskEntry{rel: newRel, abs: newAbs, size: size, mtimeNs: mtimeNs}
			if _, err := s.processScanFile(store, lib, e, rec, true); err != nil {
				return err
			}
			result.Reparsed++
		}
		return nil
	}()
	if err != nil {
		return result, err
	}
	Logger().Info("路径前缀替换完成", "libraryId", libraryID,
		"oldPrefix", oldNorm, "newPrefix", newNorm, "replaced", result.Replaced,
		"revived", result.Revived, "reparsed", result.Reparsed, "stillLost", result.StillLost)

	// 自动触发增量扫描全量收敛（锁已释放，StartScan 自取锁）：新位置三态之外的
	// 变化面（旧位置残留文件、替换波及外的新增/删除）一并收敛。扫描失败不掩盖
	// 替换结果——替换已提交，错误随整体调用上抛由前端刷新兜底
	scan, scanErr := s.StartScan(context.Background(), libraryID)
	if scanErr != nil {
		Logger().Warn("前缀替换后的自动扫描失败", "libraryId", libraryID, "err", scanErr)
		return result, scanErr
	}
	result.Scan = scan
	return result, nil
}
