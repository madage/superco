package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/coaether/server/models"
	"github.com/coaether/server/protocol"
)

// TransitionOpts carries optional context for a status transition.
type TransitionOpts struct {
	ActorID         string // user ID or agent profile ID making the change
	AgentProfileID  string // agent profile ID, if an agent made the transition
	ResultSummary   string // agent output summary (creates comment on completion)
	Comment         string // optional human/agent comment
	QueueID         string // task_agent_queue ID, for load management
	ReviewerID      *string
	ReviewerAgentID *string
	SkipNotify      bool // suppress notifications (bulk operations)
}

// taskSnapshot holds a task's state at the start of a transition.
type taskSnapshot struct {
	Status             string
	CompletionBehavior string
	AssigneeType       string
	AssigneeID         string
	WorkflowID         string
	ParentID           string
	AgentLoopCount     int
	MaxAgentLoops      int
	UserID             string
	Title              string
	WorkspaceID        string
	PendingActionsLen  int
	SystemPrompt       string
	Instructions       string
	CapsJSON           string
}

// TaskService is the single entry point for all task status transitions.
// Every UPDATE tasks SET status must go through TransitionStatus.
type TaskService struct {
	DB         *sql.DB
	Hub        *DashboardHub
	DAGEngine  *DAGEngine
	Reviewer   *ReviewRouter
	Notifier   *NotificationHandler
	RuleEngine *RuleEngine
	Bus        *protocol.MessageBus
}

// NewTaskService creates a new TaskService.
func NewTaskService(db *sql.DB, hub *DashboardHub, dag *DAGEngine, reviewer *ReviewRouter, notifier *NotificationHandler) *TaskService {
	return &TaskService{
		DB:        db,
		Hub:       hub,
		DAGEngine: dag,
		Reviewer:  reviewer,
		Notifier:  notifier,
	}
}

// TransitionStatus is the SINGLE place where task status is written to the DB.
// It validates the transition, executes the UPDATE, and fires all appropriate side effects.
// Uses optimistic locking with up to 3 retries on concurrent modification.
func (s *TaskService) TransitionStatus(taskID string, newStatus string, opts TransitionOpts) error {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		snap, err := s.readTaskSnapshot(taskID)
		if err != nil {
			return fmt.Errorf("task %s: %w", safe8(taskID), err)
		}

		// Idempotency: already in target state
		if snap.Status == newStatus {
			return nil
		}

		// Validate the transition
		if !s.transitionValid(snap.Status, newStatus) {
			return fmt.Errorf("invalid transition: %s -> %s", snap.Status, newStatus)
		}

		now := time.Now()

		// Resolve 'completed' to final target before writing — eliminates nested transition
		actualStatus := newStatus
		if newStatus == string(models.TaskCompleted) {
			actualStatus = s.resolveCompletionTarget(taskID, snap, opts)
		}

		// Write the status — the only UPDATE tasks SET status in the system
		if err := s.writeStatus(taskID, actualStatus, now, snap); err != nil {
			if errors.Is(err, ErrConcurrentMod) && attempt < maxRetries-1 {
				time.Sleep(time.Millisecond * time.Duration(10<<attempt)) // backoff: 10ms, 20ms, 40ms
				continue
			}
			return fmt.Errorf("write status: %w", err)
		}

		// Hub signal
		if s.Hub != nil {
			s.Hub.SignalChange("tasks")
		}

		// Dispatch transition-specific side effects
		s.dispatchSideEffects(taskID, snap.Status, actualStatus, snap, opts, now)

		return nil
	}
	return fmt.Errorf("unreachable")
}

