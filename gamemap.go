package main

// Xaritalarning server nusxasi: brauzerdagi o'yin bilan aynan bir xil panjaralar (grid).
// Server ulardan o'q devor ortidan tegmaganini, bomba nuqtada o'rnatilganini va
// o'yinchi devor ichida emasligini tekshirishda foydalanadi. Har xona o'z xaritasida o'ynaydi.
// Xarita ma'lumotlari mapsdata.go da — u web/index.html dagi MAPS dan yasaladi
// (TestMapsMatchClient ikkalasi bir xilligini tekshiradi).

import "math"

const (
	cellSize = 2.0
	gridW    = 44
	gridH    = 44
)

const (
	cFloor = iota
	cWall
	cCrate
	cStack
	cSiteA
	cSiteB
	cTSpawn
	cCTSpawn
)

var cellHeight = [8]float64{0, 7, 1.0, 2.3, 0, 0, 0, 0}

// mapDef — xarita ta'rifi (mapsdata.go, brauzerdagi MAPS dan yasalgan).
type mapDef struct {
	id, name     string
	holds        [][2][2]int         // himoyachi botlar turadigan joy va qaraydigan nuqta
	spots, looks map[string][][2]int // hujumchi botlar bomba qo'yadigan joylar va qarash nuqtalari
	rows         []string
}

// gameMap — panjara va botlar uchun ma'lumot.
type gameMap struct {
	mapDef
	grid              [gridW * gridH]uint8
	tCenter, ctCenter vec3
	open              [][2]int // yashik yonida bo'lmagan ochiq kataklar (deathmatch'da tug'ilish, botlar aylanishi)
}

var cellOfChar = map[byte]uint8{'#': cWall, '.': cFloor, 'c': cCrate, 's': cStack, 'A': cSiteA, 'B': cSiteB, 't': cTSpawn, 'k': cCTSpawn}

var gameMaps = func() map[string]*gameMap {
	out := map[string]*gameMap{}
	for _, d := range mapDefs {
		m := &gameMap{mapDef: d}
		for z, row := range d.rows {
			for x := 0; x < gridW; x++ {
				t, ok := cellOfChar[row[x]]
				if !ok {
					t = cWall
				}
				m.grid[z*gridW+x] = t
			}
		}
		m.tCenter, m.ctCenter = m.zoneCenter(cTSpawn), m.zoneCenter(cCTSpawn)
		for z := 0; z < gridH; z++ {
			for x := 0; x < gridW; x++ {
				ok := !m.botSolid(x, z)
				for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					if t := m.cellType(x+d[0], z+d[1]); t == cCrate || t == cStack {
						ok = false
					}
				}
				if ok {
					m.open = append(m.open, [2]int{x, z})
				}
			}
		}
		out[d.id] = m
	}
	return out
}()

const defaultMap = "qumtepa"

// mapByID: noma'lum nom — standart xarita.
func mapByID(id string) *gameMap {
	if m := gameMaps[id]; m != nil {
		return m
	}
	return gameMaps[defaultMap]
}

func validMap(id string) bool { return gameMaps[id] != nil }

// zoneCenter: baza kataklarining o'rtasiga eng yaqin baza katagi (to'siqsiz).
func (m *gameMap) zoneCenter(t uint8) vec3 {
	var sx, sz float64
	var cells [][2]int
	for z := 0; z < gridH; z++ {
		for x := 0; x < gridW; x++ {
			if m.grid[z*gridW+x] == t {
				cells = append(cells, [2]int{x, z})
				sx += float64(x)
				sz += float64(z)
			}
		}
	}
	if len(cells) == 0 {
		return vec3{gridW, 0, gridH}
	}
	mx, mz := sx/float64(len(cells)), sz/float64(len(cells))
	best, bd := cells[0], math.Inf(1)
	for _, c := range cells {
		if d := math.Hypot(float64(c[0])-mx, float64(c[1])-mz); d < bd {
			best, bd = c, d
		}
	}
	return cellCenter(best)
}

