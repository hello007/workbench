package service

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

func TestOpenInExplorer_Directory(t *testing.T) {
	t.Skip("会启动外部 GUI 进程（资源管理器/VSCode），默认跳过；如需手动验证请删除此行")
	dir := t.TempDir()
	svc := NewFileOperationService()

	err := svc.OpenInExplorer(dir)
	if err != nil {
		t.Fatalf("OpenInExplorer(directory) failed: %v", err)
	}
}

func TestOpenInExplorer_File(t *testing.T) {
	t.Skip("会启动外部 GUI 进程（资源管理器/VSCode），默认跳过；如需手动验证请删除此行")
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	os.WriteFile(file, []byte("test"), 0644)

	svc := NewFileOperationService()

	err := svc.OpenInExplorer(file)
	if err != nil {
		t.Fatalf("OpenInExplorer(file) failed: %v", err)
	}
}

func TestOpenInExplorer_NotFound(t *testing.T) {
	svc := NewFileOperationService()

	err := svc.OpenInExplorer("C:\\nonexistent\\path\\that\\does\\not\\exist")
	if err == nil {
		t.Fatal("Expected error for nonexistent path")
	}
}

func TestOpenInExplorer_EmptyPath(t *testing.T) {
	svc := NewFileOperationService()

	err := svc.OpenInExplorer("")
	if err == nil {
		t.Fatal("Expected error for empty path")
	}
}

func TestCreateDirectory_New(t *testing.T) {
	dir := t.TempDir()
	svc := NewFileOperationService()

	err := svc.CreateDirectory(dir, "newdir")
	if err != nil {
		t.Fatalf("CreateDirectory failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "newdir"))
	if err != nil {
		t.Fatalf("Created directory not found: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected directory")
	}
}

func TestCreateDirectory_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "existing"), 0755)

	svc := NewFileOperationService()

	err := svc.CreateDirectory(dir, "existing")
	if err != os.ErrExist {
		t.Fatalf("Expected os.ErrExist, got: %v", err)
	}
}

func TestCreateFile_New(t *testing.T) {
	dir := t.TempDir()
	svc := NewFileOperationService()

	err := svc.CreateFile(dir, "newfile.txt", "hello")
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "newfile.txt"))
	if err != nil {
		t.Fatalf("Created file not found: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(data))
	}
}

func TestCreateFile_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0644)

	svc := NewFileOperationService()

	err := svc.CreateFile(dir, "existing.txt", "new content")
	if err != os.ErrExist {
		t.Fatalf("Expected os.ErrExist, got: %v", err)
	}
}

func TestRename_File(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	os.WriteFile(oldPath, []byte("data"), 0644)

	svc := NewFileOperationService()

	err := svc.Rename(oldPath, "new.txt")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err != nil {
		t.Error("Renamed file not found")
	}
	if _, err := os.Stat(oldPath); err == nil {
		t.Error("Old file still exists after rename")
	}
}

func TestRename_TargetExists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)

	svc := NewFileOperationService()

	err := svc.Rename(filepath.Join(dir, "a.txt"), "b.txt")
	if err != os.ErrExist {
		t.Fatalf("Expected os.ErrExist, got: %v", err)
	}
}

func TestDelete_File(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "to-delete.txt")
	os.WriteFile(file, []byte("x"), 0644)

	svc := NewFileOperationService()

	err := svc.Delete(file)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := os.Stat(file); err == nil {
		t.Error("File still exists after delete")
	}
}

func TestDelete_Directory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "to-delete")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "inner.txt"), []byte("x"), 0644)

	svc := NewFileOperationService()

	err := svc.Delete(subdir)
	if err != nil {
		t.Fatalf("Delete directory failed: %v", err)
	}

	if _, err := os.Stat(subdir); err == nil {
		t.Error("Directory still exists after delete")
	}
}

