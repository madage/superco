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
	"github.com/lib/pq"
)

// DecompositionHandler handles decomposition plan review operations.
type DecompositionHandler struct {
	DB           *sql.DB
	Hub          *DashboardHub
	ReviewRouter *ReviewRouter
	DAGEngine    *DAGEngine
	TaskService  *TaskService // unified status transition (set after creation)
}

// NewDecompositionHandler creates a new DecompositionHandler.
func NewDecompositionHandler(db *sql.DB) *DecompositionHandler {
	return &DecompositionHandler{
		DB:        db,
		DAGEngine: NewDAGEngine(db),
	}
}

// PlanResponse represents a decomposition plan in API responses.
type PlanResponse struct {
	ID            string    `json:"id"`
	TaskID        string    `json:"task_id"`
	Status        string    `json:"status"`
	CreatedBy     string    `json:"created_by"`
	CreatedByName string    `json:"created_by_name"`
	Summary       string    `json:"summary"`
	CreatedAt     time.Time `json:"created_at"`
}

// ItemResponse represents a decomposition plan item in API responses.
type ItemResponse struct {
	ID                 string          `json:"id"`
	PlanID             string          `json:"plan_id"`
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	AssigneeID         string          `json:"assignee_id"`
	AssigneeType       string          `json:"assignee_type"`
	AssigneeName       string          `json:"assignee_name"`
	DependsOn          json.RawMessage `json:"depends_on"`
	ParallelGroup      string          `json:"parallel_group"`
	SortOrder          int             `json:"sort_order"`
	IsApproved         *bool           `json:"is_approved"`
	RealTaskID         *string         `json:"real_task_id"`
	CompletionBehavior string          `json:"completion_behavior"`
	CreatedAt          time.Time       `json:"created_at"`
}

