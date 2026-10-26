package main

// Do'stlar, partiya (do'stni o'z xonangga chaqirish) va klanlar.
//
// Do'stlik: login bo'yicha so'rov yuboriladi, ikkinchi tomon qabul qiladi. Do'stlar ro'yxatida
// kim onlayn (menyuda yoki qaysi xonada o'ynayotgani) ko'rinadi; do'st o'ynayotgan xonaga bitta
// tugma bilan qo'shilish mumkin. O'yindagi o'yinchi onlayn do'stini o'z xonasiga chaqiradi —
// taklif do'stga menyuda yoki o'yin ichida keladi, qabul qilsa ikkalasi bitta jamoada o'ynaydi.
//
// Klan: nomi va 2–5 harfli tegi bor (o'yinda ism oldida [TEG] ko'rinadi). Klan a'zolari do'stlarini
// taklif qiladi; egasi a'zoni chiqarishi mumkin; klanlar reytingi a'zolarining o'rtacha reytingi bo'yicha.

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	maxFriends    = 100
	maxClanSize   = 30
	inviteTTL     = 2 * time.Minute
	menuOnlineTTL = 40 * time.Second
)

var (
	errNoUser      = errors.New("Bunday o'yinchi topilmadi")
	errSelf        = errors.New("O'zingizni do'st qilib bo'lmaydi")
	errNotFriends  = errors.New("Avval do'st bo'lish kerak")
	errTooMany     = errors.New("Ro'yxat to'lgan")
	errInClan      = errors.New("Siz allaqachon klandasiz")
	errNoClan      = errors.New("Siz klanda emassiz")
	errClanTaken   = errors.New("Bunday nom yoki teg band")
	errClanName    = errors.New("Klan nomi 3–24 belgi, tegi 2–5 ta lotin harfi yoki raqam bo'lsin")
	errNotOwner    = errors.New("Buni faqat klan egasi qila oladi")
	errClanFull    = errors.New("Klan to'lgan")
	errOtherInClan = errors.New("Bu o'yinchi boshqa klanda")
	clanTagRe      = regexp.MustCompile(`^[A-Za-z0-9]{2,5}$`)
)

// ---- baza ----

func (s *Store) userID(ctx context.Context, name string) (int64, string, error) {
	var id int64
	var real string
	err := s.db.QueryRowContext(ctx, `SELECT id, username FROM users WHERE lower(username) = lower($1)`, strings.TrimSpace(name)).Scan(&id, &real)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", errNoUser
	}
	return id, real, err
}

// FriendAction: add (so'rov yoki qarshi so'rovni qabul), accept, decline, remove.
func (s *Store) FriendAction(ctx context.Context, me int64, action, name string) error {
	other, _, err := s.userID(ctx, name)
	if err != nil {
		return err
	}
	if other == me {
		return errSelf
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	switch action {
	case "add":
		var n int
		tx.QueryRowContext(ctx, `SELECT count(*) FROM friends WHERE user_id = $1`, me).Scan(&n)
		if n >= maxFriends {
			return errTooMany
		}
		// u allaqachon menga so'rov yuborgan bo'lsa — bu qabul qilish
		var pending bool
		err := tx.QueryRowContext(ctx, `SELECT NOT accepted FROM friends WHERE user_id = $1 AND friend_id = $2`, other, me).Scan(&pending)
		if err == nil && pending {
			return s.acceptTx(ctx, tx, me, other)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO friends (user_id, friend_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, me, other); err != nil {
			return err
		}
	case "accept":
		var pending bool
		if err := tx.QueryRowContext(ctx, `SELECT NOT accepted FROM friends WHERE user_id = $1 AND friend_id = $2`, other, me).Scan(&pending); err != nil || !pending {
			return errNoUser
		}
		return s.acceptTx(ctx, tx, me, other)
	case "decline", "remove":
		if _, err := tx.ExecContext(ctx, `DELETE FROM friends WHERE (user_id = $1 AND friend_id = $2) OR (user_id = $2 AND friend_id = $1)`, me, other); err != nil {
			return err
		}
	default:
		return errNoUser
	}
	return tx.Commit()
}

func (s *Store) acceptTx(ctx context.Context, tx *sql.Tx, me, other int64) error {
	if _, err := tx.ExecContext(ctx, `UPDATE friends SET accepted = true WHERE user_id = $1 AND friend_id = $2`, other, me); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO friends (user_id, friend_id, accepted) VALUES ($1, $2, true)
		ON CONFLICT (user_id, friend_id) DO UPDATE SET accepted = true`, me, other); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) areFriends(ctx context.Context, a, b int64) bool {
	var ok bool
	s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM friends WHERE user_id = $1 AND friend_id = $2 AND accepted)`, a, b).Scan(&ok)
	return ok
}

type friendRow struct {
	ID       int64    `json:"-"`
	Name     string   `json:"name"`
	Rating   int      `json:"rating"`
	Rank     rankInfo `json:"rank"`
	Level    int      `json:"level"`
	Clan     string   `json:"clan,omitempty"`
	Online   string   `json:"online"` // "", "menu", "game"
	Room     string   `json:"room,omitempty"`
	RoomMode string   `json:"roomMode,omitempty"`
	RoomMap  string   `json:"roomMap,omitempty"`
}

type clanMember struct {
	ID     int64  `json:"-"`
	Name   string `json:"name"`
	Rating int    `json:"rating"`
	Level  int    `json:"level"`
	Owner  bool   `json:"owner"`
	Online string `json:"online"`
}

type clanInfo struct {
	ID      int64        `json:"id"`
	Name    string       `json:"name"`
	Tag     string       `json:"tag"`
	Owner   bool         `json:"owner"`
	Members []clanMember `json:"members"`
}

type clanInvite struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Tag  string `json:"tag"`
	From string `json:"from"`
}