func TestOpenInVSCode_Directory(t *testing.T) {
	t.Skip("会启动外部 GUI 进程（资源管理器/VSCode），默认跳过；如需手动验证请删除此行")
	dir := t.TempDir()
	svc := NewFileOperationService()

	err := svc.OpenInVSCode(dir)
	if err != nil {
		t.Fatalf("OpenInVSCode(directory) failed: %v", err)
	}
}

func TestOpenInVSCode_File(t *testing.T) {
	t.Skip("会启动外部 GUI 进程（资源管理器/VSCode），默认跳过；如需手动验证请删除此行")
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	os.WriteFile(file, []byte("test"), 0644)

	svc := NewFileOperationService()

	err := svc.OpenInVSCode(file)
	if err != nil {
		t.Fatalf("OpenInVSCode(file) failed: %v", err)
	}
}

func TestOpenInVSCode_InvalidCommand(t *testing.T) {
	t.Skip("会启动外部 GUI 进程（资源管理器/VSCode），默认跳过；如需手动验证请删除此行")
	svc := NewFileOperationService()

	err := svc.OpenInVSCode("")
	_ = err
}

func TestFindAvailableName_NoConflict(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test.txt")
	result := findAvailableName(target)
	if result != target {
		t.Errorf("Expected %s, got %s", target, result)
	}
}

func TestFindAvailableName_FileConflict(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test.txt")
	os.WriteFile(target, []byte("x"), 0644)

	result := findAvailableName(target)
	expected := filepath.Join(dir, "test(1).txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFindAvailableName_MultipleConflicts(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test.txt")
	os.WriteFile(target, []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "test(1).txt"), []byte("x"), 0644)

	result := findAvailableName(target)
	expected := filepath.Join(dir, "test(2).txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFindAvailableName_DirectoryConflict(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "folder")
	os.MkdirAll(target, 0755)

	result := findAvailableName(target)
	expected := filepath.Join(dir, "folder(1)")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestCopyItem_File(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("hello"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.CopyItem(src, targetDir)
	if err != nil {
		t.Fatalf("CopyItem failed: %v", err)
	}

	expected := filepath.Join(targetDir, "test.txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("Copied file not found: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(data))
	}
}

func TestCopyItem_FileConflictAutoRename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("original"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)
	os.WriteFile(filepath.Join(targetDir, "test.txt"), []byte("existing"), 0644)

	svc := NewFileOperationService()
	result, err := svc.CopyItem(src, targetDir)
	if err != nil {
		t.Fatalf("CopyItem failed: %v", err)
	}

	expected := filepath.Join(targetDir, "test(1).txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	data, _ := os.ReadFile(expected)
	if string(data) != "original" {
		t.Errorf("Expected 'original', got '%s'", string(data))
	}
}

func TestCopyItem_Directory(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "srcdir")
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub", "nested.txt"), []byte("nested"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.CopyItem(srcDir, targetDir)
	if err != nil {
		t.Fatalf("CopyItem directory failed: %v", err)
	}

	expected := filepath.Join(targetDir, "srcdir")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	data, _ := os.ReadFile(filepath.Join(expected, "sub", "nested.txt"))
	if string(data) != "nested" {
		t.Errorf("Expected 'nested', got '%s'", string(data))
	}
}

func TestCopyItem_SourceNotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewFileOperationService()

	_, err := svc.CopyItem(filepath.Join(dir, "nonexistent"), dir)
	if err == nil {
		t.Fatal("Expected error for nonexistent source")
	}
}

func TestMoveItem_File(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	src := filepath.Join(srcDir, "test.txt")
	os.WriteFile(src, []byte("move me"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.MoveItem(src, targetDir)
	if err != nil {
		t.Fatalf("MoveItem failed: %v", err)
	}

	if _, err := os.Stat(src); err == nil {
		t.Error("Source file still exists after move")
	}

	data, _ := os.ReadFile(result)
	if string(data) != "move me" {
		t.Errorf("Expected 'move me', got '%s'", string(data))
	}
}

func TestMoveItem_SameDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("x"), 0644)

	svc := NewFileOperationService()
	_, err := svc.MoveItem(src, dir)
	if err == nil {
		t.Fatal("Expected error when source and target are same directory")
	}
}

