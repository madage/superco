package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/superco/server/models"
	"github.com/superco/server/protocol"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins in MVP
	},
}

type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type NodeConnection struct {
	Conn   *websocket.Conn
	NodeID string
	OS     string
	Mu     sync.Mutex
}

type WSHub struct {
	DB           *sql.DB
	JWTSecret    string
	Bus          *protocol.MessageBus
	Nodes        map[string]*NodeConnection
	NodesBySess  map[string]string // session_id -> node_id
	Sessions     map[string]*NodeConnection // session_id -> ui conn
	Dashboards   map[string]*NodeConnection // dashboard ws connections
	PendingOut   map[string][][]byte // buffered output before UI connects
	Mu           sync.RWMutex
}

func NewWSHub(db *sql.DB, jwtSecret string, bus *protocol.MessageBus) *WSHub {
	return &WSHub{
		DB:           db,
		JWTSecret:    jwtSecret,
		Bus:          bus,
		Nodes:        make(map[string]*NodeConnection),
		NodesBySess:  make(map[string]string),
		Sessions:     make(map[string]*NodeConnection),
		Dashboards:   make(map[string]*NodeConnection),
		PendingOut:   make(map[string][][]byte),
	}
}

func (h *WSHub) HandleNodeWS(c *gin.Context) {
	nodeID := c.Query("node_id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing node_id"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}

	nc := &NodeConnection{Conn: conn, NodeID: nodeID}
	h.Mu.Lock()
	h.Nodes[nodeID] = nc
	h.Mu.Unlock()

	// Register or update node in database
	nodeName := c.Query("name")
	if nodeName == "" {
		nodeName = "agent-node-" + nodeID[:8]
	}
	h.upsertNode(nodeID, nodeName, c.Query("os"), c.Query("arch"), c.ClientIP())

	log.Printf("[WS] Node connected: %s (%s)", nodeID, nodeName)

	// Broadcast node status to dashboards
	h.BroadcastToDashboards("node_status", map[string]interface{}{
		"node_id": nodeID,
		"name":    nodeName,
		"status":  "online",
	})

	defer func() {
		h.Mu.Lock()
		delete(h.Nodes, nodeID)
		h.Mu.Unlock()
		h.DB.Exec("UPDATE nodes SET status = $1, last_seen = NOW() WHERE id = $2",
			models.NodeStatusOffline, nodeID)
		conn.Close()
		log.Printf("[WS] Node disconnected: %s", nodeID)

		// Broadcast node offline to dashboards
		h.BroadcastToDashboards("node_status", map[string]interface{}{
			"node_id": nodeID,
			"name":    nodeName,
			"status":  "offline",
		})
	}()

	// heartbeat ping
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			nc.Mu.Lock()
			if err := nc.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				nc.Mu.Unlock()
				return
			}
			nc.Mu.Unlock()
		}
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}

		h.handleNodeMessage(nc, msg)
	}
}

func (h *WSHub) HandleUIWS(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing session_id"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] UI upgrade error: %v", err)
		return
	}

	h.Mu.Lock()
	h.Sessions[sessionID] = &NodeConnection{Conn: conn}
	// Flush any buffered output to the newly connected UI
	if buf, ok := h.PendingOut[sessionID]; ok {
		for _, msgBytes := range buf {
			conn.WriteMessage(websocket.TextMessage, msgBytes)
		}
		delete(h.PendingOut, sessionID)
	}
	h.Mu.Unlock()

	log.Printf("[WS] UI connected to session: %s", sessionID)

	defer h.cleanupSessionBinding(sessionID)
	defer func() {
		// Notify node to stop the shell when UI disconnects
		stopPayload, _ := json.Marshal(WSMessage{
			Type:    "stop",
			Payload: mustJSON(map[string]string{"session_id": sessionID}),
		})
		h.forwardToNode(sessionID, stopPayload)

		h.Mu.Lock()
		delete(h.Sessions, sessionID)
		delete(h.PendingOut, sessionID)
		h.Mu.Unlock()

		// Reset node status
		h.Mu.RLock()
		nodeID := h.NodesBySess[sessionID]
		h.Mu.RUnlock()
		if nodeID != "" {
			h.DB.Exec("UPDATE nodes SET status = 'online' WHERE id = $1", nodeID)
		}

		conn.Close()
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}

		if msg.Type == "input" {
			h.forwardToNode(sessionID, msgBytes)
		}
	}
}

