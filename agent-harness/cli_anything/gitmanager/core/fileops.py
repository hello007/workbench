"""文件操作 — 文件树浏览、文件 CRUD、预览。"""

import os
import shutil
from typing import Optional


def get_tree(path: str, max_depth: int = -1, current_depth: int = 0) -> list[dict]:
    """获取文件树，与 git-manager GetFileTreeRecursive 对应。"""
    abs_path = os.path.abspath(path)
    if not os.path.isdir(abs_path):
        raise NotADirectoryError(f"不是目录: {abs_path}")

    nodes = []
    try:
        entries = sorted(os.listdir(abs_path))
    except PermissionError:
        return nodes

    for name in entries:
        if name.startswith(".") and name != ".git":
            continue
        full = os.path.join(abs_path, name)
        is_dir = os.path.isdir(full)
        is_git = is_dir and os.path.isdir(os.path.join(full, ".git"))

        node = {
            "name": name,
            "path": full,
            "type": "directory" if is_dir else "file",
            "isGitRepo": is_git,
            "isLeaf": not is_dir,
        }

        if is_dir and (max_depth < 0 or current_depth < max_depth):
            node["children"] = get_tree(full, max_depth, current_depth + 1)

        nodes.append(node)
    return nodes


def preview_file(filepath: str, max_size: int = 1024 * 1024) -> dict:
    """预览文件内容，与 git-manager PreviewFile 对应。"""
    result = {"path": os.path.abspath(filepath), "name": os.path.basename(filepath)}

    if not os.path.isfile(filepath):
        result["error"] = "文件不存在"
        return result

    stat = os.stat(filepath)
    result["size"] = stat.st_size

    if stat.st_size > max_size:
        result["tooLarge"] = True
        return result

    try:
        with open(filepath, "rb") as f:
            data = f.read(max_size)
    except (PermissionError, OSError) as e:
        result["error"] = str(e)
        return result

    if b"\x00" in data[:1024]:
        result["isBinary"] = True
        return result

    result["content"] = data.decode("utf-8", errors="replace")
    return result


def create_file(parent: str, name: str, content: str = "") -> str:
    full = os.path.join(parent, name)
    if os.path.exists(full):
        raise FileExistsError(f"文件已存在: {full}")
    os.makedirs(parent, exist_ok=True)
    with open(full, "w", encoding="utf-8") as f:
        f.write(content)
    return full


def create_directory(parent: str, name: str) -> str:
    full = os.path.join(parent, name)
    if os.path.exists(full):
        raise FileExistsError(f"目录已存在: {full}")
    os.makedirs(full)
    return full


def rename(path: str, new_name: str) -> str:
    if not os.path.exists(path):
        raise FileNotFoundError(f"路径不存在: {path}")
    parent = os.path.dirname(path)
    new_path = os.path.join(parent, new_name)
    if os.path.exists(new_path):
        raise FileExistsError(f"目标已存在: {new_path}")
    os.rename(path, new_path)
    return new_path


def delete(path: str) -> None:
    if not os.path.exists(path):
        raise FileNotFoundError(f"路径不存在: {path}")
    if os.path.isdir(path):
        shutil.rmtree(path)
    else:
        os.remove(path)


def copy_item(source: str, target_dir: str) -> str:
    if not os.path.exists(source):
        raise FileNotFoundError(f"源路径不存在: {source}")
    name = os.path.basename(source)
    target = os.path.join(target_dir, name)
    target = _find_available_name(target)
    if os.path.isdir(source):
        shutil.copytree(source, target)
    else:
        os.makedirs(target_dir, exist_ok=True)
        shutil.copy2(source, target)
    return target


def move_item(source: str, target_dir: str) -> str:
    if not os.path.exists(source):
        raise FileNotFoundError(f"源路径不存在: {source}")
    name = os.path.basename(source)
    target = os.path.join(target_dir, name)
    target = _find_available_name(target)
    os.makedirs(target_dir, exist_ok=True)
    shutil.move(source, target)
    return target


def _find_available_name(target: str) -> str:
    if not os.path.exists(target):
        return target
    base, ext = os.path.splitext(target)
    n = 1
    while True:
        candidate = f"{base} ({n}){ext}"
        if not os.path.exists(candidate):
            return candidate
        n += 1
