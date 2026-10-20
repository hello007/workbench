package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"workbench/model"
)

// 素材库文件夹树与文件夹管理（PRD「素材管理：文件夹树与拖拽整理」PR1/PR2）。
//
// ListFolders 读库根目录的真实子目录树（D1：树 = 真实文件系统目录，非索引派生
// ——空目录可见、与目录管理操作同源）。排除规则复用扫描侧 ExcludeDirs matcher
// 单点（assets_exclude.go，禁两套口径）：目录段命中置 Excluded=true 且整棵子树
// 继承标记（对齐扫描 SkipDir 剪除整棵子树的语义）；库根自身豁免匹配。根目录
// 不存在（磁盘被删/移动）与扫描行为对齐：collectPptxEntries 对不可入目录告警
// 降级继续，根不可入即空清单——本链路同样返回空树非错误，不阻塞面板渲染。
//
// 管理操作三入口（PR2）：
//   - CreateAssetFolder 新建子目录：纯磁盘 mkdir，不触碰索引（新空目录无索引面，
//     下次扫描增量收敛）；
//   - DeleteAssetFolder 删除目录：磁盘递归删除 + 索引内该前缀文件行 markFileLost
//     收口（文件物理已删，标注/缩略图/FTS 保留待复活语义对齐 §11.2）+ 拆分记录
//     按前缀清理；
//   - RenameAssetFolder 同父重命名 = 目录级移动：磁盘 rename + 索引路径前缀批量
//     更新（复用前缀替换单点机制 assetPathRenamesForPrefix/replaceFilePaths，禁
//     复制第二份）+ 受影响文件重过排除规则双向翻转。
//
// 锁形态：Create 纯磁盘不触碰索引免锁；Delete/Rename 写 files/split_records 表，
// 持 tryLockScan 与扫描互斥（对齐 ReplaceAssetPathPrefix/RelocateAssetFile 先例：
// 锁内复验库存在，防 getLibrary 与拿锁窗口内删库重建孤儿 db）。
// FTS 说明：FTS 检索列不含路径（file_name 为标题基名派生，见 assets_db.go
// syncFtsFileRowTx），纯路径重命名无需重建 FTS 行（rowid = files.id/slides.id
// 稳定）；状态翻转必须经 markFileExcluded（excluded 无损保留 FTS 行）与
// reviveLostFile（单事务含 syncFtsFileRowTx 重建）两个既有单点，禁裸 UPDATE 翻
// 状态——§12.2 excluded/lost 教训（非 ok 期间编辑过备注的行 FTS 行已清，裸翻
// 状态会关键词检索永久漏）。

// ListFolders 库根目录真实子目录树（文件夹树数据源）。库不存在报
// errAssetLibraryNotFound（E_ASSETS_LIB_NOT_FOUND，前端自愈重拉库列表，与既有
// 素材查询链同口径）；库根目录不存在返回空树非错误（对齐扫描的降级语义）。
// 纯只读：不触碰索引库与磁盘内容。
func (s *AssetLibraryService) ListFolders(libraryID string) ([]model.AssetFolderNode, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return nil, err
	}
	// 与扫描同源编译 matcher（收集层磁盘过滤与 excluded 判定共用，禁第二口径）
	matcher := newAssetExcludeMatcher(lib.ExcludeDirs, lib.ExcludeExts)
	return collectFolderTree(lib.RootPath, matcher), nil
}