func TestMoveItem_FileConflictAutoRename(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("moving"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)
	os.WriteFile(filepath.Join(targetDir, "test.txt"), []byte("existing"), 0644)

	svc := NewFileOperationService()
	result, err := svc.MoveItem(filepath.Join(srcDir, "test.txt"), targetDir)
	if err != nil {
		t.Fatalf("MoveItem failed: %v", err)
	}

	expected := filepath.Join(targetDir, "test(1).txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestCopyTo_FileToExistingDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("hello"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.CopyTo(src, targetDir, false)
	if err != nil {
		t.Fatalf("CopyTo failed: %v", err)
	}

	expected := filepath.Join(targetDir, "test.txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("Copied file not found: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(data))
	}
}

func TestCopyTo_FileToNewDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("hello"), 0644)

	targetDir := filepath.Join(dir, "newdest")

	svc := NewFileOperationService()
	result, err := svc.CopyTo(src, targetDir, false)
	if err != nil {
		t.Fatalf("CopyTo failed: %v", err)
	}

	expected := filepath.Join(targetDir, "test.txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("Copied file not found: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(data))
	}
}

func TestCopyTo_DirWholeDir(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "srcdir")
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub", "nested.txt"), []byte("nested"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.CopyTo(srcDir, targetDir, true)
	if err != nil {
		t.Fatalf("CopyTo failed: %v", err)
	}

	expected := filepath.Join(targetDir, "srcdir")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	data, _ := os.ReadFile(filepath.Join(expected, "sub", "nested.txt"))
	if string(data) != "nested" {
		t.Errorf("Expected 'nested', got '%s'", string(data))
	}
}

func TestCopyTo_DirContentOnly(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "srcdir")
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub", "nested.txt"), []byte("nested"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	_, err := svc.CopyTo(srcDir, targetDir, false)
	if err != nil {
		t.Fatalf("CopyTo failed: %v", err)
	}

	// 目标下不应有 srcdir 目录
	srcdirInTarget := filepath.Join(targetDir, "srcdir")
	if _, err := os.Stat(srcdirInTarget); err == nil {
		t.Errorf("目标下不应存在 srcdir 目录，但找到了: %s", srcdirInTarget)
	}

	// 目标下应有 file.txt 和 sub 目录
	data, err := os.ReadFile(filepath.Join(targetDir, "file.txt"))
	if err != nil {
		t.Fatalf("file.txt not found in target: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("Expected 'content', got '%s'", string(data))
	}

	nestedData, _ := os.ReadFile(filepath.Join(targetDir, "sub", "nested.txt"))
	if string(nestedData) != "nested" {
		t.Errorf("Expected 'nested', got '%s'", string(nestedData))
	}
}

func TestCopyTo_SourceNotExist(t *testing.T) {
	dir := t.TempDir()
	svc := NewFileOperationService()

	_, err := svc.CopyTo(filepath.Join(dir, "nonexistent"), dir, false)
	if err == nil {
		t.Fatal("Expected error for nonexistent source")
	}
	expectedMsg := "原地址不存在:"
	if err != nil && !contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error containing '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestCopyTo_TargetIsFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("hello"), 0644)

	targetFile := filepath.Join(dir, "target.txt")
	os.WriteFile(targetFile, []byte("existing"), 0644)

	svc := NewFileOperationService()
	_, err := svc.CopyTo(src, targetFile, false)
	if err == nil {
		t.Fatal("Expected error when target is a file")
	}
	expectedMsg := "目标地址不是文件夹:"
	if err != nil && !contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error containing '%s', got '%s'", expectedMsg, err.Error())
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func TestCopyTo_SourceIsParentOfTarget(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "parent")
	os.MkdirAll(srcDir, 0755)
	targetDir := filepath.Join(srcDir, "child")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	_, err := svc.CopyTo(srcDir, targetDir, true)
	if err == nil {
		t.Fatal("Expected error when source is parent of target")
	}
}

