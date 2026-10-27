"""Draws the two new maps (Registon, Saroy) as 44x44 grids, checks them and prints the JS for MAPS.
'#' wall, '.' floor, 'c' crate (1 m), 's' stack (2.3 m), 'A'/'B' bomb sites, 't' attacker spawn, 'k' defender spawn."""
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

# ------------------------------------------------------------------ Registon
# A square in the middle (mid) framed by three madrasas: the west one has courtyard A, the east one a covered bazaar (B).
def registon():
    g = new()
    rect(g, 17, 3, 26, 6, 'k')                          # defenders: behind the north madrasa
    rect(g, 16, 37, 27, 40, 't')                        # attackers: south gate
    rect(g, 4, 33, 39, 36)                              # south street (attackers spread from here)
    # central square + its approaches
    rect(g, 15, 15, 28, 27)                             # Registon square
    rect(g, 19, 7, 24, 14)                              # north portal passage to the square
    rect(g, 18, 11, 18, 12, '#'); rect(g, 25, 11, 25, 12, '#')
    rect(g, 20, 28, 23, 32)                             # south gate passage
    rect(g, 21, 31, 22, 31, '#')                        # a pillar in the gate: two narrow doors
    # A: courtyard of the west madrasa
    rect(g, 3, 8, 12, 18, 'A')
    rect(g, 7, 4, 16, 6)                                # defenders' corridor to A
    rect(g, 7, 7, 9, 7)
    rect(g, 13, 17, 14, 19)                             # A portal from the square
    rect(g, 3, 19, 6, 32)                               # long west alley from the south street
    rect(g, 7, 24, 10, 26)                              # a side niche in the alley
    # B: covered bazaar (Chorsu) in the east madrasa
    rect(g, 31, 8, 40, 17, 'B')
    rect(g, 27, 4, 36, 6); rect(g, 34, 7, 36, 7)        # defenders' corridor to B
    rect(g, 29, 15, 30, 16)                             # bazaar door from the square
    rect(g, 29, 22, 33, 23); rect(g, 33, 18, 34, 22)    # second square exit up into the bazaar
    rect(g, 37, 18, 40, 22); rect(g, 36, 23, 40, 32)    # bazaar lanes down to the south street
    rect(g, 33, 26, 35, 27)
    # cover: fountain, stalls, columns
    put(g, [(21, 20), (22, 20), (21, 21), (22, 21)], 's')                                  # fountain
    put(g, [(16, 17), (27, 17), (17, 25), (26, 25), (19, 23), (24, 18)], 'c')              # stalls in the square
    put(g, [(5, 10), (10, 10), (5, 15), (10, 15)], 's')                                    # courtyard columns
    put(g, [(7, 12), (8, 13), (4, 17)], 'c')
    put(g, [(33, 10), (36, 10), (39, 12), (33, 15), (37, 14), (35, 13)], 'c')              # bazaar stalls
    put(g, [(31, 12), (40, 9)], 's')
    put(g, [(4, 27), (5, 22), (38, 28), (37, 20), (10, 34), (30, 35), (21, 9)], 'c')
    put(g, [(13, 33), (28, 34)], 's')
    holds = {"A": [[[6, 9], [13, 18]], [[11, 9], [5, 19]], [[4, 13], [13, 18]], [[9, 17], [5, 21]]],
             "B": [[[34, 9], [29, 16]], [[39, 11], [38, 21]], [[32, 14], [30, 15]], [[37, 16], [33, 22]]],
             "M": [[[21, 13], [21, 27]], [[23, 12], [22, 28]], [[20, 8], [21, 20]]]}
    plan = {"A": {"stages": [[5, 31], [20, 27], [5, 21]], "spots": [[6, 12], [9, 9], [11, 14], [4, 9]], "looks": [[13, 18], [8, 5], [5, 20]]},
            "B": {"stages": [[38, 31], [24, 24], [38, 23]], "spots": [[35, 11], [38, 9], [32, 9], [39, 15]], "looks": [[29, 15], [35, 5], [38, 19]]}}
    signs = [[14, 37, "N", "← A", 2.1], [29, 37, "N", "B →", 2.1], [19, 30, "E", "MAYDON", 2.2, 1.3], [5, 7, "S", "A", 3, 2.6], [32, 7, "S", "B", 3, 2.6],
             [14, 20, "E", "← A", 2, 1.3], [29, 24, "W", "B →", 2, 1.3], [2, 25, "E", "A ↑", 2.1], [35, 24, "E", "B ↑", 2.1]]
    cables = [[8, 41, 12, 41], [74, 47, 80, 47], [40, 63, 46, 63], [8, 55, 12, 55]]
    return g, holds, plan, signs, cables

