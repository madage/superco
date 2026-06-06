package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/superco/server/models"
)

type AgentProfileHandler struct {
	DB *sql.DB
}

func NewAgentProfileHandler(db *sql.DB) *AgentProfileHandler {
	return &AgentProfileHandler{DB: db}
}

func (h *AgentProfileHandler) List(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")

	query := `SELECT id, user_id, name, avatar, description, agent_id, version, model, backend, enabled, created_at, updated_at
		 FROM agent_profiles WHERE user_id = $1`
	args := []any{userID}
	if workspaceID != "" {
		query += ` AND workspace_id = $2`
		args = append(args, workspaceID)
	}
	query += ` ORDER BY created_at ASC`

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query profiles"})
		return
	}
	defer rows.Close()

	profiles := make([]models.AgentProfile, 0)
	for rows.Next() {
		var p models.AgentProfile
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Avatar, &p.Description,
			&p.AgentID, &p.Version, &p.Model, &p.Backend, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan profile"})
			return
		}
		profiles = append(profiles, p)
	}
	c.JSON(http.StatusOK, gin.H{"profiles": profiles})
}

func (h *AgentProfileHandler) Get(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")
	profileID := c.Param("id")

	query := `SELECT id, user_id, name, avatar, description, agent_id, version, model, backend, enabled, created_at, updated_at
		 FROM agent_profiles WHERE id = $1 AND user_id = $2`
	args := []any{profileID, userID}
	if workspaceID != "" {
		query += ` AND workspace_id = $3`
		args = append(args, workspaceID)
	}

	var p models.AgentProfile
	err := h.DB.QueryRow(query, args...).Scan(&p.ID, &p.UserID, &p.Name, &p.Avatar, &p.Description,
		&p.AgentID, &p.Version, &p.Model, &p.Backend, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query profile"})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *AgentProfileHandler) Create(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		AgentID     string `json:"agent_id"`
		Avatar      string `json:"avatar,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" || req.AgentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and agent_id are required"})
		return
	}

	avatar := req.Avatar
	if avatar == "" {
		avatar = "🤖"
	}

	id := uuid.New().String()
	now := time.Now()
	_, err := h.DB.Exec(
		`INSERT INTO agent_profiles (id, user_id, workspace_id, name, avatar, description, agent_id, version, model, backend, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, '', '', 'cli', true, $8, $8)`,
		id, userID, workspaceID, req.Name, avatar, req.Description, req.AgentID, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create profile"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id, "status": "created"})
}

func (h *AgentProfileHandler) Update(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")
	profileID := c.Param("id")

	var req struct {
		Name        *string `json:"name,omitempty"`
		Description *string `json:"description,omitempty"`
		Avatar      *string `json:"avatar,omitempty"`
		AgentID     *string `json:"agent_id,omitempty"`
		Enabled     *bool   `json:"enabled,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	addField := func(col string, val interface{}) {
		setClauses = append(setClauses, col+" = $"+fmt.Sprint(argIdx))
		args = append(args, val)
		argIdx++
	}

	if req.Name != nil {
		addField("name", *req.Name)
	}
	if req.Description != nil {
		addField("description", *req.Description)
	}
	if req.Avatar != nil {
		addField("avatar", *req.Avatar)
	}
	if req.AgentID != nil {
		addField("agent_id", *req.AgentID)
	}
	if req.Enabled != nil {
		addField("enabled", *req.Enabled)
	}

	if len(setClauses) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	whereArgs := []interface{}{profileID, userID}
	if workspaceID != "" {
		whereArgs = append(whereArgs, workspaceID)
	}
	args = append(args, whereArgs...)

	query := "UPDATE agent_profiles SET "
	for i, clause := range setClauses {
		if i > 0 {
			query += ", "
		}
		query += clause
	}
	query += fmt.Sprintf(" WHERE id = $%d AND user_id = $%d", argIdx, argIdx+1)
	argIdx += 2
	if workspaceID != "" {
		query += fmt.Sprintf(" AND workspace_id = $%d", argIdx)
	}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *AgentProfileHandler) Delete(c *gin.Context) {
	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")
	profileID := c.Param("id")

	query := `DELETE FROM agent_profiles WHERE id = $1 AND user_id = $2`
	args := []interface{}{profileID, userID}
	if workspaceID != "" {
		query += ` AND workspace_id = $3`
		args = append(args, workspaceID)
	}

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete profile"})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *AgentProfileHandler) ListRuntimes(c *gin.Context) {
	runtimes := []gin.H{
		{"id": "claude", "name": "Claude Code", "description": "AI programming assistant powered by Claude"},
		{"id": "echo", "name": "Echo", "description": "Simple echo backend for testing"},
	}
	c.JSON(http.StatusOK, gin.H{"runtimes": runtimes})
}
