# -*- coding: utf-8 -*-
"""一次性编辑脚本（10-29-session-start-hook-paths 任务）v2：

session-start.py 死代码删除（prd Requirements 3-9 的删除部分）。
文件为混合行尾（CRLF/孤立 CR），splitlines 行号不可靠，改用锚文本定位：
按「strip 后前缀」匹配定位行，逐行断言后按区间删除。
未删除行的字节（含原行尾）完整保留。
"""
from pathlib import Path

TARGET = Path(
    r"D:\workspace\workspace_ai\demo_OpenSpec\git_tools\git-manager"
    r"\.claude\hooks\session-start.py"
)

data = TARGET.read_bytes().decode("utf-8")
lines = data.splitlines(keepends=True)


def find_idx(prefix: str, start: int = 0) -> int:
    """返回第一个 strip 后以 prefix 开头的行索引，找不到抛异常。"""
    for i in range(start, len(lines)):
        if lines[i].strip().startswith(prefix):
            return i
    raise AssertionError(f"anchor not found: {prefix!r} (from line {start + 1})")


def expect(idx: int, prefix: str) -> None:
    actual = lines[idx].strip()
    assert actual.startswith(prefix), f"line {idx + 1}: expected {prefix!r}, got {actual!r}"


# ---- 区间 1: R9 三函数（_load_trellis_config / _check_legacy_spec / _resolve_spec_scope）----
i_fn_start = find_idx("def _load_trellis_config(")
i_fn_end = find_idx("def _extract_range(", i_fn_start)
expect(i_fn_end, "def _extract_range(")
expect(i_fn_end - 1, "")  # 保留的两个空行之一
expect(i_fn_end - 2, "")

# ---- 区间 2: R9 main() 级联加载（注释 + _load_trellis_config 调用 + _resolve_spec_scope 调用 + 尾随空行）----
i_cascade = find_idx("# Load config for scope filtering and legacy detection")
expect(i_cascade + 1, "is_mono, packages, scope_config, task_pkg, default_pkg = _load_trellis_config(")
expect(i_cascade + 2, "trellis_dir,")
expect(i_cascade + 3, "hook_input,")
expect(i_cascade + 4, ")")
expect(i_cascade + 5, "allowed_pkgs = _resolve_spec_scope(")
expect(i_cascade + 6, "")

# ---- 区间 3: R8 legacy migration warning（注释 + 调用 + if + write + 尾随空行）----
i_legacy = find_idx("# Legacy migration warning")
expect(i_legacy + 1, "legacy_warning = _check_legacy_spec(")
expect(i_legacy + 2, "if legacy_warning:")
expect(i_legacy + 3, 'output.write(f"<migration-warning>')
expect(i_legacy + 4, "")

# ---- 区间 4: R3 guidelines 段头三行语境句 ----
i_head = find_idx('"Project spec indexes are listed by path below')
expect(i_head + 1, '"**Pre-Development Checklist** listing the specific guideline files to ')
expect(i_head + 2, '"read before coding.')
i_list_first = i_head + 3
expect(i_list_first, '"- If you\'re spawning an implement/check sub-agent')

# ---- 区间 5: R4-R7 guides 注入段 + spec 扫描块 + paths 输出 + Discover more 行 ----
i_guides = find_idx("# guides/ is cross-package thinking")
i_gl_close = find_idx('output.write("</guidelines>', i_guides)
expect(i_gl_close, 'output.write("</guidelines>')
expect(i_guides + 1, 'guides_index = trellis_dir / "spec" / "guides" / "index.md"')
expect(i_gl_close - 1, ")")  # Discover more 的 output.write( 闭合

# ---- 自底向上执行删除 ----
ranges = [
    (i_guides, i_gl_close),   # 区间 5
    (i_head, i_list_first),   # 区间 4
    (i_legacy, i_legacy + 5), # 区间 3
    (i_cascade, i_cascade + 7),  # 区间 2
    (i_fn_start, i_fn_end - 2),  # 区间 1（保留两空行）
]
removed = 0
for start, end in sorted(ranges, reverse=True):
    del lines[start:end]
    removed += end - start

TARGET.write_bytes("".join(lines).encode("utf-8"))
print(f"OK: removed {removed} lines, new total {len(lines)}")
