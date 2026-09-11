package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mem/webservice/internal/auth"
	"mem/webservice/internal/db"
)

type APIHandler struct {
	db *db.DB
}

func NewAPIHandler(database *db.DB) *APIHandler {
	return &APIHandler{db: database}
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, APIResponse{
		Success: false,
		Error:   msg,
	})
}

func (h *APIHandler) Ping(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"time":   time.Now(),
	})
}

type PostMessageRequest struct {
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
}

// HandlePostMessage handles POST /api/v1/messages
func (h *APIHandler) HandlePostMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID, tokenID, err := auth.AuthenticateAPI(h.db, r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid or missing token")
		return
	}

	var req PostMessageRequest
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
	} else {
		// Support form-urlencoded / text/plain
		_ = r.ParseForm()
		req.Content = r.FormValue("content")
		req.Source = r.FormValue("source")
		if req.Content == "" {
			// Try reading entire body as raw text
			buf := make([]byte, 1024*1024) // 1MB max
			n, _ := r.Body.Read(buf)
			req.Content = string(buf[:n])
		}
	}

	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "Content cannot be empty")
		return
	}

	if req.Source == "" {
		req.Source = "api"
	}

	msg, err := h.db.AddMessage(userID, tokenID, req.Content, req.Source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to store message: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, APIResponse{
		Success: true,
		Data:    msg,
	})
}

// HandleGetLatestMessage handles GET /api/v1/messages/latest
func (h *APIHandler) HandleGetLatestMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID, _, err := auth.AuthenticateAPI(h.db, r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid or missing token")
		return
	}

	msg, err := h.db.GetLatestMessage(userID)
	if err != nil {
		writeJSON(w, http.StatusOK, APIResponse{
			Success: true,
			Data:    nil,
		})
		return
	}

	// Support raw text response if requested (e.g., ?format=text or Accept: text/plain)
	if r.URL.Query().Get("format") == "text" || strings.Contains(r.Header.Get("Accept"), "text/plain") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(msg.Content))
		return
	}

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    msg,
	})
}

// HandleGetMessages handles GET /api/v1/messages (supports list, pagination)
func (h *APIHandler) HandleGetMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID, _, err := auth.AuthenticateAPI(h.db, r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid or missing token")
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	var beforeID int64 = 0
	if b := r.URL.Query().Get("before_id"); b != "" {
		if val, err := strconv.ParseInt(b, 10, 64); err == nil {
			beforeID = val
		}
	}

	messages, err := h.db.GetMessages(userID, limit, offset, beforeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to retrieve messages: "+err.Error())
		return
	}

	if messages == nil {
		messages = []db.Message{}
	}

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    messages,
	})
}

// HandleDeleteMessage handles DELETE /api/v1/messages/{id}
func (h *APIHandler) HandleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID, _, err := auth.AuthenticateAPI(h.db, r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid or missing token")
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		writeError(w, http.StatusBadRequest, "Missing message ID")
		return
	}

	msgID, err := strconv.ParseInt(pathParts[3], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	if err := h.db.DeleteMessage(userID, msgID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete message: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    "Message deleted",
	})
}