# ------------------------------------------------------------------ Saroy
# A hill fortress: defenders hold the citadel (north), attackers come up from below the walls (south).
# Inner halls fill the middle; a long corridor runs along the east wall; a stepped ramp climbs the west side.
def saroy():
    g = new()
    rect(g, 17, 3, 26, 6, 'k')
    rect(g, 16, 37, 27, 40, 't')
    rect(g, 2, 33, 41, 36)                              # below the walls
    # A: gate courtyard (west), B: tower yard (east)
    rect(g, 4, 6, 13, 15, 'A')
    rect(g, 30, 6, 39, 14, 'B')
    rect(g, 14, 4, 16, 6); rect(g, 27, 4, 29, 6)        # citadel to both sites
    # inner halls: 3x3 rooms with doors
    for rz in (16, 21, 26):
        for rx in (15, 20, 25):
            rect(g, rx, rz, rx + 3, rz + 3)
    for rz in (16, 21, 26):                               # doors between rooms east-west
        g[rz + 1][19] = '.'; g[rz + 2][19] = '.'; g[rz + 1][24] = '.'; g[rz + 2][24] = '.'
    for rx in (15, 20, 25):                               # doors north-south
        g[20][rx + 1] = g[20][rx + 2] = '.'; g[25][rx + 1] = g[25][rx + 2] = '.'
    rect(g, 20, 7, 23, 15)                              # citadel stair down into the halls
    rect(g, 16, 30, 18, 32); rect(g, 26, 30, 27, 32)    # two hall exits to the slope below
    rect(g, 21, 30, 22, 32)
    # west: stepped ramp up to A
    rect(g, 2, 16, 6, 32)
    rect(g, 7, 16, 12, 18)                              # A lobby
    rect(g, 13, 17, 14, 18)                             # hall door into the A lobby
    # east: long corridor under the wall up to B
    rect(g, 40, 8, 41, 32)
    rect(g, 30, 15, 33, 18); rect(g, 34, 16, 39, 17)   # B lobby and its link to the corridor
    rect(g, 29, 17, 29, 18)                             # hall door into the B lobby
    rect(g, 38, 15, 39, 15)
    # cover
    put(g, [(3, 20), (5, 23), (2, 27), (5, 30)], 'c')                                       # steps on the ramp
    put(g, [(6, 8), (11, 8), (6, 13), (11, 13)], 's')                                       # gate towers
    put(g, [(8, 10), (9, 11), (12, 15)], 'c')
    put(g, [(32, 8), (37, 8), (34, 11), (38, 13)], 's')                                     # tower yard: tower bases
    put(g, [(31, 12), (36, 10), (39, 7)], 'c')
    put(g, [(16, 17), (22, 23), (27, 28), (17, 27), (26, 18)], 'c')                         # rubble in the halls
    put(g, [(40, 20), (41, 27), (8, 34), (34, 35), (12, 33)], 'c')
    put(g, [(21, 9), (22, 12)], 'c')
    holds = {"A": [[[7, 7], [13, 17]], [[12, 7], [4, 16]], [[5, 11], [12, 17]], [[10, 14], [4, 18]]],
             "B": [[[33, 7], [30, 17]], [[38, 9], [40, 20]], [[31, 10], [32, 17]], [[36, 13], [39, 16]]],
             "M": [[[21, 14], [21, 28]], [[22, 9], [22, 24]], [[23, 13], [20, 22]]]}
    plan = {"A": {"stages": [[4, 31], [17, 24], [4, 18]], "spots": [[8, 9], [5, 7], [10, 12], [12, 10]], "looks": [[13, 17], [15, 5], [4, 16]]},
            "B": {"stages": [[40, 31], [27, 23], [40, 14]], "spots": [[35, 9], [31, 7], [37, 12], [33, 13]], "looks": [[30, 17], [28, 5], [40, 16]]}}
    signs = [[14, 37, "N", "← A", 2.1], [29, 37, "N", "B →", 2.1], [19, 31, "W", "ZALLAR", 2.2, 1.3], [8, 5, "S", "A", 3, 2.6], [35, 5, "S", "B", 3, 2.6],
             [1, 24, "E", "A ↑", 2.1], [42, 24, "W", "B ↑", 2.1], [14, 16, "S", "A", 2.4, 1.5], [29, 16, "S", "B", 2.4, 1.5]]
    cables = [[4, 41, 10, 41], [80, 37, 84, 37], [40, 61, 46, 61]]
    return g, holds, plan, signs, cables

