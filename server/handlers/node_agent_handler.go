package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/coaether/server/models"
	"github.com/coaether/server/protocol"
)

type NodeAgentHandler struct {
	DB  *sql.DB
	Bus *protocol.MessageBus
}

func NewNodeAgentHandler(db *sql.DB, bus *protocol.MessageBus) *NodeAgentHandler {
	return &NodeAgentHandler{DB: db, Bus: bus}
}

type nodeAuthInfo struct {
	NodeID    string
	UserID    string
	NodeName  string
	WorkspaceID string
}

func (h *NodeAgentHandler) authenticate(c *gin.Context) (*nodeAuthInfo, bool) {
	secret := c.Query("node_secret")
	nodeID := c.Query("node_id")
	if secret == "" || nodeID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "node_secret and node_id required"})
		return nil, false
	}

	secretHash := sha256.Sum256([]byte(secret))
	secretHashHex := hex.EncodeToString(secretHash[:])

	var info nodeAuthInfo
	err := h.DB.QueryRow(
		`SELECT id, user_id, name FROM nodes WHERE id = $1 AND node_secret_hash = $2`,
		nodeID, secretHashHex,
	).Scan(&info.NodeID, &info.UserID, &info.NodeName)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid node credentials"})
		return nil, false
	}

	// Get the user's primary workspace
	_ = h.DB.QueryRow(
		`SELECT workspace_id FROM workspace_members WHERE user_id = $1 ORDER BY created_at ASC LIMIT 1`,
		info.UserID,
	).Scan(&info.WorkspaceID)

	return &info, true
}

// ListQueue returns queued items for this node's agent profiles.
func (h *NodeAgentHandler) ListQueue(c *gin.Context) {
	auth, ok := h.authenticate(c)
	if !ok {
		return
	}

	rows, err := h.DB.Query(`
		SELECT q.id, q.task_id, q.agent_profile_id, q.status, q.assigned_at, q.claimed_at, q.completed_at, q.result_summary, q.snapshot, q.created_at,
			ap.name AS agent_name
		FROM task_agent_queue q
		JOIN agent_profiles ap ON ap.id = q.agent_profile_id
		JOIN workspace_members wm ON wm.workspace_id = ap.workspace_id
		WHERE wm.user_id = $1 AND q.status = 'queued'
		ORDER BY q.created_at ASC
		LIMIT 10`, auth.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query queue"})
		return
	}
	defer rows.Close()

	type QueueItem struct {
		ID             string          `json:"id"`
		TaskID         string          `json:"task_id"`
		AgentProfileID string          `json:"agent_profile_id"`
		Status         string          `json:"status"`
		AssignedAt     *time.Time      `json:"assigned_at"`
		ClaimedAt      *time.Time      `json:"claimed_at"`
		CompletedAt    *time.Time      `json:"completed_at"`
		ResultSummary  string          `json:"result_summary"`
		Snapshot       json.RawMessage `json:"snapshot"`
		CreatedAt      time.Time       `json:"created_at"`
		AgentName      string          `json:"agent_name"`
	}
	items := make([]QueueItem, 0)
	for rows.Next() {
		var item QueueItem
		var snapshot sql.NullString
		if err := rows.Scan(&item.ID, &item.TaskID, &item.AgentProfileID, &item.Status,
			&item.AssignedAt, &item.ClaimedAt, &item.CompletedAt, &item.ResultSummary,
			&snapshot, &item.CreatedAt, &item.AgentName); err != nil {
			continue
		}
		if snapshot.Valid && snapshot.String != "" {
			item.Snapshot = json.RawMessage(snapshot.String)
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"queue": items})
}

// ClaimQueueItem claims a queued item.
func (h *NodeAgentHandler) ClaimQueueItem(c *gin.Context) {
	auth, ok := h.authenticate(c)
	if !ok {
		return
	}

	queueID := c.Param("id")

	var currentStatus string
	err := h.DB.QueryRow(`SELECT status FROM task_agent_queue WHERE id = $1`, queueID).Scan(&currentStatus)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue item not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query queue"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to claim item"})
		return
	}

	// Update agent current_load
	h.DB.Exec(`UPDATE agent_profiles SET current_load = current_load + 1,
		last_active_at = $1
		WHERE id = (SELECT agent_profile_id FROM task_agent_queue WHERE id = $2)`,
		now, queueID)

	log.Printf("[NodeAgent] Node %s claimed queue item %s", auth.NodeID, queueID)
	c.JSON(http.StatusOK, gin.H{"status": "claimed"})
}

