package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/coaether/server/protocol"
)

// BusHandler handles WebSocket connections that speak the Message Bus protocol.
// It replaces the functionality previously split across HandleNodeWS and HandleUIWS.
type BusHandler struct {
	DB             *sql.DB
	Bus            *protocol.MessageBus
	SessionService *SessionService
	mu             sync.Mutex
	connMutex      map[*websocket.Conn]*sync.Mutex
	Hub            *DashboardHub // for broadcasting to dashboards
}

var busUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// NewBusHandler creates a BusHandler.
func NewBusHandler(bus *protocol.MessageBus, db *sql.DB) *BusHandler {
	return &BusHandler{
		Bus:       bus,
		DB:        db,
		connMutex: make(map[*websocket.Conn]*sync.Mutex),
	}
}

// writeJSON safely writes a JSON message to a WebSocket connection.
func (h *BusHandler) writeJSON(conn *websocket.Conn, v any) error {
	h.mu.Lock()
	mtx, ok := h.connMutex[conn]
	if !ok {
		mtx = &sync.Mutex{}
		h.connMutex[conn] = mtx
	}
	h.mu.Unlock()

	mtx.Lock()
	defer mtx.Unlock()
	return conn.WriteJSON(v)
}

// writeMessage safely writes a raw WebSocket message.
func (h *BusHandler) writeMessage(conn *websocket.Conn, msgType int, data []byte) error {
	h.mu.Lock()
	mtx, ok := h.connMutex[conn]
	if !ok {
		mtx = &sync.Mutex{}
		h.connMutex[conn] = mtx
	}
	h.mu.Unlock()

	mtx.Lock()
	defer mtx.Unlock()
	return conn.WriteMessage(msgType, data)
}

// cleanupConn removes mutex tracking for a connection.
func (h *BusHandler) cleanupConn(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.connMutex, conn)
	h.mu.Unlock()
}

