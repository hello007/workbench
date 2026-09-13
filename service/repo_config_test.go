package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// createRepoConfigTestService 创建测试用 RepoConfigService，两个配置文件均在临时目录。
// 返回服务与数据目录路径（供创建本机存在的测试路径）。
func createRepoConfigTestService(t *testing.T) (*RepoConfigService, string) {
	t.Helper()
	dataDir := t.TempDir()
	dirSvc := NewDirectoryService(filepath.Join(dataDir, "directories.json"))
	favSvc := NewFavoritesService(filepath.Join(dataDir, "favorites.json"))
	return NewRepoConfigService(dirSvc, favSvc), dataDir
}

// writeLocalDirs 直写本机工作目录列表（绕过 Create 的 git 检测，控制测试前置状态）。
func writeLocalDirs(t *testing.T, svc *RepoConfigService, dirs []*model.Directory) {
	t.Helper()
	if err := svc.directorySvc.Save(dirs); err != nil {
		t.Fatalf("seed local directories: %v", err)
	}
}

// mustExistingPath 创建本机存在的目录路径供校验通过。
func mustExistingPath(t *testing.T, base, name string) string {
	t.Helper()
	p := filepath.Join(base, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	return p
}

func manifestJSON(t *testing.T, m *model.RepoConfigManifest) string {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return string(data)
}

// --- Export 测试 ---

func TestExport_ContainsManifestVersionAndBusinessFieldsOnly(t *testing.T) {
	svc, base := createRepoConfigTestService(t)

	if _, err := svc.directorySvc.Create("工作目录", base, true); err != nil {
		t.Fatalf("seed directory: %v", err)
	}
	if err := svc.favoritesSvc.Add(base, "别名", "分组"); err != nil {
		t.Fatalf("seed favorite: %v", err)
	}

	text, err := svc.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var m model.RepoConfigManifest
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("exported text not valid json: %v", err)
	}
	if m.ManifestVersion != model.RepoConfigManifestVersion {
		t.Errorf("manifestVersion: got %d, want %d", m.ManifestVersion, model.RepoConfigManifestVersion)
	}
	if len(m.Directories) != 1 || m.Directories[0].Path != base {
		t.Fatalf("directories: got %+v, want 1 entry path=%s", m.Directories, base)
	}
	if !m.Directories[0].IsDefault {
		t.Error("directory IsDefault: got false, want true")
	}
	if len(m.Favorites) != 1 || m.Favorites[0].Group != "分组" || m.Favorites[0].Alias != "别名" {
		t.Fatalf("favorites: got %+v", m.Favorites)
	}

	// 运行时态与本机标识不导出：序列化文本中不应出现这些 json 字段
	for _, banned := range []string{`"isGitRepo"`, `"hasRemote"`, `"id"`, `"createTime"`} {
		if strings.Contains(text, banned) {
			t.Errorf("exported text should not contain %s", banned)
		}
	}
}

func TestExport_EmptyConfig(t *testing.T) {
	svc, _ := createRepoConfigTestService(t)
	text, err := svc.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	var m model.RepoConfigManifest
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("not valid json: %v", err)
	}
	if len(m.Directories) != 0 || len(m.Favorites) != 0 {
		t.Errorf("empty config: got %d dirs %d favs, want 0/0", len(m.Directories), len(m.Favorites))
	}
}

// --- PreviewImport 测试 ---

func TestPreviewImport_ClassifiesNewConflictInvalid(t *testing.T) {
	svc, base := createRepoConfigTestService(t)
	existing := mustExistingPath(t, base, "existing")

	// 本机已有 existing 目录
	seedDir := model.NewDirectory("本机", existing, false)
	writeLocalDirs(t, svc, []*model.Directory{seedDir})

	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion,
		Directories: []*model.RepoConfigDirectory{
			{Name: "新目录", Path: mustExistingPath(t, base, "new-dir")},
			{Name: "冲突目录", Path: existing},
			{Name: "路径不存在", Path: filepath.Join(base, "ghost")},
			{Path: ""},
		},
		Favorites: []*model.RepoConfigFavorite{
			{Path: mustExistingPath(t, base, "fav-new"), Group: "默认"},
			{Path: existing, Group: "默认"}, // 与本机工作目录无关，收藏夹本机为空 → 新增
			{Path: filepath.Join(base, "ghost-fav")},
		},
	})

	preview, err := svc.PreviewImport(text)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.NewDirectories) != 1 || preview.NewDirectories[0].Name != "新目录" {
		t.Errorf("newDirectories: got %+v", preview.NewDirectories)
	}
	if len(preview.ConflictDirectories) != 1 || preview.ConflictDirectories[0].Path != existing {
		t.Errorf("conflictDirectories: got %+v", preview.ConflictDirectories)
	}
	if len(preview.NewFavorites) != 2 {
		t.Errorf("newFavorites: got %+v, want 2", preview.NewFavorites)
	}
	if len(preview.Invalid) != 3 {
		t.Fatalf("invalid: got %+v, want 3", preview.Invalid)
	}
	// 非法项带原因
	for _, item := range preview.Invalid {
		if item.Reason == "" {
			t.Errorf("invalid item %s: empty reason", item.Name)
		}
		if item.Kind != "directory" && item.Kind != "favorite" {
			t.Errorf("invalid item kind: got %q", item.Kind)
		}
	}
}

