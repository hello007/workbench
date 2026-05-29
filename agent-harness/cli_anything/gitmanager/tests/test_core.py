"""单元测试 — 合成数据，无外部依赖。"""

import json
import os
import tempfile

import pytest

from cli_anything.gitmanager.core.project import DirectoryConfig
from cli_anything.gitmanager.core.session import Session
from cli_anything.gitmanager.core.fileops import _find_available_name
from cli_anything.gitmanager.core.gitops import _status_type


# ─── DirectoryConfig ─────────────────────────────────────────────────

class TestDirectoryConfig:
    def setup_method(self):
        self.tmp = tempfile.mkdtemp()
        self.config_path = os.path.join(self.tmp, "directories.json")
        self.cfg = DirectoryConfig(self.config_path)

    def test_load_empty(self):
        assert self.cfg.load() == []

    def test_add_and_load(self):
        d = self.cfg.add("test", self.tmp)
        loaded = self.cfg.load()
        assert len(loaded) == 1
        assert loaded[0]["name"] == "test"
        assert loaded[0]["path"] == os.path.abspath(self.tmp)

    def test_add_duplicate_path(self):
        self.cfg.add("test", self.tmp)
        with pytest.raises(ValueError, match="已添加"):
            self.cfg.add("test2", self.tmp)

    def test_add_invalid_path(self):
        with pytest.raises(FileNotFoundError, match="路径不存在"):
            self.cfg.add("bad", "/nonexistent/path/xyz")

    def test_remove(self):
        d = self.cfg.add("test", self.tmp)
        self.cfg.remove(d["id"])
        assert self.cfg.load() == []

    def test_remove_nonexistent(self):
        with pytest.raises(ValueError, match="工作目录不存在"):
            self.cfg.remove("fake-id")

    def test_set_default(self):
        d1 = self.cfg.add("dir1", self.tmp, is_default=True)
        tmp2 = self.tmp + "_2"
        os.makedirs(tmp2, exist_ok=True)
        d2 = self.cfg.add("dir2", tmp2)

        self.cfg.set_default(d2["id"])
        assert self.cfg.get_default()["id"] == d2["id"]
        # 第一个不再是默认
        loaded = self.cfg.load()
        for d in loaded:
            if d["id"] == d1["id"]:
                assert d["isDefault"] is False

    def test_get_default_none(self):
        assert self.cfg.get_default() is None

    def test_update(self):
        d = self.cfg.add("old", self.tmp)
        updated = self.cfg.update(d["id"], name="new")
        assert updated["name"] == "new"

    def test_update_nonexistent(self):
        with pytest.raises(ValueError, match="工作目录不存在"):
            self.cfg.update("fake", name="new")

    def test_get_by_id(self):
        d = self.cfg.add("test", self.tmp)
        found = self.cfg.get(d["id"])
        assert found is not None
        assert found["name"] == "test"

    def test_get_by_id_not_found(self):
        assert self.cfg.get("fake") is None

    def test_list_all(self):
        self.cfg.add("a", self.tmp)
        tmp2 = self.tmp + "_2"
        os.makedirs(tmp2, exist_ok=True)
        self.cfg.add("b", tmp2)
        assert len(self.cfg.list_all()) == 2


# ─── Session ─────────────────────────────────────────────────────────

class TestSession:
    def setup_method(self):
        self.tmp = tempfile.mkdtemp()
        self.s = Session(self.tmp)

    def test_initial_state(self):
        assert self.s.current_directory_id is None
        assert self.s.current_path is None
        assert self.s.can_undo is False
        assert self.s.can_redo is False

    def test_set_directory(self):
        self.s.set_directory("dir-1", "/tmp/test")
        assert self.s.current_directory_id == "dir-1"
        assert self.s.current_path == "/tmp/test"

    def test_undo_redo(self):
        self.s.push_undo({"type": "create", "path": "/tmp/f"})
        assert self.s.can_undo
        action = self.s.undo()
        assert action["type"] == "create"
        assert self.s.can_redo
        assert not self.s.can_undo

        action2 = self.s.redo()
        assert action2["type"] == "create"
        assert self.s.can_undo
        assert not self.s.can_redo

    def test_undo_empty(self):
        assert self.s.undo() is None

    def test_redo_empty(self):
        assert self.s.redo() is None

    def test_push_clears_redo(self):
        self.s.push_undo({"type": "a"})
        self.s.undo()
        assert self.s.can_redo
        self.s.push_undo({"type": "b"})
        assert not self.s.can_redo

    def test_save_and_load(self):
        self.s.set_directory("dir-1", "/tmp/test")
        path = self.s.save()
        assert os.path.isfile(path)

        s2 = Session(self.tmp)
        assert s2.load()
        assert s2.current_directory_id == "dir-1"

    def test_load_nonexistent(self):
        assert not self.s.load("nonexistent")

    def test_to_dict(self):
        self.s.set_directory("dir-1", "/tmp/test")
        d = self.s.to_dict()
        assert d["directoryId"] == "dir-1"
        assert "canUndo" in d


# ─── fileops helpers ─────────────────────────────────────────────────

class TestFindAvailableName:
    def test_no_conflict(self):
        assert _find_available_name("/tmp/unique.txt") == "/tmp/unique.txt"

    def test_with_conflict(self):
        with tempfile.TemporaryDirectory() as tmp:
            p = os.path.join(tmp, "file.txt")
            with open(p, "w") as f:
                f.write("exists")
            result = _find_available_name(p)
            assert "(1)" in result


# ─── gitops helpers ──────────────────────────────────────────────────

class TestStatusType:
    def test_modified(self):
        assert _status_type("M") == "modified"

    def test_untracked(self):
        assert _status_type("??") == "untracked"

    def test_deleted(self):
        assert _status_type("D") == "deleted"

    def test_added(self):
        assert _status_type("A") == "added"

    def test_unknown(self):
        assert _status_type("X") == "unknown"