WALK = set('.ckAtB')   # bots walk over crates? no: crates block bots, players can jump; treat c as blocked for bot paths
BOTOK = set('.AtBk')

def check(name, g, holds, plan):
    rows = [''.join(r) for r in g]
    assert len(rows) == N and all(len(r) == N for r in rows)
    def bfs(start, ok):
        seen = {start}; q = deque([start])
        while q:
            x, z = q.popleft()
            for dx, dz in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                nx, nz = x + dx, z + dz
                if 0 <= nx < N and 0 <= nz < N and (nx, nz) not in seen and g[nz][nx] in ok:
                    seen.add((nx, nz)); q.append((nx, nz))
        return seen
    tsp = next((x, z) for z in range(N) for x in range(N) if g[z][x] == 't')
    reach = bfs(tsp, BOTOK)
    for ch in 'ABk':
        cells = [(x, z) for z in range(N) for x in range(N) if g[z][x] == ch]
        assert any(c in reach for c in cells), (name, 'unreachable', ch)
    open_cells = [(x, z) for z in range(N) for x in range(N) if g[z][x] in BOTOK]
    lost = [c for c in open_cells if c not in reach]
    assert not lost, (name, 'isolated cells', lost[:10])
    pts = [p for k in holds for h in holds[k] for p in h[:1]] + [p for k in plan for key in ('stages', 'spots') for p in plan[k][key]]
    for x, z in pts: assert g[z][x] in BOTOK, (name, 'blocked point', (x, z), g[z][x])
    # the edge of the grid must stay wall
    for i in range(N): assert g[0][i] == g[N - 1][i] == g[i][0] == g[i][N - 1] == '#'
    return rows

def draw(name, rows, holds, plan, out):
    S = 12; im = Image.new('RGB', (N * S, N * S)); d = ImageDraw.Draw(im)
    col = {'#': (60, 50, 40), '.': (214, 196, 160), 'c': (150, 105, 60), 's': (100, 70, 40), 'A': (230, 140, 110), 'B': (110, 150, 230), 't': (240, 200, 90), 'k': (120, 200, 230)}
    for z, r in enumerate(rows):
        for x, ch in enumerate(r): d.rectangle([x * S, z * S, x * S + S - 1, z * S + S - 1], fill=col[ch])
    for k, hs in holds.items():
        for (p, l) in hs:
            d.ellipse([p[0] * S + 2, p[1] * S + 2, p[0] * S + 10, p[1] * S + 10], fill=(20, 60, 200)); d.line([p[0] * S + 6, p[1] * S + 6, l[0] * S + 6, l[1] * S + 6], fill=(20, 60, 200))
    for k in plan:
        for p in plan[k]['stages']: d.rectangle([p[0] * S + 3, p[1] * S + 3, p[0] * S + 9, p[1] * S + 9], fill=(200, 30, 30))
        for p in plan[k]['spots']: d.ellipse([p[0] * S + 3, p[1] * S + 3, p[0] * S + 9, p[1] * S + 9], fill=(250, 250, 250))
    im.save(out)

if __name__ == '__main__':
    out = {}
    for name, fn in (('registon', registon), ('saroy', saroy)):
        g, holds, plan, signs, cables = fn()
        rows = check(name, g, holds, plan)
        draw(name, rows, holds, plan, f'{sys.argv[1]}/{name}.png')
        out[name] = dict(rows=rows, holds=holds, plan=plan, signs=signs, cables=cables)
        print(name, 'ok'); print('\n'.join(rows))
    json.dump(out, open(f'{sys.argv[1]}/maps.json', 'w'))
