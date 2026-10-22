//go:build integration

package main

// 分支对比集成测试（活动栏「分支对比」batch 1 后端，真实 git fixture，无网络）。
//
// fixture 形态：master 基线提交 → branch-a 两个提交 → branch-b：
//   (a) git cherry-pick branch-a 的一个提交（等价补丁 → 同 patch-id）
//   (b) 一个独立提交
//   (c) git mv 重命名（rename 新旧双路径）
//   (d) 一个二进制文件
// 基线含两侧未触碰文件（unchanged 并集断言）。
//
// 运行方式：go test -tags=integration -run TestBranchCompare ./...
// 前置条件：git 在 PATH（缺失 skip 不 fail）。确定性保障同 git_flows_integration_test.go：
// 显式 symbolic-ref master + 局部 user 配置 + core.autocrlf=false + commit.gpgsign=false。
// fixture 复用 util/testutil 辅助与本文件内联 helper（命名前缀 bcmp 防与既有 helper 冲突）。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/service"
	"workbench/util/testutil"
)

// bcmpFindFile 按 Path 查找文件树行，不存在返回 nil。
func bcmpFindFile(t *testing.T, files []model.BranchDiffFile, path string) *model.BranchDiffFile {
	t.Helper()
	for i := range files {
		if files[i].Path == path {
			return &files[i]
		}
	}
	return nil
}

// bcmpSetupRepo 构造分支对比 fixture 仓库，返回仓库根目录。
// 结构（见文件头）：master 基线（common.txt / untouched.txt / rename-src.txt）→
// branch-a 两提交 → branch-b（cherry-pick + 独立提交 + git mv + 二进制文件）。
func bcmpSetupRepo(t *testing.T) string {
	t.Helper()
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	testutil.RunGit(t, dir, "config", "core.autocrlf", "false")
	testutil.RunGit(t, dir, "config", "commit.gpgsign", "false")

	// 基线：跨分支未触碰的 untouched.txt + 供 branch-b 重命名的 rename-src.txt
	testutil.WriteFile(t, filepath.Join(dir, "common.txt"), "base\n")
	testutil.WriteFile(t, filepath.Join(dir, "untouched.txt"), "same\n")
	testutil.WriteFile(t, filepath.Join(dir, "rename-src.txt"), "rename me\n")
	testutil.RunGit(t, dir, "add", "-A")
	testutil.RunGit(t, dir, "commit", "-m", "base")

	// branch-a：两个提交，其中「cherry me」将被 branch-b 拣选（同 patch-id）
	testutil.RunGit(t, dir, "checkout", "-b", "branch-a")
	testutil.WriteFile(t, filepath.Join(dir, "feature-a.txt"), "a1\n")
	testutil.RunGit(t, dir, "add", "-A")
	testutil.RunGit(t, dir, "commit", "-m", "feat: a1")
	testutil.WriteFile(t, filepath.Join(dir, "cherry.txt"), "cherry content\n")
	testutil.RunGit(t, dir, "add", "-A")
	testutil.RunGit(t, dir, "commit", "-m", "feat: cherry me")
	cherrySHA := itRevParse(t, dir, "branch-a")

	// branch-b：cherry-pick（同 patch-id）+ 独立提交 + git mv + 二进制文件
	testutil.RunGit(t, dir, "checkout", "master")
	testutil.RunGit(t, dir, "checkout", "-b", "branch-b")
	testutil.RunGit(t, dir, "cherry-pick", cherrySHA)
	testutil.WriteFile(t, filepath.Join(dir, "independent.txt"), "only b\n")
	testutil.RunGit(t, dir, "add", "-A")
	testutil.RunGit(t, dir, "commit", "-m", "feat: independent")
	testutil.RunGit(t, dir, "mv", "rename-src.txt", "rename-dst.txt")
	testutil.RunGit(t, dir, "commit", "-m", "refactor: rename file")
	if err := os.WriteFile(filepath.Join(dir, "binary.bin"), []byte{0x00, 0x01, 0x02, 0xFF}, 0o644); err != nil {
		t.Fatalf("write binary.bin: %v", err)
	}
	testutil.RunGit(t, dir, "add", "-A")
	testutil.RunGit(t, dir, "commit", "-m", "feat: add binary")

	// 终态：停在 branch-b（被测服务不依赖当前分支，仅 fixture 语义完整）
	return dir
}

