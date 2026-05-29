"""Git 操作 — 通过 git CLI 执行，与 git-manager Go 后端等价。"""

import os
import re
import subprocess
from typing import Optional


def _run_git(path: str, *args: str, timeout: int = 30) -> str:
    """在指定目录执行 git 命令。"""
    env = {**os.environ, "GIT_ENCODING": "utf-8"}
    result = subprocess.run(
        ["git"] + list(args),
        cwd=path,
        capture_output=True,
        timeout=timeout,
        env=env,
    )
    stdout = result.stdout.decode("utf-8", errors="replace").strip()
    stderr = result.stderr.decode("utf-8", errors="replace").strip()
    if result.returncode != 0:
        raise RuntimeError(f"git {args[0]} failed: {stderr}")
    return stdout


def is_git_repo(path: str) -> bool:
    try:
        _run_git(path, "rev-parse", "--git-dir")
        return True
    except (RuntimeError, subprocess.TimeoutExpired):
        return False


def get_info(path: str) -> dict:
    """获取仓库信息，与 git-manager GetGitInfo 对应。"""
    info = {"path": os.path.abspath(path), "isRepo": is_git_repo(path)}
    if not info["isRepo"]:
        return info

    try:
        info["branch"] = _run_git(path, "branch", "--show-current")
    except RuntimeError:
        info["branch"] = ""

    try:
        output = _run_git(path, "remote", "-v")
        lines = output.split("\n")
        if lines and lines[0]:
            parts = lines[0].split()
            info["remote"] = parts[0] if parts else ""
            url = parts[1] if len(parts) > 1 else ""
            info["remoteUrl"] = re.sub(r"\s*\(fetch\)\s*$", "", url)
    except RuntimeError:
        info["remote"] = ""
        info["remoteUrl"] = ""

    return info


def clone(url: str, target_path: str) -> str:
    abs_path = os.path.abspath(target_path)
    if os.path.isdir(abs_path):
        raise FileExistsError(f"目标路径已存在: {abs_path}")
    return _run_git(os.path.dirname(abs_path) or ".", "clone", url, abs_path, timeout=300)


def pull(path: str) -> str:
    if not is_git_repo(path):
        raise ValueError("不是 Git 仓库")
    return _run_git(path, "pull", timeout=120)


def get_branches(path: str) -> list[dict]:
    """获取分支列表，与 git-manager GetBranches 对应。"""
    if not is_git_repo(path):
        raise ValueError("不是 Git 仓库")

    output = _run_git(path, "branch", "-a")
    branches = []
    current_name = None

    for line in output.split("\n"):
        line = line.strip()
        if not line:
            continue

        is_current = line.startswith("* ")
        if is_current:
            line = line[2:].strip()
        else:
            line = line.lstrip()

        if "HEAD ->" in line or "(HEAD detached" in line:
            continue

        if line.startswith("remotes/"):
            name = line[len("remotes/"):]
            branches.append({"name": name, "isRemote": True, "isCurrent": is_current})
        else:
            branches.append({"name": line, "isRemote": False, "isCurrent": is_current})
            if is_current:
                current_name = line

    return branches


def checkout(path: str, branch: str, is_remote: bool = False) -> str:
    """切换分支。"""
    if not is_git_repo(path):
        raise ValueError("不是 Git 仓库")

    if is_remote:
        local_name = branch.split("/")[-1] if "/" in branch else branch
        return _run_git(path, "checkout", "-b", local_name, branch)
    return _run_git(path, "checkout", branch)


def get_log(path: str, limit: int = 20, offset: int = 0) -> list[dict]:
    """获取提交历史，与 git-manager GetCommitHistory 对应。"""
    if not is_git_repo(path):
        raise ValueError("不是 Git 仓库")

    skip = offset
    format_str = "%H%n%h%n%an%n%ae%n%at%n%s%n---END---"
    output = _run_git(path, "log", f"--max-count={limit}", f"--skip={skip}",
                      f"--format={format_str}")

    commits = []
    for block in output.split("---END---"):
        lines = block.strip().split("\n")
        if len(lines) < 6:
            continue
        commits.append({
            "sha": lines[0].strip(),
            "shortSHA": lines[1].strip(),
            "author": lines[2].strip(),
            "email": lines[3].strip(),
            "timestamp": int(lines[4].strip()),
            "message": lines[5].strip(),
        })
    return commits


def get_status(path: str) -> list[dict]:
    """获取本地变更文件列表。"""
    if not is_git_repo(path):
        raise ValueError("不是 Git 仓库")

    output = _run_git(path, "status", "--porcelain")
    changes = []
    for line in output.split("\n"):
        if not line.strip():
            continue
        status = line[:2].strip()
        filepath = line[3:].strip()
        # 处理 rename 格式: "R  old -> new"
        if " -> " in filepath:
            filepath = filepath.split(" -> ")[1]
        changes.append({
            "status": status,
            "path": filepath,
            "staged": status[0] not in (" ", "?"),
            "type": _status_type(status),
        })
    return changes


def discard_changes(path: str, filepaths: Optional[list[str]] = None) -> str:
    """回滚本地变更。"""
    if not is_git_repo(path):
        raise ValueError("不是 Git 仓库")

    if filepaths:
        _run_git(path, "checkout", "--", *filepaths)
        # 同时清理未跟踪文件
        _run_git(path, "clean", "-f", "--", *filepaths)
    else:
        _run_git(path, "checkout", "--", ".")
        _run_git(path, "clean", "-fd")
    return "变更已回滚"


def scan_git_repos(dir_path: str) -> list[str]:
    """扫描目录下的所有 Git 仓库。"""
    repos = []
    for root, dirs, _files in os.walk(dir_path):
        if ".git" in dirs:
            repos.append(root)
            dirs.remove(".git")
    return repos


def _status_type(status: str) -> str:
    s = status.strip()
    if s in ("M", "MM", "AM"):
        return "modified"
    if s in ("A", "AM"):
        return "added"
    if s in ("D", "AD"):
        return "deleted"
    if s == "R":
        return "renamed"
    if s == "??" or s == "?":
        return "untracked"
    return "unknown"