// collectFolderTree 读库根目录真实子目录树：逐个一级子目录递归装配（os.ReadDir
// 按文件名字典序返回，节点顺序即文件系统字典序，PRD D8 同款目录序约定）。库根
// 自身不入树（前端固定「全部素材」根节点）；根不可读告警降级返回空树。
func collectFolderTree(root string, matcher *assetExcludeMatcher) []model.AssetFolderNode {
	dirEntries, err := os.ReadDir(root)
	if err != nil {
		// 根不可入（权限/被删/移动）：告警降级空树，对齐扫描 collectPptxEntries
		// 的根降级语义，不阻塞面板渲染
		Logger().Warn("assets folder tree entry inaccessible", "path", root, "err", err)
		return []model.AssetFolderNode{}
	}
	nodes := make([]model.AssetFolderNode, 0, len(dirEntries))
	for _, entry := range dirEntries {
		if !entry.IsDir() {
			continue
		}
		if node, ok := buildFolderNode(filepath.Join(root, entry.Name()), entry.Name(), entry.Name(), matcher, false); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// buildFolderNode 递归装配单个目录节点（relDir = 该目录相对库根的正斜杠路径，
// 一级目录与目录基名相同）。inheritedExcluded = 父链排除标记（整棵子树继承，
// 对齐扫描 SkipDir 剪除整棵子树语义）。同一遍 ReadDir 完成：子目录递归、直属
// PPT 计数（磁盘口径：.pptx 大小写不敏感 − 后缀规则命中 − 拆分派生副本，与扫描
// 收集口径同源）与派生容器标记（直属文件存在拆分派生副本）。目录不可入（权限/
// 中途消失）告警降级 ok=false——该子树从树中裁剪但其余目录照常收集，不中断。
func buildFolderNode(absDir, relDir, name string, matcher *assetExcludeMatcher, inheritedExcluded bool) (model.AssetFolderNode, bool) {
	dirEntries, err := os.ReadDir(absDir)
	if err != nil {
		Logger().Warn("assets folder tree entry inaccessible", "path", absDir, "err", err)
		return model.AssetFolderNode{}, false
	}
	node := model.AssetFolderNode{
		Path:     relDir,
		Name:     name,
		Children: []model.AssetFolderNode{},
		Excluded: inheritedExcluded || matcher.matchDir(name),
	}
	for _, entry := range dirEntries {
		if entry.IsDir() {
			childRel := entry.Name()
			if relDir != "" {
				childRel = relDir + "/" + entry.Name()
			}
			if child, ok := buildFolderNode(filepath.Join(absDir, entry.Name()), childRel, entry.Name(), matcher, node.Excluded); ok {
				node.Children = append(node.Children, child)
			}
			continue
		}
		relFile := entry.Name()
		if relDir != "" {
			relFile = relDir + "/" + entry.Name()
		}
		// 拆分派生副本：标记容器（派生产物由拆分链路管理）且不入计数（口径与
		// 扫描收集同源——派生副本不入索引，徽标不得计入）
		if isPptxSplitDerivative(relFile) {
			node.Derived = true
			continue
		}
		ext := filepath.Ext(entry.Name())
		if !strings.EqualFold(ext, ".pptx") {
			continue
		}
		if matcher.matchExt(ext) {
			continue
		}
		node.FileCount++
	}
	return node, true
}

// CreateAssetFolder 新建子目录（文件夹管理入口一，PRD R3）：父目录磁盘 mkdir。
// 父目录不存在/父为排除命中目录或拆分派生容器拒绝；重名（同名目录或文件已存在）
// 以 os.ErrExist 口径明确报错。纯磁盘操作不触碰索引（新空目录无索引面，下次
// 扫描增量收敛），免扫描锁。返回新建节点（树局部刷新回显用）。
func (s *AssetLibraryService) CreateAssetFolder(libraryID, parentRelPath, name string) (model.AssetFolderNode, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetFolderNode{}, err
	}
	clean, err := normalizeAssetFolderName(name)
	if err != nil {
		return model.AssetFolderNode{}, err
	}
	parentRel := ""
	if strings.TrimSpace(parentRelPath) != "" {
		parentRel, _, err = resolveLibraryRelPath(lib.RootPath, parentRelPath)
		if err != nil {
			return model.AssetFolderNode{}, fmt.Errorf("父目录路径非法: %w", err)
		}
	}
	matcher := newAssetExcludeMatcher(lib.ExcludeDirs, lib.ExcludeExts)
	parentAbs := lib.RootPath
	if parentRel != "" {
		// 父目录管理守卫（库根父目录豁免——root 不参与排除匹配与派生判定）
		if err := refuseUnmanageableFolderByRule(matcher, parentRel); err != nil {
			return model.AssetFolderNode{}, err
		}
		parentAbs = filepath.Join(lib.RootPath, filepath.FromSlash(parentRel))
		if dirContainsSplitDerivative(parentAbs, parentRel) {
			return model.AssetFolderNode{}, fmt.Errorf("目录「%s」内含 PPT 拆分派生产物，由拆分链路管理，不能在其中新建子目录", parentRel)
		}
		info, err := os.Stat(parentAbs)
		if err != nil {
			return model.AssetFolderNode{}, fmt.Errorf("父目录不存在或无法访问: %w", err)
		}
		if !info.IsDir() {
			return model.AssetFolderNode{}, errors.New("父路径不是目录")
		}
	}
	targetAbs := filepath.Join(parentAbs, clean)
	if err := os.Mkdir(targetAbs, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return model.AssetFolderNode{}, fmt.Errorf("同名目录或文件「%s」已存在", clean)
		}
		if errors.Is(err, os.ErrNotExist) {
			return model.AssetFolderNode{}, errors.New("父目录不存在（可能已被删除或移动），请刷新后重试")
		}
		return model.AssetFolderNode{}, fmt.Errorf("创建目录失败: %w", err)
	}
	newRel := clean
	if parentRel != "" {
		newRel = parentRel + "/" + clean
	}
	Logger().Info("素材文件夹已创建", "libraryId", libraryID, "path", newRel)
	return model.AssetFolderNode{Path: newRel, Name: clean, Children: []model.AssetFolderNode{}}, nil
}

// DeleteAssetFolder 删除目录（文件夹管理入口二，PRD R3）：磁盘递归删除 +
// 索引收口。force=false 时非空目录（含任意子目录或文件，非仅 PPT）以
// E_ASSETS_FOLDER_NOT_EMPTY 确认型拒绝（删除未发生，前端二次确认后以 force=true
// 重调）；force=true 递归删除。排除命中目录与拆分派生容器拒绝（宁严勿错）。
//
// 索引收口（磁盘删除成功后）：该前缀下文件行 markFileLost（物理已删，size/mtime/
// hash 原值保留 + 标注/缩略图/FTS 全保留，复活/重链语义对齐 §11.2；excluded 行
// 一并翻 lost——用户显式删除目录后「规则排除」语义失效，lost 如实反映文件已消失）
// + split_records 按前缀清理（源文件已删，记录保留会以陈旧哈希误判复拆增量跳过）。
// 收口失败仅告警不回滚（磁盘删除已发生不可逆；下次扫描增量三态自愈——消失文件
// 自动标 lost，两道收口互为兜底）。返回标记 lost 的索引行数（前端 toast 口径）。
func (s *AssetLibraryService) DeleteAssetFolder(libraryID, relPath string, force bool) (int64, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return 0, err
	}
	rel, abs, err := resolveLibraryRelPath(lib.RootPath, relPath)
	if err != nil {
		return 0, fmt.Errorf("目录路径非法: %w", err)
	}
	matcher := newAssetExcludeMatcher(lib.ExcludeDirs, lib.ExcludeExts)
	if err := refuseUnmanageableFolderByRule(matcher, rel); err != nil {
		return 0, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return 0, fmt.Errorf("目录不存在或无法访问: %w", err)
	}
	if !info.IsDir() {
		return 0, errors.New("路径不是目录")
	}
	if dirContainsSplitDerivative(abs, rel) {
		return 0, fmt.Errorf("目录「%s」内含 PPT 拆分派生产物，由拆分链路管理，不能删除", rel)
	}
	// 非空预检（force 确认面）：磁盘条目计数（含子目录与非 pptx 文件——前端
	// countMap 只有 PPT 数，无法可靠判定「非空」，以磁盘事实为准）。预检与锁内
	// 删除之间的目录变化窗口由锁内复验后的实际删除兜底（RemoveAll 幂等）。
	diskEntries, err := os.ReadDir(abs)
	if err != nil {
		return 0, fmt.Errorf("读取目录失败: %w", err)
	}
	if len(diskEntries) > 0 && !force {
		return 0, model.NewAppError(model.ErrCodeAssetsFolderNotEmpty,
			fmt.Sprintf("目录「%s」不是空目录（含 %d 个条目），确定要连同内容一起删除吗？", rel, len(diskEntries)))
	}

	// 串行保护：删除写 files/split_records 表，与扫描互斥（锁内复验库存在，对齐
	// ReplaceAssetPathPrefix 先例）
	release, err := s.tryLockScan(libraryID)
	if err != nil {
		return 0, err
	}
	defer release()
	lib, err = s.getLibrary(libraryID)
	if err != nil {
		return 0, err
	}
	store, err := s.storeForScan(libraryID)
	if err != nil {
		return 0, err
	}

	// 磁盘删除先行（物理事实；失败即整体拒绝，索引未动零半态）
	if err := os.RemoveAll(abs); err != nil {
		return 0, fmt.Errorf("删除目录失败: %w", err)
	}

	// 索引收口（物理删除已发生，不可逆；收口失败仅告警，下次扫描自愈兜底）
	now := time.Now().Format("2006-01-02 15:04:05")
	records, err := store.listFilesByPathPrefix(rel)
	if err != nil {
		Logger().Warn("读取删除目录的索引记录失败（下次扫描自愈）", "libraryId", libraryID, "path", rel, "err", err)
		return 0, fmt.Errorf("读取受影响索引记录失败: %w", err)
	}
	var lostMarked int64
	for _, rec := range records {
		n, err := store.markFileLost(rec.id, now)
		if err != nil {
			Logger().Warn("标记丢失文件记录失败（下次扫描自愈）", "libraryId", libraryID, "path", rec.path, "err", err)
			continue
		}
		lostMarked += n
	}
	if n, err := store.deleteSplitRecordsByPathPrefix(rel); err != nil {
		Logger().Warn("清理拆分记录失败（下次扫描收敛）", "libraryId", libraryID, "path", rel, "err", err)
	} else if n > 0 {
		Logger().Info("已清理删除目录内的拆分记录", "libraryId", libraryID, "path", rel, "removed", n)
	}
	// 文件夹标注级联删除（PRD R8/PR4：目录物理已删，meta + 标签绑定保留成孤儿键；
	// 目录边界语义与拆分记录清理同构）
	if err := store.deleteFolderMetaByPathPrefix(rel); err != nil {
		Logger().Warn("清理文件夹标注失败（孤儿键残留可手动清理）", "libraryId", libraryID, "path", rel, "err", err)
	}
	Logger().Info("素材文件夹已删除", "libraryId", libraryID, "path", rel, "force", force, "lostMarked", lostMarked)
	return lostMarked, nil
}

