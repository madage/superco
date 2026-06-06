package models

import "time"

type Workspace struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateWorkspaceReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type UpdateWorkspaceReq struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}
