package handlers

import (
	"crypto/rand"
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

type APIHandler struct {
	db        *db.DB
	wsHub     *StreamHub
	uploadDir string
}

func NewAPIHandler(database *db.DB, wsHub *StreamHub, uploadDir string) *APIHandler {
	return &APIHandler{
		db:        database,
		wsHub:     wsHub,
		uploadDir: uploadDir,
	}
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

	if h.wsHub != nil {
		h.wsHub.BroadcastToUser(userID, msg)
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

// HandleUploadFiles handles POST /api/v1/files
func (h *APIHandler) HandleUploadFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID, tokenID, err := auth.AuthenticateAPI(h.db, r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid or missing token")
		return
	}

	// Limit request body to 35MB max for multipart form
	r.Body = http.MaxBytesReader(w, r.Body, 35<<20)
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "解析上传表单失败或文件过大: "+err.Error())
		return
	}

	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		fileHeaders = r.MultipartForm.File["file"]
	}
	if len(fileHeaders) == 0 {
		writeError(w, http.StatusBadRequest, "未提供上传文件 (表单字段需为 files 或 file)")
		return
	}

	// Validate individual file sizes
	for _, fh := range fileHeaders {
		if fh.Size > MaxSingleFileSize {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("文件 [%s] 大小超过 10MB 限制 (%d 字节)", fh.Filename, fh.Size))
			return
		}
	}

	var uploadedRecords []db.FileRecord
	for _, fh := range fileHeaders {
		// Enforce storage quota: delete oldest files if needed
		if err := EnsureUserStorageQuota(h.db, h.uploadDir, userID, fh.Size); err != nil {
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
		storedName := fmt.Sprintf("u%d_%d_%s%s", userID, time.Now().UnixNano(), hex.EncodeToString(b), ext)
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

		rec, err := h.db.AddFile(userID, tokenID, fh.Filename, storedName, copied, contentType)
		if err != nil {
			_ = os.Remove(dstPath)
			writeError(w, http.StatusInternalServerError, "记录文件信息失败: "+err.Error())
			return
		}
		uploadedRecords = append(uploadedRecords, *rec)
	}

	writeJSON(w, http.StatusCreated, APIResponse{
		Success: true,
		Data:    uploadedRecords,
	})
}

// HandleGetFiles handles GET /api/v1/files
func (h *APIHandler) HandleGetFiles(w http.ResponseWriter, r *http.Request) {
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

	files, err := h.db.GetFiles(userID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取文件列表失败: "+err.Error())
		return
	}
	if files == nil {
		files = []db.FileRecord{}
	}

	totalSize, err := h.db.GetUserTotalFileSize(userID)
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

// HandleDownloadFile handles GET /api/v1/files/{id}/download
func (h *APIHandler) HandleDownloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID, _, err := auth.AuthenticateAPI(h.db, r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid or missing token")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// Expected parts: ["api", "v1", "files", "{id}", "download"]
	if len(parts) < 5 {
		writeError(w, http.StatusBadRequest, "Invalid download URL path")
		return
	}

	fileID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid file ID")
		return
	}

	fileRec, err := h.db.GetFileByID(userID, fileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "File not found")
		return
	}

	serveFileDownload(w, r, h.uploadDir, fileRec)
}

// HandleDeleteFile handles DELETE /api/v1/files/{id}
func (h *APIHandler) HandleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userID, _, err := auth.AuthenticateAPI(h.db, r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid or missing token")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// Expected parts: ["api", "v1", "files", "{id}"]
	if len(parts) < 4 {
		writeError(w, http.StatusBadRequest, "Missing file ID")
		return
	}

	fileID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid file ID")
		return
	}

	deleted, err := h.db.DeleteFile(userID, fileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete file from DB: "+err.Error())
		return
	}

	_ = os.Remove(filepath.Join(h.uploadDir, deleted.StoredName))

	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    "File deleted",
	})
}
