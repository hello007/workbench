package main

import (
	"context"
	"time"

	"workbench/model"
)

// ===== 搜索相关 =====

// SearchFiles 搜索文件
func (a *App) SearchFiles(rootDir, query string, maxResults int) []*model.SearchResult {
	results, err := a.searchSvc.Search(rootDir, query, maxResults)
	if err != nil {
		println("SearchFiles error:", err.Error())
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

	// 加载设置获取排除配置
	settings, _ := a.settingsSvc.Load()

	// 确定搜索目录列表
	var dirs []string
	var repoNames []string

	if searchAll {
		directories, err := a.directorySvc.Load()
		if err != nil {
			return nil, err
		}
		for _, d := range directories {
			dirs = append(dirs, d.Path)
			repoNames = append(repoNames, d.Name)
		}
	} else {
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
	}

	if len(dirs) == 0 {
		return nil, nil
	}

	// 全局搜索超时 60s，单目录 10s
	timeout := 10
	if searchAll {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	return a.contentSearchSvc.ContentSearch(
		ctx, dirs, repoNames,
		keyword, fileExt, subDir,
		settings.SearchExcludeDirs, settings.SearchExcludeFiles,
		20,
	)
}
