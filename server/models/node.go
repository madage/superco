package models

import "time"

type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
	NodeStatusBusy    NodeStatus = "busy"
)

type Node struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Name        string     `json:"name"`
	OS          string     `json:"os"`
	Arch        string     `json:"arch"`
	Status      NodeStatus `json:"status"`
	Version     string     `json:"version"`
	IP          string     `json:"ip"`
	MaxSessions int        `json:"max_sessions"`
	LastSeen    time.Time  `json:"last_seen"`
	CreatedAt   time.Time  `json:"created_at"`
	Agents      []Agent    `json:"agents,omitempty"`
	CanManage   bool       `json:"can_manage,omitempty"`
}

type NodeRegisterReq struct {
	NodeToken string `json:"node_token" binding:"required"`
	Name      string `json:"name"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Version   string `json:"version"`
}

type NodeRegisterResp struct {
	NodeID string `json:"node_id"`
}

type NodeHeartbeatReq struct {
	NodeID string `json:"node_id" binding:"required"`
	Status string `json:"status"`
}

// NodeJoinToken represents a one-time join token for remote node registration.
type NodeJoinToken struct {
	Token       string    `json:"token"`
	UserID      string    `json:"user_id"`
	WorkspaceID string    `json:"workspace_id"`
	NodeName    string    `json:"node_name"`
	Status      string    `json:"status"` // pending, used, expired
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	UsedAt      *time.Time `json:"used_at,omitempty"`
}

type GenerateTokenReq struct {
	NodeName string `json:"node_name" binding:"required"`
}

type GenerateTokenResp struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Command   string    `json:"command"`
	CommandPS1 string   `json:"command_ps1"`
}

type RemoveNodeReq struct {
	NodeID string `uri:"id" binding:"required"`
}
