package service

import (
	"encoding/json"
	"fmt"
	"time"

	"workbench/model"
	"workbench/util"
)

// RepoConfigService 仓库列表配置（工作目录 + 收藏夹）导入导出服务。
//
// 聚合 DirectoryService 与 FavoritesService 两个数据源：导出组装 manifest JSON
// 文本（落盘由前端经 SaveFileDialog + SaveFile 完成）；导入解析 manifest 生成
// 预览（不落盘）与按决策执行合并。导入写路径独立于 Add/Create 的 path 唯一
// 约束实现（「另存为新项」需同 path 并存），不改动现有方法语义。
type RepoConfigService struct {
	directorySvc *DirectoryService
	favoritesSvc *FavoritesService
}

// NewRepoConfigService 创建仓库列表配置导入导出服务（纯构造，两依赖均只读复用）。
func NewRepoConfigService(directorySvc *DirectoryService, favoritesSvc *FavoritesService) *RepoConfigService {
	return &RepoConfigService{
		directorySvc: directorySvc,
		favoritesSvc: favoritesSvc,
	}
}

// Export 导出仓库列表配置为 manifest JSON 文本（带 manifestVersion）。
// 仅含业务字段：工作目录 name/path/isDefault，收藏夹 path/alias/group/createdAt；
// 运行时态（IsGitRepo/HasRemote）与本机标识（Directory.ID/CreateTime）不导出，
// 导入侧按本机重算/重生成，保证跨设备语义正确。
func (s *RepoConfigService) Export() (string, error) {
	directories, err := s.directorySvc.Load()
	if err != nil {
		return "", fmt.Errorf("读取工作目录配置失败: %w", err)
	}
	favorites, err := s.favoritesSvc.Load()
	if err != nil {
		return "", fmt.Errorf("读取收藏夹配置失败: %w", err)
	}

	manifest := &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion,
		ExportedAt:      time.Now(),
		Directories:     make([]*model.RepoConfigDirectory, 0, len(directories)),
		Favorites:       make([]*model.RepoConfigFavorite, 0, len(favorites)),
	}
	for _, d := range directories {
		manifest.Directories = append(manifest.Directories, &model.RepoConfigDirectory{
			Name:      d.Name,
			Path:      d.Path,
			IsDefault: d.IsDefault,
		})
	}
	for _, f := range favorites {
		manifest.Favorites = append(manifest.Favorites, &model.RepoConfigFavorite{
			Path:      f.Path,
			Alias:     f.Alias,
			Group:     f.Group,
			CreatedAt: f.CreatedAt,
		})
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化仓库列表配置失败: %w", err)
	}
	return string(data), nil
}

// PreviewImport 解析导入文件并生成预览（不落盘）。
// 全局校验（JSON 结构 / manifestVersion）失败返回 AppError 整体拒绝；
// 逐项校验（字段缺失 / 路径在本机不存在）失败列入 Invalid，不阻断合法项。
// 冲突判定键为 path：工作目录按规范化绝对路径，收藏夹按原 path。
func (s *RepoConfigService) PreviewImport(jsonText string) (*model.RepoConfigImportPreview, error) {
	parsed, err := s.parseManifest(jsonText)
	if err != nil {
		return nil, err
	}

	directories, err := s.directorySvc.Load()
	if err != nil {
		return nil, fmt.Errorf("读取本机工作目录配置失败: %w", err)
	}
	favorites, err := s.favoritesSvc.Load()
	if err != nil {
		return nil, fmt.Errorf("读取本机收藏夹配置失败: %w", err)
	}
	localDirsByPath := dirPathSet(directories)
	localFavByPath := favPathSet(favorites)

	preview := &model.RepoConfigImportPreview{}
	for _, d := range parsed.Directories {
		if reason := validateImportDirectory(d); reason != "" {
			preview.Invalid = append(preview.Invalid, invalidItem("directory", d.Path, d.Name, reason))
			continue
		}
		item := &model.RepoConfigDirectoryPreview{Name: d.Name, Path: d.Path, IsDefault: d.IsDefault}
		if localDirsByPath[normalizePath(d.Path)] {
			preview.ConflictDirectories = append(preview.ConflictDirectories, item)
		} else {
			preview.NewDirectories = append(preview.NewDirectories, item)
		}
	}
	for _, f := range parsed.Favorites {
		if reason := validateImportFavorite(f); reason != "" {
			preview.Invalid = append(preview.Invalid, invalidItem("favorite", f.Path, f.Alias, reason))
			continue
		}
		item := &model.RepoConfigFavoritePreview{Path: f.Path, Alias: f.Alias, Group: f.Group, CreatedAt: f.CreatedAt}
		if _, ok := localFavByPath[f.Path]; ok {
			preview.ConflictFavorites = append(preview.ConflictFavorites, item)
		} else {
			preview.NewFavorites = append(preview.NewFavorites, item)
		}
	}
	return preview, nil
}

