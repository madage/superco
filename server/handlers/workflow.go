package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/coaether/server/harness"
	"github.com/coaether/server/middleware"
	"github.com/coaether/server/models"
)

// WorkflowHandler manages workflows and their tasks.
type WorkflowHandler struct {
	DB         *sql.DB
	Hub        *DashboardHub
	DAGEngine  *DAGEngine
	Harness    *harness.Harness
	Notifier   *NotificationHandler
}

// NewWorkflowHandler creates a new workflow handler.
func NewWorkflowHandler(db *sql.DB) *WorkflowHandler {
	return &WorkflowHandler{
		DB:        db,
		DAGEngine: NewDAGEngine(db),
		Harness:   harness.NewHarness(db),
	}
}

// ==================== Workflows ====================

// ListWorkflows returns workflows for the current workspace.
func (h *WorkflowHandler) ListWorkflows(c *gin.Context) {
	wsID, _ := c.Get("validated_workspace_id")
	wsIDStr, _ := wsID.(string)

	query := `SELECT id, title, COALESCE(description,''), status, token_budget, tokens_used,
		created_by, workspace_id, created_at, updated_at
		FROM workflows`
	args := []interface{}{}
	argIdx := 1

	if wsIDStr != "" {
		query += fmt.Sprintf(" WHERE workspace_id = $%d", argIdx)
		args = append(args, wsIDStr)
		argIdx++
	}
	query += " ORDER BY created_at DESC"

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query workflows"})
		return
	}
	defer rows.Close()

	workflows := make([]models.Workflow, 0)
	for rows.Next() {
		var w models.Workflow
		if err := rows.Scan(&w.ID, &w.Title, &w.Description, &w.Status,
			&w.TokenBudget, &w.TokensUsed, &w.CreatedBy, &w.WorkspaceID,
			&w.CreatedAt, &w.UpdatedAt); err != nil {
			continue
		}
		workflows = append(workflows, w)
	}
	c.JSON(http.StatusOK, gin.H{"workflows": workflows})
}

// CreateWorkflow creates a new workflow.
func (h *WorkflowHandler) CreateWorkflow(c *gin.Context) {
	if !middleware.CanWrite(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	userID, _ := c.Get("user_id")
	wsID, _ := c.Get("validated_workspace_id")

	var req models.CreateWorkflowReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id := uuid.New().String()
	now := time.Now()
	tokenBudget := int64(100000)
	if req.TokenBudget != nil && *req.TokenBudget > 0 {
		tokenBudget = *req.TokenBudget
	}

	_, err := h.DB.Exec(
		`INSERT INTO workflows (id, title, description, status, token_budget, tokens_used, created_by, workspace_id, created_at, updated_at)
		 VALUES ($1, $2, $3, 'active', $4, 0, $5, $6, $7, $7)`,
		id, req.Title, req.Description, tokenBudget, userID, wsID, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create workflow"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("workflows")
	}

	log.Printf("[Workflow] Created workflow %s: %s", id[:8], req.Title)
	c.JSON(http.StatusCreated, gin.H{"id": id, "status": "active"})
}

// GetWorkflow returns a single workflow with task summary.
func (h *WorkflowHandler) GetWorkflow(c *gin.Context) {
	wfID := c.Param("id")

	var w models.Workflow
	err := h.DB.QueryRow(
		`SELECT id, title, COALESCE(description,''), status, token_budget, tokens_used,
			created_by, workspace_id, created_at, updated_at
		 FROM workflows WHERE id = $1`, wfID,
	).Scan(&w.ID, &w.Title, &w.Description, &w.Status,
		&w.TokenBudget, &w.TokensUsed, &w.CreatedBy, &w.WorkspaceID,
		&w.CreatedAt, &w.UpdatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "workflow not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch workflow"})
		return
	}

	// Get task summary
	type TaskSummary struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	rows, err := h.DB.Query(
		`SELECT status, COUNT(*) FROM tasks WHERE workflow_id = $1 AND deleted_at IS NULL GROUP BY status`,
		wfID,
	)
	if err == nil {
		defer rows.Close()
		summaries := make([]TaskSummary, 0)
		for rows.Next() {
			var s TaskSummary
			if err := rows.Scan(&s.Status, &s.Count); err == nil {
				summaries = append(summaries, s)
			}
		}
		c.JSON(http.StatusOK, gin.H{"workflow": w, "task_summary": summaries})
		return
	}

	c.JSON(http.StatusOK, gin.H{"workflow": w})
}

