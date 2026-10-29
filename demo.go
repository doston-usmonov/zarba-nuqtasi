package main

// O'yin yozuvlari (demo) va qayta ko'rish.
//
// Raundli o'yinlar (Raqobat, Wingman, Retake, Aim) boshidan oxirigacha yoziladi: har 100 ms da
// o'yin holati (g — brauzerga boradigan o'sha holat) va har o'yinchining joyi, qarashi, joni,
// quroli; o'q uzish, o'lim va granata hodisalari faqat yangilanganda; ism, hisob, bo'yoq — faqat
// o'zgarganda. Yozuv to'g'ridan-to'g'ri diskka siqib yoziladi (DATA_DIR/demos/*.jsonl.gz), xotirada
// turmaydi. O'yin oxirigacha yetgan yozuv bazaga ro'yxatga olinadi (demos jadvali), tugamagani
// o'chiriladi. Eng so'nggi demoKeep tasi saqlanadi.
//
// Brauzer yozuvni kuzatuvchi rejimida o'ynatadi: yozuv «soxta xona» bo'lib o'yinga beriladi,
// shuning uchun kamera, hisob, bomba, o'q izlari, o'ldirishlar lentasi jonli kuzatishdagidek.
//
// Fayl: 1-qator — sarlavha {"v":1,"map","mode","room","started"}; keyin har qator — kadr
// {"t":ms,"g":{...},"p":[{...}, ...]}.

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	demoEvery    = 100 * time.Millisecond
	demoMaxBytes = 30 << 20 // siqilmagan holda; undan katta yozuv to'xtatiladi
	demoKeep     = 300
)

// har kadrda yoziladigan maydonlar (brauzerdagi presence nomlari)
var demoMoving = []string{"sq", "tm", "x", "y", "z", "yw", "pt", "cr", "al", "hp", "w", "mv", "k", "d", "ar"}

// faqat o'zgarganda
var demoStatic = []string{"nm", "bot", "lv", "acc", "fn"}

// hodisalar: "n" raqami oshganda
var demoEvents = []string{"sh", "de", "gn", "sp"}

type demoRec struct {
	path    string
	f       *os.File
	gz      *gzip.Writer
	epoch   string
	started time.Time
	nextAt  time.Time
	raw     int64
	frames  int
	last    map[string]map[string]string // mijoz → maydon → oxirgi qiymat (statik va hodisalar)
	names   map[string]demoPlayer
	done    bool
	broken  bool
}

type demoPlayer struct {
	Name string `json:"name"`
	UID  int64  `json:"uid,omitempty"`
	Bot  bool   `json:"bot,omitempty"`
}

func (h *hub) demoDir() string {
	if h.dataDir == "" {
		return ""
	}
	return filepath.Join(h.dataDir, "demos")
}

// demoTickLocked: run tsiklidan har taktda chaqiriladi.
func (r *room) demoTickLocked(h *hub, now time.Time) {
	g := r.gs
	dir := h.demoDir()
	rounds := !g.respawns() && g.E != "" && g.R > 0 && (g.Ph == "freeze" || g.Ph == "live" || g.Ph == "post" || g.Ph == "over")
	if dir == "" || !h.store.Ready() || !rounds {
		if r.demo != nil && !r.demo.done && g.Ph == "warmup" {
			r.demo.discard() // o'yin oxirigacha yetmadi
			r.demo = nil
		}
		return
	}
	d := r.demo
	if d != nil && d.epoch != g.E {
		if !d.done {
			d.discard()
		}
		d, r.demo = nil, nil
	}
	if d == nil {
		if _, humans := r.botCountLocked(); humans == 0 {
			return
		}
		var err error
		if d, err = newDemo(dir, r, now); err != nil {
			log.Printf("demo yozilmaydi (%s): %v", r.name, err)
			r.demo = &demoRec{epoch: g.E, done: true, broken: true} // shu o'yinda qayta urinmaymiz
			return
		}
		r.demo = d
	}
	if d.done || now.Before(d.nextAt) {
		return
	}
	d.nextAt = now.Add(demoEvery)
	d.frameLocked(r, now)
	if g.Ph == "over" {
		d.done = true
		meta := d.meta(r, now)
		if err := d.close(); err != nil {
			log.Printf("demo yopilmadi: %v", err)
			return
		}
		go h.saveDemo(meta)
	}
}