// ApplyImport 按用户决策执行导入合并并落盘（merge 语义：以本机两列表为基底）。
// 新增项追加（工作目录重新生成 ID 并重算 git 状态）；冲突项按决策跳过/覆盖
// （保留本机 ID）/另存为新项（新 ID + 名称追加「（导入）」后缀，isDefault 强制
// false 避免抢占本机默认目录）；收藏夹同 path 可并存。返回计数汇总。
func (s *RepoConfigService) ApplyImport(jsonText string, decisions *model.RepoConfigImportDecisions) (*model.RepoConfigImportResult, error) {
	parsed, err := s.parseManifest(jsonText)
	if err != nil {
		return nil, err
	}
	if decisions == nil {
		decisions = &model.RepoConfigImportDecisions{}
	}

	result := &model.RepoConfigImportResult{}

	directories, err := s.directorySvc.Load()
	if err != nil {
		return nil, fmt.Errorf("读取本机工作目录配置失败: %w", err)
	}
	directories, err = s.mergeDirectories(directories, parsed.Directories, decisions.Directories, result)
	if err != nil {
		return nil, err
	}
	if err := s.directorySvc.Save(directories); err != nil {
		return nil, fmt.Errorf("保存工作目录配置失败: %w", err)
	}

	favorites, err := s.favoritesSvc.Load()
	if err != nil {
		return nil, fmt.Errorf("读取本机收藏夹配置失败: %w", err)
	}
	favorites = s.mergeFavorites(favorites, parsed.Favorites, decisions.Favorites, result)
	if err := s.favoritesSvc.save(favorites); err != nil {
		return nil, fmt.Errorf("保存收藏夹配置失败: %w", err)
	}
	return result, nil
}

// parseManifest 解析并做全局校验：JSON 非法 → InvalidJSON；版本缺失或高于
// 当前支持 → UnsupportedVersion。低于当前版本的合法历史结构按现有字段解析
// （缺失字段零值，天然兼容）。
func (s *RepoConfigService) parseManifest(jsonText string) (*model.RepoConfigManifest, error) {
	var manifest model.RepoConfigManifest
	if err := json.Unmarshal([]byte(jsonText), &manifest); err != nil {
		return nil, model.WrapAppError(model.ErrCodeRepoConfigInvalidJSON,
			"导入文件不是合法的仓库列表配置 JSON", err)
	}
	if manifest.ManifestVersion <= 0 {
		return nil, model.NewAppError(model.ErrCodeRepoConfigUnsupportedVersion,
			"导入文件缺少 manifestVersion 字段，无法识别配置版本")
	}
	if manifest.ManifestVersion > model.RepoConfigManifestVersion {
		return nil, model.NewAppError(model.ErrCodeRepoConfigUnsupportedVersion,
			fmt.Sprintf("导入文件版本 %d 高于当前支持的版本 %d，请升级应用后重试",
				manifest.ManifestVersion, model.RepoConfigManifestVersion))
	}
	return &manifest, nil
}

