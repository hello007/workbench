"""会话管理 — REPL 状态持久化。"""

import json
import os
import time
from typing import Optional


class Session:
    """维护 CLI 会话状态：当前工作目录、选中路径。"""

    def __init__(self, session_dir: Optional[str] = None):
        self.session_dir = session_dir or os.path.join(
            os.path.expanduser("~"), ".cli-anything-gitmanager", "sessions"
        )
        self.current_directory_id: Optional[str] = None
        self.current_directory_path: Optional[str] = None
        self.current_path: Optional[str] = None
        self._undo_stack: list[dict] = []
        self._redo_stack: list[dict] = []

    def set_directory(self, dir_id: str, dir_path: str) -> None:
        self.current_directory_id = dir_id
        self.current_directory_path = dir_path
        self.current_path = dir_path

    def set_path(self, path: str) -> None:
        self.current_path = path

    def push_undo(self, action: dict) -> None:
        self._undo_stack.append(action)
        self._redo_stack.clear()

    def undo(self) -> Optional[dict]:
        if not self._undo_stack:
            return None
        action = self._undo_stack.pop()
        self._redo_stack.append(action)
        return action

    def redo(self) -> Optional[dict]:
        if not self._redo_stack:
            return None
        action = self._redo_stack.pop()
        self._undo_stack.append(action)
        return action

    @property
    def can_undo(self) -> bool:
        return bool(self._undo_stack)

    @property
    def can_redo(self) -> bool:
        return bool(self._redo_stack)

    def save(self, name: str = "default") -> str:
        os.makedirs(self.session_dir, exist_ok=True)
        path = os.path.join(self.session_dir, f"{name}.json")
        data = {
            "directoryId": self.current_directory_id,
            "directoryPath": self.current_directory_path,
            "currentPath": self.current_path,
            "savedAt": time.strftime("%Y-%m-%dT%H:%M:%S"),
        }
        with open(path, "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
        return path

    def load(self, name: str = "default") -> bool:
        path = os.path.join(self.session_dir, f"{name}.json")
        if not os.path.isfile(path):
            return False
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
        self.current_directory_id = data.get("directoryId")
        self.current_directory_path = data.get("directoryPath")
        self.current_path = data.get("currentPath")
        return True

    def to_dict(self) -> dict:
        return {
            "directoryId": self.current_directory_id,
            "directoryPath": self.current_directory_path,
            "currentPath": self.current_path,
            "canUndo": self.can_undo,
            "canRedo": self.can_redo,
            "undoCount": len(self._undo_stack),
            "redoCount": len(self._redo_stack),
        }
