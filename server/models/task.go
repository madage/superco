package models

import "time"

type TaskStatus string

const (
	TaskTodo       TaskStatus = "todo"
	TaskInProgress TaskStatus = "in_progress"
	TaskBlocked    TaskStatus = "blocked"
	TaskDone       TaskStatus = "done"
	TaskReview     TaskStatus = "review"
)

type Task struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	ProjectID   *string    `json:"project_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type CreateTaskReq struct {
	Title       string  `json:"title" binding:"required"`
	Description string  `json:"description"`
	ProjectID   *string `json:"project_id,omitempty"`
}

type UpdateTaskReq struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
	ProjectID   *string `json:"project_id,omitempty"`
}

type SetStatusReq struct {
	Status string `json:"status" binding:"required"`
}