// mergeDirectories 将导入的工作目录按决策合并进本机列表，返回合并后列表。
// 另存为新项的条目 isDefault 强制 false；任一导入项 isDefault=true 生效时
// 清除本机其余 default，保证唯一默认约束。
func (s *RepoConfigService) mergeDirectories(local []*model.Directory, imported []*model.RepoConfigDirectory, decisions map[string]string, result *model.RepoConfigImportResult) ([]*model.Directory, error) {
	byPath := make(map[string]*model.Directory, len(local))
	for _, d := range local {
		byPath[normalizePath(d.Path)] = d
	}

	gitCmd := util.NewGitCommand()
	for _, d := range imported {
		if reason := validateImportDirectory(d); reason != "" {
			result.Failed++
			result.FailedReasons = append(result.FailedReasons, fmt.Sprintf("工作目录 %s：%s", d.Path, reason))
			continue
		}
		decision := decisions[d.Path]
		storePath := normalizePath(d.Path) // 入库前规范化（绝对化+清理），兼容手改 JSON 的相对路径写法
		existing, exists := byPath[storePath]
		switch {
		case !exists:
			// 新增：生成新 ID 并按 Create 先例重算运行时态（IsGitRepo/HasRemote）
			newDir := model.NewDirectory(d.Name, storePath, false)
			newDir.IsGitRepo = gitCmd.IsGitRepository(newDir.Path)
			if newDir.IsGitRepo {
				if _, _, err := gitCmd.GetRemote(newDir.Path); err == nil {
					newDir.HasRemote = true
				}
			}
			local = append(local, newDir)
			byPath[storePath] = newDir
			if d.IsDefault {
				applyExclusiveDefault(local, newDir)
			}
			result.Added++
		case decision == model.ImportDecisionOverwrite:
			// 覆盖：保留本机 ID 与 CreateTime，业务字段取导入值，重算运行时态
			existing.Name = d.Name
			if d.IsDefault && !existing.IsDefault {
				applyExclusiveDefault(local, existing)
			}
			existing.IsDefault = d.IsDefault
			existing.IsGitRepo = gitCmd.IsGitRepository(existing.Path)
			if existing.IsGitRepo {
				if _, _, err := gitCmd.GetRemote(existing.Path); err == nil {
					existing.HasRemote = true
				}
			}
			result.Overwritten++
		case decision == model.ImportDecisionSaveAsNew:
			// 另存为新项：本机保留，导入项以新身份并存；同 path 场景 isDefault
			// 强制 false（并存语义下不抢占默认目录），名称加后缀区分。
			// 入库路径与新增分支一致用规范化后的 storePath，保证 git 检测不依赖进程工作目录
			newDir := model.NewDirectory(d.Name+"（导入）", storePath, false)
			newDir.IsGitRepo = gitCmd.IsGitRepository(newDir.Path)
			if newDir.IsGitRepo {
				if _, _, err := gitCmd.GetRemote(newDir.Path); err == nil {
					newDir.HasRemote = true
				}
			}
			local = append(local, newDir)
			byPath[storePath] = newDir
			result.Added++
		default:
			// 新增项无决策、冲突项默认跳过
			result.Skipped++
		}
	}
	return local, nil
}