// RenameAssetFolder 同父重命名（文件夹管理入口三，PRD R3/D：目录级移动）：
// 磁盘 rename（同盘原子）+ 索引路径前缀批量更新 + 排除规则双向翻转 + 自动增量
// 扫描收敛。
//
// 索引同步复用前缀替换单点机制（assetPathRenamesForPrefix 构造 + 冲突预检 +
// replaceFilePaths 单事务批量 UPDATE，RowsAffected 零命中上抛——并发清理窗口
// 如实报错）：同父重命名的替换集新路径全部落在 parent/新名/ 之下，与替换集旧
// 路径（parent/旧名/…）不相交，无链式腾位，长度降序排序为无害恒等序。
//
// FTS：检索列不含路径（见文件头），纯路径更新无需重建 FTS 行；排除翻转双向
// 经既有单点——新名命中 markFileExcluded（excluded 无损保留 FTS）、原 excluded
// 新名解除经 fileHasSlides 资格门槛 + reviveLostFile（单事务含 syncFtsFileRowTx
// 重建，§12.2 契约）；无产物行不翻转，交由随后自动扫描重解析重分类。
//
// 拆分记录守卫（宁严勿错）：子树内存在拆分记录时拒绝重命名——split_records 以
// 源文件相对路径为键且无重链机制，重命名会断「打开拆分单页」溯源与增量复拆
// 判定（PRD 拍板：无重链机制则拒绝，不做部分迁移）。
//
// 失败恢复姿态：磁盘 rename 成功后的索引写失败不回滚磁盘（rename 不可逆），错误
// 上抛由前端刷新 + 下次扫描增量三态自愈（旧路径消失标 lost、新路径新入库）。
// 自动扫描在锁释放后执行（StartScan 自取锁），失败不掩盖重命名结果（已提交）。
func (s *AssetLibraryService) RenameAssetFolder(libraryID, relPath, newName string) (model.AssetFolderRenameResult, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetFolderRenameResult{}, err
	}
	rel, abs, err := resolveLibraryRelPath(lib.RootPath, relPath)
	if err != nil {
		return model.AssetFolderRenameResult{}, fmt.Errorf("目录路径非法: %w", err)
	}
	clean, err := normalizeAssetFolderName(newName)
	if err != nil {
		return model.AssetFolderRenameResult{}, err
	}
	if clean == path.Base(rel) {
		return model.AssetFolderRenameResult{}, errors.New("新名称与当前名称相同，无需重命名")
	}
	// 新父 = 源自身父目录（同父重命名；rel 无斜杠 = 一级目录，父为库根空串）
	parentRel := ""
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		parentRel = rel[:i]
	}
	return s.runFolderRelocate(libraryID, rel, abs, parentRel, clean, "重命名")
}