// readTaskSnapshot reads all relevant fields for transition decision-making in one query.
func (s *TaskService) readTaskSnapshot(taskID string) (taskSnapshot, error) {
	var snap taskSnapshot
	var parentID, workflowID, assigneeID, assigneeType sql.NullString
	var pendingActions []byte
	err := s.DB.QueryRow(
		`SELECT status, COALESCE(completion_behavior, 'auto_done'),
			COALESCE(assignee_type, ''), COALESCE(assignee_id, ''),
			workflow_id, parent_id,
			COALESCE(agent_loop_count, 0), COALESCE(max_agent_loops, 6),
			user_id, title, workspace_id,
			COALESCE(pending_review_actions, '[]')
		 FROM tasks WHERE id = $1 AND deleted_at IS NULL`,
		taskID,
	).Scan(&snap.Status, &snap.CompletionBehavior,
		&snap.AssigneeType, &snap.AssigneeID,
		&workflowID, &parentID,
		&snap.AgentLoopCount, &snap.MaxAgentLoops,
		&snap.UserID, &snap.Title, &snap.WorkspaceID,
		&pendingActions,
	)
	if err != nil {
		return snap, err
	}
	if workflowID.Valid {
		snap.WorkflowID = workflowID.String
	}
	if parentID.Valid {
		snap.ParentID = parentID.String
	}
	if assigneeID.Valid {
		snap.AssigneeID = assigneeID.String
	}
	if assigneeType.Valid {
		snap.AssigneeType = assigneeType.String
	}
	snap.PendingActionsLen = len(pendingActions)
	return snap, nil
}

// writeStatus executes the raw UPDATE. Kept private to enforce the single-write-point contract.
// Uses optimistic locking: checks old status in WHERE clause. Returns ErrConcurrentMod
// if RowsAffected == 0, meaning another goroutine changed the status first.
var ErrConcurrentMod = fmt.Errorf("concurrent modification detected")

func (s *TaskService) writeStatus(taskID string, newStatus string, now time.Time, snap taskSnapshot) error {
	var result sql.Result
	var err error
	if newStatus == string(models.TaskDone) || newStatus == string(models.TaskCompleted) {
		result, err = s.DB.Exec(
			`UPDATE tasks SET status = $1, completed_at = $2, updated_at = $2
			 WHERE id = $3 AND status = $4 AND deleted_at IS NULL`,
			newStatus, now, taskID, snap.Status,
		)
	} else {
		result, err = s.DB.Exec(
			`UPDATE tasks SET status = $1, updated_at = $2
			 WHERE id = $3 AND status = $4 AND deleted_at IS NULL`,
			newStatus, now, taskID, snap.Status,
		)
	}
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: task %s status changed from %s by another process",
			ErrConcurrentMod, safe8(taskID), snap.Status)
	}
	return nil
}

// transitionValid checks if the transition is allowed by the state machine.
func (s *TaskService) transitionValid(from, to string) bool {
	valid := map[string]map[string]bool{
		"todo":        {"in_progress": true, "blocked": true},
		"in_progress": {"completed": true, "blocked": true, "review": true, "stuck": true, "done": true},
		"blocked":     {"todo": true, "stuck": true},
		"completed":   {"done": true, "review": true, "completed": true},
		"review":      {"done": true, "in_progress": true, "stuck": true, "blocked": true},
		"done":        {"todo": true, "in_progress": true, "completed": true},
		"stuck":       {"in_progress": true, "todo": true},
	}
	toMap, ok := valid[from]
	if !ok {
		return false
	}
	return toMap[to]
}