func TestPreviewImport_InvalidJSON(t *testing.T) {
	svc, _ := createRepoConfigTestService(t)
	_, err := svc.PreviewImport("{not json")
	if err == nil {
		t.Fatal("invalid json: expected error, got nil")
	}
	appErr, ok := err.(*model.AppError)
	if !ok {
		t.Fatalf("error type: got %T, want *model.AppError", err)
	}
	if appErr.Code != model.ErrCodeRepoConfigInvalidJSON {
		t.Errorf("code: got %q, want %q", appErr.Code, model.ErrCodeRepoConfigInvalidJSON)
	}
}

func TestPreviewImport_MissingManifestVersion(t *testing.T) {
	svc, base := createRepoConfigTestService(t)
	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: 0,
		Directories:     []*model.RepoConfigDirectory{{Name: "x", Path: base}},
	})
	_, err := svc.PreviewImport(text)
	appErr, ok := err.(*model.AppError)
	if !ok {
		t.Fatalf("error type: got %T, want *model.AppError", err)
	}
	if appErr.Code != model.ErrCodeRepoConfigUnsupportedVersion {
		t.Errorf("code: got %q, want %q", appErr.Code, model.ErrCodeRepoConfigUnsupportedVersion)
	}
}

func TestPreviewImport_FutureVersionRejected(t *testing.T) {
	svc, _ := createRepoConfigTestService(t)
	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion + 1,
	})
	_, err := svc.PreviewImport(text)
	appErr, ok := err.(*model.AppError)
	if !ok {
		t.Fatalf("error type: got %T, want *model.AppError", err)
	}
	if appErr.Code != model.ErrCodeRepoConfigUnsupportedVersion {
		t.Errorf("code: got %q, want %q", appErr.Code, model.ErrCodeRepoConfigUnsupportedVersion)
	}
}

// --- ApplyImport 测试 ---

func TestApplyImport_NewDirectoriesAddedWithGitStateAndDefault(t *testing.T) {
	svc, base := createRepoConfigTestService(t)
	newPath := mustExistingPath(t, base, "imported")

	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion,
		Directories:     []*model.RepoConfigDirectory{{Name: "导入目录", Path: newPath, IsDefault: true}},
	})
	result, err := svc.ApplyImport(text, nil)
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if result.Added != 1 || result.Overwritten != 0 || result.Skipped != 0 || result.Failed != 0 {
		t.Errorf("result: got %+v", result)
	}
	dirs, err := svc.directorySvc.Load()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("loaded dirs: %v %d", err, len(dirs))
	}
	d := dirs[0]
	if d.ID == "" || d.Name != "导入目录" || d.Path != newPath {
		t.Errorf("dir fields: %+v", d)
	}
	if !d.IsDefault {
		t.Error("IsDefault: got false, want true")
	}
	// 非 git 仓库路径，运行时态应为 false（IsGitRepository 对普通目录返回 false）
	if d.IsGitRepo || d.HasRemote {
		t.Errorf("git state on plain dir: isGitRepo=%v hasRemote=%v, want false/false", d.IsGitRepo, d.HasRemote)
	}
}