// HandleWS is the unified entry point for all WebSocket connections.
// Query parameters:
//
//	?type=ui&user_id=xxx            → ui://<user_id>/<conn_id>
//	?type=runtime&node_id=xxx       → runtime://<node_id>
func (h *BusHandler) HandleWS(c *gin.Context) {
	epType := c.Query("type")
	if epType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing type"})
		return
	}

	conn, err := busUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[Bus] Upgrade error: %v", err)
		return
	}

	// Build endpoint address
	var endpointID string
	metadata := make(map[string]any)

	var tokenUserID string

	switch epType {
	case "ui":
		userID := c.Query("user_id")
		connID := newConnID()
		endpointID = "ui://" + userID + "/" + connID
		metadata["user_id"] = userID

	case "runtime":
		// Path A: Reconnect using node_secret
		secret := c.Query("secret")
		if secret != "" {
			secretHash := sha256.Sum256([]byte(secret))
			secretHashHex := hex.EncodeToString(secretHash[:])

			var nodeID, userID, nodeName string
			err := h.DB.QueryRow(
				`SELECT id, user_id, name FROM nodes WHERE node_secret_hash = $1`,
				secretHashHex,
			).Scan(&nodeID, &userID, &nodeName)
			if err != nil {
				conn.Close()
				log.Printf("[Bus] Secret lookup failed: %v", err)
				return
			}

			endpointID = "runtime://" + nodeID
			metadata["node_id"] = nodeID
			metadata["user_id"] = userID
			metadata["node_name"] = nodeName
			tokenUserID = userID

			h.DB.Exec(
				`UPDATE nodes SET status = 'online', last_seen = NOW(), ip = $2 WHERE id = $1`,
				nodeID, c.ClientIP(),
			)
		} else {
			// Path B: First-time registration using token
			token := c.Query("token")
			if token == "" {
				conn.Close()
				c.JSON(http.StatusBadRequest, gin.H{"error": "missing token or secret"})
				return
			}

			var nodeName, status string
			var expiresAt time.Time
			err := h.DB.QueryRow(
				`SELECT user_id, node_name, status, expires_at FROM node_join_tokens WHERE token = $1`,
				token,
			).Scan(&tokenUserID, &nodeName, &status, &expiresAt)
			if err != nil {
				conn.Close()
				log.Printf("[Bus] Token lookup failed: %v", err)
				return
			}
			if status != "pending" {
				// Used token + offline node → reconnect with fresh secret
				if status == "used" {
					var existingNodeID string
					if err := h.DB.QueryRow(
						`SELECT node_id FROM node_join_tokens WHERE token = $1`, token,
					).Scan(&existingNodeID); err == nil && existingNodeID != "" {
						var nodeStatus string
						if err := h.DB.QueryRow(
							`SELECT status FROM nodes WHERE id = $1`, existingNodeID,
						).Scan(&nodeStatus); err == nil && nodeStatus == "offline" {
							secretBytes := make([]byte, 32)
							rand.Read(secretBytes)
							nodeSecret := hex.EncodeToString(secretBytes)
							secretHash := sha256.Sum256([]byte(nodeSecret))
							secretHashHex := hex.EncodeToString(secretHash[:])
							h.DB.Exec(
								`UPDATE nodes SET status='online', last_seen=NOW(), ip=$2, node_secret_hash=$3 WHERE id=$1`,
								existingNodeID, c.ClientIP(), secretHashHex,
							)
							endpointID = "runtime://" + existingNodeID
							metadata["node_id"] = existingNodeID
							metadata["user_id"] = tokenUserID
							metadata["node_name"] = nodeName
							regPayload := &protocol.Payload{
								Metadata: map[string]any{
									"node_secret": nodeSecret,
									"node_id":     existingNodeID,
								},
							}
							h.writeJSON(conn, protocol.NewEnvelope("system://bus", "", "registration", regPayload))
							log.Printf("[Bus] Reconnected existing node %s via used token", existingNodeID)
							goto afterFirstTime
						}
					}
				}
				conn.Close()
				log.Printf("[Bus] Token already %s", status)
				return
			}
			if time.Now().After(expiresAt) {
				h.DB.Exec(`UPDATE node_join_tokens SET status = 'expired' WHERE token = $1`, token)
				conn.Close()
				log.Printf("[Bus] Token expired")
				return
			}
			// Mark as used
			h.DB.Exec(`UPDATE node_join_tokens SET status = 'used', used_at = NOW() WHERE token = $1`, token)

			// Generate node_secret
			secretBytes := make([]byte, 32)
			rand.Read(secretBytes)
			nodeSecret := hex.EncodeToString(secretBytes)
			secretHash := sha256.Sum256([]byte(nodeSecret))
			secretHashHex := hex.EncodeToString(secretHash[:])

			// Create real UUID node record
			nodeID := uuid.New().String()
			endpointID = "runtime://" + nodeID
			metadata["node_id"] = nodeID
			metadata["user_id"] = tokenUserID
			metadata["node_name"] = nodeName

			h.DB.Exec(
				`INSERT INTO nodes (id, user_id, name, os, arch, status, version, ip,
					max_sessions, last_seen, created_at, node_secret_hash)
					 VALUES ($1, $2, $3, $4, $5, 'online', $6, $7, $8, NOW(), NOW(), $9)`,
				nodeID, tokenUserID, nodeName, "unknown", "unknown",
				"0.1.0", c.ClientIP(), 3, secretHashHex,
			)

			// Save node_id on token for future reconnection
			h.DB.Exec(`UPDATE node_join_tokens SET node_id = $1 WHERE token = $2`, nodeID, token)

			// Send registration message with node_secret to runtime
			regPayload := &protocol.Payload{
				Metadata: map[string]any{
					"node_secret": nodeSecret,
					"node_id":     nodeID,
				},
			}
			regEnv := protocol.NewEnvelope("system://bus", "", "registration", regPayload)
			h.writeJSON(conn, regEnv)
		}

	default:
		conn.Close()
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported type: " + epType})
		return
	}

		afterFirstTime:
		h.Bus.Register(endpointID, conn, metadata)
		log.Printf("[Bus] Connected: %s", endpointID)
			// Broadcast online status to dashboards (node record already created/updated above)
		if epType == "runtime" && h.Hub != nil {
			nodeID := metadata["node_id"].(string)
			nodeName := metadata["node_name"].(string)
			h.Hub.BroadcastToDashboards("node_status", map[string]interface{}{
				"node_id": nodeID,
				"name":    nodeName,
				"status":  "online",
			})
		}
	// Set up bus delivery for this connection using our safe writer
	h.Bus.SetEndpointDeliver(conn, func(env *protocol.Envelope) error {
		return h.writeJSON(conn, env)
	})

	// Heartbeat
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.writeMessage(conn, websocket.PingMessage, nil)
			case <-done:
				return
			}
		}
	}()

	// Read loop
	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var env protocol.Envelope
		if err := json.Unmarshal(msgBytes, &env); err != nil {
			log.Printf("[Bus] Invalid message from %s: %v", endpointID, err)
			continue
		}

		// Auto-set fields — always trust the actual connection's endpoint ID
		env.From = endpointID
		if env.Timestamp == 0 {
			env.Timestamp = time.Now().UnixMilli()
		}

		h.handleEnvelope(endpointID, &env)
	}

	// Cleanup — mark runtime sessions as failed before unregistering
	if epType == "runtime" && h.SessionService != nil {
		for _, sid := range h.Bus.GetEndpointSessions(endpointID) {
			h.SessionService.MarkFailed(sid, "runtime disconnected")
		}
	}
	h.Bus.Unregister(endpointID)
	h.cleanupConn(conn)
	conn.Close()
	log.Printf("[Bus] Disconnected: %s", endpointID)

	// Update DB and broadcast to dashboards
	if epType == "runtime" && h.Hub != nil {
		nodeID, _ := metadata["node_id"].(string)
		if nodeID != "" {
			h.Hub.DB.Exec("UPDATE nodes SET status = 'offline', last_seen = NOW() WHERE id = $1", nodeID)
			h.Hub.BroadcastToDashboards("node_status", map[string]interface{}{
				"node_id": nodeID,
				"status":  "offline",
			})
		}
	}
}

