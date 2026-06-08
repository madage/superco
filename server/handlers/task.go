package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/coaether/server/middleware"
	"github.com/coaether/server/models"
	"github.com/coaether/server/protocol"
)

type TaskHandler struct {
	DB             *sql.DB
	Hub            *DashboardHub
	Notifier       *NotificationHandler
	RuleEngine     *RuleEngine
	AgentScheduler *AgentScheduler
	MessageBus     *protocol.MessageBus
}

func NewTaskHandler(db *sql.DB) *TaskHandler {
	return &TaskHandler{DB: db}
}

const taskSelectCols = `t.id, t.user_id, COALESCE(u.username, '') AS creator_name, t.title, t.description, t.status, t.project_id,
	t.parent_id, t.assignee_id, t.assignee_type, t.priority, t.due_at, t.completed_at, t.created_at, t.updated_at`

func (h *TaskHandler) scanTask(scanner interface {
	Scan(dest ...interface{}) error
}, t *models.Task) error {
	return scanner.Scan(
		&t.ID, &t.UserID, &t.CreatorName, &t.Title, &t.Description, &t.Status,
		&t.ProjectID, &t.ParentID, &t.AssigneeID, &t.AssigneeType,
		&t.Priority, &t.DueAt, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt,
	)
}

// fetchTags retrieves tags for a given task ID.
func (h *TaskHandler) fetchTags(taskID string) []string {
	rows, err := h.DB.Query(`SELECT tag FROM task_tags WHERE task_id = $1 ORDER BY tag`, taskID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err == nil {
			tags = append(tags, tag)
		}
	}
	return tags
}

