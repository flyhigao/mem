package handlers

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mem/webservice/internal/auth"
	"mem/webservice/internal/db"
)

//go:embed index.html
var indexHTML []byte

type WebHandler struct {
	db        *db.DB
	sessions  *auth.SessionManager
	uploadDir string
}

func NewWebHandler(database *db.DB, sm *auth.SessionManager, uploadDir string) *WebHandler {
	return &WebHandler{
		db:        database,
		sessions:  sm,
		uploadDir: uploadDir,
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

// HandleWebUploadFiles handles POST /web/api/files
func (h *WebHandler) HandleWebUploadFiles(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSessionUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 35<<20)
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "解析上传表单失败: "+err.Error())
		return
	}

	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		fileHeaders = r.MultipartForm.File["file"]
	}
	if len(fileHeaders) == 0 {
		writeError(w, http.StatusBadRequest, "请选择需要上传的文件")
		return
	}

	for _, fh := range fileHeaders {
		if fh.Size > MaxSingleFileSize {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("文件 [%s] 超过 10MB 上限 (%d 字节)", fh.Filename, fh.Size))
			return
		}
	}

	var uploadedRecords []db.FileRecord
	for _, fh := range fileHeaders {
		if err := EnsureUserStorageQuota(h.db, h.uploadDir, sess.UserID, fh.Size); err != nil {
			writeError(w, http.StatusInternalServerError, "处理存储配额失败: "+err.Error())
			return
		}

		src, err := fh.Open()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "无法打开上传文件: "+err.Error())
			return
		}

		ext := filepath.Ext(fh.Filename)
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		storedName := fmt.Sprintf("u%d_%d_%s%s", sess.UserID, time.Now().UnixNano(), hex.EncodeToString(b), ext)
		dstPath := filepath.Join(h.uploadDir, storedName)

		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			writeError(w, http.StatusInternalServerError, "保存文件失败: "+err.Error())
			return
		}

		copied, err := io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			_ = os.Remove(dstPath)
			writeError(w, http.StatusInternalServerError, "写入文件失败: "+err.Error())
			return
		}

		contentType := fh.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		rec, err := h.db.AddFile(sess.UserID, nil, fh.Filename, storedName, copied, contentType)
		if err != nil {
			_ = os.Remove(dstPath)
			writeError(w, http.StatusInternalServerError, "保存记录失败: "+err.Error())
			return
		}
		uploadedRecords = append(uploadedRecords, *rec)
	}

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    uploadedRecords,
	})
}

// HandleWebGetFiles handles GET /web/api/files
func (h *WebHandler) HandleWebGetFiles(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSessionUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
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

	files, err := h.db.GetFiles(sess.UserID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if files == nil {
		files = []db.FileRecord{}
	}

	totalSize, err := h.db.GetUserTotalFileSize(sess.UserID)
	if err != nil {
		totalSize = 0
	}

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data: FileListResponse{
			Files:     files,
			TotalSize: totalSize,
			MaxSize:   MaxTotalUserQuota,
		},
	})
}

// HandleWebDownloadFile handles GET /web/api/files/{id}/download
func (h *WebHandler) HandleWebDownloadFile(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSessionUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// Expected parts: ["web", "api", "files", "{id}", "download"]
	if len(parts) < 5 {
		writeError(w, http.StatusBadRequest, "Invalid download URL path")
		return
	}

	fileID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid file ID")
		return
	}

	fileRec, err := h.db.GetFileByID(sess.UserID, fileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "File not found")
		return
	}

	serveFileDownload(w, r, h.uploadDir, fileRec)
}

// HandleWebDeleteFile handles DELETE /web/api/files/{id}
func (h *WebHandler) HandleWebDeleteFile(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSessionUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// Expected parts: ["web", "api", "files", "{id}"]
	if len(parts) < 4 {
		writeError(w, http.StatusBadRequest, "Missing ID")
		return
	}

	fileID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	deleted, err := h.db.DeleteFile(sess.UserID, fileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_ = os.Remove(filepath.Join(h.uploadDir, deleted.StoredName))

	writeJSON(w, http.StatusOK, APIResponse{Success: true})
}
