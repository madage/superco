package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/superco/server/models"
)

type TaskHandler struct {
	DB  *sql.DB
	Hub *DashboardHub
}

func NewTaskHandler(db *sql.DB) *TaskHandler {
	return &TaskHandler{DB: db}
}

func (h *TaskHandler) List(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")

	query := `SELECT id, user_id, title, description, status, project_id, created_at, updated_at
		 FROM tasks WHERE user_id = $1 AND deleted_at IS NULL`
	args := []any{userID}
	argIdx := 2

	if workspaceID != "" {
		query += fmt.Sprintf(" AND workspace_id = $%d", argIdx)
		args = append(args, workspaceID)
		argIdx++
	}

	if projectID := c.Query("project_id"); projectID != "" {
		if projectID == "none" {
			query += fmt.Sprintf(" AND project_id IS NULL")
		} else {
			query += fmt.Sprintf(" AND project_id = $%d", argIdx)
			args = append(args, projectID)
			argIdx++
		}
	}

	query += " ORDER BY updated_at DESC"

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query tasks"})
		return
	}
	defer rows.Close()

	tasks := make([]models.Task, 0)
	for rows.Next() {
		var t models.Task
		if err := rows.Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Status, &t.ProjectID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		tasks = append(tasks, t)
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (h *TaskHandler) Create(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")

	var req models.CreateTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	task := models.Task{
		ID:          uuid.New().String(),
		UserID:      userID.(string),
		Title:       req.Title,
		Description: req.Description,
		Status:      models.TaskTodo,
		CreatedAt:   now,
		UpdatedAt:   now,
		ProjectID:   req.ProjectID,
	}

	_, err := h.DB.Exec(
		`INSERT INTO tasks (id, user_id, workspace_id, title, description, status, project_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		task.ID, task.UserID, workspaceID, task.Title, task.Description, task.Status, task.ProjectID, task.CreatedAt, task.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}
	c.JSON(http.StatusCreated, task)
}

func (h *TaskHandler) Get(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")
	taskID := c.Param("id")

	query := `SELECT id, user_id, title, description, status, project_id, created_at, updated_at
		 FROM tasks WHERE id = $1 AND user_id = $2`
	args := []any{taskID, userID}
	argIdx := 3

	if workspaceID != "" {
		query += fmt.Sprintf(" AND workspace_id = $%d", argIdx)
		args = append(args, workspaceID)
	}

	var t models.Task
	err := h.DB.QueryRow(query, args...).Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Status, &t.ProjectID, &t.CreatedAt, &t.UpdatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, t)
}

func (h *TaskHandler) Update(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")
	taskID := c.Param("id")

	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	var req models.UpdateTaskReq
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Parse body into a map to detect which fields were explicitly provided
	var fields map[string]interface{}
	json.Unmarshal(bodyBytes, &fields)

	// Build dynamic SET clause
	var sets []string
	var args []any
	argIdx := 1

	if req.Title != nil {
		sets = append(sets, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *req.Title)
		argIdx++
	}
	if req.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.Status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *req.Status)
		argIdx++
	}

	// project_id can be explicitly set to null (clear project), distinguish from "not provided"
	if _, exists := fields["project_id"]; exists {
		if req.ProjectID != nil {
			sets = append(sets, fmt.Sprintf("project_id = $%d", argIdx))
			args = append(args, *req.ProjectID)
		} else {
			sets = append(sets, fmt.Sprintf("project_id = $%d", argIdx))
			args = append(args, nil)
		}
		argIdx++
	}

	if len(sets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	sets = append(sets, "updated_at = NOW()")
	whereArgs := []any{taskID, userID}
	if workspaceID != "" {
		whereArgs = append(whereArgs, workspaceID)
	}
	args = append(args, whereArgs...)

	whereClause := fmt.Sprintf("WHERE id = $%d AND user_id = $%d", argIdx, argIdx+1)
	argIdx += 2
	if workspaceID != "" {
		whereClause += fmt.Sprintf(" AND workspace_id = $%d", argIdx)
	}

	query := fmt.Sprintf("UPDATE tasks SET %s %s", strings.Join(sets, ", "), whereClause)

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update task"})
		return
	}

	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	// Return updated task
	var t models.Task
	h.DB.QueryRow(
		`SELECT id, user_id, title, description, status, project_id, created_at, updated_at
		 FROM tasks WHERE id = $1`, taskID,
	).Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Status, &t.ProjectID, &t.CreatedAt, &t.UpdatedAt)

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}
	c.JSON(http.StatusOK, t)
}

func (h *TaskHandler) Delete(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")
	taskID := c.Param("id")

	query := `UPDATE tasks SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`
	args := []any{taskID, userID}
	if workspaceID != "" {
		query += ` AND workspace_id = $3`
		args = append(args, workspaceID)
	}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete task"})
		return
	}

	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *TaskHandler) ListTrash(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")

	query := `SELECT id, user_id, title, description, status, project_id, created_at, updated_at
		 FROM tasks WHERE user_id = $1 AND deleted_at IS NOT NULL`
	args := []any{userID}
	if workspaceID != "" {
		query += ` AND workspace_id = $2`
		args = append(args, workspaceID)
	}
	query += ` ORDER BY updated_at DESC`

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query trash"})
		return
	}
	defer rows.Close()

	tasks := make([]models.Task, 0)
	for rows.Next() {
		var t models.Task
		if err := rows.Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Status, &t.ProjectID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		tasks = append(tasks, t)
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (h *TaskHandler) PermanentDelete(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")
	taskID := c.Param("id")

	query := `DELETE FROM tasks WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL`
	args := []any{taskID, userID}
	if workspaceID != "" {
		query += ` AND workspace_id = $3`
		args = append(args, workspaceID)
	}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to permanently delete task"})
		return
	}

	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}
	c.JSON(http.StatusOK, gin.H{"status": "permanently deleted"})
}

func (h *TaskHandler) Restore(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")
	taskID := c.Param("id")

	query := `UPDATE tasks SET deleted_at = NULL, updated_at = NOW() WHERE id = $1 AND user_id = $2`
	args := []any{taskID, userID}
	if workspaceID != "" {
		query += ` AND workspace_id = $3`
		args = append(args, workspaceID)
	}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restore task"})
		return
	}

	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}
	c.JSON(http.StatusOK, gin.H{"status": "restored"})
}

func (h *TaskHandler) SetStatus(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")
	taskID := c.Param("id")

	var req models.SetStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate status
	valid := map[string]bool{"todo": true, "in_progress": true, "blocked": true, "done": true, "review": true}
	if !valid[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	query := `UPDATE tasks SET status = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3`
	args := []any{req.Status, taskID, userID}
	if workspaceID != "" {
		query += ` AND workspace_id = $4`
		args = append(args, workspaceID)
	}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}

	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	// Return updated task
	var t models.Task
	h.DB.QueryRow(
		`SELECT id, user_id, title, description, status, project_id, created_at, updated_at
		 FROM tasks WHERE id = $1`, taskID,
	).Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Status, &t.ProjectID, &t.CreatedAt, &t.UpdatedAt)

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}
	c.JSON(http.StatusOK, t)
}
