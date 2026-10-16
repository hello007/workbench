package service

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"workbench/model"
)

// 素材批量评分（素材管理第八批，SRS §2.6 P0）。
//
// 文件组织：本文件承载评分批量域完整链路——assetsStore 层数据访问（单连接池
// 自锁纪律见 assets_db.go libraryStats 注释）与 AssetLibraryService 编排（库校验 +
// 参数校验），模式对齐 assets_tags.go 的域内聚先例。
//
// 契约（任务 10-16-srs-2-5-3-srs-2-6-p0 PRD D4/D5）：
//   - 单一入口混合两类目标：SlideNum = 0 评文件（files.rating）；≥ 1 评页
//     （slide_meta.rating 页级标注）。目标键 = (fileId, slideNum)，禁 slides.id
//     （每轮重解析删旧插新全变，与 slide_tags/slide_meta 存储键同口径）；
//   - 评分口径：rating 0-5，0 = 清除评分（与 UpdateFileMeta/UpdateSlideMeta
//     单条语义一致）；页级 upsert 保留业务备注（只改 rating 列，备注列回写现值）；
//   - 事务边界 = 每条目一事务逐条目提交（对齐批量打标签 splitOutcome 明细先例），
//     条目失败（文件/页不存在、db 错误）记明细继续，部分失败不中断不回滚；
//   - 纯标注层约束（对齐批量打标签文件头）：评分不入 FTS（检索列不含星级）、
//     不碰 thumb_cache、不触发重解析——facet 星级命中与排序口径由检索路径实时
//     读 files.rating / slide_meta.rating 天然同步。

// ---------- store 层：批量评分 ----------

// batchSetRating 批量评分（每条目一事务逐条目提交）：条目失败记明细继续。
// 每条目 Label 在条目自身事务内从存在性校验顺带读出（SELECT path 替代 COUNT(*)，
// 同一次查询既校验存在又取定位文案，免二次查询），页条目追加页码后缀。
func (s *assetsStore) batchSetRating(targets []model.AssetRatingTarget, rating int) (model.AssetBatchRatingResult, error) {
	result := model.AssetBatchRatingResult{Total: len(targets), Items: []model.AssetBatchRatingItem{}}
	// 批次级统一时间戳：对齐 updateFileMeta 单条评分链路的 updated_at 写入口径
	//（同一操作单条/批量语义一致），批量内恒定不逐条目取时
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, target := range targets {
		label, err := s.applyRatingTarget(target.FileID, int64(target.SlideNum), rating, now)
		if err != nil {
			result.Failed++
			result.Items = append(result.Items, model.AssetBatchRatingItem{
				FileID:   target.FileID,
				SlideNum: target.SlideNum,
				Status:   model.AssetBatchTagStatusFailed,
				Message:  err.Error(),
				Label:    label,
			})
			continue
		}
		result.Succeeded++
		result.Items = append(result.Items, model.AssetBatchRatingItem{
			FileID:   target.FileID,
			SlideNum: target.SlideNum,
			Status:   model.AssetBatchTagStatusOK,
			Label:    label,
		})
	}
	return result, nil
}

// applyRatingTarget 单条目评分（单事务）：文件条目 UPDATE files.rating；页条目
// 校验宿主文件行 + 页行后 upsert slide_meta（INSERT OR REPLACE 回写现存业务备注，
// 只改 rating 列——整行 REPLACE 需带全部列值，漏回写备注会清空页业务备注）。
// 返回定位展示 Label；目标不存在返回可读错误（调用方记 failed 明细继续）。
// 文件行存在性校验与 Label 拼装共用同一次 path 查询（见 batchSetRating 注释）。
func (s *assetsStore) applyRatingTarget(fileID, slideNum int64, rating int, now string) (string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var path string
	err = tx.QueryRow("SELECT path FROM files WHERE id = ?", fileID).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("文件不存在或已被删除（fileId=%d），请刷新列表", fileID)
	}
	if err != nil {
		return "", err
	}
	label := path
	if slideNum == 0 {
		if _, err := tx.Exec(
			"UPDATE files SET rating = ?, updated_at = ? WHERE id = ?", rating, now, fileID,
		); err != nil {
			return "", err
		}
		return label, tx.Commit()
	}

	// 页条目：页行校验对齐 updateSlideMeta（页不存在属前端数据过期，引导刷新）
	label = fmt.Sprintf("%s · 第 %d 页", path, slideNum)
	var exists int
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM slides WHERE file_id = ? AND slide_num = ?", fileID, slideNum,
	).Scan(&exists); err != nil {
		return label, err
	}
	if exists == 0 {
		return label, fmt.Errorf("幻灯片页不存在或已被重新解析（fileId=%d，slideNum=%d），请刷新列表", fileID, slideNum)
	}
	// 业务备注保留：现存行读回 note（无行视为空串），整行 REPLACE 回写
	var note string
	if err := tx.QueryRow(
		"SELECT note FROM slide_meta WHERE file_id = ? AND slide_num = ?", fileID, slideNum,
	).Scan(&note); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return label, err
	}
	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO slide_meta(file_id, slide_num, note, rating) VALUES(?, ?, ?, ?)",
		fileID, slideNum, note, rating,
	); err != nil {
		return label, err
	}
	return label, tx.Commit()
}

// ---------- service 层：编排与参数校验 ----------

// BatchSetRating 批量修改评分（SRS §2.6 P0，选中集操作）：单一入口混合文件条目
// （SlideNum = 0 → files.rating）与页条目（≥ 1 → slide_meta.rating，保留业务备注）。
// rating 0-5，0 = 清除评分；越界为整体参数错误（前端星级控件 0-5 档位为第一防线，
// 此为后端双防线）。每条目一事务逐条目提交，部分失败以明细返回（不报错、不回滚
// 成功条目）。
func (s *AssetLibraryService) BatchSetRating(libraryID string, targets []model.AssetRatingTarget, rating int64) (model.AssetBatchRatingResult, error) {
	if rating < 0 || rating > 5 {
		return model.AssetBatchRatingResult{}, errors.New("评分须在 0-5 星之间（0 = 清除评分）")
	}
	if len(targets) == 0 {
		return model.AssetBatchRatingResult{}, errors.New("请先选择要评分的条目")
	}
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetBatchRatingResult{}, err
	}
	var result model.AssetBatchRatingResult
	err := s.withStore(libraryID, func(store *assetsStore) (err error) {
		result, err = store.batchSetRating(targets, int(rating))
		return err
	})
	if err != nil {
		return model.AssetBatchRatingResult{}, err
	}
	return result, nil
}
