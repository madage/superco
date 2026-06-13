package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/coaether/server/harness"
	"github.com/coaether/server/models"
	"github.com/coaether/server/protocol"
)

type NodeAgentHandler struct {
	DB      *sql.DB
	Hub     *DashboardHub
	Bus     *protocol.MessageBus
	Harness     *harness.Harness
	TaskService *TaskService
}

func NewNodeAgentHandler(db *sql.DB, bus *protocol.MessageBus) *NodeAgentHandler {
	return &NodeAgentHandler{DB: db, Bus: bus}
}

type nodeAuthInfo struct {
	NodeID      string
	UserID      string
	NodeName    string
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
		SELECT q.id, q.task_id, q.agent_profile_id, q.status, q.trigger_type, q.assigned_at, q.claimed_at, q.completed_at, q.result_summary, q.snapshot, q.created_at,
			ap.name AS agent_name
		FROM task_agent_queue q
		JOIN agent_profiles ap ON ap.id = q.agent_profile_id AND ap.enabled = true
		JOIN workspace_members wm ON wm.workspace_id = ap.workspace_id
		WHERE wm.user_id = $1 AND q.status = 'queued' AND ap.node_id = $2
		ORDER BY q.created_at ASC
		LIMIT 10`, auth.UserID, auth.NodeID)
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
		TriggerType    string          `json:"trigger_type,omitempty"`
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
			&item.TriggerType, &item.AssignedAt, &item.ClaimedAt, &item.CompletedAt, &item.ResultSummary,
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

	// Verify this node is the assigned node for the agent profile
	var agentNodeID string
	err = h.DB.QueryRow(
		`SELECT ap.node_id FROM task_agent_queue q
		 JOIN agent_profiles ap ON ap.id = q.agent_profile_id
		 WHERE q.id = $1`, queueID,
	).Scan(&agentNodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify node assignment"})
		return
	}
	if agentNodeID != auth.NodeID {
		c.JSON(http.StatusForbidden, gin.H{"error": "this agent is assigned to a different node"})
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
	if h.Hub != nil {
		h.Hub.SignalChange("task_agent_queue")
	}
}

// UpdateQueueStatus updates a queue item's status.
// UpdateQueueStatus updates a queue item's status.
func (h *NodeAgentHandler) UpdateQueueStatus(c *gin.Context) {
	auth, ok := h.authenticate(c)
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
		h.DB.Exec("UPDATE task_agent_queue SET status = $1, claimed_at = COALESCE(claimed_at, $2) WHERE id = $3",
			req.Status, now, queueID)
		// Auto-set task to in_progress via TaskService
		var taskID string
		h.DB.QueryRow("SELECT task_id FROM task_agent_queue WHERE id = $1", queueID).Scan(&taskID)
		if taskID != "" {
			opts := TransitionOpts{ActorID: auth.UserID}
			h.TaskService.MarkInProgress(taskID, opts)
		}
	} else if req.Status == "completed" || req.Status == "failed" {
		h.DB.Exec("UPDATE task_agent_queue SET status = $1, completed_at = $2, result_summary = $3 WHERE id = $4",
			req.Status, now, req.ResultSummary, queueID)
		// Decrement current_load
		h.DB.Exec("UPDATE agent_profiles SET current_load = GREATEST(0, current_load - 1) WHERE id = (SELECT agent_profile_id FROM task_agent_queue WHERE id = $1)", queueID)

		// Sync session DB status to match queue status
		sessionStatus := models.SessionCompleted
		if req.Status == "failed" {
			sessionStatus = models.SessionFailed
		}
		h.DB.Exec(
			`UPDATE sessions SET status = $1, completed_at = NOW(), updated_at = NOW() WHERE queue_id = $2 AND status IN ('pending', 'running')`,
			sessionStatus, queueID,
		)

		if req.Status == "completed" {
			var taskID, agentProfileID string
			h.DB.QueryRow("SELECT task_id, agent_profile_id FROM task_agent_queue WHERE id = $1", queueID).Scan(&taskID, &agentProfileID)
			if taskID != "" {
				// Guard: pending_review_actions — waiting for human approval, skip status change
				var pendingActions []byte
				h.DB.QueryRow("SELECT pending_review_actions FROM tasks WHERE id = $1 AND deleted_at IS NULL", taskID).Scan(&pendingActions)
				if len(pendingActions) > 5 {
					log.Printf("[NodeAgent] Task %s has pending_review_actions, skipping status update", taskID[:8])
					goto afterStatusUpdate
				}

				// Delegate to TaskService for complete orchestration
				opts := TransitionOpts{
					AgentProfileID: agentProfileID,
					ActorID:        auth.UserID,
					ResultSummary:  req.ResultSummary,
					QueueID:        queueID,
				}
				if err := h.TaskService.MarkCompleted(taskID, opts); err != nil {
					log.Printf("[NodeAgent] MarkCompleted failed: %v", err)
				}

				// If task was routed to review and has a different assignee agent, trigger review queue
				var currentStatus, assigneeType, assigneeID string
				h.DB.QueryRow("SELECT status, COALESCE(assignee_type,''), COALESCE(assignee_id,'') FROM tasks WHERE id = $1 AND deleted_at IS NULL", taskID).
					Scan(&currentStatus, &assigneeType, &assigneeID)
				if currentStatus == "review" && assigneeType == "agent_profile" && assigneeID != "" && assigneeID != agentProfileID {
					var existingID string
					err := h.DB.QueryRow(
						"SELECT id FROM task_agent_queue WHERE task_id = $1 AND agent_profile_id = $2 AND status IN ('queued', 'claimed', 'processing') LIMIT 1",
						taskID, assigneeID,
					).Scan(&existingID)
					if err != nil {
						reviewQueueID := uuid.New().String()
						reviewNow := time.Now()
						h.DB.Exec(
							"INSERT INTO task_agent_queue (id, task_id, agent_profile_id, status, trigger_type, assigned_at, created_at) VALUES ($1, $2, $3, 'queued', 'review', $4, $4)",
							reviewQueueID, taskID, assigneeID, reviewNow,
						)
						h.DB.Exec("UPDATE agent_profiles SET current_load = current_load + 1 WHERE id = $1", assigneeID)
						if h.Bus != nil {
							autoProcessReview(h.DB, h.Bus, taskID, assigneeID, reviewQueueID, req.ResultSummary)
						}
					}
				}
			}
		}
	afterStatusUpdate:
	} else {
		h.DB.Exec("UPDATE task_agent_queue SET status = $1 WHERE id = $2", req.Status, queueID)
	}

	log.Printf("[NodeAgent] Queue item %s → %s", queueID[:8], req.Status)
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
	if h.Hub != nil {
		h.Hub.SignalChange("task_agent_queue")
		if req.Status == "processing" || req.Status == "completed" {
			h.Hub.SignalChange("tasks")
		}
	}
}

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
		AgentID string `json:"agent_id"`           // agent profile ID
		QueueID string `json:"queue_id,omitempty"` // optional queue item ID for completion callback
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

	// Fetch agent capabilities to determine if this is a decomposition or execution agent
	var capsJSON, agentSysPrompt, agentInstructions string
	isDecomposer := false
	if agentID != "claude" && agentID != "echo" {
		h.DB.QueryRow(
			`SELECT COALESCE(capabilities::text,'[]'), COALESCE(system_prompt,''), COALESCE(instructions,'')
			 FROM agent_profiles WHERE id = $1`, agentID,
		).Scan(&capsJSON, &agentSysPrompt, &agentInstructions)
		isDecomposer = strings.Contains(capsJSON, "propose_decomposition_plan")
	}

	var prompt string
	if isDecomposer {
		// Fetch available agent profiles for the workspace
			agentRows, _ := h.DB.Query(
				`SELECT id, name, COALESCE(description, '') FROM agent_profiles WHERE enabled = true
				 AND workspace_id = (SELECT workspace_id FROM tasks WHERE id = $1) ORDER BY name`,
				req.TaskID,
			)
			var agentList string
			if agentRows != nil {
				var agents []string
				for agentRows.Next() {
					var id, name, desc string
					if err := agentRows.Scan(&id, &name, &desc); err == nil {
						short := id
						if len(desc) > 60 {
							desc = desc[:60] + "..."
						}
						agents = append(agents, fmt.Sprintf("  - %s (%s): %s", name, short, desc))
					}
				}
				agentRows.Close()
				if len(agents) > 0 {
					agentList = "\nAvailable agents to assign sub-tasks to:\n" + strings.Join(agents, "\n")
				}
			}

			prompt = fmt.Sprintf(`Task ID: %s
Title: %s

Description: %s

## Your Role

You are a task-decomposition agent. Your ONLY job is to break down this task into sub-tasks and assign them to appropriate agents. You MUST NOT attempt to do the work yourself. Do NOT research, analyze, or produce content — only decompose and assign.

## How It Works

1. Analyze the task and decide how to split it into manageable sub-tasks
2. Call mcp__coaether-harness__propose_decomposition_plan ONCE with ALL sub-tasks as an items array:
   - Each item must have: title, description, assignee_id, assignee_type="agent_profile"
   - Optional: depends_on (array of other item indices/groups), parallel_group, completion_behavior
3. Add a summary explaining your decomposition strategy
4. The system will present your plan to the user for human review with per-task checkboxes
5. After a human approves (per-item or all), the system creates sub-tasks automatically
6. DO NOT call create_sub_task - your capabilities do not include this tool, calls will be DENIED

## CRITICAL RULES

- Call propose_decomposition_plan ONCE with ALL sub-tasks in the items array
- Every item MUST include assignee_id AND assignee_type="agent_profile"
- DO NOT call create_sub_task - your capabilities do not include this tool, calls will be DENIED
- Do NOT use WebSearch, codegraph, or any research tools — you decompose, you do NOT execute
- Do NOT attempt to answer the task question yourself
- Do NOT use filesystem, shell, or code execution tools
- Use ONLY mcp__coaether-harness__ tools: propose_decomposition_plan, list_sub_tasks, add_comment, get_task_detail
- If you do not know which agent to assign, use get_task_detail to inspect the task further%s`,
				req.TaskID, title, description, agentList)
	} else {
		if agentSysPrompt != "" {
			prompt = fmt.Sprintf("SYSTEM: %s\n\nTask ID: %s\nTitle: %s\n\nDescription: %s\n\n## Your Role\n\nYou are an execution agent. Complete this task directly using your available tools.\n\n## Instructions\n\n%s\n\n## CRITICAL RULES\n\n- Do NOT call propose_decomposition_plan or create_sub_task — you execute, you do NOT decompose\n- Complete the task described above using the appropriate tools available to you\n- Report your results clearly when done\n- Use harness tools (mcp__coaether-harness__ prefix) for task management: add_comment, get_task_detail, update_task_status\n- add_comment MUST be called at most ONCE per round. Put ALL your content into a SINGLE add_comment call. After calling add_comment, STOP — do not post any follow-up comments, summaries, or confirmations.", agentSysPrompt, req.TaskID, title, description, agentInstructions)
		} else {
			prompt = fmt.Sprintf("Task ID: %s\nTitle: %s\n\nDescription: %s\n\n## Your Role\n\nYou are an execution agent. Complete this task directly using your available tools.\n\n## CRITICAL RULES\n\n- Do NOT call propose_decomposition_plan or create_sub_task — you execute, you do NOT decompose\n- Complete the task described above using the appropriate tools available to you\n- Report your results clearly when done\n- Use harness tools (mcp__coaether-harness__ prefix) for task management: add_comment, get_task_detail, update_task_status\n- add_comment MUST be called at most ONCE per round. Put ALL your content into a SINGLE add_comment call. After calling add_comment, STOP — do not post any follow-up comments, summaries, or confirmations.", req.TaskID, title, description)
		}
	}

	// --- 前情提要: inject task context for retry sessions ---
	if req.QueueID != "" && req.TaskID != "" {
		var ctxLines []string
		ctxLines = append(ctxLines, "\n\n--- 前情提要 ---")

		// Task status & retry count
		var taskStatus string
		var loopCount, maxLoops int
		if err := h.DB.QueryRow(
			`SELECT COALESCE(status,''), COALESCE(agent_loop_count,0), COALESCE(max_agent_loops,12) FROM tasks WHERE id = $1`,
			req.TaskID,
		).Scan(&taskStatus, &loopCount, &maxLoops); err == nil {
			ctxLines = append(ctxLines, fmt.Sprintf("\n任务状态: %s\n重试次数: %d/%d", taskStatus, loopCount, maxLoops))
		}

		// Current round number — count agent comments already posted, then +1
		var agentCommentCount int
		if err := h.DB.QueryRow(
			`SELECT COUNT(*) FROM task_comments WHERE task_id = $1 AND agent_profile_id = $2 AND is_agent_comment = true`,
			req.TaskID, req.AgentID,
		).Scan(&agentCommentCount); err == nil {
			ctxLines = append(ctxLines, fmt.Sprintf("\n当前为第 %d 轮对话", agentCommentCount+1))
		}

		// Last review (rejection reason)
		var reviewAction, reviewComment string
		if err := h.DB.QueryRow(
			`SELECT COALESCE(action,''), COALESCE(comment,'') FROM task_reviews WHERE task_id = $1 ORDER BY created_at DESC LIMIT 1`,
			req.TaskID,
		).Scan(&reviewAction, &reviewComment); err == nil && reviewComment != "" {
			ctxLines = append(ctxLines, fmt.Sprintf("\n最近一次审核结果:\n%s: %s", reviewAction, reviewComment))
		}

		// Last agent execution result
		var resultSummary string
		if err := h.DB.QueryRow(
			`SELECT COALESCE(result_summary,'') FROM task_agent_queue WHERE task_id = $1 AND result_summary != '' ORDER BY completed_at DESC NULLS LAST LIMIT 1`,
			req.TaskID,
		).Scan(&resultSummary); err == nil && resultSummary != "" {
			ctxLines = append(ctxLines, fmt.Sprintf("\n最近一次 Agent 执行结果:\n%s", resultSummary))
		}

		// Recent comments (last 5), excluding current agent's own comments
		// (--resume already restores the agent's full conversation history;
		// including agent comments here would duplicate context and cause
		// Claude to repeat itself within a single round.)
		rows, err := h.DB.Query(
			`SELECT COALESCE(u.username,''), COALESCE(ap.name,''), c.content, c.created_at, c.is_agent_comment
			 FROM task_comments c
			 LEFT JOIN users u ON u.id = c.user_id
			 LEFT JOIN agent_profiles ap ON ap.id = c.agent_profile_id
			 WHERE c.task_id = $1 AND c.agent_profile_id != $2
			 ORDER BY c.created_at DESC LIMIT 5`, req.TaskID, req.AgentID,
		)
		if err == nil {
			var commentLines []string
			for rows.Next() {
				var userName, agentName, content string
				var createdAt time.Time
				var isAgentComment bool
				if err := rows.Scan(&userName, &agentName, &content, &createdAt, &isAgentComment); err == nil {
					source := userName
					if isAgentComment && agentName != "" {
						source = agentName + " (Agent)"
					}
					if source == "" {
						if isAgentComment {
							source = "Agent"
						} else {
							source = "Unknown"
						}
					}
					commentLines = append(commentLines, fmt.Sprintf("%s [%s]: %s", createdAt.Format("2006-01-02 15:04"), source, content))
				}
			}
			rows.Close()
			if len(commentLines) > 0 {
				ctxLines = append(ctxLines, "\n最近评论:")
				for i := len(commentLines) - 1; i >= 0; i-- {
					ctxLines = append(ctxLines, "  "+commentLines[i])
				}
			}
		}

		if len(ctxLines) > 1 {
			prompt += strings.Join(ctxLines, "\n")
		}
	}

	// Dedup: if an active session already exists for this task, reuse it.
	// Only applies to duplicate polls (no queue_id). When queue_id is
	// present (e.g. review rejection retry), always create a fresh session.
	if req.TaskID != "" && req.QueueID == "" {
		var existingID string
		err := h.DB.QueryRow(
			`SELECT id FROM sessions WHERE task_id = $1 AND status IN ('pending', 'running') LIMIT 1`,
			req.TaskID,
		).Scan(&existingID)
		if err == nil {
			c.JSON(http.StatusOK, gin.H{"session_id": existingID, "status": "existing"})
			return
		}
	}

	// If this is a retry (has queue_id), close any stale running session for this task
	if req.QueueID != "" && req.TaskID != "" {
		var staleID string
		err := h.DB.QueryRow(
			`SELECT id FROM sessions WHERE task_id = $1 AND status IN ('pending', 'running') LIMIT 1`,
			req.TaskID,
		).Scan(&staleID)
		if err == nil {
			h.DB.Exec(
				`UPDATE sessions SET status = $1, error_log = 'superseded by retry', completed_at = NOW(), updated_at = NOW() WHERE id = $2`,
				models.SessionFailed, staleID,
			)
			h.Bus.EndSession(staleID)
			log.Printf("[NodeAgent] Closed stale session %s for task %s (retry)", staleID[:8], req.TaskID[:8])
		}
	}

	sessionID := uuid.New().String()
	now := time.Now()

	// Create session in MessageBus
	h.Bus.CreateSession(sessionID, map[string]protocol.MemberRole{
		"system://api": protocol.RoleOwner,
	})

	// Insert into DB
	h.DB.Exec(
		`INSERT INTO sessions (id, user_id, node_id, agent_id, task_id, queue_id, agent_profile_id, status, prompt, workspace, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)`,
		sessionID, auth.UserID, auth.NodeID, agentID, req.TaskID, req.QueueID, req.AgentID, models.SessionPending, prompt, workspaceID, now,
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
				"task_id":      req.TaskID,
				"task_title":   title,
				"is_auto_task": true,
				"queue_id":          req.QueueID,
				"agent_profile_id":  req.AgentID,
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
				"task_id":          req.TaskID,
				"queue_id":         req.QueueID,
				"agent_profile_id": req.AgentID,
				"auto_task":        true,
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

// CreateAgentComment creates a comment on behalf of an agent (used by runtime for @mention replies).
func (h *NodeAgentHandler) CreateAgentComment(c *gin.Context) {
	auth, ok := h.authenticate(c)
	if !ok {
		return
	}

	taskID := c.Param("id")

	var req struct {
		Content         string `json:"content"`
		AgentProfileID  string `json:"agent_profile_id"`
		QueueID         string `json:"queue_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Content == "" || req.AgentProfileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content and agent_profile_id are required"})
		return
	}

	now := time.Now()
	commentID := uuid.New().String()

	_, err := h.DB.Exec(
		`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, true, $6, $7)`,
		commentID, taskID, auth.UserID, req.AgentProfileID, req.Content, now, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create comment"})
		return
	}

	// Fetch the created comment with agent info
	var comment models.TaskComment
	h.DB.QueryRow(`
		SELECT c.id, c.task_id, c.user_id, '' AS username,
			c.agent_profile_id, COALESCE(ap.name, '') AS agent_name, COALESCE(ap.avatar, '') AS agent_avatar,
			c.content, c.parent_id, c.is_agent_comment, c.created_at, c.updated_at
		FROM task_comments c
		LEFT JOIN agent_profiles ap ON ap.id = c.agent_profile_id
		WHERE c.id = $1`, commentID,
	).Scan(
		&comment.ID, &comment.TaskID, &comment.UserID, &comment.Username,
		&comment.AgentProfileID, &comment.AgentName, &comment.AgentAvatar,
		&comment.Content, &comment.ParentID, &comment.IsAgentComment, &comment.CreatedAt, &comment.UpdatedAt,
	)

	if h.DB != nil {
		// Signal change via Hub (passed through Bus or we can call from task handler)
		_ = auth // use auth to suppress unused warning
	}

	log.Printf("[NodeAgent] Agent %s commented on task %s", req.AgentProfileID[:8], taskID[:8])
	c.JSON(http.StatusCreated, comment)
}


