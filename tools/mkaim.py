"""Aim xaritalari (Maydon, Chorraha, Hovli): kichik va simmetrik — ikki tomon aynan bir xil sharoitda.
Faqat «Aim» rejimida o'ynaladi: bomba nuqtalari yo'q, raund faqat raqibni yo'q qilish bilan tugaydi.
'#' devor, '.' pol, 'c' yashik (1 m), 's' baland yashik (2,3 m), 't' hujum bazasi, 'k' himoya bazasi.
Ishga tushirish: python3 tools/mkaim.py <papka>  → <papka>/<xarita>.png va maps.json (MAPS uchun)."""
import json, sys
from collections import deque
from PIL import Image, ImageDraw

N = 44
def new(): return [['#'] * N for _ in range(N)]
def rect(g, x0, z0, x1, z1, ch='.'):
    for z in range(z0, z1 + 1):
        for x in range(x0, x1 + 1): g[z][x] = ch
def put(g, pts, ch):
    for x, z in pts: g[z][x] = ch
def mirror_x(pts, cx2):   # cx2 = 2*centre: a point (x, z) and its mirror (cx2 - x, z)
    return [p for (x, z) in pts for p in ((x, z), (cx2 - x, z))]

# Maydon: an open court; a broken wall of tall boxes down the middle, low crates in rows — long sight lines for the
# sniper, cover to peek for the rifles.
def maydon():
    g = new()
    rect(g, 8, 11, 35, 32)                                  # the court
    rect(g, 8, 18, 9, 25, 't'); rect(g, 34, 18, 35, 25, 'k')
    put(g, [(21, 13), (22, 13), (21, 17), (22, 17), (21, 21), (22, 22), (21, 26), (22, 26), (21, 30), (22, 30)], 's')   # broken centre wall
    put(g, mirror_x([(14, 14), (15, 14), (13, 19), (16, 22), (16, 23), (13, 27), (14, 30), (15, 30), (18, 18), (18, 25)], 43), 'c')
    put(g, mirror_x([(11, 12), (11, 31), (12, 21)], 43), 's')
    holds = {"A": [[[12, 14], [31, 14]], [[16, 18], [27, 25]]], "B": [[[31, 29], [12, 29]], [[27, 25], [16, 18]]], "M": [[[20, 21], [30, 21]], [[23, 22], [13, 22]]]}
    signs = [[8, 10, "S", "HUJUM", 2.4, 1.4], [33, 10, "S", "HIMOYA", 2.4, 1.4]]
    return g, holds, signs, []

# Chorraha: a plus-shaped crossroads — the long east–west street is the duel line, the north and south arms are flanks.
def chorraha():
    g = new()
    rect(g, 6, 18, 37, 25)                                  # east–west street
    rect(g, 18, 6, 25, 37)                                  # north–south street
    rect(g, 16, 16, 27, 27)                                 # the crossing
    rect(g, 6, 19, 7, 24, 't'); rect(g, 36, 19, 37, 24, 'k')
    put(g, [(21, 21), (22, 21)], 's'); put(g, [(21, 22), (22, 22)], 'c')                     # centre: two boxes and two crates
    put(g, mirror_x([(11, 19), (11, 24), (14, 21), (14, 22), (17, 17), (17, 26)], 43), 'c')
    put(g, [(19, 8), (24, 8), (21, 11), (22, 11), (19, 35), (24, 35), (21, 32), (22, 32)], 'c')   # the arms
    put(g, [(20, 14), (23, 29), (23, 14), (20, 29)], 's')
    holds = {"A": [[[19, 9], [21, 30]], [[12, 22], [31, 22]]], "B": [[[24, 34], [22, 13]], [[31, 21], [12, 21]]], "M": [[[19, 19], [24, 24]], [[24, 24], [19, 19]]]}
    signs = [[6, 17, "S", "HUJUM", 2.2, 1.3], [37, 17, "S", "HIMOYA", 2.2, 1.3]]
    return g, holds, signs, []

