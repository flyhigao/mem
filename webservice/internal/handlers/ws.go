package handlers

import (
"crypto/sha1"
"encoding/base64"
"encoding/json"
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

type WSHub struct {
db      *db.DB
mu      sync.RWMutex
clients map[int64]map[*WSClientConn]bool
}

func NewWSHub(database *db.DB) *WSHub {
return &WSHub{
db:      database,
clients: make(map[int64]map[*WSClientConn]bool),
}
}

func computeAccept(key string) string {
h := sha1.New()
h.Write([]byte(strings.TrimSpace(key) + wsGUID))
return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// HandleWS upgrades HTTP connection to WebSocket (RFC 6455)
func (h *WSHub) HandleWS(w http.ResponseWriter, r *http.Request) {
// Authenticate client
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

h.register(clientConn)
defer h.unregister(clientConn)

log.Printf("🔌 WebSocket client connected for User %d (%s)", userID, conn.RemoteAddr())

// Read loop (keep connection open and respond to Ping/Close)
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

// Opcode handling
if opcode == 0x8 {
// Close frame
break
} else if opcode == 0x9 {
// Ping frame -> send Pong (0x8A)
clientConn.sendFrame(0x8A, payload)
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

func (h *WSHub) register(c *WSClientConn) {
h.mu.Lock()
defer h.mu.Unlock()

if h.clients[c.userID] == nil {
h.clients[c.userID] = make(map[*WSClientConn]bool)
}
h.clients[c.userID][c] = true
}

func (h *WSHub) unregister(c *WSClientConn) {
h.mu.Lock()
defer h.mu.Unlock()

if userMap, exists := h.clients[c.userID]; exists {
delete(userMap, c)
if len(userMap) == 0 {
delete(h.clients, c.userID)
}
}
_ = c.conn.Close()
}

// BroadcastToUser sends a new message to all active WebSocket connections belonging to userID
func (h *WSHub) BroadcastToUser(userID int64, msg *db.Message) {
h.mu.RLock()
userMap, exists := h.clients[userID]
if !exists || len(userMap) == 0 {
h.mu.RUnlock()
return
}

conns := make([]*WSClientConn, 0, len(userMap))
for c := range userMap {
conns = append(conns, c)
}
h.mu.RUnlock()

data, err := json.Marshal(msg)
if err != nil {
return
}

for _, c := range conns {
go func(conn *WSClientConn) {
_ = conn.sendFrame(0x81, data) // 0x81: FIN + Text frame
}(c)
}
}