// fetchAssignees retrieves delegated assignees for a given task ID.
func (h *TaskHandler) fetchAssignees(taskID string) []models.TaskAssignee {
	rows, err := h.DB.Query(`SELECT task_id, assignee_id, assignee_type, role FROM task_assignees WHERE task_id = $1`, taskID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var as []models.TaskAssignee
	for rows.Next() {
		var a models.TaskAssignee
		if err := rows.Scan(&a.TaskID, &a.AssigneeID, &a.AssigneeType, &a.Role); err == nil {
			as = append(as, a)
		}
	}
	return as
}

// replaceTags removes all existing tags and inserts new ones in a transaction.
func (h *TaskHandler) replaceTags(tx *sql.Tx, taskID string, tags []string) error {
	if _, err := tx.Exec(`DELETE FROM task_tags WHERE task_id = $1`, taskID); err != nil {
		return err
	}
	for _, tag := range tags {
		if tag = strings.TrimSpace(tag); tag == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO task_tags (task_id, tag) VALUES ($1, $2) ON CONFLICT DO NOTHING`, taskID, tag); err != nil {
			return err
		}
	}
	return nil
}

func (h *TaskHandler) List(c *gin.Context) {
	workspaceID := c.Query("workspace_id")
	isMember, _ := c.Get("is_workspace_member")

	query := fmt.Sprintf(`SELECT %s FROM tasks t LEFT JOIN users u ON u.id = t.user_id WHERE t.deleted_at IS NULL`, taskSelectCols)
	args := []any{}
	argIdx := 1

	if workspaceID != "" && isMember.(bool) {
		query += fmt.Sprintf(" AND t.workspace_id = $%d", argIdx)
		args = append(args, workspaceID)
		argIdx++
	} else {
		userID, _ := c.Get("user_id")
		query += fmt.Sprintf(" AND t.user_id = $%d", argIdx)
		args = append(args, userID)
		argIdx++
	}

	// Optional filters
	if projectID := c.Query("project_id"); projectID != "" {
		if projectID == "none" {
			query += " AND t.project_id IS NULL"
		} else {
			query += fmt.Sprintf(" AND t.project_id = $%d", argIdx)
			args = append(args, projectID)
			argIdx++
		}
	}
	if parentID := c.Query("parent_id"); parentID != "" {
		if parentID == "none" {
			query += " AND t.parent_id IS NULL"
		} else {
			query += fmt.Sprintf(" AND t.parent_id = $%d", argIdx)
			args = append(args, parentID)
			argIdx++
		}
	}
	if assigneeID := c.Query("assignee_id"); assigneeID != "" {
		query += fmt.Sprintf(" AND t.assignee_id = $%d", argIdx)
		args = append(args, assigneeID)
		argIdx++
	}
	if delegatedID := c.Query("delegated_assignee_id"); delegatedID != "" {
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM task_assignees ta WHERE ta.task_id = t.id AND ta.assignee_id = $%d)", argIdx)
		args = append(args, delegatedID)
		argIdx++
	}
	if priority := c.Query("priority"); priority != "" {
		query += fmt.Sprintf(" AND t.priority = $%d", argIdx)
		args = append(args, priority)
		argIdx++
	}
	if tag := c.Query("tag"); tag != "" {
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM task_tags tt WHERE tt.task_id = t.id AND tt.tag = $%d)", argIdx)
		args = append(args, tag)
		argIdx++
	}

	query += " ORDER BY t.updated_at DESC"

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query tasks"})
		return
	}
	defer rows.Close()

	tasks := make([]models.Task, 0)
	for rows.Next() {
		var t models.Task
		if err := h.scanTask(rows, &t); err != nil {
			continue
		}
		t.Tags = h.fetchTags(t.ID)
			t.Assignees = h.fetchAssignees(t.ID)
		tasks = append(tasks, t)
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (h *TaskHandler) Create(c *gin.Context) {
	if !middleware.CanWrite(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions to create tasks"})
		return
	}

	userID, _ := c.Get("user_id")
	workspaceID := c.Query("workspace_id")

	var req models.CreateTaskReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	priority := models.PriorityMedium
	if req.Priority != nil {
		priority = *req.Priority
	}

	now := time.Now()
	taskID := uuid.New().String()
	task := models.Task{
		ID:           taskID,
		UserID:       userID.(string),
		Title:        req.Title,
		Description:  req.Description,
		Status:       models.TaskTodo,
		ProjectID:    req.ProjectID,
		ParentID:     req.ParentID,
		AssigneeID:   req.AssigneeID,
		AssigneeType: req.AssigneeType,
		Priority:     priority,
		DueAt:        req.DueAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO tasks (id, user_id, workspace_id, title, description, status, project_id,
		 parent_id, assignee_id, assignee_type, priority, due_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		task.ID, task.UserID, workspaceID, task.Title, task.Description, task.Status,
		task.ProjectID, task.ParentID, task.AssigneeID, task.AssigneeType,
		task.Priority, task.DueAt, task.CreatedAt, task.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	if len(req.Tags) > 0 {
		if err := h.replaceTags(tx, taskID, req.Tags); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save tags"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	task.Tags = req.Tags

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}

	// Notify assignee
	if h.Notifier != nil && req.AssigneeID != nil && req.AssigneeType != nil && *req.AssigneeType == "user" {
		h.Notifier.Create(*req.AssigneeID, string(models.NotifTaskAssigned),
			"Task Assigned",
			fmt.Sprintf("You have been assigned to \"%s\"", req.Title),
			&taskID)
	}

	// Evaluate automation rules
	if h.RuleEngine != nil {
		aid := ""
		atp := ""
		if req.AssigneeID != nil {
			aid = *req.AssigneeID
		}
		if req.AssigneeType != nil {
			atp = *req.AssigneeType
		}
		h.RuleEngine.Evaluate("on_task_create", taskID, ExtractTaskContext(taskID, req.Title, aid, atp))
		if aid != "" {
			h.RuleEngine.Evaluate("on_assignee_change", taskID, ExtractAssigneeContext(taskID, aid, atp))
		}
	}

	// Auto-assign to an agent profile if requested
	if req.AutoAssign && h.AgentScheduler != nil {
		h.autoAssignTask(taskID, workspaceID)
	}

	c.JSON(http.StatusCreated, task)
}

func (h *TaskHandler) autoAssignTask(taskID, workspaceID string) {
	rows, err := h.DB.Query(`
		SELECT id FROM agent_profiles
		WHERE workspace_id = $1 AND enabled = true
			AND COALESCE(current_load, 0) < COALESCE(max_concurrency, 1)
		ORDER BY current_load ASC, last_active_at DESC NULLS LAST
		LIMIT 1`, workspaceID)
	if err != nil {
		return
	}
	defer rows.Close()

	if !rows.Next() {
		return
	}

	var agentID string
	rows.Scan(&agentID)

	queueID := uuid.New().String()
	now := time.Now()
	h.DB.Exec(
		`INSERT INTO task_agent_queue (id, task_id, agent_profile_id, status, assigned_at, created_at)
		 VALUES ($1, $2, $3, 'queued', $4, $4)`,
		queueID, taskID, agentID, now,
	)
	h.DB.Exec(`UPDATE agent_profiles SET current_load = current_load + 1, last_active_at = $1 WHERE id = $2`, now, agentID)

	if h.Hub != nil {
		h.Hub.SignalChange("task_agent_queue")
	}

	// Auto-process: create session and send task to the runtime
	h.processAgentTask(taskID, agentID, queueID)
}

func (h *TaskHandler) processAgentTask(taskID, agentProfileID, queueID string) {
	autoProcessTask(h.DB, h.MessageBus, taskID, agentProfileID, queueID)
}

// autoProcessTask creates a session on the message bus and delivers the task prompt
// to the connected agent runtime. It is shared between TaskHandler and AgentScheduler.
func autoProcessTask(db *sql.DB, bus *protocol.MessageBus, taskID, agentProfileID, queueID string) {
	if bus == nil {
		return
	}

	// Get task details + agent profile node_id + task owner
	var title, description, workspaceID, nodeID, userID string
	err := db.QueryRow(`
		SELECT t.title, COALESCE(t.description,''), t.workspace_id, ap.node_id, t.user_id
		FROM tasks t
		JOIN agent_profiles ap ON ap.id = $2
		WHERE t.id = $1 AND t.deleted_at IS NULL`,
		taskID, agentProfileID,
	).Scan(&title, &description, &workspaceID, &nodeID, &userID)
	if err != nil || nodeID == "" {
		return // can't process without a node
	}

	// Check if the runtime is connected on the bus
	runtimeEndpoint := "runtime://" + nodeID
	found := false
	for _, ep := range bus.EndpointsByType(protocol.EndpointRuntime) {
		if ep.ID == runtimeEndpoint {
			found = true
			break
		}
	}
	if !found {
		return // runtime not connected, leave as queued
	}

	// Create a session
	sessionID := uuid.New().String()
	now := time.Now()
	prompt := fmt.Sprintf("Task: %s\n\nDescription: %s\n\nPlease work on this task.", title, description)

	bus.CreateSession(sessionID, map[string]protocol.MemberRole{
		"system://api": protocol.RoleOwner,
	})

	db.Exec(
		`INSERT INTO sessions (id, user_id, node_id, agent_id, status, prompt, workspace, created_at, updated_at)
		 VALUES ($1, $2, $3, 'claude', $4, $5, $6, $7, $7)`,
		sessionID, userID, nodeID, models.SessionPending, prompt, workspaceID, now,
	)

	// Send session.create to the runtime
	createEnv := protocol.NewEnvelope("system://api", runtimeEndpoint, protocol.MsgSessionCreate,
		&protocol.Payload{
			Agents:    []protocol.AgentSpec{{ID: "claude"}},
			Workspace: workspaceID,
			Context: map[string]any{
				"queue_id":    queueID,
				"task_id":     taskID,
				"is_auto_task": true,
			},
		},
	)
	createEnv.SessionID = sessionID
	bus.Deliver(createEnv)

	// Brief wait for runtime to join, then send the task prompt
	time.Sleep(500 * time.Millisecond)
	msgEnv := protocol.NewEnvelope("system://api", runtimeEndpoint, protocol.MsgMessage,
		&protocol.Payload{
			Content: []protocol.ContentBlock{protocol.TextBlock(prompt)},
			Metadata: map[string]any{
				"task_id":      taskID,
				"auto_task":    true,
			},
		},
	)
	msgEnv.SessionID = sessionID
	bus.Deliver(msgEnv)

	// Update queue to processing
	db.Exec(`UPDATE task_agent_queue SET status = 'processing', claimed_at = $1 WHERE id = $2`, now, queueID)

	log.Printf("[Task] Auto-processed task %s → session %s on node %s", taskID[:8], sessionID[:8], nodeID[:8])
}

func (h *TaskHandler) Get(c *gin.Context) {
	workspaceID := c.Query("workspace_id")
	isMember, _ := c.Get("is_workspace_member")
	taskID := c.Param("id")

	query := fmt.Sprintf(`SELECT %s FROM tasks t LEFT JOIN users u ON u.id = t.user_id WHERE t.id = $1`, taskSelectCols)
	args := []any{taskID}
	argIdx := 2

	if workspaceID != "" && isMember.(bool) {
		query += fmt.Sprintf(" AND t.workspace_id = $%d", argIdx)
		args = append(args, workspaceID)
	} else {
		userID, _ := c.Get("user_id")
		query += fmt.Sprintf(" AND t.user_id = $%d", argIdx)
		args = append(args, userID)
	}

	var t models.Task
	err := h.scanTask(h.DB.QueryRow(query, args...), &t)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	t.Tags = h.fetchTags(t.ID)
			t.Assignees = h.fetchAssignees(t.ID)
	c.JSON(http.StatusOK, t)
}

func (h *TaskHandler) canModifyTask(c *gin.Context, creatorID string) bool {
	return middleware.HasRole(c, "admin", "owner") ||
		(middleware.HasRole(c, "worker") && middleware.IsOwner(c, creatorID))
}

func (h *TaskHandler) Update(c *gin.Context) {
	taskID := c.Param("id")
	workspaceID := c.Query("workspace_id")

	// Check permission
	var creatorID, oldAssigneeID, oldAssigneeType string
	err := h.DB.QueryRow(`SELECT user_id, COALESCE(assignee_id, ''), COALESCE(assignee_type, '') FROM tasks WHERE id = $1`, taskID).Scan(&creatorID, &oldAssigneeID, &oldAssigneeType)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if !h.canModifyTask(c, creatorID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

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

	var fields map[string]interface{}
	json.Unmarshal(bodyBytes, &fields)

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
		// Auto-set completed_at when status changes to/from done
		if *req.Status == string(models.TaskDone) {
			now := time.Now()
			sets = append(sets, fmt.Sprintf("completed_at = $%d", argIdx))
			args = append(args, now)
			argIdx++
		} else {
			sets = append(sets, fmt.Sprintf("completed_at = $%d", argIdx))
			args = append(args, nil)
			argIdx++
		}
	}
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
	if _, exists := fields["parent_id"]; exists {
		if req.ParentID != nil {
			sets = append(sets, fmt.Sprintf("parent_id = $%d", argIdx))
			args = append(args, *req.ParentID)
		} else {
			sets = append(sets, fmt.Sprintf("parent_id = $%d", argIdx))
			args = append(args, nil)
		}
		argIdx++
	}
	if _, exists := fields["assignee_id"]; exists {
		if req.AssigneeID != nil {
			sets = append(sets, fmt.Sprintf("assignee_id = $%d", argIdx))
			args = append(args, *req.AssigneeID)
		} else {
			sets = append(sets, fmt.Sprintf("assignee_id = $%d", argIdx))
			args = append(args, nil)
		}
		argIdx++
	}
	if _, exists := fields["assignee_type"]; exists {
		if req.AssigneeType != nil {
			sets = append(sets, fmt.Sprintf("assignee_type = $%d", argIdx))
			args = append(args, *req.AssigneeType)
		} else {
			sets = append(sets, fmt.Sprintf("assignee_type = $%d", argIdx))
			args = append(args, nil)
		}
		argIdx++
	}
	if _, exists := fields["priority"]; exists {
		sets = append(sets, fmt.Sprintf("priority = $%d", argIdx))
		if req.Priority != nil {
			args = append(args, *req.Priority)
		} else {
			args = append(args, string(models.PriorityMedium))
		}
		argIdx++
	}
	if _, exists := fields["due_at"]; exists {
		if req.DueAt != nil {
			sets = append(sets, fmt.Sprintf("due_at = $%d", argIdx))
			args = append(args, *req.DueAt)
		} else {
			sets = append(sets, fmt.Sprintf("due_at = $%d", argIdx))
			args = append(args, nil)
		}
		argIdx++
	}

	if len(sets) == 0 && req.Tags == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	sets = append(sets, "updated_at = NOW()")
	isMember, _ := c.Get("is_workspace_member")

	args = append(args, taskID)
	whereClause := fmt.Sprintf("WHERE id = $%d", argIdx)
	argIdx++

	if workspaceID != "" && isMember.(bool) {
		args = append(args, workspaceID)
		whereClause += fmt.Sprintf(" AND workspace_id = $%d", argIdx)
	}

	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to begin transaction"})
		return
	}
	defer tx.Rollback()

	if len(sets) > 0 {
		query := fmt.Sprintf("UPDATE tasks SET %s %s", strings.Join(sets, ", "), whereClause)
		result, err := tx.Exec(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update task"})
			return
		}
		if n, _ := result.RowsAffected(); n == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
	}

	// Handle tags update (field is always sent as empty array to clear, or populated)
	if _, exists := fields["tags"]; exists {
		if err := h.replaceTags(tx, taskID, req.Tags); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tags"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit update"})
		return
	}

	var t models.Task
	query := fmt.Sprintf(`SELECT %s FROM tasks t LEFT JOIN users u ON u.id = t.user_id WHERE t.id = $1`, taskSelectCols)
	h.DB.QueryRow(query, taskID).Scan(
		&t.ID, &t.UserID, &t.CreatorName, &t.Title, &t.Description, &t.Status,
		&t.ProjectID, &t.ParentID, &t.AssigneeID, &t.AssigneeType,
		&t.Priority, &t.DueAt, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	t.Tags = h.fetchTags(t.ID)
	t.Assignees = h.fetchAssignees(t.ID)

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}

	// Notifications for Update
	if h.Notifier != nil {
		actorID, _ := c.Get("user_id")
		actorStr, _ := actorID.(string)

		// Notify about status change
		if req.Status != nil {
			assignees := t.Assignees
			for _, a := range assignees {
				if a.AssigneeType == "user" && a.AssigneeID != actorStr {
					h.Notifier.Create(a.AssigneeID, string(models.NotifTaskStatusChanged),
						fmt.Sprintf("Task \"%s\" is now %s", t.Title, *req.Status),
						fmt.Sprintf("Status changed to %s", *req.Status),
						&t.ID)
				}
			}
			if t.AssigneeID != nil && t.AssigneeType != nil && *t.AssigneeType == "user" && *t.AssigneeID != actorStr {
				alreadyNotified := false
				for _, a := range t.Assignees {
					if a.AssigneeID == *t.AssigneeID { alreadyNotified = true; break }
				}
				if !alreadyNotified {
					h.Notifier.Create(*t.AssigneeID, string(models.NotifTaskStatusChanged),
						fmt.Sprintf("Task \"%s\" is now %s", t.Title, *req.Status),
						fmt.Sprintf("Status changed to %s", *req.Status),
						&t.ID)
				}
			}
		}

		// Notify new assignee if changed
		if _, exists := fields["assignee_id"]; exists && req.AssigneeID != nil && req.AssigneeType != nil && *req.AssigneeType == "user" {
			newID := *req.AssigneeID
			if newID != oldAssigneeID && newID != actorStr {
				h.Notifier.Create(newID, string(models.NotifTaskAssigned),
					"Task Assigned",
					fmt.Sprintf("You have been assigned to \"%s\"", t.Title),
					&t.ID)
			}
		}
	}

	// Evaluate automation rules on status change
	if h.RuleEngine != nil {
		h.RuleEngine.Evaluate("on_status_change", taskID, ExtractStatusContext(taskID, string(t.Status)))
	}

	c.JSON(http.StatusOK, t)
}

func (h *TaskHandler) Delete(c *gin.Context) {
	workspaceID := c.Query("workspace_id")
	taskID := c.Param("id")

	var creatorID string
	err := h.DB.QueryRow(`SELECT user_id FROM tasks WHERE id = $1`, taskID).Scan(&creatorID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if !h.canModifyTask(c, creatorID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	query := `UPDATE tasks SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	args := []any{taskID}
	isMember, _ := c.Get("is_workspace_member")
	if workspaceID != "" && isMember.(bool) {
		query += ` AND workspace_id = $2`
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
	workspaceID := c.Query("workspace_id")
	isMember, _ := c.Get("is_workspace_member")

	query := fmt.Sprintf(`SELECT %s FROM tasks t LEFT JOIN users u ON u.id = t.user_id WHERE t.deleted_at IS NOT NULL`, taskSelectCols)
	args := []any{}
	argIdx := 1

	if workspaceID != "" && isMember.(bool) {
		query += fmt.Sprintf(" AND t.workspace_id = $%d", argIdx)
		args = append(args, workspaceID)
		argIdx++
	} else {
		userID, _ := c.Get("user_id")
		query += fmt.Sprintf(" AND t.user_id = $%d", argIdx)
		args = append(args, userID)
		argIdx++
	}
	query += ` ORDER BY t.updated_at DESC`

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query trash"})
		return
	}
	defer rows.Close()

	tasks := make([]models.Task, 0)
	for rows.Next() {
		var t models.Task
		if err := h.scanTask(rows, &t); err != nil {
			continue
		}
		t.Tags = h.fetchTags(t.ID)
			t.Assignees = h.fetchAssignees(t.ID)
		tasks = append(tasks, t)
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (h *TaskHandler) PermanentDelete(c *gin.Context) {
	workspaceID := c.Query("workspace_id")
	taskID := c.Param("id")

	var creatorID string
	err := h.DB.QueryRow(`SELECT user_id FROM tasks WHERE id = $1`, taskID).Scan(&creatorID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if !h.canModifyTask(c, creatorID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	query := `DELETE FROM tasks WHERE id = $1 AND deleted_at IS NOT NULL`
	args := []any{taskID}
	isMember, _ := c.Get("is_workspace_member")
	if workspaceID != "" && isMember.(bool) {
		query += ` AND workspace_id = $2`
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
	workspaceID := c.Query("workspace_id")
	taskID := c.Param("id")

	var creatorID string
	err := h.DB.QueryRow(`SELECT user_id FROM tasks WHERE id = $1`, taskID).Scan(&creatorID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if !h.canModifyTask(c, creatorID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	query := `UPDATE tasks SET deleted_at = NULL, updated_at = NOW() WHERE id = $1`
	args := []any{taskID}
	isMember, _ := c.Get("is_workspace_member")
	if workspaceID != "" && isMember.(bool) {
		query += ` AND workspace_id = $2`
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
	workspaceID := c.Query("workspace_id")
	taskID := c.Param("id")

	var creatorID string
	err := h.DB.QueryRow(`SELECT user_id FROM tasks WHERE id = $1`, taskID).Scan(&creatorID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if !h.canModifyTask(c, creatorID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	var req models.SetStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	valid := map[string]bool{"todo": true, "in_progress": true, "blocked": true, "done": true, "review": true}
	if !valid[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	// Auto-set completed_at
	var completedAt interface{}
	if req.Status == string(models.TaskDone) {
		completedAt = time.Now()
	} else {
		completedAt = nil
	}

	query := `UPDATE tasks SET status = $1, completed_at = $2, updated_at = NOW() WHERE id = $3`
	args := []any{req.Status, completedAt, taskID}
	isMember, _ := c.Get("is_workspace_member")
	if workspaceID != "" && isMember.(bool) {
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

	var t models.Task
	h.DB.QueryRow(
		fmt.Sprintf(`SELECT %s FROM tasks t LEFT JOIN users u ON u.id = t.user_id WHERE t.id = $1`, taskSelectCols), taskID,
	).Scan(
		&t.ID, &t.UserID, &t.CreatorName, &t.Title, &t.Description, &t.Status,
		&t.ProjectID, &t.ParentID, &t.AssigneeID, &t.AssigneeType,
		&t.Priority, &t.DueAt, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	t.Tags = h.fetchTags(t.ID)
	t.Assignees = h.fetchAssignees(t.ID)

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}

	// Notify assignees about status change
	if h.Notifier != nil {
		actorID, _ := c.Get("user_id")
		actorStr, _ := actorID.(string)
		for _, a := range t.Assignees {
			if a.AssigneeType == "user" && a.AssigneeID != actorStr {
				h.Notifier.Create(a.AssigneeID, string(models.NotifTaskStatusChanged),
					fmt.Sprintf("Task \"%s\" is now %s", t.Title, t.Status),
					fmt.Sprintf("Status changed to %s", t.Status),
					&t.ID)
			}
		}
		if t.AssigneeID != nil && t.AssigneeType != nil && *t.AssigneeType == "user" && *t.AssigneeID != actorStr {
			alreadyNotified := false
			for _, a := range t.Assignees {
				if a.AssigneeID == *t.AssigneeID { alreadyNotified = true; break }
			}
			if !alreadyNotified {
				h.Notifier.Create(*t.AssigneeID, string(models.NotifTaskStatusChanged),
					fmt.Sprintf("Task \"%s\" is now %s", t.Title, t.Status),
					fmt.Sprintf("Status changed to %s", t.Status),
					&t.ID)
			}
		}
	}

	c.JSON(http.StatusOK, t)
}

// === Assignee Management ===

func (h *TaskHandler) AddAssignee(c *gin.Context) {
	taskID := c.Param("id")

	var creatorID string
	err := h.DB.QueryRow(`SELECT user_id FROM tasks WHERE id = $1`, taskID).Scan(&creatorID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if !h.canModifyTask(c, creatorID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	var req models.AddAssigneeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.AssigneeType != "user" && req.AssigneeType != "agent_profile" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assignee_type must be 'user' or 'agent_profile'"})
		return
	}

	_, err = h.DB.Exec(
		`INSERT INTO task_assignees (task_id, assignee_id, assignee_type, role)
		 VALUES ($1, $2, $3, 'assignee') ON CONFLICT DO NOTHING`,
		taskID, req.AssigneeID, req.AssigneeType,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add assignee"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}
		// Notify new assignee and task creator
		if h.Notifier != nil {
			actorID, _ := c.Get("user_id")
			actorStr, _ := actorID.(string)
			if req.AssigneeType == "user" {
				h.Notifier.Create(req.AssigneeID, string(models.NotifTaskAssigned),
					"Task Assigned",
					"You have been assigned to a task",
					&taskID)
			}
			if creatorID != actorStr {
				h.Notifier.Create(creatorID, string(models.NotifTaskAssigned),
					"New Assignee",
					"Someone was assigned to your task",
					&taskID)
			}
		}

	// Evaluate automation rules on assignee change
	if h.RuleEngine != nil {
		h.RuleEngine.Evaluate("on_assignee_change", taskID, ExtractAssigneeContext(taskID, req.AssigneeID, req.AssigneeType))
	}

	c.JSON(http.StatusCreated, gin.H{"status": "added"})
}

func (h *TaskHandler) RemoveAssignee(c *gin.Context) {
	taskID := c.Param("id")
	assigneeID := c.Param("assigneeId")

	var creatorID string
	err := h.DB.QueryRow(`SELECT user_id FROM tasks WHERE id = $1`, taskID).Scan(&creatorID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if !h.canModifyTask(c, creatorID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	result, err := h.DB.Exec(
		`DELETE FROM task_assignees WHERE task_id = $1 AND assignee_id = $2`,
		taskID, assigneeID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove assignee"})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "assignee not found"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

func (h *TaskHandler) ListAssignees(c *gin.Context) {
	taskID := c.Param("id")

	rows, err := h.DB.Query(
		`SELECT task_id, assignee_id, assignee_type, role FROM task_assignees WHERE task_id = $1 ORDER BY assignee_type, assignee_id`,
		taskID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query assignees"})
		return
	}
	defer rows.Close()

	assignees := make([]models.TaskAssignee, 0)
	for rows.Next() {
		var a models.TaskAssignee
		if err := rows.Scan(&a.TaskID, &a.AssigneeID, &a.AssigneeType, &a.Role); err != nil {
			continue
		}
		assignees = append(assignees, a)
	}

	c.JSON(http.StatusOK, gin.H{"assignees": assignees})
}

// === Subtask Management ===

func (h *TaskHandler) ListSubtasks(c *gin.Context) {
	taskID := c.Param("id")

	query := fmt.Sprintf(`SELECT %s FROM tasks t LEFT JOIN users u ON u.id = t.user_id WHERE t.parent_id = $1 AND t.deleted_at IS NULL ORDER BY t.created_at`, taskSelectCols)
	rows, err := h.DB.Query(query, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query subtasks"})
		return
	}
	defer rows.Close()

	tasks := make([]models.Task, 0)
	for rows.Next() {
		var t models.Task
		if err := h.scanTask(rows, &t); err != nil {
			continue
		}
		t.Tags = h.fetchTags(t.ID)
			t.Assignees = h.fetchAssignees(t.ID)
		tasks = append(tasks, t)
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

// === Comment Management ===

func (h *TaskHandler) ListComments(c *gin.Context) {
	taskID := c.Param("id")

	rows, err := h.DB.Query(`
		SELECT c.id, c.task_id, c.user_id, COALESCE(u.username, '') AS username,
			c.agent_profile_id, COALESCE(ap.name, '') AS agent_name, COALESCE(ap.avatar, '') AS agent_avatar,
			c.content, c.parent_id, c.is_agent_comment, c.created_at, c.updated_at
		FROM task_comments c
		LEFT JOIN users u ON u.id = c.user_id
		LEFT JOIN agent_profiles ap ON ap.id = c.agent_profile_id
		WHERE c.task_id = $1
		ORDER BY COALESCE(c.parent_id, c.id), c.created_at ASC`, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query comments"})
		return
	}
	defer rows.Close()

	comments := make([]models.TaskComment, 0)
	for rows.Next() {
		var cm models.TaskComment
		if err := rows.Scan(
			&cm.ID, &cm.TaskID, &cm.UserID, &cm.Username,
			&cm.AgentProfileID, &cm.AgentName, &cm.AgentAvatar,
			&cm.Content, &cm.ParentID, &cm.CreatedAt, &cm.UpdatedAt,
		); err != nil {
			continue
		}
		comments = append(comments, cm)
	}

	c.JSON(http.StatusOK, gin.H{"comments": comments})
}

func (h *TaskHandler) CreateComment(c *gin.Context) {
	taskID := c.Param("id")
	userID, _ := c.Get("user_id")

	var req models.CreateCommentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Agent autonomous comments bypass user permission check
	if !req.IsAgentComment || req.AgentProfileID == nil || *req.AgentProfileID == "" {
		if !middleware.CanWrite(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
	}

	// Verify task exists
	var exists bool
	h.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM tasks WHERE id = $1)`, taskID).Scan(&exists)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	now := time.Now()
	comment := models.TaskComment{
		ID:            uuid.New().String(),
		TaskID:        taskID,
		UserID:        userID.(string),
		AgentProfileID: req.AgentProfileID,
		Content:       req.Content,
		ParentID:      req.ParentID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	_, err := h.DB.Exec(
		`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, parent_id, is_agent_comment, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		comment.ID, comment.TaskID, comment.UserID, comment.AgentProfileID,
		comment.Content, comment.ParentID, req.IsAgentComment, comment.CreatedAt, comment.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create comment"})
		return
	}

	// Fetch the created comment with user/agent info
	h.DB.QueryRow(`
		SELECT c.id, c.task_id, c.user_id, COALESCE(u.username, '') AS username,
			c.agent_profile_id, COALESCE(ap.name, '') AS agent_name, COALESCE(ap.avatar, '') AS agent_avatar,
			c.content, c.parent_id, c.is_agent_comment, c.created_at, c.updated_at
		FROM task_comments c
		LEFT JOIN users u ON u.id = c.user_id
		LEFT JOIN agent_profiles ap ON ap.id = c.agent_profile_id
		WHERE c.id = $1`, comment.ID,
	).Scan(
		&comment.ID, &comment.TaskID, &comment.UserID, &comment.Username,
		&comment.AgentProfileID, &comment.AgentName, &comment.AgentAvatar,
		&comment.Content, &comment.ParentID, &comment.IsAgentComment, &comment.CreatedAt, &comment.UpdatedAt,
	)

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}
		// Notify task assignees about the new comment
		if h.Notifier != nil {
			commenterID, _ := c.Get("user_id")
			commenterStr, _ := commenterID.(string)
			var taskTitle string
			h.DB.QueryRow(`SELECT title FROM tasks WHERE id = $1`, taskID).Scan(&taskTitle)
			assignees := h.fetchAssignees(taskID)
			for _, a := range assignees {
				if a.AssigneeType == "user" && a.AssigneeID != commenterStr {
					msg := req.Content
					if len([]rune(msg)) > 80 {
						msg = string([]rune(msg)[:80]) + "..."
					}
					h.Notifier.Create(a.AssigneeID, string(models.NotifTaskComment),
						fmt.Sprintf("New comment on \"%s\"", taskTitle),
						msg,
						&taskID)
				}
			}

			// Notify @mentioned users
			mentioned := parseMentions(req.Content)
			for _, username := range mentioned {
				var uid string
				if err := h.DB.QueryRow(`SELECT id FROM users WHERE username = $1`, username).Scan(&uid); err != nil {
					continue
				}
				if uid == commenterStr {
					continue
				}
				h.Notifier.Create(uid, string(models.NotifTaskMention),
					fmt.Sprintf("You were mentioned in \"%s\"", taskTitle),
					req.Content,
					&taskID)
			}
		}

	// Evaluate automation rules on comment
	if h.RuleEngine != nil {
		h.RuleEngine.Evaluate("on_comment", taskID, ExtractCommentContext(taskID, req.Content))
	}

	c.JSON(http.StatusCreated, comment)
}

// parseMentions extracts @username mentions from a comment string.
func parseMentions(content string) []string {
	re := regexp.MustCompile(`@(\w{2,64})`)
	matches := re.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result
}

func (h *TaskHandler) DeleteComment(c *gin.Context) {
	taskID := c.Param("id")
	commentID := c.Param("commentId")

	// Get comment owner
	var commentUserID string
	err := h.DB.QueryRow(`SELECT user_id FROM task_comments WHERE id = $1 AND task_id = $2`, commentID, taskID).Scan(&commentUserID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Allow if admin/owner, or if the comment owner matches the current user
	currentUserID, _ := c.Get("user_id")
	isOwner := commentUserID == currentUserID.(string)
	if !middleware.HasRole(c, "admin", "owner") && !isOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	result, err := h.DB.Exec(`DELETE FROM task_comments WHERE id = $1 AND task_id = $2`, commentID, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete comment"})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