// dispatchSideEffects fires all orchestration logic based on the from/to pair.
func (s *TaskService) dispatchSideEffects(taskID, from, to string, snap taskSnapshot, opts TransitionOpts, now time.Time) {
	switch {
	// ---- in_progress -> done (resolved from completed, single write) ----
	case from == string(models.TaskInProgress) && to == string(models.TaskDone):
		s.handleInProgressToDone(taskID, snap, opts)

	// ---- in_progress -> review (resolved from completed, single write) ----
	case from == string(models.TaskInProgress) && to == string(models.TaskReview):
		s.handleInProgressToReview(taskID, snap, opts)

	// ---- review -> done (approved) ----
	case from == string(models.TaskReview) && to == string(models.TaskDone):
		s.handleReviewToDone(taskID, snap, opts, now)

	// ---- review -> in_progress (rejected) ----
	case from == string(models.TaskReview) && to == string(models.TaskInProgress):
		s.handleReviewToInProgress(taskID, snap, opts, now)

	// ---- review -> stuck (meltdown, or manual) ----
	case from == string(models.TaskReview) && to == string(models.TaskStuck):
		s.handleReviewToStuck(taskID, snap, opts)

	// ---- todo -> in_progress ----
	case from == string(models.TaskTodo) && to == string(models.TaskInProgress):
		s.tryAutoDispatch(taskID, snap)

	// ---- blocked -> todo (DAG unblock) ----
	case from == string(models.TaskBlocked) && to == string(models.TaskTodo):
		s.tryAutoDispatch(taskID, snap)

	// ---- in_progress -> blocked (circuit breaker) ----
	case from == string(models.TaskInProgress) && to == string(models.TaskBlocked):
		s.logAppEvent("error", "workflow", "Task blocked", opts.Comment, taskID, opts.AgentProfileID)

	// ---- in_progress -> stuck (safety guard) ----
	case from == string(models.TaskInProgress) && to == string(models.TaskStuck):
		s.logAppEvent("warning", "safety", "Task stuck by safety guard",
			"Task was idle beyond timeout", taskID, "")
		if s.Reviewer != nil {
			s.Reviewer.notifyStuck(taskID, snap.WorkflowID, snap.AgentLoopCount, snap.MaxAgentLoops)
		}

		// ---- done -> todo / in_progress (re-open) ----
	case from == string(models.TaskDone) && (to == string(models.TaskTodo) || to == string(models.TaskInProgress)):
		s.tryAutoDispatch(taskID, snap)

	// ---- stuck -> todo / in_progress (manual unstick) ----
	case from == string(models.TaskStuck):
		if to == string(models.TaskInProgress) || to == string(models.TaskTodo) {
			s.tryAutoDispatch(taskID, snap)
		}
	}
}

// ========== Transition handlers ==========

// handleInProgressToDone is the unified handler for in_progress → done.
// It combines what was previously split across in_progress→completed→done (two writes).
func (s *TaskService) handleInProgressToDone(taskID string, snap taskSnapshot, opts TransitionOpts) {
	// 1. Post agent comment if result summary provided
	if opts.ResultSummary != "" && opts.AgentProfileID != "" {
		s.postAgentComment(taskID, opts.AgentProfileID, opts.ResultSummary, snap.UserID)
	}

	// 2. Process pending dispatch actions (create_sub_task, assign_task)
	s.processPendingActions(taskID)

	// 3. DAG advancement
	s.DAGEngine.OnTaskCompleted(taskID)

	// 4. Rule engine
	if s.RuleEngine != nil {
		s.RuleEngine.Evaluate("on_status_change", taskID, ExtractTaskContext(taskID, snap.Title, snap.AssigneeID, snap.AssigneeType))
	}
}

// handleInProgressToReview is the unified handler for in_progress → review.
// It combines what was previously split across in_progress→completed→review (two writes).
func (s *TaskService) handleInProgressToReview(taskID string, snap taskSnapshot, opts TransitionOpts) {
	// 1. Post agent comment if result summary provided
	if opts.ResultSummary != "" && opts.AgentProfileID != "" {
		s.postAgentComment(taskID, opts.AgentProfileID, opts.ResultSummary, snap.UserID)
	}

	// 2. Completion-behavior-specific review setup
	hasPendingPlan := s.hasPendingDecompositionPlan(taskID)
	if hasPendingPlan && opts.AgentProfileID != "" {
		var creatorID, taskTitle string
		s.DB.QueryRow(`SELECT user_id, title FROM tasks WHERE id = $1`, taskID).Scan(&creatorID, &taskTitle)
		if creatorID != "" {
			commentID := uuid.New().String()
			now := time.Now()
			s.DB.Exec(
				`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
				 VALUES ($1, $2, $3, NULL, $4, true, $5, $5)`,
				commentID, taskID, creatorID,
				fmt.Sprintf("@%s 任务「%s」的分解方案已准备好，请审核并批准子任务。", creatorID, taskTitle),
				now,
			)
		}
	}

	// 3. If auto_review with agent assignee, create review queue for the agent
	if snap.CompletionBehavior == models.CompletionAutoReview && snap.AssigneeType == "agent_profile" && snap.AssigneeID != "" && s.Reviewer != nil {
		s.Reviewer.createReviewQueue(taskID, snap.AssigneeID, snap.WorkflowID)
	}

	// 4. Notify workspace about review needed
	if s.Reviewer != nil {
		s.Reviewer.notifyWorkspace(taskID, "task_needs_review", "任务等待审核")
	}

	// 5. DAG advancement (work product is ready even though review is pending)
	s.DAGEngine.OnTaskCompleted(taskID)

	// 6. Rule engine
	if s.RuleEngine != nil {
		s.RuleEngine.Evaluate("on_status_change", taskID, ExtractTaskContext(taskID, snap.Title, snap.AssigneeID, snap.AssigneeType))
	}
}

