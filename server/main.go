package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/superco/server/config"
	"github.com/superco/server/database"
	"github.com/superco/server/handlers"
	"github.com/superco/server/middleware"
	"github.com/superco/server/protocol"
	"github.com/superco/server/store"
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

	// Message Bus
	messageBus := protocol.NewMessageBus()
	msgStore := store.NewPostgresStore(database.DB)
	messageBus.SetStore(msgStore)
	log.Println("[Server] Message store initialized")
	busH := handlers.NewBusHandler(messageBus)

	// Handlers
	authH := handlers.NewAuthHandler(database.DB, cfg.JWTSecret)
	nodeH := handlers.NewNodeHandler(database.DB, messageBus)
	dashHub := handlers.NewDashboardHub(database.DB, cfg.JWTSecret, messageBus)
	sessionH := handlers.NewSessionHandler(database.DB, messageBus)
	profileH := handlers.NewAgentProfileHandler(database.DB)
	taskH := handlers.NewTaskHandler(database.DB)
	taskH.Hub = dashHub
	projectH := handlers.NewProjectHandler(database.DB)
	projectH.Hub = dashHub
	workspaceH := handlers.NewWorkspaceHandler(database.DB)
	workspaceH.Hub = dashHub
	busH.Hub = dashHub // link for dashboard broadcasting

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

	// Health check
	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// WebSocket routes
	r.GET("/ws/dashboard", dashHub.HandleDashboardWS)
	r.GET("/ws/bus", busH.HandleWS)

	// Auth required
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware(cfg.JWTSecret))
	{
		api.GET("/nodes", nodeH.List)
		api.GET("/nodes/:id", nodeH.GetByID)
		api.POST("/nodes/register", nodeH.Register)
		api.POST("/nodes/heartbeat", nodeH.Heartbeat)
		api.GET("/nodes/:id/agents", nodeH.ListAgents)
		api.POST("/nodes/:id/scan", nodeH.TriggerScan)

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
	}

	log.Printf("[Server] Starting on :%s", cfg.ServerPort)
	if err := r.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("[FATAL] Failed to start server: %v", err)
	}
}
