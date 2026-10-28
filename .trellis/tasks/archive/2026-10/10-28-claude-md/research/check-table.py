import io, re
src = io.open('CLAUDE.md', encoding='utf-8').read()
m = re.search(r'### 关键规则\n(.*?)\n## 项目记忆', src, re.S)
rows = [l for l in m.group(1).split('\n') if l.startswith('|')]
pat = re.compile(r'(?<!\\)\|')
bad = []
for i, r in enumerate(rows):
    top = len(pat.findall(r)) - 1
    if top != 2:
        bad.append((i, top, r[:40]))
print(f'表格总行: {len(rows)}  列数异常: {bad if bad else "NONE"}')
