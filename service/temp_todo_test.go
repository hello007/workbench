package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
	"workbench/util"
)

func newTestTempTodoService(t *testing.T) *TempTodoService {
	t.Helper()
	return NewTempTodoService(filepath.Join(t.TempDir(), "temp_todos.json"))
}

// fixedClock 返回固定时钟，避免保留期测试依赖真实时间。
func fixedClock(ts time.Time) func() time.Time {
	return func() time.Time { return ts }
}

func TestTempTodoAddAndList(t *testing.T) {
	svc := newTestTempTodoService(t)

	created, err := svc.Add("  第一条\n待办  ", []string{" 重构 ", "", "重构", "AI"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("Add() 未生成 ID")
	}
	if created.Text != "第一条\n待办" {
		t.Errorf("正文未归一，got %q", created.Text)
	}
	if got, want := strings.Join(created.Tags, ","), "重构,AI"; got != want {
		t.Errorf("标签归一结果 = %q, want %q", got, want)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("List() = %+v, want 单条且 ID 一致", list)
	}
	if list[0].CompletedAt != nil {
		t.Errorf("新建条目不应带完成时间")
	}
}

func TestTempTodoRejectsEmptyText(t *testing.T) {
	svc := newTestTempTodoService(t)

	for _, text := range []string{"", "   ", "\n\t "} {
		_, err := svc.Add(text, nil)
		var appErr *model.AppError
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeTempTodoTextRequired {
			t.Errorf("Add(%q) error = %v, want %s", text, err, model.ErrCodeTempTodoTextRequired)
		}
	}

	if list, _ := svc.List(); len(list) != 0 {
		t.Errorf("空正文不应落盘，got %d 条", len(list))
	}
}

func TestTempTodoAddFromClipboard(t *testing.T) {
	t.Run("成功落条", func(t *testing.T) {
		svc := newTestTempTodoService(t)
		svc.readClipboard = func() (string, error) { return "剪贴板里的提示词", nil }

		todo, err := svc.AddFromClipboard()
		if err != nil {
			t.Fatalf("AddFromClipboard() error = %v", err)
		}
		if todo.Text != "剪贴板里的提示词" {
			t.Errorf("Text = %q", todo.Text)
		}
		if todo.Tags == nil || len(todo.Tags) != 0 {
			t.Errorf("剪贴板记录不应比带标签，got %v", todo.Tags)
		}
	})

	t.Run("空剪贴板", func(t *testing.T) {
		svc := newTestTempTodoService(t)
		svc.readClipboard = func() (string, error) { return "", util.ErrClipboardEmpty }

		_, err := svc.AddFromClipboard()
		var appErr *model.AppError
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeTempTodoClipboardEmpty {
			t.Fatalf("error = %v, want %s", err, model.ErrCodeTempTodoClipboardEmpty)
		}
	})

	t.Run("剪贴板无文本", func(t *testing.T) {
		svc := newTestTempTodoService(t)
		svc.readClipboard = func() (string, error) { return "", util.ErrClipboardNoText }

		_, err := svc.AddFromClipboard()
		var appErr *model.AppError
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeTempTodoClipboardNoText {
			t.Fatalf("error = %v, want %s", err, model.ErrCodeTempTodoClipboardNoText)
		}
	})

	t.Run("读取失败", func(t *testing.T) {
		svc := newTestTempTodoService(t)
		svc.readClipboard = func() (string, error) { return "", errors.New("OpenClipboard failed") }

		_, err := svc.AddFromClipboard()
		var appErr *model.AppError
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeTempTodoClipboardFailed {
			t.Fatalf("error = %v, want %s", err, model.ErrCodeTempTodoClipboardFailed)
		}
		if !strings.Contains(err.Error(), "OpenClipboard failed") {
			t.Errorf("根因未透出: %v", err)
		}
	})
}

func TestTempTodoCompleteFlow(t *testing.T) {
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	svc := newTestTempTodoService(t)
	svc.now = fixedClock(base)

	todo, err := svc.Add("要完成的事", nil)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if err := svc.SetCompleted(todo.ID, true); err != nil {
		t.Fatalf("SetCompleted(true) error = %v", err)
	}
	list, _ := svc.List()
	if list[0].CompletedAt == nil || *list[0].CompletedAt != base.UnixMilli() {
		t.Fatalf("完成时间 = %v, want %d", list[0].CompletedAt, base.UnixMilli())
	}

	// 重复完成幂等
	if err := svc.SetCompleted(todo.ID, true); err != nil {
		t.Fatalf("重复完成应幂等，error = %v", err)
	}

	if err := svc.SetCompleted(todo.ID, false); err != nil {
		t.Fatalf("SetCompleted(false) error = %v", err)
	}
	list, _ = svc.List()
	if list[0].CompletedAt != nil {
		t.Fatalf("取消完成后应清空 CompletedAt，got %v", *list[0].CompletedAt)
	}
}

func TestTempTodoUpdateAndDelete(t *testing.T) {
	svc := newTestTempTodoService(t)
	todo, err := svc.Add("原正文", []string{"旧"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if err := svc.Update(todo.ID, "新正文", []string{"新", "追加"}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	list, _ := svc.List()
	if list[0].Text != "新正文" || strings.Join(list[0].Tags, ",") != "新,追加" {
		t.Fatalf("更新结果 = %+v", list[0])
	}
	if list[0].CreatedAt != todo.CreatedAt {
		t.Errorf("编辑不应改变创建时间")
	}

	if err := svc.Update(todo.ID, "  ", nil); err == nil {
		t.Error("空白正文应被拒绝")
	}

	if err := svc.Delete(todo.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if list, _ := svc.List(); len(list) != 0 {
		t.Errorf("删除后列表应为空，got %d 条", len(list))
	}

	// 已删除条目的操作返回 NotFound 而非静默成功
	for name, err := range map[string]error{
		"update":   svc.Update(todo.ID, "x", nil),
		"complete": svc.SetCompleted(todo.ID, true),
		"delete":   svc.Delete(todo.ID),
	} {
		var appErr *model.AppError
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeTempTodoNotFound {
			t.Errorf("%s 已删除条目 error = %v, want %s", name, err, model.ErrCodeTempTodoNotFound)
		}
	}
}

// 落盘失败时删除必须回滚：条目插回原位，列表内容与顺序不变。
// 失败构造：把落盘目标换成已存在的目录——SaveJSON 的 rename 无法用文件覆盖目录，
// 稳定失败；service 已 loaded=true，loadLocked 短路，不影响前置数据。
func TestTempTodoDeleteSaveFailureRollsBack(t *testing.T) {
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	svc := newTestTempTodoService(t)
	svc.now = fixedClock(base)

	first, _ := svc.Add("第一条", nil)
	svc.now = fixedClock(base.Add(time.Minute))
	second, _ := svc.Add("第二条", nil)
	svc.now = fixedClock(base.Add(2 * time.Minute))
	third, _ := svc.Add("第三条", nil)

	svc.configPath = t.TempDir()

	err := svc.Delete(second.ID)
	if err == nil {
		t.Fatal("落盘失败时 Delete 应返回错误")
	}
	if !strings.Contains(err.Error(), "保存临时待办失败") {
		t.Errorf("错误信息应说明保存失败，got %v", err)
	}

	// 内存态回滚：中间条目仍在原位（内部切片顺序 = 创建序），不因删除塌陷
	if len(svc.todos) != 3 {
		t.Fatalf("回滚后内部条目数 = %d, want 3", len(svc.todos))
	}
	wantOrder := []string{first.ID, second.ID, third.ID}
	for i, want := range wantOrder {
		if svc.todos[i].ID != want {
			t.Errorf("回滚后第 %d 位 = %s, want %s", i, svc.todos[i].ID, want)
		}
	}

	list, lerr := svc.List()
	if lerr != nil {
		t.Fatalf("List() error = %v", lerr)
	}
	// List 按创建时间降序：最新在前，删除未生效
	if len(list) != 3 {
		t.Fatalf("回滚后 List 条目数 = %d, want 3", len(list))
	}
	gotOrder := []string{list[0].ID, list[1].ID, list[2].ID}
	wantListOrder := []string{third.ID, second.ID, first.ID}
	for i := range wantListOrder {
		if gotOrder[i] != wantListOrder[i] {
			t.Errorf("回滚后 List 第 %d 位 = %s, want %s", i, gotOrder[i], wantListOrder[i])
		}
	}
}

func TestTempTodoPruneByRetention(t *testing.T) {
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	svc := newTestTempTodoService(t)
	svc.now = fixedClock(base)

	old, _ := svc.Add("31 天前完成", nil)
	recent, _ := svc.Add("29 天前完成", nil)
	pending, _ := svc.Add("仍未完成", nil)

	// 借助注入时钟分别写入不同的完成时间
	svc.now = fixedClock(base.Add(-31 * 24 * time.Hour))
	if err := svc.SetCompleted(old.ID, true); err != nil {
		t.Fatalf("SetCompleted(old) error = %v", err)
	}
	svc.now = fixedClock(base.Add(-29 * 24 * time.Hour))
	if err := svc.SetCompleted(recent.ID, true); err != nil {
		t.Fatalf("SetCompleted(recent) error = %v", err)
	}
	svc.now = fixedClock(base)

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	ids := map[string]bool{}
	for _, todo := range list {
		ids[todo.ID] = true
	}
	if ids[old.ID] {
		t.Error("超 30 天的已完成条目应被清理")
	}
	if !ids[recent.ID] || !ids[pending.ID] {
		t.Errorf("未超期条目不应被清理，got %v", ids)
	}

	// 清理结果须已落盘：新实例读取同一文件不复活旧条目
	reloaded := NewTempTodoService(svc.configPath)
	reloaded.now = fixedClock(base)
	again, _ := reloaded.List()
	for _, todo := range again {
		if todo.ID == old.ID {
			t.Error("清理结果未落盘，重新加载后旧条目复活")
		}
	}
}

func TestTempTodoPruneByCount(t *testing.T) {
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	svc := newTestTempTodoService(t)
	svc.now = fixedClock(base)

	// 直接构造超量数据：逐条 Add 后再标记完成，完成时间依次递增
	ids := make([]string, 0, TempTodoCompletedMax+5)
	for i := 0; i < TempTodoCompletedMax+5; i++ {
		todo, err := svc.Add(fmt.Sprintf("条目 %d", i), nil)
		if err != nil {
			t.Fatalf("Add(%d) error = %v", i, err)
		}
		ids = append(ids, todo.ID)
		svc.now = fixedClock(base.Add(time.Duration(i) * time.Minute))
		if err := svc.SetCompleted(todo.ID, true); err != nil {
			t.Fatalf("SetCompleted(%d) error = %v", i, err)
		}
	}
	svc.now = fixedClock(base.Add(time.Hour))

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	completed := 0
	for _, todo := range list {
		if todo.CompletedAt != nil {
			completed++
		}
	}
	if completed != TempTodoCompletedMax {
		t.Fatalf("已完成条目数 = %d, want %d", completed, TempTodoCompletedMax)
	}
	// 最旧的 5 条应被裁剪
	for _, id := range ids[:5] {
		for _, todo := range list {
			if todo.ID == id {
				t.Errorf("最旧已完成条目 %s 应被裁剪", id)
			}
		}
	}
}

func TestTempTodoSortOrder(t *testing.T) {
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	svc := newTestTempTodoService(t)
	svc.now = fixedClock(base)

	first, _ := svc.Add("先创建", nil)
	svc.now = fixedClock(base.Add(time.Minute))
	second, _ := svc.Add("后创建", nil)
	svc.now = fixedClock(base.Add(2 * time.Minute))
	done, _ := svc.Add("已完成", nil)
	if err := svc.SetCompleted(done.ID, true); err != nil {
		t.Fatalf("SetCompleted() error = %v", err)
	}

	list, _ := svc.List()
	if len(list) != 3 {
		t.Fatalf("len = %d", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Errorf("待办区应按创建时间降序，got %s, %s", list[0].ID, list[1].ID)
	}
	if list[2].ID != done.ID {
		t.Errorf("已完成条目应排在最后，got %v", list)
	}
}

func TestTempTodoCorruptFileFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "temp_todos.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("写坏文件失败: %v", err)
	}

	svc := NewTempTodoService(path)
	list, err := svc.List()
	if err != nil {
		t.Fatalf("损坏文件应降级空列表，error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("降级后应为空列表，got %d 条", len(list))
	}

	matches, _ := filepath.Glob(path + ".bak.*")
	if len(matches) != 1 {
		t.Errorf("损坏文件应留底恰好 1 份，got %v", matches)
	}
}

func TestTempTodoTextTruncation(t *testing.T) {
	svc := newTestTempTodoService(t)
	long := strings.Repeat("甲", TempTodoTextMaxRunes+10)

	todo, err := svc.Add(long, nil)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if got := len([]rune(todo.Text)); got != TempTodoTextMaxRunes {
		t.Errorf("正文长度 = %d, want %d", got, TempTodoTextMaxRunes)
	}
}
