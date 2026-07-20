#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""扫描 repo_meta.json 里 workspace_ai/github 下所有条目，提取 README/package.json/go.mod 供推断标签。"""
import json, os, sys

META = r'D:/Program Files/WorkBench/data/repo_meta.json'
with open(META, encoding='utf-8') as f:
    d = json.load(f)
repos = d.get('repos', {})
gh = {p: m for p, m in repos.items() if 'workspace_ai' in p and 'github' in p.lower()}

def short(p):
    return p.split('github\\')[-1] if 'github\\' in p else p.split('github/')[-1]

out = []
for p, m in gh.items():
    readme = ''
    for rf in ['README.md', 'README.MD', 'readme.md', 'README.rst', 'README.txt', 'README']:
        fp = os.path.join(p, rf)
        if os.path.isfile(fp):
            try:
                readme = open(fp, encoding='utf-8', errors='ignore').read(600)
            except Exception:
                pass
            break
    pkg = ''
    pp = os.path.join(p, 'package.json')
    if os.path.isfile(pp):
        try:
            pj = json.load(open(pp, encoding='utf-8', errors='ignore'))
            pkg = (pj.get('name', '') + ' | ' + (pj.get('description') or ''))[:180]
            kw = pj.get('keywords')
            if kw:
                pkg += ' | kw=' + ','.join(kw[:8])
        except Exception:
            pass
    gomod = ''
    gp = os.path.join(p, 'go.mod')
    if os.path.isfile(gp):
        try:
            gomod = open(gp, encoding='utf-8', errors='ignore').readline().strip()
        except Exception:
            pass
    exists = os.path.isdir(p)
    out.append((short(p), p, exists, m.get('tags'), pkg, gomod, readme[:280].replace('\n', ' ').strip()))

lines = []
for name, p, exists, tags, pkg, gomod, readme in out:
    lines.append('=== ' + name + ' ===')
    lines.append('PATH: ' + p)
    lines.append('EXISTS: ' + str(exists) + ' | CURTAGS: ' + str(tags))
    if pkg:
        lines.append('PKG: ' + pkg)
    if gomod:
        lines.append('GO: ' + gomod)
    lines.append('README: ' + readme)
    lines.append('')
lines.append('TOTAL: ' + str(len(out)))
outpath = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'scan_output.txt')
with open(outpath, 'w', encoding='utf-8') as f:
    f.write('\n'.join(lines))
sys.stderr.write('written ' + str(len(out)) + ' repos to ' + outpath + '\n')