// mergeFavorites 将导入的收藏夹条目按决策合并进本机列表，返回合并后列表。
// 收藏夹上限 100 条沿用现有约束：放不下的导入项计入失败（不静默截断）。
func (s *RepoConfigService) mergeFavorites(local []*model.Favorite, imported []*model.RepoConfigFavorite, decisions map[string]string, result *model.RepoConfigImportResult) []*model.Favorite {
	byPath := make(map[string]*model.Favorite, len(local))
	for _, f := range local {
		byPath[f.Path] = f
	}

	for _, f := range imported {
		if reason := validateImportFavorite(f); reason != "" {
			result.Failed++
			result.FailedReasons = append(result.FailedReasons, fmt.Sprintf("收藏 %s：%s", f.Path, reason))
			continue
		}
		decision := decisions[f.Path]
		existing, conflict := byPath[f.Path]
		switch {
		case !conflict:
			if len(local) >= favoriteLimit {
				result.Failed++
				result.FailedReasons = append(result.FailedReasons,
					fmt.Sprintf("收藏 %s：收藏夹已满（最多 %d 条）", f.Path, favoriteLimit))
				continue
			}
			group := f.Group
			if group == "" {
				group = "默认"
			}
			local = append(local, &model.Favorite{Path: f.Path, Alias: f.Alias, Group: group, CreatedAt: f.CreatedAt})
			byPath[f.Path] = local[len(local)-1]
			result.Added++
		case decision == model.ImportDecisionOverwrite:
			existing.Alias = f.Alias
			existing.Group = f.Group
			if f.Group == "" {
				existing.Group = "默认"
			}
			result.Overwritten++
		case decision == model.ImportDecisionSaveAsNew:
			if len(local) >= favoriteLimit {
				result.Failed++
				result.FailedReasons = append(result.FailedReasons,
					fmt.Sprintf("收藏 %s：收藏夹已满（最多 %d 条）", f.Path, favoriteLimit))
				continue
			}
			// 同 path 并存不更新 byPath（仍指向本机条目，供后续重复导入项冲突判定）
			group := f.Group
			if group == "" {
				group = "默认"
			}
			local = append(local, &model.Favorite{Path: f.Path, Alias: f.Alias, Group: group, CreatedAt: f.CreatedAt})
			result.Added++
		default:
			result.Skipped++
		}
	}
	return local
}

// favoriteLimit 收藏夹条数上限（与 FavoritesService.Add 保持一致）。
const favoriteLimit = 100

// validateImportDirectory 逐项校验导入工作目录，非法返回原因，合法返回空串。
// 路径须在本机存在（跨设备导入时未检出的仓库路径无法使用，列入非法项提示）。
func validateImportDirectory(d *model.RepoConfigDirectory) string {
	if d == nil {
		return "条目为空"
	}
	if d.Path == "" {
		return "路径为空"
	}
	if !util.FileExists(d.Path) {
		return "路径在本机不存在"
	}
	return ""
}

// validateImportFavorite 逐项校验导入收藏条目，非法返回原因，合法返回空串。
func validateImportFavorite(f *model.RepoConfigFavorite) string {
	if f == nil {
		return "条目为空"
	}
	if f.Path == "" {
		return "路径为空"
	}
	if !util.FileExists(f.Path) {
		return "路径在本机不存在"
	}
	return ""
}

// invalidItem 构造非法项，Name 优先取展示路径，为空时回退名称/别名字段。
func invalidItem(kind, path, fallback string, reason string) *model.RepoConfigInvalidItem {
	name := path
	if name == "" {
		name = fallback
	}
	return &model.RepoConfigInvalidItem{Kind: kind, Name: name, Reason: reason}
}

// normalizePath 复用 repo_meta.go 同名函数（绝对化为主键语义，与
// DirectoryService.Create 的 filepath.Abs 一致），保证冲突判定跨写法稳定。

// dirPathSet 本机工作目录 path 集合（规范化后）。
func dirPathSet(directories []*model.Directory) map[string]bool {
	set := make(map[string]bool, len(directories))
	for _, d := range directories {
		set[normalizePath(d.Path)] = true
	}
	return set
}

// favPathSet 本机收藏 path 集合。
func favPathSet(favorites []*model.Favorite) map[string]struct{} {
	set := make(map[string]struct{}, len(favorites))
	for _, f := range favorites {
		set[f.Path] = struct{}{}
	}
	return set
}

// applyExclusiveDefault 将 target 设为唯一默认目录，清除列表内其余 IsDefault。
func applyExclusiveDefault(directories []*model.Directory, target *model.Directory) {
	for _, d := range directories {
		d.IsDefault = d == target
	}
	target.IsDefault = true
}
