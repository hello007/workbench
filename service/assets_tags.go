package service

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"workbench/model"
)

// 素材标签树与批量打标签（素材管理第六批，SRS §2.4）。
//
// 文件组织：本文件承载标签域完整链路——assetsStore 层数据访问（单连接池自锁
// 纪律见 assets_db.go libraryStats 注释）与 AssetLibraryService 编排（库校验 +
// 参数归一），模式对齐 assets_thumbs_ledger.go 的域内聚先例。
//
// 决策依据（任务 10-15-srs-2-4 PRD D1/D2/D3/D4/D7 + D8#1 拖拽排序增量）：
//   - 标签树 = 启用 tags.parent_id 现列（v1 基线预留，NULL/0 = 根标签），单一
//     tags 表无独立字典；ensureTagTx 自动创建的新标签恒为根标签；
//   - 筛选/检索命中口径 = 所选标签 ID ∪ 全部后代（expandTagDescendants 单点展开，
//     由 searchAssetsInStore 在两路径分流前调用）；批量绑定不级联（绑定是精确
//     动作，筛选才是聚合语义）；
//   - 批量 = 每条目一事务逐条目提交，失败记明细继续；add/remove 双数组相对
//     条目现状追加/移除，非整体替换；交集非空/双全空/条目空均为参数错误
//     （普通 error 形态，跟随 UpdateFileMeta 等既有参数校验口径，无域错误码）；
//   - 删除标签 = 级联删除自身 + 全部后代并解除全部绑定（删除标签不删素材），
//     单事务；返回解除关联的绑定行数。
//
// 纯标注层约束（对齐 updateSlideMeta）：批量页标签变更不写 FTS（标签不入检索
// 列，业务备注也不入）、不碰 thumb_cache、不触发重解析——检索命中口径由筛选
// 路径实时读 file_tags/slide_tags 关系表天然同步。

// ---------- store 层：标签树读写 ----------

