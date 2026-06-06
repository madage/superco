package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/superco/server/protocol"
)

// BusHandler handles WebSocket connections that speak the Message Bus protocol.
// It replaces the functionality previously split across HandleNodeWS and HandleUIWS.
type BusHandler struct {
	Bus       *protocol.MessageBus
	mu        sync.Mutex
	connMutex map[*websocket.Conn]*sync.Mutex
	Hub       *DashboardHub // for broadcasting to dashboards
}

var busUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// NewBusHandler creates a BusHandler.
func NewBusHandler(bus *protocol.MessageBus) *BusHandler {
	return &BusHandler{
		Bus:       bus,
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

	switch epType {
	case "ui":
		userID := c.Query("user_id")
		connID := newConnID()
		endpointID = "ui://" + userID + "/" + connID
		metadata["user_id"] = userID

	case "runtime":
		nodeID := c.Query("node_id")
		if nodeID == "" {
			conn.Close()
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing node_id"})
			return
		}
		endpointID = "runtime://" + nodeID
		metadata["node_id"] = nodeID

	default:
		conn.Close()
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported type: " + epType})
		return
	}

	// Register with the bus and set custom delivery function
	h.Bus.Register(endpointID, conn, metadata)
	log.Printf("[Bus] Connected: %s", endpointID)
		// Upsert bus node in DB and broadcast to dashboards
		if epType == "runtime" && h.Hub != nil {
			sanID := "bus-" + strings.ReplaceAll(endpointID, "://", "--")
			h.Hub.DB.Exec(
				`INSERT INTO nodes (id, user_id, name, os, status, version, ip, max_sessions, last_seen, created_at)
				 VALUES ($1, (SELECT id FROM users ORDER BY created_at ASC LIMIT 1), $2, $3, 'online', $4, $5, $6, NOW(), NOW())
				 ON CONFLICT (id) DO UPDATE SET status = 'online', last_seen = NOW()`,
				sanID, "Runtime: "+endpointID, "unknown", "0.1.0", "bus", 3,
			)
			h.Hub.BroadcastToDashboards("node_status", map[string]interface{}{
				"node_id": sanID,
				"name":    endpointID,
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

	// Cleanup
	h.Bus.Unregister(endpointID)
	h.cleanupConn(conn)
	conn.Close()
	log.Printf("[Bus] Disconnected: %s", endpointID)

	// Update DB and broadcast to dashboards
	if epType == "runtime" && h.Hub != nil {
		sanID := "bus-" + strings.ReplaceAll(endpointID, "://", "--")
		h.Hub.DB.Exec("UPDATE nodes SET status = 'offline', last_seen = NOW() WHERE id = $1", sanID)
		h.Hub.BroadcastToDashboards("node_status", map[string]interface{}{
			"node_id": sanID,
			"name":    endpointID,
			"status":  "offline",
		})
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
	addr := protocol.ParseAddr(env.From)
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