// handleEnvelope routes an incoming envelope.
func (h *BusHandler) handleEnvelope(from string, env *protocol.Envelope) {
	switch env.Type {
	case protocol.MsgHello:
		if ep := h.Bus.GetEndpoint(from); ep != nil && env.Payload != nil {
			ep.Capabilities = env.Payload.Capabilities
			if env.Payload.Metadata != nil {
				if ep.Metadata == nil {
					ep.Metadata = make(map[string]any)
				}
				for k, v := range env.Payload.Metadata {
					ep.Metadata[k] = v
				}
			}
		}
		log.Printf("[Bus] Hello from %s", from)

	case protocol.MsgPing:
		pong := protocol.NewEnvelope("system://bus", from, protocol.MsgPong, nil)
		h.Bus.Deliver(pong)

	case protocol.MsgSessionCreate:
		h.handleSessionCreate(env)

	case protocol.MsgSessionJoin:
		h.handleSessionJoin(env)

	case protocol.MsgSessionLeave:
		h.handleSessionLeave(env)

	case protocol.MsgSessionEnd:
		h.handleSessionEnd(env)

	default:
		h.Bus.Deliver(env)
	}
}

func (h *BusHandler) handleSessionCreate(env *protocol.Envelope) {
	if env.Payload == nil {
		return
	}

	sessionID := env.SessionID
	if sessionID == "" {
		sessionID = newConnID()
		env.SessionID = sessionID
	}

	members := map[string]protocol.MemberRole{env.From: protocol.RoleOwner}

	h.Bus.CreateSession(sessionID, members)
	log.Printf("[Bus] Session created: %s by %s", sessionID, env.From)

	// Respond to creator
	created := protocol.NewEnvelope("system://bus", env.From, protocol.MsgSessionCreated,
		&protocol.Payload{
			Members: []protocol.MemberSpec{
				{Endpoint: env.From, Role: "owner"},
			},
		},
	)
	created.SessionID = sessionID
	created.ReplyTo = env.ID
	h.Bus.Deliver(created)

	// Forward to runtimes that can handle the requested agents
	for _, as := range env.Payload.Agents {
		runtimes := h.Bus.FindRuntimesForAgent(as.ID)
		for _, rt := range runtimes {
			forward := protocol.NewEnvelope("system://bus", rt, protocol.MsgSessionCreate,
				&protocol.Payload{
					Agents:    []protocol.AgentSpec{as},
					Workspace: env.Payload.Workspace,
					Context:   env.Payload.Context,
				},
			)
			forward.SessionID = sessionID
			forward.ReplyTo = env.ID
			h.Bus.Deliver(forward)
		}
	}
}

