package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/coaether/server/models"
	"github.com/coaether/server/protocol"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type DashboardConn struct {
	Conn *websocket.Conn
	Mu   sync.Mutex
}

// DashboardHub manages dashboard WebSocket connections and broadcasting.
type DashboardHub struct {
	DB         *sql.DB
	JWTSecret  string
	Bus        *protocol.MessageBus
	Dashboards map[string]*DashboardConn
	UserConns  map[string]map[string]bool // userID → set of connIDs
	Mu         sync.RWMutex
}

func NewDashboardHub(db *sql.DB, jwtSecret string, bus *protocol.MessageBus) *DashboardHub {
	return &DashboardHub{
		DB:         db,
		JWTSecret:  jwtSecret,
		Bus:        bus,
		Dashboards: make(map[string]*DashboardConn),
		UserConns:  make(map[string]map[string]bool),
	}
}

// HandleDashboardWS handles WebSocket connections from the UI dashboard
// for real-time node/session list updates.
// Auth: prefers user_id query param (matching /ws/bus pattern), falls back to JWT token.
func (h *DashboardHub) HandleDashboardWS(c *gin.Context) {
	userID := c.Query("user_id")

	if userID == "" {
		// Fallback: JWT token auth
		token := c.Query("token")
		if token == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing user_id or token"})
			return
		}

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
		userID, _ = claims["user_id"].(string)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user"})
			return
		}
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[Dashboard] Upgrade error: %v", err)
		return
	}

	nc := &DashboardConn{Conn: conn}
	connID := uuid.New().String()

	h.Mu.Lock()
	h.Dashboards[connID] = nc
	if h.UserConns[userID] == nil {
		h.UserConns[userID] = make(map[string]bool)
	}
	h.UserConns[userID][connID] = true
	h.Mu.Unlock()

	workspaceID := c.Query("workspace_id")
	log.Printf("[Dashboard] Connected: %s (user: %s, workspace: %s)", connID, userID, workspaceID)

	// Send initial state
	h.sendDashboardInit(nc, userID, workspaceID)

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
		if h.UserConns[userID] != nil {
			delete(h.UserConns[userID], connID)
			if len(h.UserConns[userID]) == 0 {
				delete(h.UserConns, userID)
			}
		}
		h.Mu.Unlock()
		conn.Close()
		log.Printf("[Dashboard] Disconnected: %s", connID)
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *DashboardHub) sendDashboardInit(nc *DashboardConn, userID string, workspaceID string) {
	// Build set of currently active runtime endpoint node IDs
	activeNodes := make(map[string]bool)
	if h.Bus != nil {
		allRT := h.Bus.EndpointsByType(protocol.EndpointRuntime)
		log.Printf("[Dashboard] Found %d runtime endpoints on bus", len(allRT))
		for _, ep := range allRT {
			if len(ep.ID) > 9 && ep.ID[:9] == "runtime://" {
				activeNodes[ep.ID[9:]] = true
				log.Printf("[Dashboard] -> runtime endpoint: %s -> node=%s", ep.ID, ep.ID[9:])
			} else {
				log.Printf("[Dashboard] -> non-matching endpoint: %s (len=%d)", ep.ID, len(ep.ID))
			}
		}
	}
	log.Printf("[Dashboard] activeNodes map: %v", activeNodes)

	// Fetch nodes — if workspaceID is set, show all workspace members' nodes
	nodes := make([]models.Node, 0)
	var rows *sql.Rows
	var err error
	if workspaceID != "" {
		rows, err = h.DB.Query(
			`SELECT n.id, n.user_id, n.name, n.os, n.arch, n.status, n.version, n.ip, n.last_seen, n.created_at
			 FROM nodes n
			 JOIN workspace_members wm ON wm.user_id = n.user_id
			 WHERE wm.workspace_id = $1
			 ORDER BY n.created_at DESC`, workspaceID,
		)
	} else {
		rows, err = h.DB.Query(
			`SELECT id, user_id, name, os, arch, status, version, ip, last_seen, created_at
			 FROM nodes WHERE user_id = $1 ORDER BY created_at DESC`, userID,
		)
	}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var n models.Node
			if err := rows.Scan(&n.ID, &n.UserID, &n.Name, &n.OS, &n.Arch, &n.Status, &n.Version, &n.IP, &n.LastSeen, &n.CreatedAt); err == nil {
				if activeNodes[n.ID] {
					n.Status = models.NodeStatusOnline
				}
				nodes = append(nodes, n)
			}
		}
	} else {
		log.Printf("[Dashboard] Failed to query nodes for init: %v", err)
	}

	// Compute can_manage for each node
	runtimePath := findRuntimePath()
	localIPs := getLocalIPs()
	for i := range nodes {
		if runtimePath != "" {
			for _, ip := range localIPs {
				if nodes[i].IP == ip {
					nodes[i].CanManage = true
					break
				}
			}
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
		log.Printf("[Dashboard] Failed to query sessions for init: %v", err)
	}

	payload := map[string]interface{}{
		"nodes":    nodes,
		"sessions": sessions,
	}
	data, _ := json.Marshal(wsMessage{Type: "init", Payload: mustJSON(payload)})
	nc.Mu.Lock()
	nc.Conn.WriteMessage(websocket.TextMessage, data)
	nc.Mu.Unlock()
}

