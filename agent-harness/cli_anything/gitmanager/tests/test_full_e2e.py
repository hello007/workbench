"""E2E 测试 — 真实文件系统和 Git 操作。"""

import json
import os
import subprocess
import sys
import tempfile

import pytest

from cli_anything.gitmanager.core import gitops, fileops, project


# ─── Git 操作 ─────────────────────────────────────────────────────────

class TestGitOperations:
    """基于当前 git-manager 仓库的真实 Git 操作。"""

    @pytest.fixture
    def repo_path(self):
        return os.path.dirname(os.path.dirname(os.path.dirname(
            os.path.dirname(os.path.dirname(os.path.abspath(__file__))))))

    def test_is_git_repo(self, repo_path):
        assert gitops.is_git_repo(repo_path) is True

    def test_is_not_git_repo(self):
        with tempfile.TemporaryDirectory() as tmp:
            assert gitops.is_git_repo(tmp) is False

    def test_get_info(self, repo_path):
        info = gitops.get_info(repo_path)
        assert info["isRepo"] is True
        assert info["branch"]
        assert "path" in info

    def test_get_info_non_repo(self):
        with tempfile.TemporaryDirectory() as tmp:
            info = gitops.get_info(tmp)
            assert info["isRepo"] is False

    def test_get_branches(self, repo_path):
        branches = gitops.get_branches(repo_path)
        assert len(branches) > 0
        names = [b["name"] for b in branches]
        assert "master" in names or "main" in names

    def test_get_branches_non_repo(self):
        with tempfile.TemporaryDirectory() as tmp:
            with pytest.raises(ValueError, match="不是 Git 仓库"):
                gitops.get_branches(tmp)

    def test_get_log(self, repo_path):
        commits = gitops.get_log(repo_path, limit=5)
        assert len(commits) > 0
        assert commits[0]["sha"]
        assert commits[0]["message"]

    def test_get_status(self, repo_path):
        changes = gitops.get_status(repo_path)
        assert isinstance(changes, list)

    def test_scan_git_repos(self, repo_path):
        parent = os.path.dirname(repo_path)
        repos = gitops.scan_git_repos(parent)
        assert len(repos) >= 1


# ─── 文件操作 ─────────────────────────────────────────────────────────

class TestFileOperations:
    @pytest.fixture
    def tmp_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            yield tmp

    def test_create_and_delete_file(self, tmp_dir):
        path = fileops.create_file(tmp_dir, "test.txt", "hello")
        assert os.path.isfile(path)
        with open(path) as f:
            assert f.read() == "hello"
        fileops.delete(path)
        assert not os.path.exists(path)

    def test_create_and_delete_directory(self, tmp_dir):
        path = fileops.create_directory(tmp_dir, "subdir")
        assert os.path.isdir(path)
        fileops.delete(path)
        assert not os.path.exists(path)

    def test_rename(self, tmp_dir):
        path = fileops.create_file(tmp_dir, "old.txt", "data")
        new_path = fileops.rename(path, "new.txt")
        assert not os.path.exists(path)
        assert os.path.isfile(new_path)

    def test_file_tree(self, tmp_dir):
        fileops.create_file(tmp_dir, "a.txt")
        fileops.create_directory(tmp_dir, "sub")
        tree = fileops.get_tree(tmp_dir, max_depth=1)
        names = [n["name"] for n in tree]
        assert "a.txt" in names
        assert "sub" in names

    def test_preview_text_file(self, tmp_dir):
        path = fileops.create_file(tmp_dir, "readme.md", "# Hello")
        result = fileops.preview_file(path)
        assert result["content"] == "# Hello"
        assert not result.get("isBinary")

    def test_preview_binary_file(self, tmp_dir):
        path = os.path.join(tmp_dir, "bin.dat")
        with open(path, "wb") as f:
            f.write(b"\x00\x01\x02")
        result = fileops.preview_file(path)
        assert result.get("isBinary")

    def test_copy_item(self, tmp_dir):
        src = fileops.create_file(tmp_dir, "src.txt", "data")
        target_dir = fileops.create_directory(tmp_dir, "dest")
        result = fileops.copy_item(src, target_dir)
        assert os.path.isfile(result)

    def test_move_item(self, tmp_dir):
        src = fileops.create_file(tmp_dir, "src.txt", "data")
        target_dir = fileops.create_directory(tmp_dir, "dest")
        result = fileops.move_item(src, target_dir)
        assert not os.path.exists(src)
        assert os.path.isfile(result)

    def test_create_file_exists(self, tmp_dir):
        fileops.create_file(tmp_dir, "dup.txt")
        with pytest.raises(FileExistsError):
            fileops.create_file(tmp_dir, "dup.txt")

    def test_rename_conflict(self, tmp_dir):
        fileops.create_file(tmp_dir, "a.txt")
        fileops.create_file(tmp_dir, "b.txt")
        with pytest.raises(FileExistsError):
            fileops.rename(os.path.join(tmp_dir, "a.txt"), "b.txt")

    def test_delete_nonexistent(self, tmp_dir):
        with pytest.raises(FileNotFoundError):
            fileops.delete(os.path.join(tmp_dir, "nope.txt"))


# ─── CLI 子进程测试 ───────────────────────────────────────────────────

def _resolve_cli(name):
    """查找已安装的 CLI 命令，未找到则回退到 python -m。"""
    force = os.environ.get("CLI_ANYTHING_FORCE_INSTALLED", "").strip() == "1"
    import shutil
    path = shutil.which(name)
    if path:
        print(f"[_resolve_cli] Using installed command: {path}")
        return [path]
    if force:
        raise RuntimeError(f"{name} not found in PATH. Install with: pip install -e .")
    module = "cli_anything.gitmanager"
    print(f"[_resolve_cli] Falling back to: {sys.executable} -m {module}")
    return [sys.executable, "-m", module]


class TestCLISubprocess:
    CLI_BASE = _resolve_cli("cli-anything-gitmanager")

    def _run(self, args, check=True):
        return subprocess.run(
            self.CLI_BASE + args,
            capture_output=True, text=True, check=check,
        )

    def test_help(self):
        result = self._run(["--help"])
        assert result.returncode == 0
        assert "COMMAND" in result.stdout or "command" in result.stdout

    def test_directory_help(self):
        result = self._run(["directory", "--help"])
        assert result.returncode == 0

    def test_git_help(self):
        result = self._run(["git", "--help"])
        assert result.returncode == 0

    def test_directory_list_json(self):
        result = self._run(["--json", "directory", "list"])
        assert result.returncode == 0
        data = json.loads(result.stdout)
        assert isinstance(data, list)

    def test_git_info_json(self):
        repo = os.path.dirname(os.path.dirname(os.path.dirname(
            os.path.dirname(os.path.dirname(os.path.abspath(__file__))))))
        result = self._run(["--json", "git", "info", repo])
        assert result.returncode == 0
        data = json.loads(result.stdout)
        assert data["isRepo"] is True

    def test_session_status_json(self):
        result = self._run(["--json", "session", "status"])
        assert result.returncode == 0
        data = json.loads(result.stdout)
        assert "canUndo" in data
