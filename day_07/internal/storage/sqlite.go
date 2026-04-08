package storage

import (
	"database/sql"
	"errors"
	"fmt"

	"ai-agent-cli/internal/deepseek"

	_ "modernc.org/sqlite"
)

const (
	dbFile = "chat_history.db"
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
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_agent_name ON messages (agent_name);
	`
	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	return nil
}

// SaveMessage stores a message in the database.
func SaveMessage(db *sql.DB, agentName, role, content string) error {
	if db == nil {
		return errors.New("database connection is nil")
	}
	stmt, err := db.Prepare("INSERT INTO messages (agent_name, role, content) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(agentName, role, content)
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
	rows, err := db.Query("SELECT role, content FROM messages WHERE agent_name = ? ORDER BY created_at ASC", agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []deepseek.Message
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		messages = append(messages, deepseek.Message{Role: role, Content: content})
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
	// Keep only the latest 'limit' messages per agent
	_, err := db.Exec(`
		DELETE FROM messages WHERE id NOT IN (
			SELECT id FROM messages WHERE agent_name = ? ORDER BY created_at DESC LIMIT ?
		) AND agent_name = ?
	`, agentName, limit, agentName)
	if err != nil {
		return fmt.Errorf("failed to prune messages: %w", err)
	}
	return nil
}