// MoveAssetFolder 目录级移动（PRD R5/D，文件夹树 PR4）：把目录整体移动到目标
// 目录下（targetFolder 空串 = 库根），保持自身名称。与 RenameAssetFolder 共用
// moveAssetFolderCore 统一核心（磁盘 rename 先行 + 前缀替换单点 + 排除双向翻转
// + folder_meta/folder_tags 键同事务级联 + 自动增量扫描收敛）。
//
// 目标校验（入口整体执行，对全部受影响文件生效）：目标目录存在且为目录、非排除
// 命中、非拆分派生容器；**目标不得位于源目录自身子树内**（rename 进后代会把
// 目录挂到自己下面，文件系统行为未定义且索引前缀替换语义崩坏，明确拒绝）；目
// 标内重名（磁盘面，核心内与重命名同口径预检）。源含拆分记录维持 PR2 拒绝口径
// （宁严勿错，目录级迁移的镜像树失效面不做部分迁移）。
func (s *AssetLibraryService) MoveAssetFolder(libraryID, folderRelPath, targetFolder string) (model.AssetFolderRenameResult, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetFolderRenameResult{}, err
	}
	rel, abs, err := resolveLibraryRelPath(lib.RootPath, folderRelPath)
	if err != nil {
		return model.AssetFolderRenameResult{}, fmt.Errorf("目录路径非法: %w", err)
	}
	if rel == "" {
		return model.AssetFolderRenameResult{}, errors.New("库根目录不可移动")
	}
	// 目标目录归一（空串 = 库根）与整体校验（形态与 MoveAssetFiles 目标校验同构）
	targetRel := ""
	if strings.TrimSpace(targetFolder) != "" {
		targetRel, _, err = resolveLibraryRelPath(lib.RootPath, targetFolder)
		if err != nil {
			return model.AssetFolderRenameResult{}, fmt.Errorf("目标目录路径非法: %w", err)
		}
	}
	matcher := newAssetExcludeMatcher(lib.ExcludeDirs, lib.ExcludeExts)
	targetAbs := filepath.Join(lib.RootPath, filepath.FromSlash(targetRel))
	info, err := os.Stat(targetAbs)
	if err != nil {
		return model.AssetFolderRenameResult{}, fmt.Errorf("目标目录不存在或无法访问: %w", err)
	}
	if !info.IsDir() {
		return model.AssetFolderRenameResult{}, errors.New("目标路径不是目录")
	}
	if targetRel != "" && matcher.matchDirPath(targetRel) {
		return model.AssetFolderRenameResult{}, fmt.Errorf("目标目录「%s」命中索引排除规则，不能移入", targetRel)
	}
	if dirContainsSplitDerivative(targetAbs, targetRel) {
		return model.AssetFolderRenameResult{}, fmt.Errorf("目标目录「%s」内含 PPT 拆分派生产物，不能移入", targetRel)
	}
	if dirContainsSplitDerivative(abs, rel) {
		return model.AssetFolderRenameResult{}, fmt.Errorf("目录「%s」内含 PPT 拆分派生产物，由拆分链路管理，不能移动", rel)
	}
	// 位置无变化：移入当前所在父目录 = no-op 明确拒绝（防误触发自动扫描）
	if targetRel == parentRelOf(rel) {
		return model.AssetFolderRenameResult{}, errors.New("目标目录与当前位置相同，无需移动")
	}
	return s.runFolderRelocate(libraryID, rel, abs, targetRel, path.Base(rel), "移动")
}

// parentRelOf 目录相对路径的父目录（无斜杠 = 一级目录，父为库根空串）。
func parentRelOf(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}

// GetAssetFolderMeta 读文件夹标注（PRD R8）：业务备注 + 标签集（Tags 为标签本体，
// ID 口径与文件/页标签同源 tags 表）。未标注过的目录返回零值形态（note 空、Tags
// 空切片恒非 nil，前端免判空）。守卫与禁管理口径一致：库根/排除命中目录/拆分
// 派生容器禁标注。
func (s *AssetLibraryService) GetAssetFolderMeta(libraryID, folderPath string) (model.AssetFolderMeta, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetFolderMeta{}, err
	}
	rel, abs, err := resolveLibraryRelPath(lib.RootPath, folderPath)
	if err != nil {
		return model.AssetFolderMeta{}, fmt.Errorf("目录路径非法: %w", err)
	}
	if err := refuseFolderAnnotation(matcherFor(lib), rel, abs); err != nil {
		return model.AssetFolderMeta{}, err
	}
	var note, updatedAt string
	var tags []model.AssetTag
	err = s.withStore(libraryID, func(store *assetsStore) error {
		note, updatedAt, tags, err = store.getFolderMeta(rel)
		return err
	})
	if err != nil {
		return model.AssetFolderMeta{}, fmt.Errorf("读取文件夹标注失败: %w", err)
	}
	return model.AssetFolderMeta{FolderPath: rel, Note: note, Tags: tags, UpdatedAt: updatedAt}, nil
}

// UpdateAssetFolderMeta 文件夹标注编辑（整体替换语义，对齐 UpdateFileMeta）：
// 业务备注 + 标签集（不存在的标签自动创建）。纯标注层：不入 FTS（对齐页级口径）、
// 不触发重解析与缩略图失效。守卫与 Get 同口径（禁标注目录拒绝）。
func (s *AssetLibraryService) UpdateAssetFolderMeta(libraryID, folderPath string, meta model.AssetFolderMetaUpdate) error {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return err
	}
	rel, abs, err := resolveLibraryRelPath(lib.RootPath, folderPath)
	if err != nil {
		return fmt.Errorf("目录路径非法: %w", err)
	}
	if err := refuseFolderAnnotation(matcherFor(lib), rel, abs); err != nil {
		return err
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	return s.withStore(libraryID, func(store *assetsStore) error {
		return store.updateFolderMeta(rel, meta.Note, meta.TagNames, now)
	})
}

// matcherFor 库排除 matcher 编译（标注守卫与扫描/管理同源单点，禁第二口径）。
func matcherFor(lib model.AssetLibrary) *assetExcludeMatcher {
	return newAssetExcludeMatcher(lib.ExcludeDirs, lib.ExcludeExts)
}

// refuseFolderAnnotation 文件夹标注守卫（Get/Update 共用单点）：库根不可标注
// （根为固定聚合节点无磁盘对应物）；排除命中目录与拆分派生容器禁标注（与禁
// 管理同口径——排除目录文件不可见，标注无从谈起；派生产物由拆分链路管理）。
func refuseFolderAnnotation(matcher *assetExcludeMatcher, rel, abs string) error {
	if rel == "" {
		return errors.New("库根目录不可标注")
	}
	if matcher.matchDirPath(rel) {
		return fmt.Errorf("目录「%s」命中索引排除规则，禁用标注", rel)
	}
	if dirContainsSplitDerivative(abs, rel) {
		return fmt.Errorf("目录「%s」内含 PPT 拆分派生产物，由拆分链路管理，禁用标注", rel)
	}
	return nil
}