// BroadcastToDashboards sends a message to all connected dashboard clients.
func (h *DashboardHub) BroadcastToDashboards(msgType string, payload interface{}) {
	data, err := json.Marshal(wsMessage{Type: msgType, Payload: mustJSON(payload)})
	if err != nil {
		return
	}

	h.Mu.RLock()
	defer h.Mu.RUnlock()

	for id, dc := range h.Dashboards {
		dc.Mu.Lock()
		if err := dc.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[Dashboard] Write error (%s): %v", id, err)
		}
		dc.Mu.Unlock()
	}
}

// BroadcastSessionUpdate sends a session status update to all dashboard clients.
func (h *DashboardHub) BroadcastSessionUpdate(sessionID string, status interface{}, prompt, workspace, nodeID string) {
	h.BroadcastToDashboards("session_update", map[string]interface{}{
		"id":        sessionID,
		"status":    status,
		"prompt":    prompt,
		"workspace": workspace,
		"node_id":   nodeID,
	})
}

// SignalChange broadcasts a lightweight "resource changed" signal to all dashboard clients.
// Components use this to know when to refetch data.
func (h *DashboardHub) SignalChange(resource string) {
	h.BroadcastToDashboards("resource_change", map[string]string{
		"resource": resource,
	})
}

// SignalUser sends a "resource changed" signal only to a specific user's dashboard connections.
// If the user has no active connections, the signal is silently dropped.
func (h *DashboardHub) SignalUser(userID string, resource string) {
	data, err := json.Marshal(wsMessage{Type: "resource_change", Payload: mustJSON(map[string]string{
		"resource": resource,
	})})
	if err != nil {
		return
	}

	h.Mu.RLock()
	connIDs := h.UserConns[userID]
	h.Mu.RUnlock()

	for connID := range connIDs {
		h.Mu.RLock()
		dc := h.Dashboards[connID]
		h.Mu.RUnlock()
		if dc == nil {
			continue
		}
		dc.Mu.Lock()
		if err := dc.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[Dashboard] SignalUser write error (%s): %v", connID, err)
		}
		dc.Mu.Unlock()
	}
}

// SendNotification sends a notification message to a specific user's dashboard connections.
// The notification appears as a visible popup/toast in the UI.
func (h *DashboardHub) SendNotification(userID string, notifType string, title, message string) {
	data, err := json.Marshal(wsMessage{Type: "notification", Payload: mustJSON(map[string]string{
		"type":    notifType,
		"title":   title,
		"message": message,
	})})
	if err != nil {
		return
	}

	h.Mu.RLock()
	connIDs := h.UserConns[userID]
	h.Mu.RUnlock()

	for connID := range connIDs {
		h.Mu.RLock()
		dc := h.Dashboards[connID]
		h.Mu.RUnlock()
		if dc == nil {
			continue
		}
		dc.Mu.Lock()
		if err := dc.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[Dashboard] SendNotification write error (%s): %v", connID, err)
		}
		dc.Mu.Unlock()
	}
}

// wsMessage is the wire format for dashboard WebSocket messages.
type wsMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func mustJSON(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