func (s *TaskService) handleReviewToDone(taskID string, snap taskSnapshot, opts TransitionOpts, now time.Time) {
	// Process pending dispatch actions
	s.processPendingActions(taskID)

	// Audit log
	action := "approved"
	comment := ""
	if opts.Comment != "" {
		comment = opts.Comment
	}
	s.auditReview(taskID, opts.ReviewerAgentID, action, comment)

	// DAG advancement
	s.DAGEngine.OnTaskCompleted(taskID)

	// Cancel remaining active queue entries for this task and decrement agent load
	s.DB.Exec(`UPDATE agent_profiles SET current_load = GREATEST(0, current_load - 1)
		WHERE id IN (
			SELECT agent_profile_id FROM task_agent_queue
			WHERE task_id = $1 AND status IN ('queued', 'claimed', 'processing')
		)`, taskID)
	s.DB.Exec(`UPDATE task_agent_queue SET status = 'failed', completed_at = $1
		WHERE task_id = $2 AND status IN ('queued', 'claimed', 'processing')`, now, taskID)

	if s.Hub != nil {
		s.Hub.SignalChange("task_agent_queue")
	}

	// Rule engine
	if s.RuleEngine != nil {
		s.RuleEngine.Evaluate("on_status_change", taskID, ExtractTaskContext(taskID, snap.Title, snap.AssigneeID, snap.AssigneeType))
	}
}

func (s *TaskService) handleReviewToInProgress(taskID string, snap taskSnapshot, opts TransitionOpts, now time.Time) {
	newLoopCount := snap.AgentLoopCount + 1

	// Update loop count
	s.DB.Exec(`UPDATE tasks SET agent_loop_count = $1 WHERE id = $2`, newLoopCount, taskID)

	if newLoopCount >= snap.MaxAgentLoops {
		// Meltdown — use MarkStuck for unified side effects (signal, notify, rules, audit)
		log.Printf("[TaskService] Task %s meltdown after %d loops (max %d)", safe8(taskID), newLoopCount, snap.MaxAgentLoops)
		s.MarkStuck(taskID, TransitionOpts{
			ActorID:        opts.ActorID,
			Comment:        fmt.Sprintf("已打回 %d 次（上限 %d 次），已熔断", newLoopCount, snap.MaxAgentLoops),
			ReviewerID:     opts.ReviewerID,
			ReviewerAgentID: opts.ReviewerAgentID,
		})
	} else {
		// Re-queue agent if applicable
		if snap.AssigneeType == "agent_profile" && snap.AssigneeID != "" && s.Reviewer != nil {
			s.Reviewer.createReviewQueue(taskID, snap.AssigneeID, snap.WorkflowID)
		}
		s.auditReview(taskID, opts.ReviewerAgentID, "rejected", opts.Comment)
	}
}

func (s *TaskService) handleReviewToStuck(taskID string, snap taskSnapshot, opts TransitionOpts) {
	// Called for meltdown-stuck, notify and audit
	if s.Reviewer != nil {
		s.Reviewer.notifyStuck(taskID, snap.WorkflowID, snap.AgentLoopCount, snap.MaxAgentLoops)
	}
	s.logAppEvent("warning", "review", "Task stuck (meltdown)",
		fmt.Sprintf("Task %s reached review loop limit", safe8(taskID)), taskID, "")
}

// ========== Completion routing (single-write, no nested transitions) ==========

