package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenInExplorer_Directory(t *testing.T) {
	dir := t.TempDir()
	svc := NewFileOperationService()

	err := svc.OpenInExplorer(dir)
	if err != nil {
		t.Fatalf("OpenInExplorer(directory) failed: %v", err)
	}
}

func TestOpenInExplorer_File(t *testing.T) {
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
	dir := t.TempDir()
	svc := NewFileOperationService()

	err := svc.OpenInVSCode(dir)
	if err != nil {
		t.Fatalf("OpenInVSCode(directory) failed: %v", err)
	}
}

func TestOpenInVSCode_File(t *testing.T) {
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