// HandleDashboardWS handles WebSocket connections from the UI dashboard
// for real-time node/session list updates.
func (h *WSHub) HandleDashboardWS(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing token"})
		return
	}

	// Verify JWT
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(h.JWTSecret), nil
	})
	if err != nil || !parsedToken.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
		return
	}
	userID, _ := claims["user_id"].(string)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] Dashboard upgrade error: %v", err)
		return
	}

	nc := &NodeConnection{Conn: conn}
	connID := uuid.New().String()

	h.Mu.Lock()
	h.Dashboards[connID] = nc
	h.Mu.Unlock()

	log.Printf("[WS] Dashboard connected: %s (user: %s)", connID, userID)

	// Send initial state
	h.sendDashboardInit(nc, userID)

	// Heartbeat
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			nc.Mu.Lock()
			if err := nc.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				nc.Mu.Unlock()
				return
			}
			nc.Mu.Unlock()
		}
	}()

	defer func() {
		h.Mu.Lock()
		delete(h.Dashboards, connID)
		h.Mu.Unlock()
		conn.Close()
		log.Printf("[WS] Dashboard disconnected: %s", connID)
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *WSHub) sendDashboardInit(nc *NodeConnection, userID string) {
	// Build set of currently active bus runtime node IDs
	activeBusNodes := make(map[string]bool)
	if h.Bus != nil {
		for _, ep := range h.Bus.EndpointsByType(protocol.EndpointRuntime) {
			nodeID := "bus-" + strings.ReplaceAll(ep.ID, "://", "--")
			activeBusNodes[nodeID] = true
		}
	}

	// Fetch nodes for this user
	nodes := make([]models.Node, 0)
	rows, err := h.DB.Query(
		`SELECT id, user_id, name, os, arch, status, version, ip, last_seen, created_at
		 FROM nodes WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var n models.Node
			if err := rows.Scan(&n.ID, &n.UserID, &n.Name, &n.OS, &n.Arch, &n.Status, &n.Version, &n.IP, &n.LastSeen, &n.CreatedAt); err == nil {
				// Skip bus virtual nodes that have no active runtime connection.
				// These are stale DB records — either marked offline from a past clean
				// disconnect, or stuck "online" from a killed process.
				if strings.HasPrefix(n.ID, "bus-") && !activeBusNodes[n.ID] {
					continue
				}
				nodes = append(nodes, n)
			}
		}
	} else {
		log.Printf("[WS] Failed to query nodes for init: %v", err)
	}

	// Add bus-connected runtime endpoints as virtual nodes for dashboard visibility.
	// Skip any already in the DB result to avoid duplicates.
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
			getMeta := func(key, def string) string {
				if v, ok := ep.Metadata[key]; ok {
					if s, ok := v.(string); ok {
						return s
					}
				}
				return def
			}
			nodes = append(nodes, models.Node{
				ID:        nodeID,
				Name:      getMeta("name", ep.ID),
				Status:    models.NodeStatusOnline,
				OS:        getMeta("os", "unknown"),
				Arch:      getMeta("arch", ""),
				Version:   getMeta("version", ""),
				IP:        "bus",
				MaxSessions: 3,
				LastSeen:  time.Now(),
				CreatedAt: time.Now(),
			})
		}

	}

	// Fetch sessions for this user
	sessions := make([]models.SessionResp, 0)
	srows, err := h.DB.Query(
		`SELECT id, agent_id, status, prompt, workspace, node_id, created_at
		 FROM sessions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50`, userID,
	)
	if err == nil {
		defer srows.Close()
		for srows.Next() {
			var s models.SessionResp
			if err := srows.Scan(&s.ID, &s.AgentID, &s.Status, &s.Prompt, &s.Workspace, &s.NodeID, &s.CreatedAt); err == nil {
				sessions = append(sessions, s)
			}
		}
	} else {
		log.Printf("[WS] Failed to query sessions for init: %v", err)
	}

	payload := map[string]interface{}{
		"nodes":    nodes,
		"sessions": sessions,
	}
	data, _ := json.Marshal(WSMessage{Type: "init", Payload: mustJSON(payload)})
	nc.Mu.Lock()
	nc.Conn.WriteMessage(websocket.TextMessage, data)
	nc.Mu.Unlock()
}

func (h *WSHub) BroadcastToDashboards(msgType string, payload interface{}) {
	data, err := json.Marshal(WSMessage{Type: msgType, Payload: mustJSON(payload)})
	if err != nil {
		return
	}

	h.Mu.RLock()
	defer h.Mu.RUnlock()

	for id, dc := range h.Dashboards {
		dc.Mu.Lock()
		if err := dc.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[WS] Dashboard write error (%s): %v", id, err)
		}
		dc.Mu.Unlock()
	}
}

func (h *WSHub) handleNodeMessage(nc *NodeConnection, msg WSMessage) {
	switch msg.Type {
	case "output":
		var payload struct {
			SessionID string `json:"session_id"`
			Data      string `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return
		}
		// Update session status to running on first output
		h.DB.Exec("UPDATE sessions SET status = 'running', updated_at = NOW() WHERE id = $1 AND status = 'pending'",
			payload.SessionID)
		// Forward to UI if connected, otherwise buffer
		h.Mu.RLock()
		_, uiConnected := h.Sessions[payload.SessionID]
		h.Mu.RUnlock()
		msgBytes := msgBytesReconstruct(msg.Type, msg.Payload)
		if uiConnected {
			h.forwardToUI(payload.SessionID, msgBytes)
		} else {
			h.Mu.Lock()
			if buf, ok := h.PendingOut[payload.SessionID]; ok {
				h.PendingOut[payload.SessionID] = append(buf, msgBytes)
			}
			h.Mu.Unlock()
		}

	case "task_result":
		var payload struct {
			SessionID string `json:"session_id"`
			Success   bool   `json:"success"`
			Error     string `json:"error,omitempty"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return
		}
		status := models.SessionCompleted
		if !payload.Success {
			status = models.SessionFailed
		}
		h.DB.Exec("UPDATE sessions SET status = $1, updated_at = NOW(), completed_at = NOW() WHERE id = $2",
			status, payload.SessionID)
		h.forwardToUI(payload.SessionID, msgBytesReconstruct(msg.Type, msg.Payload))

		// Reset node status back to online
		h.Mu.RLock()
		nodeID := h.NodesBySess[payload.SessionID]
		h.Mu.RUnlock()
		if nodeID != "" {
			h.DB.Exec("UPDATE nodes SET status = 'online' WHERE id = $1", nodeID)
		}

		h.cleanupSessionBinding(payload.SessionID)

		// Broadcast session update to dashboards
		h.BroadcastToDashboards("session_update", map[string]interface{}{
			"id":     payload.SessionID,
			"status": status,
		})

	case "agent_list":
		var payload struct {
			Agents []models.AgentScanResult `json:"agents"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return
		}
		h.handleAgentList(nc.NodeID, payload.Agents)

	case "claim_task":
		h.assignTask(nc)
	}
}