// resolveCompletionTarget determines the final status for a task completing from in_progress.
// Returns "done" or "review" — never "completed", eliminating the nested transition.
// Does NOT call TransitionStatus or have DB write side effects.
func (s *TaskService) resolveCompletionTarget(taskID string, snap taskSnapshot, opts TransitionOpts) string {
	// Guard: pending decomposition plan always forces review
	if s.hasPendingDecompositionPlan(taskID) {
		log.Printf("[TaskService] Task %s has pending decomposition plan → review", safe8(taskID))
		return string(models.TaskReview)
	}

	switch snap.CompletionBehavior {
	case models.CompletionAutoDone:
		if opts.ResultSummary != "" {
			return string(models.TaskReview)
		}
		return string(models.TaskDone)

	case models.CompletionAutoReview:
		return string(models.TaskReview)

	case models.CompletionSampleReview:
		sampleRate := 0.2
		if snap.WorkflowID != "" {
			s.DB.QueryRow(
				`SELECT COALESCE(ap.review_sample_rate, 0.2) FROM agent_profiles ap
				 JOIN tasks t ON t.assignee_id = ap.id
				 WHERE t.id = $1`, taskID,
			).Scan(&sampleRate)
		}
		if rand.Float64() < sampleRate {
			return string(models.TaskReview)
		}
		return string(models.TaskDone)

	case models.CompletionNeedsReview:
		return string(models.TaskReview)

	default:
		log.Printf("[TaskService] WARNING: unknown completion_behavior '%s' for task %s, defaulting to review",
			snap.CompletionBehavior, safe8(taskID))
		return string(models.TaskReview)
	}
}

// hasPendingDecompositionPlan checks whether a task has a pending decomposition plan
// that requires human review before the task can be auto-completed.
func (s *TaskService) hasPendingDecompositionPlan(taskID string) bool {
	var hasPending bool
	s.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM decomposition_plans WHERE task_id = $1 AND status = 'pending')`,
		taskID,
	).Scan(&hasPending)
	return hasPending
}

// ========== Review handling (inlined from ReviewRouter.HandleReview) ==========

// HandleReview processes a review action (approved/rejected) and transitions the task.
func (s *TaskService) HandleReview(taskID string, reviewerID *string, reviewerAgentID *string,
	action string, comment string) error {

	// Get current task state
	var currentStatus string
	var loopCount, maxLoops int
	err := s.DB.QueryRow(
		`SELECT status, agent_loop_count, max_agent_loops
		 FROM tasks WHERE id = $1 AND deleted_at IS NULL`,
		taskID,
	).Scan(&currentStatus, &loopCount, &maxLoops)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	if currentStatus != string(models.TaskReview) {
		return fmt.Errorf("task is not in review status (current: %s)", currentStatus)
	}

	// Record the review
	reviewID := uuid.New().String()
	now := time.Now()
	s.DB.Exec(
		`INSERT INTO task_reviews (id, task_id, reviewer_id, reviewer_agent_id, action, comment, loop_count, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		reviewID, taskID, reviewerID, reviewerAgentID, action, comment, loopCount+1, now,
	)

	opts := TransitionOpts{
		ActorID:         "",
		AgentProfileID:  "",
		Comment:         comment,
		ReviewerID:      reviewerID,
		ReviewerAgentID: reviewerAgentID,
	}

	switch action {
	case "approved":
		return s.TransitionStatus(taskID, string(models.TaskDone), opts)
	case "rejected":
		return s.TransitionStatus(taskID, string(models.TaskInProgress), opts)
	default:
		return fmt.Errorf("invalid action: %s", action)
	}
}

// ========== Convenience methods ==========

// MarkInProgress transitions a task to in_progress.
func (s *TaskService) MarkInProgress(taskID string, opts TransitionOpts) error {
	return s.TransitionStatus(taskID, string(models.TaskInProgress), opts)
}

// MarkCompleted transitions a task to completed.
func (s *TaskService) MarkCompleted(taskID string, opts TransitionOpts) error {
	return s.TransitionStatus(taskID, string(models.TaskCompleted), opts)
}

// MarkDone transitions a task to done.
func (s *TaskService) MarkDone(taskID string, opts TransitionOpts) error {
	return s.TransitionStatus(taskID, string(models.TaskDone), opts)
}

// MarkBlocked transitions a task to blocked.
func (s *TaskService) MarkBlocked(taskID string, opts TransitionOpts) error {
	return s.TransitionStatus(taskID, string(models.TaskBlocked), opts)
}