func TestCopyTo_SourceIsAncestorOfTarget(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "grandparent")
	os.MkdirAll(filepath.Join(srcDir, "middle", "leaf"), 0755)
	targetDir := filepath.Join(srcDir, "middle", "leaf")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	_, err := svc.CopyTo(srcDir, targetDir, true)
	if err == nil {
		t.Fatal("Expected error when source is ancestor of target")
	}
}

func TestCopyTo_SamePath(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "same")
	os.MkdirAll(srcDir, 0755)

	svc := NewFileOperationService()
	_, err := svc.CopyTo(srcDir, srcDir, true)
	if err == nil {
		t.Fatal("Expected error when source and target are the same")
	}
}

func TestPreviewFile_TextFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	content := "Hello, world!"
	os.WriteFile(file, []byte(content), 0644)

	svc := NewFileOperationService()
	maxSize := int64(1024 * 1024) // 1MB
	preview, err := svc.PreviewFile(file, maxSize)

	if err != nil {
		t.Fatalf("PreviewFile failed: %v", err)
	}
	if preview.Error != "" {
		t.Errorf("Unexpected error: %s", preview.Error)
	}
	if preview.Content != content {
		t.Errorf("Expected content '%s', got '%s'", content, preview.Content)
	}
	if preview.IsBinary {
		t.Error("Expected non-binary file")
	}
	if preview.TooLarge {
		t.Error("Expected file not to be too large")
	}
	if preview.Kind != model.KindText {
		t.Errorf("Expected kind %s, got %s", model.KindText, preview.Kind)
	}
	if preview.Name != "test.txt" {
		t.Errorf("Expected name 'test.txt', got '%s'", preview.Name)
	}
	if preview.Size != int64(len(content)) {
		t.Errorf("Expected size %d, got %d", len(content), preview.Size)
	}
}

// TestPreviewFile_TextTooLarge 文本类超过 maxSize（1MB）应标记 tooLarge、不读内容
func TestPreviewFile_TextTooLarge(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "large.txt")
	// 1.1MB 文本内容
	content := make([]byte, 1024*1024+100)
	for i := range content {
		content[i] = byte('a' + (i % 26))
	}
	os.WriteFile(file, content, 0644)

	svc := NewFileOperationService()
	maxSize := int64(1024 * 1024) // 1MB
	preview, err := svc.PreviewFile(file, maxSize)

	if err != nil {
		t.Fatalf("PreviewFile failed: %v", err)
	}
	if preview.Error != "" {
		t.Errorf("Unexpected error: %s", preview.Error)
	}
	if preview.Content != "" {
		t.Errorf("Expected empty content for too large file, got %d bytes", len(preview.Content))
	}
	if !preview.TooLarge {
		t.Error("Expected file to be too large")
	}
	if preview.Kind != model.KindText {
		t.Errorf("Expected kind %s, got %s", model.KindText, preview.Kind)
	}
	if preview.Size != int64(len(content)) {
		t.Errorf("Expected size %d, got %d", len(content), preview.Size)
	}
}

// TestPreviewFile_NonTextNotTooLarge 非文本类型（image/pdf/office）不判 tooLarge、不读内容，
// 仅按扩展名返回 kind，由前端按 kind 走各自路径处理（image/office→ReadFileBytes，pdf→iframe）。
func TestPreviewFile_NonTextNotTooLarge(t *testing.T) {
	// 故意构造 >1MB 的内容，验证不再被误标 tooLarge
	content := make([]byte, 1024*1024+100)
	for i := range content {
		content[i] = byte('a' + (i % 26))
	}

	cases := []struct {
		name string
		ext  string
		kind string
	}{
		{"image png", ".png", model.KindImage},
		{"image jpg", ".jpg", model.KindImage},
		{"pdf", ".pdf", model.KindPDF},
		{"docx", ".docx", model.KindOffice},
		{"xlsx", ".xlsx", model.KindOffice},
	}

	dir := t.TempDir()
	svc := NewFileOperationService()
	maxSize := int64(1024 * 1024) // 1MB

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(dir, "test"+tc.ext)
			os.WriteFile(file, content, 0644)

			preview, err := svc.PreviewFile(file, maxSize)
			if err != nil {
				t.Fatalf("PreviewFile failed: %v", err)
			}
			if preview.Error != "" {
				t.Errorf("Unexpected error: %s", preview.Error)
			}
			if preview.TooLarge {
				t.Error("Non-text file should NOT be marked tooLarge (size limit only applies to text)")
			}
			if preview.Content != "" {
				t.Errorf("Non-text file should have empty content, got %d bytes", len(preview.Content))
			}
			if preview.Kind != tc.kind {
				t.Errorf("Expected kind %s, got %s", tc.kind, preview.Kind)
			}
		})
	}
}

