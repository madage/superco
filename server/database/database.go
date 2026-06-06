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

	CREATE TABLE IF NOT EXISTS agent_profiles (
		id            VARCHAR(36) PRIMARY KEY,
		user_id       VARCHAR(36) NOT NULL REFERENCES users(id),
		name          VARCHAR(64) NOT NULL,
		avatar        VARCHAR(32) NOT NULL DEFAULT '🤖',
		description   TEXT NOT NULL DEFAULT '',
		agent_id      VARCHAR(64) NOT NULL,
		version       VARCHAR(32) NOT NULL DEFAULT '',
		model         VARCHAR(64) NOT NULL DEFAULT '',
		backend       VARCHAR(16) NOT NULL DEFAULT 'cli',
		enabled       BOOLEAN NOT NULL DEFAULT true,
		created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_agent_profiles_user_id ON agent_profiles(user_id);

	CREATE TABLE IF NOT EXISTS tasks (
		id          VARCHAR(36) PRIMARY KEY,
		user_id     VARCHAR(36) NOT NULL REFERENCES users(id),
		title       VARCHAR(255) NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status      VARCHAR(16) NOT NULL DEFAULT 'todo',
		created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at  TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_user_id ON tasks(user_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

	CREATE TABLE IF NOT EXISTS projects (
		id          VARCHAR(36) PRIMARY KEY,
		user_id     VARCHAR(36) NOT NULL REFERENCES users(id),
		name        VARCHAR(255) NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		color       VARCHAR(7) NOT NULL DEFAULT '#1976d2',
		created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at  TIMESTAMP NOT NULL DEFAULT NOW(),
		deleted_at  TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_projects_user_id ON projects(user_id);

	CREATE TABLE IF NOT EXISTS workspaces (
		id          VARCHAR(36) PRIMARY KEY,
		user_id     VARCHAR(36) NOT NULL REFERENCES users(id),
		name        VARCHAR(128) NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at  TIMESTAMP NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_workspaces_user_id ON workspaces(user_id);
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
		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP",
		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS project_id VARCHAR(36) REFERENCES projects(id)",
		"CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id)",
		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(36) REFERENCES workspaces(id)",
		"ALTER TABLE projects ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(36) REFERENCES workspaces(id)",
		"ALTER TABLE agent_profiles ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(36) REFERENCES workspaces(id)",
		"CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks(workspace_id)",
		"CREATE INDEX IF NOT EXISTS idx_projects_workspace_id ON projects(workspace_id)",
		"CREATE INDEX IF NOT EXISTS idx_agent_profiles_workspace_id ON agent_profiles(workspace_id)",
	}
	for _, a := range alterations {
		if _, err := DB.Exec(a); err != nil {
			log.Printf("[DB] Alter warning: %v", err)
		}
	}

	log.Println("[DB] Migrations completed")

	// Clean up stale sessions from previous server run
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

	// Clean up offline bus virtual nodes
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

	backfillWorkspaces()
	return nil
}

func backfillWorkspaces() {
	rows, err := DB.Query(`SELECT DISTINCT u.id FROM users u WHERE NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.user_id = u.id)`)
	if err != nil {
		log.Printf("[DB] Failed to query users for workspace backfill: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		_, err := DB.Exec(
			`INSERT INTO workspaces (id, user_id, name, description, created_at, updated_at)
			 VALUES (gen_random_uuid()::text, $1, 'Default', 'Default workspace', NOW(), NOW())`,
			userID,
		)
		if err != nil {
			log.Printf("[DB] Failed to create default workspace for user %s: %v", userID, err)
			continue
		}
		DB.Exec(`UPDATE tasks SET workspace_id = (SELECT id FROM workspaces WHERE user_id = $1 ORDER BY created_at ASC LIMIT 1) WHERE user_id = $1 AND workspace_id IS NULL`, userID)
		DB.Exec(`UPDATE projects SET workspace_id = (SELECT id FROM workspaces WHERE user_id = $1 ORDER BY created_at ASC LIMIT 1) WHERE user_id = $1 AND workspace_id IS NULL`, userID)
		DB.Exec(`UPDATE agent_profiles SET workspace_id = (SELECT id FROM workspaces WHERE user_id = $1 ORDER BY created_at ASC LIMIT 1) WHERE user_id = $1 AND workspace_id IS NULL`, userID)
		log.Printf("[DB] Backfilled default workspace for user %s", userID)
	}
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
