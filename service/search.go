package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"workbench/model"
)

type SearchService struct{}

func NewSearchService() *SearchService {
	return &SearchService{}
}

func (s *SearchService) Search(rootDir, query string, maxResults int) ([]*model.SearchResult, error) {
	var results []*model.SearchResult
	queryLower := strings.ToLower(query)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Name() == ".git" && info.IsDir() {
			return filepath.SkipDir
		}
		if info.Name() == "node_modules" && info.IsDir() {
			return filepath.SkipDir
		}
		if path == rootDir {
			return nil
		}

		name := info.Name()
		if queryLower == "" || fuzzyMatch(strings.ToLower(name), queryLower) {
			fileType := "file"
			if info.IsDir() {
				fileType = "directory"
			}
			relPath, _ := filepath.Rel(rootDir, path)
			results = append(results, &model.SearchResult{
				Name: name,
				Path: relPath,
				Type: fileType,
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		iScore := matchScore(strings.ToLower(results[i].Name), queryLower)
		jScore := matchScore(strings.ToLower(results[j].Name), queryLower)
		return iScore > jScore
	})

	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}

func fuzzyMatch(text, pattern string) bool {
	if pattern == "" {
		return true
	}
	pi := 0
	for i := 0; i < len(text) && pi < len(pattern); i++ {
		if text[i] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

func matchScore(text, pattern string) int {
	if pattern == "" {
		return 0
	}
	if text == pattern {
		return 100
	}
	if strings.HasPrefix(text, pattern) {
		return 80
	}
	if strings.Contains(text, pattern) {
		return 60
	}
	return 40
}