// runFolderRelocate 目录重定位公共骨架（重命名/移动共用，PRD PR4 统一核心的
// 编排面）：规则面守卫（根拒绝/派生容器拒绝）→ 锁段收拢闭包（moveAssetFolderCore）
// → 自动增量扫描收敛。opName 供日志与错误措辞区分（重命名/移动）。
func (s *AssetLibraryService) runFolderRelocate(libraryID, rel, abs, newParentRel, newName, opName string) (model.AssetFolderRenameResult, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetFolderRenameResult{}, err
	}
	matcher := newAssetExcludeMatcher(lib.ExcludeDirs, lib.ExcludeExts)
	if dirContainsSplitDerivative(abs, rel) {
		return model.AssetFolderRenameResult{}, fmt.Errorf("目录「%s」内含 PPT 拆分派生产物，由拆分链路管理，不能%s", rel, opName)
	}
	result := model.AssetFolderRenameResult{}
	// 串行保护（同 Replace 前缀替换先例：锁段收拢进闭包——锁内闭包先返回释放
	// 扫描锁，随后的自动扫描 StartScan 自取锁才不会自撞 E_ASSETS_SCAN_IN_PROGRESS）
	err = func() error {
		release, err := s.tryLockScan(libraryID)
		if err != nil {
			return err
		}
		defer release()
		lib, err = s.getLibrary(libraryID)
		if err != nil {
			return err
		}
		store, err := s.storeForScan(libraryID)
		if err != nil {
			return err
		}
		return s.moveAssetFolderCore(lib, store, matcher, rel, abs, newParentRel, newName, opName, &result)
	}()
	if err != nil {
		return result, err
	}
	Logger().Info("素材文件夹已"+opName, "libraryId", libraryID, "oldPath", result.OldPath, "newPath", result.NewPath,
		"replaced", result.Replaced, "excluded", result.Excluded, "revived", result.Revived)

	// 自动增量扫描收敛（锁已释放，StartScan 自取锁）：翻转遗留（无产物行重分类、
	// 新名命中而显式翻转遗漏面等）与库内其余变化一并收敛。重定位（磁盘 + 索引）
	// 已提交，扫描失败（含 E_ASSETS_SCAN_IN_PROGRESS 被并发扫描抢占）**降级为
	// 告警照常返回成功结果，不随调用上抛**——前端成功分支刷新链保持、失败提示
	// 自然消失；最终一致由在途扫描（用户手动或后续操作触发）的 done 事件兜底
	if _, err := s.autoScan(context.Background(), libraryID); err != nil {
		Logger().Warn("文件夹"+opName+"后的自动增量扫描失败（操作已提交，最终一致由在途扫描兜底）",
			"libraryId", libraryID, "err", err)
	}
	return result, nil
}

// moveAssetFolderCore 目录重定位统一核心（持扫描锁调用；重命名/移动共用，PRD
// PR4 抽取——RenameAssetFolder 委托 newParentRel=源自身父目录、MoveAssetFolder
// 传目标目录）：目标重名与子树包含预检 → 磁盘 rename 先行（同盘原子；失败时索
// 引未动零半态）→ 索引前缀同步（assetPathRenamesForPrefix 单点 + replaceFilePaths
// 单事务批量 UPDATE，folder_meta/folder_tags 键同事务级联改键）→ 排除规则双向
// 翻转。索引写失败不回滚磁盘（rename 不可逆），错误上抛由前端刷新 + 下次扫描
// 增量三态自愈（旧路径消失标 lost、新路径新入库）。
func (s *AssetLibraryService) moveAssetFolderCore(lib model.AssetLibrary, store *assetsStore, matcher *assetExcludeMatcher, rel, abs, newParentRel, newName, opName string, result *model.AssetFolderRenameResult) error {
	libraryID := lib.ID
	// 新路径（newParentRel/新名；父为库根时新路径即新名）
	newRel := newName
	if newParentRel != "" {
		newRel = newParentRel + "/" + newName
	}
	result.OldPath = rel
	result.NewPath = newRel
	// 子树包含预检：目标不得位于源目录自身子树内（rename 进后代 = 目录挂到
	// 自己下面，文件系统语义未定义且索引前缀替换崩坏），恰等自身 = 位置无变化
	if newRel == rel || strings.HasPrefix(newRel, rel+"/") {
		return fmt.Errorf("目标目录位于源目录「%s」内部，不能%s到自身子树", rel, opName)
	}
	// 目标重名预检（磁盘面）：同名目录或文件已存在拒绝（os.ErrExist 口径）
	newAbs := filepath.Join(lib.RootPath, filepath.FromSlash(newRel))
	if _, err := os.Stat(newAbs); err == nil {
		return fmt.Errorf("同名目录或文件「%s」已存在", newRel)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("检查目标路径失败: %w", err)
	}
	// 拆分记录守卫（宁严勿错，见 RenameAssetFolder 函数头注释）
	hasSplit, err := store.splitRecordExistsByPathPrefix(rel)
	if err != nil {
		return fmt.Errorf("检查拆分记录失败: %w", err)
	}
	if hasSplit {
		return fmt.Errorf(
			"目录「%s」内存在已拆分文件（含拆分记录），%s会断开拆分产物溯源，请先删除对应拆分记录后再操作", rel, opName)
	}
	// 磁盘重定位先行（同盘原子；失败时索引未动零半态）
	if err := os.Rename(abs, newAbs); err != nil {
		return fmt.Errorf("%s目录失败: %w", opName, err)
	}

	// 索引前缀同步（复用前缀替换单点：构造 + 冲突预检 + 单事务批量 UPDATE）
	records, err := store.listFilesByPathPrefix(rel)
	if err != nil {
		Logger().Warn("读取重命名目录的索引记录失败（下次扫描自愈）", "libraryId", libraryID, "path", rel, "err", err)
		return fmt.Errorf("读取受影响索引记录失败: %w", err)
	}
	var renames []assetPathRename
	if len(records) > 0 {
		allPaths, err := store.listFileRecords()
		if err != nil {
			return fmt.Errorf("读取文件索引失败: %w", err)
		}
		renames, err = assetPathRenamesForPrefix(records, allPaths, rel, newRel)
		if err != nil {
			return err
		}
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	// 索引前缀同步（复用前缀替换单点：构造 + 冲突预检 + 单事务批量 UPDATE）。
	// records 为空（无 pptx 索引面）不得提前 return：空目录/仅非 PPT 文件的目录
	// 同样可能带 folder_meta/folder_tags 标注，replaceFilePaths 对空 renames 仅
	// 执行文件夹标注键级联改键，跳过则标注成旧路径孤儿键（标注静默丢失）。
	// 文件夹标注键同事务级联改键（PR4 R8：folder_meta/folder_tags 以 folder_path
	// 为键，目录重命名/移动时随路径前缀替换——与 files.path 更新同一事务保证原子）
	if err := store.replaceFilePaths(renames, now, rel, newRel); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// 并发窗口：命中行被「清理丢失记录」删除（ClearLostAssetFiles 不持
			// 扫描锁）——事务已整体回滚，如实报错防 Replaced 虚报
			return errors.New("命中记录已被清理，请刷新后重试")
		}
		return fmt.Errorf("批量更新路径失败: %w", err)
	}
	result.Replaced = int64(len(renames))

	// 受影响文件重过 ExcludeDirs matcher（双向翻转）。配对不变式：renames 已按
	// 旧路径长度降序重排，与 records 原序（path 升序）不同序——必须经 recByID
	// 按 fileID 取记录快照遍历 renames，禁按下标 zip 配对（前缀替换同款教训，
	// TestAssetPrefixReplace_MixedLengthPairing 守护同款错位）
	recByID := make(map[int64]assetFileRecord, len(records))
	for _, rec := range records {
		recByID[rec.id] = rec
	}
	for _, rn := range renames {
		rec, ok := recByID[rn.fileID]
		if !ok {
			// 结构性不可达（renames 与 records 同源），防御未来来源变更零值
			// 静默错判状态
			return fmt.Errorf("内部错误：重命名记录快照缺失（fileId=%d）", rn.fileID)
		}
		matched := matcher.matchPath(rn.newRel)
		if matched && rec.status != model.AssetFileStatusExcluded {
			// 新名命中排除：翻 excluded（任何现存态命中即标，幂等，§12.2 先排除
			// 后存在性；FTS 行无损保留）
			if n, err := store.markFileExcluded(rec.id, now); err != nil {
				Logger().Warn("标记排除文件记录失败（下次扫描自愈）", "libraryId", libraryID, "path", rn.newRel, "err", err)
			} else {
				result.Excluded += int(n)
			}
			continue
		}
		if !matched && rec.status == model.AssetFileStatusExcluded {
			// 原 excluded 新名解除：过 fileHasSlides 资格门槛直复活 ok（FTS 同
			// 事务重建，reviveLostFile 单点）；无产物行不翻转，交由随后自动扫描
			// 重解析按实际内容重分类（禁直复活谎报 ok，§12.2 契约）
			hasSlides, err := store.fileHasSlides(rec.id)
			if err != nil {
				Logger().Warn("检查解析产物失败（下次扫描自愈）", "libraryId", libraryID, "path", rn.newRel, "err", err)
				continue
			}
			if hasSlides {
				if err := store.reviveLostFile(rec.id, now); err != nil {
					Logger().Warn("复活排除文件记录失败（下次扫描自愈）", "libraryId", libraryID, "path", rn.newRel, "err", err)
					continue
				}
				result.Revived++
			}
		}
	}
	return nil
}

