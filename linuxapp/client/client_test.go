package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMemClientMethods(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"time":   time.Now(),
		})
	})

	mux.HandleFunc("/api/v1/messages/latest", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid_token" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Invalid token",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		msg := Message{
			ID:        100,
			UserID:    1,
			Content:   "Hello Latest Test",
			Source:    "UnitTester",
			CreatedAt: time.Now(),
		}
		rawMsg, _ := json.Marshal(msg)
		_ = json.NewEncoder(w).Encode(APIResponse{
			Success: true,
			Data:    rawMsg,
		})
	})

	mux.HandleFunc("/api/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid_token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Method == http.MethodPost {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			msg := Message{
				ID:        101,
				UserID:    1,
				Content:   body["content"],
				Source:    body["source"],
				CreatedAt: time.Now(),
			}
			rawMsg, _ := json.Marshal(msg)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(APIResponse{
				Success: true,
				Data:    rawMsg,
			})
			return
		}

		if r.Method == http.MethodGet {
			list := []Message{
				{ID: 101, Content: "Msg 1", Source: "Tester"},
				{ID: 100, Content: "Msg 0", Source: "Tester"},
			}
			rawList, _ := json.Marshal(list)
			_ = json.NewEncoder(w).Encode(APIResponse{
				Success: true,
				Data:    rawList,
			})
			return
		}
	})

	mux.HandleFunc("/api/v1/messages/101", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			_ = json.NewEncoder(w).Encode(APIResponse{
				Success: true,
				Data:    json.RawMessage(`"deleted"`),
			})
			return
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := Config{
		ServerURL: ts.URL,
		Token:     "valid_token",
		Source:    "TestLinux",
	}
	c := NewMemClient(cfg)

	// 1. Test Ping
	dur, err := c.Ping()
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if dur <= 0 {
		t.Errorf("Expected positive duration")
	}

	// 2. Test FetchLatestMessage
	msg, err := c.FetchLatestMessage()
	if err != nil {
		t.Fatalf("FetchLatestMessage failed: %v", err)
	}
	if msg.Content != "Hello Latest Test" {
		t.Errorf("Unexpected content: %s", msg.Content)
	}

	// 3. Test PostMessage
	postMsg, err := c.PostMessage("New Post Content", "")
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}
	if postMsg.Content != "New Post Content" || postMsg.Source != "TestLinux" {
		t.Errorf("Unexpected post response: %+v", postMsg)
	}

	// 4. Test FetchMessages
	msgs, err := c.FetchMessages(10, 0, 0)
	if err != nil {
		t.Fatalf("FetchMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(msgs))
	}

	// 5. Test DeleteMessage
	err = c.DeleteMessage(101)
	if err != nil {
		t.Fatalf("DeleteMessage failed: %v", err)
	}

	// 6. Test invalid token
	cBad := NewMemClient(Config{ServerURL: ts.URL, Token: "wrong_token"})
	_, err = cBad.FetchLatestMessage()
	if err == nil {
		t.Errorf("Expected error with wrong token, got nil")
	}
}
