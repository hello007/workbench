package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
)

func newTestPromptService(t *testing.T) *PromptService {
	t.Helper()
	return NewPromptService(filepath.Join(t.TempDir(), "prompts.json"))
}

func TestPromptAddAndList(t *testing.T) {
	svc := newTestPromptService(t)
	svc.now = fixedClock(time.UnixMilli(1000))

	created, err := svc.Add("  代码评审\n话术  ", "请评审 {{文件路径}} 的改动\r\n关注 {{关注点}}")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("Add() 未生成 ID")
	}
	if created.Title != "代码评审\n话术" {
		t.Errorf("标题未归一，got %q", created.Title)
	}
	if strings.Contains(created.Content, "\r") {
		t.Errorf("正文换行未归一，got %q", created.Content)
	}
	if created.CreatedAt != 1000 || created.UpdatedAt != 1000 {
		t.Errorf("时间字段 = (%d,%d), want (1000,1000)", created.CreatedAt, created.UpdatedAt)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("List() = %+v, want 单条且 ID 一致", list)
	}
}

func TestPromptListSortsByCreatedAtDesc(t *testing.T) {
	svc := newTestPromptService(t)

	clock := time.UnixMilli(1000)
	svc.now = func() time.Time { return clock }
	first, _ := svc.Add("第一条", "正文一")
	clock = time.UnixMilli(2000)
	second, _ := svc.Add("第二条", "正文二")

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("List() 顺序 = %+v, want 创建时间降序", list)
	}
}

func TestPromptRejectsEmptyFields(t *testing.T) {
	svc := newTestPromptService(t)

	cases := []struct {
		name    string
		title   string
		content string
		code    string
	}{
		{"空标题", "", "正文", model.ErrCodePromptTitleRequired},
		{"空白标题", " \n\t ", "正文", model.ErrCodePromptTitleRequired},
		{"空正文", "标题", "", model.ErrCodePromptContentRequired},
		{"空白正文", "标题", " \n", model.ErrCodePromptContentRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Add(tc.title, tc.content)
			var appErr *model.AppError
			if !errors.As(err, &appErr) || appErr.Code != tc.code {
				t.Fatalf("Add() error = %v, want %s", err, tc.code)
			}
		})
	}

	if list, _ := svc.List(); len(list) != 0 {
		t.Errorf("非法输入不应落盘，got %d 条", len(list))
	}
}

func TestPromptUpdate(t *testing.T) {
	svc := newTestPromptService(t)
	svc.now = fixedClock(time.UnixMilli(1000))
	created, _ := svc.Add("旧标题", "旧正文")

	svc.now = fixedClock(time.UnixMilli(5000))
	if err := svc.Update(created.ID, " 新标题 ", "新正文"); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	list, _ := svc.List()
	if got := list[0]; got.Title != "新标题" || got.Content != "新正文" {
		t.Errorf("更新后字段 = %+v", got)
	} else if got.CreatedAt != 1000 || got.UpdatedAt != 5000 {
		t.Errorf("时间字段 = (%d,%d), want (1000,5000)", got.CreatedAt, got.UpdatedAt)
	}

	if err := svc.Update(created.ID, "", "正文"); !isPromptCode(err, model.ErrCodePromptTitleRequired) {
		t.Errorf("更新空标题 error = %v, want %s", err, model.ErrCodePromptTitleRequired)
	}
	if err := svc.Update(created.ID, "标题", "  "); !isPromptCode(err, model.ErrCodePromptContentRequired) {
		t.Errorf("更新空正文 error = %v, want %s", err, model.ErrCodePromptContentRequired)
	}
	if err := svc.Update("missing", "标题", "正文"); !isPromptCode(err, model.ErrCodePromptNotFound) {
		t.Errorf("更新不存在条目 error = %v, want %s", err, model.ErrCodePromptNotFound)
	}
}

func TestPromptDelete(t *testing.T) {
	svc := newTestPromptService(t)
	created, _ := svc.Add("标题", "正文")

	if err := svc.Delete(created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if list, _ := svc.List(); len(list) != 0 {
		t.Errorf("删除后仍有 %d 条", len(list))
	}
	if err := svc.Delete(created.ID); !isPromptCode(err, model.ErrCodePromptNotFound) {
		t.Errorf("重复删除 error = %v, want %s", err, model.ErrCodePromptNotFound)
	}
}

func TestPromptPersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")

	svc := NewPromptService(path)
	created, err := svc.Add("标题", "正文 {{变量}}")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	reloaded := NewPromptService(path)
	list, err := reloaded.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID || list[0].Content != "正文 {{变量}}" {
		t.Fatalf("重载结果 = %+v", list)
	}
}

