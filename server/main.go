package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/coaether/server/config"

	"github.com/coaether/server/database"

	"github.com/coaether/server/handlers"
	"github.com/coaether/server/harness"

	"github.com/coaether/server/mailer"

	"github.com/coaether/server/middleware"

	"github.com/coaether/server/protocol"

	"github.com/coaether/server/store"

	"github.com/coaether/server/plugin"
)

func main() {

	cfg := config.Load()

	// Database

	if err := database.Connect(cfg.PostgresDSN); err != nil {

		log.Fatalf("[FATAL] %v", err)

	}

	defer database.Close()

	if err := database.Migrate(); err != nil {

		log.Fatalf("[FATAL] %v", err)

	}

	// Mailer

	mail := mailer.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom, cfg.PublicURL)

	if mail.IsConfigured() {

		log.Println("[Mailer] SMTP configured")

	} else {

		log.Println("[Mailer] SMTP not configured — invitation links will be logged")

	}

	// Message Bus

	messageBus := protocol.NewMessageBus()

	msgStore := store.NewPostgresStore(database.DB)

	messageBus.SetStore(msgStore)

	// Start bus session GC: every 10 min, clean sessions older than 30 min with only system members
	messageBus.StartGC(10*time.Minute, 30*time.Minute)

	log.Println("[Server] Message store initialized")

	// SessionService — DB session lifecycle + GC
	sessionSvc := handlers.NewSessionService(database.DB, messageBus)

	busH := handlers.NewBusHandler(messageBus, database.DB)
	busH.SessionService = sessionSvc

	// DB session GC: every 30 min, mark sessions idle >2h as failed
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			n, err := sessionSvc.CleanStaleDBSessions(2 * time.Hour)
			if err != nil {
				log.Printf("[SessionGC] DB cleanup error: %v", err)
			} else if n > 0 {
				log.Printf("[SessionGC] Cleaned %d stale DB sessions", n)
			}
		}
	}()

	// Message store GC: daily, keep 7 days
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			n, err := msgStore.CleanOldMessages(7 * 24 * time.Hour)
			if err != nil {
				log.Printf("[MessageGC] Cleanup error: %v", err)
			} else if n > 0 {
				log.Printf("[MessageGC] Cleaned %d old messages", n)
			}
		}
	}()

	// Handlers

	authH := handlers.NewAuthHandler(database.DB, cfg.JWTSecret)

	nodeH := handlers.NewNodeHandler(database.DB, messageBus)

	dashHub := handlers.NewDashboardHub(database.DB, cfg.JWTSecret, messageBus)
	nodeH.Hub = dashHub // for node status broadcasting

	sessionH := handlers.NewSessionHandler(database.DB, messageBus)

	profileH := handlers.NewAgentProfileHandler(database.DB)
	profileH.Hub = dashHub

	skillH := handlers.NewSkillHandler(database.DB)
	skillH.Hub = dashHub

	agentSched := handlers.NewAgentScheduler(database.DB)
	agentSched.Hub = dashHub
	agentSched.MessageBus = messageBus

	taskH := handlers.NewTaskHandler(database.DB)

	taskH.Hub = dashHub
	taskH.MessageBus = messageBus
	notifH := handlers.NewNotificationHandler(database.DB, dashHub)
	notifH.Mailer = mail
	taskH.Notifier = notifH
	ruleEngine := handlers.NewRuleEngine(database.DB, dashHub, notifH)
	taskH.RuleEngine = ruleEngine
	taskH.AgentScheduler = agentSched
	ruleH := handlers.NewRuleHandler(database.DB, dashHub)

	projectH := handlers.NewProjectHandler(database.DB)

	projectH.Hub = dashHub

	workspaceH := handlers.NewWorkspaceHandler(database.DB)

	workspaceH.Hub = dashHub

	workspaceH.Mailer = mail

	userH := handlers.NewUserHandler(database.DB)

	busH.Hub = dashHub // link for dashboard broadcasting

	// ===== Plugin System =====

	pluginDir := "."
	pluginMgr := plugin.NewManager(pluginDir, ".")

	loaded, err := pluginMgr.LoadAndRegister()
	if err != nil {
		log.Printf("[Server] Plugin scan warning: %v", err)
	}
	if len(loaded) > 0 {
		log.Printf("[Server] Registered plugins: %v", loaded)
	} else {
		log.Println("[Server] No plugins registered")
	}

	hostSvc := plugin.NewHostService(messageBus, pluginMgr)
	pluginH := handlers.NewPluginHandler(pluginMgr)

	nodeAgentH := handlers.NewNodeAgentHandler(database.DB, messageBus)
	nodeAgentH.Hub = dashHub

	// Workflow handler with Harness
	workflowH := handlers.NewWorkflowHandler(database.DB)
	workflowH.Hub = dashHub
	workflowH.Notifier = notifH
	workflowH.RegisterToolExecutors()

	// Share Harness instance with node agent handler
	nodeAgentH.Harness = workflowH.Harness

	// Review Router
	reviewRouter := handlers.NewReviewRouter(database.DB)
	reviewRouter.Hub = dashHub
	reviewRouter.Notifier = notifH
	taskH.ReviewRouter = reviewRouter

	// TaskService — unified status transition orchestration
	taskService := handlers.NewTaskService(database.DB, dashHub, workflowH.DAGEngine, reviewRouter, notifH)
	taskService.RuleEngine = ruleEngine
	taskService.Bus = messageBus
	workflowH.TaskService = taskService
	nodeAgentH.TaskService = taskService
	taskH.TaskService = taskService
	reviewRouter.TaskService = taskService
	ruleEngine.TaskService = taskService

	// Wire DAGEngine Hub for real-time updates
	workflowH.DAGEngine.Hub = dashHub
	reviewRouter.DAGEngine.Hub = dashHub
	workflowH.DAGEngine.TaskService = taskService
	reviewRouter.DAGEngine.TaskService = taskService

	// Decomposition Handler (plan review flow)
	decompH := handlers.NewDecompositionHandler(database.DB)
	decompH.Hub = dashHub
	decompH.ReviewRouter = reviewRouter
	decompH.DAGEngine.Hub = dashHub
	decompH.TaskService = taskService

	// Tool Set Handler (global tool on/off management)
	toolSetH := handlers.NewToolSetHandler(database.DB)
	toolSetH.Hub = dashHub
	toolSetH.PolicyEngine = workflowH.Harness.Policy

	// API Token handler
	tokenH := handlers.NewTokenHandler(database.DB)

	// Log handler
	logH := handlers.NewLogHandler(database.DB)

	// Safety Guard (anti-runaway monitor)
	safetyGuard := harness.NewSafetyGuard(database.DB)
	safetyGuard.StartPeriodicCheck(5 * time.Minute)

	// Router

	r := gin.Default()
	r.Use(middleware.AccessLogMiddleware(database.DB))

	// CORS

	r.Use(func(c *gin.Context) {

		c.Header("Access-Control-Allow-Origin", "*")

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {

			c.AbortWithStatus(204)

			return

		}

		c.Next()

	})

	// Public routes

	r.POST("/api/auth/login", authH.Login)

	r.POST("/api/auth/register", authH.Register)

	// Public invitation routes (no auth needed — info only)

	r.GET("/api/invitations/:token", workspaceH.GetInvitationByToken)

	r.POST("/api/invitations/:token/decline", workspaceH.DeclineInvitation)

	// Health check

	r.GET("/api/health", func(c *gin.Context) {

		c.JSON(200, gin.H{"status": "ok"})

	})

	// WebSocket routes

	r.GET("/ws/dashboard", dashHub.HandleDashboardWS)

	r.GET("/ws/bus", busH.HandleWS)

	// Public node routes (no auth — token-based security)

	r.GET("/api/nodes/install.sh", nodeH.InstallScript)

	r.GET("/api/nodes/install.ps1", nodeH.InstallScriptPS1)

	r.GET("/api/nodes/bin/:os/:arch", nodeH.DownloadBinary)

	// Node-agent routes (auth via node_secret query param)

	r.GET("/api/node/queue", nodeAgentH.ListQueue)
	r.POST("/api/node/queue/:id/claim", nodeAgentH.ClaimQueueItem)
	r.PUT("/api/node/queue/:id/status", nodeAgentH.UpdateQueueStatus)
	r.GET("/api/node/tasks/:id", nodeAgentH.GetTask)
	r.POST("/api/node/sessions", nodeAgentH.CreateSession)
	r.POST("/api/node/tasks/:id/comments", nodeAgentH.CreateAgentComment)
	r.POST("/api/node/token-usage", nodeAgentH.ReportTokenUsage)
	r.POST("/api/node/tool-call", nodeAgentH.HandleToolCall)

	// Auth required

	api := r.Group("/api")

	api.Use(middleware.AuthMiddleware(cfg.JWTSecret, database.DB))

	api.Use(middleware.WorkspaceAuthMiddleware(database.DB))

	{

		api.GET("/nodes", nodeH.List)

		api.GET("/nodes/:id", nodeH.GetByID)

		api.POST("/nodes/register", nodeH.Register)

		api.POST("/nodes/heartbeat", nodeH.Heartbeat)

		api.POST("/nodes/token", nodeH.GenerateToken)

		api.DELETE("/nodes/:id", nodeH.RemoveNode)

		api.GET("/nodes/:id/agents", nodeH.ListAgents)

		api.POST("/nodes/:id/scan", nodeH.TriggerScan)

		api.POST("/nodes/:id/start", nodeH.StartNode)

		api.POST("/nodes/:id/stop", nodeH.StopNode)

		api.PATCH("/agents/:id", nodeH.UpdateAgent)

		// Agent profiles

		api.GET("/agents/profiles", profileH.List)

		api.POST("/agents/profiles", profileH.Create)

		api.GET("/agents/profiles/:id", profileH.Get)

		api.PUT("/agents/profiles/:id", profileH.Update)

		api.DELETE("/agents/profiles/:id", profileH.Delete)

		api.GET("/agents/runtimes", profileH.ListRuntimes)

		// Tasks

		api.GET("/tasks", taskH.List)

		api.POST("/tasks", taskH.Create)

		api.GET("/tasks/trash", taskH.ListTrash)

		api.GET("/tasks/:id", taskH.Get)

		api.PUT("/tasks/:id", taskH.Update)

		api.DELETE("/tasks/:id", taskH.Delete)

		api.DELETE("/tasks/:id/force", taskH.PermanentDelete)

		api.POST("/tasks/:id/restore", taskH.Restore)

		api.PATCH("/tasks/:id/status", taskH.SetStatus)
		api.POST("/tasks/:id/assignees", taskH.AddAssignee)
		api.DELETE("/tasks/:id/assignees/:assigneeId", taskH.RemoveAssignee)
		api.GET("/tasks/:id/assignees", taskH.ListAssignees)
		api.GET("/tasks/:id/subtasks", taskH.ListSubtasks)
			api.GET("/tasks/:id/comments", taskH.ListComments)
			api.POST("/tasks/:id/comments", taskH.CreateComment)
			api.DELETE("/tasks/:id/comments/:commentId", taskH.DeleteComment)
			api.POST("/tasks/:id/review", reviewRouter.HandleReviewHTTP)
			api.GET("/tasks/:id/decomposition-plan", decompH.GetPlan)
			api.POST("/tasks/:id/decomposition-plan/approve", decompH.ApprovePlan)
			api.POST("/tasks/:id/decomposition-plan/reject", decompH.RejectPlan)

		// Task rules
		api.GET("/rules", ruleH.List)
		api.POST("/rules", ruleH.Create)
		api.GET("/rules/:id", ruleH.Get)
		api.PUT("/rules/:id", ruleH.Update)
		api.DELETE("/rules/:id", ruleH.Delete)
		api.GET("/rules/:id/logs", ruleH.ListLogs)

		// Tools (system harness tool management)
		api.GET("/tools", toolSetH.List)
		api.POST("/tools/:toolName/toggle", toolSetH.Toggle)

		// Skills
		api.GET("/skills", skillH.List)
		api.POST("/skills", skillH.Create)
		api.GET("/skills/:id", skillH.Get)
		api.PUT("/skills/:id", skillH.Update)
		api.DELETE("/skills/:id", skillH.Delete)
		api.POST("/skills/extract-from-task", skillH.ExtractFromTask)
			// Agent Queue & Scheduler
			api.GET("/agents/queue", agentSched.List)
			api.POST("/agents/auto-assign/:taskId", agentSched.AutoAssign)
			api.POST("/agents/queue/:id/claim", agentSched.Claim)
			api.PUT("/agents/queue/:id/status", agentSched.UpdateStatus)
			api.GET("/agents/queue/agents", agentSched.ListAgentsWithLoad)

		// Projects

		api.GET("/projects", projectH.List)

		api.POST("/projects", projectH.Create)

		api.GET("/projects/trash", projectH.ListTrash)

		api.GET("/projects/:id", projectH.Get)

		api.PUT("/projects/:id", projectH.Update)

		api.DELETE("/projects/:id", projectH.Delete)

		api.DELETE("/projects/:id/force", projectH.PermanentDelete)

		api.POST("/projects/:id/restore", projectH.Restore)

		// Workspaces

		api.GET("/workspaces", workspaceH.List)

		api.POST("/workspaces", workspaceH.Create)

		api.GET("/workspaces/:id", workspaceH.Get)

		api.PUT("/workspaces/:id", workspaceH.Update)

		api.DELETE("/workspaces/:id", workspaceH.Delete)

		// Workspace members

		api.GET("/workspaces/:id/members", workspaceH.ListMembers)

		api.POST("/workspaces/:id/members", workspaceH.AddMember)

		api.PUT("/workspaces/:id/members/:userId", workspaceH.UpdateMemberRole)

		api.DELETE("/workspaces/:id/members/:userId", workspaceH.RemoveMember)

		// Workspace invitations (authenticated)

		api.POST("/workspaces/:id/invitations", workspaceH.CreateInvitation)

		api.GET("/workspaces/:id/invitations", workspaceH.ListInvitations)

		api.DELETE("/workspaces/:id/invitations/:invitationId", workspaceH.CancelInvitation)

		api.POST("/invitations/:token/accept", workspaceH.AcceptInvitation)

		api.GET("/invitations/pending", workspaceH.ListPendingInvitations)

		// Log management
		api.GET("/logs/agent-tool", logH.AgentToolLogs)
		api.GET("/logs/access", logH.AccessLogs)
		api.GET("/logs/token-usage", logH.TokenUsage)
		api.GET("/logs/system-events", logH.SystemEvents)

		// API Token management (admin/owner)
		api.GET("/tokens", tokenH.List)
		api.POST("/tokens", tokenH.Create)
		api.DELETE("/tokens/:id", tokenH.Delete)

		// User management (admin/owner)

		api.GET("/users", userH.List)

		api.DELETE("/users/:id", userH.Delete)
		// Notifications
		api.GET("/notifications", notifH.List)
		api.GET("/notifications/unread-count", notifH.UnreadCount)
		api.PATCH("/notifications/:id/read", notifH.MarkRead)
		api.PATCH("/notifications/read-all", notifH.MarkAllRead)
		api.DELETE("/notifications/:id", notifH.Delete)

		api.POST("/sessions", sessionH.Create)

		api.GET("/sessions", sessionH.List)

		api.GET("/sessions/:id", sessionH.GetByID)

		api.GET("/sessions/:id/messages", func(c *gin.Context) {

			sessionID := c.Param("id")

			store := messageBus.GetStore()

			if store == nil {

				c.JSON(500, gin.H{"error": "message store not available"})

				return

			}

			envelopes, err := store.GetBySession(sessionID, 200)

			if err != nil {

				c.JSON(500, gin.H{"error": "failed to fetch messages"})

				return

			}

			c.JSON(200, gin.H{"messages": envelopes})

		})
		plugin.RegisterPluginRoutes(api, pluginMgr, hostSvc, pluginH)

		// Workflows
		api.GET("/workflows", workflowH.ListWorkflows)
		api.POST("/workflows", workflowH.CreateWorkflow)
		api.GET("/workflows/:id", workflowH.GetWorkflow)
		api.PATCH("/workflows/:id/status", workflowH.UpdateWorkflowStatus)
		api.GET("/workflows/:id/tasks", workflowH.ListWorkflowTasks)
		api.POST("/workflows/attach", workflowH.AttachToWorkflow)

	}

	// Auto-start discovered plugins
	started := pluginMgr.StartAll(context.Background())
	if len(started) > 0 {
		log.Printf("[Server] Auto-started plugins: %v", started)
	}
	
	log.Printf("[Server] Starting on :%s", cfg.ServerPort)

	if err := r.Run(":" + cfg.ServerPort); err != nil {

		log.Fatalf("[FATAL] Failed to start server: %v", err)

	}

}