type socialOut struct {
	Friends     []friendRow  `json:"friends"`
	Incoming    []string     `json:"incoming"`
	Outgoing    []string     `json:"outgoing"`
	Invites     []roomInvite `json:"invites"`
	Clan        *clanInfo    `json:"clan"`
	ClanInvites []clanInvite `json:"clanInvites"`
}

func (s *Store) Social(ctx context.Context, me int64) (*socialOut, error) {
	out := &socialOut{Friends: []friendRow{}, Incoming: []string{}, Outgoing: []string{}, Invites: []roomInvite{}, ClanInvites: []clanInvite{}}
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.username, u.rating, u.matches, u.xp, COALESCE(c.tag, '')
		FROM friends f JOIN users u ON u.id = f.friend_id
		LEFT JOIN clan_members m ON m.user_id = u.id LEFT JOIN clans c ON c.id = m.clan_id
		WHERE f.user_id = $1 AND f.accepted ORDER BY lower(u.username)`, me)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var f friendRow
		var matches int
		var xp int64
		if err := rows.Scan(&f.ID, &f.Name, &f.Rating, &matches, &xp, &f.Clan); err != nil {
			rows.Close()
			return nil, err
		}
		f.Rank = rankOf(f.Rating, matches)
		f.Level, _, _ = levelOf(xp)
		out.Friends = append(out.Friends, f)
	}
	rows.Close()
	list := func(q string, dst *[]string) error {
		r, err := s.db.QueryContext(ctx, q, me)
		if err != nil {
			return err
		}
		defer r.Close()
		for r.Next() {
			var n string
			r.Scan(&n)
			*dst = append(*dst, n)
		}
		return r.Err()
	}
	if err := list(`SELECT u.username FROM friends f JOIN users u ON u.id = f.user_id WHERE f.friend_id = $1 AND NOT f.accepted ORDER BY f.created_at`, &out.Incoming); err != nil {
		return nil, err
	}
	if err := list(`SELECT u.username FROM friends f JOIN users u ON u.id = f.friend_id WHERE f.user_id = $1 AND NOT f.accepted ORDER BY f.created_at`, &out.Outgoing); err != nil {
		return nil, err
	}
	if out.Clan, err = s.clanOf(ctx, me); err != nil {
		return nil, err
	}
	r, err := s.db.QueryContext(ctx, `SELECT c.id, c.name, c.tag, u.username FROM clan_invites i JOIN clans c ON c.id = i.clan_id JOIN users u ON u.id = i.from_id
		WHERE i.user_id = $1 ORDER BY i.created_at`, me)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for r.Next() {
		var ci clanInvite
		r.Scan(&ci.ID, &ci.Name, &ci.Tag, &ci.From)
		out.ClanInvites = append(out.ClanInvites, ci)
	}
	return out, r.Err()
}

func (s *Store) clanOf(ctx context.Context, me int64) (*clanInfo, error) {
	var c clanInfo
	err := s.db.QueryRowContext(ctx, `SELECT c.id, c.name, c.tag, m.owner FROM clan_members m JOIN clans c ON c.id = m.clan_id WHERE m.user_id = $1`, me).Scan(&c.ID, &c.Name, &c.Tag, &c.Owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.username, u.rating, u.xp, m.owner FROM clan_members m JOIN users u ON u.id = m.user_id
		WHERE m.clan_id = $1 ORDER BY m.owner DESC, u.rating DESC, lower(u.username)`, c.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m clanMember
		var xp int64
		rows.Scan(&m.ID, &m.Name, &m.Rating, &xp, &m.Owner)
		m.Level, _, _ = levelOf(xp)
		c.Members = append(c.Members, m)
	}
	return &c, rows.Err()
}

