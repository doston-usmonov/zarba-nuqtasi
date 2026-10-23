package main

// Xaritaning server nusxasi: brauzerdagi o'yin bilan aynan bir xil panjara (grid).
// Server undan o'q devor ortidan tegmaganini, bomba nuqtada o'rnatilganini va
// o'yinchi devor ichida emasligini tekshirishda foydalanadi.
// O'zgartirilsa, web/index.html dagi MAP bo'limi bilan birga o'zgartiring.

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

var grid = buildGrid()

func buildGrid() [gridW * gridH]uint8 {
	var g [gridW * gridH]uint8
	for i := range g {
		g[i] = cWall
	}
	carve := func(x0, y0, x1, y1 int, t uint8) {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				g[y*gridW+x] = t
			}
		}
	}
	put := func(t uint8, pts ...[2]int) {
		for _, p := range pts {
			g[p[1]*gridW+p[0]] = t
		}
	}
	carve(14, 35, 29, 40, cTSpawn)
	carve(16, 2, 27, 7, cCTSpawn)
	carve(3, 3, 13, 13, cSiteA)
	carve(30, 3, 40, 13, cSiteB)
	carve(3, 14, 6, 33, cFloor)
	carve(3, 33, 13, 37, cFloor)
	carve(19, 8, 24, 34, cFloor)
	carve(9, 15, 18, 17, cFloor)
	carve(9, 14, 11, 14, cFloor)
	carve(14, 4, 15, 6, cFloor)
	carve(28, 4, 29, 6, cFloor)
	carve(36, 14, 39, 34, cFloor)
	carve(30, 33, 39, 37, cFloor)
	carve(25, 21, 35, 23, cFloor)
	put(cWall, [2]int{19, 12}, [2]int{20, 12}, [2]int{23, 12}, [2]int{24, 12})
	put(cStack, [2]int{6, 6}, [2]int{7, 6}, [2]int{6, 7}, [2]int{7, 7}, [2]int{34, 7}, [2]int{35, 7}, [2]int{34, 8}, [2]int{35, 8},
		[2]int{3, 26}, [2]int{4, 26}, [2]int{21, 39}, [2]int{22, 39}, [2]int{21, 33}, [2]int{22, 33})
	put(cCrate, [2]int{11, 9}, [2]int{11, 10}, [2]int{4, 11}, [2]int{9, 4}, [2]int{31, 5}, [2]int{38, 11}, [2]int{39, 11}, [2]int{32, 11},
		[2]int{21, 22}, [2]int{23, 28}, [2]int{20, 30}, [2]int{5, 20}, [2]int{17, 36}, [2]int{26, 38}, [2]int{18, 3}, [2]int{25, 5},
		[2]int{38, 26}, [2]int{37, 18}, [2]int{30, 22}, [2]int{9, 35})
	return g
}

func cellType(cx, cz int) uint8 {
	if cx < 0 || cz < 0 || cx >= gridW || cz >= gridH {
		return cWall
	}
	return grid[cz*gridW+cx]
}

func cellH(cx, cz int) float64 { return cellHeight[cellType(cx, cz)] }
func wc(v float64) int         { return int(math.Floor(v / cellSize)) }

// siteAt: "A", "B" yoki "".
func siteAt(x, z float64) string {
	switch cellType(wc(x), wc(z)) {
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
func rayGrid(o, d vec3, maxD float64) float64 {
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
		h := cellH(cx, cz)
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
func lineOfSight(a, b vec3) bool {
	d := b.sub(a)
	dist := d.len()
	if dist < .01 {
		return true
	}
	return rayGrid(a, d.scale(1/dist), dist) >= dist-.05
}

// insideSolid: o'yinchi oyog'i turgan nuqta devor/yashik ichidami (yashik ustida turish mumkin).
func insideSolid(p vec3) bool {
	return cellH(wc(p.X), wc(p.Z)) > p.Y+.6
}

// spawnZone: o'z tomonining boshlang'ich hududi.
func inSpawn(p vec3, side string) bool {
	t := cellType(wc(p.X), wc(p.Z))
	return (side == "T" && t == cTSpawn) || (side == "CT" && t == cCTSpawn)
}
