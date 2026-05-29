# Test Plan — cli-anything-gitmanager

## Test Inventory

| File | Tests | Type |
|------|-------|------|
| test_core.py | ~25 | Unit (synthetic data) |
| test_full_e2e.py | ~15 | E2E (real filesystem + git) |

## Unit Test Plan

### project.py (DirectoryConfig)
- `test_load_empty`: 空配置文件返回空列表
- `test_add_and_load`: 添加目录后可加载
- `test_add_duplicate_path`: 重复路径抛出 ValueError
- `test_add_invalid_path`: 不存在路径抛出 FileNotFoundError
- `test_remove`: 删除目录
- `test_remove_nonexistent`: 删除不存在目录抛出 ValueError
- `test_set_default`: 设置默认目录
- `test_get_default`: 获取默认目录
- `test_update`: 更新目录信息

### session.py (Session)
- `test_initial_state`: 初始状态为空
- `test_set_directory`: 设置工作目录
- `test_undo_redo`: 撤销/重做栈
- `test_save_and_load`: 保存加载会话

### fileops.py (部分)
- `test_find_available_name`: 文件名冲突时自动编号

### gitops.py
- `test_status_type`: 状态码映射

## E2E Test Plan

### Git 操作
- `test_is_git_repo`: 检测当前项目是 Git 仓库
- `test_get_info`: 获取当前仓库信息
- `test_get_branches`: 获取分支列表
- `test_get_log`: 获取提交历史
- `test_get_status`: 获取状态

### 文件操作
- `test_create_and_delete_file`: 创建和删除文件
- `test_create_and_delete_directory`: 创建和删除目录
- `test_rename_file`: 重命名文件
- `test_file_tree`: 文件树浏览
- `test_preview_text_file`: 预览文本文件

### CLI 子进程
- `test_cli_help`: --help 正常输出
- `test_cli_directory_list`: directory list 命令
- `test_cli_git_info`: git info 命令
- `test_cli_json_output`: --json 输出格式

## Realistic Workflow Scenarios

### 工作流 1: 目录管理
1. 添加工作目录 → 2. 列出 → 3. 设为默认 → 4. 切换使用

### 工作流 2: Git 仓库检查
1. 查看仓库信息 → 2. 检查分支 → 3. 查看提交历史 → 4. 检查变更

## Test Results

```
============================= test session starts =============================
platform win32 -- Python 3.13.12, pytest-9.0.3, pluggy-1.6.0
collected 55 items

test_core.py::TestDirectoryConfig::test_load_empty PASSED
test_core.py::TestDirectoryConfig::test_add_and_load PASSED
test_core.py::TestDirectoryConfig::test_add_duplicate_path PASSED
test_core.py::TestDirectoryConfig::test_add_invalid_path PASSED
test_core.py::TestDirectoryConfig::test_remove PASSED
test_core.py::TestDirectoryConfig::test_remove_nonexistent PASSED
test_core.py::TestDirectoryConfig::test_set_default PASSED
test_core.py::TestDirectoryConfig::test_get_default_none PASSED
test_core.py::TestDirectoryConfig::test_update PASSED
test_core.py::TestDirectoryConfig::test_update_nonexistent PASSED
test_core.py::TestDirectoryConfig::test_get_by_id PASSED
test_core.py::TestDirectoryConfig::test_get_by_id_not_found PASSED
test_core.py::TestDirectoryConfig::test_list_all PASSED
test_core.py::TestSession::test_initial_state PASSED
test_core.py::TestSession::test_set_directory PASSED
test_core.py::TestSession::test_undo_redo PASSED
test_core.py::TestSession::test_undo_empty PASSED
test_core.py::TestSession::test_redo_empty PASSED
test_core.py::TestSession::test_push_clears_redo PASSED
test_core.py::TestSession::test_save_and_load PASSED
test_core.py::TestSession::test_load_nonexistent PASSED
test_core.py::TestSession::test_to_dict PASSED
test_core.py::TestFindAvailableName::test_no_conflict PASSED
test_core.py::TestFindAvailableName::test_with_conflict PASSED
test_core.py::TestStatusType::test_modified PASSED
test_core.py::TestStatusType::test_untracked PASSED
test_core.py::TestStatusType::test_deleted PASSED
test_core.py::TestStatusType::test_added PASSED
test_core.py::TestStatusType::test_unknown PASSED
test_full_e2e.py::TestGitOperations::test_is_git_repo PASSED
test_full_e2e.py::TestGitOperations::test_is_not_git_repo PASSED
test_full_e2e.py::TestGitOperations::test_get_info PASSED
test_full_e2e.py::TestGitOperations::test_get_info_non_repo PASSED
test_full_e2e.py::TestGitOperations::test_get_branches PASSED
test_full_e2e.py::TestGitOperations::test_get_branches_non_repo PASSED
test_full_e2e.py::TestGitOperations::test_get_log PASSED
test_full_e2e.py::TestGitOperations::test_get_status PASSED
test_full_e2e.py::TestGitOperations::test_scan_git_repos PASSED
test_full_e2e.py::TestFileOperations::test_create_and_delete_file PASSED
test_full_e2e.py::TestFileOperations::test_create_and_delete_directory PASSED
test_full_e2e.py::TestFileOperations::test_rename PASSED
test_full_e2e.py::TestFileOperations::test_file_tree PASSED
test_full_e2e.py::TestFileOperations::test_preview_text_file PASSED
test_full_e2e.py::TestFileOperations::test_preview_binary_file PASSED
test_full_e2e.py::TestFileOperations::test_copy_item PASSED
test_full_e2e.py::TestFileOperations::test_move_item PASSED
test_full_e2e.py::TestFileOperations::test_create_file_exists PASSED
test_full_e2e.py::TestFileOperations::test_rename_conflict PASSED
test_full_e2e.py::TestFileOperations::test_delete_nonexistent PASSED
test_full_e2e.py::TestCLISubprocess::test_help PASSED
test_full_e2e.py::TestCLISubprocess::test_directory_help PASSED
test_full_e2e.py::TestCLISubprocess::test_git_help PASSED
test_full_e2e.py::TestCLISubprocess::test_directory_list_json PASSED
test_full_e2e.py::TestCLISubprocess::test_git_info_json PASSED
test_full_e2e.py::TestCLISubprocess::test_session_status_json PASSED

============================= 55 passed in 5.35s ==============================
```

### Summary

- **Total tests**: 55
- **Pass rate**: 100%
- **Execution time**: 5.35s
- **Unit tests**: 29 (synthetic data, no external deps)
- **E2E tests**: 26 (real filesystem + git CLI + subprocess)
- **Coverage gaps**: clone/checkout 操作未覆盖（需要远程仓库和干净的工作目录）
