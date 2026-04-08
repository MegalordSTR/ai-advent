package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"ai-agent-cli/internal/deepseek"

	_ "modernc.org/sqlite"
)

// OpenDB opens SQLite database connection.
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	return db, nil
}

// InitDB creates messages table if it doesn't exist.
func InitDB(db *sql.DB) error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_name TEXT NOT NULL,
		role TEXT NOT NULL CHECK(role IN ('user', 'assistant', 'system')),
		content TEXT NOT NULL,
		token_count INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_agent_name ON messages (agent_name);
	`
	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Add token_count column if it doesn't exist (for existing databases)
	// SQLite doesn't support ADD COLUMN IF NOT EXISTS, so we check via pragma
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name='token_count'").Scan(&count)
	if err != nil {
		// If pragma fails, continue without column addition
		log.Printf("warning: failed to check for token_count column: %v", err)
		return nil
	}
	if count == 0 {
		_, err = db.Exec("ALTER TABLE messages ADD COLUMN token_count INTEGER")
		if err != nil {
			log.Printf("warning: failed to add token_count column: %v", err)
		}
	}

	return nil
}

// SaveMessage stores a message in the database.
func SaveMessage(db *sql.DB, agentName, role, content string, tokenCount int) error {
	if db == nil {
		return errors.New("database connection is nil")
	}
	stmt, err := db.Prepare("INSERT INTO messages (agent_name, role, content, token_count) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			log.Printf("failed to close statement: %v", err)
		}
	}()

	_, err = stmt.Exec(agentName, role, content, tokenCount)
	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}
	return nil
}

// LoadMessages retrieves all messages for a given agent, ordered by creation time.
func LoadMessages(db *sql.DB, agentName string) ([]deepseek.Message, error) {
	if db == nil {
		return nil, errors.New("database connection is nil")
	}
	rows, err := db.Query("SELECT role, content, token_count FROM messages WHERE agent_name = ? ORDER BY created_at ASC", agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}()

	var messages []deepseek.Message
	for rows.Next() {
		var role, content string
		var tokenCount sql.NullInt64 // token_count may be NULL
		if err := rows.Scan(&role, &content, &tokenCount); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		msg := deepseek.Message{Role: role, Content: content}
		if tokenCount.Valid {
			msg.TokenCount = int(tokenCount.Int64)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return messages, nil
}

// DeleteMessages deletes all messages for a given agent (optional cleanup).
func DeleteMessages(db *sql.DB, agentName string) error {
	if db == nil {
		return errors.New("database connection is nil")
	}
	_, err := db.Exec("DELETE FROM messages WHERE agent_name = ?", agentName)
	if err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}
	return nil
}

// PruneMessages removes old messages beyond the limit for an agent.
func PruneMessages(db *sql.DB, agentName string, limit int) error {
	if db == nil {
		return errors.New("database connection is nil")
	}
	// Keep only the latest 'limit' messages per agent (order by id to guarantee deterministic order)
	_, err := db.Exec(`
		DELETE FROM messages WHERE id NOT IN (
			SELECT id FROM messages WHERE agent_name = ? ORDER BY id DESC LIMIT ?
		) AND agent_name = ?
	`, agentName, limit, agentName)
	if err != nil {
		return fmt.Errorf("failed to prune messages: %w", err)
	}
	return nil
}
