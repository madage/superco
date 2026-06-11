package models

import "time"

type SessionStatus string

const (
	SessionPending   SessionStatus = "pending"
	SessionRunning   SessionStatus = "running"
	SessionPaused    SessionStatus = "paused"
	SessionCompleted SessionStatus = "completed"
	SessionFailed    SessionStatus = "failed"
)

type Session struct {
	ID             string        `json:"id"`
	UserID         string        `json:"user_id"`
	NodeID         string        `json:"node_id"`
	AgentID        string        `json:"agent_id"`
	TaskID         string        `json:"task_id,omitempty"`
	QueueID        string        `json:"queue_id,omitempty"`
	AgentProfileID string        `json:"agent_profile_id,omitempty"`
	Status         SessionStatus `json:"status"`
	Prompt         string        `json:"prompt"`
	Workspace      string        `json:"workspace"`
	OutputLog      string        `json:"output_log,omitempty"`
	ErrorLog       string        `json:"error_log,omitempty"`
	Pid            int           `json:"pid,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	CompletedAt    *time.Time    `json:"completed_at,omitempty"`
}

type CreateSessionReq struct {
	Prompt    string `json:"prompt"`
	Workspace string `json:"workspace" binding:"required"`
	NodeID    string `json:"node_id" binding:"required"`
	AgentID   string `json:"agent_id" binding:"required"`
}

type SessionResp struct {
	ID        string        `json:"id"`
	Status    SessionStatus `json:"status"`
	AgentID   string        `json:"agent_id"`
	Prompt    string        `json:"prompt"`
	Workspace string        `json:"workspace"`
	NodeID    string        `json:"node_id"`
	CreatedAt time.Time     `json:"created_at"`
}
