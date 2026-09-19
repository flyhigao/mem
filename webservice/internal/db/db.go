package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Token struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	Token      string     `json:"token"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type Message struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	TokenID   *int64    `json:"token_id,omitempty"`
	TokenName string    `json:"token_name,omitempty"`
	Content   string    `json:"content"`
	Source    string    `json:"source,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type FileRecord struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	TokenID     *int64    `json:"token_id,omitempty"`
	TokenName   string    `json:"token_name,omitempty"`
	FileName    string    `json:"filename"`
	StoredName  string    `json:"-"`
	FileSize    int64     `json:"file_size"`
	ContentType string    `json:"content_type"`
	CreatedAt   time.Time `json:"created_at"`
}

func InitDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	conn.SetMaxOpenConns(1) // SQLite file handles single-writer well with max 1 open connection or WAL mode

	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL COLLATE NOCASE,
		password_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_used_at DATETIME,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token_id INTEGER,
		content TEXT NOT NULL,
		source TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY(token_id) REFERENCES tokens(id) ON DELETE SET NULL
	);

	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token_id INTEGER,
		filename TEXT NOT NULL,
		stored_name TEXT NOT NULL,
		file_size INTEGER NOT NULL,
		content_type TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY(token_id) REFERENCES tokens(id) ON DELETE SET NULL
	);

	CREATE INDEX IF NOT EXISTS idx_messages_user_id ON messages(user_id, id DESC);
	CREATE INDEX IF NOT EXISTS idx_tokens_token ON tokens(token);
	CREATE INDEX IF NOT EXISTS idx_files_user_id ON files(user_id, id DESC);
	`

	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	return &DB{conn}, nil
}

func (d *DB) CreateUser(username, passwordHash string) (*User, error) {
	res, err := d.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, passwordHash)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return d.GetUserByID(id)
}

func (d *DB) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := d.QueryRow("SELECT id, username, password_hash, created_at FROM users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (d *DB) GetUserByID(id int64) (*User, error) {
	u := &User{}
	err := d.QueryRow("SELECT id, username, password_hash, created_at FROM users WHERE id = ?", id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (d *DB) CreateToken(userID int64, token, name string) (*Token, error) {
	res, err := d.Exec("INSERT INTO tokens (user_id, token, name) VALUES (?, ?, ?)", userID, token, name)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Token{
		ID:        id,
		UserID:    userID,
		Token:     token,
		Name:      name,
		CreatedAt: time.Now(),
	}, nil
}

func (d *DB) GetTokensByUserID(userID int64) ([]Token, error) {
	rows, err := d.Query("SELECT id, user_id, token, name, created_at, last_used_at FROM tokens WHERE user_id = ? ORDER BY id DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Token
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.UserID, &t.Token, &t.Name, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (d *DB) GetToken(tokenStr string) (*Token, error) {
	t := &Token{}
	err := d.QueryRow("SELECT id, user_id, token, name, created_at, last_used_at FROM tokens WHERE token = ?", tokenStr).
		Scan(&t.ID, &t.UserID, &t.Token, &t.Name, &t.CreatedAt, &t.LastUsedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (d *DB) DeleteToken(userID, tokenID int64) error {
	_, err := d.Exec("DELETE FROM tokens WHERE id = ? AND user_id = ?", tokenID, userID)
	return err
}

func (d *DB) UpdateTokenLastUsed(tokenStr string) error {
	_, err := d.Exec("UPDATE tokens SET last_used_at = CURRENT_TIMESTAMP WHERE token = ?", tokenStr)
	return err
}

func (d *DB) AddMessage(userID int64, tokenID *int64, content, source string) (*Message, error) {
	res, err := d.Exec("INSERT INTO messages (user_id, token_id, content, source) VALUES (?, ?, ?, ?)",
		userID, tokenID, content, source)
	if err != nil {
		return nil, err
	}
	msgID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Maintain latest 500 records for this user
	_, _ = d.Exec(`
		DELETE FROM messages 
		WHERE user_id = ? 
		  AND id NOT IN (
			SELECT id FROM messages WHERE user_id = ? ORDER BY id DESC LIMIT 500
		  )
	`, userID, userID)

	return d.GetMessageByID(userID, msgID)
}

func (d *DB) GetMessageByID(userID, msgID int64) (*Message, error) {
	m := &Message{}
	var tokenName sql.NullString
	err := d.QueryRow(`
		SELECT m.id, m.user_id, m.token_id, COALESCE(t.name, ''), m.content, m.source, m.created_at
		FROM messages m
		LEFT JOIN tokens t ON m.token_id = t.id
		WHERE m.user_id = ? AND m.id = ?
	`, userID, msgID).Scan(&m.ID, &m.UserID, &m.TokenID, &tokenName, &m.Content, &m.Source, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	if tokenName.Valid {
		m.TokenName = tokenName.String
	}
	return m, nil
}

func (d *DB) GetLatestMessage(userID int64) (*Message, error) {
	m := &Message{}
	var tokenName sql.NullString
	err := d.QueryRow(`
		SELECT m.id, m.user_id, m.token_id, COALESCE(t.name, ''), m.content, m.source, m.created_at
		FROM messages m
		LEFT JOIN tokens t ON m.token_id = t.id
		WHERE m.user_id = ?
		ORDER BY m.id DESC LIMIT 1
	`, userID).Scan(&m.ID, &m.UserID, &m.TokenID, &tokenName, &m.Content, &m.Source, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	if tokenName.Valid {
		m.TokenName = tokenName.String
	}
	return m, nil
}

func (d *DB) GetMessages(userID int64, limit, offset int, beforeID int64) ([]Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `
		SELECT m.id, m.user_id, m.token_id, COALESCE(t.name, ''), m.content, m.source, m.created_at
		FROM messages m
		LEFT JOIN tokens t ON m.token_id = t.id
		WHERE m.user_id = ?
	`
	args := []interface{}{userID}

	if beforeID > 0 {
		query += " AND m.id < ?"
		args = append(args, beforeID)
	}

	query += " ORDER BY m.id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Message
	for rows.Next() {
		var m Message
		var tokenName sql.NullString
		if err := rows.Scan(&m.ID, &m.UserID, &m.TokenID, &tokenName, &m.Content, &m.Source, &m.CreatedAt); err != nil {
			return nil, err
		}
		if tokenName.Valid {
			m.TokenName = tokenName.String
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (d *DB) DeleteMessage(userID, msgID int64) error {
	_, err := d.Exec("DELETE FROM messages WHERE id = ? AND user_id = ?", msgID, userID)
	return err
}

func (d *DB) ClearMessages(userID int64) error {
	_, err := d.Exec("DELETE FROM messages WHERE user_id = ?", userID)
	return err
}

func (d *DB) GetUserTotalFileSize(userID int64) (int64, error) {
	var total sql.NullInt64
	err := d.QueryRow("SELECT SUM(file_size) FROM files WHERE user_id = ?", userID).Scan(&total)
	if err != nil {
		return 0, err
	}
	if total.Valid {
		return total.Int64, nil
	}
	return 0, nil
}

func (d *DB) AddFile(userID int64, tokenID *int64, filename, storedName string, fileSize int64, contentType string) (*FileRecord, error) {
	res, err := d.Exec("INSERT INTO files (user_id, token_id, filename, stored_name, file_size, content_type) VALUES (?, ?, ?, ?, ?, ?)",
		userID, tokenID, filename, storedName, fileSize, contentType)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return d.GetFileByID(userID, id)
}

func (d *DB) GetFileByID(userID, fileID int64) (*FileRecord, error) {
	f := &FileRecord{}
	var tokenName sql.NullString
	err := d.QueryRow(`
		SELECT f.id, f.user_id, f.token_id, COALESCE(t.name, ''), f.filename, f.stored_name, f.file_size, f.content_type, f.created_at
		FROM files f
		LEFT JOIN tokens t ON f.token_id = t.id
		WHERE f.user_id = ? AND f.id = ?
	`, userID, fileID).Scan(&f.ID, &f.UserID, &f.TokenID, &tokenName, &f.FileName, &f.StoredName, &f.FileSize, &f.ContentType, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	if tokenName.Valid {
		f.TokenName = tokenName.String
	}
	return f, nil
}

func (d *DB) GetOldestFiles(userID int64) ([]FileRecord, error) {
	rows, err := d.Query("SELECT id, user_id, stored_name, file_size FROM files WHERE user_id = ? ORDER BY id ASC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []FileRecord
	for rows.Next() {
		var f FileRecord
		if err := rows.Scan(&f.ID, &f.UserID, &f.StoredName, &f.FileSize); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

func (d *DB) DeleteFile(userID, fileID int64) (*FileRecord, error) {
	f, err := d.GetFileByID(userID, fileID)
	if err != nil {
		return nil, err
	}
	_, err = d.Exec("DELETE FROM files WHERE id = ? AND user_id = ?", fileID, userID)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (d *DB) GetFiles(userID int64, limit, offset int) ([]FileRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `
		SELECT f.id, f.user_id, f.token_id, COALESCE(t.name, ''), f.filename, f.stored_name, f.file_size, f.content_type, f.created_at
		FROM files f
		LEFT JOIN tokens t ON f.token_id = t.id
		WHERE f.user_id = ?
		ORDER BY f.id DESC LIMIT ? OFFSET ?
	`
	rows, err := d.Query(query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []FileRecord
	for rows.Next() {
		var f FileRecord
		var tokenName sql.NullString
		if err := rows.Scan(&f.ID, &f.UserID, &f.TokenID, &tokenName, &f.FileName, &f.StoredName, &f.FileSize, &f.ContentType, &f.CreatedAt); err != nil {
			return nil, err
		}
		if tokenName.Valid {
			f.TokenName = tokenName.String
		}
		list = append(list, f)
	}
	return list, rows.Err()
}