// MoveAssetFiles 文件批量移动（PRD R4/R6/R9，PR3 拖拽移动后端入口）：把文件记录
// 及其磁盘文件移动到目标目录（targetFolder 空串 = 库根）。每条目一事务逐条目
// 提交（对齐批量评分/批量打标签先例），部分失败不中断不回滚，逐条目明细返回；
// 终态仅经返回值汇总（禁逐文件发事件）。
//
// 单文件移动链路（磁盘先行，索引随行——与 RenameAssetFolder 同姿态）：
//   - 校验（写面前置，任一失败记 failed 明细继续）：fileID 存在（SELECT path 顺带
//     拼装 Label）；源磁盘文件存在；同盘（VolumeName 比对，跨盘明确拒绝——MVP
//     不做 copy+delete 降级，PRD Out of Scope）；目标内重名（磁盘面 os.Stat，
//     磁盘无同名时索引 UNIQUE 亦无冲突——索引行恒随磁盘文件存在，lost 行例外见
//     下）；目标目录校验在入口整体执行（存在/是目录/非排除命中/非派生容器）；
//   - os.Rename（同盘原子）→ 事务内 files.path 更新（RowsAffected 零命中 =
//     并发清理窗口，sql.ErrNoRows）+ split_records 源路径同事务重写（单表主键
//     重写，output_dir 为绝对路径不受影响、文件基名不变 → 「打开拆分单页」溯源
//     保持有效；PR2 目录重命名的拒绝口径源于**目录级**迁移的镜像树失效面，单
//     文件移动无此复杂度，故走重写不拒绝）；
//   - FTS：检索列不含路径（file_name 为标题基名派生，见 syncFtsFileRowTx），
//     纯路径移动零重建（rowid = files.id/slides.id 稳定）；排除翻转经既有单点
//     （新路径命中 markFileExcluded——FTS 无损保留；原 excluded 解除经
//     fileHasSlides 门槛 + reviveLostFile——FTS 同事务重建），禁裸 UPDATE 翻状态
//     （§12.2 教训）；
//   - relPath 前缀派生字段：files 表无冗余目录列（path/title 即全部位置语义，
//     title 为基名不受移动影响），无重算面。
//
// lost 行可移动（磁盘文件在旧路径时 rename 自然成功，lost 记录随路径修正复活
// 语义交由状态翻转与随后自动扫描收敛）；processing 残留行拒绝移动（自愈不变式
// ——半态行随路径修正会让自愈判定失据，等下轮扫描重处理后再移）。
//
// 锁形态：批量与单条目写 files/split_records，持 tryLockScan 与扫描互斥（锁内
// 复验库存在）；自动增量扫描在锁释放后执行（翻转遗留与库内其余变化收敛）。
// fileID 重复静默去重（保持首次出现序，对齐前端 targets 反查天然去重口径）。
func (s *AssetLibraryService) MoveAssetFiles(libraryID string, fileIDs []int64, targetFolder string) (model.AssetBatchMoveResult, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetBatchMoveResult{}, err
	}
	// 目标目录归一（空串 = 库根；非空经越界遏制单点清洗）
	targetRel := ""
	if strings.TrimSpace(targetFolder) != "" {
		targetRel, _, err = resolveLibraryRelPath(lib.RootPath, targetFolder)
		if err != nil {
			return model.AssetBatchMoveResult{}, fmt.Errorf("目标目录路径非法: %w", err)
		}
	}
	matcher := newAssetExcludeMatcher(lib.ExcludeDirs, lib.ExcludeExts)
	targetAbs := filepath.Join(lib.RootPath, filepath.FromSlash(targetRel))
	// 目标目录整体校验（一次执行对全部条目生效）：存在且为目录、非排除命中
	//（matchDirPath 逐段判定，库根 targetRel=="" 天然豁免）、非拆分派生容器
	info, err := os.Stat(targetAbs)
	if err != nil {
		return model.AssetBatchMoveResult{}, fmt.Errorf("目标目录不存在或无法访问: %w", err)
	}
	if !info.IsDir() {
		return model.AssetBatchMoveResult{}, errors.New("目标路径不是目录")
	}
	if targetRel != "" && matcher.matchDirPath(targetRel) {
		return model.AssetBatchMoveResult{}, fmt.Errorf("目标目录「%s」命中索引排除规则，不能移入", targetRel)
	}
	if dirContainsSplitDerivative(targetAbs, targetRel) {
		return model.AssetBatchMoveResult{}, fmt.Errorf("目标目录「%s」内含 PPT 拆分派生产物，不能移入", targetRel)
	}
	// fileID 去重（保序，对齐前端选中集天然无重复口径的防御）
	seen := make(map[int64]bool, len(fileIDs))
	uniqueIDs := make([]int64, 0, len(fileIDs))
	for _, id := range fileIDs {
		if id < 1 || seen[id] {
			continue
		}
		seen[id] = true
		uniqueIDs = append(uniqueIDs, id)
	}
	result := model.AssetBatchMoveResult{Total: len(uniqueIDs), Items: []model.AssetBatchMoveItem{}}

	// 串行保护（锁段收拢进闭包——自动扫描须在锁释放后自取锁，RenameAssetFolder
	// 同款时序）
	err = func() error {
		release, err := s.tryLockScan(libraryID)
		if err != nil {
			return err
		}
		defer release()
		lib, err = s.getLibrary(libraryID)
		if err != nil {
			return err
		}
		store, err := s.storeForScan(libraryID)
		if err != nil {
			return err
		}
		now := time.Now().Format("2006-01-02 15:04:05")
		for _, fileID := range uniqueIDs {
			item := s.moveOneAssetFile(store, lib, matcher, fileID, targetRel, targetAbs, now)
			result.Items = append(result.Items, item)
			if item.Status == model.AssetBatchTagStatusOK {
				result.Succeeded++
			} else {
				result.Failed++
			}
		}
		return nil
	}()
	if err != nil {
		return result, err
	}
	if result.Succeeded > 0 {
		Logger().Info("素材文件批量移动完成", "libraryId", libraryID, "target", targetRel,
			"succeeded", result.Succeeded, "failed", result.Failed)
	}
	// 自动增量扫描收敛（锁已释放）：排除翻转遗漏面（无产物行重分类）与库内其余
	// 变化一并收敛。移动（磁盘 + 索引）已提交，扫描失败（含 E_ASSETS_SCAN_IN_PROGRESS
	// 被并发扫描抢占）**降级为告警照常返回成功结果，不随调用上抛**——前端成功分支
	// 刷新链保持、失败提示自然消失；最终一致由在途扫描的 done 事件兜底
	if result.Succeeded > 0 {
		if _, err := s.autoScan(context.Background(), libraryID); err != nil {
			Logger().Warn("文件移动后的自动增量扫描失败（操作已提交，最终一致由在途扫描兜底）",
				"libraryId", libraryID, "err", err)
		}
	}
	return result, nil
}