func TestPromptLoadSkipsDirtyEntriesAndBacksUpCorruptFile(t *testing.T) {
	t.Run("脏条目剔除并回填 UpdatedAt", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "prompts.json")
		raw := `{"prompts":[
			{"id":"a","title":"有效","content":"正文","createdAt":10,"updatedAt":0},
			{"id":"","title":"无ID","content":"正文"},
			{"id":"b","title":"  ","content":"正文"},
			{"id":"c","title":"标题","content":" "}
		]}`
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}

		list, err := NewPromptService(path).List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(list) != 1 || list[0].ID != "a" {
			t.Fatalf("脏条目未剔除，got %+v", list)
		}
		if list[0].UpdatedAt != 10 {
			t.Errorf("UpdatedAt 未回填，got %d", list[0].UpdatedAt)
		}
	})

	t.Run("损坏文件降级空列表并留底", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "prompts.json")
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}

		list, err := NewPromptService(path).List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("损坏文件应降级空列表，got %+v", list)
		}
		matches, _ := filepath.Glob(path + ".bak.*")
		if len(matches) != 1 {
			t.Errorf("损坏留底备份 = %v, want 恰好 1 份", matches)
		}
	})
}

func TestPromptContentPreservesIndentation(t *testing.T) {
	svc := newTestPromptService(t)

	created, err := svc.Add("代码模板", "\n  首行缩进\n第二行\n\n")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	// 只裁首尾空行与行尾空白；首行缩进属于正文语义（代码块/嵌套列表），必须保留
	if want := "  首行缩进\n第二行"; created.Content != want {
		t.Errorf("Content = %q, want %q", created.Content, want)
	}
}

func TestPromptSaveFailureRollsBackAllMutations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	svc := NewPromptService(path)
	clock := time.UnixMilli(1000)
	svc.now = func() time.Time { return clock }

	first, err := svc.Add("第一条", "正文一")
	if err != nil {
		t.Fatalf("前置 Add() error = %v", err)
	}
	clock = time.UnixMilli(2000)
	if _, err := svc.Add("第二条", "正文二"); err != nil {
		t.Fatalf("前置 Add() error = %v", err)
	}

	// 用目录占位目标文件：util.SaveJSON 的 rename 恒失败，可稳定制造落盘错误
	// （跨平台：rename file → existing dir 在 Windows/Unix 均报错）
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	assertSaveFailed := func(t *testing.T, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "保存提示词失败") {
			t.Fatalf("落盘失败应返回保存错误, got %v", err)
		}
	}

	// Add：内存态须回滚，不能留下未持久化的条目
	assertSaveFailed(t, func() error { _, err := svc.Add("第三条", "正文三"); return err }())
	if list, _ := svc.List(); len(list) != 2 {
		t.Fatalf("Add 失败后条目数 = %d, want 2（内存态未回滚）", len(list))
	}

	// Update：标题/正文/UpdatedAt 三项均须回到编辑前的值
	assertSaveFailed(t, svc.Update(first.ID, "改标题", "改正文"))
	for _, p := range mustListPrompt(t, svc) {
		if p.ID != first.ID {
			continue
		}
		if p.Title != "第一条" || p.Content != "正文一" || p.UpdatedAt != 1000 {
			t.Errorf("Update 失败后字段 = %+v, want 原值且 UpdatedAt=1000", p)
		}
	}

	// Delete：条目须插回原位，内存态与落盘态不分叉
	assertSaveFailed(t, svc.Delete(first.ID))
	if list := mustListPrompt(t, svc); len(list) != 2 {
		t.Fatalf("Delete 失败后条目数 = %d, want 2（条目丢失）", len(list))
	}
}

func mustListPrompt(t *testing.T, svc *PromptService) []model.Prompt {
	t.Helper()
	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	return list
}

func TestPromptTruncatesOverlongFields(t *testing.T) {
	svc := newTestPromptService(t)
	title := strings.Repeat("标", PromptTitleMaxRunes+10)
	content := strings.Repeat("文", PromptContentMaxRunes+10)

	created, err := svc.Add(title, content)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got := len([]rune(created.Title)); got != PromptTitleMaxRunes {
		t.Errorf("标题长度 = %d, want %d", got, PromptTitleMaxRunes)
	}
	if got := len([]rune(created.Content)); got != PromptContentMaxRunes {
		t.Errorf("正文长度 = %d, want %d", got, PromptContentMaxRunes)
	}
}

func isPromptCode(err error, code string) bool {
	var appErr *model.AppError
	return errors.As(err, &appErr) && appErr.Code == code
}