# Hovli: a square courtyard around a small building with four doors — close fights round the corners.
def hovli():
    g = new()
    rect(g, 9, 9, 34, 34)                                   # the yard
    rect(g, 18, 18, 25, 25, '#'); rect(g, 19, 19, 24, 24)   # the house (hollow)
    for x, z in [(21, 18), (22, 25), (18, 22), (25, 21)]: g[z][x] = '.'          # four doors, offset (no straight line through)
    rect(g, 9, 30, 13, 34, 't'); rect(g, 30, 9, 34, 13, 'k')                     # opposite corners
    put(g, [(21, 21), (22, 22)], 'c')
    put(g, [(13, 14), (14, 14), (29, 29), (30, 29), (16, 30), (27, 13), (11, 22), (32, 21), (21, 11), (22, 32)], 'c')
    put(g, [(15, 25), (28, 18), (24, 15), (19, 28)], 's')
    holds = {"A": [[[12, 12], [26, 26]], [[20, 20], [24, 24]]], "B": [[[31, 31], [17, 17]], [[23, 23], [19, 19]]], "M": [[[16, 21], [27, 22]], [[27, 22], [16, 21]]]}
    signs = [[19, 18, "N", "UY", 2, 1.2], [24, 25, "S", "UY", 2, 1.2]]
    return g, holds, signs, []

BOTOK = set('.tk')

def check(name, g, holds):
    rows = [''.join(r) for r in g]
    assert len(rows) == N and all(len(r) == N for r in rows)
    def bfs(start):
        seen = {start}; q = deque([start])
        while q:
            x, z = q.popleft()
            for dx, dz in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                nx, nz = x + dx, z + dz
                if 0 <= nx < N and 0 <= nz < N and (nx, nz) not in seen and g[nz][nx] in BOTOK:
                    seen.add((nx, nz)); q.append((nx, nz))
        return seen
    tsp = next((x, z) for z in range(N) for x in range(N) if g[z][x] == 't')
    reach = bfs(tsp)
    assert any(g[z][x] == 'k' for (x, z) in reach), (name, "himoya bazasiga yo'l yo'q")
    lost = [(x, z) for z in range(N) for x in range(N) if g[z][x] in BOTOK and (x, z) not in reach]
    assert not lost, (name, 'ajralib qolgan kataklar', lost[:10])
    for k in holds:
        for p, _ in holds[k]: assert g[p[1]][p[0]] in BOTOK, (name, 'band nuqta', p, g[p[1]][p[0]])
    for i in range(N): assert g[0][i] == g[N - 1][i] == g[i][0] == g[i][N - 1] == '#'
    return rows

def draw(rows, holds, out):
    S = 12; im = Image.new('RGB', (N * S, N * S)); d = ImageDraw.Draw(im)
    col = {'#': (60, 50, 40), '.': (214, 196, 160), 'c': (150, 105, 60), 's': (100, 70, 40), 't': (240, 200, 90), 'k': (120, 200, 230)}
    for z, r in enumerate(rows):
        for x, ch in enumerate(r): d.rectangle([x * S, z * S, x * S + S - 1, z * S + S - 1], fill=col[ch])
    for hs in holds.values():
        for (p, l) in hs:
            d.ellipse([p[0] * S + 2, p[1] * S + 2, p[0] * S + 10, p[1] * S + 10], fill=(20, 60, 200)); d.line([p[0] * S + 6, p[1] * S + 6, l[0] * S + 6, l[1] * S + 6], fill=(20, 60, 200))
    im.save(out)

if __name__ == '__main__':
    out = {}
    for name, fn in (('maydon', maydon), ('chorraha', chorraha), ('hovli', hovli)):
        g, holds, signs, cables = fn()
        rows = check(name, g, holds)
        draw(rows, holds, f'{sys.argv[1]}/{name}.png')
        out[name] = dict(rows=rows, holds=holds, signs=signs, cables=cables)
        print(name, 'ok')
    json.dump(out, open(f'{sys.argv[1]}/maps.json', 'w'))
