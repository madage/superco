package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/coaether/agent-runtime/backends"
	"github.com/coaether/server/protocol"
)

// Runtime connects to the Message Bus and manages agent backends.
type Runtime struct {
	ServerURL string
	NodeID    string
	Name      string
	Token     string
	Secret    string

	conn      *websocket.Conn
	connMu    sync.Mutex
	backends  map[string]Backend
	endpoint  string
}

// NewRuntime creates a new Runtime.
func NewRuntime(serverURL, nodeID, name, token, secret string) *Runtime {
	return &Runtime{
		ServerURL: serverURL,
		NodeID:    nodeID,
		Name:      name,
		Token:     token,
		Secret:    secret,
		backends:  make(map[string]Backend),
		endpoint:  "runtime://" + nodeID,
	}
}

// RegisterBackend adds a backend handler for a specific agent ID.
func (r *Runtime) RegisterBackend(agentID string, backend Backend) {
	r.backends[agentID] = backend
	log.Printf("[Runtime] Registered backend: %s (%s)", agentID, backend.Name())
}

// Run connects to the Message Bus and starts the message loop.
func (r *Runtime) Run() error {
	q := url.Values{
		"type": {"runtime"},
	}
	if r.Secret != "" {
		// Reconnect path: use persistent node_secret
		q.Set("secret", r.Secret)
		if r.NodeID != "" {
			q.Set("node_id", r.NodeID)
		}
	} else if r.Token != "" {
		// First-time registration path: use one-time token
		q.Set("token", r.Token)
		q.Set("node_id", r.NodeID)
	}
	u := url.URL{
		Scheme:   "ws",
		Host:     r.ServerURL,
		Path:     "/ws/bus",
		RawQuery: q.Encode(),
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	r.conn = conn
	log.Printf("[Runtime] Connected to bus as %s", r.endpoint)

	r.sendHello()

	done := make(chan struct{})
	defer close(done)
	go func() {
		pingTicker := time.NewTicker(30 * time.Second)
		cleanTicker := time.NewTicker(60 * time.Second)
		defer pingTicker.Stop()
		defer cleanTicker.Stop()
		for {
			select {
			case <-pingTicker.C:
				r.sendPing()
			case <-cleanTicker.C:
				r.cleanIdleSessions()
			case <-done:
				return
			}
		}
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var env protocol.Envelope
		if err := json.Unmarshal(msgBytes, &env); err != nil {
			log.Printf("[Runtime] Invalid message: %v", err)
			continue
		}
		r.handleMessage(&env)
	}
}

func (r *Runtime) sendHello() {
	caps := make([]protocol.Capability, 0, len(r.backends))
	for id, b := range r.backends {
		caps = append(caps, protocol.Capability{
			ID:      id,
			Name:    b.Name(),
			Version: b.Version(),
			Backend: "api",
		})
	}

	r.send(protocol.NewEnvelope(r.endpoint, "system://bus", protocol.MsgHello,
		&protocol.Payload{Capabilities: caps, EndpointType: "runtime",
			Metadata: map[string]any{
				"name":    r.Name,
				"version": "0.1.0",
			},
		}))
}

func (r *Runtime) sendPing() {
	r.send(protocol.NewEnvelope(r.endpoint, "system://bus", protocol.MsgPing, nil))
}

func (r *Runtime) handleMessage(env *protocol.Envelope) {
	switch env.Type {
	case "registration":
		log.Printf("[Runtime] Registration received")
		if env.Payload != nil {
			if nodeID, ok := env.Payload.Metadata["node_id"]; ok {
				r.NodeID = nodeID.(string)
				r.endpoint = "runtime://" + nodeID.(string)
			}
			if secret, ok := env.Payload.Metadata["node_secret"]; ok {
				r.saveNodeSecret(secret.(string))
			}
		}

	case protocol.MsgPong:
		// heartbeat ok

	case protocol.MsgSessionCreate:
		log.Printf("[Runtime] Session create received: %s", env.SessionID)
		join := protocol.NewEnvelope(r.endpoint, "system://bus", protocol.MsgSessionJoin, nil)
		join.SessionID = env.SessionID
		r.send(join)

	case protocol.MsgSessionJoined:
		log.Printf("[Runtime] Joined session: %s", env.SessionID)

	case protocol.MsgSessionEnd:
		log.Printf("[Runtime] Session end: %s", env.SessionID)
		if cli, ok := r.backends["claude"].(*backends.ClaudeCLIBackend); ok {
			cli.CloseSession(env.SessionID)
		}

	case protocol.MsgMessage:
		log.Printf("[Runtime] Message received for session %s from %s", env.SessionID, env.From)
		r.handleAgentMessage(env)

	case protocol.MsgEvent, protocol.MsgToolUse, protocol.MsgToolResult:
		// Session-scoped events consumed by UI clients

	case protocol.MsgPermissionResponse:
		log.Printf("[Runtime] Permission response for session %s", env.SessionID)
		if cli, ok := r.backends["claude"].(*backends.ClaudeCLIBackend); ok {
			cli.HandlePermissionResponse(env)
		}

	default:
		log.Printf("[Runtime] Unhandled type: %s", env.Type)
	}
}

func (r *Runtime) handleAgentMessage(env *protocol.Envelope) {
	if env.Payload == nil {
		return
	}

	// Route to the first matching backend
	for _, backend := range r.backends {
		resp, err := backend.HandleMessage(env)
		if err != nil {
			log.Printf("[Runtime] Backend error: %v", err)
			r.send(protocol.NewEnvelope(
				r.endpoint, env.From, protocol.MsgError,
				&protocol.Payload{Code: "BACKEND_ERROR", Message: err.Error()},
			).WithSession(env.SessionID).WithReplyTo(env.ID))
		}
		if resp != nil {
			resp.From = r.endpoint
			resp.To = env.From
			resp.SessionID = env.SessionID
			resp.ReplyTo = env.ID
			r.send(resp)
		}
		break
	}
}

func (r *Runtime) send(env *protocol.Envelope) {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	if r.conn == nil {
		return
	}
	if err := r.conn.WriteJSON(env); err != nil {
		log.Printf("[Runtime] Write error: %v", err)
	}
}

// Shutdown gracefully disconnects from the bus and closes the WebSocket connection.
func (r *Runtime) Shutdown() {
	log.Println("[Runtime] Shutting down...")

	r.connMu.Lock()
	if r.conn != nil {
		r.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		bye := protocol.NewEnvelope(r.endpoint, "system://bus", protocol.MsgBye, nil)
		if err := r.conn.WriteJSON(bye); err != nil {
			log.Printf("[Runtime] Bye send error: %v", err)
		}
		r.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"))
		r.conn.Close()
		r.conn = nil
	}
	r.connMu.Unlock()

	log.Println("[Runtime] Shutdown complete")
}

func (r *Runtime) cleanIdleSessions() {
	if cli, ok := r.backends["claude"].(*backends.ClaudeCLIBackend); ok {
		cli.CleanIdleSessions()
	}
}

// Backend handles messages for a specific AI agent.
type Backend interface {
	Name() string
	Version() string
	HandleMessage(env *protocol.Envelope) (*protocol.Envelope, error)
}

// loadConfig reads ~/.coaether/env and sets env vars if not already set.

// saveNodeSecret persists the node_secret to ~/.coaether/env (and optionally node_id).
func (r *Runtime) saveNodeSecret(secret string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[Runtime] Cannot save node secret: %v", err)
		return
	}
	envPath := filepath.Join(homeDir, ".coaether", "env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		data = []byte("SERVER_URL=" + r.ServerURL + "\nNODE_TOKEN=\nNODE_SECRET=\nRUNTIME_NAME=\n")
	}
	lines := strings.Split(string(data), "\n")
	secretFound := false
	for i, line := range lines {
		if strings.HasPrefix(line, "NODE_SECRET=") {
			lines[i] = "NODE_SECRET=" + secret
			secretFound = true
			break
		}
	}
	if !secretFound {
		lines = append(lines, "NODE_SECRET="+secret)
	}
	if r.NodeID != "" {
		idFound := false
		for i, line := range lines {
			if strings.HasPrefix(line, "NODE_ID=") {
				lines[i] = "NODE_ID=" + r.NodeID
				idFound = true
				break
			}
		}
		if !idFound {
			lines = append(lines, "NODE_ID="+r.NodeID)
		}
	}
	os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0644)
	log.Printf("[Runtime] Node secret saved to %s", envPath)
}

