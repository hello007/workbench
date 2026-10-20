#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Session Start Hook - 注入项目记忆索引（docs/memory/MEMORY.md）

独立于 session-start.py：只负责项目记忆的自动加载。
- docs/memory/MEMORY.md 存在：注入索引内容 + 权威源声明
- 不存在：静默降级（exit 0，无输出），不阻塞会话启动
"""
from __future__ import annotations

import json
import sys
from io import StringIO
from pathlib import Path

# Windows 默认 codepage（cp936 等）下中文内容会触发
# UnicodeDecodeError / UnicodeEncodeError，强制三流 UTF-8，
# 做法与 session-start.py 对齐。
for _stream in (sys.stdin, sys.stdout, sys.stderr):
    try:
        _stream.reconfigure(encoding="utf-8")  # type: ignore[union-attr]
    except AttributeError:
        pass


def _repo_root() -> Path:
    """以脚本自身位置为锚解析仓库根（不依赖 hook 运行时 cwd）。"""
    return Path(__file__).resolve().parent.parent.parent


def build_memory_context(memory_index: Path) -> str | None:
    """读取记忆索引并组装注入文本；索引缺失或为空返回 None。"""
    if not memory_index.is_file():
        return None
    try:
        index_text = memory_index.read_text(encoding="utf-8").strip()
    except OSError:
        return None
    if not index_text:
        return None

    output = StringIO()
    output.write("<project-memory>\n")
    output.write("以下是项目长期记忆索引（docs/memory/MEMORY.md），已自动加载：\n\n")
    output.write(index_text)
    output.write("\n\n")
    output.write("规则：\n")
    output.write(
        "1. `docs/memory/` 是项目记忆唯一权威源；每条记忆一个 markdown 文件"
        "（frontmatter 含 name/description/metadata.type）+ 索引行登记在"
        " docs/memory/MEMORY.md。\n"
    )
    output.write(
        "2. 需要某条记忆详情时按索引中的相对路径读取正文文件，"
        "索引未命中时才考虑新建；新记忆写入 docs/memory/ 并同步更新索引。\n"
    )
    output.write(
        "3. 若发现 user 级自动记忆目录（~/.claude/projects/<工程>/memory/）"
        "出现本索引未收录的新条目，先人工合并回 docs/memory/ 再消费。\n"
    )
    output.write("</project-memory>")
    return output.getvalue()


def main() -> int:
    memory_index = _repo_root() / "docs" / "memory" / "MEMORY.md"
    context_text = build_memory_context(memory_index)
    if context_text is None:
        return 0

    result = {
        # Claude Code / Qoder / CodeBuddy / Droid / Gemini / Copilot 格式
        "hookSpecificOutput": {
            "hookEventName": "SessionStart",
            "additionalContext": context_text,
        },
        # Cursor sessionStart 格式（顶层 snake_case）
        "additional_context": context_text,
    }
    print(json.dumps(result, ensure_ascii=False), flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