// bcmpNewApp 按生产装配范式构造仅含 branchCompareSvc 的 App（无持久化数据，纯构造）。
func bcmpNewApp() *App {
	return &App{AppServices: &AppServices{
		branchCompareSvc: service.NewBranchCompareService(),
	}}
}

// TestBranchCompare_Alignment 对齐主链：merge-base 正确 + 共同历史计数 + cherry-pick
// 提交以 patch-id 配对 + 独立提交两侧独有 + 统计一致。
func TestBranchCompare_Alignment(t *testing.T) {
	itRequireGit(t)
	repo := bcmpSetupRepo(t)
	app := bcmpNewApp()

	mergeBase := itRevParse(t, repo, "master")

	align, err := app.GetBranchCompareAlignment(repo, "branch-a", "branch-b")
	if err != nil {
		t.Fatalf("GetBranchCompareAlignment: %v", err)
	}
	if align.BranchA != "branch-a" || align.BranchB != "branch-b" {
		t.Errorf("A/B 分支名应原样回传, got %+v", align)
	}
	if align.MergeBaseHash != mergeBase {
		t.Errorf("merge-base 应为基线提交 %s, got %s", mergeBase, align.MergeBaseHash)
	}
	if align.CommonHistoryCount != 1 {
		t.Errorf("共同历史应为基线 1 条, got %d", align.CommonHistoryCount)
	}
	if align.Truncated {
		t.Error("小仓库不应截断")
	}

	// cherry-pick 提交以 patch-id 配对（同补丁不同 hash）
	foundPatch := false
	for _, r := range align.Rows {
		if r.Side == model.RowSideMatched && r.MatchedBy == model.RowMatchedByPatchID {
			foundPatch = true
			if r.A == nil || r.B == nil || r.A.Hash == r.B.Hash {
				t.Errorf("patch-id 配对行应双侧异 hash 同补丁, got %+v", r)
			}
		}
	}
	if !foundPatch {
		t.Error("应存在 patch-id（疑似拣选）配对行, rows 见上")
	}

	// a1 提交：仅 branch-a 有（独立内容）→ a-only
	aOnly, bOnly := 0, 0
	for _, r := range align.Rows {
		switch r.Side {
		case model.RowSideAOnly:
			aOnly++
		case model.RowSideBOnly:
			bOnly++
		}
	}
	if aOnly < 1 {
		t.Errorf("branch-a 的 a1 提交应为 a-only, got %d", aOnly)
	}
	// branch-b 独有：independent + rename + binary 三个提交
	if bOnly < 3 {
		t.Errorf("branch-b 应至少 3 条 b-only, got %d", bOnly)
	}

	// 统计条与行自洽
	if align.Stats.Matched+align.Stats.AOnly+align.Stats.BOnly != len(align.Rows) {
		t.Errorf("统计应覆盖全部行: stats=%+v rows=%d", align.Stats, len(align.Rows))
	}
	if align.Stats.PatchMatched != 1 {
		t.Errorf("疑似拣选数应为 1, got %d", align.Stats.PatchMatched)
	}
	if align.Stats.CommonBelow != 1 {
		t.Errorf("共同历史统计应为 1, got %d", align.Stats.CommonBelow)
	}
}

