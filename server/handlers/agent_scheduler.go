package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/coaether/server/middleware"
	"github.com/coaether/server/models"
)

type AgentScheduler struct {
	DB  *sql.DB
	Hub *DashboardHub
}

func NewAgentScheduler(db *sql.DB) *AgentScheduler {
	return &AgentScheduler{DB: db}
}

// List returns the agent task queue, optionally filtered by agent_profile_id.
func (h *AgentScheduler) List(c *gin.Context) {
	workspaceID := c.Query("workspace_id")
	isMember, _ := c.Get("is_workspace_member")
	agentID := c.Query("agent_profile_id")
	status := c.Query("status")

	query := `SELECT q.id, q.task_id, q.agent_profile_id, q.status, q.assigned_at, q.claimed_at, q.completed_at, q.result_summary, q.snapshot, q.created_at
		FROM task_agent_queue q
		JOIN agent_profiles ap ON ap.id = q.agent_profile_id`
	args := []interface{}{}
	argIdx := 1
	where := []string{}

	if workspaceID != "" && isMember.(bool) {
		where = append(where, fmt.Sprintf("ap.workspace_id = $%d", argIdx))
		args = append(args, workspaceID)
		argIdx++
	}
	if agentID != "" {
		where = append(where, fmt.Sprintf("q.agent_profile_id = $%d", argIdx))
		args = append(args, agentID)
		argIdx++
	}
	if status != "" {
		where = append(where, fmt.Sprintf("q.status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	if len(where) > 0 {
		query += " WHERE "
		for i, w := range where {
			if i > 0 {
				query += " AND "
			}
			query += w
		}
	}

	query += " ORDER BY q.created_at DESC"

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query agent queue"})
		return
	}
	defer rows.Close()

	items := make([]models.TaskAgentQueue, 0)
	for rows.Next() {
		var item models.TaskAgentQueue
		if err := rows.Scan(&item.ID, &item.TaskID, &item.AgentProfileID, &item.Status,
			&item.AssignedAt, &item.ClaimedAt, &item.CompletedAt, &item.ResultSummary,
			&item.Snapshot, &item.CreatedAt); err != nil {
			continue
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"queue": items})
}

// AutoAssign finds the best available agent and assigns the task to it.
func (h *AgentScheduler) AutoAssign(c *gin.Context) {
	if !middleware.CanWrite(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	taskID := c.Param("taskId")
	workspaceID := c.Query("workspace_id")

	// Verify task exists
	var taskTitle string
	err := h.DB.QueryRow(`SELECT title FROM tasks WHERE id = $1`, taskID).Scan(&taskTitle)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch task"})
		return
	}

	// Find best available agent: enabled, not at max capacity, in the same workspace
	// Ordered by lowest load first, then most recently active
	rows, err := h.DB.Query(`
		SELECT id, name, COALESCE(max_concurrency, 1), COALESCE(current_load, 0)
		FROM agent_profiles
		WHERE workspace_id = $1 AND enabled = true
			AND COALESCE(current_load, 0) < COALESCE(max_concurrency, 1)
		ORDER BY current_load ASC, last_active_at DESC NULLS LAST
		LIMIT 1`, workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to find available agents"})
		return
	}
	defer rows.Close()

	if !rows.Next() {
		c.JSON(http.StatusNotFound, gin.H{"error": "no available agent"})
		return
	}

	var agentID, agentName string
	var maxConc, currLoad int
	rows.Scan(&agentID, &agentName, &maxConc, &currLoad)

	// Create queue entry
	queueID := uuid.New().String()
	now := time.Now()
	_, err = h.DB.Exec(
		`INSERT INTO task_agent_queue (id, task_id, agent_profile_id, status, assigned_at, created_at)
		 VALUES ($1, $2, $3, 'queued', $4, $4)`,
		queueID, taskID, agentID, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create queue entry"})
		return
	}

	// Increment current_load on the agent profile
	h.DB.Exec(`UPDATE agent_profiles SET current_load = current_load + 1, last_active_at = $1 WHERE id = $2`, now, agentID)

	c.JSON(http.StatusCreated, gin.H{
		"id":               queueID,
		"task_id":          taskID,
		"agent_profile_id": agentID,
		"agent_name":       agentName,
		"status":           "queued",
	})

	if h.Hub != nil {
		h.Hub.SignalChange("task_agent_queue")
	}
}

// Claim marks a queue item as claimed by an agent.
func (h *AgentScheduler) Claim(c *gin.Context) {
	if !middleware.CanWrite(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	queueID := c.Param("id")

	// Verify item exists and is in queued status
	var currentStatus string
	err := h.DB.QueryRow(`SELECT status FROM task_agent_queue WHERE id = $1`, queueID).Scan(&currentStatus)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue item not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query queue item"})
		return
	}
	if currentStatus != "queued" {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("cannot claim item with status %s", currentStatus)})
		return
	}

	now := time.Now()
	_, err = h.DB.Exec(
		`UPDATE task_agent_queue SET status = 'claimed', claimed_at = $1 WHERE id = $2`,
		now, queueID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to claim queue item"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "claimed"})
	if h.Hub != nil {
		h.Hub.SignalChange("task_agent_queue")
	}
}