// ClanTag: o'yinda ism oldida ko'rinadigan teg ("" — klanda emas).
func (s *Store) ClanTag(ctx context.Context, uid int64) string {
	var tag string
	s.db.QueryRowContext(ctx, `SELECT c.tag FROM clan_members m JOIN clans c ON c.id = m.clan_id WHERE m.user_id = $1`, uid).Scan(&tag)
	return tag
}

// ClanAction: create, invite, accept, decline, leave, kick.
func (s *Store) ClanAction(ctx context.Context, me int64, action, name, tag string, clanID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var myClan int64
	var owner bool
	err = tx.QueryRowContext(ctx, `SELECT clan_id, owner FROM clan_members WHERE user_id = $1`, me).Scan(&myClan, &owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	switch action {
	case "create":
		name = strings.Join(strings.Fields(name), " ")
		if myClan != 0 {
			return errInClan
		}
		if n := len([]rune(name)); n < 3 || n > 24 || !clanTagRe.MatchString(tag) {
			return errClanName
		}
		var id int64
		err := tx.QueryRowContext(ctx, `INSERT INTO clans (name, tag, owner_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING RETURNING id`, name, strings.ToUpper(tag), me).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return errClanTaken
		} else if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO clan_members (user_id, clan_id, owner) VALUES ($1, $2, true)`, me, id); err != nil {
			return err
		}
		tx.ExecContext(ctx, `DELETE FROM clan_invites WHERE user_id = $1`, me)
	case "invite":
		if myClan == 0 {
			return errNoClan
		}
		other, _, err := s.userID(ctx, name)
		if err != nil {
			return err
		}
		if !s.areFriends(ctx, me, other) {
			return errNotFriends
		}
		var inClan bool
		tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM clan_members WHERE user_id = $1)`, other).Scan(&inClan)
		if inClan {
			return errOtherInClan
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO clan_invites (clan_id, user_id, from_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, myClan, other, me); err != nil {
			return err
		}
	case "accept":
		if myClan != 0 {
			return errInClan
		}
		var ok bool
		tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM clan_invites WHERE clan_id = $1 AND user_id = $2)`, clanID, me).Scan(&ok)
		if !ok {
			return errNoClan
		}
		var n int
		tx.QueryRowContext(ctx, `SELECT count(*) FROM clan_members WHERE clan_id = $1`, clanID).Scan(&n)
		if n >= maxClanSize {
			return errClanFull
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO clan_members (user_id, clan_id) VALUES ($1, $2)`, me, clanID); err != nil {
			return err
		}
		tx.ExecContext(ctx, `DELETE FROM clan_invites WHERE user_id = $1`, me)
	case "decline":
		tx.ExecContext(ctx, `DELETE FROM clan_invites WHERE clan_id = $1 AND user_id = $2`, clanID, me)
	case "leave":
		if myClan == 0 {
			return errNoClan
		}
		tx.ExecContext(ctx, `DELETE FROM clan_members WHERE user_id = $1`, me)
		if owner { // egasi ketsa — eng eski a'zo ega bo'ladi, hech kim qolmasa klan o'chadi
			var next int64
			err := tx.QueryRowContext(ctx, `SELECT user_id FROM clan_members WHERE clan_id = $1 ORDER BY joined_at, user_id LIMIT 1`, myClan).Scan(&next)
			if errors.Is(err, sql.ErrNoRows) {
				tx.ExecContext(ctx, `DELETE FROM clans WHERE id = $1`, myClan)
			} else if err == nil {
				tx.ExecContext(ctx, `UPDATE clan_members SET owner = true WHERE user_id = $1`, next)
				tx.ExecContext(ctx, `UPDATE clans SET owner_id = $2 WHERE id = $1`, myClan, next)
			}
		}
	case "kick":
		if myClan == 0 || !owner {
			return errNotOwner
		}
		other, _, err := s.userID(ctx, name)
		if err != nil {
			return err
		}
		if other == me {
			return errSelf
		}
		tx.ExecContext(ctx, `DELETE FROM clan_members WHERE user_id = $1 AND clan_id = $2`, other, myClan)
	default:
		return errNoClan
	}
	return tx.Commit()
}