func newDemo(dir string, r *room, now time.Time) (*demoRec, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s-%s-%s.jsonl.gz", now.UTC().Format("20060102-150405"), strings.TrimPrefix(r.name, "zn-"), r.gs.E)
	path := filepath.Join(dir, name)
	f, err := os.Create(path + ".part")
	if err != nil {
		return nil, err
	}
	d := &demoRec{path: path, f: f, gz: gzip.NewWriter(f), epoch: r.gs.E, started: now, last: map[string]map[string]string{}, names: map[string]demoPlayer{}}
	head := map[string]any{"v": 1, "map": r.gs.M, "mode": r.gs.modeID(), "room": strings.TrimPrefix(r.name, "zn-"), "started": now.Unix()}
	d.write(jsonMsg(head))
	return d, nil
}

func (d *demoRec) write(line []byte) {
	if d.broken {
		return
	}
	d.raw += int64(len(line) + 1)
	if d.raw > demoMaxBytes {
		d.broken = true
		return
	}
	if _, err := d.gz.Write(append(line, '\n')); err != nil {
		d.broken = true
	}
}

// frameLocked: bitta kadr — holat va o'yinchilar.
func (d *demoRec) frameLocked(r *room, now time.Time) {
	var ps []map[string]json.RawMessage
	for _, c := range r.clients {
		if c.spec || len(c.pres) == 0 || c.info.Sq == "" {
			continue
		}
		var f map[string]json.RawMessage
		if json.Unmarshal(c.pres, &f) != nil {
			continue
		}
		out := map[string]json.RawMessage{"id": jsonMsg(c.id)}
		for _, k := range demoMoving {
			if v, ok := f[k]; ok {
				out[k] = v
			}
		}
		last := d.last[c.id]
		if last == nil {
			last = map[string]string{}
			d.last[c.id] = last
		}
		for _, k := range demoStatic {
			if v, ok := f[k]; ok && string(v) != last[k] {
				out[k], last[k] = v, string(v)
			}
		}
		for _, k := range demoEvents {
			v, ok := f[k]
			if !ok || len(v) == 0 || v[0] != '{' {
				continue
			}
			var ev struct {
				N int `json:"n"`
			}
			json.Unmarshal(v, &ev)
			if n := strconv.Itoa(ev.N); ev.N > 0 && n != last[k] {
				out[k], last[k] = v, n
			}
		}
		ps = append(ps, out)
		p := demoPlayer{Name: clientName(c), Bot: c.bot != nil}
		if c.acct != nil {
			p.UID = c.acct.id
		}
		d.names[c.id] = p
	}
	d.frames++
	d.write(jsonMsg(map[string]any{"t": now.Sub(d.started).Milliseconds(), "g": r.gs, "p": ps}))
}

type demoMeta struct {
	file           string
	room, gmap, md string
	started, ended time.Time
	sa, sb         int
	players        []demoPlayer
	ok             bool
}

func (d *demoRec) meta(r *room, now time.Time) demoMeta {
	m := demoMeta{file: filepath.Base(d.path), room: strings.TrimPrefix(r.name, "zn-"), gmap: r.gs.M, md: r.gs.modeID(), started: d.started, ended: now, sa: r.gs.SA, sb: r.gs.SB, ok: !d.broken}
	for _, p := range d.names {
		m.players = append(m.players, p)
	}
	return m
}

func (d *demoRec) close() error {
	err := d.gz.Close()
	if e := d.f.Close(); err == nil {
		err = e
	}
	if err != nil || d.broken {
		os.Remove(d.path + ".part")
		if err == nil {
			err = errors.New("yozuv juda katta yoki buzilgan")
		}
		return err
	}
	return os.Rename(d.path+".part", d.path)
}

