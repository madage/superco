package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	Notifier    *NotificationHandler
	TaskService *TaskService
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
	harness.RegisterExecutor(harness.ToolProposeDecompositionPlan, func(ctx *harness.AgentContext, params json.RawMessage) (interface{}, error) {
		var p struct {
			Items   []struct {
				Title              string   `json:"title"`
				Description        string   `json:"description"`
				DependsOn          []string `json:"depends_on"`
				ParallelGroup      string   `json:"parallel_group"`
				AssigneeID         string   `json:"assignee_id"`
				AssigneeType       string   `json:"assignee_type"`
				CompletionBehavior string   `json:"completion_behavior"`
			} `json:"items"`
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		if len(p.Items) == 0 {
			return nil, fmt.Errorf("至少需要包含一个子任务")
		}

		now := time.Now()
		planID := uuid.New().String()

		// Check parent task exists and is in a decomposable state
		if ctx.TaskID == nil || *ctx.TaskID == "" {
			return nil, fmt.Errorf("必须在任务上下文中提出分解计划")
		}

		var parentStatus string
		h.DB.QueryRow(`SELECT status FROM tasks WHERE id = $1 AND deleted_at IS NULL`, *ctx.TaskID).Scan(&parentStatus)
		if parentStatus != "in_progress" && parentStatus != "todo" {
			return nil, fmt.Errorf("当前任务状态为 %s，不能提出分解计划", parentStatus)
		}

		// Prevent duplicate decomposition: if an approved plan already exists, deny
		var hasApprovedPlan bool
		h.DB.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM decomposition_plans WHERE task_id = $1 AND status = 'approved')`,
			*ctx.TaskID,
		).Scan(&hasApprovedPlan)
		if hasApprovedPlan {
			return nil, fmt.Errorf("该任务已有审核通过的分解计划，子任务正在执行中，无需重复分解")
		}

		// Cancel any pending plan for this task
		h.DB.Exec(`UPDATE decomposition_plans SET status = 'rejected' WHERE task_id = $1 AND status = 'pending'`, *ctx.TaskID)

		// Insert plan
		agentName := ctx.AgentName
		if agentName == "" {
			agentName = "Unknown"
		}
		h.DB.Exec(
			`INSERT INTO decomposition_plans (id, task_id, status, created_by, created_by_name, summary, created_at, updated_at)
			 VALUES ($1, $2, 'pending', $3, $4, $5, $6, $6)`,
			planID, *ctx.TaskID, ctx.AgentProfileID, agentName, p.Summary, now,
		)

		// Insert plan items
		for i, item := range p.Items {
			itemID := uuid.New().String()
			assigneeName := ""
			if item.AssigneeID != "" {
				h.DB.QueryRow(`SELECT COALESCE(name,'') FROM agent_profiles WHERE id = $1`, item.AssigneeID).Scan(&assigneeName)
			}
			dependsJSON, _ := json.Marshal(item.DependsOn)
			cb := item.CompletionBehavior
			if cb == "" {
				cb = models.CompletionNeedsReview
			}
			h.DB.Exec(
				`INSERT INTO decomposition_plan_items
				 (id, plan_id, title, description, assignee_id, assignee_type, assignee_name,
				  depends_on, parallel_group, sort_order, completion_behavior, created_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
				itemID, planID, item.Title, item.Description,
				item.AssigneeID, item.AssigneeType, assigneeName,
				dependsJSON, item.ParallelGroup, i, cb, now,
			)
		}

		// Post summary comment
		var creatorName string
		h.DB.QueryRow(`SELECT COALESCE(username,'') FROM users WHERE id = $1`, ctx.UserID).Scan(&creatorName)
		mentionStr := ""
		if creatorName != "" {
			mentionStr = "@" + creatorName
		}
		commentContent := fmt.Sprintf("%s\n\n智能体「%s」提出了分解计划（共 %d 个子任务），请审核。\n\n概要：%s",
			mentionStr, ctx.AgentName, len(p.Items), p.Summary)
		commentID := uuid.New().String()
		h.DB.Exec(
			`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, true, $6, $6)`,
			commentID, *ctx.TaskID, ctx.UserID, ctx.AgentProfileID, commentContent, now,
		)

		// Keep parent in "in_progress" (do NOT change status to review)
		h.DB.Exec(`UPDATE tasks SET updated_at = $1 WHERE id = $2`, now, *ctx.TaskID)

		if h.Hub != nil {
			h.Hub.SignalChange("tasks")
		}

		log.Printf("[Harness] Agent %s proposed decomposition plan %s for task %s (%d items)",
			safe8(ctx.AgentName), planID[:8], (*ctx.TaskID)[:8], len(p.Items))

		return map[string]interface{}{
			"plan_id": planID,
			"status":  "pending_review",
			"message": "分解计划已提交，等待人工审核",
			"count":   len(p.Items),
		}, nil
	})

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

		// === Decomposition flow: convert create_sub_task to plan items ===
		// If the agent is calling create_sub_task on a parent they are assigned to,
		// auto-create a decomposition plan so the human can review before real tasks are created.
		if ctx.TaskID != nil && *ctx.TaskID != "" {
			var parentStatus, parentAssigneeID string
			h.DB.QueryRow(`SELECT status, COALESCE(assignee_id,'') FROM tasks WHERE id = $1 AND deleted_at IS NULL`,
				*ctx.TaskID).Scan(&parentStatus, &parentAssigneeID)

			// Only convert if parent is assigned to the calling agent and is in decomposable state
			log.Printf("[Harness] create_sub_task check: parent=%s agent_profile=%s parentStatus=%s assignee=%s",
				(*ctx.TaskID)[:8], ctx.AgentProfileID[:8], parentStatus, parentAssigneeID[:8])
			if parentAssigneeID == ctx.AgentProfileID && (parentStatus == "in_progress" || parentStatus == "todo" || parentStatus == "review") {
				// Find or create a pending plan
				var planID string
				err := h.DB.QueryRow(
					`SELECT id FROM decomposition_plans WHERE task_id = $1 AND status = 'pending' LIMIT 1`,
					*ctx.TaskID,
				).Scan(&planID)
				if err != nil {
					// Create a new plan automatically
					planID = uuid.New().String()
					agentName := ctx.AgentName
					if agentName == "" {
						agentName = "Unknown"
					}
					h.DB.Exec(
						`INSERT INTO decomposition_plans (id, task_id, status, created_by, created_by_name, summary, created_at, updated_at)
						 VALUES ($1, $2, 'pending', $3, $4, $5, $6, $6)`,
						planID, *ctx.TaskID, ctx.AgentProfileID, agentName,
						"智能体通过 create_sub_task 自动创建了分解计划", now,
					)
				}

				// Add as plan item instead of real task
				// Deduplicate: skip if same title already exists in this plan
				var existingID string
				err = h.DB.QueryRow(
					`SELECT id FROM decomposition_plan_items WHERE plan_id = $1 AND title = $2 LIMIT 1`,
					planID, p.Title,
				).Scan(&existingID)
				if err == nil {
					log.Printf("[Harness] Duplicate plan item skipped: %s (already exists as %s)", p.Title[:20], existingID[:8])
					h.DB.Exec(`UPDATE tasks SET updated_at = $1 WHERE id = $2`, now, *ctx.TaskID)
					return map[string]interface{}{
						"status":       "plan_item_exists",
						"plan_item_id": existingID,
						"plan_id":      planID,
						"message":      "子任务已在分解计划中（跳过重复）",
						"title":        p.Title,
					}, nil
				}
				itemID := uuid.New().String()
				assigneeName := ""
				resolvedAssigneeID := p.AssigneeID
				if p.AssigneeID != "" && len(p.AssigneeID) < 36 {
					var fullID string
					if err := h.DB.QueryRow(
						`SELECT id FROM agent_profiles WHERE id LIKE $1 || '%' AND enabled = true LIMIT 1`,
						p.AssigneeID,
					).Scan(&fullID); err == nil {
						resolvedAssigneeID = fullID
					}
				}
				if resolvedAssigneeID != "" {
					h.DB.QueryRow(`SELECT COALESCE(name,'') FROM agent_profiles WHERE id = $1`, resolvedAssigneeID).Scan(&assigneeName)
				}

				// Validate depends_on IDs exist in the current plan
				if len(p.DependsOn) > 0 {
					var badIDs []string
					for _, depID := range p.DependsOn {
						if depID == "" {
							continue
						}
						var exists bool
						h.DB.QueryRow(
							`SELECT EXISTS(SELECT 1 FROM decomposition_plan_items WHERE plan_id = $1 AND id = $2)`,
							planID, depID,
						).Scan(&exists)
						if !exists {
							badIDs = append(badIDs, depID)
						}
					}
					if len(badIDs) > 0 {
						// Increment error count on the plan
						h.DB.Exec(`UPDATE decomposition_plans SET error_count = COALESCE(error_count, 0) + 1, updated_at = $1 WHERE id = $2`, now, planID)
						var errorCount int
						h.DB.QueryRow(`SELECT COALESCE(error_count, 0) FROM decomposition_plans WHERE id = $1`, planID).Scan(&errorCount)
						
						log.Printf("[Harness] Invalid depends_on IDs for plan %s: %v (error_count=%d)", planID[:8], badIDs, errorCount)
						
						if errorCount >= 3 {
							// Circuit breaker: mark plan as failed, block parent task, notify user
							h.DB.Exec(`UPDATE decomposition_plans SET status = 'failed', updated_at = $1 WHERE id = $2`, now, planID)
							opts := TransitionOpts{ActorID: ctx.AgentProfileID}
							if err := h.TaskService.MarkBlocked(*ctx.TaskID, opts); err != nil {
								log.Printf("[Harness] Failed to block task %s: %v", (*ctx.TaskID)[:8], err)
							}
							
							// Post critical notification comment
							commentID := uuid.New().String()
							commentContent := fmt.Sprintf("【严重错误】智能体「%s」已连续3次提交无效的依赖关系，任务已自动阻塞。请驳回计划后重新尝试。", ctx.AgentName)
							h.DB.Exec(
								`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
								 VALUES ($1, $2, $3, NULL, $4, true, $5, $5)`,
								commentID, *ctx.TaskID, ctx.UserID, commentContent, now,
							)

							if h.Hub != nil {
								h.Hub.SignalChange("tasks")
							}

							log.Printf("[Harness] CIRCUIT BREAKER: plan %s failed after 3 errors, task %s blocked", planID[:8], (*ctx.TaskID)[:8])
							return nil, fmt.Errorf("【严重错误】分解计划已连续3次提交无效的依赖关系，任务已被阻塞。请驳回计划后重试")
						}

					}
				}
				dependsJSON, _ := json.Marshal(p.DependsOn)
				cb := p.CompletionBehavior
				if cb == "" {
					cb = models.CompletionNeedsReview
				}

				// Count existing items for sort_order
				var sortOrder int
				h.DB.QueryRow(`SELECT COUNT(*) FROM decomposition_plan_items WHERE plan_id = $1`, planID).Scan(&sortOrder)

				_, err = h.DB.Exec(
					`INSERT INTO decomposition_plan_items
					 (id, plan_id, title, description, assignee_id, assignee_type, assignee_name,
					  depends_on, parallel_group, sort_order, completion_behavior, created_at)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
					itemID, planID, p.Title, p.Description,
					resolvedAssigneeID, p.AssigneeType, assigneeName,
					dependsJSON, p.ParallelGroup, sortOrder, cb, now,
				)
				if err != nil {
					// Unique constraint violation — duplicate title in same plan, return existing item
					h.DB.QueryRow(
						`SELECT id FROM decomposition_plan_items WHERE plan_id = $1 AND title = $2 LIMIT 1`,
						planID, p.Title,
					).Scan(&itemID)
					log.Printf("[Harness] Duplicate plan item (unique constraint): %s → %s", p.Title[:20], itemID[:8])
				}

				// Keep parent in_progress
				h.DB.Exec(`UPDATE tasks SET updated_at = $1 WHERE id = $2`, now, *ctx.TaskID)

				if h.Hub != nil {
					h.Hub.SignalChange("tasks")
				}

				log.Printf("[Harness] Agent %s create_sub_task → plan item %s (plan=%s)", safe8(ctx.AgentName), itemID[:8], planID[:8])
				return map[string]interface{}{
					"status":       "plan_item_created",
					"plan_item_id": itemID,
					"plan_id":      planID,
					"message":      "子任务已加入分解计划，等待人工审核通过后创建实际任务",
					"title":        p.Title,
				}, nil
			}
		}
		// === End decomposition flow ===

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

		// Review gate: if assigning to another agent, require human approval first
		if p.AssigneeType == "agent_profile" && p.AssigneeID != "" && ctx.TaskID != nil {
			// --- Circular delegation prevention ---
			if p.AssigneeID == ctx.AgentProfileID {
				return nil, fmt.Errorf("不能将子任务委派给自己，请选择其他智能体")
			}
			if isCircularDelegation(h.DB, *ctx.TaskID, p.AssigneeID) {
				return nil, fmt.Errorf("目标智能体已在祖先任务链中，不能循环委派")
			}
			// --- End circular delegation prevention ---
			var enabled bool
			h.DB.QueryRow(`SELECT enabled FROM agent_profiles WHERE id = $1`, p.AssigneeID).Scan(&enabled)

			// Store pending action in parent task
			action := map[string]interface{}{
				"type":              "create_sub_task",
				"sub_task_id":       taskID,
				"target_agent_id":   p.AssigneeID,
				"title":             p.Title,
				"target_agent_name": "",
			}
			var targetAgentName string
			h.DB.QueryRow(`SELECT COALESCE(name,'') FROM agent_profiles WHERE id = $1`, p.AssigneeID).Scan(&targetAgentName)
			action["target_agent_name"] = targetAgentName
			actionJSON, _ := json.Marshal([]interface{}{action})

			// Set pending_review_actions, then delegate status change to TaskService
			h.DB.Exec(
				`UPDATE tasks SET pending_review_actions = $1, updated_at = $2 WHERE id = $3 AND deleted_at IS NULL`,
				actionJSON, now, *ctx.TaskID,
			)
			opts := TransitionOpts{ActorID: ctx.AgentProfileID}
			if err := h.TaskService.MarkReview(*ctx.TaskID, opts); err != nil {
				log.Printf("[Harness] Failed to set task %s to review: %v", (*ctx.TaskID)[:8], err)
			}

			// Build @mention comment for human users
			var creatorName string
			h.DB.QueryRow(`SELECT COALESCE(username,'') FROM users WHERE id = $1`, ctx.UserID).Scan(&creatorName)

			assigneeRows, _ := h.DB.Query(
				`SELECT assignee_id FROM task_assignees WHERE task_id = $1 AND assignee_type = 'user'`,
				*ctx.TaskID,
			)
			humanMentions := []string{}
			if creatorName != "" {
				humanMentions = append(humanMentions, "@"+creatorName)
			}
			if assigneeRows != nil {
				for assigneeRows.Next() {
					var uid string
					assigneeRows.Scan(&uid)
					var uname string
					h.DB.QueryRow(`SELECT username FROM users WHERE id = $1`, uid).Scan(&uname)
					if uname != "" && uname != creatorName {
						humanMentions = append(humanMentions, "@"+uname)
					}
				}
				assigneeRows.Close()
			}
			// Also check parent task's assignee
			var pAssigneeID, pAssigneeType string
			h.DB.QueryRow(`SELECT COALESCE(assignee_id,''), COALESCE(assignee_type,'') FROM tasks WHERE id = $1`, *ctx.TaskID).Scan(&pAssigneeID, &pAssigneeType)
			if pAssigneeType == "user" && pAssigneeID != "" {
				var uname string
				h.DB.QueryRow(`SELECT username FROM users WHERE id = $1`, pAssigneeID).Scan(&uname)
				mentioned := false
				for _, m := range humanMentions {
					if m == "@"+uname {
						mentioned = true
						break
					}
				}
				if uname != "" && !mentioned {
					humanMentions = append(humanMentions, "@"+uname)
				}
			}

			mentionStr := strings.Join(humanMentions, " ")
			targetName, _ := action["target_agent_name"].(string)
			commentContent := fmt.Sprintf("%s\n\n智能体「%s」请求将子任务「%s」派发给智能体「%s」，请审核。",
				mentionStr, ctx.AgentName, p.Title, targetName)

			commentID := uuid.New().String()
			h.DB.Exec(
				`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5, true, $6, $6)`,
				commentID, *ctx.TaskID, ctx.UserID, ctx.AgentProfileID, commentContent, now,
			)

			if h.Hub != nil {
				h.Hub.SignalChange("tasks")
			}

			log.Printf("[Harness] Agent %s created subtask %s → parent %s set to review (pending human approval)", safe8(ctx.AgentName), taskID[:8], (*ctx.TaskID)[:8])
			return map[string]interface{}{
				"task_id": taskID,
				"title":   p.Title,
				"status":  "pending_review",
				"message": "等待人工审核通过后派发",
			}, nil
		}

		// Auto-start: if assigned to an agent and not blocked by dependencies,
		// create a queue entry so the runtime picks it up.
		if p.AssigneeType == "agent_profile" && p.AssigneeID != "" && len(p.DependsOn) == 0 {
			var enabled bool
			h.DB.QueryRow(`SELECT enabled FROM agent_profiles WHERE id = $1`, p.AssigneeID).Scan(&enabled)
			if !enabled {
				log.Printf("[Harness] Skipping auto-queue for disabled agent %s", p.AssigneeID[:8])
			} else {
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
		}

		if h.Hub != nil {
			h.Hub.SignalChange("tasks")
		}

		log.Printf("[Harness] Agent %s created subtask %s: %s", safe8(ctx.AgentName), taskID[:8], p.Title)
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

		// Use nil for empty profile ID to satisfy nullable FK constraint.
		var agentProfileID interface{}
		if ctx.AgentProfileID != "" {
			agentProfileID = ctx.AgentProfileID
		}

		// Dedup: if same agent already posted identical content to this task
		// within 15s, return the existing comment ID instead of inserting.
		if agentProfileID != nil {
			var dupID string
			if err := h.DB.QueryRow(
				`SELECT id FROM task_comments WHERE task_id = $1 AND agent_profile_id = $2 AND content = $3
				 AND created_at > NOW() - INTERVAL '15 seconds' LIMIT 1`,
				p.TaskID, agentProfileID, p.Content,
			).Scan(&dupID); err == nil {
				return map[string]interface{}{
					"comment_id": dupID,
					"status":     "duplicate",
				}, nil
			}
		}

		commentID := uuid.New().String()
		now := time.Now()

		_, err := h.DB.Exec(
			`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, true, $6, $6)`,
			commentID, p.TaskID, ctx.UserID, agentProfileID, p.Content, now,
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

		now := time.Now()

		// Review gate: assigning to agent requires human approval
		if p.AssigneeType == "agent_profile" {
			// --- Circular delegation prevention ---
			if p.AssigneeID == ctx.AgentProfileID {
				return nil, fmt.Errorf("不能将任务委派给自己，请选择其他智能体")
			}
			if isCircularDelegation(h.DB, p.TaskID, p.AssigneeID) {
				return nil, fmt.Errorf("目标智能体已在祖先任务链中，不能循环委派")
			}
			// --- End circular delegation prevention ---
			// Update assignee but don't dispatch
			_, err := h.DB.Exec(
				`UPDATE tasks SET assignee_id = $1, assignee_type = $2, status = 'review', updated_at = $3 WHERE id = $4 AND deleted_at IS NULL`,
				p.AssigneeID, p.AssigneeType, now, p.TaskID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to assign task: %w", err)
			}

			// Store pending action
			var targetName string
			h.DB.QueryRow(`SELECT COALESCE(name,'') FROM agent_profiles WHERE id = $1`, p.AssigneeID).Scan(&targetName)
			action := map[string]interface{}{
				"type":              "assign_task",
				"target_agent_id":   p.AssigneeID,
				"target_agent_name": targetName,
			}
			actionJSON, _ := json.Marshal([]interface{}{action})
			h.DB.Exec(`UPDATE tasks SET pending_review_actions = $1 WHERE id = $2`, actionJSON, p.TaskID)

			// Build @mention comment
			var creatorName string
			h.DB.QueryRow(`SELECT t.user_id, COALESCE(u.username,'') FROM tasks t LEFT JOIN users u ON u.id = t.user_id WHERE t.id = $1`, p.TaskID).Scan(&creatorName)
			humanMentions := []string{}
			if creatorName != "" {
				humanMentions = append(humanMentions, "@"+creatorName)
			}
			assigneeRows, _ := h.DB.Query(
				`SELECT assignee_id FROM task_assignees WHERE task_id = $1 AND assignee_type = 'user'`,
				p.TaskID,
			)
			if assigneeRows != nil {
				for assigneeRows.Next() {
					var uid string
					assigneeRows.Scan(&uid)
					var uname string
					h.DB.QueryRow(`SELECT username FROM users WHERE id = $1`, uid).Scan(&uname)
					if uname != "" && uname != creatorName {
						humanMentions = append(humanMentions, "@"+uname)
					}
				}
				assigneeRows.Close()
			}

			mentionStr := strings.Join(humanMentions, " ")
			commentContent := fmt.Sprintf("%s\n\n智能体「%s」请求将任务指派给智能体「%s」，请审核。",
				mentionStr, ctx.AgentName, targetName)
			commentID := uuid.New().String()
			h.DB.Exec(
				`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5, true, $6, $6)`,
				commentID, p.TaskID, ctx.UserID, ctx.AgentProfileID, commentContent, now,
			)

			if h.Hub != nil {
				h.Hub.SignalChange("tasks")
			}

			log.Printf("[Harness] Agent %s assigned task %s to agent %s → review (pending human approval)", safe8(ctx.AgentName), p.TaskID[:8], p.AssigneeID[:8])
			return map[string]interface{}{
				"status":   "pending_review",
				"task_id":  p.TaskID,
				"assignee_id": p.AssigneeID,
				"message":  "等待人工审核通过后生效",
			}, nil
		}

		// Direct assignment to user
		_, err := h.DB.Exec(
			`UPDATE tasks SET assignee_id = $1, assignee_type = $2, updated_at = $3 WHERE id = $4 AND deleted_at IS NULL`,
			p.AssigneeID, p.AssigneeType, now, p.TaskID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to assign task: %w", err)
		}

		if h.Hub != nil {
			h.Hub.SignalChange("tasks")
		}

		log.Printf("[Harness] Agent %s assigned task %s to %s (%s)", safe8(ctx.AgentName), p.TaskID[:8], p.AssigneeID[:8], p.AssigneeType)
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

			// Agents cannot review tasks with pending dispatch actions — requires human approval
			var pendingActions []byte
			h.DB.QueryRow(`SELECT pending_review_actions FROM tasks WHERE id = $1 AND deleted_at IS NULL`, p.TaskID).Scan(&pendingActions)
			if len(pendingActions) > 5 {
				return nil, fmt.Errorf("该任务有待人工审核的操作，智能体不能审核，请联系管理员")
			}

			if p.Action != "approved" && p.Action != "rejected" {
				return nil, fmt.Errorf("invalid action: %s (must be approved or rejected)", p.Action)
			}

			err := h.TaskService.HandleReview(p.TaskID, nil, &ctx.AgentProfileID, p.Action, p.Comment)
			if err != nil {
				return nil, fmt.Errorf("failed to review task: %w", err)
			}

			log.Printf("[Harness] Agent %s reviewed task %s: %s", safe8(ctx.AgentName), p.TaskID[:8], p.Action)
			return map[string]interface{}{
				"status":  p.Action,
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

			validStatuses := map[string]bool{"todo": true, "in_progress": true, "completed": true, "blocked": true, "done": true, "review": true}
			if !validStatuses[p.Status] {
				return nil, fmt.Errorf("invalid status: %s (must be todo, in_progress, completed, blocked, done, or review)", p.Status)
			}

			opts := TransitionOpts{
				AgentProfileID: ctx.AgentProfileID,
				ActorID:        ctx.AgentProfileID,
			}

			var err error
			switch p.Status {
			case "completed", "done":
				err = h.TaskService.MarkCompleted(p.TaskID, opts)
			case "todo":
				err = h.TaskService.MarkTodo(p.TaskID, opts)
			case "in_progress":
				err = h.TaskService.MarkInProgress(p.TaskID, opts)
			case "blocked":
				err = h.TaskService.MarkBlocked(p.TaskID, opts)
			case "review":
				err = h.TaskService.MarkReview(p.TaskID, opts)
			}
			if err != nil {
				return nil, fmt.Errorf("failed to update status: %w", err)
			}

			// Release agent load for terminal statuses (tool-call path doesn't go through UpdateQueueStatus)
			if p.Status == "completed" || p.Status == "done" || p.Status == "blocked" || p.Status == "review" {
				h.DB.Exec(`UPDATE agent_profiles SET current_load = GREATEST(0, current_load - 1) WHERE id = $1`, ctx.AgentProfileID)
			}

			log.Printf("[Harness] Agent %s updated task %s status → %s", safe8(ctx.AgentName), p.TaskID[:8], p.Status)
			return map[string]interface{}{
				"status":  p.Status,
				"task_id": p.TaskID,
			}, nil
		})

	harness.RegisterExecutor(harness.ToolSearchAgentProfiles, func(ctx *harness.AgentContext, params json.RawMessage) (interface{}, error) {
		var p struct {
			Name       string   `json:"name"`
			Tags       []string `json:"tags"`
			Capability string   `json:"capability"`
			Limit      int      `json:"limit"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		if p.Limit <= 0 {
			p.Limit = 10
		}
		if p.Limit > 20 {
			p.Limit = 20
		}

		// Get the calling agent's workspace_id
		var workspaceID string
		h.DB.QueryRow(
			`SELECT COALESCE(workspace_id,'') FROM agent_profiles WHERE id = $1`,
			ctx.AgentProfileID,
		).Scan(&workspaceID)

		query := `SELECT id, name, COALESCE(description,''), COALESCE(tags,'[]'::jsonb),
				COALESCE(capabilities,'[]'::jsonb), COALESCE(current_load,0),
				enabled, last_active_at
			 FROM agent_profiles
			 WHERE enabled = true
			   AND id != $1`
		args := []interface{}{ctx.AgentProfileID}
		argIdx := 2

		if workspaceID != "" {
			query += fmt.Sprintf(" AND workspace_id = $%d", argIdx)
			args = append(args, workspaceID)
			argIdx++
		}

		if p.Name != "" {
			query += fmt.Sprintf(" AND name ILIKE $%d", argIdx)
			args = append(args, "%"+p.Name+"%")
			argIdx++
		}

		if len(p.Tags) > 0 {
			tagsJSON, _ := json.Marshal(p.Tags)
			query += fmt.Sprintf(" AND tags ?| $%d", argIdx)
			args = append(args, string(tagsJSON))
			argIdx++
		}

		if p.Capability != "" {
			capArray, _ := json.Marshal([]string{p.Capability})
			query += fmt.Sprintf(" AND capabilities @> $%d", argIdx)
			args = append(args, string(capArray))
			argIdx++
		}

		query += fmt.Sprintf(" ORDER BY name LIMIT $%d", argIdx)
		args = append(args, p.Limit)

		rows, err := h.DB.Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to query agent profiles: %w", err)
		}
		defer rows.Close()

		type AgentInfo struct {
			ID           string   `json:"id"`
			Name         string   `json:"name"`
			Description  string   `json:"description"`
			Tags         []string `json:"tags"`
			Capabilities []string `json:"capabilities"`
			CurrentLoad  int      `json:"current_load"`
			Enabled      bool     `json:"enabled"`
			LastActiveAt *string  `json:"last_active_at"`
		}

		agents := make([]AgentInfo, 0)
		for rows.Next() {
			var a AgentInfo
			var tagsJSON, capsJSON string
			if err := rows.Scan(&a.ID, &a.Name, &a.Description, &tagsJSON, &capsJSON,
				&a.CurrentLoad, &a.Enabled, &a.LastActiveAt); err != nil {
				continue
			}
			json.Unmarshal([]byte(tagsJSON), &a.Tags)
			json.Unmarshal([]byte(capsJSON), &a.Capabilities)
			agents = append(agents, a)
		}

		log.Printf("[Harness] Agent %s searched agent profiles (name=%q tags=%v capability=%s) → %d results",
			safe8(ctx.AgentName), p.Name, p.Tags, p.Capability, len(agents))

		return map[string]interface{}{
			"agents": agents,
			"total":  len(agents),
			"query": map[string]interface{}{
				"name":       p.Name,
				"tags":       p.Tags,
				"capability": p.Capability,
				"limit":      p.Limit,
			},
		}, nil
	})
}

// isCircularDelegation traverses the ancestor chain of a task to check if
// targetAgentID appears as an agent_profile assignee anywhere in the chain.
func isCircularDelegation(db *sql.DB, taskID string, targetAgentID string) bool {
	currentID := taskID
	for currentID != "" {
		var assigneeID, assigneeType, parentID string
		err := db.QueryRow(
			`SELECT COALESCE(assignee_id,''), COALESCE(assignee_type,''), COALESCE(parent_id,'')
			 FROM tasks WHERE id = $1 AND deleted_at IS NULL`,
			currentID,
		).Scan(&assigneeID, &assigneeType, &parentID)
		if err != nil {
			return false
		}
		if assigneeType == "agent_profile" && assigneeID == targetAgentID {
			return true
		}
		currentID = parentID
	}
	return false
}

func safe8(s string) string {
	if len(s) >= 8 {
		return s[:8]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