// HandleToolCall accepts a tool call from the runtime, executes it via Harness, and returns the result.
func (h *NodeAgentHandler) HandleToolCall(c *gin.Context) {
	auth, ok := h.authenticate(c)
	if !ok {
		return
	}

	var req struct {
		TaskID    string          `json:"task_id"`
		QueueID   string          `json:"queue_id"`
		Tool      string          `json:"tool"`
		Params    json.RawMessage `json:"params"`
		CallID    string          `json:"call_id,omitempty"`
		ProfileID string          `json:"agent_profile_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Tool == "" || req.TaskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tool and task_id are required"})
		return
	}

	// Build agent context from the profile
	ctx := h.resolveAgentContext(auth, req.ProfileID, req.TaskID)

	tc := &harness.ToolCall{
		Type:   "tool_call",
		Tool:   req.Tool,
		Params: req.Params,
		ID:     req.CallID,
	}

	result := h.Harness.HandleToolCall(ctx, tc)
	c.JSON(http.StatusOK, result)
}

// resolveAgentContext builds a harness.AgentContext from the agent profile and task info.
func (h *NodeAgentHandler) resolveAgentContext(auth *nodeAuthInfo, profileID, taskID string) *harness.AgentContext {
	ctx := &harness.AgentContext{
		TaskID: &taskID,
	}

	if profileID != "" {
		ctx.AgentProfileID = profileID

		var name, capsJSON, permsJSON string
		var maxDepth int
		err := h.DB.QueryRow(
			`SELECT name, COALESCE(capabilities,'[]'), COALESCE(permissions,'[]'), COALESCE(max_depth,5)
			 FROM agent_profiles WHERE id = $1`, profileID,
		).Scan(&name, &capsJSON, &permsJSON, &maxDepth)
		if err == nil {
			ctx.AgentName = name
			ctx.MaxDepth = maxDepth
			var capsList []string
			if err := json.Unmarshal([]byte(capsJSON), &capsList); err != nil {
				// Fallback: try parsing as object (old format: {} or {"tool_name":true})
				var capsMap map[string]interface{}
				if e2 := json.Unmarshal([]byte(capsJSON), &capsMap); e2 == nil {
					for k, v := range capsMap {
						if b, ok := v.(bool); ok && b {
							capsList = append(capsList, k)
						}
					}
				}
				// Fallback: try {"tools": [...]} format (UI format)
				if len(capsList) == 0 {
					var capsObj struct {
						Tools []string `json:"tools"`
					}
					if e3 := json.Unmarshal([]byte(capsJSON), &capsObj); e3 == nil {
						capsList = append(capsList, capsObj.Tools...)
					}
				}
			}
			ctx.Capabilities = make(map[string]bool)
			for _, c := range capsList {
				ctx.Capabilities[c] = true
			}
			json.Unmarshal([]byte(permsJSON), &ctx.Permissions)
		}
	}

	// Get workflow info, user_id, and server-side token tracking from task+workflow
	var workflowID *string
	var depth int
	var userID string
	var tokenBudget, tokensUsed int64
	maxDepth := ctx.MaxDepth
	h.DB.QueryRow(
		`SELECT t.workflow_id, t.depth, COALESCE(t.max_depth, $1), t.user_id,
		        COALESCE(w.token_budget, 0), COALESCE(w.tokens_used, 0)
		 FROM tasks t
		 LEFT JOIN workflows w ON t.workflow_id = w.id
		 WHERE t.id = $2 AND t.deleted_at IS NULL`,
		maxDepth, taskID,
	).Scan(&workflowID, &depth, &maxDepth, &userID, &tokenBudget, &tokensUsed)
	if workflowID != nil && *workflowID != "" {
		ctx.WorkflowID = workflowID
	}
	ctx.Depth = depth
	ctx.MaxDepth = maxDepth
	ctx.UserID = userID

	// Server-side token tracking — authoritative for budget enforcement
	ctx.TokenBudget = tokenBudget
	ctx.TokensUsed = tokensUsed

	return ctx
}

// ReportTokenUsage records token consumption from the runtime.
func (h *NodeAgentHandler) ReportTokenUsage(c *gin.Context) {
	auth, ok := h.authenticate(c)
	if !ok {
		return
	}

	var req struct {
		TaskID           string `json:"task_id" binding:"required"`
		AgentProfileID   string `json:"agent_profile_id" binding:"required"`
		SessionID        string `json:"session_id"`
		PromptTokens     int    `json:"prompt_tokens"`
		CompletionTokens int    `json:"completion_tokens"`
		TotalTokens      int    `json:"total_tokens"`
		Stage            string `json:"stage"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id := uuid.New().String()
	now := time.Now()
	stage := req.Stage
	if stage == "" {
		stage = "work"
	}

	_, err := h.DB.Exec(
		`INSERT INTO token_usage (id, task_id, agent_profile_id, session_id, prompt_tokens, completion_tokens, total_tokens, stage, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, req.TaskID, req.AgentProfileID, req.SessionID,
		req.PromptTokens, req.CompletionTokens, req.TotalTokens, stage, now,
	)
	if err != nil {
		log.Printf("[NodeAgent] Failed to record token usage: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record token usage"})
		return
	}

	// Update workflow tokens if applicable
	var workflowID string
	h.DB.QueryRow(`SELECT workflow_id FROM tasks WHERE id = $1 AND deleted_at IS NULL`, req.TaskID).Scan(&workflowID)
	if workflowID != "" {
		h.DB.Exec(`UPDATE workflows SET tokens_used = tokens_used + $1 WHERE id = $2`, req.TotalTokens, workflowID)
	}

	log.Printf("[NodeAgent] Token usage recorded: agent=%s task=%s tokens=%d", req.AgentProfileID[:8], req.TaskID[:8], req.TotalTokens)
	_ = auth
	c.JSON(http.StatusOK, gin.H{"status": "recorded"})
}
