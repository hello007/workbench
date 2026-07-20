#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""为 repo_meta.json 的 github 仓库批量写入分类标签。
默认 dry-run：生成 tags_preview.md，不修改 meta。
--apply：备份后写入 meta（执行前必须关闭 WorkBench，避免内存旧数据覆盖）。
策略：预设 13 类中文 + 术语/库名英文原样；统一覆盖现有 tags。"""
import json, os, sys, shutil
from datetime import datetime, timezone, timedelta

META = r'D:/Program Files/WorkBench/data/repo_meta.json'
HERE = os.path.dirname(os.path.abspath(__file__))
TZ = timezone(timedelta(hours=8))

# path -> tags（预设类目中文 + 术语/库名英文）
TAGS = {
    r'D:\workspace\workspace_ai\github\AI框架\spring-ai': ['AI框架', 'spring-ai'],
    r'D:\workspace\workspace_ai\github\HarnessEngineering\harness-engineering': ['Claude生态', 'harness'],
    r'D:\workspace\workspace_ai\github\MCP\Unla': ['MCP', 'gateway', 'unla'],
    r'D:\workspace\workspace_ai\github\MCP\codebase-memory-mcp': ['MCP', 'codebase-memory'],
    r'D:\workspace\workspace_ai\github\MCP\mcp-gateway-java': ['MCP', 'gateway'],
    r'D:\workspace\workspace_ai\github\PPT\demo_ppt\.claude\skills\frontend-slides': ['PPT演示', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\PPT\demo_ppt\.claude\skills\guizang-ppt-skill': ['PPT演示', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\PPT\frontend-slides': ['PPT演示', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\PPT\guizang-ppt-skill': ['PPT演示', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\PPT\html-anything': ['PPT演示', '前端UI'],
    r'D:\workspace\workspace_ai\github\PPT\ppt-master': ['PPT演示'],
    r'D:\workspace\workspace_ai\github\SDD\BMAD-METHOD': ['工作流编排', 'bmad'],
    r'D:\workspace\workspace_ai\github\SDD\OpenSpec': ['工作流编排', 'OpenSpec'],
    r'D:\workspace\workspace_ai\github\SDD\OpenSpec-Docs-zh': ['文档翻译', 'OpenSpec'],
    r'D:\workspace\workspace_ai\github\SDD\Trellis': ['工作流编排', 'Trellis'],
    r'D:\workspace\workspace_ai\github\SDD\flow-kit': ['工作流编排'],
    r'D:\workspace\workspace_ai\github\UI\awesome-design-md': ['前端UI'],
    r'D:\workspace\workspace_ai\github\UI\taste-skill': ['前端UI', 'Skill技能包'],
    r'D:\workspace\workspace_ai\github\agency-agents': ['Agent', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\agent\aiagentdemo': ['Agent', 'AI框架', 'spring-ai'],
    r'D:\workspace\workspace_ai\github\awesome\awesome-llm-apps': ['Agent'],
    r'D:\workspace\workspace_ai\github\awesome\chinese-independent-developer': ['其他'],
    r'D:\workspace\workspace_ai\github\claude\ai-coding-guide': ['Claude生态', '文档翻译'],
    r'D:\workspace\workspace_ai\github\claude\claude-howto-zh-cn': ['Claude生态', '文档翻译'],
    r'D:\workspace\workspace_ai\github\claude\claude-hud': ['Claude生态'],
    r'D:\workspace\workspace_ai\github\douyin-downloader': ['开发工具'],
    r'D:\workspace\workspace_ai\github\enable_openclaw_feishu_lark': ['开发工具', 'feishu'],
    r'D:\workspace\workspace_ai\github\everything-claude-code': ['Claude生态', 'harness'],
    r'D:\workspace\workspace_ai\github\flow-流程化\openflow': ['工作流编排', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\flow-流程化\superpowers-openspec-team-skills': ['工作流编排', 'Skill技能包'],
    r'D:\workspace\workspace_ai\github\graph-知识图谱\Understand-Anything': ['知识图谱'],
    r'D:\workspace\workspace_ai\github\gstack': ['Skill技能包', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\hello007.github.io': ['其他'],
    r'D:\workspace\workspace_ai\github\loop工程\loopany-platform': ['工作流编排', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\prompts\claude-code-system-prompts': ['Claude生态'],
    r'D:\workspace\workspace_ai\github\skills': ['Skill技能包', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\stock\daily_stock_analysis': ['股票量化'],
    r'D:\workspace\workspace_ai\github\stock\stock-analysis': ['股票量化'],
    r'D:\workspace\workspace_ai\github\zread\zread-docs-cli': ['开发工具', 'zread'],
    r'D:\workspace\workspace_ai\github\中文翻译仓库\agency-agents-zh': ['Agent', '文档翻译'],
    r'D:\workspace\workspace_ai\github\中文翻译仓库\agency-orchestrator': ['Agent', '工作流编排'],
    r'D:\workspace\workspace_ai\github\中文翻译仓库\ai-coding-guide': ['文档翻译', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\中文翻译仓库\library': ['文档翻译'],
    r'D:\workspace\workspace_ai\github\中文翻译仓库\superpowers-zh': ['Skill技能包', '文档翻译'],
    r'D:\workspace\workspace_ai\github\工具类\省token\caveman': ['开发工具', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\工具类\省token\rtk': ['开发工具'],
    r'D:\workspace\workspace_ai\github\常用Skills\CLI-Anything': ['Skill技能包', '开发工具'],
    r'D:\workspace\workspace_ai\github\常用Skills\Enterprise-ai-scenario-map-skill': ['Skill技能包'],
    r'D:\workspace\workspace_ai\github\常用Skills\andrej-karpathy-skills': ['Skill技能包', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\常用Skills\awesome-design-md': ['前端UI', 'Skill技能包'],
    r'D:\workspace\workspace_ai\github\常用Skills\claude-skills': ['Skill技能包', 'Claude生态'],
    r'D:\workspace\workspace_ai\github\常用Skills\drawio-skills': ['Skill技能包'],
    r'D:\workspace\workspace_ai\github\常用Skills\khazix-skills': ['Skill技能包'],
    r'D:\workspace\workspace_ai\github\常用Skills\legion-mind': ['Agent', '工作流编排'],
    r'D:\workspace\workspace_ai\github\常用Skills\ljg-skill-roundtable': ['Skill技能包'],
    r'D:\workspace\workspace_ai\github\常用Skills\planning-with-files': ['Skill技能包'],
    r'D:\workspace\workspace_ai\github\常用Skills\skills': ['Skill技能包'],
    r'D:\workspace\workspace_ai\github\常用Skills\superpowers': ['Skill技能包', '工作流编排'],
}


def short(p):
    return p.split('github\\')[-1] if 'github\\' in p else p


def main():
    apply = '--apply' in sys.argv
    with open(META, encoding='utf-8') as f:
        d = json.load(f)
    repos = d.get('repos', {})

    found = [(p, t) for p, t in TAGS.items() if p in repos]
    missing = [(p, t) for p, t in TAGS.items() if p not in repos]

    # 生成预览 markdown
    lines = ['# 仓库标签方案预览（共 %d 个）' % len(TAGS), '',
             '> 策略：预设 13 类中文 + 术语/库名英文；统一覆盖现有 tags', '',
             '| # | 仓库 | 标签 | 现有（将被覆盖） |', '|---|---|---|---|']
    for i, (p, tags) in enumerate(TAGS.items(), 1):
        if p in repos:
            cur = repos[p].get('tags') or '—'
        else:
            cur = '【meta缺失】'
        lines.append('| %d | %s | %s | %s |' % (i, short(p), ' '.join(tags), cur))
    lines += ['', '**将更新 %d 个 | meta 缺失 %d 个**' % (len(found), len(missing))]
    if missing:
        lines.append('### meta 缺失路径（WorkBench 下次扫描才纳入，本次跳过）')
        for p, _ in missing:
            lines.append('- ' + short(p))
    preview_path = os.path.join(HERE, 'tags_preview.md')
    with open(preview_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))
    sys.stderr.write('preview -> %s\nfound=%d missing=%d apply=%s\n'
                     % (preview_path, len(found), len(missing), apply))

    if not apply:
        print('DRY-RUN ok. preview: ' + preview_path)
        print('确认无误 + 关闭 WorkBench 后执行: python apply_tags.py --apply')
        return

    # 真写入
    bak = META + '.bak-apply'
    shutil.copy2(META, bak)
    now = datetime.now(TZ).isoformat()
    for p, tags in found:
        repos[p]['tags'] = tags
        repos[p]['updatedAt'] = now
    with open(META, 'w', encoding='utf-8') as f:
        json.dump(d, f, ensure_ascii=False, indent=2)
    print('APPLIED: updated=%d missing_skipped=%d backup=%s' % (len(found), len(missing), bak))


if __name__ == '__main__':
    main()