// UpdateQueueStatus updates a queue item's status.
func (h *NodeAgentHandler) UpdateQueueStatus(c *gin.Context) {
	_, ok := h.authenticate(c)
	if !ok {
		return
	}

	queueID := c.Param("id")

	var req struct {
		Status        string `json:"status"`
		ResultSummary string `json:"result_summary,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	valid := map[string]bool{"claimed": true, "processing": true, "completed": true, "failed": true}
	if !valid[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	now := time.Now()
	if req.Status == "processing" {
		h.DB.Exec(`UPDATE task_agent_queue SET status = $1, claimed_at = COALESCE(claimed_at, $2) WHERE id = $3`,
			req.Status, now, queueID)
	} else if req.Status == "completed" || req.Status == "failed" {
		h.DB.Exec(`UPDATE task_agent_queue SET status = $1, completed_at = $2, result_summary = $3 WHERE id = $4`,
			req.Status, now, req.ResultSummary, queueID)
		// Decrement current_load
		h.DB.Exec(`UPDATE agent_profiles SET current_load = GREATEST(0, current_load - 1)
			WHERE id = (SELECT agent_profile_id FROM task_agent_queue WHERE id = $1)`, queueID)
	} else {
		h.DB.Exec(`UPDATE task_agent_queue SET status = $1 WHERE id = $2`, req.Status, queueID)
	}

	log.Printf("[NodeAgent] Queue item %s → %s", queueID, req.Status)
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// GetTask returns task details (title + description) for a task.
func (h *NodeAgentHandler) GetTask(c *gin.Context) {
	_, ok := h.authenticate(c)
	if !ok {
		return
	}

	taskID := c.Param("id")
	var task struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	err := h.DB.QueryRow(
		`SELECT id, title, description, status FROM tasks WHERE id = $1 AND deleted_at IS NULL`,
		taskID,
	).Scan(&task.ID, &task.Title, &task.Description, &task.Status)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch task"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"task": task})
}

// CreateSession creates a session for a task and routes it to this node's runtime.
func (h *NodeAgentHandler) CreateSession(c *gin.Context) {
	auth, ok := h.authenticate(c)
	if !ok {
		return
	}

	var req struct {
		TaskID  string `json:"task_id"`
		AgentID string `json:"agent_id"` // agent profile ID
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.TaskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	// Get task details
	var title, description, workspaceID string
	err := h.DB.QueryRow(
		`SELECT title, COALESCE(description,''), workspace_id FROM tasks WHERE id = $1 AND deleted_at IS NULL`,
		req.TaskID,
	).Scan(&title, &description, &workspaceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	// Determine agent_id for the session
	agentID := req.AgentID
	if agentID == "" {
		agentID = "claude"
	}

	prompt := fmt.Sprintf("Task: %s\n\nDescription: %s\n\nPlease work on this task.", title, description)

	sessionID := uuid.New().String()
	now := time.Now()

	// Create session in MessageBus
	h.Bus.CreateSession(sessionID, map[string]protocol.MemberRole{
		"system://api": protocol.RoleOwner,
	})

	// Insert into DB
	h.DB.Exec(
		`INSERT INTO sessions (id, user_id, node_id, agent_id, status, prompt, workspace, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		sessionID, auth.UserID, auth.NodeID, agentID, models.SessionPending, prompt, workspaceID, now, now,
	)

	// Create session on bus — route to the runtime
	epID := "runtime://" + auth.NodeID
	createEnv := protocol.NewEnvelope("system://api", epID, protocol.MsgSessionCreate,
		&protocol.Payload{
			Agents: []protocol.AgentSpec{
				{ID: agentID},
			},
			Workspace: workspaceID,
			Context: map[string]any{
				"task_id":     req.TaskID,
				"task_title":  title,
				"is_auto_task": true,
			},
		},
	)
	createEnv.SessionID = sessionID
	h.Bus.Deliver(createEnv)

	log.Printf("[NodeAgent] Created session %s for task %s on node %s", sessionID, req.TaskID, auth.NodeID)

	// Send the task prompt directly to the runtime, tagged with the session ID
	time.Sleep(300 * time.Millisecond) // brief wait for runtime to join
	msgEnv := protocol.NewEnvelope("system://api", "runtime://"+auth.NodeID, protocol.MsgMessage,
		&protocol.Payload{
			Content: []protocol.ContentBlock{protocol.TextBlock(prompt)},
			Metadata: map[string]any{
				"task_id":      req.TaskID,
				"auto_task":    true,
			},
		},
	)
	msgEnv.SessionID = sessionID
	h.Bus.Deliver(msgEnv)
	log.Printf("[NodeAgent] Sent task prompt to %s", msgEnv.To)

	c.JSON(http.StatusCreated, gin.H{
		"session_id": sessionID,
		"status":     "pending",
		"prompt":     prompt,
	})
}