// TestPreviewFile_UnsupportedKind 不支持的扩展名返回 kind=unsupported
func TestPreviewFile_UnsupportedKind(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "binary.dat")
	content := []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x00, 0x77, 0x6F, 0x72, 0x6C, 0x64}
	os.WriteFile(file, content, 0644)

	svc := NewFileOperationService()
	maxSize := int64(1024 * 1024)
	preview, err := svc.PreviewFile(file, maxSize)

	if err != nil {
		t.Fatalf("PreviewFile failed: %v", err)
	}
	if preview.Error != "" {
		t.Errorf("Unexpected error: %s", preview.Error)
	}
	if preview.Kind != model.KindUnsupported {
		t.Errorf("Expected kind %s, got %s", model.KindUnsupported, preview.Kind)
	}
	if preview.TooLarge {
		t.Error("Unsupported file should NOT be tooLarge")
	}
	if preview.Content != "" {
		t.Errorf("Unsupported file should have empty content, got %d bytes", len(preview.Content))
	}
}

func TestPreviewFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nonexistent.txt")

	svc := NewFileOperationService()
	maxSize := int64(1024 * 1024)
	preview, err := svc.PreviewFile(file, maxSize)

	if err == nil {
		t.Fatal("Expected error for nonexistent file")
	}
	if preview.Error == "" {
		t.Error("Expected error to be set in preview")
	}
}

func TestPreviewFile_PreviewableExtension(t *testing.T) {
	testCases := []struct {
		name        string
		ext         string
		content     []byte
		expectError bool
	}{
		{"Go source file", ".go", []byte("package main\nfunc main() {}"), false},
		{"JavaScript file", ".js", []byte("console.log('hello');"), false},
		{"Vue file", ".vue", []byte("<template><div></div></template><script>export default {}</script>"), false},
		{"Markdown file", ".md", []byte("# Header\nHello world"), false},
		{"JSON file", ".json", []byte(`{"name": "test", "value": 42}`), false},
		{"Text file", ".txt", []byte("Plain text file"), false},
		{"HTML file", ".html", []byte("<html><body><p>test</p></body></html>"), false},
		{"CSS file", ".css", []byte("body { font-size: 14px; }"), false},
	}

	dir := t.TempDir()
	svc := NewFileOperationService()
	maxSize := int64(1024 * 1024)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(dir, "test"+tc.ext)
			os.WriteFile(file, tc.content, 0644)

			preview, err := svc.PreviewFile(file, maxSize)

			if tc.expectError {
				if err == nil && preview.Error == "" {
					t.Error("Expected error for non-previewable file")
				}
			} else {
				if err != nil {
					t.Fatalf("PreviewFile failed: %v", err)
				}
				if preview.Error != "" {
					t.Errorf("Unexpected error: %s", preview.Error)
				}
				if !preview.IsBinary && !preview.TooLarge {
					if preview.Content == "" {
						t.Error("Expected content for previewable file")
					}
				}
			}
		})
	}
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSaveFile_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	os.WriteFile(file, []byte("original"), 0644)

	svc := NewFileOperationService()
	err := svc.SaveFile(file, "updated content", "")
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if string(data) != "updated content" {
		t.Errorf("Expected 'updated content', got '%s'", string(data))
	}
}