// listTagsWithCounts 全量标签清单，带直接绑定计数（file_tags + slide_tags 两处
// 绑定行数之和）。相关子查询形态：标签量级为个人 DAM 桌面场景（百级），无需
// JOIN + GROUP BY 聚合。parent_id 可空列（ensureTagTx 既有行写 NULL）经 COALESCE
// 归一为 0（模型层 0 = 根标签）。
//
// 排序契约（PRD D8#1 拖拽排序）：平铺序 = 父级分组（根组在前、其余按父 ID 升序）
// + 组内 sort_order 升序、同序按 id。buildTagTree 只依赖「同组内相对序」（按平铺
// 出现序挂 children），本排序即兄弟展示序的单一来源；组间全局序不影响树形消费。
func (s *assetsStore) listTagsWithCounts() ([]model.AssetTagListItem, error) {
	rows, err := s.db.Query(
		"SELECT t.id, t.name, t.color, COALESCE(t.parent_id, 0), t.sort_order, " +
			"(SELECT COUNT(*) FROM file_tags ft WHERE ft.tag_id = t.id) + " +
			"(SELECT COUNT(*) FROM slide_tags st WHERE st.tag_id = t.id) " +
			"FROM tags t ORDER BY COALESCE(t.parent_id, 0), t.sort_order, t.id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.AssetTagListItem, 0)
	for rows.Next() {
		var item model.AssetTagListItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Color, &item.ParentID, &item.SortOrder, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// listTagParentEdges 全量标签父子边（id -> 父标签 ID，根为 0）：树闭包计算的
// 数据源。标签量级百级，全量载入内存计算优于递归 SQL（直观可测，且删除事务内
// 亦可经 tx 复用同一纯函数展开）。
func (s *assetsStore) listTagParentEdges() (map[int64]int64, error) {
	return listTagParentEdgesFrom(s.db)
}

// expandTagEdgesToClosure 树闭包纯函数：父子边集上把所选标签 ID 展开为
// 「自身 ∪ 全部后代」闭包集（去重升序）。BFS 沿「父 → 子」方向收集；visited
// 以 selected 集合承担，存量数据异常成环时自然终止不死循环。
// expandTagDescendants（非事务读）与 deleteTagTree（事务内子树收集）共用本函数，
// 禁两处各自展开。
func expandTagEdgesToClosure(edges map[int64]int64, tagIDs []int64) []int64 {
	children := make(map[int64][]int64, len(edges))
	for id, parentID := range edges {
		if parentID != 0 {
			children[parentID] = append(children[parentID], id)
		}
	}
	selected := make(map[int64]bool, len(tagIDs))
	queue := make([]int64, 0, len(tagIDs))
	for _, id := range tagIDs {
		if !selected[id] {
			selected[id] = true
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range children[cur] {
			if !selected[child] {
				selected[child] = true
				queue = append(queue, child)
			}
		}
	}
	out := make([]int64, 0, len(selected))
	for id := range selected {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// expandTagDescendants 标签子树展开（筛选级联命中口径的单点，PRD D2）：所选
// 标签 ID 展开为「自身 ∪ 全部后代」闭包集。平铺老数据（全 parent_id=0）展开
// 结果 = 去重后的自身集合，行为与多级标签引入前完全一致。调用方为
// searchAssetsInStore（FTS 关键词路径与纯标签浏览路径分流前统一展开透传下游），
// 禁在单一路径内另行展开。
func (s *assetsStore) expandTagDescendants(tagIDs []int64) ([]int64, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	edges, err := s.listTagParentEdges()
	if err != nil {
		return nil, err
	}
	return expandTagEdgesToClosure(edges, tagIDs), nil
}

// nullTagParent parent_id 写入值归一：0（根标签）写 NULL，与 ensureTagTx 既有
// 行形态一致（列语义「无父级」）；非 0 原样。
func nullTagParent(parentID int64) any {
	if parentID == 0 {
		return nil
	}
	return parentID
}

// createTag 新建标签（库内重名拒绝）：parentID ≠ 0 时须为已存在标签——新建
// 标签无子代不可能成环，仅需父存在性校验；color 为空时按标签名确定性分配默认
// 色板（assetTagColorFor，同名跨库同色）。sort_order 追加到目标兄弟组末尾
// （组内 MAX+1）：与拖拽排序的连续化序号衔接，避免 DEFAULT 0 插队组内首位
// （迁移回填/重排后的组内序号为 0..n-1 连续，追加序号恒取组尾）。
func (s *assetsStore) createTag(name string, parentID int64, color string) (model.AssetTag, error) {
	var exists int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tags WHERE name = ?", name).Scan(&exists); err != nil {
		return model.AssetTag{}, err
	}
	if exists > 0 {
		return model.AssetTag{}, fmt.Errorf("标签「%s」已存在", name)
	}
	if parentID != 0 {
		var parentExists int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM tags WHERE id = ?", parentID).Scan(&parentExists); err != nil {
			return model.AssetTag{}, err
		}
		if parentExists == 0 {
			return model.AssetTag{}, errors.New("父标签不存在或已被删除，请刷新标签列表")
		}
	}
	if color == "" {
		color = assetTagColorFor(name)
	}
	sortOrder, err := nextTagSortOrder(s.db, parentID)
	if err != nil {
		return model.AssetTag{}, err
	}
	res, err := s.db.Exec("INSERT INTO tags(name, color, parent_id, sort_order) VALUES(?, ?, ?, ?)",
		name, color, nullTagParent(parentID), sortOrder)
	if err != nil {
		// 并发重名窗口由 tags.name UNIQUE 约束兜底（应用层已前置查询给可读错误）
		return model.AssetTag{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.AssetTag{}, err
	}
	return model.AssetTag{ID: id, Name: name, Color: color, ParentID: parentID}, nil
}

// nextTagSortOrder 目标兄弟组的追加序号（组内 MAX(sort_order)+1，空组为 0）：
// 新建/移入标签恒排组尾。store 级调用全部经 withStore 串行（service 层 s.mu），
// 事务内经 tx 变体（nextTagSortOrderTx）读同快照，无竞态窗口。
func nextTagSortOrder(q interface {
	QueryRow(string, ...any) *sql.Row
}, parentID int64) (int64, error) {
	var sortOrder int64
	err := q.QueryRow(
		"SELECT COALESCE(MAX(sort_order), -1) + 1 FROM tags WHERE COALESCE(parent_id, 0) = ?", parentID,
	).Scan(&sortOrder)
	return sortOrder, err
}

// updateTagFull 标签编辑（重命名/改色/移动父级整体提交，对齐前端管理弹窗一次
// 提交形态）：重名拒绝（不同 ID 同名）；移动父级防环——新父不能是自身或自身
// 后代（从新父沿 parent 链向上遍历，途经自身 = 新父在自身子树内 = 成环）。
// 移动父级时 sort_order 追加到新兄弟组末尾（与 createTag/moveTag 的组序衔接，
// 避免沿用旧组序号在新组内插队）；仅重命名/改色不改动序号（不变更兄弟序）。
func (s *assetsStore) updateTagFull(tagID int64, name string, parentID int64, color string) error {
	var curParentID, curSortOrder int64
	err := s.db.QueryRow(
		"SELECT COALESCE(parent_id, 0), sort_order FROM tags WHERE id = ?", tagID,
	).Scan(&curParentID, &curSortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("标签不存在或已被删除，请刷新标签列表")
	}
	if err != nil {
		return err
	}
	var nameConflict int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tags WHERE name = ? AND id != ?", name, tagID).Scan(&nameConflict); err != nil {
		return err
	}
	if nameConflict > 0 {
		return fmt.Errorf("标签「%s」已存在", name)
	}
	if parentID == tagID {
		return errors.New("父标签不能选择自身")
	}
	if parentID != 0 {
		var parentExists int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM tags WHERE id = ?", parentID).Scan(&parentExists); err != nil {
			return err
		}
		if parentExists == 0 {
			return errors.New("父标签不存在或已被删除，请刷新标签列表")
		}
		edges, err := s.listTagParentEdges()
		if err != nil {
			return err
		}
		cur := parentID
		visited := make(map[int64]bool)
		for cur != 0 {
			if cur == tagID {
				return errors.New("父标签不能选择自身或其后代（会形成循环引用）")
			}
			if visited[cur] {
				break // 存量数据异常成环兜底，防遍历死循环
			}
			visited[cur] = true
			cur = edges[cur]
		}
	}
	sortOrder := curSortOrder
	if parentID != curParentID {
		sortOrder, err = nextTagSortOrder(s.db, parentID)
		if err != nil {
			return err
		}
	}
	_, err = s.db.Exec("UPDATE tags SET name = ?, color = ?, parent_id = ?, sort_order = ? WHERE id = ?",
		name, color, nullTagParent(parentID), sortOrder, tagID)
	return err
}

// moveTag 拖拽排序/移动父级（单事务，PRD D8#1）：语义 = 把 tagID 插入到目标父
// 下兄弟列表（排除自身、按现 sort_order 序）的第 index 位（index = 落位后自身在
// 组内的最终序号，越界钳到 [0, 组尾]——前端拖拽落点经 resolveTagMoveDrop 换算，
// 正常不越界，钳制为防御）。落位写自身 parent_id + sort_order，目标组全部兄弟
// 连续化重排为 0..n-1（重排整个兄弟列表而非只写落位标签，防断号失档）；跨父
// 移动时旧父下剩余兄弟同样连续化重排（消除断号，同父移动新旧父相同由目标组
// 重排覆盖）。防环沿用 updateTagFull 链路逻辑（事务内经 tx 读边集，收集与写入
// 同事务无「校验与落位之间树被改」窗口）。
func (s *assetsStore) moveTag(tagID int64, newParentID int64, index int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var oldParentID int64
	err = tx.QueryRow("SELECT COALESCE(parent_id, 0) FROM tags WHERE id = ?", tagID).Scan(&oldParentID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("标签不存在或已被删除，请刷新标签列表")
	}
	if err != nil {
		return err
	}
	if newParentID == tagID {
		return errors.New("父标签不能选择自身")
	}
	if newParentID != 0 {
		var parentExists int
		if err := tx.QueryRow("SELECT COUNT(*) FROM tags WHERE id = ?", newParentID).Scan(&parentExists); err != nil {
			return err
		}
		if parentExists == 0 {
			return errors.New("父标签不存在或已被删除，请刷新标签列表")
		}
		edges, err := listTagParentEdgesFrom(tx)
		if err != nil {
			return err
		}
		cur := newParentID
		visited := make(map[int64]bool)
		for cur != 0 {
			if cur == tagID {
				return errors.New("父标签不能选择自身或其后代（会形成循环引用）")
			}
			if visited[cur] {
				break // 存量数据异常成环兜底，防遍历死循环
			}
			visited[cur] = true
			cur = edges[cur]
		}
	}

	// 目标组最终兄弟序：现序（排除自身）+ 自身按 index 插入，整组连续化重排
	rest, err := siblingTagIDsTx(tx, newParentID, tagID)
	if err != nil {
		return err
	}
	if index < 0 {
		index = 0
	}
	if index > len(rest) {
		index = len(rest)
	}
	order := make([]int64, 0, len(rest)+1)
	order = append(order, rest[:index]...)
	order = append(order, tagID)
	order = append(order, rest[index:]...)
	if err := writeTagSiblingOrderTx(tx, newParentID, order, tagID); err != nil {
		return err
	}
	// 跨父移动：旧父剩余兄弟连续化重排（消除断号；同父移动跳过，目标组已覆盖）
	if oldParentID != newParentID {
		oldRest, err := siblingTagIDsTx(tx, oldParentID, tagID)
		if err != nil {
			return err
		}
		if err := writeTagSiblingOrderTx(tx, oldParentID, oldRest, tagID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// siblingTagIDsTx 读指定父下兄弟标签 ID 序（排除 excludeID，组内 sort_order 升序、
// 同序按 id——与 listTagsWithCounts 组内排序同口径，事务内消费）。
func siblingTagIDsTx(tx *sql.Tx, parentID, excludeID int64) ([]int64, error) {
	rows, err := tx.Query(
		"SELECT id FROM tags WHERE COALESCE(parent_id, 0) = ? AND id != ? ORDER BY sort_order, id",
		parentID, excludeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// writeTagSiblingOrderTx 兄弟列表连续化重排：按传入序写 sort_order = 0..n-1；
// movingID 行同时写 parent_id（移动落位），其余行仅调序号（movingID 不在列表内
// 时该分支自然不触发，旧父重排复用同函数）。
func writeTagSiblingOrderTx(tx *sql.Tx, parentID int64, order []int64, movingID int64) error {
	for i, id := range order {
		if id == movingID {
			if _, err := tx.Exec(
				"UPDATE tags SET parent_id = ?, sort_order = ? WHERE id = ?",
				nullTagParent(parentID), i, id,
			); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec("UPDATE tags SET sort_order = ? WHERE id = ?", i, id); err != nil {
			return err
		}
	}
	return nil
}

// deleteTagTree 级联删除标签子树（单事务）：先闭包收集自身 + 全部后代 ID 集，
// 显式删除两处绑定（file_tags/slide_tags——外键 ON DELETE CASCADE 为兜底，显式
// DELETE 对齐 clearIndexForRebuild 双保险先例，防 pragma 漂移静默漏清）后删标签
// 本体行。返回解除关联的绑定行数（文件绑定行数 + 页绑定行数之和——确认弹窗
// 影响面口径为「绑定行数」，同一条目绑多标签按行计，不做条目去重）。
//
// 子树收集在删除事务内经 tx 读边集执行（闭包计算复用 expandTagEdgesToClosure），
// 与删行同事务避免「收集与删除之间标签树被改」的窗口。
func (s *assetsStore) deleteTagTree(tagID int64) (int64, error) {
	var exists int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tags WHERE id = ?", tagID).Scan(&exists); err != nil {
		return 0, err
	}
	if exists == 0 {
		return 0, errors.New("标签不存在或已被删除，请刷新标签列表")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	edges, err := listTagParentEdgesFrom(tx)
	if err != nil {
		return 0, err
	}
	ids := expandTagEdgesToClosure(edges, []int64{tagID})
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	var affected int64
	for _, table := range []string{"file_tags", "slide_tags"} {
		res, err := tx.Exec("DELETE FROM "+table+" WHERE tag_id IN ("+placeholders+")", args...)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		affected += n
	}
	if _, err := tx.Exec("DELETE FROM tags WHERE id IN ("+placeholders+")", args...); err != nil {
		return 0, err
	}
	return affected, tx.Commit()
}

// listTagParentEdgesFrom 从 Queryer 读全量标签父子边（*sql.DB 与事务 *sql.Tx
// 共用单点，deleteTagTree 事务内子树收集依赖）。
func listTagParentEdgesFrom(q interface {
	Query(string, ...any) (*sql.Rows, error)
}) (map[int64]int64, error) {
	rows, err := q.Query("SELECT id, COALESCE(parent_id, 0) FROM tags")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	edges := make(map[int64]int64)
	for rows.Next() {
		var id, parentID int64
		if err := rows.Scan(&id, &parentID); err != nil {
			return nil, err
		}
		edges[id] = parentID
	}
	return edges, rows.Err()
}

// ---------- store 层：批量打标签 ----------

// ensureTagsByNames 批量前单点 ensure 全部添加标签名（PRD D7：批量内同名词
// 去重 ensure 一次——逐条目事务内各自 ensure 会重复走「查无 → 插入」路径，
// 批量开始一次 ensure 后逐条目复用 ID）。不存在名自动创建为根标签（parent_id
// NULL，默认色板按名确定性分配）。单事务包裹（失败整体回滚不留半建标签）。
func (s *assetsStore) ensureTagsByNames(names []string) (map[string]int64, error) {
	if len(names) == 0 {
		return map[string]int64{}, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	ids := make(map[string]int64, len(names))
	for _, name := range names {
		id, err := ensureTagTx(tx, name)
		if err != nil {
			return nil, err
		}
		ids[name] = id
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}

// resolveTagIDsByNames 标签名解析为 ID（移除集用）：不存在的名不进返回 map
// （移除不存在的标签 = 无操作语义）。QueryRow+Scan 逐名查询（单连接池自动
// 排空结果集，见 libraryStats 注释）。
func (s *assetsStore) resolveTagIDsByNames(names []string) (map[string]int64, error) {
	ids := make(map[string]int64, len(names))
	for _, name := range names {
		var id int64
		err := s.db.QueryRow("SELECT id FROM tags WHERE name = ?", name).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		ids[name] = id
	}
	return ids, nil
}

// prepareBatchTagIDs 批量前置解析单点：添加名 ensure（自动建根标签）、移除名
// 解析（不存在的名忽略）。返回按入参名序排列的 ID 列表（确定性）。
func (s *assetsStore) prepareBatchTagIDs(addNames, removeNames []string) (addIDs, removeIDs []int64, err error) {
	addMap, err := s.ensureTagsByNames(addNames)
	if err != nil {
		return nil, nil, err
	}
	for _, name := range addNames {
		addIDs = append(addIDs, addMap[name])
	}
	removeMap, err := s.resolveTagIDsByNames(removeNames)
	if err != nil {
		return nil, nil, err
	}
	for _, name := range removeNames {
		if id, ok := removeMap[name]; ok {
			removeIDs = append(removeIDs, id)
		}
	}
	return addIDs, removeIDs, nil
}

// batchUpdateFileTags 文件批量打标签（每条目一事务逐条目提交，PRD D3）：现标签
// 集相对语义「− remove ∪ add」，只动标签不动星级/备注（也不更新 files 行——
// updated_at 为索引时间戳语义，纯标签层变更不触碰）。条目失败（文件不存在/db
// 错误）记明细继续下一条，部分失败不中断不回滚已成功条目。
func (s *assetsStore) batchUpdateFileTags(fileIDs []int64, addNames, removeNames []string) (model.AssetBatchTagResult, error) {
	result := model.AssetBatchTagResult{Total: len(fileIDs), Items: []model.AssetBatchTagItem{}}
	addIDs, removeIDs, err := s.prepareBatchTagIDs(addNames, removeNames)
	if err != nil {
		return model.AssetBatchTagResult{}, err
	}
	for _, fileID := range fileIDs {
		if err := s.applyFileTagOp(fileID, addIDs, removeIDs); err != nil {
			result.Failed++
			result.Items = append(result.Items, model.AssetBatchTagItem{
				FileID:  fileID,
				Status:  model.AssetBatchTagStatusFailed,
				Message: err.Error(),
			})
			continue
		}
		result.Succeeded++
		result.Items = append(result.Items, model.AssetBatchTagItem{
			FileID: fileID,
			Status: model.AssetBatchTagStatusOK,
		})
	}
	return result, nil
}

// applyFileTagOp 单文件标签操作（单事务）：文件不存在返回可读错误（调用方记
// failed 明细继续）。绑定重建用「移除集逐条 DELETE + 添加集 INSERT OR IGNORE」
// 差异应用——相对语义下无需读现标签集（主键 (file_id, tag_id) 保证幂等）。
func (s *assetsStore) applyFileTagOp(fileID int64, addIDs, removeIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	if err := tx.QueryRow("SELECT COUNT(*) FROM files WHERE id = ?", fileID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("文件不存在或已被删除（fileId=%d），请刷新列表", fileID)
	}
	for _, tagID := range removeIDs {
		if _, err := tx.Exec("DELETE FROM file_tags WHERE file_id = ? AND tag_id = ?", fileID, tagID); err != nil {
			return err
		}
	}
	for _, tagID := range addIDs {
		if _, err := tx.Exec("INSERT OR IGNORE INTO file_tags(file_id, tag_id) VALUES(?, ?)", fileID, tagID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// batchUpdateSlideTags 页自有标签批量打标签（每条目一事务）：落 slide_tags 页
// 自有标签（键 (file_id, slide_num)，禁 slides.id 同 assets-management 契约）。
// 语义与明细口径对齐 batchUpdateFileTags；条目失败（文件/页不存在）记明细继续。
func (s *assetsStore) batchUpdateSlideTags(targets []model.AssetSlideTarget, addNames, removeNames []string) (model.AssetBatchTagResult, error) {
	result := model.AssetBatchTagResult{Total: len(targets), Items: []model.AssetBatchTagItem{}}
	addIDs, removeIDs, err := s.prepareBatchTagIDs(addNames, removeNames)
	if err != nil {
		return model.AssetBatchTagResult{}, err
	}
	for _, target := range targets {
		if err := s.applySlideTagOp(target.FileID, int64(target.SlideNum), addIDs, removeIDs); err != nil {
			result.Failed++
			result.Items = append(result.Items, model.AssetBatchTagItem{
				FileID:   target.FileID,
				SlideNum: target.SlideNum,
				Status:   model.AssetBatchTagStatusFailed,
				Message:  err.Error(),
			})
			continue
		}
		result.Succeeded++
		result.Items = append(result.Items, model.AssetBatchTagItem{
			FileID:   target.FileID,
			SlideNum: target.SlideNum,
			Status:   model.AssetBatchTagStatusOK,
		})
	}
	return result, nil
}

// applySlideTagOp 单页标签操作（单事务）：宿主行校验对齐 updateSlideMeta（文件
// 行 + 页行双重——页不存在属前端数据过期，记明细引导刷新）。slide_tags 变更不
// 写 FTS、不碰缩略图记账（纯标注层约束见文件头）。
func (s *assetsStore) applySlideTagOp(fileID, slideNum int64, addIDs, removeIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	if err := tx.QueryRow("SELECT COUNT(*) FROM files WHERE id = ?", fileID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("文件不存在或已被删除（fileId=%d），请刷新列表", fileID)
	}
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM slides WHERE file_id = ? AND slide_num = ?", fileID, slideNum,
	).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("幻灯片页不存在或已被重新解析（fileId=%d，slideNum=%d），请刷新列表", fileID, slideNum)
	}
	for _, tagID := range removeIDs {
		if _, err := tx.Exec(
			"DELETE FROM slide_tags WHERE file_id = ? AND slide_num = ? AND tag_id = ?", fileID, slideNum, tagID,
		); err != nil {
			return err
		}
	}
	for _, tagID := range addIDs {
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO slide_tags(file_id, slide_num, tag_id) VALUES(?, ?, ?)", fileID, slideNum, tagID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- service 层：编排与参数归一 ----------

// normalizeBatchTagOp 批量操作参数归一单点（两批量入口共用）：标签名去首尾空白、
// 批内去重（保序）；校验双全空与交集非空拒绝。错误为普通参数错误（跟随既有
// 参数校验形态，前端按 message 展示）。
func normalizeBatchTagOp(op model.AssetBatchTagOp) (addNames, removeNames []string, err error) {
	addNames = dedupTrimTagNames(op.AddTags)
	removeNames = dedupTrimTagNames(op.RemoveTags)
	if len(addNames) == 0 && len(removeNames) == 0 {
		return nil, nil, errors.New("请至少选择一个要添加或移除的标签")
	}
	removeSet := make(map[string]bool, len(removeNames))
	for _, name := range removeNames {
		removeSet[name] = true
	}
	for _, name := range addNames {
		if removeSet[name] {
			return nil, nil, fmt.Errorf("标签「%s」不能同时添加与移除", name)
		}
	}
	return addNames, removeNames, nil
}

// dedupTrimTagNames 标签名去空白去重（保序，对齐 updateFileMeta 的 seen 去重口径）。
func dedupTrimTagNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// ListTags 全量标签清单（父级分组 + 组内 sort_order 序，含直接绑定计数）：标签
// 树构建数据源（含未使用标签），替代前端从文件底册聚合 tagOptions 的既有明账
// （PRD D1）；组内兄弟序即拖拽排序契约的展示序（PRD D8#1，见 listTagsWithCounts）。
func (s *AssetLibraryService) ListTags(libraryID string) ([]model.AssetTagListItem, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return nil, err
	}
	var items []model.AssetTagListItem
	err := s.withStore(libraryID, func(store *assetsStore) (err error) {
		items, err = store.listTagsWithCounts()
		return err
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// CreateTag 新建标签：重名拒绝；parentID ≠ 0 须为已存在标签（新建无子代不存在
// 成环）；color 空则按标签名确定性分配默认色板。
func (s *AssetLibraryService) CreateTag(libraryID, name string, parentID int64, color string) (model.AssetTag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.AssetTag{}, errors.New("请输入标签名")
	}
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetTag{}, err
	}
	var tag model.AssetTag
	err := s.withStore(libraryID, func(store *assetsStore) (err error) {
		tag, err = store.createTag(name, parentID, color)
		return err
	})
	if err != nil {
		return model.AssetTag{}, err
	}
	return tag, nil
}

// UpdateTag 标签编辑（重命名/改色/移动父级整体提交）：重名拒绝；移动父级防环
// （新父不能是自身或自身后代）。
func (s *AssetLibraryService) UpdateTag(libraryID string, tagID int64, name string, parentID int64, color string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("请输入标签名")
	}
	if _, err := s.getLibrary(libraryID); err != nil {
		return err
	}
	return s.withStore(libraryID, func(store *assetsStore) error {
		return store.updateTagFull(tagID, name, parentID, color)
	})
}

// DeleteTag 删除标签：级联删除自身 + 全部后代并解除全部绑定（删除标签不删素材，
// 单事务）。返回解除关联的绑定行数（确认弹窗影响面口径，见 deleteTagTree 注释）。
func (s *AssetLibraryService) DeleteTag(libraryID string, tagID int64) (int64, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return 0, err
	}
	var affected int64
	err := s.withStore(libraryID, func(store *assetsStore) (err error) {
		affected, err = store.deleteTagTree(tagID)
		return err
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// MoveTag 标签拖拽排序/移动父级（PRD D8#1 真机增量）：index 语义 = 落位后自身在
// 目标父下兄弟组内的最终序号（前端拖拽落点经 resolveTagMoveDrop 换算：before/
// after = 同级排序、inner = 变子级组尾），事务内防环 + 目标组与旧组兄弟连续化
// 重排，见 store.moveTag 注释。
func (s *AssetLibraryService) MoveTag(libraryID string, tagID int64, newParentID int64, index int) error {
	if _, err := s.getLibrary(libraryID); err != nil {
		return err
	}
	return s.withStore(libraryID, func(store *assetsStore) error {
		return store.moveTag(tagID, newParentID, index)
	})
}

// BatchUpdateFileTags 文件批量打标签（SRS §2.4，文件模式选中集操作）：op 相对
// 语义见 AssetBatchTagOp。参数校验（交集/双空/条目空）为普通参数错误；批量逐
// 条目事务，部分失败以明细返回（不报错、不回滚成功条目）。
func (s *AssetLibraryService) BatchUpdateFileTags(libraryID string, fileIDs []int64, op model.AssetBatchTagOp) (model.AssetBatchTagResult, error) {
	addNames, removeNames, err := normalizeBatchTagOp(op)
	if err != nil {
		return model.AssetBatchTagResult{}, err
	}
	if len(fileIDs) == 0 {
		return model.AssetBatchTagResult{}, errors.New("请先选择要打标签的文件")
	}
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetBatchTagResult{}, err
	}
	var result model.AssetBatchTagResult
	err = s.withStore(libraryID, func(store *assetsStore) (err error) {
		result, err = store.batchUpdateFileTags(fileIDs, addNames, removeNames)
		return err
	})
	if err != nil {
		return model.AssetBatchTagResult{}, err
	}
	return result, nil
}

// BatchUpdateSlideTags 页自有标签批量打标签（幻灯片模式选中集操作）：目标键
// (fileId, slideNum)。纯标注层——不触发重解析与缩略图失效，检索命中口径由筛选
// 路径实时读关系表同步。slideNum < 1 为调用方构造错误，整体参数拒绝（区别于
// 数据态的「页不存在」逐条目明细）。
func (s *AssetLibraryService) BatchUpdateSlideTags(libraryID string, targets []model.AssetSlideTarget, op model.AssetBatchTagOp) (model.AssetBatchTagResult, error) {
	addNames, removeNames, err := normalizeBatchTagOp(op)
	if err != nil {
		return model.AssetBatchTagResult{}, err
	}
	if len(targets) == 0 {
		return model.AssetBatchTagResult{}, errors.New("请先选择要打标签的幻灯片页")
	}
	for _, target := range targets {
		if target.SlideNum < 1 {
			return model.AssetBatchTagResult{}, fmt.Errorf("页序须从 1 计（fileId=%d，slideNum=%d）", target.FileID, target.SlideNum)
		}
	}
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetBatchTagResult{}, err
	}
	var result model.AssetBatchTagResult
	err = s.withStore(libraryID, func(store *assetsStore) (err error) {
		result, err = store.batchUpdateSlideTags(targets, addNames, removeNames)
		return err
	})
	if err != nil {
		return model.AssetBatchTagResult{}, err
	}
	return result, nil
}
