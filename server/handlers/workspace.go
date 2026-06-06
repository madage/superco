package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/superco/server/models"
)

type WorkspaceHandler struct {
	DB  *sql.DB
	Hub *DashboardHub
}

func NewWorkspaceHandler(db *sql.DB) *WorkspaceHandler {
	return &WorkspaceHandler{DB: db}
}

func (h *WorkspaceHandler) List(c *gin.Context) {
	userID, _ := c.Get("user_id")

	rows, err := h.DB.Query(
		`SELECT id, user_id, name, description, created_at, updated_at
		 FROM workspaces WHERE user_id = $1 ORDER BY created_at ASC`, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query workspaces"})
		return
	}
	defer rows.Close()

	workspaces := make([]models.Workspace, 0)
	for rows.Next() {
		var w models.Workspace
		if err := rows.Scan(&w.ID, &w.UserID, &w.Name, &w.Description, &w.CreatedAt, &w.UpdatedAt); err != nil {
			continue
		}
		workspaces = append(workspaces, w)
	}

	c.JSON(http.StatusOK, gin.H{"workspaces": workspaces})
}

func (h *WorkspaceHandler) Create(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req models.CreateWorkspaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	ws := models.Workspace{
		ID:          uuid.New().String(),
		UserID:      userID.(string),
		Name:        req.Name,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := h.DB.Exec(
		`INSERT INTO workspaces (id, user_id, name, description, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		ws.ID, ws.UserID, ws.Name, ws.Description, ws.CreatedAt, ws.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create workspace"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("workspaces")
	}
	c.JSON(http.StatusCreated, ws)
}

func (h *WorkspaceHandler) Get(c *gin.Context) {
	userID, _ := c.Get("user_id")
	wsID := c.Param("id")

	var w models.Workspace
	err := h.DB.QueryRow(
		`SELECT id, user_id, name, description, created_at, updated_at
		 FROM workspaces WHERE id = $1 AND user_id = $2`, wsID, userID,
	).Scan(&w.ID, &w.UserID, &w.Name, &w.Description, &w.CreatedAt, &w.UpdatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, w)
}

func (h *WorkspaceHandler) Update(c *gin.Context) {
	userID, _ := c.Get("user_id")
	wsID := c.Param("id")

	var req models.UpdateWorkspaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var sets []string
	var args []any
	argIdx := 1

	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}

	if len(sets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, wsID, userID)

	query := fmt.Sprintf(
		`UPDATE workspaces SET %s WHERE id = $%d AND user_id = $%d`,
		strings.Join(sets, ", "), argIdx, argIdx+1,
	)

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update workspace"})
		return
	}

	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("workspaces")
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *WorkspaceHandler) Delete(c *gin.Context) {
	userID, _ := c.Get("user_id")
	wsID := c.Param("id")

	// Check if this is the user's only workspace
	var count int
	h.DB.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE user_id = $1`, userID).Scan(&count)
	if count <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the only workspace"})
		return
	}

	// Check if this is the "Default" workspace
	var wsName string
	h.DB.QueryRow(`SELECT name FROM workspaces WHERE id = $1 AND user_id = $2`, wsID, userID).Scan(&wsName)
	if wsName == "Default" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the Default workspace"})
		return
	}

	// Unlink entities from this workspace
	h.DB.Exec(`UPDATE tasks SET workspace_id = NULL WHERE workspace_id = $1 AND user_id = $2`, wsID, userID)
	h.DB.Exec(`UPDATE projects SET workspace_id = NULL WHERE workspace_id = $1 AND user_id = $2`, wsID, userID)
	h.DB.Exec(`UPDATE agent_profiles SET workspace_id = NULL WHERE workspace_id = $1 AND user_id = $2`, wsID, userID)

	result, err := h.DB.Exec(`DELETE FROM workspaces WHERE id = $1 AND user_id = $2`, wsID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete workspace"})
		return
	}

	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("workspaces")
		h.Hub.SignalChange("tasks")
		h.Hub.SignalChange("projects")
		h.Hub.SignalChange("agent_profiles")
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