// TestBranchCompare_FileTree 差异文件树：unchanged 并集 / rename 双路径 / 二进制标记 /
// added / deleted / Path 排序。
func TestBranchCompare_FileTree(t *testing.T) {
	itRequireGit(t)
	repo := bcmpSetupRepo(t)
	app := bcmpNewApp()

	files, err := app.GetBranchCompareFileTree(repo, "branch-a", "branch-b")
	if err != nil {
		t.Fatalf("GetBranchCompareFileTree: %v", err)
	}

	// 排序：Path 严格升序
	for i := 1; i < len(files); i++ {
		if files[i-1].Path >= files[i].Path {
			t.Fatalf("文件树应按 Path 升序, got %v", files)
		}
	}

	// unchanged：两侧未触碰的 untouched.txt
	if u := bcmpFindFile(t, files, "untouched.txt"); u == nil || u.Status != model.BranchDiffStatusUnchanged {
		t.Errorf("untouched.txt 应为 unchanged, got %+v", u)
	}

	// 方向语义：diff 方向为 branch-a → branch-b。
	// deleted：feature-a.txt 仅 branch-a 有（切到 branch-b 视角已消失）
	if f := bcmpFindFile(t, files, "feature-a.txt"); f == nil || f.Status != model.BranchDiffStatusDeleted {
		t.Errorf("feature-a.txt 应为 deleted（A→B 方向）, got %+v", f)
	}

	// rename：新路径行带 OldPath 双路径；旧路径并集补回（branch-a 仍在）
	if r := bcmpFindFile(t, files, "rename-dst.txt"); r == nil || r.Status != model.BranchDiffStatusRenamed || r.OldPath != "rename-src.txt" {
		t.Errorf("rename-dst.txt 应为 renamed 且 OldPath=rename-src.txt, got %+v", r)
	}
	if o := bcmpFindFile(t, files, "rename-src.txt"); o == nil {
		t.Error("rename 旧路径应在并集中保留")
	}

	// added：binary.bin 仅 branch-b 有
	if b := bcmpFindFile(t, files, "binary.bin"); b == nil || !b.Binary {
		t.Errorf("binary.bin 应标记二进制, got %+v", b)
	}
	if b := bcmpFindFile(t, files, "binary.bin"); b == nil || b.Status != model.BranchDiffStatusAdded {
		t.Errorf("binary.bin 应为 added, got %+v", b)
	}

	// unchanged：cherry-pick 提交两侧同名同内容，仅出现在 ls-tree 并集
	if c := bcmpFindFile(t, files, "cherry.txt"); c == nil || c.Status != model.BranchDiffStatusUnchanged {
		t.Errorf("cherry.txt 两侧同名同内容应为 unchanged, got %+v", c)
	}
}

// TestBranchCompare_FileTree_Deleted 删除态：branch-b 删除一个 branch-a 继承的文件
// → deleted 状态行（diff 方向 A→B：A 有 B 无即 deleted）。
func TestBranchCompare_FileTree_Deleted(t *testing.T) {
	itRequireGit(t)
	repo := bcmpSetupRepo(t)
	// branch-b 上删除 untouched.txt 并提交（branch-a 仍保留）
	testutil.RunGit(t, repo, "checkout", "branch-b")
	testutil.RunGit(t, repo, "rm", "untouched.txt")
	testutil.RunGit(t, repo, "commit", "-m", "chore: remove untouched")

	app := bcmpNewApp()
	files, err := app.GetBranchCompareFileTree(repo, "branch-a", "branch-b")
	if err != nil {
		t.Fatalf("GetBranchCompareFileTree: %v", err)
	}
	if d := bcmpFindFile(t, files, "untouched.txt"); d == nil || d.Status != model.BranchDiffStatusDeleted {
		t.Errorf("branch-a 独存的 untouched.txt 应为 deleted, got %+v", d)
	}
}

// TestBranchCompare_FileDiff 文件 diff：文本文件输出含 @@ hunk 头；空文件路径中文报错。
func TestBranchCompare_FileDiff(t *testing.T) {
	itRequireGit(t)
	repo := bcmpSetupRepo(t)
	app := bcmpNewApp()

	diff, err := app.GetBranchCompareFileDiff(repo, "branch-a", "branch-b", "independent.txt")
	if err != nil {
		t.Fatalf("GetBranchCompareFileDiff: %v", err)
	}
	if !strings.Contains(diff, "@@") || !strings.Contains(diff, "only b") {
		t.Errorf("独立文件的 diff 应含 @@ 与新增内容, got %q", diff)
	}

	if _, err := app.GetBranchCompareFileDiff(repo, "branch-a", "branch-b", ""); err == nil {
		t.Error("空文件路径应报错")
	} else if !strings.Contains(err.Error(), "文件路径不能为空") {
		t.Errorf("空文件路径错误文案应中文, got %v", err)
	}
}