// autoScan 自动增量扫描入口（runFolderRelocate/MoveAssetFiles 消费）：autoScanFn
// 注入优先（单测 fake），nil 回退生产实现 StartScan。
func (s *AssetLibraryService) autoScan(ctx context.Context, libraryID string) (model.AssetScanResult, error) {
	if s.autoScanFn != nil {
		return s.autoScanFn(ctx, libraryID)
	}
	return s.StartScan(ctx, libraryID)
}

// moveOneAssetFile 单文件移动（每条目一事务 + 明细，批量评分 applyRatingTarget
// 同构）：磁盘 rename 先行（同盘原子，失败时索引未动零半态）→ 事务内 files.path
// 更新 + split_records 源路径重写 → 排除翻转经既有单点（事务外单语句，与
// RenameAssetFolder 同形态）。永不返回 error——失败以 failed 明细隔离（批量
// 不中断），符合「部分失败不中断」契约。
func (s *AssetLibraryService) moveOneAssetFile(store *assetsStore, lib model.AssetLibrary, matcher *assetExcludeMatcher, fileID int64, targetRel, targetAbs string, now string) model.AssetBatchMoveItem {
	item := model.AssetBatchMoveItem{FileID: fileID, Status: model.AssetBatchTagStatusOK}
	fail := func(label, format string, args ...any) model.AssetBatchMoveItem {
		return model.AssetBatchMoveItem{FileID: fileID, Status: model.AssetBatchTagStatusFailed,
			Message: fmt.Sprintf(format, args...), Label: label}
	}

	// 存在性校验 + Label 拼装共用一次 path 查询（批量评分同款）；processing 残留
	// 行拒绝（自愈不变式，见 MoveAssetFiles 函数头注释）
	rec, err := store.fileRecordByID(fileID)
	if errors.Is(err, sql.ErrNoRows) {
		return fail("", "文件不存在或已被删除（fileId=%d），请刷新列表", fileID)
	}
	if err != nil {
		return fail("", "读取文件记录失败: %v", err)
	}
	item.Label = rec.path
	if rec.status == model.AssetFileStatusProcessing {
		return fail(item.Label, "文件正在解析中（fileId=%d），请稍后重试", fileID)
	}

	oldAbs := filepath.Join(lib.RootPath, filepath.FromSlash(rec.path))
	base := path.Base(rec.path)
	newRel := base
	if targetRel != "" {
		newRel = targetRel + "/" + base
	}
	// no-op 识别（P2 收口）：移入文件当前所在目录（新路径与现路径相同）单独报错，
	// 与目标内重名区分——重名报错语义是「目标有别的同名文件」，no-op 是「文件本身
	// 已就位」；计入 failed 明细正常返回（非错误码新增），磁盘与索引零操作
	if newRel == rec.path {
		return fail(item.Label, "文件已在该目录中，无需移动")
	}
	newAbs := filepath.Join(targetAbs, base)

	// 同盘校验（跨盘明确拒绝，MVP 不做 copy+delete 降级，PRD Out of Scope）
	if filepath.VolumeName(oldAbs) != filepath.VolumeName(newAbs) {
		return fail(item.Label, "跨盘移动不支持（%s → %s 为不同磁盘卷），请手动复制", oldAbs, newAbs)
	}
	// 源文件存在性（lost 记录的磁盘文件缺失时如实拒绝——无文件可移动）
	if _, err := os.Stat(oldAbs); err != nil {
		return fail(item.Label, "源文件不存在或无法访问: %v", err)
	}
	// 目标内重名拒绝（磁盘面权威：目标已存在同名文件/目录）
	if _, err := os.Stat(newAbs); err == nil {
		return fail(item.Label, "目标目录已存在同名文件「%s」", base)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(item.Label, "检查目标路径失败: %v", err)
	}

	// 磁盘移动先行（同盘原子；失败时索引未动零半态）
	if err := os.Rename(oldAbs, newAbs); err != nil {
		return fail(item.Label, "移动文件失败: %v", err)
	}

	// 事务内：files.path 更新（RowsAffected 零命中 = 并发清理窗口）+ split_records
	// 源路径同事务重写（无拆分记录零行更新，非错误）
	if err := store.moveFilePath(relPathRename{fileID: fileID, newRel: newRel, oldRel: rec.path}, now); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fail(item.Label, "记录已被清理，请刷新后重试")
		}
		return fail(item.Label, "更新索引路径失败: %v", err)
	}

	// 排除翻转（新路径重过 ExcludeDirs matcher，双向；事务外单语句，与
	// RenameAssetFolder 同形态——reviveLostFile 自管事务不可嵌套）
	matched := matcher.matchPath(newRel)
	switch {
	case matched && rec.status != model.AssetFileStatusExcluded:
		// 新路径命中排除：翻 excluded（FTS 无损保留，§12.2）
		if _, err := store.markFileExcluded(fileID, now); err != nil {
			Logger().Warn("标记排除文件记录失败（下次扫描自愈）", "libraryId", lib.ID, "path", newRel, "err", err)
		}
	case !matched && rec.status == model.AssetFileStatusExcluded:
		// 原排除解除：过 fileHasSlides 资格门槛直复活 ok（FTS 同事务重建）；无产物
		// 行不翻转，交由随后自动扫描重解析重分类
		hasSlides, err := store.fileHasSlides(fileID)
		if err == nil && hasSlides {
			if err := store.reviveLostFile(fileID, now); err != nil {
				Logger().Warn("复活排除文件记录失败（下次扫描自愈）", "libraryId", lib.ID, "path", newRel, "err", err)
			}
		}
	}
	return item
}

