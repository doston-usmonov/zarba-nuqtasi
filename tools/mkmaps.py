"""Draws the newer maps (Bekat, Saroy, Zavod, Ko'shk) as 44x44 grids, checks them and prints the JS for MAPS.
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

# ------------------------------------------------------------------ Bekat
# An old railway station: the platform square in the middle (mid), the locomotive depot (A) to the west, the goods warehouse (B) to the east.
def bekat():
    g = new()
    rect(g, 17, 3, 26, 6, 'k')                          # defenders: behind the station building
    rect(g, 16, 37, 27, 40, 't')                        # attackers: south gate
    rect(g, 4, 33, 39, 36)                              # south street (attackers spread from here)
    # central square + its approaches
    rect(g, 15, 15, 28, 27)                             # platform square
    rect(g, 19, 7, 24, 14)                              # passage from the station building
    rect(g, 18, 11, 18, 12, '#'); rect(g, 25, 11, 25, 12, '#')
    rect(g, 20, 28, 23, 32)                             # south gate passage
    rect(g, 21, 31, 22, 31, '#')                        # a pillar in the gate: two narrow doors
    # A: the locomotive depot
    rect(g, 3, 8, 12, 18, 'A')
    rect(g, 7, 4, 16, 6)                                # defenders' corridor to A
    rect(g, 7, 7, 9, 7)
    rect(g, 13, 17, 14, 19)                             # A door from the platform
    rect(g, 3, 19, 6, 32)                               # long west alley from the south street
    rect(g, 7, 24, 10, 26)                              # a side niche in the alley
    # B: the goods warehouse
    rect(g, 31, 8, 40, 17, 'B')
    rect(g, 27, 4, 36, 6); rect(g, 34, 7, 36, 7)        # defenders' corridor to B
    rect(g, 29, 15, 30, 16)                             # warehouse door from the platform
    rect(g, 29, 22, 33, 23); rect(g, 33, 18, 34, 22)    # second platform exit up into the warehouse
    rect(g, 37, 18, 40, 22); rect(g, 36, 23, 40, 32)    # warehouse lanes down to the south street
    rect(g, 33, 26, 35, 27)
    # cover: water tower, cargo, columns
    put(g, [(21, 20), (22, 20), (21, 21), (22, 21)], 's')                                  # water tower
    put(g, [(16, 17), (27, 17), (17, 25), (26, 25), (19, 23), (24, 18)], 'c')              # cargo on the platform
    put(g, [(5, 10), (10, 10), (5, 15), (10, 15)], 's')                                    # depot columns
    put(g, [(7, 12), (8, 13), (4, 17)], 'c')
    put(g, [(33, 10), (36, 10), (39, 12), (33, 15), (37, 14), (35, 13)], 'c')              # warehouse crates
    put(g, [(31, 12), (40, 9)], 's')
    put(g, [(4, 27), (5, 22), (38, 28), (37, 20), (10, 34), (30, 35), (21, 9)], 'c')
    put(g, [(13, 33), (28, 34)], 's')
    holds = {"A": [[[6, 9], [13, 18]], [[11, 9], [5, 19]], [[4, 13], [13, 18]], [[9, 17], [5, 21]]],
             "B": [[[34, 9], [29, 16]], [[39, 11], [38, 21]], [[32, 14], [30, 15]], [[37, 16], [33, 22]]],
             "M": [[[21, 13], [21, 27]], [[23, 12], [22, 28]], [[20, 8], [21, 20]]]}
    plan = {"A": {"stages": [[5, 31], [20, 27], [5, 21]], "spots": [[6, 12], [9, 9], [11, 14], [4, 9]], "looks": [[13, 18], [8, 5], [5, 20]]},
            "B": {"stages": [[38, 31], [24, 24], [38, 23]], "spots": [[35, 11], [38, 9], [32, 9], [39, 15]], "looks": [[29, 15], [35, 5], [38, 19]]}}
    signs = [[14, 37, "N", "← A", 2.1], [29, 37, "N", "B →", 2.1], [19, 30, "E", "PERRON", 2.2, 1.3], [5, 7, "S", "A", 3, 2.6], [32, 7, "S", "B", 3, 2.6],
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

# ------------------------------------------------------------------ Zavod
# An old textile mill. Defenders hold the offices (north), attackers come in from the rail yard (south).
# A: the weaving hall (west) full of looms; B: the warehouse yard (east) with stacked containers.
# Mid: the long conveyor hall; flanks: the boiler room (west) and the repair shop (east).
def zavod():
    g = new()
    rect(g, 17, 3, 26, 6, 'k')                          # offices
    rect(g, 16, 37, 27, 40, 't')                        # rail yard gate
    rect(g, 2, 33, 41, 36)                              # rail yard
    # A: weaving hall
    rect(g, 3, 7, 14, 17, 'A')
    rect(g, 9, 4, 16, 6); rect(g, 9, 7, 10, 7)          # offices -> A (west corridor)
    rect(g, 3, 18, 5, 32)                               # long pipe corridor from the yard up to A
    rect(g, 6, 18, 7, 19)
    # B: warehouse yard
    rect(g, 30, 7, 40, 16, 'B')
    rect(g, 27, 4, 34, 6); rect(g, 33, 7, 34, 7)        # offices -> B
    rect(g, 38, 17, 40, 32)                             # loading ramp from the yard up to B
    # mid: conveyor hall and its doors
    rect(g, 18, 12, 25, 28)
    rect(g, 20, 7, 23, 11)                              # office stairs down into the hall
    rect(g, 15, 13, 17, 14)                             # hall -> A side door
    rect(g, 26, 14, 29, 15)                             # hall -> B side door
    rect(g, 20, 29, 23, 32)                             # hall -> yard
    # flanks
    rect(g, 8, 22, 13, 28)                              # boiler room
    rect(g, 6, 24, 7, 25); rect(g, 14, 24, 17, 25)      # pipe corridor <-> boiler room <-> hall
    rect(g, 10, 29, 11, 32)                             # boiler room -> yard
    rect(g, 30, 21, 35, 27)                             # repair shop
    rect(g, 36, 23, 37, 24); rect(g, 26, 23, 29, 24)    # ramp <-> shop <-> hall
    # cover: looms in the hall, conveyors in the middle, containers on B, wagons in the yard
    put(g, [(5, 9), (8, 9), (11, 9), (5, 13), (8, 13), (11, 13)], 's')                       # looms
    put(g, [(6, 11), (10, 11), (13, 15), (4, 16)], 'c')
    put(g, [(21, 15), (22, 15), (21, 19), (22, 20), (21, 24), (22, 24)], 'c')                # conveyor belts
    put(g, [(19, 13), (24, 27)], 's')
    put(g, [(32, 9), (33, 9), (37, 9), (38, 9), (32, 13), (36, 13), (37, 13)], 's')          # containers
    put(g, [(35, 11), (40, 11), (31, 15)], 'c')
    put(g, [(9, 24), (12, 26), (11, 23)], 's')                                               # boilers
    put(g, [(32, 23), (34, 25), (31, 26)], 'c')                                              # workbenches
    put(g, [(4, 21), (5, 27), (39, 20), (38, 28), (40, 25)], 'c')
    put(g, [(7, 34), (8, 34), (14, 35), (29, 34), (30, 34), (35, 35)], 's')                  # wagons
    put(g, [(18, 34), (25, 35), (3, 35), (40, 34)], 'c')
    holds = {"A": [[[7, 8], [15, 13]], [[12, 16], [4, 19]], [[4, 11], [14, 14]], [[13, 8], [9, 7]]],
             "B": [[[34, 8], [27, 14]], [[39, 15], [39, 19]], [[31, 11], [28, 15]], [[38, 12], [36, 17]]],
             "M": [[[21, 12], [21, 28]], [[23, 9], [22, 24]], [[18, 26], [15, 24]]]}
    plan = {"A": {"stages": [[4, 31], [19, 27], [7, 19]], "spots": [[7, 10], [9, 15], [12, 11], [4, 8]], "looks": [[15, 13], [10, 5], [4, 19]]},
            "B": {"stages": [[39, 31], [24, 26], [36, 24]], "spots": [[35, 10], [39, 13], [31, 14], [34, 15]], "looks": [[27, 14], [33, 5], [39, 18]]}}
    signs = [[14, 37, "N", "← A", 2.1], [29, 37, "N", "B →", 2.1], [21, 31, "W", "SEX", 2.2, 1.3], [6, 6, "S", "A", 3, 2.6], [36, 6, "S", "B", 3, 2.6],
             [2, 24, "E", "A ↑", 2.1], [41, 24, "W", "B ↑", 2.1], [10, 21, "S", "QOZONXONA", 2.4, 1.5], [33, 20, "S", "USTAXONA", 2.4, 1.5]]
    cables = [[8, 41, 14, 41], [78, 45, 84, 45], [40, 59, 46, 59]]
    return g, holds, plan, signs, cables

# ------------------------------------------------------------------ Ko'shk
# A country mansion. The attackers start INSIDE the house (the grand hall, south) and break out; the defenders come in
# through the front gate (north) and hold the grounds: A — the garage and driveway (west), B — the pool terrace (east).
# Mid: the front garden with hedges between the porch and the gate. The house has four ways out: the front door
# (foyer -> porch), the library door (west wing -> west path), the kitchen door (east wing -> east path) and the
# back corridors from the hall into both wings.
def koshk():
    g = new()
    rect(g, 16, 3, 27, 5, 'k')                          # defenders: the front gate and the guard post
    rect(g, 4, 6, 39, 7)                                # path along the fence
    rect(g, 20, 8, 23, 8)                               # gate into the garden
    # A: garage and driveway (west)
    rect(g, 3, 8, 13, 17, 'A')
    # B: pool terrace (east)
    rect(g, 30, 8, 40, 17, 'B')
    # mid: front garden
    rect(g, 15, 9, 28, 20)
    rect(g, 14, 12, 14, 14); rect(g, 29, 12, 29, 14)    # garden <-> sites
    rect(g, 19, 21, 24, 24)                             # porch and front steps
    # side paths round the house
    rect(g, 3, 18, 6, 30)                               # west path: A <-> library door
    rect(g, 37, 18, 40, 30)                             # east path: B <-> kitchen door
    rect(g, 7, 20, 9, 21); rect(g, 34, 20, 36, 21)      # little lawns by the paths
    # the house (south)
    rect(g, 18, 25, 25, 30)                             # foyer
    rect(g, 16, 31, 27, 34)                             # stair hall
    rect(g, 14, 35, 29, 39, 't')                        # attackers: the grand hall
    rect(g, 8, 27, 14, 33)                              # west wing: library
    rect(g, 15, 28, 17, 29)                             # foyer <-> library
    rect(g, 7, 29, 7, 30)                               # library door to the west path
    rect(g, 10, 34, 13, 36)                             # grand hall <-> library (back corridor)
    rect(g, 29, 27, 35, 33)                             # east wing: kitchen and dining room
    rect(g, 26, 28, 28, 29)                             # foyer <-> dining room
    rect(g, 36, 29, 36, 30)                             # kitchen door to the east path
    rect(g, 30, 34, 33, 36)                             # grand hall <-> kitchen (back corridor)
    # cover: cars and boxes in the garage, sun loungers and the pool house on B, hedges and planters in the garden,
    # furniture inside
    put(g, [(5, 10), (5, 11), (5, 12), (9, 10), (9, 11), (9, 12)], 's')                     # two cars
    put(g, [(12, 9), (4, 16), (11, 15), (7, 15)], 'c')
    put(g, [(38, 9), (39, 9), (38, 10), (39, 10)], 's')                                     # pool house
    put(g, [(32, 15), (34, 15), (36, 15), (31, 9)], 'c')                                    # loungers
    put(g, [(17, 11), (18, 11), (25, 11), (26, 11), (17, 17), (18, 17), (25, 17), (26, 17)], 's')   # hedges
    put(g, [(21, 14), (22, 14), (16, 14), (27, 14), (21, 19)], 'c')                         # planters, bench
    put(g, [(8, 29), (11, 29), (8, 32), (12, 31)], 's')                                     # library shelves
    put(g, [(10, 31)], 'c')
    put(g, [(31, 29), (32, 29), (31, 30), (32, 30)], 's')                                   # dining table
    put(g, [(34, 28), (34, 32), (30, 33)], 'c')                                             # kitchen counter
    put(g, [(19, 27), (24, 27)], 'c')                                                       # foyer: statues on plinths
    put(g, [(17, 33), (26, 33)], 's')                                                       # stair hall pillars
    put(g, [(4, 22), (39, 23), (5, 27), (38, 26)], 'c')
    put(g, [(17, 37), (26, 37), (21, 38)], 'c')                                             # sofas in the grand hall
    holds = {"A": [[[11, 9], [4, 18]], [[4, 9], [14, 13]], [[12, 16], [5, 18]], [[7, 13], [14, 12]]],
             "B": [[[32, 9], [39, 18]], [[40, 12], [29, 13]], [[31, 16], [38, 18]], [[35, 9], [29, 12]]],
             "M": [[[21, 9], [21, 22]], [[16, 15], [21, 21]], [[27, 16], [22, 21]]]}
    plan = {"A": {"stages": [[5, 28], [20, 22], [5, 20]], "spots": [[7, 10], [8, 14], [11, 11], [4, 13]], "looks": [[14, 13], [8, 7], [4, 18]]},
            "B": {"stages": [[38, 28], [23, 22], [38, 20]], "spots": [[31, 12], [33, 16], [37, 16], [40, 13]], "looks": [[29, 13], [35, 7], [39, 18]]}}
    signs = [[2, 12, "E", "A", 3, 2.6], [41, 12, "W", "B", 3, 2.6], [14, 10, "W", "GARAJ", 3.4, 1.3], [29, 10, "E", "HOVUZ", 3.4, 1.3],
             [16, 21, "N", "← A", 2.1], [27, 21, "N", "B →", 2.1], [11, 26, "S", "KUTUBXONA", 2.4, 1.5], [32, 26, "S", "OSHXONA", 2.4, 1.5],
             [2, 24, "E", "A ↑", 2.1], [41, 24, "W", "B ↑", 2.1], [21, 40, "N", "MEHMONXONA", 2.4, 1.4]]
    cables = [[30, 30, 58, 30], [30, 36, 58, 36]]
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
    for name, fn in (('bekat', bekat), ('saroy', saroy), ('zavod', zavod), ('koshk', koshk)):
        g, holds, plan, signs, cables = fn()
        rows = check(name, g, holds, plan)
        draw(name, rows, holds, plan, f'{sys.argv[1]}/{name}.png')
        out[name] = dict(rows=rows, holds=holds, plan=plan, signs=signs, cables=cables)
        print(name, 'ok'); print('\n'.join(rows))
    json.dump(out, open(f'{sys.argv[1]}/maps.json', 'w'))
