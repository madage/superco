package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/superco/server/models"
	"github.com/superco/server/protocol"
	"github.com/superco/server/redis"
)

type NodeHandler struct {
	DB  *sql.DB
	Bus *protocol.MessageBus
}

func NewNodeHandler(db *sql.DB, bus *protocol.MessageBus) *NodeHandler {
	return &NodeHandler{DB: db, Bus: bus}
}

// isBusNode checks if this is a bus-connected virtual node.
// Returns the original endpoint ID (e.g., "runtime://runtime-default").
func isBusNode(nodeID string) string {
	if len(nodeID) > 4 && nodeID[:4] == "bus-" {
		raw := nodeID[4:]
		// Desanitize: runtime--runtime-default → runtime://runtime-default
		return strings.ReplaceAll(raw, "--", "://")
	}
	return ""
}

func (h *NodeHandler) Register(c *gin.Context) {
	var req models.NodeRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")

	nodeID := uuid.New().String()
	node := models.Node{
		ID:        nodeID,
		UserID:    userID.(string),
		Name:      req.Name,
		OS:        req.OS,
		Arch:      req.Arch,
		Status:    models.NodeStatusOnline,
		Version:   req.Version,
		IP:        c.ClientIP(),
		LastSeen:  time.Now(),
		CreatedAt: time.Now(),
	}

	_, err := h.DB.Exec(
		`INSERT INTO nodes (id, user_id, name, os, arch, status, version, ip, last_seen, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		node.ID, node.UserID, node.Name, node.OS, node.Arch, node.Status, node.Version, node.IP, node.LastSeen, node.CreatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register node"})
		return
	}

	wsToken := uuid.New().String()
	if err := redis.SetNodeOnline(nodeID, wsToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set node online"})
		return
	}

	c.JSON(http.StatusOK, models.NodeRegisterResp{
		NodeID:  nodeID,
		WSToken: wsToken,
	})
}

func (h *NodeHandler) List(c *gin.Context) {
	userID, _ := c.Get("user_id")

	// Build set of currently active bus runtime node IDs
	activeBusNodes := make(map[string]bool)
	if h.Bus != nil {
		for _, ep := range h.Bus.EndpointsByType(protocol.EndpointRuntime) {
			nodeID := "bus-" + strings.ReplaceAll(ep.ID, "://", "--")
			activeBusNodes[nodeID] = true
		}
	}

	rows, err := h.DB.Query(
		`SELECT id, user_id, name, os, arch, status, version, ip, max_sessions, last_seen, created_at
		 FROM nodes WHERE user_id = $1 ORDER BY last_seen DESC`, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query nodes"})
		return
	}
	defer rows.Close()

	var nodes []models.Node
	for rows.Next() {
		var n models.Node
		if err := rows.Scan(&n.ID, &n.UserID, &n.Name, &n.OS, &n.Arch, &n.Status, &n.Version, &n.IP, &n.MaxSessions, &n.LastSeen, &n.CreatedAt); err != nil {
			continue
		}
		// Skip bus virtual nodes that have no active runtime connection.
		// These are stale DB records — either marked offline from a past clean
		// disconnect, or stuck "online" from a killed process.
		if strings.HasPrefix(n.ID, "bus-") && !activeBusNodes[n.ID] {
			continue
		}
		nodes = append(nodes, n)
	}

	if nodes == nil {
		nodes = []models.Node{}
	}

	// Inject active bus runtime endpoints that are not yet in the result
	if h.Bus != nil {
		existing := make(map[string]bool, len(nodes))
		for _, n := range nodes {
			existing[n.ID] = true
		}
		for _, ep := range h.Bus.EndpointsByType(protocol.EndpointRuntime) {
			nodeID := "bus-" + strings.ReplaceAll(ep.ID, "://", "--")
			if existing[nodeID] {
				continue
			}
			nodes = append(nodes, models.Node{
				ID:          nodeID,
				UserID:      userID.(string),
				Name:        "Runtime: " + ep.ID,
				OS:          "unknown",
				Status:      models.NodeStatusOnline,
				Version:     "0.1.0",
				IP:          "bus",
				MaxSessions: 3,
				LastSeen:    time.Now(),
				CreatedAt:   time.Now(),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
}

func (h *NodeHandler) Heartbeat(c *gin.Context) {
	var req models.NodeHeartbeatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.DB.Exec(
		"UPDATE nodes SET status = $1, last_seen = NOW() WHERE id = $2",
		req.Status, req.NodeID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update heartbeat"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *NodeHandler) GetByID(c *gin.Context) {
	nodeID := c.Param("id")

	var n models.Node
	err := h.DB.QueryRow(
		`SELECT id, user_id, name, os, arch, status, version, ip, max_sessions, last_seen, created_at
		 FROM nodes WHERE id = $1`, nodeID,
	).Scan(&n.ID, &n.UserID, &n.Name, &n.OS, &n.Arch, &n.Status, &n.Version, &n.IP, &n.MaxSessions, &n.LastSeen, &n.CreatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, n)
}

func (h *NodeHandler) ListAgents(c *gin.Context) {
	nodeID := c.Param("id")

	// For bus-connected virtual nodes, return runtime capabilities as agents
	if epID := isBusNode(nodeID); epID != "" && h.Bus != nil {
		ep := h.Bus.GetEndpoint(epID)
		if ep != nil {
			agents := make([]models.Agent, 0, len(ep.Capabilities))
			for _, cap := range ep.Capabilities {
				agents = append(agents, models.Agent{
					ID:           epID + "/" + cap.ID,
					NodeID:       nodeID,
					Name:         cap.Name,
					Command:      cap.ID,
					Version:      cap.Version,
					Enabled:      true,
					AutoDetected: true,
					CreatedAt:    time.Now(),
					UpdatedAt:    time.Now(),
				})
			}
			c.JSON(http.StatusOK, gin.H{"agents": agents})
			return
		}
	}

	rows, err := h.DB.Query(
		`SELECT id, node_id, name, command, version, enabled, auto_detected, created_at, updated_at
		 FROM agents WHERE node_id = $1 ORDER BY name`, nodeID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query agents"})
		return
	}
	defer rows.Close()

	var agents []models.Agent
	for rows.Next() {
		var a models.Agent
		if err := rows.Scan(&a.ID, &a.NodeID, &a.Name, &a.Command, &a.Version, &a.Enabled, &a.AutoDetected, &a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []models.Agent{}
	}

	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

func (h *NodeHandler) TriggerScan(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "scanning"})
}

func (h *NodeHandler) UpdateAgent(c *gin.Context) {
	agentID := c.Param("id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.DB.Exec(
		"UPDATE agents SET enabled = $1, updated_at = NOW() WHERE id = $2",
		req.Enabled, agentID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update agent"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
