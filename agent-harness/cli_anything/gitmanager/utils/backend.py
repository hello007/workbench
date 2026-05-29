"""Git Manager 后端调用 — 直接调用 git CLI 和读写配置文件。"""

import os
import shutil
import subprocess


def find_git() -> str:
    """查找 git 可执行文件。"""
    path = shutil.which("git")
    if not path:
        raise RuntimeError(
            "git 未安装。请安装 Git: https://git-scm.com/downloads"
        )
    return path


def find_code_editor(editor: str = "code") -> str:
    """查找编辑器可执行文件。"""
    path = shutil.which(editor)
    if not path:
        raise RuntimeError(
            f"{editor} 未找到。请确认已安装并添加到 PATH。"
        )
    return path


def open_in_explorer(path: str) -> None:
    """在系统文件管理器中打开。"""
    abs_path = os.path.abspath(path)
    if os.path.isfile(abs_path):
        subprocess.run(["explorer", "/select,", abs_path])
    elif os.path.isdir(abs_path):
        subprocess.run(["explorer", abs_path])
    else:
        raise FileNotFoundError(f"路径不存在: {abs_path}")


def open_in_vscode(path: str) -> None:
    """用 VSCode 打开。"""
    editor = find_code_editor("code")
    subprocess.run([editor, os.path.abspath(path)])


def open_with_default(path: str) -> None:
    """用系统默认程序打开文件。"""
    abs_path = os.path.abspath(path)
    if os.path.isdir(abs_path):
        raise ValueError("不支持打开文件夹")
    if not os.path.isfile(abs_path):
        raise FileNotFoundError(f"文件不存在: {abs_path}")
    subprocess.run(["cmd", "/c", "start", "", abs_path])
