package main

// RFC 6455 WebSocket: faqat serverga kerakli qismi (matnli xabarlar, ping/pong, yopish).

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	wsGUID     = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	opText     = 0x1
	opClose    = 0x8
	opPing     = 0x9
	opPong     = 0xA
	readIdle   = 70 * time.Second
	writeLimit = 5 * time.Second
)

var errTooBig = errors.New("xabar juda katta")

type wsConn struct {
	c   net.Conn
	br  *bufio.Reader
	wmu sync.Mutex
}

func headerHas(h http.Header, key, token string) bool {
	for _, v := range h.Values(key) {
		for _, p := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(p), token) {
				return true
			}
		}
	}
	return false
}

func upgrade(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if r.Method != http.MethodGet || !headerHas(r.Header, "Connection", "upgrade") || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "WebSocket ulanishi kutilgan", http.StatusBadRequest)
		return nil, errors.New("not a websocket request")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, "WebSocket 13-versiyasi kerak", http.StatusUpgradeRequired)
		return nil, errors.New("bad version")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Sec-WebSocket-Key yo'q", http.StatusBadRequest)
		return nil, errors.New("no key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "server ulanishni o'tkaza olmaydi", http.StatusInternalServerError)
		return nil, errors.New("no hijacker")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n"
	conn.SetWriteDeadline(time.Now().Add(writeLimit))
	if _, err := conn.Write([]byte(resp)); err != nil {
		conn.Close()
		return nil, err
	}
	return &wsConn{c: conn, br: brw.Reader}, nil
}

// read returns the next complete text message. Pings are answered here.
func (ws *wsConn) read(max int) ([]byte, error) {
	var msg []byte
	started := false
	for {
		ws.c.SetReadDeadline(time.Now().Add(readIdle))
		var h [2]byte
		if _, err := io.ReadFull(ws.br, h[:]); err != nil {
			return nil, err
		}
		fin, op, masked := h[0]&0x80 != 0, h[0]&0x0f, h[1]&0x80 != 0
		if h[0]&0x70 != 0 || !masked {
			return nil, errors.New("protokol xatosi")
		}
		n := uint64(h[1] & 0x7f)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(ws.br, b[:]); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(ws.br, b[:]); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if n > uint64(max) || uint64(len(msg))+n > uint64(max) {
			return nil, errTooBig
		}
		var mask [4]byte
		if _, err := io.ReadFull(ws.br, mask[:]); err != nil {
			return nil, err
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(ws.br, p); err != nil {
			return nil, err
		}
		for i := range p {
			p[i] ^= mask[i&3]
		}
		if op >= 0x8 {
			if !fin || n > 125 {
				return nil, errors.New("protokol xatosi")
			}
			switch op {
			case opClose:
				ws.write(opClose, nil)
				return nil, io.EOF
			case opPing:
				ws.write(opPong, p)
			}
			continue
		}
		if op == 0x0 {
			if !started {
				return nil, errors.New("protokol xatosi")
			}
			msg = append(msg, p...)
		} else {
			if started || op != opText {
				return nil, errors.New("faqat matnli xabarlar")
			}
			started, msg = true, p
		}
		if fin {
			return msg, nil
		}
	}
}

func (ws *wsConn) write(op byte, p []byte) error {
	ws.wmu.Lock()
	defer ws.wmu.Unlock()
	hdr := make([]byte, 0, 10+len(p))
	hdr = append(hdr, 0x80|op)
	switch n := len(p); {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n < 1<<16:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr = append(append(hdr, 127), b[:]...)
	}
	ws.c.SetWriteDeadline(time.Now().Add(writeLimit))
	_, err := ws.c.Write(append(hdr, p...))
	return err
}

func (ws *wsConn) close() { ws.c.Close() }
