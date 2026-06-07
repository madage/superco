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

		email       VARCHAR(128) UNIQUE NOT NULL DEFAULT '',

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



	CREATE TABLE IF NOT EXISTS workspace_members (

		workspace_id VARCHAR(36) NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,

		user_id      VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,

		role         VARCHAR(32) NOT NULL DEFAULT 'worker',

		joined_at    TIMESTAMP NOT NULL DEFAULT NOW(),

		PRIMARY KEY (workspace_id, user_id)

	);

	CREATE INDEX IF NOT EXISTS idx_workspace_members_user_id ON workspace_members(user_id);

	CREATE INDEX IF NOT EXISTS idx_workspace_members_workspace_id ON workspace_members(workspace_id);




	CREATE TABLE IF NOT EXISTS node_join_tokens (
		token       VARCHAR(128) PRIMARY KEY,
		user_id     VARCHAR(36) NOT NULL REFERENCES users(id),
		workspace_id VARCHAR(36) REFERENCES workspaces(id),
		node_name   VARCHAR(128) NOT NULL DEFAULT '',
		status      VARCHAR(16) NOT NULL DEFAULT 'pending',
		expires_at  TIMESTAMP NOT NULL,
		used_at     TIMESTAMP,
		created_at  TIMESTAMP NOT NULL DEFAULT NOW()
	);

			CREATE TABLE IF NOT EXISTS task_assignees (
			task_id       VARCHAR(36) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			assignee_id   VARCHAR(36) NOT NULL,
			assignee_type VARCHAR(16) NOT NULL,
			role          VARCHAR(16) NOT NULL DEFAULT 'assignee',
			PRIMARY KEY (task_id, assignee_id, assignee_type)
		);

		CREATE TABLE IF NOT EXISTS task_tags (
			task_id VARCHAR(36) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			tag     VARCHAR(64) NOT NULL,
			PRIMARY KEY (task_id, tag)
		);

		CREATE INDEX IF NOT EXISTS idx_task_tags_tag ON task_tags(tag);

		CREATE TABLE IF NOT EXISTS pending_invitations (

		id             VARCHAR(36) PRIMARY KEY,

		workspace_id   VARCHAR(36) NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,

		inviter_id     VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,

		invitee_email  VARCHAR(128) NOT NULL,

		token          VARCHAR(64) UNIQUE NOT NULL,

		role           VARCHAR(32) NOT NULL DEFAULT 'worker',

		status         VARCHAR(16) NOT NULL DEFAULT 'pending',

		created_at     TIMESTAMP NOT NULL DEFAULT NOW(),

		expires_at     TIMESTAMP NOT NULL

	);

	CREATE INDEX IF NOT EXISTS idx_pending_invitations_token ON pending_invitations(token);

	CREATE INDEX IF NOT EXISTS idx_pending_invitations_email ON pending_invitations(invitee_email);

	CREATE INDEX IF NOT EXISTS idx_pending_invitations_workspace ON pending_invitations(workspace_id);
		CREATE TABLE IF NOT EXISTS task_comments (
			id              VARCHAR(36) PRIMARY KEY,
			task_id         VARCHAR(36) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			user_id         VARCHAR(36) NOT NULL REFERENCES users(id),
			agent_profile_id VARCHAR(36) REFERENCES agent_profiles(id),
			content         TEXT NOT NULL,
			created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_task_comments_task_id ON task_comments(task_id);
		CREATE INDEX IF NOT EXISTS idx_task_comments_created_at ON task_comments(task_id, created_at);

		CREATE TABLE IF NOT EXISTS task_rules (
			id           VARCHAR(36) PRIMARY KEY,
			workspace_id VARCHAR(36) NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			name         VARCHAR(128) NOT NULL,
			description  TEXT NOT NULL DEFAULT '',
			trigger_type VARCHAR(32) NOT NULL,
			conditions   JSONB NOT NULL DEFAULT '{}',
			actions      JSONB NOT NULL DEFAULT '[]',
			enabled      BOOLEAN NOT NULL DEFAULT true,
			created_by   VARCHAR(36) NOT NULL REFERENCES users(id),
			created_at   TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMP NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_task_rules_workspace ON task_rules(workspace_id);
		CREATE INDEX IF NOT EXISTS idx_task_rules_trigger ON task_rules(workspace_id, trigger_type);

		CREATE TABLE IF NOT EXISTS task_rule_logs (
			id            VARCHAR(36) PRIMARY KEY,
			rule_id       VARCHAR(36) NOT NULL REFERENCES task_rules(id) ON DELETE CASCADE,
			task_id       VARCHAR(36) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			trigger_event VARCHAR(32) NOT NULL,
			matched       BOOLEAN NOT NULL DEFAULT false,
			result        TEXT NOT NULL DEFAULT '',
			log           TEXT NOT NULL DEFAULT '',
			created_at    TIMESTAMP NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_task_rule_logs_rule ON task_rule_logs(rule_id);
		CREATE INDEX IF NOT EXISTS idx_task_rule_logs_task ON task_rule_logs(task_id);

		CREATE TABLE IF NOT EXISTS notifications (
				id          VARCHAR(36) PRIMARY KEY,
				user_id     VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				type        VARCHAR(32) NOT NULL,
				title       VARCHAR(255) NOT NULL,
				message     TEXT NOT NULL DEFAULT '',
				task_id     VARCHAR(36) REFERENCES tasks(id) ON DELETE SET NULL,
				is_read     BOOLEAN NOT NULL DEFAULT false,
				created_at  TIMESTAMP NOT NULL DEFAULT NOW()
			);

			CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
			CREATE INDEX IF NOT EXISTS idx_notifications_user_unread ON notifications(user_id, is_read);
			CREATE INDEX IF NOT EXISTS idx_notifications_user_time ON notifications(user_id, created_at DESC);

			CREATE TABLE IF NOT EXISTS skills (
				id              VARCHAR(36) PRIMARY KEY,
				workspace_id    VARCHAR(36) NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				name            VARCHAR(128) NOT NULL,
				description     TEXT NOT NULL DEFAULT '',
				content         TEXT NOT NULL,
				tags            JSONB NOT NULL DEFAULT '[]',
				source_task_id  VARCHAR(36) REFERENCES tasks(id) ON DELETE SET NULL,
				source_agent_id VARCHAR(36) REFERENCES agent_profiles(id) ON DELETE SET NULL,
				usage_count     INT NOT NULL DEFAULT 0,
				created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_skills_workspace_id ON skills(workspace_id);
			CREATE INDEX IF NOT EXISTS idx_skills_tags ON skills USING GIN(tags);



	`



	_, err := DB.Exec(schema)

	if err != nil {

		return fmt.Errorf("failed to run migrations: %w", err)

	}



	// Alter existing tables to add columns that may not exist yet

	alterations := []string{

		"ALTER TABLE users ADD COLUMN IF NOT EXISTS email VARCHAR(128) NOT NULL DEFAULT ''",

		"ALTER TABLE nodes ADD COLUMN IF NOT EXISTS max_sessions INT NOT NULL DEFAULT 3",

		"ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_id VARCHAR(36)",

		"ALTER TABLE sessions ALTER COLUMN prompt DROP NOT NULL",

		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP",

		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS project_id VARCHAR(36) REFERENCES projects(id)",

		"CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id)",

		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(36) REFERENCES workspaces(id)",

		"ALTER TABLE projects ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(36) REFERENCES workspaces(id)",

		"ALTER TABLE agent_profiles ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(36) REFERENCES workspaces(id)",

		"ALTER TABLE node_join_tokens ALTER COLUMN workspace_id DROP NOT NULL",

		"CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks(workspace_id)",

		"CREATE INDEX IF NOT EXISTS idx_projects_workspace_id ON projects(workspace_id)",

		"CREATE INDEX IF NOT EXISTS idx_agent_profiles_workspace_id ON agent_profiles(workspace_id)",
			"ALTER TABLE agent_profiles ADD COLUMN IF NOT EXISTS node_id VARCHAR(36) NOT NULL DEFAULT ''",

		"ALTER TABLE nodes ADD COLUMN IF NOT EXISTS node_secret_hash VARCHAR(128)",

		// Task enhancements for project management
		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS parent_id VARCHAR(36) REFERENCES tasks(id)",
		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS assignee_id VARCHAR(36)",
		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS assignee_type VARCHAR(16) DEFAULT ''",
		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS priority VARCHAR(8) NOT NULL DEFAULT 'medium'",
		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS due_at TIMESTAMP",
		"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP",
		"CREATE INDEX IF NOT EXISTS idx_tasks_parent_id ON tasks(parent_id)",
		"CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee_id)",
		"CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority)",

		// Project enhancements for project management
		"ALTER TABLE projects ADD COLUMN IF NOT EXISTS assignee_id VARCHAR(36)",
		"ALTER TABLE projects ADD COLUMN IF NOT EXISTS assignee_type VARCHAR(16) DEFAULT ''",
		"ALTER TABLE projects ADD COLUMN IF NOT EXISTS status VARCHAR(16) NOT NULL DEFAULT 'planning'",
		"ALTER TABLE projects ADD COLUMN IF NOT EXISTS started_at TIMESTAMP",
		"ALTER TABLE projects ADD COLUMN IF NOT EXISTS due_at TIMESTAMP",
		"CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status)",

		// Comment thread support
		"ALTER TABLE task_comments ADD COLUMN IF NOT EXISTS parent_id VARCHAR(36) REFERENCES task_comments(id)",
		"CREATE INDEX IF NOT EXISTS idx_task_comments_parent_id ON task_comments(parent_id)",

		// Agent as Colleague: Phase 1 — Profile enhancements
		"ALTER TABLE agent_profiles ADD COLUMN IF NOT EXISTS system_prompt TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE agent_profiles ADD COLUMN IF NOT EXISTS skills JSONB NOT NULL DEFAULT '[]'",
		"ALTER TABLE agent_profiles ADD COLUMN IF NOT EXISTS max_concurrency INT NOT NULL DEFAULT 1",
		"ALTER TABLE agent_profiles ADD COLUMN IF NOT EXISTS current_load INT NOT NULL DEFAULT 0",
		"ALTER TABLE agent_profiles ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'",
		"ALTER TABLE agent_profiles ADD COLUMN IF NOT EXISTS last_active_at TIMESTAMP",

		// Agent as Colleague: Agent autonomous comment flag
		"ALTER TABLE task_comments ADD COLUMN IF NOT EXISTS is_agent_comment BOOLEAN NOT NULL DEFAULT false",

			// Agent as Colleague: Phase 3 — Task auto_assign flag
			"ALTER TABLE tasks ADD COLUMN IF NOT EXISTS auto_assign BOOLEAN NOT NULL DEFAULT false",

	}

	for _, a := range alterations {

		if _, err := DB.Exec(a); err != nil {

			log.Printf("[DB] Alter warning: %v", err)

		}

	}



	// Backfill email from username for existing users

	DB.Exec(`UPDATE users SET email = username WHERE email = ''`)

	// Add unique constraint on email (safe after backfill)

	DB.Exec(`DROP INDEX IF EXISTS users_email_key`)

	DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS users_email_key ON users(email)`)

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



	// Expire old pending invitations

	DB.Exec(`UPDATE pending_invitations SET status = 'expired' WHERE status = 'pending' AND expires_at < NOW()`)



	backfillWorkspaces()

	backfillWorkspaceMembers()

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



func backfillWorkspaceMembers() {

	_, err := DB.Exec(`

		INSERT INTO workspace_members (workspace_id, user_id, role, joined_at)

		SELECT id, user_id, 'owner', created_at FROM workspaces w

		WHERE NOT EXISTS (

			SELECT 1 FROM workspace_members wm

			WHERE wm.workspace_id = w.id AND wm.user_id = w.user_id

		)

	`)

	if err != nil {

		log.Printf("[DB] Failed to backfill workspace members: %v", err)

	} else {

		log.Println("[DB] Backfilled workspace members")

	}

}



func Close() {

	if DB != nil {

		DB.Close()

	}

}