// relPathRename 单文件移动的路径改写载荷（moveFilePath 消费）。
type relPathRename struct {
	fileID int64
	oldRel string // 移动前相对路径（split_records 源路径重写匹配键）
	newRel string // 移动后相对路径
}

// moveFilePath 单文件移动的事务落库（每条目一事务）：files.path 更新 + 该源的
// split_records 源路径同事务重写。FTS 说明见 MoveAssetFiles 函数头（检索列不含
// 路径，纯路径移动零重建；状态翻转经调用方既有单点，禁在本事务裸翻状态）。
func (s *assetsStore) moveFilePath(r relPathRename, now string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		"UPDATE files SET path = ?, updated_at = ? WHERE id = ?", r.newRel, now, r.fileID,
	)
	if err != nil {
		return err
	}
	// 零命中 = 行已被并发「清理丢失记录」删除：以 sql.ErrNoRows 上抛（对齐
	// replaceFilePaths 同窗口口径），调用方转「记录已被清理」可读错误
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(
		"UPDATE split_records SET source_rel_path = ? WHERE source_rel_path = ?", r.newRel, r.oldRel,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// refuseUnmanageableFolderByRule 文件夹管理操作的规则面守卫（新建子目录的父目录
// 与删除/重命名的目标共用单点）：库根（rel 为空）不可管理；排除规则命中目录
// （matchDirPath 逐段判定，含被排除祖先继承）拒绝。派生容器判定需磁盘访问，
// 由调用方按语境单独执行（dirContainsSplitDerivative）。
func refuseUnmanageableFolderByRule(matcher *assetExcludeMatcher, rel string) error {
	if rel == "" {
		return errors.New("库根目录不可执行此管理操作")
	}
	if matcher.matchDirPath(rel) {
		return fmt.Errorf("目录「%s」命中索引排除规则，禁用管理操作（可在「索引设置」中调整规则）", rel)
	}
	return nil
}

// dirContainsSplitDerivative 目录直属文件中是否存在拆分派生副本（文件夹管理
// 操作禁操作判定用）：逐个直属文件过 isPptxSplitDerivative 单点判定，禁第二套
// 派生判定口径。目录不可入按不含派生处理（后续磁盘操作自身会暴露真实错误）。
func dirContainsSplitDerivative(absDir, relDir string) bool {
	dirEntries, err := os.ReadDir(absDir)
	if err != nil {
		return false
	}
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		relFile := entry.Name()
		if relDir != "" {
			relFile = relDir + "/" + entry.Name()
		}
		if isPptxSplitDerivative(relFile) {
			return true
		}
	}
	return false
}

// normalizeAssetFolderName 文件夹名归一与校验（创建/重命名共用单点）：TrimSpace；
// 拒绝空名、路径分隔符（/ 与 \——目录名是单段，含分隔符即路径注入）、点形态
// （. / ..）、Windows 保留字符（<>:"|?*）与控制字符。
func normalizeAssetFolderName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("请输入文件夹名称")
	}
	if strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("文件夹名称 %q 非法：不能包含路径分隔符", name)
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("文件夹名称 %q 非法", name)
	}
	if strings.ContainsAny(name, `<>:"|?*`) {
		return "", fmt.Errorf("文件夹名称 %q 含非法字符（<>:\"|?*）", name)
	}
	for _, r := range name {
		if r < 0x20 {
			return "", errors.New("文件夹名称含非法控制字符")
		}
	}
	return name, nil
}