func (h *WSHub) handleAgentList(nodeID string, agents []models.AgentScanResult) {
	// Delete old auto-detected agents, then insert current ones
	h.DB.Exec("DELETE FROM agents WHERE node_id = $1 AND auto_detected = true", nodeID)

	for _, a := range agents {
		h.DB.Exec(
			`INSERT INTO agents (id, node_id, name, command, version, enabled, auto_detected, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, true, true, NOW(), NOW())
			 ON CONFLICT DO NOTHING`,
			uuid.New().String(), nodeID, a.Name, a.Command, a.Version,
		)
	}

	var count int
	h.DB.QueryRow("SELECT COUNT(*) FROM agents WHERE node_id = $1", nodeID).Scan(&count)
	log.Printf("[WS] Node %s reported %d agents (%d total)", nodeID, len(agents), count)
}

func (h *WSHub) assignTask(nc *NodeConnection) {
	// In MVP: pop from Redis queue and assign
	// The task assignment is done via Redis BRPop from the node side,
	// but for simplicity we can also push via WebSocket
}

func (h *WSHub) forwardToNode(sessionID string, msg []byte) {
	h.Mu.RLock()
	nodeID, ok := h.NodesBySess[sessionID]
	h.Mu.RUnlock()
	if !ok {
		return
	}

	h.Mu.RLock()
	nc, ok := h.Nodes[nodeID]
	h.Mu.RUnlock()
	if !ok {
		return
	}

	nc.Mu.Lock()
	nc.Conn.WriteMessage(websocket.TextMessage, msg)
	nc.Mu.Unlock()
}

