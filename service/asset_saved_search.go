package service

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// AssetSavedSearchService 保存的素材检索条件服务（SRS §2.5，素材管理第七批）。
//
// 存储 data/asset_saved_searches.json（全局单文件，条目带 libraryId 库归属），
// 走 util.SaveJSON 原子写（temp → fsync → rename，docs/spec/data-atomic-write.md
// 契约，禁 os.WriteFile 截断写）。整体覆盖模型（内存列表全量序列化覆盖文件）：
// last-writer-wins，与 settings.json 同口径——运行期手改该文件前须关闭 workbench.exe。
//
// 损坏降级：读取失败（JSON 非法/半截）回退空列表并 slog.Warn，不阻塞保存链路
// （复用 SettingsService 损坏降级模式；降级后下一次 Save 即以空列表重建文件）。
//
// 并发口径：mu 保护「读-改-写」全程（List 返回深拷贝快照；Save/Delete 全程持锁
// 落盘），与 SettingsService「锁跨落盘全程」用法对齐；无内存缓存——保存/调取/
// 删除均为低频 UI 操作，每次读盘换取无缓存一致性负担。
type AssetSavedSearchService struct {
	configPath string

	mu       sync.Mutex
	loaded   bool                     // 首次 List/Save 懒加载标记（与素材库注册表同模式）
	searches []model.AssetSavedSearch // 内存态（变更即时原子落盘）
}

// assetSavedSearchesJSON 落盘文件顶层结构（对象包装便于后续扩展伴随字段，
// 对齐素材库注册表 assetLibraryRegistryJSON 先例）。
type assetSavedSearchesJSON struct {
	Searches []model.AssetSavedSearch `json:"searches"`
}

// NewAssetSavedSearchService 构造保存检索条件服务（configPath 通常为
// <dataDir>/asset_saved_searches.json）。
func NewAssetSavedSearchService(configPath string) *AssetSavedSearchService {
	return &AssetSavedSearchService{configPath: configPath}
}

// List 全部保存的检索条件（按保存顺序，即 ID 升序）。返回深拷贝快照，调用方
// 修改不影响服务内存态。
func (s *AssetSavedSearchService) List() ([]model.AssetSavedSearch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return nil, err
	}
	out := make([]model.AssetSavedSearch, len(s.searches))
	copy(out, s.searches)
	return out, nil
}

// Save 保存检索条件（upsert 语义）：Search.ID 为 0 = 新增（后端生成自增 ID 与
// createdAt，忽略调用方传入的同名字段）；非 0 = 更新覆盖（保留原 createdAt，
// 不存在报错）。Name 全局唯一（TrimSpace 后非空 + 重名拒绝，重名判定排除自身
// ID——覆盖自身现名不拒绝）。TagIDs/TagNames 双份数据由调用方（前端）同步提交，
// 服务层不做两份数据的一致性校验（跨库重映射消费侧按名优先）。
func (s *AssetSavedSearchService) Save(search model.AssetSavedSearch) (model.AssetSavedSearch, error) {
	search.Name = strings.TrimSpace(search.Name)
	if search.Name == "" {
		return model.AssetSavedSearch{}, errors.New("请输入保存条件名称")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return model.AssetSavedSearch{}, err
	}

	if search.ID == 0 {
		// 新增：自增 ID = 现有最大 ID + 1（删除最大项后复用其 ID 无妨——该项已删，
		// 无悬挂引用）；createdAt 后端权威生成
		nextID := int64(0)
		for _, item := range s.searches {
			if item.ID > nextID {
				nextID = item.ID
			}
		}
		search.ID = nextID + 1
		search.CreatedAt = time.Now().Format(time.RFC3339)
		if err := rejectDuplicateNameLocked(s.searches, search.Name, search.ID); err != nil {
			return model.AssetSavedSearch{}, err
		}
		s.searches = append(s.searches, search)
	} else {
		// 更新覆盖：按 ID 定位整体替换，保留原 createdAt（创建时间不随覆盖漂移）
		idx := -1
		for i := range s.searches {
			if s.searches[i].ID == search.ID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return model.AssetSavedSearch{}, fmt.Errorf("保存条件不存在或已被删除（ID=%d），请刷新列表", search.ID)
		}
		if err := rejectDuplicateNameLocked(s.searches, search.Name, search.ID); err != nil {
			return model.AssetSavedSearch{}, err
		}
		search.CreatedAt = s.searches[idx].CreatedAt
		s.searches[idx] = search
	}

	if err := s.saveLocked(); err != nil {
		return model.AssetSavedSearch{}, err
	}
	return search, nil
}

// Delete 删除指定保存条件（不存在报错，前端列表过期时刷新重试）。
func (s *AssetSavedSearchService) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	idx := -1
	for i := range s.searches {
		if s.searches[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("保存条件不存在或已被删除（ID=%d），请刷新列表", id)
	}
	s.searches = append(s.searches[:idx], s.searches[idx+1:]...)
	return s.saveLocked()
}

// rejectDuplicateNameLocked 重名拒绝（全局唯一，排除自身 ID——覆盖自身现名不
// 拒绝），须持 s.mu。
func rejectDuplicateNameLocked(searches []model.AssetSavedSearch, name string, selfID int64) error {
	for _, item := range searches {
		if item.ID != selfID && item.Name == name {
			return fmt.Errorf("已存在同名保存条件「%s」，请换一个名称或直接覆盖原条件", name)
		}
	}
	return nil
}

// loadLocked 懒加载（文件不存在 = 首次使用，空列表；损坏降级空列表 + 告警日志，
// 不阻塞调用链路），须持 s.mu。
func (s *AssetSavedSearchService) loadLocked() error {
	if s.loaded {
		return nil
	}
	if !util.FileExists(s.configPath) {
		// 首次使用：文件尚不存在，空列表
		s.searches = []model.AssetSavedSearch{}
		s.loaded = true
		return nil
	}
	var data assetSavedSearchesJSON
	if err := util.LoadJSON(s.configPath, &data); err != nil {
		// 损坏降级：回退空列表（下一次 Save 即以空列表重建文件），不阻塞调用链路
		Logger().Warn("保存检索条件文件读取失败，降级为空列表", "path", s.configPath, "err", err)
		s.searches = []model.AssetSavedSearch{}
		s.loaded = true
		return nil
	}
	if data.Searches == nil {
		data.Searches = []model.AssetSavedSearch{}
	}
	s.searches = data.Searches
	s.loaded = true
	return nil
}

// saveLocked 原子落盘（util.SaveJSON：temp → fsync → rename，禁 os.WriteFile
// 直写），须持 s.mu。
func (s *AssetSavedSearchService) saveLocked() error {
	if err := util.SaveJSON(s.configPath, assetSavedSearchesJSON{Searches: s.searches}); err != nil {
		return fmt.Errorf("保存检索条件失败: %w", err)
	}
	return nil
}
