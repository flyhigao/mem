package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"mem/webservice/internal/db"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
)

type Session struct {
	UserID    int64
	Username  string
	ExpiresAt time.Time
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]Session),
	}
	// Cleanup routine every hour
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			sm.mu.Lock()
			now := time.Now()
			for k, v := range sm.sessions {
				if now.After(v.ExpiresAt) {
					delete(sm.sessions, k)
				}
			}
			sm.mu.Unlock()
		}
	}()
	return sm
}

func (sm *SessionManager) CreateSession(userID int64, username string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	sessionID := hex.EncodeToString(b)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.sessions[sessionID] = Session{
		UserID:    userID,
		Username:  username,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	return sessionID, nil
}

func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	s, ok := sm.sessions[sessionID]
	if !ok || time.Now().After(s.ExpiresAt) {
		return nil, false
	}
	return &s, true
}

func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateAPIToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "mem_" + hex.EncodeToString(b), nil
}

// ExtractTokenFromRequest looks for Token in Bearer header, X-API-Token header, or ?token= query param
func ExtractTokenFromRequest(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	if headerToken := r.Header.Get("X-API-Token"); headerToken != "" {
		return strings.TrimSpace(headerToken)
	}
	if headerToken := r.Header.Get("X-Token"); headerToken != "" {
		return strings.TrimSpace(headerToken)
	}
	if queryToken := r.URL.Query().Get("token"); queryToken != "" {
		return strings.TrimSpace(queryToken)
	}
	return ""
}

// AuthenticateAPI validates the token against the database and returns the UserID and TokenID
func AuthenticateAPI(database *db.DB, r *http.Request) (int64, *int64, error) {
	tokenStr := ExtractTokenFromRequest(r)
	if tokenStr == "" {
		return 0, nil, ErrUnauthorized
	}

	tok, err := database.GetToken(tokenStr)
	if err != nil {
		return 0, nil, ErrUnauthorized
	}

	_ = database.UpdateTokenLastUsed(tokenStr)
	return tok.UserID, &tok.ID, nil
}
