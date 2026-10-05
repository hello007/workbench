package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"workbench/model"
)

// ===== 搜索相关 =====

// SearchFiles 搜索文件
func (a *App) SearchFiles(rootDir, query string, maxResults int) []*model.SearchResult {
	results, err := a.searchSvc.Search(rootDir, query, maxResults)
	if err != nil {
		slog.Error("search files failed", "rootDir", rootDir, "query", query, "err", err)
		return []*model.SearchResult{}
	}
	return results
}

// ContentSearch 内容搜索
// keyword: 搜索关键词
// fileExt: 文件类型过滤（如 ".java"，为空则不过滤）
// subDir: 子目录路径（相对于工作目录，为空则搜索整个目录）
// searchAll: 是否搜索所有工作目录（true 搜索全部，false 仅当前目录）
func (a *App) ContentSearch(keyword, fileExt, subDir string, searchAll bool) ([]*model.ContentSearchGroup, error) {
	if keyword == "" {
		return nil, nil
	}

	dirs, repoNames, err := a.resolveSearchDirs(searchAll)
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		return nil, nil
	}

	// 全局搜索超时 60s，单目录 10s
	timeout := 10
	if searchAll {
		timeout = 60
	}
	return a.searchContentInDirs(dirs, repoNames, keyword, fileExt, subDir, timeout)
}

// ContentSearchDirs 在指定工作目录列表中搜索内容（命令面板超时目录单目录重试入口）
// dirs 与 repoNames 按下标一一对应，均来自上次搜索结果组，目录合法性由上游保证；
// 重试目标多为 10s 内跑不完的大目录，固定 60s 给足第二次机会（与全局搜索同语义）
func (a *App) ContentSearchDirs(keyword, fileExt, subDir string, dirs, repoNames []string) ([]*model.ContentSearchGroup, error) {
	if keyword == "" || len(dirs) == 0 {
		return nil, nil
	}
	if len(repoNames) != len(dirs) {
		return nil, fmt.Errorf("dirs 与 repoNames 数量不一致: %d vs %d", len(dirs), len(repoNames))
	}
	return a.searchContentInDirs(dirs, repoNames, keyword, fileExt, subDir, 60)
}

// resolveSearchDirs 确定搜索目录列表
func (a *App) resolveSearchDirs(searchAll bool) (dirs []string, repoNames []string, err error) {
	if searchAll {
		directories, loadErr := a.directorySvc.Load()
		if loadErr != nil {
			return nil, nil, loadErr
		}
		for _, d := range directories {
			dirs = append(dirs, d.Path)
			repoNames = append(repoNames, d.Name)
		}
		return dirs, repoNames, nil
	}

	// 仅当前选中的工作目录
	directories, _ := a.directorySvc.Load()
	for _, d := range directories {
		if d.IsDefault {
			dirs = append(dirs, d.Path)
			repoNames = append(repoNames, d.Name)
			break
		}
	}
	// 如果没有默认目录，用第一个
	if len(dirs) == 0 && len(directories) > 0 {
		dirs = append(dirs, directories[0].Path)
		repoNames = append(repoNames, directories[0].Name)
	}
	return dirs, repoNames, nil
}

// searchContentInDirs 按目录列表执行内容搜索（ContentSearch 与 ContentSearchDirs 公共出口）
func (a *App) searchContentInDirs(dirs, repoNames []string, keyword, fileExt, subDir string, timeoutSec int) ([]*model.ContentSearchGroup, error) {
	// 加载设置获取排除配置
	settings, _ := a.settingsSvc.Load()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	return a.contentSearchSvc.ContentSearch(
		ctx, dirs, repoNames,
		keyword, fileExt, subDir,
		settings.SearchExcludeDirs, settings.SearchExcludeFiles,
		20,
	)
}