func (h *BusHandler) handleSessionJoin(env *protocol.Envelope) {
	if env.SessionID == "" {
		return
	}
	ok := h.Bus.JoinSession(env.SessionID, env.From, protocol.RoleMember)

	// Mark DB session as running when a runtime joins
	addr := protocol.ParseAddr(env.From)
	if ok && addr.Type == protocol.EndpointRuntime && h.SessionService != nil {
		h.SessionService.MarkRunning(env.SessionID)
	}

	if !ok {
		// Session doesn't exist on the bus — likely a stale session from a previous server run
		log.Printf("[Bus] Join failed: session %s not found for %s", env.SessionID, env.From)
		errEnv := protocol.NewEnvelope("system://bus", env.From,
			protocol.MsgError, &protocol.Payload{
				Code:    "SESSION_NOT_FOUND",
				Message: "Session not found on bus (may have ended)",
			})
		errEnv.SessionID = env.SessionID
		errEnv.ReplyTo = env.ID
		h.Bus.Deliver(errEnv)
		return
	}

	// If the joining endpoint is a UI and the session has no runtime, invite available runtimes
	addr = protocol.ParseAddr(env.From)
	if addr.Type == protocol.EndpointUI {
		sess := h.Bus.GetSession(env.SessionID)
		hasRuntime := false
		if sess != nil {
			for memberID := range sess.Members {
				maddr := protocol.ParseAddr(memberID)
				if maddr.Type == protocol.EndpointRuntime {
					hasRuntime = true
					break
				}
			}
		}
		if !hasRuntime {
			for _, rt := range h.Bus.EndpointsByType(protocol.EndpointRuntime) {
				forward := protocol.NewEnvelope("system://bus", rt.ID, protocol.MsgSessionCreate,
					&protocol.Payload{
						Agents: []protocol.AgentSpec{{ID: "claude"}},
					},
				)
				forward.SessionID = env.SessionID
				h.Bus.Deliver(forward)
				log.Printf("[Bus] Forwarded session.create to runtime %s for session %s", rt.ID, env.SessionID)
			}
		}
	}

	joined := protocol.NewEnvelope("system://bus", "session://"+env.SessionID,
		protocol.MsgSessionJoined, &protocol.Payload{
			Members: []protocol.MemberSpec{
				{Endpoint: env.From, Role: "member"},
			},
		},
	)
	joined.SessionID = env.SessionID
	h.Bus.BroadcastToSession(env.SessionID, joined)
}

func (h *BusHandler) handleSessionLeave(env *protocol.Envelope) {
	if env.SessionID == "" {
		return
	}
	h.Bus.LeaveSession(env.SessionID, env.From)
}

func (h *BusHandler) handleSessionEnd(env *protocol.Envelope) {
	if env.SessionID == "" {
		return
	}
	addr := protocol.ParseAddr(env.From)
	if addr.Type == protocol.EndpointRuntime && h.SessionService != nil {
		output := ""
		if env.Payload != nil {
			output = env.Payload.Output
		}
		h.SessionService.MarkCompleted(env.SessionID, output)
	}
	h.Bus.EndSession(env.SessionID)
}

var connIDCounter int64

func newConnID() string {
	connIDCounter++
	return "c" + itoa(connIDCounter)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