type clanRow struct {
	Pos     int    `json:"pos"`
	Name    string `json:"name"`
	Tag     string `json:"tag"`
	Members int    `json:"members"`
	Rating  int    `json:"rating"` // a'zolarining o'rtacha reytingi
	Wins    int    `json:"wins"`
}

func (s *Store) Clans(ctx context.Context, limit int) ([]clanRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.name, c.tag, count(*), round(avg(u.rating))::int, sum(u.wins)::int
		FROM clans c JOIN clan_members m ON m.clan_id = c.id JOIN users u ON u.id = m.user_id
		GROUP BY c.id ORDER BY avg(u.rating) DESC, sum(u.wins) DESC, c.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []clanRow{}
	for rows.Next() {
		var c clanRow
		rows.Scan(&c.Name, &c.Tag, &c.Members, &c.Rating, &c.Wins)
		c.Pos = len(out) + 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- kim onlayn: xonadagi o'yinchilar va menyudagilar ----

type roomInvite struct {
	From string    `json:"from"`
	Room string    `json:"room"`
	Mode string    `json:"mode"`
	Map  string    `json:"map"`
	At   time.Time `json:"-"`
}

type social struct {
	inRoom  map[int64]*room        // hisob → xona
	seen    map[int64]time.Time    // menyuda oxirgi ko'rinish (/api/social so'rovi)
	invites map[int64][]roomInvite // hisob → kelgan chaqiriqlar
}

func (h *hub) socialLocked() *social {
	if h.soc == nil {
		h.soc = &social{inRoom: map[int64]*room{}, seen: map[int64]time.Time{}, invites: map[int64][]roomInvite{}}
	}
	return h.soc
}

// presenceOf: do'stning holati (h.mu ostida).
func (h *hub) presenceOf(uid int64, now time.Time) (state string, r *room) {
	soc := h.socialLocked()
	if r := soc.inRoom[uid]; r != nil {
		return "game", r
	}
	if now.Sub(soc.seen[uid]) < menuOnlineTTL {
		return "menu", nil
	}
	return "", nil
}

func roomCode(r *room) string { return strings.TrimPrefix(r.name, "zn-") }

// inviteToRoom: o'yinchi onlayn do'stini o'z xonasiga chaqiradi.
func (h *hub) inviteToRoom(from *account, to int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	soc := h.socialLocked()
	r := soc.inRoom[from.id]
	if r == nil {
		return errors.New("Avval xonaga kiring")
	}
	if soc.inRoom[to] == r {
		return errors.New("Do'stingiz allaqachon shu xonada")
	}
	r.mu.Lock()
	inv := roomInvite{From: from.name, Room: roomCode(r), Mode: r.gs.modeID(), Map: r.gs.M, At: time.Now()}
	// do'st boshqa xonada o'ynayotgan bo'lsa — darhol o'yin ichida xabar beramiz
	r.mu.Unlock()
	list := soc.invites[to][:0]
	for _, x := range soc.invites[to] {
		if x.From != from.name && time.Since(x.At) < inviteTTL {
			list = append(list, x)
		}
	}
	soc.invites[to] = append(list, inv)
	if other := soc.inRoom[to]; other != nil && other != r {
		other.mu.Lock()
		for _, c := range other.clients {
			if c.acct != nil && c.acct.id == to {
				c.trySend(jsonMsg(map[string]any{"t": "inv", "from": inv.From, "room": inv.Room, "mode": inv.Mode, "map": inv.Map}))
			}
		}
		other.mu.Unlock()
	}
	return nil
}

// ---- API ----

func (a *api) socialGet(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Hisobga kirilmagan")
		return
	}
	out, err := a.st.Social(r.Context(), u.ID)
	if err != nil {
		serverErr(w, "do'stlar", err)
		return
	}
	if a.hub != nil {
		h, now := a.hub, time.Now()
		h.mu.Lock()
		soc := h.socialLocked()
		if soc.inRoom[u.ID] == nil {
			soc.seen[u.ID] = now
		}
		for i := range out.Friends {
			f := &out.Friends[i]
			state, rm := h.presenceOf(f.ID, now)
			f.Online = state
			if rm != nil {
				rm.mu.Lock()
				f.Room, f.RoomMode, f.RoomMap = roomCode(rm), rm.gs.modeID(), rm.gs.M
				rm.mu.Unlock()
			}
		}
		for _, inv := range soc.invites[u.ID] {
			if now.Sub(inv.At) < inviteTTL {
				out.Invites = append(out.Invites, inv)
			}
		}
		if out.Clan != nil {
			for i := range out.Clan.Members {
				out.Clan.Members[i].Online, _ = h.presenceOf(out.Clan.Members[i].ID, now)
			}
		}
		h.mu.Unlock()
	}
	sort.SliceStable(out.Friends, func(i, j int) bool { // avval onlayn
		return (out.Friends[i].Online != "") && (out.Friends[j].Online == "")
	})
	writeJSON(w, http.StatusOK, out)
}

func (a *api) friendPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if !a.readJSON(w, r, &req) {
		return
	}
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Hisobga kirilmagan")
		return
	}
	if req.Action == "invite" {
		other, _, err := a.st.userID(r.Context(), req.Name)
		if err == nil && !a.st.areFriends(r.Context(), u.ID, other) {
			err = errNotFriends
		}
		if err == nil && a.hub != nil {
			err = a.hub.inviteToRoom(&account{id: u.ID, name: u.Username}, other)
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if err := a.st.FriendAction(r.Context(), u.ID, req.Action, req.Name); err != nil {
		socialErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *api) clanPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		Name   string `json:"name"`
		Tag    string `json:"tag"`
		ID     int64  `json:"id"`
	}
	if !a.readJSON(w, r, &req) {
		return
	}
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Hisobga kirilmagan")
		return
	}
	if err := a.st.ClanAction(r.Context(), u.ID, req.Action, req.Name, req.Tag, req.ID); err != nil {
		socialErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *api) clansGet(w http.ResponseWriter, r *http.Request) {
	list, err := a.st.Clans(r.Context(), 50)
	if err != nil {
		serverErr(w, "klanlar", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"clans": list})
}

func socialErr(w http.ResponseWriter, err error) {
	for _, e := range []error{errNoUser, errSelf, errNotFriends, errTooMany, errInClan, errNoClan, errClanTaken, errClanName, errNotOwner, errClanFull, errOtherInClan} {
		if errors.Is(err, e) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	serverErr(w, "ijtimoiy", err)
}

// joinedLocked / leftLocked: h.mu ostida (serve va leave).
func (h *hub) joinedLocked(uid int64, r *room) {
	soc := h.socialLocked()
	soc.inRoom[uid] = r
	code := roomCode(r)
	list := soc.invites[uid][:0]
	for _, x := range soc.invites[uid] { // shu xonaga chaqiriqlar bajarildi
		if x.Room != code {
			list = append(list, x)
		}
	}
	if len(list) == 0 {
		delete(soc.invites, uid)
	} else {
		soc.invites[uid] = list
	}
}

func (h *hub) leftLocked(uid int64, r *room) {
	soc := h.socialLocked()
	if soc.inRoom[uid] == r {
		delete(soc.inRoom, uid)
		soc.seen[uid] = time.Now() // menyuga qaytdi
	}
}
