# web/index.html dagi MAPS dan mapsdata.go ni yasaydi. Repo ildizidan ishga tushiring: python3 tools/genmaps.py && gofmt -w mapsdata.go
import re, json

s=open('web/index.html',encoding='utf-8').read()
blk=s[s.index('const MAPS = {'):s.index('const MAP_CH')]
out=['// Code generated from web/index.html (MAPS) — tools/genmaps.py; DO NOT EDIT (TestMapsMatchClient tekshiradi).','','package main','','var mapDefs = []mapDef{']
for m in re.finditer(r"\n  (\w+):\{ name:(\"[^\"]*\")", blk):
    mid=m.group(1); seg=blk[m.end():]
    nxt=re.search(r"\n  \w+:\{ name:", seg); seg=seg[:nxt.start()] if nxt else seg
    holds=json.loads(re.search(r"holds:(\{.*?\}),\n", seg).group(1))
    plan=json.loads(re.search(r"plan:(\{.*?\}),\n", seg).group(1))
    rows=re.findall(r"'([#.cskAtB]{44})'", seg); assert len(rows)==44, (mid,len(rows))
    g=lambda L: '{'+', '.join('{%d, %d}'%tuple(p) for p in L)+'}'
    hl=[h for k in 'ABM' for h in holds[k]]
    out.append('\t{id: "%s", name: %s,'%(mid, m.group(2)))
    out.append('\t\tholds: [][2][2]int{'+', '.join('{{%d, %d}, {%d, %d}}'%(a[0],a[1],b[0],b[1]) for a,b in hl)+'},')
    out.append('\t\tspots: map[string][][2]int{"A": %s, "B": %s},'%(g(plan['A']['spots']), g(plan['B']['spots'])))
    out.append('\t\tlooks: map[string][][2]int{"A": %s, "B": %s},'%(g(plan['A']['looks']), g(plan['B']['looks'])))
    out.append('\t\trows: []string{'); out += ['\t\t\t"%s",'%r for r in rows]; out.append('\t\t}},')
out.append('}')
open('mapsdata.go','w').write('\n'.join(out)+'\n')
print('mapsdata.go ok')
