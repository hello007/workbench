"""工作目录管理 — 读写 git-manager 的 directories.json 配置。"""

import json
import os
import time
from typing import Optional


def _default_config_path() -> str:
    """推断 git-manager 的 data/directories.json 路径。"""
    candidates = [
        os.path.join(os.getcwd(), "data", "directories.json"),
        os.path.join(os.path.dirname(__file__), "..", "..", "..", "..", "data", "directories.json"),
    ]
    for p in candidates:
        if os.path.isfile(p):
            return os.path.abspath(p)
    return candidates[0]


class DirectoryConfig:
    """管理 git-manager 工作目录配置。"""

    def __init__(self, config_path: Optional[str] = None):
        self.config_path = config_path or _default_config_path()

    def load(self) -> list[dict]:
        if not os.path.isfile(self.config_path):
            return []
        with open(self.config_path, "r", encoding="utf-8") as f:
            data = json.load(f)
        return data.get("directories", [])

    def save(self, directories: list[dict]) -> None:
        os.makedirs(os.path.dirname(self.config_path), exist_ok=True)
        with open(self.config_path, "w", encoding="utf-8") as f:
            json.dump({"directories": directories}, f, ensure_ascii=False, indent=2)

    def list_all(self) -> list[dict]:
        return self.load()

    def get(self, dir_id: str) -> Optional[dict]:
        for d in self.load():
            if d["id"] == dir_id:
                return d
        return None

    def get_default(self) -> Optional[dict]:
        for d in self.load():
            if d.get("isDefault"):
                return d
        return None

    def add(self, name: str, path: str, is_default: bool = False) -> dict:
        abs_path = os.path.abspath(path)
        if not os.path.isdir(abs_path):
            raise FileNotFoundError(f"路径不存在: {abs_path}")

        directories = self.load()
        for d in directories:
            if d["path"] == abs_path:
                raise ValueError(f"该目录已添加: {abs_path}")

        new_dir = {
            "id": f"dir-{int(time.time() * 1e9)}",
            "name": name,
            "path": abs_path,
            "isDefault": is_default,
            "createTime": time.strftime("%Y-%m-%dT%H:%M:%S.0000000+08:00"),
        }

        if is_default:
            for d in directories:
                d["isDefault"] = False

        directories.append(new_dir)
        self.save(directories)
        return new_dir

    def update(self, dir_id: str, name: Optional[str] = None,
               path: Optional[str] = None, is_default: Optional[bool] = None) -> dict:
        directories = self.load()
        target = None
        for d in directories:
            if d["id"] == dir_id:
                target = d
                break

        if target is None:
            raise ValueError("工作目录不存在")

        if name is not None:
            target["name"] = name
        if path is not None:
            abs_path = os.path.abspath(path)
            if not os.path.isdir(abs_path):
                raise FileNotFoundError(f"路径不存在: {abs_path}")
            target["path"] = abs_path
        if is_default is not None and is_default:
            for d in directories:
                d["isDefault"] = False
            target["isDefault"] = True

        self.save(directories)
        return target

    def remove(self, dir_id: str) -> None:
        directories = self.load()
        new_dirs = [d for d in directories if d["id"] != dir_id]
        if len(new_dirs) == len(directories):
            raise ValueError("工作目录不存在")
        self.save(new_dirs)

    def set_default(self, dir_id: str) -> None:
        directories = self.load()
        found = False
        for d in directories:
            if d["id"] == dir_id:
                d["isDefault"] = True
                found = True
            else:
                d["isDefault"] = False
        if not found:
            raise ValueError("工作目录不存在")
        self.save(directories)