func (d *demoRec) discard() {
	if d.gz != nil {
		d.gz.Close()
		d.f.Close()
		os.Remove(d.path + ".part")
	}
}

// saveDemo: tugagan yozuvni bazaga yozadi, eskilarini o'chiradi.
func (h *hub) saveDemo(m demoMeta) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pl, _ := json.Marshal(m.players)
	var id int64
	err := h.store.db.QueryRowContext(ctx, `INSERT INTO demos (file, room, map, mode, started_at, ended_at, score_a, score_b, players)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`, m.file, m.room, m.gmap, m.md, m.started, m.ended, m.sa, m.sb, pl).Scan(&id)
	if err != nil {
		log.Printf("demo bazaga yozilmadi: %v", err)
		os.Remove(filepath.Join(h.demoDir(), m.file))
		return
	}
	log.Printf("demo #%d saqlandi: %s (%d:%d)", id, m.file, m.sa, m.sb)
	rows, err := h.store.db.QueryContext(ctx, `DELETE FROM demos WHERE id IN (SELECT id FROM demos ORDER BY id DESC OFFSET $1) RETURNING file`, demoKeep)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var f string
		if rows.Scan(&f) == nil && !strings.ContainsAny(f, `/\`) {
			os.Remove(filepath.Join(h.demoDir(), f))
		}
	}
}

// ---- API ----

type demoRow struct {
	ID      int64        `json:"id"`
	Room    string       `json:"room"`
	Map     string       `json:"map"`
	Mode    string       `json:"mode"`
	Started int64        `json:"started"`
	Secs    int          `json:"secs"`
	SA      int          `json:"sA"`
	SB      int          `json:"sB"`
	Players []demoPlayer `json:"players"`
}

// demosHandler: GET /api/demos?player=<login> — so'nggi yozuvlar.
func (h *hub) demosHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.store.Ready() {
		writeErr(w, http.StatusServiceUnavailable, "Yozuvlar vaqtincha ishlamayapti")
		return
	}
	q := `SELECT id, room, map, mode, extract(epoch FROM started_at)::bigint, extract(epoch FROM ended_at - started_at)::int, score_a, score_b, players FROM demos`
	args := []any{}
	if p := strings.TrimSpace(r.URL.Query().Get("player")); p != "" {
		q += ` WHERE EXISTS (SELECT 1 FROM jsonb_array_elements(players) e WHERE lower(e->>'name') = lower($1) AND NOT coalesce((e->>'bot')::boolean, false))`
		args = append(args, p)
	}
	q += ` ORDER BY id DESC LIMIT 30`
	rows, err := h.store.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		serverErr(w, "demolar", err)
		return
	}
	defer rows.Close()
	out := []demoRow{}
	for rows.Next() {
		var d demoRow
		var pl []byte
		if err := rows.Scan(&d.ID, &d.Room, &d.Map, &d.Mode, &d.Started, &d.Secs, &d.SA, &d.SB, &pl); err != nil {
			serverErr(w, "demolar", err)
			return
		}
		json.Unmarshal(pl, &d.Players)
		if d.Players == nil {
			d.Players = []demoPlayer{}
		}
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"demos": out})
}

// demoFileHandler: GET /api/demos/{id} — yozuvning o'zi (gzip).
func (h *hub) demoFileHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if !h.store.Ready() {
		writeErr(w, http.StatusServiceUnavailable, "Yozuvlar vaqtincha ishlamayapti")
		return
	}
	var file string
	if err := h.store.db.QueryRowContext(r.Context(), `SELECT file FROM demos WHERE id = $1`, id).Scan(&file); err != nil || strings.ContainsAny(file, `/\`) {
		writeErr(w, http.StatusNotFound, "Yozuv topilmadi")
		return
	}
	b, err := os.ReadFile(filepath.Join(h.demoDir(), file))
	if err != nil {
		writeErr(w, http.StatusNotFound, "Yozuv fayli yo'q")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(b)
		return
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Yozuv buzilgan")
		return
	}
	defer zr.Close()
	var buf bytes.Buffer
	buf.ReadFrom(zr)
	w.Write(buf.Bytes())
}