func loadConfig() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(homeDir, ".coaether", "env"))
	if err != nil {
		return // config file doesn't exist, use env vars or defaults
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if os.Getenv(key) != "" {
			continue // don't override existing env vars
		}
		os.Setenv(key, val)
	}
}

func writePIDFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	pidFile := filepath.Join(home, ".coaether", "runtime.pid")
	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(pidFile, []byte(pid+"\n"), 0644); err != nil {
		log.Printf("[Runtime] Failed to write PID file: %v", err)
		return ""
	}
	return pidFile
}

func removePIDFile(pidFile string) {
	if pidFile != "" {
		os.Remove(pidFile)
	}
}

func runStart() {
	loadConfig()

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "localhost:8088"
	}

	name := os.Getenv("RUNTIME_NAME")
	if name == "" {
		name, _ = os.Hostname()
	}

	// Write PID file so "agent-runtime stop" can find this process
	pidFile := writePIDFile()
	defer removePIDFile(pidFile)

	// Prefer persistent node_secret over one-time token
	nodeSecret := os.Getenv("NODE_SECRET")
	if nodeSecret != "" {
		nodeID := os.Getenv("NODE_ID")
		log.Printf("[Runtime] Reconnecting with persistent secret, node=%s", nodeID)
		rt := NewRuntime(serverURL, nodeID, name, "", nodeSecret)
		rt.registerBackends()
		rt.runLoop()
		return
	}

	nodeToken := os.Getenv("NODE_TOKEN")
	if nodeToken == "" {
		log.Fatal("[Runtime] NODE_TOKEN or NODE_SECRET is required. Generate a token via the CoAether Web UI (Nodes -> Add Node).")
	}

	// Derive deterministic node ID from token (matches old server-side HashToken)
	h := sha256.Sum256([]byte(nodeToken))
	nodeID := "tok-" + hex.EncodeToString(h[:8])

	log.Printf("[Runtime] First-time registration with token, node=%s, server=%s", nodeID, serverURL)

	rt := NewRuntime(serverURL, nodeID, name, nodeToken, "")
	rt.registerBackends()
	rt.runLoop()
}

func (r *Runtime) registerBackends() {
	if cli := backends.NewClaudeCLIBackend(""); cli != nil {
		cli.SetSendFunc(r.send)
		r.RegisterBackend("claude", cli)
		log.Println("[Runtime] Claude CLI backend enabled (stream-json)")
	} else if api := backends.NewClaudeBackend(); api != nil {
		r.RegisterBackend("claude", api)
		log.Println("[Runtime] Claude API backend enabled")
	} else {
		r.RegisterBackend("echo", backends.NewEchoBackend())
		log.Println("[Runtime] No claude CLI or API key, using echo backend")
	}
}

func (r *Runtime) runLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := r.Run()
				if err != nil {
					log.Printf("[Runtime] Connection error: %v (retry in 3s)", err)
					select {
					case <-ctx.Done():
						return
					case <-time.After(3 * time.Second):
					}
				} else {
					return
				}
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	signal.Stop(sig)
	log.Println("[Runtime] Shutting down...")
	cancel()
	r.Shutdown()
}