func TestApplyImport_ConflictDecisions(t *testing.T) {
	svc, base := createRepoConfigTestService(t)
	conflictPath := mustExistingPath(t, base, "conflict")
	local := model.NewDirectory("本机名", conflictPath, false)
	localID := local.ID
	writeLocalDirs(t, svc, []*model.Directory{local})

	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion,
		Directories:     []*model.RepoConfigDirectory{{Name: "导入名", Path: conflictPath}},
	})

	// 跳过：本机条目原样保留
	result, err := svc.ApplyImport(text, &model.RepoConfigImportDecisions{
		Directories: map[string]string{conflictPath: model.ImportDecisionSkip},
	})
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	dirs, _ := svc.directorySvc.Load()
	if result.Skipped != 1 || len(dirs) != 1 || dirs[0].Name != "本机名" || dirs[0].ID != localID {
		t.Fatalf("skip result: %+v dirs=%+v", result, dirs)
	}

	// 覆盖：保留本机 ID，名称取导入值
	result, err = svc.ApplyImport(text, &model.RepoConfigImportDecisions{
		Directories: map[string]string{conflictPath: model.ImportDecisionOverwrite},
	})
	if err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	dirs, _ = svc.directorySvc.Load()
	if result.Overwritten != 1 || len(dirs) != 1 || dirs[0].ID != localID || dirs[0].Name != "导入名" {
		t.Fatalf("overwrite result: %+v dirs=%+v", result, dirs)
	}

	// 另存为新项：本机保留 + 导入项新 ID 并存，名称加「（导入）」后缀
	result, err = svc.ApplyImport(text, &model.RepoConfigImportDecisions{
		Directories: map[string]string{conflictPath: model.ImportDecisionSaveAsNew},
	})
	if err != nil {
		t.Fatalf("saveAsNew: %v", err)
	}
	dirs, _ = svc.directorySvc.Load()
	if result.Added != 1 || len(dirs) != 2 {
		t.Fatalf("saveAsNew result: %+v dirs=%+v", result, dirs)
	}
	var imported *model.Directory
	for _, d := range dirs {
		if d.ID != localID {
			imported = d
		}
	}
	if imported == nil {
		t.Fatal("saveAsNew: imported entry not found")
	}
	if imported.Name != "导入名（导入）" || imported.IsDefault {
		t.Errorf("saveAsNew entry: %+v", imported)
	}
}

func TestApplyImport_SaveAsNewDoesNotStealDefault(t *testing.T) {
	svc, base := createRepoConfigTestService(t)
	conflictPath := mustExistingPath(t, base, "conflict")
	local := model.NewDirectory("本机", conflictPath, true) // 本机为默认
	writeLocalDirs(t, svc, []*model.Directory{local})

	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion,
		Directories:     []*model.RepoConfigDirectory{{Name: "导入", Path: conflictPath, IsDefault: true}},
	})
	_, err := svc.ApplyImport(text, &model.RepoConfigImportDecisions{
		Directories: map[string]string{conflictPath: model.ImportDecisionSaveAsNew},
	})
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	dirs, _ := svc.directorySvc.Load()
	if len(dirs) != 2 {
		t.Fatalf("dirs: %d", len(dirs))
	}
	defaultCount := 0
	for _, d := range dirs {
		if d.IsDefault {
			defaultCount++
			if d.ID != local.ID {
				t.Errorf("default stolen by imported entry: %+v", d)
			}
		}
	}
	if defaultCount != 1 {
		t.Errorf("default count: got %d, want 1", defaultCount)
	}
}

func TestApplyImport_ImportedDefaultClearsLocalDefaults(t *testing.T) {
	svc, base := createRepoConfigTestService(t)
	local := model.NewDirectory("本机", base, true)
	writeLocalDirs(t, svc, []*model.Directory{local})

	newPath := mustExistingPath(t, base, "imported-default")
	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion,
		Directories:     []*model.RepoConfigDirectory{{Name: "导入默认", Path: newPath, IsDefault: true}},
	})
	if _, err := svc.ApplyImport(text, nil); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	dirs, _ := svc.directorySvc.Load()
	defaultCount := 0
	for _, d := range dirs {
		if d.IsDefault {
			defaultCount++
			if d.Path != newPath {
				t.Errorf("default held by non-imported entry: %+v", d)
			}
		}
	}
	if defaultCount != 1 {
		t.Errorf("default count: got %d, want 1", defaultCount)
	}
}

