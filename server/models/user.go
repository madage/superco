package models

import "time"

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type LoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResp struct {
	Token       string `json:"token"`
	User        User   `json:"user"`
	WorkspaceID string `json:"workspace_id"`
}