func TestSaveFile_FileNotFound(t *testing.T) {
	svc := NewFileOperationService()
	err := svc.SaveFile(filepath.Join(t.TempDir(), "nonexistent.txt"), "content", "")
	if err == nil {
		t.Fatal("Expected error for nonexistent file")
	}
}

func TestSaveFile_DirectoryPath(t *testing.T) {
	dir := t.TempDir()
	svc := NewFileOperationService()
	err := svc.SaveFile(dir, "content", "")
	if err == nil {
		t.Fatal("Expected error when saving to a directory")
	}
}

func TestSaveFile_ContentTooLarge(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "big.txt")
	os.WriteFile(file, []byte("small"), 0644)

	svc := NewFileOperationService()
	largeContent := string(make([]byte, 1024*1024+1)) // > 1MB
	err := svc.SaveFile(file, largeContent, "")
	if err == nil {
		t.Fatal("Expected error for content exceeding size limit")
	}
}

func TestSaveFile_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "empty.txt")
	os.WriteFile(file, []byte("has content"), 0644)

	svc := NewFileOperationService()
	err := svc.SaveFile(file, "", "")
	if err != nil {
		t.Fatalf("SaveFile with empty content failed: %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if string(data) != "" {
		t.Errorf("Expected empty content, got '%s'", string(data))
	}
}

func TestSaveFile_NoTempFileLeak(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "clean.txt")
	os.WriteFile(file, []byte("before"), 0644)

	svc := NewFileOperationService()
	err := svc.SaveFile(file, "after", "")
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "clean.txt" {
			t.Errorf("Unexpected file left behind: %s", e.Name())
		}
	}
}

// TestSaveFile_UTF8Encoding 按 UTF-8 保存中文应写入 UTF-8 字节
func TestSaveFile_UTF8Encoding(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "utf8.txt")
	os.WriteFile(file, []byte("placeholder"), 0644)

	svc := NewFileOperationService()
	content := "中文UTF-8文本"
	err := svc.SaveFile(file, content, "utf-8")
	if err != nil {
		t.Fatalf("SaveFile with UTF-8 failed: %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if string(data) != content {
		t.Errorf("UTF-8 保存内容不匹配, 期望 '%s', 实际 '%s'", content, string(data))
	}
}

