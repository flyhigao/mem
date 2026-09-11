package handlers

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mem/webservice/internal/auth"
	"mem/webservice/internal/db"
)

//go:embed index.html
var indexHTML []byte

type WebHandler struct {
	db       *db.DB
	sessions *auth.SessionManager
}

func NewWebHandler(database *db.DB, sm *auth.SessionManager) *WebHandler {
	return &WebHandler{
		db:       database,
		sessions: sm,
	}
}

func (h *WebHandler) getSessionUser(r *http.Request) (*auth.Session, bool) {
	cookie, err := r.Cookie("mem_session")
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	return h.sessions.GetSession(cookie.Value)
}

func (h *WebHandler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *WebHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	if len(req.Username) < 3 || len(req.Password) < 4 {
		writeError(w, http.StatusBadRequest, "用户名至少3位，密码至少4位")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	user, err := h.db.CreateUser(req.Username, hash)
	if err != nil {
		writeError(w, http.StatusBadRequest, "用户名已存在")
		return
	}

	// Create default token
	rawTok, _ := auth.GenerateAPIToken()
	_, _ = h.db.CreateToken(user.ID, rawTok, "默认Token")

	// Create session
	sessID, _ := h.sessions.CreateSession(user.ID, user.Username)
	http.SetCookie(w, &http.Cookie{
		Name:     "mem_session",
		Value:    sessID,
		Path:     "/",
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		HttpOnly: true,
	})

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]interface{}{"id": user.ID, "username": user.Username},
	})
}

func (h *WebHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	user, err := h.db.GetUserByUsername(req.Username)
	if err != nil || !auth.CheckPasswordHash(req.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	sessID, _ := h.sessions.CreateSession(user.ID, user.Username)
	http.SetCookie(w, &http.Cookie{
		Name:     "mem_session",
		Value:    sessID,
		Path:     "/",
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		HttpOnly: true,
	})

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]interface{}{"id": user.ID, "username": user.Username},
	})
}

func (h *WebHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("mem_session")
	if err == nil {
		h.sessions.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "mem_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *WebHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSessionUser(r)
	if !ok {
		writeJSON(w, http.StatusOK, APIResponse{Success: false, Error: "Not logged in"})
		return
	}
	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]interface{}{"id": sess.UserID, "username": sess.Username},
	})
}

func (h *WebHandler) HandleWebMessages(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSessionUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	switch r.Method {
	case http.MethodGet:
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if val, err := strconv.Atoi(l); err == nil && val > 0 {
				limit = val
			}
		}
		messages, err := h.db.GetMessages(sess.UserID, limit, 0, 0)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if messages == nil {
			messages = []db.Message{}
		}
		writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: messages})

	case http.MethodPost:
		var req PostMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid body")
			return
		}
		req.Content = strings.TrimSpace(req.Content)
		if req.Content == "" {
			writeError(w, http.StatusBadRequest, "Content cannot be empty")
			return
		}
		msg, err := h.db.AddMessage(sess.UserID, nil, req.Content, "web")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: msg})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (h *WebHandler) HandleWebDeleteMessage(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSessionUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		writeError(w, http.StatusBadRequest, "Missing ID")
		return
	}
	msgID, err := strconv.ParseInt(pathParts[3], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := h.db.DeleteMessage(sess.UserID, msgID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, APIResponse{Success: true})
}

func (h *WebHandler) HandleWebTokens(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSessionUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	switch r.Method {
	case http.MethodGet:
		tokens, err := h.db.GetTokensByUserID(sess.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tokens == nil {
			tokens = []db.Token{}
		}
		writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: tokens})

	case http.MethodPost:
		type CreateTokenReq struct {
			Name string `json:"name"`
		}
		var req CreateTokenReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			req.Name = "未命名设备"
		}
		rawTok, err := auth.GenerateAPIToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to generate token")
			return
		}
		tok, err := h.db.CreateToken(sess.UserID, rawTok, req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, APIResponse{Success: true, Data: tok})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (h *WebHandler) HandleWebDeleteToken(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSessionUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		writeError(w, http.StatusBadRequest, "Missing ID")
		return
	}
	tokID, err := strconv.ParseInt(pathParts[3], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := h.db.DeleteToken(sess.UserID, tokID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, APIResponse{Success: true})
}
