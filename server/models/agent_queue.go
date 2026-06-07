package models

import (
	"encoding/json"
	"time"
)

type AgentQueueStatus string

const (
	AgentQueued     AgentQueueStatus = "queued"
	AgentClaimed    AgentQueueStatus = "claimed"
	AgentProcessing AgentQueueStatus = "processing"
	AgentCompleted  AgentQueueStatus = "completed"
	AgentFailed     AgentQueueStatus = "failed"
)

type TaskAgentQueue struct {
	ID             string          `json:"id"`
	TaskID         string          `json:"task_id"`
	AgentProfileID string          `json:"agent_profile_id"`
	Status         string          `json:"status"`
	AssignedAt     *time.Time      `json:"assigned_at,omitempty"`
	ClaimedAt      *time.Time      `json:"claimed_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	ResultSummary  string          `json:"result_summary"`
	Snapshot       json.RawMessage `json:"snapshot,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}
