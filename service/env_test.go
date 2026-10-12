package service

import (
	"errors"
	"reflect"
	"testing"

	"workbench/model"
	"workbench/util"
)

// fakeEnvSourceProvider 环境来源 fake：全部断言走注入数据，不依赖本机真实环境变量
// 与注册表内容（docs/spec/test-stability.md 禁环境外部状态依赖）。
type fakeEnvSourceProvider struct {
	system     []util.EnvEntry
	systemErr  error
	user       []util.EnvEntry
	userErr    error
	process    []util.EnvEntry
	processErr error
}

func (f *fakeEnvSourceProvider) SystemEnv() ([]util.EnvEntry, error) { return f.system, f.systemErr }

func (f *fakeEnvSourceProvider) UserEnv() ([]util.EnvEntry, error) { return f.user, f.userErr }

func (f *fakeEnvSourceProvider) ProcessEnv() ([]util.EnvEntry, error) { return f.process, f.processErr }

func newTestEnvService(p *fakeEnvSourceProvider) *EnvService {
	return &EnvService{provider: p}
}

// TestListEnvVars_AllSourcesSuccess 三来源全成功：分组顺序固定 system → user →
// process，展示名与脱敏标记正确，组内按名称大小写不敏感排序。
func TestListEnvVars_AllSourcesSuccess(t *testing.T) {
	p := &fakeEnvSourceProvider{
		system: []util.EnvEntry{
			{Name: "PATH", Value: "C:\\Windows"},
			{Name: "GITHUB_TOKEN", Value: "ghp_secret"},
		},
		user: []util.EnvEntry{
			{Name: "MY_PASSWD", Value: "p@ss"},
		},
		process: []util.EnvEntry{
			{Name: "b_var", Value: "2"},
			{Name: "ALPHA", Value: "1"},
			{Name: "C_VAR", Value: "3"},
		},
	}
	got, err := newTestEnvService(p).ListEnvVars()
	if err != nil {
		t.Fatalf("ListEnvVars: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("应返回 3 个分组: got %d", len(got))
	}
	wantSources := []string{EnvSourceSystem, EnvSourceUser, EnvSourceProcess}
	wantNames := []string{"系统环境变量", "用户环境变量", "进程环境变量"}
	for i, g := range got {
		if g.Source != wantSources[i] || g.Name != wantNames[i] {
			t.Errorf("分组[%d] 来源/展示名不符: got (%q, %q), want (%q, %q)",
				i, g.Source, g.Name, wantSources[i], wantNames[i])
		}
		if g.Error != "" {
			t.Errorf("分组[%d] 成功时 Error 应为空: got %q", i, g.Error)
		}
	}
	if !reflect.DeepEqual(got[0].Vars, []model.EnvVarInfo{
		{Name: "GITHUB_TOKEN", Value: "ghp_secret", Sensitive: true},
		{Name: "PATH", Value: "C:\\Windows", Sensitive: false},
	}) {
		t.Errorf("系统级变量（脱敏标记 + 排序）不符: %+v", got[0].Vars)
	}
	if !reflect.DeepEqual(got[1].Vars, []model.EnvVarInfo{
		{Name: "MY_PASSWD", Value: "p@ss", Sensitive: true},
	}) {
		t.Errorf("用户级变量不符: %+v", got[1].Vars)
	}
	// 大小写不敏感排序：ALPHA < b_var < C_VAR
	if !reflect.DeepEqual(got[2].Vars, []model.EnvVarInfo{
		{Name: "ALPHA", Value: "1", Sensitive: false},
		{Name: "b_var", Value: "2", Sensitive: false},
		{Name: "C_VAR", Value: "3", Sensitive: false},
	}) {
		t.Errorf("进程级变量（大小写不敏感排序）不符: %+v", got[2].Vars)
	}
}

// TestListEnvVars_SingleSourceFailure 单来源失败不拖垮整体：失败来源产生带 Error 的
// 空列表分组，其余来源正常返回，不返整体 error。
func TestListEnvVars_SingleSourceFailure(t *testing.T) {
	p := &fakeEnvSourceProvider{
		systemErr: errors.New("registry access denied"),
		process:   []util.EnvEntry{{Name: "PATH", Value: "/usr/bin"}},
	}
	got, err := newTestEnvService(p).ListEnvVars()
	if err != nil {
		t.Fatalf("单来源失败不应返整体 error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("失败来源也应有占位分组: got %d", len(got))
	}
	if got[0].Source != EnvSourceSystem || got[0].Error == "" || len(got[0].Vars) != 0 {
		t.Errorf("失败分组应带 Error 且 Vars 为空: %+v", got[0])
	}
	if len(got[1].Vars) != 0 || got[1].Error != "" {
		t.Errorf("user 无数据成功分组应为空列表且无 Error: %+v", got[1])
	}
	if got[2].Source != EnvSourceProcess || len(got[2].Vars) != 1 {
		t.Errorf("process 应正常返回: %+v", got[2])
	}
}

// TestListEnvVars_UnsupportedSkipped 平台不支持来源（Linux 系统/用户级）静默跳过：
// 不产生 Error 分组、不计失败。
func TestListEnvVars_UnsupportedSkipped(t *testing.T) {
	p := &fakeEnvSourceProvider{
		systemErr: util.ErrEnvSourceUnsupported,
		userErr:   util.ErrEnvSourceUnsupported,
		process:   []util.EnvEntry{{Name: "SHELL", Value: "/bin/bash"}},
	}
	got, err := newTestEnvService(p).ListEnvVars()
	if err != nil {
		t.Fatalf("仅 unsupported 不应返整体 error: %v", err)
	}
	if len(got) != 1 || got[0].Source != EnvSourceProcess {
		t.Fatalf("Linux 应仅返回进程级分组: got %+v", got)
	}
	if len(got[0].Vars) != 1 || got[0].Vars[0].Name != "SHELL" {
		t.Errorf("进程级变量不符: %+v", got[0].Vars)
	}
}

// TestListEnvVars_AllFailed 全部来源真实失败：返整体 error（E_ENV_ENUM_FAILED），
// 不返分组。
func TestListEnvVars_AllFailed(t *testing.T) {
	p := &fakeEnvSourceProvider{
		systemErr:  errors.New("system boom"),
		userErr:    errors.New("user boom"),
		processErr: errors.New("process boom"),
	}
	got, err := newTestEnvService(p).ListEnvVars()
	if got != nil {
		t.Fatalf("全部失败应不返分组: got %+v", got)
	}
	if err == nil {
		t.Fatal("全部失败应返整体 error")
	}
	if code := appErrorCode(t, err); code != model.ErrCodeEnvEnumFailed {
		t.Errorf("错误码不符: got %q, want %q", code, model.ErrCodeEnvEnumFailed)
	}
}

// TestListEnvVars_AllAttemptedFailedWithUnsupported 全部「枚举到的」来源均失败：
// unsupported 跳过的来源不计失败，但剩余来源全失败时仍返整体 error。
func TestListEnvVars_AllAttemptedFailedWithUnsupported(t *testing.T) {
	p := &fakeEnvSourceProvider{
		systemErr:  util.ErrEnvSourceUnsupported,
		userErr:    util.ErrEnvSourceUnsupported,
		processErr: errors.New("environ read failed"),
	}
	got, err := newTestEnvService(p).ListEnvVars()
	if got != nil || err == nil {
		t.Fatalf("仅剩来源失败应返整体 error: got %+v, err %v", got, err)
	}
}

// TestListEnvVars_MixedUnsupportedAndPartialSuccess unsupported 与成功/失败混合：
// 有成功来源即不返整体 error，失败来源以 Error 分组呈现。
func TestListEnvVars_MixedUnsupportedAndPartialSuccess(t *testing.T) {
	p := &fakeEnvSourceProvider{
		systemErr: util.ErrEnvSourceUnsupported,
		userErr:   errors.New("user registry boom"),
		process:   []util.EnvEntry{{Name: "HOME", Value: "/root"}},
	}
	got, err := newTestEnvService(p).ListEnvVars()
	if err != nil {
		t.Fatalf("存在成功来源不应返整体 error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应有 user（失败占位）+ process（成功）两个分组: got %+v", got)
	}
	if got[0].Source != EnvSourceUser || got[0].Error == "" {
		t.Errorf("user 失败分组不符: %+v", got[0])
	}
	if got[1].Source != EnvSourceProcess || len(got[1].Vars) != 1 {
		t.Errorf("process 成功分组不符: %+v", got[1])
	}
}

// TestSourceFailureMessage 分组 Error 消息提炼：AppError 取用户可读 Message
// （不含 "[E_...]" 错误码前缀），裸错误拼兜底前缀。
func TestSourceFailureMessage(t *testing.T) {
	if got := sourceFailureMessage(model.NewAppError(model.ErrCodeEnvEnumFailed, "系统环境变量读取失败，请重试")); got != "系统环境变量读取失败，请重试" {
		t.Errorf("AppError 应取 Message: got %q", got)
	}
	if got := sourceFailureMessage(errors.New("boom")); got != "读取失败：boom" {
		t.Errorf("裸错误应拼兜底前缀: got %q", got)
	}
}
