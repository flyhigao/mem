package handlers

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"mem/webservice/internal/auth"
	"mem/webservice/internal/db"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type WSClientConn struct {
	conn   net.Conn
	userID int64
	sendMu sync.Mutex
}

// StreamHub handles both WebSocket (for Desktop/Linux) and SSE (for Android/Mobile)
type StreamHub struct {
	db         *db.DB
	mu         sync.RWMutex
	wsClients  map[int64]map[*WSClientConn]bool
	sseClients map[int64]map[chan *db.Message]bool
}

func NewStreamHub(database *db.DB) *StreamHub {
	return &StreamHub{
		db:         database,
		wsClients:  make(map[int64]map[*WSClientConn]bool),
		sseClients: make(map[int64]map[chan *db.Message]bool),
	}
}

func computeAccept(key string) string {
	h := sha1.New()
	h.Write([]byte(strings.TrimSpace(key) + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// HandleWS upgrades HTTP connection to WebSocket (RFC 6455) for Desktop / Linux
func (h *StreamHub) HandleWS(w http.ResponseWriter, r *http.Request) {
	userID, _, err := auth.AuthenticateAPI(h.db, r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Bad Request: missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket hijack not supported", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(w, "Failed to hijack connection: "+err.Error(), http.StatusInternalServerError)
		return
	}

	accept := computeAccept(key)
	handshake := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"

	if _, err := bufrw.WriteString(handshake); err != nil {
		conn.Close()
		return
	}
	if err := bufrw.Flush(); err != nil {
		conn.Close()
		return
	}

	clientConn := &WSClientConn{
		conn:   conn,
		userID: userID,
	}

	h.registerWS(clientConn)
	defer h.unregisterWS(clientConn)

	log.Printf("🔌 WebSocket client connected for User %d (%s)", userID, conn.RemoteAddr())

	// Read loop (handle Ping/Pong/Close)
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(conn, header); err != nil {
			break
		}

		opcode := header[0] & 0x0F
		masked := (header[1] & 0x80) != 0
		length := int(header[1] & 0x7F)

		if length == 126 {
			ext := make([]byte, 2)
			if _, err := io.ReadFull(conn, ext); err != nil {
				break
			}
			length = int(ext[0])<<8 | int(ext[1])
		} else if length == 127 {
			ext := make([]byte, 8)
			if _, err := io.ReadFull(conn, ext); err != nil {
				break
			}
			length = int(ext[4])<<24 | int(ext[5])<<16 | int(ext[6])<<8 | int(ext[7])
		}

		var maskKey []byte
		if masked {
			maskKey = make([]byte, 4)
			if _, err := io.ReadFull(conn, maskKey); err != nil {
				break
			}
		}

		payload := make([]byte, length)
		if length > 0 {
			if _, err := io.ReadFull(conn, payload); err != nil {
				break
			}
			if masked {
				for i := 0; i < length; i++ {
					payload[i] ^= maskKey[i%4]
				}
			}
		}

		if opcode == 0x8 {
			break // Close frame
		} else if opcode == 0x9 {
			clientConn.sendFrame(0x8A, payload) // Reply Pong (0x8A)
		}
	}
}

// HandleSSE handles GET /api/v1/messages/stream (SSE for Android deep-sleep low power push)
func (h *StreamHub) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, _, err := auth.AuthenticateAPI(h.db, r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	msgChan := make(chan *db.Message, 16)
	h.registerSSE(userID, msgChan)
	defer h.unregisterSSE(userID, msgChan)

	log.Printf("📱 SSE client connected for User %d (%s)", userID, r.RemoteAddr)

	// Send connection established comment
	fmt.Fprintf(w, ":connected\n\n")
	flusher.Flush()

	// 30-second NAT keepalive heartbeat per ROADMAP.md
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ":keepalive\n\n")
			flusher.Flush()
		case msg := <-msgChan:
			data, err := json.Marshal(msg)
			if err == nil {
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", string(data))
				flusher.Flush()
			}
		}
	}
}

func (c *WSClientConn) sendFrame(headerByte byte, payload []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	frame := []byte{headerByte}
	l := len(payload)
	if l < 126 {
		frame = append(frame, byte(l))
	} else if l < 65536 {
		frame = append(frame, 126, byte(l>>8), byte(l))
	} else {
		frame = append(frame, 127, 0, 0, 0, 0, byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
	}
	frame = append(frame, payload...)

	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := c.conn.Write(frame)
	return err
}

func (h *StreamHub) registerWS(c *WSClientConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.wsClients[c.userID] == nil {
		h.wsClients[c.userID] = make(map[*WSClientConn]bool)
	}
	h.wsClients[c.userID][c] = true
}

func (h *StreamHub) unregisterWS(c *WSClientConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if userMap, exists := h.wsClients[c.userID]; exists {
		delete(userMap, c)
		if len(userMap) == 0 {
			delete(h.wsClients, c.userID)
		}
	}
	_ = c.conn.Close()
}

func (h *StreamHub) registerSSE(userID int64, ch chan *db.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.sseClients[userID] == nil {
		h.sseClients[userID] = make(map[chan *db.Message]bool)
	}
	h.sseClients[userID][ch] = true
}

func (h *StreamHub) unregisterSSE(userID int64, ch chan *db.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if userMap, exists := h.sseClients[userID]; exists {
		delete(userMap, ch)
		if len(userMap) == 0 {
			delete(h.sseClients, userID)
		}
	}
}

// BroadcastToUser dispatches a new message to ALL active WebSocket and SSE connections for that user
func (h *StreamHub) BroadcastToUser(userID int64, msg *db.Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 1. Broadcast to WebSocket connections (Desktop / Linux)
	if wsMap, exists := h.wsClients[userID]; exists && len(wsMap) > 0 {
		data, err := json.Marshal(msg)
		if err == nil {
			for c := range wsMap {
				conn := c
				go func() {
					_ = conn.sendFrame(0x81, data) // 0x81: FIN + Text frame
				}()
			}
		}
	}

	// 2. Broadcast to SSE connections (Mobile / Android)
	if sseMap, exists := h.sseClients[userID]; exists && len(sseMap) > 0 {
		for ch := range sseMap {
			select {
			case ch <- msg:
			default:
				// Channel full, drop or skip to avoid blocking
			}
		}
	}
}
