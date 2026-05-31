package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func Connect(dsn string) error {
	var err error
	DB, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err = DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)

	log.Println("[DB] Connected to PostgreSQL")
	return nil
}

func Migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id          VARCHAR(36) PRIMARY KEY,
		username    VARCHAR(64) UNIQUE NOT NULL,
		password    VARCHAR(256) NOT NULL,
		created_at  TIMESTAMP DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS nodes (
		id            VARCHAR(36) PRIMARY KEY,
		user_id       VARCHAR(36) NOT NULL REFERENCES users(id),
		name          VARCHAR(128) NOT NULL,
		os            VARCHAR(32) NOT NULL,
		arch          VARCHAR(32) NOT NULL DEFAULT '',
		status        VARCHAR(16) NOT NULL DEFAULT 'offline',
		version       VARCHAR(32) NOT NULL DEFAULT '',
		ip            VARCHAR(45) NOT NULL DEFAULT '',
		max_sessions  INT NOT NULL DEFAULT 3,
		last_seen     TIMESTAMP DEFAULT NOW(),
		created_at    TIMESTAMP DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id            VARCHAR(36) PRIMARY KEY,
		user_id       VARCHAR(36) NOT NULL REFERENCES users(id),
		node_id       VARCHAR(36) NOT NULL REFERENCES nodes(id),
		agent_id      VARCHAR(36),
		status        VARCHAR(16) NOT NULL DEFAULT 'pending',
		prompt        TEXT,
		workspace     TEXT NOT NULL,
		output_log    TEXT NOT NULL DEFAULT '',
		error_log     TEXT NOT NULL DEFAULT '',
		pid           INT NOT NULL DEFAULT 0,
		created_at    TIMESTAMP DEFAULT NOW(),
		updated_at    TIMESTAMP DEFAULT NOW(),
		completed_at  TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agents (
		id            VARCHAR(36) PRIMARY KEY,
		node_id       VARCHAR(36) NOT NULL REFERENCES nodes(id),
		name          VARCHAR(64) NOT NULL,
		command       VARCHAR(256) NOT NULL,
		version       VARCHAR(32) NOT NULL DEFAULT '',
		enabled       BOOLEAN NOT NULL DEFAULT true,
		auto_detected BOOLEAN NOT NULL DEFAULT false,
		created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_nodes_user_id ON nodes(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_node_id ON sessions(node_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
	CREATE INDEX IF NOT EXISTS idx_agents_node_id ON agents(node_id);

	CREATE TABLE IF NOT EXISTS messages (
		id          VARCHAR(36) PRIMARY KEY,
		session_id  VARCHAR(36) NOT NULL,
		envelope    JSONB NOT NULL,
		created_at  TIMESTAMP DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
	CREATE INDEX IF NOT EXISTS idx_messages_session_time ON messages(session_id, created_at);
	`

	_, err := DB.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Alter existing tables to add columns that may not exist yet
	alterations := []string{
		"ALTER TABLE nodes ADD COLUMN IF NOT EXISTS max_sessions INT NOT NULL DEFAULT 3",
		"ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_id VARCHAR(36)",
		"ALTER TABLE sessions ALTER COLUMN prompt DROP NOT NULL",
	}
	for _, a := range alterations {
		if _, err := DB.Exec(a); err != nil {
			log.Printf("[DB] Alter warning: %v", err)
		}
	}

	log.Println("[DB] Migrations completed")

	// Clean up stale sessions from previous server run
	// Mark running/pending sessions as failed since the server restarted
	staleResult, err := DB.Exec(
		`UPDATE sessions SET status = 'failed', error_log = 'server restarted', updated_at = NOW(), completed_at = NOW()
		 WHERE status IN ('running', 'pending')`,
	)
	if err != nil {
		log.Printf("[DB] Stale session cleanup warning: %v", err)
	} else {
		if n, _ := staleResult.RowsAffected(); n > 0 {
			log.Printf("[DB] Marked %d stale session(s) as failed (server restart)", n)
		}
	}

	// Clean up offline bus virtual nodes that have no connected runtime
	// These are stale DB records from previous connections or session FK upserts
	cleanResult, err := DB.Exec(
		`DELETE FROM nodes WHERE status = 'offline' AND id LIKE 'bus-%'`,
	)
	if err != nil {
		log.Printf("[DB] Bus node cleanup warning: %v", err)
	} else {
		if n, _ := cleanResult.RowsAffected(); n > 0 {
			log.Printf("[DB] Cleaned up %d offline bus node(s)", n)
		}
	}

	return nil
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