// UpdateWorkflowStatus updates a workflow's status (active/paused/done).
func (h *WorkflowHandler) UpdateWorkflowStatus(c *gin.Context) {
	if !middleware.CanWrite(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	wfID := c.Param("id")
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	valid := map[string]bool{"active": true, "paused": true, "done": true, "stuck": true}
	if !valid[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	now := time.Now()
	_, err := h.DB.Exec(
		`UPDATE workflows SET status = $1, updated_at = $2 WHERE id = $3`,
		req.Status, now, wfID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update workflow"})
		return
	}

	if h.Hub != nil {
		h.Hub.SignalChange("workflows")
	}

	c.JSON(http.StatusOK, gin.H{"status": req.Status})
}

// ==================== Workflow Tasks ====================

// ListWorkflowTasks returns all tasks in a workflow with dependency info.
func (h *WorkflowHandler) ListWorkflowTasks(c *gin.Context) {
	wfID := c.Param("id")

	rows, err := h.DB.Query(`
		SELECT t.id, t.title, t.status, t.depth, t.completion_behavior,
			t.assignee_id, t.assignee_type, t.agent_loop_count, t.max_agent_loops,
			t.parallel_group, t.created_at
		FROM tasks t
		WHERE t.workflow_id = $1 AND t.deleted_at IS NULL
		ORDER BY t.depth ASC, t.created_at ASC`, wfID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query tasks"})
		return
	}
	defer rows.Close()

	type WorkflowTask struct {
		ID                 string   `json:"id"`
		Title              string   `json:"title"`
		Status             string   `json:"status"`
		Depth              int      `json:"depth"`
		CompletionBehavior string   `json:"completion_behavior"`
		AssigneeID         *string  `json:"assignee_id,omitempty"`
		AssigneeType       *string  `json:"assignee_type,omitempty"`
		AgentLoopCount     int      `json:"agent_loop_count"`
		MaxAgentLoops      int      `json:"max_agent_loops"`
		ParallelGroup      *string  `json:"parallel_group,omitempty"`
		Dependencies       []string `json:"dependencies"`
		CreatedAt          time.Time `json:"created_at"`
	}

	tasks := make([]WorkflowTask, 0)
	for rows.Next() {
		var t WorkflowTask
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.Depth, &t.CompletionBehavior,
			&t.AssigneeID, &t.AssigneeType, &t.AgentLoopCount, &t.MaxAgentLoops,
			&t.ParallelGroup, &t.CreatedAt); err != nil {
			continue
		}
		// Get dependencies
		deps, _ := h.DAGEngine.GetDependencies(t.ID)
		if deps != nil {
			t.Dependencies = deps
		} else {
			t.Dependencies = make([]string, 0)
		}
		tasks = append(tasks, t)
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

// AttachToWorkflow attaches an existing task to a workflow.
func (h *WorkflowHandler) AttachToWorkflow(c *gin.Context) {
	if !middleware.CanWrite(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	var req struct {
		TaskID     string   `json:"task_id" binding:"required"`
		WorkflowID string   `json:"workflow_id" binding:"required"`
		DependsOn  []string `json:"depends_on,omitempty"`
		Depth      int      `json:"depth"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	_, err := h.DB.Exec(
		`UPDATE tasks SET workflow_id = $1, depth = $2, updated_at = $3 WHERE id = $4 AND deleted_at IS NULL`,
		req.WorkflowID, req.Depth, now, req.TaskID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to attach task"})
		return
	}

	// Create dependencies
	for _, depID := range req.DependsOn {
		cycle, err := h.DAGEngine.WouldCreateCycle(req.WorkflowID, req.TaskID, depID)
		if err != nil {
			log.Printf("[Workflow] Cycle check error: %v", err)
			continue
		}
		if cycle {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("adding dependency on %s would create a cycle", depID[:8])})
			return
		}
		if err := h.DAGEngine.CreateDependency(req.TaskID, depID); err != nil {
			log.Printf("[Workflow] Failed to create dependency: %v", err)
		}
	}

	// Mark as blocked if has dependencies
	if len(req.DependsOn) > 0 {
		h.DAGEngine.SetTaskBlocked(req.TaskID)
	}

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
		h.Hub.SignalChange("workflows")
	}

	c.JSON(http.StatusOK, gin.H{"status": "attached", "task_id": req.TaskID, "workflow_id": req.WorkflowID})
}

// ==================== Tool Call Executor (from Harness) ====================

// RegisterToolExecutors registers all Harness tool executors with the database.
// These are called by the agent runtime when an agent makes a Tool Call.
func (h *WorkflowHandler) RegisterToolExecutors() {
	harness.RegisterExecutor(harness.ToolCreateSubTask, func(ctx *harness.AgentContext, params json.RawMessage) (interface{}, error) {
		var p struct {
			Title              string   `json:"title"`
			Description        string   `json:"description"`
			DependsOn          []string `json:"depends_on"`
			ParallelGroup      string   `json:"parallel_group"`
			AssigneeID         string   `json:"assignee_id"`
			AssigneeType       string   `json:"assignee_type"`
			CompletionBehavior string   `json:"completion_behavior"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		now := time.Now()
		taskID := uuid.New().String()
		cb := p.CompletionBehavior
		if cb == "" {
			cb = models.CompletionNeedsReview
		}

		// Inherit workspace_id from parent task
		var workspaceID sql.NullString
		if ctx.TaskID != nil {
			h.DB.QueryRow(`SELECT workspace_id FROM tasks WHERE id = $1`, *ctx.TaskID).Scan(&workspaceID)
		}

		_, err := h.DB.Exec(
			`INSERT INTO tasks (id, user_id, parent_id, title, description, status, workspace_id, workflow_id, depth, max_depth,
				completion_behavior, parallel_group, assignee_id, assignee_type, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, 'todo', $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)`,
			taskID, ctx.UserID, ctx.TaskID, p.Title, p.Description,
			workspaceID, ctx.WorkflowID, ctx.Depth+1, ctx.MaxDepth,
			cb, p.ParallelGroup, p.AssigneeID, p.AssigneeType, now,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create subtask: %w", err)
		}

		// Create dependencies
		for _, depID := range p.DependsOn {
			if depID == "" {
				continue
			}
			if ctx.WorkflowID != nil {
				cycle, _ := h.DAGEngine.WouldCreateCycle(*ctx.WorkflowID, taskID, depID)
				if cycle {
					// Delete the task we just created
					h.DB.Exec(`DELETE FROM tasks WHERE id = $1`, taskID)
					return nil, fmt.Errorf("dependency on %s would create a cycle", depID[:8])
				}
			}
			h.DAGEngine.CreateDependency(taskID, depID)
		}

		// Mark blocked if has dependencies
		if len(p.DependsOn) > 0 {
			h.DAGEngine.SetTaskBlocked(taskID)
		}

		// Auto-start: if assigned to an agent and not blocked by dependencies,
		// create a queue entry so the runtime picks it up.
		if p.AssigneeType == "agent_profile" && p.AssigneeID != "" && len(p.DependsOn) == 0 {
			queueID := uuid.New().String()
			h.DB.Exec(
				"INSERT INTO task_agent_queue (id, task_id, agent_profile_id, status, trigger_type, assigned_at, created_at) VALUES ($1, $2, $3, 'queued', 'sub_task', $4, $4)",
				queueID, taskID, p.AssigneeID, now,
			)
			h.DB.Exec("UPDATE agent_profiles SET current_load = current_load + 1 WHERE id = $1", p.AssigneeID)
			if h.Hub != nil {
				h.Hub.SignalChange("task_agent_queue")
			}
			log.Printf("[Harness] Auto-queued subtask %s for agent %s", taskID[:8], p.AssigneeID[:8])
		}

		if h.Hub != nil {
			h.Hub.SignalChange("tasks")
		}

		log.Printf("[Harness] Agent %s created subtask %s: %s", ctx.AgentName[:8], taskID[:8], p.Title)
		return map[string]interface{}{
			"task_id": taskID,
			"title":   p.Title,
			"status":  "created",
		}, nil
	})

	harness.RegisterExecutor(harness.ToolAddComment, func(ctx *harness.AgentContext, params json.RawMessage) (interface{}, error) {
		var p struct {
			TaskID  string `json:"task_id"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		commentID := uuid.New().String()
		now := time.Now()

		_, err := h.DB.Exec(
			`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, true, $6, $6)`,
			commentID, p.TaskID, ctx.UserID, ctx.AgentProfileID, p.Content, now,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to add comment: %w", err)
		}

		if h.Hub != nil {
			h.Hub.SignalChange("tasks")
		}

		return map[string]interface{}{
			"comment_id": commentID,
			"status":     "created",
		}, nil
	})

	harness.RegisterExecutor(harness.ToolGetTaskDetail, func(ctx *harness.AgentContext, params json.RawMessage) (interface{}, error) {
		var p struct {
			TaskID string `json:"task_id"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		var title, description, status string
		err := h.DB.QueryRow(
			`SELECT title, description, status FROM tasks WHERE id = $1 AND deleted_at IS NULL`,
			p.TaskID,
		).Scan(&title, &description, &status)
		if err != nil {
			return nil, fmt.Errorf("task not found: %w", err)
		}

		deps, _ := h.DAGEngine.GetDependencies(p.TaskID)
		dependents, _ := h.DAGEngine.GetDependents(p.TaskID)

		return map[string]interface{}{
			"task_id":     p.TaskID,
			"title":       title,
			"description": description,
			"status":      status,
			"dependencies": deps,
			"dependents":  dependents,
		}, nil
	})

	harness.RegisterExecutor(harness.ToolListSubTasks, func(ctx *harness.AgentContext, params json.RawMessage) (interface{}, error) {
		var p struct {
			TaskID string `json:"task_id"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		rows, err := h.DB.Query(
			`SELECT id, title, status, depth, completion_behavior FROM tasks
			 WHERE parent_id = $1 AND deleted_at IS NULL
			 ORDER BY created_at ASC`,
			p.TaskID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to query subtasks: %w", err)
		}
		defer rows.Close()

		type SubTask struct {
			ID                 string `json:"id"`
			Title              string `json:"title"`
			Status             string `json:"status"`
			Depth              int    `json:"depth"`
			CompletionBehavior string `json:"completion_behavior"`
		}
		subtasks := make([]SubTask, 0)
		for rows.Next() {
			var st SubTask
			if err := rows.Scan(&st.ID, &st.Title, &st.Status, &st.Depth, &st.CompletionBehavior); err == nil {
				subtasks = append(subtasks, st)
			}
		}
		return map[string]interface{}{
			"task_id":  p.TaskID,
			"subtasks": subtasks,
			"count":    len(subtasks),
		}, nil
	})

	harness.RegisterExecutor(harness.ToolAssignTask, func(ctx *harness.AgentContext, params json.RawMessage) (interface{}, error) {
		var p struct {
			TaskID       string `json:"task_id"`
			AssigneeID   string `json:"assignee_id"`
			AssigneeType string `json:"assignee_type"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		if p.TaskID == "" || p.AssigneeID == "" || p.AssigneeType == "" {
			return nil, fmt.Errorf("task_id, assignee_id, assignee_type are required")
		}

		_, err := h.DB.Exec(
			`UPDATE tasks SET assignee_id = $1, assignee_type = $2, updated_at = $3 WHERE id = $4 AND deleted_at IS NULL`,
			p.AssigneeID, p.AssigneeType, time.Now(), p.TaskID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to assign task: %w", err)
		}

		if h.Hub != nil {
			h.Hub.SignalChange("tasks")
		}

		log.Printf("[Harness] Agent %s assigned task %s to %s (%s)", ctx.AgentName[:8], p.TaskID[:8], p.AssigneeID[:8], p.AssigneeType)
		return map[string]interface{}{
			"status":  "assigned",
			"task_id": p.TaskID,
		}, nil
	})

	harness.RegisterExecutor(harness.ToolReviewTask, func(ctx *harness.AgentContext, params json.RawMessage) (interface{}, error) {
		var p struct {
			TaskID  string `json:"task_id"`
			Action  string `json:"action"`
			Comment string `json:"comment,omitempty"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		if p.TaskID == "" || p.Action == "" {
			return nil, fmt.Errorf("task_id and action are required")
		}

		var newStatus string
		switch p.Action {
		case "approved":
			newStatus = "done"
		case "rejected":
			newStatus = "in_progress"
		default:
			return nil, fmt.Errorf("invalid action: %s (must be approved or rejected)", p.Action)
		}

		_, err := h.DB.Exec(
			`UPDATE tasks SET status = $1, updated_at = $2 WHERE id = $3 AND deleted_at IS NULL`,
			newStatus, time.Now(), p.TaskID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to review task: %w", err)
		}

		if p.Comment != "" {
			commentID := uuid.New().String()
			h.DB.Exec(
				`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5, true, $6, $6)`,
				commentID, p.TaskID, ctx.UserID, ctx.AgentProfileID, p.Comment, time.Now(),
			)
		}

		if h.Hub != nil {
			h.Hub.SignalChange("tasks")
		}

		log.Printf("[Harness] Agent %s reviewed task %s: %s", ctx.AgentName[:8], p.TaskID[:8], p.Action)
		return map[string]interface{}{
			"status":  newStatus,
			"task_id": p.TaskID,
		}, nil
	})

	harness.RegisterExecutor(harness.ToolUpdateStatus, func(ctx *harness.AgentContext, params json.RawMessage) (interface{}, error) {
		var p struct {
			TaskID string `json:"task_id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		if p.TaskID == "" || p.Status == "" {
			return nil, fmt.Errorf("task_id and status are required")
		}

		validStatuses := map[string]bool{"todo": true, "in_progress": true, "completed": true, "blocked": true}
		if !validStatuses[p.Status] {
			return nil, fmt.Errorf("invalid status: %s (must be todo, in_progress, completed, or blocked)", p.Status)
		}

		_, err := h.DB.Exec(
			`UPDATE tasks SET status = $1, updated_at = $2 WHERE id = $3 AND deleted_at IS NULL`,
			p.Status, time.Now(), p.TaskID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update status: %w", err)
		}

		if h.Hub != nil {
			h.Hub.SignalChange("tasks")
		}

		log.Printf("[Harness] Agent %s updated task %s status → %s", ctx.AgentName[:8], p.TaskID[:8], p.Status)
		return map[string]interface{}{
			"status":  p.Status,
			"task_id": p.TaskID,
		}, nil
	})
}
