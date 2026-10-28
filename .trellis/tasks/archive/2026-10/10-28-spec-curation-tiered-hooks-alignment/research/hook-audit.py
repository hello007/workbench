# -*- coding: utf-8 -*-
"""关键规则表分档口径审计 + 钩子导流静态抽查。

1. 逐行统计整行字符数与钩子数（反引号内标识符），按两档口径（普通行 <=250 /
   钩子密集行 >=7 钩子与合并行 <=350）判定合规性。
2. 取钩子最多的 5 行，提取特征标识符 grep docs/spec/*.md，验证每个钩子可定位承接章节。
"""
import re
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[4]
CLAUDE = ROOT / "CLAUDE.md"
SPEC = ROOT / "docs" / "spec"

BACKTICK = re.compile(r"`([^`]+)`")
# 纯散文/过短/链接片段不算特征标识符
SKIP_TOKENS = {"|", "---", "docs/spec", "CLAUDE.md", "npm test", "go test", "wails dev", "wails build"}


def rows():
    in_section = False
    for line in CLAUDE.read_text(encoding="utf-8").splitlines():
        s = line.strip()
        if s.startswith("### "):
            in_section = s == "### 关键规则"
            continue
        if s.startswith("## "):
            in_section = False
            continue
        if not in_section or not s.startswith("|"):
            continue
        if s.startswith("|规则") or s.startswith("|--"):
            continue
        yield s


def token_set(text):
    return {t for t in BACKTICK.findall(text) if t not in SKIP_TOKENS and len(t) >= 2}


rows = list(rows())
print(f"数据行数: {len(rows)}")

# ---- 1. 分档合规 ----
print("\n== 分档口径审计（普通 <=250 / 钩子密集>=7 或合并行 <=350）==")
over = []
tier_counts = {"normal_le250": 0, "dense_251_350": 0, "over_350": 0}
for i, r in enumerate(rows, 1):
    n_hooks = len(token_set(r))
    length = len(r)
    if length <= 250:
        tier_counts["normal_le250"] += 1
    elif length <= 350:
        tier_counts["dense_251_350"] += 1
    else:
        tier_counts["over_350"] += 1
        over.append((i, length, n_hooks, r[:60]))
print(f"普通行(<=250): {tier_counts['normal_le250']}  密集/合并(251-350): {tier_counts['dense_251_350']}  超硬顶350: {tier_counts['over_350']}")
for i, ln, nh, head in over:
    print(f"  超350 行{i} len={ln} hooks={nh}: {head}...")

# ---- 2. 钩子最多的 5 行导流抽查 ----
ranked = sorted(((len(token_set(r)), i, r) for i, r in enumerate(rows, 1)), reverse=True)
print("\n== 钩子导流静态抽查（钩子最多 5 行）==")
spec_files = list(SPEC.glob("*.md"))
spec_texts = {f.name: f.read_text(encoding="utf-8") for f in spec_files}


def variants(tok):
    """精确 token 未命中时的弱命中变体：去花括号、取 basename、去路径段。"""
    v = {tok}
    v.add(tok.replace("{", "").replace("}", "").replace(" ", ""))
    base = tok.rsplit("/", 1)[-1]
    if base != tok:
        v.add(base)
    return [x for x in v if len(x) >= 2]


all_fail = 0
for n_hooks, i, r in ranked[:5]:
    toks = sorted(token_set(r), key=lambda t: -len(t))
    miss = []
    for t in toks:
        if not any(any(v in txt for v in variants(t)) for txt in spec_texts.values()):
            miss.append(t)
    link = re.search(r"\]\(([^)]+)\)", r)
    link_target = link.group(1) if link else "(无)"
    status = "OK" if not miss else f"MISS {miss}"
    all_fail += len(miss)
    print(f"行{i} hooks={n_hooks} tokens={len(toks)} 承接={link_target} -> {status}")
    if miss:
        for t in miss:
            print(f"    未命中: {t}")

print(f"\n结论: {'全部钩子可定位' if all_fail == 0 else f'{all_fail} 个钩子未命中，须核对'}")
