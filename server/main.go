package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/coaether/server/config"

	"github.com/coaether/server/database"

	"github.com/coaether/server/handlers"

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

	log.Println("[Server] Message store initialized")

	busH := handlers.NewBusHandler(messageBus, database.DB)

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

	// Router

	r := gin.Default()

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

	// Auth required

	api := r.Group("/api")

	api.Use(middleware.AuthMiddleware(cfg.JWTSecret))

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

		// Task rules
		api.GET("/rules", ruleH.List)
		api.POST("/rules", ruleH.Create)
		api.GET("/rules/:id", ruleH.Get)
		api.PUT("/rules/:id", ruleH.Update)
		api.DELETE("/rules/:id", ruleH.Delete)
		api.GET("/rules/:id/logs", ruleH.ListLogs)

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
