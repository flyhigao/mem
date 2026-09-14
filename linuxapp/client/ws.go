package client

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

type WSClient struct {
	conn       net.Conn
	msgChan    chan *Message
	closeChan  chan struct{}
	closeOnce  sync.Once
	sendMu     sync.Mutex
	pingTicker *time.Ticker
}

// ConnectWS establishes a WebSocket connection to the relay server
func ConnectWS(serverURL string, token string) (*WSClient, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}

	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "https" || u.Scheme == "wss" {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	hostName := strings.Split(host, ":")[0]

	var conn net.Conn
	if u.Scheme == "https" || u.Scheme == "wss" {
		tlsConfig := &tls.Config{
			ServerName: hostName,
		}
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", host, tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", host, 5*time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to dial WebSocket host: %w", err)
	}

	// Generate Sec-WebSocket-Key
	rawKey := make([]byte, 16)
	_, _ = rand.Read(rawKey)
	key := base64.StdEncoding.EncodeToString(rawKey)

	wsPath := strings.TrimRight(u.Path, "/") + "/api/v1/ws"
	if token != "" {
		wsPath += "?token=" + url.QueryEscape(token)
	}

	handshakeReq := fmt.Sprintf(
		"GET %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: %s\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"Authorization: Bearer %s\r\n\r\n",
		wsPath, u.Host, key, token,
	)

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(handshakeReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to write handshake request: %w", err)
	}

	// Read handshake response
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read handshake response: %w", err)
	}

	respStr := string(buf[:n])
	if !strings.Contains(respStr, "101 Switching Protocols") {
		conn.Close()
		return nil, fmt.Errorf("server rejected WebSocket upgrade: %s", strings.Split(respStr, "\r\n")[0])
	}

	// Clear deadline for long-lived connection
	_ = conn.SetDeadline(time.Time{})

	client := &WSClient{
		conn:       conn,
		msgChan:    make(chan *Message, 32),
		closeChan:  make(chan struct{}),
		pingTicker: time.NewTicker(25 * time.Second),
	}

	go client.readLoop()
	go client.pingLoop()

	return client, nil
}

func (c *WSClient) Messages() <-chan *Message {
	return c.msgChan
}

func (c *WSClient) Close() {
	c.closeOnce.Do(func() {
		c.pingTicker.Stop()
		close(c.closeChan)
		_ = c.conn.Close()
	})
}

func (c *WSClient) sendFrame(opcode byte, payload []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	// Client-to-server frames must be masked (RFC 6455 5.3)
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)

	l := len(payload)
	frame := []byte{0x80 | (opcode & 0x0F)} // FIN=1 + opcode

	if l < 126 {
		frame = append(frame, 0x80|byte(l)) // Mask=1 + len
	} else if l < 65536 {
		frame = append(frame, 0x80|126, byte(l>>8), byte(l))
	} else {
		frame = append(frame, 0x80|127, 0, 0, 0, 0, byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
	}

	frame = append(frame, mask...)

	maskedPayload := make([]byte, l)
	for i := 0; i < l; i++ {
		maskedPayload[i] = payload[i] ^ mask[i%4]
	}
	frame = append(frame, maskedPayload...)

	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := c.conn.Write(frame)
	return err
}

func (c *WSClient) pingLoop() {
	for {
		select {
		case <-c.closeChan:
			return
		case <-c.pingTicker.C:
			// Opcode 0x9 = Ping
			if err := c.sendFrame(0x9, []byte("ping")); err != nil {
				c.Close()
				return
			}
		}
	}
}

func (c *WSClient) readLoop() {
	defer c.Close()

	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(c.conn, header); err != nil {
			return
		}

		opcode := header[0] & 0x0F
		masked := (header[1] & 0x80) != 0
		length := int(header[1] & 0x7F)

		if length == 126 {
			ext := make([]byte, 2)
			if _, err := io.ReadFull(c.conn, ext); err != nil {
				return
			}
			length = int(ext[0])<<8 | int(ext[1])
		} else if length == 127 {
			ext := make([]byte, 8)
			if _, err := io.ReadFull(c.conn, ext); err != nil {
				return
			}
			length = int(ext[4])<<24 | int(ext[5])<<16 | int(ext[6])<<8 | int(ext[7])
		}

		var maskKey []byte
		if masked {
			maskKey = make([]byte, 4)
			if _, err := io.ReadFull(c.conn, maskKey); err != nil {
				return
			}
		}

		payload := make([]byte, length)
		if length > 0 {
			if _, err := io.ReadFull(c.conn, payload); err != nil {
				return
			}
			if masked {
				for i := 0; i < length; i++ {
					payload[i] ^= maskKey[i%4]
				}
			}
		}

		switch opcode {
		case 0x8: // Close
			return
		case 0x9: // Ping -> Pong
			_ = c.sendFrame(0xA, payload)
		case 0x1: // Text
			var msg Message
			if err := json.Unmarshal(payload, &msg); err == nil && msg.ID > 0 {
				select {
				case c.msgChan <- &msg:
				default:
				}
			}
		}
	}
}