// UpdateStatus updates the status of a queue item (claimed -> processing -> completed/failed).
func (h *AgentScheduler) UpdateStatus(c *gin.Context) {
	if !middleware.CanWrite(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	queueID := c.Param("id")

	var req struct {
		Status        string          `json:"status"`
		ResultSummary string          `json:"result_summary,omitempty"`
		Snapshot      json.RawMessage `json:"snapshot,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validStatuses := map[string]bool{
		"claimed": true, "processing": true, "completed": true, "failed": true,
	}
	if !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	// Fetch current status and agent_profile_id for load management
	var currentStatus, agentProfileID string
	err := h.DB.QueryRow(`SELECT status, agent_profile_id FROM task_agent_queue WHERE id = $1`, queueID).Scan(&currentStatus, &agentProfileID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue item not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query queue item"})
		return
	}

	// Build dynamic SET for status update
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	addField := func(col string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	addField("status", req.Status)

	if req.Status == "processing" {
		addField("claimed_at", time.Now())
	}
	if req.Status == "completed" || req.Status == "failed" {
		addField("completed_at", time.Now())
	}
	if req.ResultSummary != "" {
		addField("result_summary", req.ResultSummary)
	}
	if req.Snapshot != nil {
		addField("snapshot", req.Snapshot)
	}

	query := "UPDATE task_agent_queue SET "
	for i, clause := range setClauses {
		if i > 0 {
			query += ", "
		}
		query += clause
	}
	query += fmt.Sprintf(" WHERE id = $%d", argIdx)
	args = append(args, queueID)

	result, err := h.DB.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update queue item"})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue item not found"})
		return
	}

	// Adjust current_load when transitioning from claimed/processing to completed/failed
	if (req.Status == "completed" || req.Status == "failed") && (currentStatus == "claimed" || currentStatus == "processing") {
		h.DB.Exec(`UPDATE agent_profiles SET current_load = GREATEST(0, current_load - 1), last_active_at = $1 WHERE id = $2`, time.Now(), agentProfileID)
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
	if h.Hub != nil {
		h.Hub.SignalChange("task_agent_queue")
	}
}

// ListAgentsWithLoad returns agents with their current load info for assignment UI.
func (h *AgentScheduler) ListAgentsWithLoad(c *gin.Context) {
	workspaceID := c.Query("workspace_id")
	isMember, _ := c.Get("is_workspace_member")

	query := `SELECT id, name, avatar, description, COALESCE(max_concurrency,1), COALESCE(current_load,0)
		FROM agent_profiles WHERE enabled = true`
	args := []interface{}{}
	argIdx := 1

	if workspaceID != "" && isMember.(bool) {
		query += fmt.Sprintf(" AND workspace_id = $%d", argIdx)
		args = append(args, workspaceID)
		argIdx++
	}
	query += " ORDER BY name ASC"

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query agents"})
		return
	}
	defer rows.Close()

	type AgentLoad struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Avatar         string `json:"avatar"`
		Description    string `json:"description"`
		MaxConcurrency int    `json:"max_concurrency"`
		CurrentLoad    int    `json:"current_load"`
		Available      bool   `json:"available"`
	}
	agents := make([]AgentLoad, 0)
	for rows.Next() {
		var a AgentLoad
		rows.Scan(&a.ID, &a.Name, &a.Avatar, &a.Description, &a.MaxConcurrency, &a.CurrentLoad)
		a.Available = a.CurrentLoad < a.MaxConcurrency
		agents = append(agents, a)
	}

	c.JSON(http.StatusOK, gin.H{"agents": agents})
}