// TestSaveFile_GBKEncoding 按 GBK 保存中文应写入 GBK 字节（非 UTF-8），读回验证编码未被改成 UTF-8
func TestSaveFile_GBKEncoding(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "gbk.txt")
	os.WriteFile(file, []byte("placeholder"), 0644)

	svc := NewFileOperationService()
	content := "中文GBK文本"
	err := svc.SaveFile(file, content, "gbk")
	if err != nil {
		t.Fatalf("SaveFile with GBK failed: %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	// "中文GBK文本" 的 GBK 编码字节
	// 中=D6D0 文=CEC4 G=47 B=42 K=4B 文=CEC4 本=B1BE
	expectedGBK := []byte{0xD6, 0xD0, 0xCE, 0xC4, 0x47, 0x42, 0x4B, 0xCE, 0xC4, 0xB1, 0xBE}
	if !bytes.Equal(data, expectedGBK) {
		t.Errorf("GBK 保存字节不匹配, 期望 %v, 实际 %v", expectedGBK, data)
	}
	// 关键回归：保存后文件不应是 UTF-8 编码（否则会改变原文件编码）
	if string(data) == content {
		t.Error("GBK 保存后文件不应是 UTF-8 编码（字节应与 UTF-8 不同）")
	}
}

// TestPreviewFile_UnsupportedTextDowngrade unsupported 类型的小文本文件应降级为 text 显示
func TestPreviewFile_UnsupportedTextDowngrade(t *testing.T) {
	dir := t.TempDir()
	// 无扩展名文件（detectPreviewKind 返回 unsupported）
	file := filepath.Join(dir, "readme")
	content := "this is a plain text file without extension"
	os.WriteFile(file, []byte(content), 0644)

	svc := NewFileOperationService()
	maxSize := int64(1024 * 1024)
	preview, err := svc.PreviewFile(file, maxSize)

	if err != nil {
		t.Fatalf("PreviewFile failed: %v", err)
	}
	if preview.Error != "" {
		t.Errorf("Unexpected error: %s", preview.Error)
	}
	if preview.Kind != model.KindText {
		t.Errorf("unsupported 文本应降级为 text, 实际 %s", preview.Kind)
	}
	if preview.Content != content {
		t.Errorf("内容期望 '%s', 实际 '%s'", content, preview.Content)
	}
	if preview.Encoding != "utf-8" {
		t.Errorf("编码期望 utf-8, 实际 %s", preview.Encoding)
	}
	if preview.IsBinary {
		t.Error("UTF-8 文本不应判为二进制")
	}
	if preview.TooLarge {
		t.Error("小文件不应判为过大")
	}
}

// TestPreviewFile_GBKTextFile GBK 编码的文本文件应正确解码显示中文，encoding=gbk
func TestPreviewFile_GBKTextFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "gbk.txt")
	// "中文GBK文本" 的 GBK 编码字节
	gbkData := []byte{0xD6, 0xD0, 0xCE, 0xC4, 0x47, 0x42, 0x4B, 0xCE, 0xC4, 0xB1, 0xBE}
	os.WriteFile(file, gbkData, 0644)

	svc := NewFileOperationService()
	maxSize := int64(1024 * 1024)
	preview, err := svc.PreviewFile(file, maxSize)

	if err != nil {
		t.Fatalf("PreviewFile failed: %v", err)
	}
	if preview.Error != "" {
		t.Errorf("Unexpected error: %s", preview.Error)
	}
	if preview.Kind != model.KindText {
		t.Errorf("GBK 文本应为 text, 实际 %s", preview.Kind)
	}
	if preview.Encoding != "gbk" {
		t.Errorf("编码期望 gbk, 实际 %s", preview.Encoding)
	}
	if preview.Content != "中文GBK文本" {
		t.Errorf("解码内容期望 '中文GBK文本', 实际 '%s'", preview.Content)
	}
	if preview.IsBinary {
		t.Error("GBK 文本不应判为二进制")
	}
}

// TestPreviewFile_BinaryUnsupportedIsBinary unsupported 二进制文件（含 NUL）应标记 IsBinary，不显示内容
func TestPreviewFile_BinaryUnsupportedIsBinary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "binary.dat")
	content := []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x00, 0x77, 0x6F, 0x72, 0x6C, 0x64}
	os.WriteFile(file, content, 0644)

	svc := NewFileOperationService()
	maxSize := int64(1024 * 1024)
	preview, err := svc.PreviewFile(file, maxSize)

	if err != nil {
		t.Fatalf("PreviewFile failed: %v", err)
	}
	if !preview.IsBinary {
		t.Error("含 NUL 字节应判为二进制（IsBinary=true）")
	}
	if preview.Content != "" {
		t.Errorf("二进制文件内容应为空, 实际 %d bytes", len(preview.Content))
	}
	if preview.Kind != model.KindUnsupported {
		t.Errorf("Kind 应保留 unsupported, 实际 %s", preview.Kind)
	}
}

// TestPreviewFile_UnsupportedTooLarge unsupported 大文件应标记 tooLarge，不读内容
func TestPreviewFile_UnsupportedTooLarge(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "big.log")
	content := make([]byte, 1024*1024+100)
	for i := range content {
		content[i] = byte('a' + (i % 26))
	}
	os.WriteFile(file, content, 0644)

	svc := NewFileOperationService()
	maxSize := int64(1024 * 1024)
	preview, err := svc.PreviewFile(file, maxSize)

	if err != nil {
		t.Fatalf("PreviewFile failed: %v", err)
	}
	if !preview.TooLarge {
		t.Error("超大 unsupported 文件应标记 tooLarge")
	}
	if preview.Content != "" {
		t.Errorf("超大文件内容应为空, 实际 %d bytes", len(preview.Content))
	}
}