// MarkReview transitions a task to review.
func (s *TaskService) MarkReview(taskID string, opts TransitionOpts) error {
	return s.TransitionStatus(taskID, string(models.TaskReview), opts)
}

// MarkStuck transitions a task to stuck.
func (s *TaskService) MarkStuck(taskID string, opts TransitionOpts) error {
	return s.TransitionStatus(taskID, string(models.TaskStuck), opts)
}

// MarkTodo transitions a task to todo.
func (s *TaskService) MarkTodo(taskID string, opts TransitionOpts) error {
	return s.TransitionStatus(taskID, string(models.TaskTodo), opts)
}

// ========== Private helpers ==========

// tryAutoDispatch queues a task to an agent if assignee is agent_profile and agent has capacity.
func (s *TaskService) tryAutoDispatch(taskID string, snap taskSnapshot) {
	if snap.AssigneeType != "agent_profile" || snap.AssigneeID == "" {
		return
	}

	// Skip if already queued
	var existingID string
	err := s.DB.QueryRow(
		`SELECT id FROM task_agent_queue WHERE task_id = $1 AND agent_profile_id = $2 AND status IN ('queued', 'claimed', 'processing') LIMIT 1`,
		taskID, snap.AssigneeID,
	).Scan(&existingID)
	if err == nil {
		return
	}

	// Check if agent is enabled (capacity is validated at claim time, not dispatch time)
	var enabled bool
	s.DB.QueryRow(`SELECT enabled FROM agent_profiles WHERE id = $1`, snap.AssigneeID).Scan(&enabled)
	if !enabled {
		return
	}

	now := time.Now()
	queueID := uuid.New().String()
	s.DB.Exec(
		`INSERT INTO task_agent_queue (id, task_id, agent_profile_id, status, trigger_type, assigned_at, created_at)
		 VALUES ($1, $2, $3, 'queued', 'status_change', $4, $4)`,
		queueID, taskID, snap.AssigneeID, now,
	)
	s.DB.Exec(`UPDATE agent_profiles SET current_load = current_load + 1, last_active_at = $1 WHERE id = $2`,
		now, snap.AssigneeID)

	if s.Hub != nil {
		s.Hub.SignalChange("task_agent_queue")
	}

	log.Printf("[TaskService] Auto-dispatched task %s to agent %s (queue=%s)", safe8(taskID), safe8(snap.AssigneeID), safe8(queueID))
}

// processPendingActions releases gated sub-tasks and assignments when review is approved.
func (s *TaskService) processPendingActions(taskID string) {
	if s.Reviewer != nil {
		s.Reviewer.processPendingActions(taskID)
	}
}

// auditReview records a review entry in the audit log.
func (s *TaskService) auditReview(taskID string, reviewerAgentID *string, action, comment string) {
	log.Printf("[TaskService] Review task %s: %s", safe8(taskID), action)
	// The task_reviews insert is handled by the caller (HandleReview).
	// This method exists so future enhancements (like app_events insertion) have a hook.
}

// postAgentComment creates an agent comment on a task.
func (s *TaskService) postAgentComment(taskID, agentProfileID, content, ownerUserID string) {
	commentID := uuid.New().String()
	now := time.Now()

	summary := content
	if len([]rune(summary)) > 5000 {
		summary = string([]rune(summary)[:5000]) + "\n\n...（结果过长已截断）"
	}

	s.DB.Exec(
		`INSERT INTO task_comments (id, task_id, user_id, agent_profile_id, content, is_agent_comment, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, true, $6, $6)`,
		commentID, taskID, ownerUserID, agentProfileID, summary, now,
	)
}

// logAppEvent writes an application event for UI visibility.
func (s *TaskService) logAppEvent(eventType, source, title, detail, taskID, agentID string) {
	InsertAppEvent(s.DB, eventType, source, title, detail, taskID, agentID)
}

// DecrementAgentLoad decrements an agent's current_load (called when queue item completes/fails).
func (s *TaskService) DecrementAgentLoad(queueID string) {
	s.DB.Exec(`UPDATE agent_profiles SET current_load = GREATEST(0, current_load - 1)
		WHERE id = (SELECT agent_profile_id FROM task_agent_queue WHERE id = $1)`, queueID)
}