func TestApplyImport_FavoritesMergeAndLimit(t *testing.T) {
	svc, base := createRepoConfigTestService(t)
	conflictPath := mustExistingPath(t, base, "fav-conflict")

	// 本机收藏：1 条冲突 + 99 条填充，合计 100 条（达上限）
	if err := svc.favoritesSvc.Add(conflictPath, "本机别名", "本机分组"); err != nil {
		t.Fatalf("seed favorite: %v", err)
	}
	for i := 0; i < 99; i++ {
		p := mustExistingPath(t, base, "filler-"+strings.Repeat("x", i+1))
		if err := svc.favoritesSvc.Add(p, "", "填充"); err != nil {
			t.Fatalf("seed filler %d: %v", i, err)
		}
	}

	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion,
		Favorites: []*model.RepoConfigFavorite{
			{Path: conflictPath, Alias: "导入别名", Group: "导入分组"},
			{Path: mustExistingPath(t, base, "fav-new"), Group: ""},
		},
	})

	// 冲突另存为新项：超上限 → 失败计数；新增项同样放不下 → 失败
	result, err := svc.ApplyImport(text, &model.RepoConfigImportDecisions{
		Favorites: map[string]string{conflictPath: model.ImportDecisionSaveAsNew},
	})
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if result.Failed != 2 || len(result.FailedReasons) != 2 {
		t.Fatalf("limit result: %+v", result)
	}
	favs, _ := svc.favoritesSvc.Load()
	if len(favs) != 100 {
		t.Errorf("favorites count: got %d, want 100", len(favs))
	}

	// 冲突覆盖：不占新名额，成功；新增项仍超限失败
	result, err = svc.ApplyImport(text, &model.RepoConfigImportDecisions{
		Favorites: map[string]string{conflictPath: model.ImportDecisionOverwrite},
	})
	if err != nil {
		t.Fatalf("ApplyImport overwrite: %v", err)
	}
	if result.Overwritten != 1 || result.Failed != 1 {
		t.Errorf("overwrite result: %+v", result)
	}
	favs, _ = svc.favoritesSvc.Load()
	for _, f := range favs {
		if f.Path == conflictPath {
			if f.Alias != "导入别名" || f.Group != "导入分组" {
				t.Errorf("overwrite favorite fields: %+v", f)
			}
		}
	}
}

func TestApplyImport_FavoriteNewFillsDefaultGroup(t *testing.T) {
	svc, base := createRepoConfigTestService(t)
	newPath := mustExistingPath(t, base, "fav-nogroup")

	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion,
		Favorites:       []*model.RepoConfigFavorite{{Path: newPath}},
	})
	result, err := svc.ApplyImport(text, nil)
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if result.Added != 1 {
		t.Fatalf("result: %+v", result)
	}
	favs, _ := svc.favoritesSvc.Load()
	if len(favs) != 1 || favs[0].Group != "默认" {
		t.Errorf("favorites: %+v, want group 默认", favs)
	}
}

func TestApplyImport_InvalidItemsCountedAsFailed(t *testing.T) {
	svc, base := createRepoConfigTestService(t)
	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion,
		Directories:     []*model.RepoConfigDirectory{{Name: "幽灵", Path: filepath.Join(base, "ghost")}},
		Favorites:       []*model.RepoConfigFavorite{{Path: ""}},
	})
	result, err := svc.ApplyImport(text, nil)
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if result.Failed != 2 || len(result.FailedReasons) != 2 {
		t.Fatalf("result: %+v", result)
	}
}

func TestApplyImport_SaveAsNewNormalizesStorePath(t *testing.T) {
	svc, base := createRepoConfigTestService(t)
	conflictPath := mustExistingPath(t, base, "normalize-me")
	trailingSepPath := conflictPath + string(os.PathSeparator) // 尾部分隔符写法，normalizePath（Abs+Clean）会消除差异

	local := model.NewDirectory("本机", conflictPath, false)
	writeLocalDirs(t, svc, []*model.Directory{local})

	text := manifestJSON(t, &model.RepoConfigManifest{
		ManifestVersion: model.RepoConfigManifestVersion,
		Directories:     []*model.RepoConfigDirectory{{Name: "导入", Path: trailingSepPath}},
	})
	// 决策表 key 与前端契约一致：取预览返回的原始 path（带尾分隔符）
	result, err := svc.ApplyImport(text, &model.RepoConfigImportDecisions{
		Directories: map[string]string{trailingSepPath: model.ImportDecisionSaveAsNew},
	})
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if result.Added != 1 {
		t.Fatalf("result: %+v", result)
	}
	dirs, _ := svc.directorySvc.Load()
	if len(dirs) != 2 {
		t.Fatalf("dirs: %d", len(dirs))
	}
	var imported *model.Directory
	for _, d := range dirs {
		if d.ID != local.ID {
			imported = d
		}
	}
	if imported == nil {
		t.Fatal("imported entry not found")
	}
	// 入库路径须与新增分支一致规范化（无尾分隔符），不得原样落盘相对/非 Clean 写法
	if imported.Path != conflictPath {
		t.Errorf("imported path: got %q, want %q", imported.Path, conflictPath)
	}
}

func TestApplyImport_InvalidJSONRejectedBeforeWrite(t *testing.T) {
	svc, _ := createRepoConfigTestService(t)
	_, err := svc.ApplyImport("}", nil)
	if err == nil {
		t.Fatal("invalid json: expected error, got nil")
	}
	appErr, ok := err.(*model.AppError)
	if !ok || appErr.Code != model.ErrCodeRepoConfigInvalidJSON {
		t.Fatalf("error: %v", err)
	}
}