// TestBranchCompare_CommitChangedFiles 提交变更文件列表：name-status 解析与状态字母。
func TestBranchCompare_CommitChangedFiles(t *testing.T) {
	itRequireGit(t)
	repo := bcmpSetupRepo(t)
	app := bcmpNewApp()

	sha := itRevParse(t, repo, "branch-b")
	files, err := app.GetCommitChangedFiles(repo, sha)
	if err != nil {
		t.Fatalf("GetCommitChangedFiles: %v", err)
	}
	// branch-b 顶提交为 binary.bin 新增
	found := false
	for _, f := range files {
		if f.Path == "binary.bin" {
			found = true
			if f.Status != "A" {
				t.Errorf("binary.bin 状态字母应为 A, got %+v", f)
			}
		}
	}
	if !found {
		t.Errorf("顶提交变更文件应含 binary.bin, got %+v", files)
	}

	if _, err := app.GetCommitChangedFiles(repo, ""); err == nil {
		t.Error("空 SHA 应报错")
	}
}

// TestBranchCompare_UnrelatedHistories 无共同祖先（orphan 分支）：merge-base 容忍为空串，
// 不 panic、不报错。
func TestBranchCompare_UnrelatedHistories(t *testing.T) {
	itRequireGit(t)
	repo := bcmpSetupRepo(t)
	testutil.RunGit(t, repo, "checkout", "--orphan", "isolated")
	testutil.RunGit(t, repo, "rm", "-rf", ".")
	testutil.WriteFile(t, filepath.Join(repo, "orphan.txt"), "orphan\n")
	testutil.RunGit(t, repo, "add", "-A")
	testutil.RunGit(t, repo, "commit", "-m", "orphan root")
	testutil.RunGit(t, repo, "checkout", "branch-a")

	app := bcmpNewApp()
	align, err := app.GetBranchCompareAlignment(repo, "branch-a", "isolated")
	if err != nil {
		t.Fatalf("无共同祖先不应报错: %v", err)
	}
	if align.MergeBaseHash != "" {
		t.Errorf("无共同祖先 merge-base 应为空串, got %s", align.MergeBaseHash)
	}
	if align.CommonHistoryCount != 0 {
		t.Errorf("无共同祖先共同历史应为 0, got %d", align.CommonHistoryCount)
	}
	if len(align.Rows) == 0 {
		t.Error("两侧独有提交仍应产出对齐行")
	}
}

// TestBranchCompare_BranchErrors 分支解析失败分流：不存在的分支返回中文错误
// （文件树显式报「不存在或无法解析」；对齐区间日志失败同样报错不静默）。
func TestBranchCompare_BranchErrors(t *testing.T) {
	itRequireGit(t)
	repo := bcmpSetupRepo(t)
	app := bcmpNewApp()

	if _, err := app.GetBranchCompareFileTree(repo, "branch-a", "no-such-branch"); err == nil {
		t.Fatal("不存在的分支应报错")
	} else if !strings.Contains(err.Error(), "不存在或无法解析") {
		t.Errorf("分支解析错误文案应中文, got %v", err)
	}

	if _, err := app.GetBranchCompareAlignment(repo, "branch-a", "no-such-branch"); err == nil {
		t.Fatal("对齐遇到不存在的分支应报错")
	} else if !strings.Contains(err.Error(), "获取分支") {
		t.Errorf("对齐区间日志错误文案应中文, got %v", err)
	}

	if _, err := app.GetBranchCompareAlignment(repo, "", "branch-b"); err == nil {
		t.Fatal("空分支 A 应报错")
	} else if !strings.Contains(err.Error(), "分支 A 不能为空") {
		t.Errorf("空分支错误文案应中文, got %v", err)
	}
}