func (h *WSHub) forwardToUI(sessionID string, msg []byte) {
	h.Mu.RLock()
	uic, ok := h.Sessions[sessionID]
	h.Mu.RUnlock()
	if !ok {
		return
	}

	uic.Mu.Lock()
	uic.Conn.WriteMessage(websocket.TextMessage, msg)
	uic.Mu.Unlock()
}

func (h *WSHub) cleanupSessionBinding(sessionID string) {
	h.Mu.Lock()
	delete(h.NodesBySess, sessionID)
	delete(h.Sessions, sessionID)
	delete(h.PendingOut, sessionID)
	h.Mu.Unlock()
}

func (h *WSHub) upsertNode(nodeID, name, os, arch, ip string) {
	var count int
	h.DB.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = $1", nodeID).Scan(&count)

	if count == 0 {
		// Get first user as owner (single-user MVP assumption)
		var userID string
		err := h.DB.QueryRow("SELECT id FROM users ORDER BY created_at ASC LIMIT 1").Scan(&userID)
		if err != nil {
			log.Printf("[WS] No user found to associate node '%s': %v", nodeID, err)
			return
		}
		_, err = h.DB.Exec(
			`INSERT INTO nodes (id, user_id, name, os, arch, status, version, ip, last_seen, created_at)
			 VALUES ($1, $2, $3, $4, $5, 'online', '', $6, NOW(), NOW())`,
			nodeID, userID, name, os, arch, ip,
		)
		if err != nil {
			log.Printf("[WS] Failed to insert node '%s': %v", nodeID, err)
		}
	} else {
		h.DB.Exec(
			"UPDATE nodes SET status = 'online', name = $1, ip = $2, last_seen = NOW() WHERE id = $3",
			name, ip, nodeID,
		)
	}
}

// SendTaskToNode sends a "task" WebSocket message to a connected node
// and records the session-to-node mapping for input forwarding.
func (h *WSHub) SendTaskToNode(nodeID, sessionID string, task interface{}) bool {
	h.Mu.RLock()
	nc, ok := h.Nodes[nodeID]
	h.Mu.RUnlock()
	if !ok {
		log.Printf("[WS] Cannot send task to node '%s': not connected", nodeID)
		return false
	}

	payload, err := json.Marshal(map[string]interface{}{
		"type":    "task",
		"payload": task,
	})
	if err != nil {
		log.Printf("[WS] Failed to marshal task: %v", err)
		return false
	}

	nc.Mu.Lock()
	defer nc.Mu.Unlock()
	if err := nc.Conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		log.Printf("[WS] Failed to send task to node '%s': %v", nodeID, err)
		return false
	}

	// Initialize pending output buffer for this session
	h.Mu.Lock()
	h.NodesBySess[sessionID] = nodeID
	h.PendingOut[sessionID] = make([][]byte, 0)
	h.Mu.Unlock()

	// Update node status to busy
	h.DB.Exec("UPDATE nodes SET status = 'busy' WHERE id = $1", nodeID)

	log.Printf("[WS] Task sent to node '%s' for session '%s'", nodeID, sessionID)
	return true
}

// BroadcastSessionUpdate sends a session status update to all dashboard clients.
func (h *WSHub) BroadcastSessionUpdate(sessionID string, status interface{}, prompt, workspace, nodeID string) {
	h.BroadcastToDashboards("session_update", map[string]interface{}{
		"id":        sessionID,
		"status":    status,
		"prompt":    prompt,
		"workspace": workspace,
		"node_id":   nodeID,
	})
}

func msgBytesReconstruct(msgType string, payload json.RawMessage) []byte {
	msg := WSMessage{Type: msgType, Payload: payload}
	b, _ := json.Marshal(msg)
	return b
}

func mustJSON(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
