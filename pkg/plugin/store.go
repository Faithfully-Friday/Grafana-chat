package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Conversation is a single chat thread shown in the UI sidebar.
type Conversation struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Message is a single chat message persisted in SQLite.
type Message struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// Store persists chat history in an embedded SQLite database.
type Store struct {
	db *sql.DB
}

// OpenStore opens (and creates if needed) the SQLite database inside the
// plugin data directory and applies migrations.
func OpenStore() (*Store, error) {
	dir := os.Getenv("GF_PLUGIN_DATA_DIR")
	if dir == "" {
		dir = "data"
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "chat.db"))
	if err != nil {
		return nil, err
	}
	// Single-writer embedded DB: one connection avoids SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			a2a_context_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// ListConversations returns all conversations, most recently active first.
func (s *Store) ListConversations(ctx context.Context) ([]Conversation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, updated_at FROM conversations ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateConversation inserts a new conversation with the default title.
func (s *Store) CreateConversation(ctx context.Context) (*Conversation, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO conversations (title) VALUES ('New chat')`)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Conversation{ID: id, Title: "New chat", UpdatedAt: time.Now().UTC()}, nil
}

// DeleteConversation removes a conversation and its messages.
func (s *Store) DeleteConversation(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM conversations WHERE id = ?`, id)
	return err
}

// ListMessages returns the messages of a conversation in chronological order.
func (s *Store) ListMessages(ctx context.Context, conversationID int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, role, content, created_at FROM messages WHERE conversation_id = ? ORDER BY id ASC`,
		conversationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMessage appends a message and bumps the conversation's updated_at.
func (s *Store) AddMessage(ctx context.Context, conversationID int64, role, content string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO messages (conversation_id, role, content) VALUES (?, ?, ?)`,
		conversationID, role, content); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE conversations SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, conversationID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetTitle updates the conversation title (used for auto-titling from the
// first user message).
func (s *Store) SetTitle(ctx context.Context, id int64, title string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE conversations SET title = ? WHERE id = ? AND title = 'New chat'`, title, id)
	return err
}

// ContextID returns the A2A contextId bound to a conversation ("" if none).
func (s *Store) ContextID(ctx context.Context, id int64) (string, error) {
	var cid sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT a2a_context_id FROM conversations WHERE id = ?`, id).Scan(&cid)
	if err != nil {
		return "", err
	}
	return cid.String, nil
}

// SetContextID binds an A2A contextId to a conversation.
func (s *Store) SetContextID(ctx context.Context, id int64, contextID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE conversations SET a2a_context_id = ? WHERE id = ?`, contextID, id)
	return err
}