func (m *gameMap) cellType(cx, cz int) uint8 {
	if cx < 0 || cz < 0 || cx >= gridW || cz >= gridH {
		return cWall
	}
	return m.grid[cz*gridW+cx]
}

func (m *gameMap) cellH(cx, cz int) float64 { return cellHeight[m.cellType(cx, cz)] }
func wc(v float64) int                      { return int(math.Floor(v / cellSize)) }

// siteAt: "A", "B" yoki "".
func (m *gameMap) siteAt(x, z float64) string {
	switch m.cellType(wc(x), wc(z)) {
	case cSiteA:
		return "A"
	case cSiteB:
		return "B"
	}
	return ""
}

type vec3 struct{ X, Y, Z float64 }

func (a vec3) sub(b vec3) vec3      { return vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a vec3) len() float64         { return math.Sqrt(a.X*a.X + a.Y*a.Y + a.Z*a.Z) }
func (a vec3) flat(b vec3) float64  { return math.Hypot(a.X-b.X, a.Z-b.Z) }
func (a vec3) add(b vec3) vec3      { return vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a vec3) scale(k float64) vec3 { return vec3{a.X * k, a.Y * k, a.Z * k} }

// rayGrid — brauzerdagi rayGrid bilan bir xil: devorlar, yashik ustlari va pol.
func (m *gameMap) rayGrid(o, d vec3, maxD float64) float64 {
	cx, cz := wc(o.X), wc(o.Z)
	stepX, stepZ := -1, -1
	if d.X > 0 {
		stepX = 1
	}
	if d.Z > 0 {
		stepZ = 1
	}
	inf := math.Inf(1)
	tDX, tDZ := inf, inf
	if d.X != 0 {
		tDX = math.Abs(cellSize / d.X)
	}
	if d.Z != 0 {
		tDZ = math.Abs(cellSize / d.Z)
	}
	tMX, tMZ := inf, inf
	if d.X != 0 {
		if d.X > 0 {
			tMX = (float64(cx+1)*cellSize - o.X) / math.Abs(d.X)
		} else {
			tMX = (o.X - float64(cx)*cellSize) / math.Abs(d.X)
		}
	}
	if d.Z != 0 {
		if d.Z > 0 {
			tMZ = (float64(cz+1)*cellSize - o.Z) / math.Abs(d.Z)
		} else {
			tMZ = (o.Z - float64(cz)*cellSize) / math.Abs(d.Z)
		}
	}
	tE := 0.0
	for i := 0; i < 400; i++ {
		tX := math.Min(math.Min(tMX, tMZ), maxD)
		h := m.cellH(cx, cz)
		if tE > 0 && o.Y+d.Y*tE < h {
			return tE
		}
		if d.Y < 0 {
			if tt := (h - o.Y) / d.Y; tt >= tE-1e-6 && tt <= tX {
				return math.Max(tt, 0)
			}
		}
		if tX >= maxD {
			return maxD
		}
		if tMX < tMZ {
			tE = tMX
			tMX += tDX
			cx += stepX
		} else {
			tE = tMZ
			tMZ += tDZ
			cz += stepZ
		}
		if cx < -2 || cz < -2 || cx > gridW+1 || cz > gridH+1 {
			return tE
		}
	}
	return maxD
}

// lineOfSight: a va b nuqtalar orasida devor yo'qmi.
func (m *gameMap) lineOfSight(a, b vec3) bool {
	d := b.sub(a)
	dist := d.len()
	if dist < .01 {
		return true
	}
	return m.rayGrid(a, d.scale(1/dist), dist) >= dist-.05
}

// insideSolid: o'yinchi oyog'i turgan nuqta devor/yashik ichidami (yashik ustida turish mumkin).
func (m *gameMap) insideSolid(p vec3) bool {
	return m.cellH(wc(p.X), wc(p.Z)) > p.Y+.6
}

// spawnZone: o'z tomonining boshlang'ich hududi.
func (m *gameMap) inSpawn(p vec3, side string) bool {
	t := m.cellType(wc(p.X), wc(p.Z))
	return (side == "T" && t == cTSpawn) || (side == "CT" && t == cCTSpawn)
}