// GetPlan returns the decomposition plan and items for a task.
func (h *DecompositionHandler) GetPlan(c *gin.Context) {
	taskID := c.Param("id")

	var plan PlanResponse
	err := h.DB.QueryRow(
		`SELECT id, task_id, status, created_by, created_by_name, COALESCE(summary,''), created_at
		 FROM decomposition_plans WHERE task_id = $1 AND status = 'pending'
		 ORDER BY created_at DESC LIMIT 1`,
		taskID,
	).Scan(&plan.ID, &plan.TaskID, &plan.Status, &plan.CreatedBy, &plan.CreatedByName, &plan.Summary, &plan.CreatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusOK, gin.H{"plan": nil, "items": []interface{}{}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query plan"})
		return
	}

	rows, err := h.DB.Query(
		`SELECT id, plan_id, title, COALESCE(description,''), COALESCE(assignee_id,''),
		        COALESCE(assignee_type,''), COALESCE(assignee_name,''),
		        depends_on, COALESCE(parallel_group,''), sort_order,
		        is_approved, real_task_id, completion_behavior, created_at
		 FROM decomposition_plan_items
		 WHERE plan_id = $1
		 ORDER BY sort_order ASC`,
		plan.ID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query items"})
		return
	}
	defer rows.Close()

	items := make([]ItemResponse, 0)
	for rows.Next() {
		var item ItemResponse
		if err := rows.Scan(&item.ID, &item.PlanID, &item.Title, &item.Description,
			&item.AssigneeID, &item.AssigneeType, &item.AssigneeName,
			&item.DependsOn, &item.ParallelGroup, &item.SortOrder,
			&item.IsApproved, &item.RealTaskID, &item.CompletionBehavior, &item.CreatedAt); err != nil {
			continue
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"plan": plan, "items": items})
}

// ApprovePlan approves selected (or all) items in a decomposition plan.
func (h *DecompositionHandler) ApprovePlan(c *gin.Context) {
	taskID := c.Param("id")

	var req struct {
		ItemIDs []string `json:"item_ids,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)
	now := time.Now()

	// Get the pending plan
	var planID, createdBy string
	err := h.DB.QueryRow(
		`SELECT id, created_by FROM decomposition_plans
		 WHERE task_id = $1 AND status = 'pending'
		 ORDER BY created_at DESC LIMIT 1`,
		taskID,
	).Scan(&planID, &createdBy)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "没有待审核的分解计划"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query plan"})
		return
	}

	// Query items to approve
	query := `SELECT id, title, COALESCE(description,''), COALESCE(assignee_id,''),
	                 COALESCE(assignee_type,''), depends_on, COALESCE(parallel_group,''),
	                 completion_behavior, sort_order
	          FROM decomposition_plan_items WHERE plan_id = $1`
	args := []interface{}{planID}

	if len(req.ItemIDs) > 0 {
		// Build WHERE id = ANY(...)
		query += ` AND id = ANY($2)`
		args = append(args, pq.Array(req.ItemIDs))
	} else {
		query += ` AND is_approved IS NULL`
	}
	query += ` ORDER BY sort_order ASC`

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query plan items"})
		return
	}
	defer rows.Close()

	type pendingItem struct {
		ID                 string
		Title              string
		Description        string
		AssigneeID         string
		AssigneeType       string
		DependsOn          []byte
		ParallelGroup      string
		CompletionBehavior string
		SortOrder          int
	}

	var pendingItems []pendingItem
	for rows.Next() {
		var item pendingItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Description,
			&item.AssigneeID, &item.AssigneeType, &item.DependsOn,
			&item.ParallelGroup, &item.CompletionBehavior, &item.SortOrder); err != nil {
			continue
		}
		pendingItems = append(pendingItems, item)
	}

	if len(pendingItems) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有可审核的子任务项"})
		return
	}

	// Secondary validation: resolve depends_on (titles or IDs) to actual plan item IDs
	itemIDSet := make(map[string]bool, len(pendingItems))
	titleToID := make(map[string]string, len(pendingItems))
	for _, item := range pendingItems {
		itemIDSet[item.ID] = true
		titleToID[item.Title] = item.ID
		titleToID[strings.TrimSpace(item.Title)] = item.ID
	}

	var badDeps []string
	resolvedDeps := make(map[string][]string) // item ID → resolved dependency IDs

	for _, item := range pendingItems {
		var deps []string
		if err := json.Unmarshal(item.DependsOn, &deps); err != nil || len(deps) == 0 {
			continue
		}
		var resolved []string
		for _, dep := range deps {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			// 1) Direct ID match
			if itemIDSet[dep] {
				resolved = append(resolved, dep)
				continue
			}
			// 2) Exact title match
			if id, ok := titleToID[dep]; ok {
				resolved = append(resolved, id)
				continue
			}
			// 3) Case-insensitive title match
			found := false
			for title, id := range titleToID {
				if strings.EqualFold(title, dep) {
					resolved = append(resolved, id)
					found = true
					break
				}
			}
			if !found {
				badDeps = append(badDeps, fmt.Sprintf("%s depends_on '%s' 未找到对应项", item.Title, dep))
			}
		}
		if len(resolved) > 0 {
			resolvedDeps[item.ID] = resolved
		}
	}

	if len(badDeps) > 0 {
		// Circuit breaker: mark plan as failed, block parent task, notify user
		h.DB.Exec(`UPDATE decomposition_plans SET status = 'failed', updated_at = $1 WHERE id = $2`, now, planID)
		opts := TransitionOpts{ActorID: userIDStr}
		if err := h.TaskService.MarkBlocked(taskID, opts); err != nil {
			log.Printf("[Decomposition] Failed to block task %s: %v", taskID[:8], err)
		}
		commentContent := fmt.Sprintf("【严重错误】分解计划中存在无效的依赖关系：\n%s\n任务已被阻塞，请驳回计划后重新尝试。", strings.Join(badDeps, "\n"))
		commentID := uuid.New().String()
		h.DB.Exec(
			`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
			 VALUES ($1, $2, $3, NULL, $4, false, $5, $5)`,
			commentID, taskID, userIDStr, commentContent, now,
		)
		if h.Hub != nil {
			h.Hub.SignalChange("tasks")
		}
		log.Printf("[Decomposition] APPROVE REJECTED: plan %s has invalid depends_on: %v", planID[:8], badDeps)
			InsertAppEvent(h.DB, "error", "decomposition", "分解计划审批失败: 无效依赖",
				fmt.Sprintf("Plan %s: %v", planID[:8], badDeps), taskID, "")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   fmt.Sprintf("分解计划包含无效的依赖关系，审批已被拒绝。%s", strings.Join(badDeps, "; ")),
			"blocked": true,
		})
		return
	}


	// Phase 1: Create all tasks
	type createdTask struct {
		ItemID       string
		TaskID       string
		DependsOn    []string
		AssigneeID   string
		AssigneeType string
	}

	var createdTasks []createdTask
	for _, item := range pendingItems {
		newTaskID := uuid.New().String()

		// Use resolved dependency IDs from the secondary validation above
		dependsOnResolved := resolvedDeps[item.ID]
		if dependsOnResolved == nil {
			dependsOnResolved = []string{}
		}

		// Inherit workspace_id from parent task
		var workspaceID sql.NullString
		h.DB.QueryRow(`SELECT workspace_id FROM tasks WHERE id = $1`, taskID).Scan(&workspaceID)

		cb := item.CompletionBehavior
		if cb == "" {
			cb = "needs_review"
		}
		// Resolve assignee_id — may be truncated UUID, agent name, or keyword
		assigneeID := item.AssigneeID
		if assigneeID != "" && len(assigneeID) < 36 && item.AssigneeType == "agent_profile" {
			var fullID string
			// 1) UUID prefix match
			if err := h.DB.QueryRow(
				`SELECT id FROM agent_profiles WHERE id LIKE $1 || '%' AND enabled = true LIMIT 1`,
				assigneeID,
			).Scan(&fullID); err == nil {
				log.Printf("[Decomposition] Resolved assignee %s → %s", assigneeID, fullID[:8])
				assigneeID = fullID
			} else if err2 := h.DB.QueryRow(
				// 2) Exact name match
				`SELECT id FROM agent_profiles WHERE name = $1 AND enabled = true LIMIT 1`,
				assigneeID,
			).Scan(&fullID); err2 == nil {
				log.Printf("[Decomposition] Resolved assignee by name %s → %s", assigneeID, fullID[:8])
				assigneeID = fullID
			} else {
				// 3) Keyword search (e.g. "search-agent" → match "搜索师")
				for _, kw := range extractKeywords(assigneeID) {
					if err3 := h.DB.QueryRow(
						`SELECT id FROM agent_profiles
						 WHERE enabled = true
						   AND (name ILIKE '%' || $1 || '%'
						     OR description ILIKE '%' || $1 || '%'
						     OR tags::text ILIKE '%' || $1 || '%')
						 LIMIT 1`,
						kw,
					).Scan(&fullID); err3 == nil {
						log.Printf("[Decomposition] Resolved assignee by keyword '%s': %s → %s", kw, assigneeID, fullID[:8])
						assigneeID = fullID
						break
					}
				}
			}
		}

		// Get depth from parent
		var depth int
		h.DB.QueryRow(`SELECT COALESCE(depth,0) FROM tasks WHERE id = $1`, taskID).Scan(&depth)

		_, err := h.DB.Exec(
			`INSERT INTO tasks (id, user_id, parent_id, title, description, status, workspace_id,
			 completion_behavior, parallel_group, assignee_id, assignee_type, depth, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)`,
			newTaskID, userIDStr, taskID, item.Title, item.Description,
			"todo", workspaceID, cb, item.ParallelGroup,
			assigneeID, item.AssigneeType, depth+1, now,
		)
		if err != nil {
			log.Printf("[Decomposition] Failed to create task for item %s: %v", item.ID[:8], err)
			InsertAppEvent(h.DB, "error", "decomposition", "创建子任务失败",
				fmt.Sprintf("Item %s: %v", item.ID[:8], err), taskID, "")
			continue
		}

		createdTasks = append(createdTasks, createdTask{
			ItemID:       item.ID,
			TaskID:       newTaskID,
			DependsOn:    dependsOnResolved,
			AssigneeID:   assigneeID,
			AssigneeType: item.AssigneeType,
		})
	}

	// Phase 2: Resolve plan-item dependencies to real task IDs and create DAG edges
	itemToTask := make(map[string]string)
	for _, ct := range createdTasks {
		itemToTask[ct.ItemID] = ct.TaskID
	}

	for _, ct := range createdTasks {
		for _, depPlanItemID := range ct.DependsOn {
			if realDepID, ok := itemToTask[depPlanItemID]; ok {
				h.DAGEngine.CreateDependency(ct.TaskID, realDepID)
			}
		}

		// Mark blocked if has dependencies
		if len(ct.DependsOn) > 0 {
			h.DAGEngine.SetTaskBlocked(ct.TaskID)
		}

		// If no dependencies and has agent assignee, auto-queue
		if len(ct.DependsOn) == 0 {
			if ct.AssigneeType == "agent_profile" && ct.AssigneeID != "" {
				var enabled bool
				err := h.DB.QueryRow(`SELECT enabled FROM agent_profiles WHERE id = $1`, ct.AssigneeID).Scan(&enabled)
				if err == nil && enabled {
					queueID := uuid.New().String()
					h.DB.Exec(
						`INSERT INTO task_agent_queue (id, task_id, agent_profile_id, status, trigger_type, assigned_at, created_at)
						 VALUES ($1, $2, $3, 'queued', 'sub_task', $4, $4)`,
						queueID, ct.TaskID, ct.AssigneeID, now,
					)
					h.DB.Exec(`UPDATE agent_profiles SET current_load = current_load + 1 WHERE id = $1`, ct.AssigneeID)
					log.Printf("[Decomposition] Auto-queued task %s to agent %s", ct.TaskID[:8], ct.AssigneeID[:8])
				} else if err != nil {
					log.Printf("[Decomposition] Auto-queue failed: agent %s not found for task %s", ct.AssigneeID, ct.TaskID[:8])
					InsertAppEvent(h.DB, "error", "decomposition", "自动派发失败: Agent未找到",
						fmt.Sprintf("Agent '%s' not found for task %s", ct.AssigneeID, ct.TaskID[:8]), ct.TaskID, ct.AssigneeID)
				}
			}
		}

		// Update plan item with real_task_id and is_approved=true
		h.DB.Exec(
			`UPDATE decomposition_plan_items SET is_approved = true, real_task_id = $1 WHERE id = $2`,
			ct.TaskID, ct.ItemID,
		)
	}

	// Mark plan as approved
	h.DB.Exec(`UPDATE decomposition_plans SET status = 'approved', updated_at = $1 WHERE id = $2`, now, planID)

	// Set parent task to "blocked" — it now has sub-tasks being worked on.
	// Blocked prevents the decomposition agent from being re-dispatched.
	opts := TransitionOpts{ActorID: userIDStr}
	if err := h.TaskService.MarkBlocked(taskID, opts); err != nil {
		log.Printf("[Decomposition] Failed to block task %s: %v", taskID[:8], err)
	}

	// Post approval comment
	commentContent := fmt.Sprintf("分解计划已审核通过，共创建 %d 个子任务。", len(createdTasks))
	commentID := uuid.New().String()
	h.DB.Exec(
		`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
		 VALUES ($1, $2, $3, NULL, $4, false, $5, $5)`,
		commentID, taskID, userIDStr, commentContent, now,
	)

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
		h.Hub.SignalChange("task_agent_queue")
	}

	log.Printf("[Decomposition] Plan %s approved for task %s (%d items → %d tasks)",
		planID[:8], taskID[:8], len(pendingItems), len(createdTasks))

	c.JSON(http.StatusOK, gin.H{
		"status": "approved",
		"count":  len(createdTasks),
	})
}

// RejectPlan rejects the entire decomposition plan.
func (h *DecompositionHandler) RejectPlan(c *gin.Context) {
	taskID := c.Param("id")

	var req struct {
		Comment string `json:"comment,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)
	now := time.Now()

	var planID string
	err := h.DB.QueryRow(
		`SELECT id FROM decomposition_plans
		 WHERE task_id = $1 AND status = 'pending'
		 ORDER BY created_at DESC LIMIT 1`,
		taskID,
	).Scan(&planID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "没有待审核的分解计划"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query plan"})
		return
	}

	// Mark plan as rejected
	h.DB.Exec(`UPDATE decomposition_plans SET status = 'rejected', updated_at = $1 WHERE id = $2`, now, planID)
	// Mark all items as rejected
	h.DB.Exec(`UPDATE decomposition_plan_items SET is_approved = false WHERE plan_id = $1 AND is_approved IS NULL`, planID)

	// Parent stays in_progress — agent can retry
	opts := TransitionOpts{ActorID: userIDStr}
	if err := h.TaskService.MarkInProgress(taskID, opts); err != nil {
		log.Printf("[Decomposition] Failed to set task %s in_progress: %v", taskID[:8], err)
	}

	// Post rejection comment
	commentText := "分解计划已被驳回"
	if req.Comment != "" {
		commentText += "：\n" + req.Comment
	}
	commentID := uuid.New().String()
	h.DB.Exec(
		`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
		 VALUES ($1, $2, $3, NULL, $4, false, $5, $5)`,
		commentID, taskID, userIDStr, commentText, now,
	)

	if h.Hub != nil {
		h.Hub.SignalChange("tasks")
	}

	log.Printf("[Decomposition] Plan %s rejected for task %s (re-opened for agent)", planID[:8], taskID[:8])
	c.JSON(http.StatusOK, gin.H{"status": "rejected"})
}

// extractKeywords splits a hyphenated/underscored name into searchable words.
// Generic words like "agent", "profile", "assignee" are filtered out.
func extractKeywords(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	var out []string
	skip := map[string]bool{"agent": true, "profile": true, "assignee": true, "user": true, "the": true, "for": true, "and": true, "or": true}
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if len(p) > 2 && !skip[p] {
			out = append(out, p)
		}
	}
	return out
}
