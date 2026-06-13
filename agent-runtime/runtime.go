package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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

	conn        *websocket.Conn
	connMu      sync.Mutex
	backends    map[string]Backend
	endpoint    string
	sessionMeta        map[string]map[string]string // sessionID → {queueID, taskID, ...}
	recentlyCompleted  map[string]time.Time          // "taskID:agentProfileID" → completion time
}

// NewRuntime creates a new Runtime.
func NewRuntime(serverURL, nodeID, name, token, secret string) *Runtime {
	return &Runtime{
		ServerURL:   serverURL,
		NodeID:      nodeID,
		Name:        name,
		Token:       token,
		Secret:      secret,
		backends:    make(map[string]Backend),
		sessionMeta: make(map[string]map[string]string),
		recentlyCompleted: make(map[string]time.Time),
		endpoint:    "runtime://" + nodeID,
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

	// Start agent queue poller for autonomous task processing
	r.startQueuePoller()

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
		// Store session context (queue_id, task_id) for completion callback
		if env.Payload != nil && env.Payload.Context != nil {
			ctx, ok := env.Payload.Context.(map[string]any)
			if !ok {
				break
			}
			meta := make(map[string]string)
			for k, v := range ctx {
				switch val := v.(type) {
				case string:
					meta[k] = val
				case bool:
					if val {
						meta[k] = "true"
					}
				}
			}
			r.connMu.Lock()
			r.sessionMeta[env.SessionID] = meta
			r.connMu.Unlock()
			log.Printf("[Runtime] Session context: %v", meta)
		}
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

	case protocol.MsgEvent, protocol.MsgToolResult:
		// Session-scoped events consumed by UI clients

	case protocol.MsgToolUse:
		// Intercept tool calls from auto-task sessions for Harness execution
		if r.isAutoTaskSession(env.SessionID) && env.Payload != nil && env.Payload.Tool != "" {
			r.handleAutoTaskToolCall(env)
		}
		// Non-auto-task sessions: tool_use is forwarded to UI by backend

	case protocol.MsgPermissionResponse:
		log.Printf("[Runtime] Permission response for session %s", env.SessionID)
		if cli, ok := r.backends["claude"].(*backends.ClaudeCLIBackend); ok {
			cli.HandlePermissionResponse(env)
		}

	case protocol.MsgNodeStop:
		log.Printf("[Runtime] Node stop received, shutting down...")
		r.Shutdown()
		os.Exit(0)

	case protocol.MsgAgentMention:
		r.handleAgentMention(env)

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

// Built-in Harness tools that are forwarded to the server Harness API.
// MCP/built-in Claude Code tools (WebSearch, WebFetch, Grep, Read, etc.)
// are handled locally by the runtime.
var harnessTools = map[string]bool{
	"propose_decomposition_plan": true,
	"create_sub_task":            true,
	"assign_task":                true,
	"review_task":                true,
	"add_comment":                true,
	"get_task_detail":            true,
	"list_sub_tasks":             true,
	"update_task_status":         true,
		"search_agent_profiles":      true,
}

// isAutoTaskSession checks if the session is for an auto-task agent.
func (r *Runtime) isAutoTaskSession(sessionID string) bool {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	meta, ok := r.sessionMeta[sessionID]
	return ok && meta["is_auto_task"] == "true"
}

// handleAutoTaskToolCall routes tool calls from auto-task sessions:
// - MCP-prefixed harness tools (mcp__coaether-harness__create_sub_task) → server Harness API
// - Plain harness tools (create_sub_task) → server Harness API
// - Non-harness MCP tools → error with redirect hint to use coaether-harness tools
// - Built-in tools → handled locally or returned as unavailable
func (r *Runtime) handleAutoTaskToolCall(env *protocol.Envelope) {
	toolName := env.Payload.Tool

	// Strip mcp__<server>__ prefix to get the base tool name
	if strings.HasPrefix(toolName, "mcp__") {
		baseName := toolName
		if parts := strings.SplitN(toolName, "__", 3); len(parts) == 3 {
			baseName = parts[2]
		}

		if harnessTools[baseName] {
			// MCP server handles this — do NOT forward to Harness API
			return
		}

		// Non-harness MCP tool: return error with redirect hint
		log.Printf("[Runtime] Rejecting non-harness MCP tool: %s (base: %s)", toolName, baseName)
		if cli, ok := r.backends["claude"].(*backends.ClaudeCLIBackend); ok {
			cli.SendToolResult(env.SessionID, env.Payload.ToolUseID, map[string]interface{}{
				"status": "error",
				"error": map[string]interface{}{
					"message": fmt.Sprintf(
						"MCP tool '%s' is not available. You are a task-decomposition agent. Use ONLY mcp__coaether-harness__ tools: propose_decomposition_plan, list_sub_tasks, add_comment, get_task_detail.",
						toolName,
					),
				},
			})
		}
		return
	}

	if harnessTools[toolName] {
		// Harness tool without MCP prefix — MCP server handles this too
	} else {
		r.handleMCPToolCall(env)
	}
}

// handleMCPToolCall executes an MCP or built-in Claude Code tool locally.
func (r *Runtime) handleMCPToolCall(env *protocol.Envelope) {
	toolName := env.Payload.Tool
	toolUseID := env.Payload.ToolUseID

	log.Printf("[Runtime] Executing MCP tool locally: %s", toolName)

	var result map[string]interface{}
	switch toolName {
	case "WebSearch":
		result = r.executeWebSearch(env)
	case "WebFetch":
		result = r.executeWebFetch(env)
	default:
		result = r.handleUnavailableTool(toolName, env)
	}

	if cli, ok := r.backends["claude"].(*backends.ClaudeCLIBackend); ok {
		cli.SendToolResult(env.SessionID, toolUseID, result)
		log.Printf("[Runtime] MCP tool result sent: tool=%s status=%s", toolName, result["status"])
	}
}

// executeWebSearch performs a web search by querying DuckDuckGo's instant answer API.
func (r *Runtime) executeWebSearch(env *protocol.Envelope) map[string]interface{} {
	var params struct {
		Query string `json:"query"`
	}
	inputStr, ok := env.Payload.Input.(string)
	if ok {
		json.Unmarshal([]byte(inputStr), &params)
	} else {
		paramBytes, _ := json.Marshal(env.Payload.Input)
		json.Unmarshal(paramBytes, &params)
	}

	if params.Query == "" {
		return map[string]interface{}{"status": "error", "error": map[string]interface{}{"message": "query is required"}}
	}

	log.Printf("[Runtime] WebSearch: %s", params.Query)

	// Try DuckDuckGo instant answer API
	client := &http.Client{Timeout: 10 * time.Second}
	ddgURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1", url.QueryEscape(params.Query))
	resp, err := client.Get(ddgURL)
	if err != nil {
		log.Printf("[Runtime] WebSearch request failed: %v", err)
		// Fallback: try a simple web fetch
		return r.fallbackWebSearch(params.Query)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var ddgResult struct {
		AbstractText string `json:"AbstractText"`
		AbstractURL  string `json:"AbstractURL"`
		Answer       string `json:"Answer"`
		Results      []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"Results"`
	}
	json.Unmarshal(body, &ddgResult)

	// Build search results text
	var sb strings.Builder
	if ddgResult.AbstractText != "" {
		sb.WriteString("Summary: " + ddgResult.AbstractText + "\n")
		if ddgResult.AbstractURL != "" {
			sb.WriteString("Source: " + ddgResult.AbstractURL + "\n")
		}
	}
	if ddgResult.Answer != "" {
		sb.WriteString("Answer: " + ddgResult.Answer + "\n")
	}
	for _, r := range ddgResult.Results {
		sb.WriteString(fmt.Sprintf("- %s (%s)\n", r.Text, r.FirstURL))
	}

	if sb.Len() == 0 {
		return r.fallbackWebSearch(params.Query)
	}

	return map[string]interface{}{
		"status": "success",
		"result": sb.String(),
	}
}

// fallbackWebSearch tries a simple HTTP GET as fallback when DuckDuckGo API returns nothing.
func (r *Runtime) fallbackWebSearch(query string) map[string]interface{} {
	client := &http.Client{Timeout: 10 * time.Second}
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	resp, err := client.Get(searchURL)
	if err != nil {
		return map[string]interface{}{"status": "error", "error": map[string]interface{}{"message": err.Error()}}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return map[string]interface{}{
		"status": "success",
		"result": fmt.Sprintf("Search results for '%s':\n%s", query, truncateStr(string(body), 5000)),
	}
}

// executeWebFetch fetches content from a URL and returns it as text.
func (r *Runtime) executeWebFetch(env *protocol.Envelope) map[string]interface{} {
	var params struct {
		URL string `json:"url"`
	}
	inputStr, ok := env.Payload.Input.(string)
	if ok {
		json.Unmarshal([]byte(inputStr), &params)
	} else {
		paramBytes, _ := json.Marshal(env.Payload.Input)
		json.Unmarshal(paramBytes, &params)
	}

	if params.URL == "" {
		return map[string]interface{}{"status": "error", "error": map[string]interface{}{"message": "url is required"}}
	}

	log.Printf("[Runtime] WebFetch: %s", params.URL)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(params.URL)
	if err != nil {
		return map[string]interface{}{"status": "error", "error": map[string]interface{}{"message": err.Error()}}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	contentType := resp.Header.Get("Content-Type")

	return map[string]interface{}{
		"status": "success",
		"result": truncateStr(string(body), 10000),
		"content_type": contentType,
		"url":    params.URL,
	}
}

// handleUnavailableTool returns a helpful response for tools that aren't available in auto-task mode.
// The response guides the agent to use only Harness task-management tools instead.
func (r *Runtime) handleUnavailableTool(toolName string, env *protocol.Envelope) map[string]interface{} {
	// For Bash, return empty output so the agent doesn't get stuck
	if toolName == "Bash" {
		return map[string]interface{}{
			"status":      "success",
			"exit_code":   0,
			"stdout":      "",
			"stderr":      "Bash execution is not available in auto-task mode. Use create_sub_task to delegate work.",
		}
	}

	// For Write, acknowledge the write but note it has no effect
	if toolName == "Write" {
		return map[string]interface{}{
			"status":  "success",
			"message": "File written. Note: This file will not persist. Use add_comment to record results on a task.",
		}
	}

	// For Read/Glob/Grep, return empty results
	if toolName == "Read" || toolName == "Glob" || toolName == "Grep" || toolName == "Edit" {
		return map[string]interface{}{
			"status": "error",
			"error":  map[string]interface{}{"message": "File system tools are not available in auto-task mode. Use get_task_detail and list_sub_tasks to access task data."},
		}
	}

	// For all other tools (including mcp__*), return a clear guidance message
	return map[string]interface{}{
		"status": "error",
		"error": map[string]interface{}{
			"message": fmt.Sprintf("Tool '%s' is not available in auto-task mode. Available tools: create_sub_task, assign_task, review_task, add_comment, get_task_detail, list_sub_tasks, update_task_status.", toolName),
		},
	}
}

// handleHarnessToolCall sends a tool call to the server Harness API for execution.
func (r *Runtime) handleHarnessToolCall(env *protocol.Envelope) {
	toolName := env.Payload.Tool
	toolUseID := env.Payload.ToolUseID

	r.connMu.Lock()
	meta, ok := r.sessionMeta[env.SessionID]
	r.connMu.Unlock()
	if !ok {
		log.Printf("[Runtime] No session meta for %s, cannot handle tool call", env.SessionID[:8])
		return
	}

	taskID := meta["task_id"]
	queueID := meta["queue_id"]
	profileID := meta["agent_profile_id"]

	if taskID == "" {
		log.Printf("[Runtime] No task_id in session meta, skipping tool call")
		return
	}

	log.Printf("[Runtime] Auto-task tool call: session=%s tool=%s task=%s", env.SessionID[:8], toolName, taskID[:8])

	// Serialize tool input for Harness API
	// Serialize tool input for Harness API as raw JSON.
	// Input from Payload may be a JSON string; convert to RawMessage
	// so it serializes as a raw JSON object, not a quoted string.
	var params json.RawMessage
	if inputStr, ok := env.Payload.Input.(string); ok {
		params = json.RawMessage(inputStr)
	} else {
		params, _ = json.Marshal(env.Payload.Input)
	}

	// Build request to Harness API
	body := map[string]interface{}{
		"task_id":          taskID,
		"queue_id":         queueID,
		"tool":             toolName,
		"params":           params,
		"agent_profile_id": profileID,
		"call_id":          toolUseID,
	}

	// Call server Harness API
	result := r.callHarnessAPI(body)

	// Send tool result back to claude
	if cli, ok := r.backends["claude"].(*backends.ClaudeCLIBackend); ok {
		cli.SendToolResult(env.SessionID, toolUseID, result)
		log.Printf("[Runtime] Tool result sent back to claude: tool=%s status=%s", toolName, result["status"])
	} else {
		log.Printf("[Runtime] No claude backend available for tool result")
	}
}

// callHarnessAPI sends a tool call request to the server's Harness HTTP API.
func (r *Runtime) callHarnessAPI(body map[string]interface{}) map[string]interface{} {
	baseURL := "http://" + r.ServerURL
	auth := fmt.Sprintf("node_id=%s&node_secret=%s", r.NodeID, r.Secret)

	bodyBytes, _ := json.Marshal(body)
	u := fmt.Sprintf("%s/api/node/tool-call?%s", baseURL, auth)

	resp, err := http.Post(u, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Printf("[Runtime] Harness API call failed: %v", err)
		return map[string]interface{}{"status": "error", "error": map[string]interface{}{"message": err.Error()}}
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[Runtime] Failed to decode Harness response: %v", err)
		return map[string]interface{}{"status": "error", "error": map[string]interface{}{"message": err.Error()}}
	}

	return result
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

// ── Agent Queue Poller ──────────────────────────────────────────────

type queueItem struct {
	ID             string `json:"id"`
	TaskID         string `json:"task_id"`
	AgentProfileID string `json:"agent_profile_id"`
	Status         string `json:"status"`
	TriggerType    string `json:"trigger_type"`
	AgentName      string `json:"agent_name"`
	CreatedAt      string `json:"created_at"`
}

type queueResponse struct {
	Queue []queueItem `json:"queue"`
}

// startQueuePoller begins polling the agent queue for unclaimed tasks.
func (r *Runtime) startQueuePoller() {
	if r.Secret == "" {
		log.Println("[Runtime] No node_secret available, queue poller disabled")
		return
	}

	go func() {
		// Wait a moment for the connection to stabilize
		time.Sleep(5 * time.Second)
		log.Println("[Runtime] Agent queue poller started (interval: 15s)")

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			r.pollQueue()
		}
	}()
}

func (r *Runtime) pollQueue() {
	baseURL := "http://" + r.ServerURL

	// GET /api/node/queue?node_id=...&node_secret=...
	u := fmt.Sprintf("%s/api/node/queue?node_id=%s&node_secret=%s", baseURL, r.NodeID, r.Secret)
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
	resp, err := client.Get(u)
	if err != nil {
		log.Printf("[Runtime] Queue poll failed: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("[Runtime] Queue poll unexpected status %d: %s", resp.StatusCode, string(body))
		return
	}

	var qr queueResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return
	}

	for _, item := range qr.Queue {
		log.Printf("[Runtime] Found queued task %s for agent %s", item.TaskID[:8], item.AgentName)
		r.processQueueItem(item)
	}
}

func (r *Runtime) processQueueItem(item queueItem) {
	baseURL := "http://" + r.ServerURL
	auth := fmt.Sprintf("node_id=%s&node_secret=%s", r.NodeID, r.Secret)

	// 1. Claim the item
	claimURL := fmt.Sprintf("%s/api/node/queue/%s/claim?%s", baseURL, item.ID, auth)
	resp, err := http.Post(claimURL, "application/json", nil)
	if err != nil {
		log.Printf("[Runtime] Claim failed: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[Runtime] Claim rejected (status %d)", resp.StatusCode)
		return
	}
	log.Printf("[Runtime] Claimed queue item %s", item.ID[:8])

	// 2. Set status to processing
	statusURL := fmt.Sprintf("%s/api/node/queue/%s/status?%s", baseURL, item.ID, auth)
	body := `{"status":"processing"}`
		req, _ := http.NewRequest("PUT", statusURL, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err = http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[Runtime] Status update failed: %v", err)
		return
	}
	resp.Body.Close()

	// 3. Get task details
	taskURL := fmt.Sprintf("%s/api/node/tasks/%s?%s", baseURL, item.TaskID, auth)
	resp, err = http.Get(taskURL)
	if err != nil {
		log.Printf("[Runtime] Get task failed: %v", err)
		return
	}
	taskBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var taskResp struct {
		Task struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"task"`
	}
	if err := json.Unmarshal(taskBody, &taskResp); err != nil {
		log.Printf("[Runtime] Task parse error: %v", err)
		return
	}
	log.Printf("[Runtime] Processing task: %s", taskResp.Task.Title)

	// 4. Create a session for this task
	sessionURL := fmt.Sprintf("%s/api/node/sessions?%s", baseURL, auth)
	sessionReq := map[string]string{
		"task_id":  item.TaskID,
		"agent_id": item.AgentProfileID,
		"queue_id": item.ID,
	}
	sessionBody, _ := json.Marshal(sessionReq)
	resp, err = http.Post(sessionURL, "application/json", bytes.NewBuffer(sessionBody))
	if err != nil {
		log.Printf("[Runtime] Session creation failed: %v", err)
		return
	}
	sessionRespBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusCreated {
		var sr struct {
			SessionID string `json:"session_id"`
		}
		json.Unmarshal(sessionRespBody, &sr)
		log.Printf("[Runtime] Session %s created for task '%s'", sr.SessionID[:8], taskResp.Task.Title)
	} else {
		log.Printf("[Runtime] Session creation returned %d", resp.StatusCode)
	}
}

// Backend handles messages for a specific AI agent.
type Backend interface {
	Name() string
	Version() string
	HandleMessage(env *protocol.Envelope) (*protocol.Envelope, error)
	Evaluate(prompt string) (string, error)
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
		cli.SetOnSessionComplete(func(sessionID, result, stopReason string, isError bool) {
			r.handleSessionComplete(sessionID, result, stopReason, isError)
		})
		// Configure MCP server path so .mcp.json is written for each session
		mcpServerPath := "mcp-server.exe"
		if exe, err := os.Executable(); err == nil {
			exePath := filepath.Join(filepath.Dir(exe), "mcp-server.exe")
			if _, err := os.Stat(exePath); err == nil {
				mcpServerPath = exePath
			}
		}
		cli.SetRuntimeConfig(r.ServerURL, r.NodeID, r.Secret, mcpServerPath)
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

// handleSessionComplete is called when a claude process finishes for an auto-task session.
// It updates the task queue status via the server HTTP API.
func (r *Runtime) handleSessionComplete(sessionID, result, stopReason string, isError bool) {
	r.connMu.Lock()
	meta, ok := r.sessionMeta[sessionID]
	delete(r.sessionMeta, sessionID)
	r.connMu.Unlock()

	if !ok || meta["is_auto_task"] != "true" {
		return
	}

	queueID := meta["queue_id"]
	if queueID == "" {
		log.Printf("[Runtime] No queue_id for session %s, skipping queue update", sessionID[:8])
		return
	}

	baseURL := "http://" + r.ServerURL
	auth := fmt.Sprintf("node_id=%s&node_secret=%s", r.NodeID, r.Secret)


	// Record task+agent completion to prevent duplicate sessions from rapid @mentions
	taskID := meta["task_id"]
	agentProfileID := meta["agent_profile_id"]
	if taskID != "" && agentProfileID != "" {
		r.connMu.Lock()
		r.recentlyCompleted[taskID+":"+agentProfileID] = time.Now()
		r.connMu.Unlock()
	}
	status := "completed"
	if isError {
		status = "failed"
	}

	body := map[string]string{
		"status":         status,
		"result_summary": result,
	}
	bodyBytes, _ := json.Marshal(body)

	u := fmt.Sprintf("%s/api/node/queue/%s/status?%s", baseURL, queueID, auth)
	req4, _ := http.NewRequest("PUT", u, bytes.NewBuffer(bodyBytes))
	req4.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req4)
	if err != nil {
		log.Printf("[Runtime] Queue update failed: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		log.Printf("[Runtime] Queue %s updated to %s", queueID[:8], status)
	} else {
		log.Printf("[Runtime] Queue update returned %d", resp.StatusCode)
	}
}

// findActiveSession looks for an active (non-completed) Claude session for a given task and agent.
// Returns the session ID, or empty string if none found.
func (r *Runtime) findActiveSession(taskID, agentProfileID string) string {
	r.connMu.Lock()
	defer r.connMu.Unlock()

	cli, ok := r.backends["claude"].(*backends.ClaudeCLIBackend)
	if !ok {
		return ""
	}

	for sessionID, meta := range r.sessionMeta {
		if meta["task_id"] == taskID && meta["agent_profile_id"] == agentProfileID {
			if cli.HasSession(sessionID) {
				return sessionID
			}
		}
	}
	return ""
}

// handleAgentMention processes an @mention event from the server.
// It evaluates whether the agent should work on the task or just reply.
func (r *Runtime) handleAgentMention(env *protocol.Envelope) {
	if env.Payload == nil || env.Payload.Metadata == nil {
		return
	}
	meta := env.Payload.Metadata

	taskID, _ := meta["task_id"].(string)
	queueID, _ := meta["queue_id"].(string)
	commentContent, _ := meta["comment_content"].(string)
	taskTitle, _ := meta["task_title"].(string)
	agentProfileID, _ := meta["agent_profile_id"].(string)
	systemPrompt, _ := meta["system_prompt"].(string)
	instructions, _ := meta["instructions"].(string)

	if taskID == "" || queueID == "" {
		log.Printf("[Runtime] Incomplete mention event: missing task_id or queue_id")
		return
	}

	log.Printf("[Runtime] Agent mentioned in task %s (queue: %s)", taskID[:8], queueID[:8])

	// Get task details for evaluation context
	baseURL := "http://" + r.ServerURL
	auth := fmt.Sprintf("node_id=%s&node_secret=%s", r.NodeID, r.Secret)

	taskDesc := ""
	taskURL := fmt.Sprintf("%s/api/node/tasks/%s?%s", baseURL, taskID, auth)
	resp, err := http.Get(taskURL)
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var taskResp struct {
			Task struct {
				Description string `json:"description"`
			} `json:"task"`
		}
		json.Unmarshal(body, &taskResp)
		taskDesc = taskResp.Task.Description
	}

	// Build evaluation prompt with agent personality
	evalPrompt := fmt.Sprintf(`You have been @mentioned in a comment on the task "%s".

System Prompt (your role):
%s

Behavior Instructions (your communication style):
%s

Task Description: %s

Comment: %s

Respond with exactly one of these two formats:
- WORK: <brief reason> — if this task needs your work
- REPLY: <your response> — if a simple reply is sufficient
  (IMPORTANT: When REPLY, follow the System Prompt and Behavior Instructions above.)`, taskTitle, systemPrompt, instructions, taskDesc, commentContent)

	// Use the first available backend to evaluate
	var evalResult string
	for _, backend := range r.backends {
		evalResult, err = backend.Evaluate(evalPrompt)
		if err != nil {
			log.Printf("[Runtime] Evaluation error: %v", err)
			return
		}
		break
	}

	evalResult = strings.TrimSpace(evalResult)
	log.Printf("[Runtime] Evaluation result: %s", truncateStr(evalResult, 200))

	switch {
	case strings.HasPrefix(evalResult, "REPLY:"):
		reply := strings.TrimSpace(strings.TrimPrefix(evalResult, "REPLY:"))
		if reply == "" {
			reply = "Acknowledged."
		}
		// updateQueueStatus passes result_summary → MarkCompleted → handleInProgressToCompleted
		// which posts the comment. Do NOT also call postAgentComment — that causes duplicates.
		r.updateQueueStatus(queueID, "completed", reply)

	case strings.HasPrefix(evalResult, "WORK:"):
		// Check if there's already an active session for this task+agent
		existingSessionID := r.findActiveSession(taskID, agentProfileID)

		if existingSessionID != "" {
			// An active session already exists — likely the queue poller
			// created it while handleAgentMention was evaluating (race).
			// The agent will discover the @mention through get_task_detail
			// during normal processing. Do NOT inject (avoids double reply)
			// and do NOT touch the queue status (session lifecycle owns it).
			log.Printf("[Runtime] Session %s already active for task %s — skipping (agent will discover via get_task_detail)", existingSessionID[:8], taskID[:8])
			return
		}

		// Brief dedup window (15s) only to prevent race between
		// handleAgentMention and queue poller creating duplicate sessions.
		// Persistent workspaces + --resume make continuation sessions safe.
		recentKey := taskID + ":" + agentProfileID
		r.connMu.Lock()
		if completedAt, exists := r.recentlyCompleted[recentKey]; exists {
			if time.Since(completedAt) < 15*time.Second {
				r.connMu.Unlock()
				log.Printf("[Runtime] Task %s session just completed — skipping to avoid race", taskID[:8])
				return
			}
			delete(r.recentlyCompleted, recentKey)
		}
		r.connMu.Unlock()

		// No active session — create a new one (original flow)
		r.updateQueueStatus(queueID, "processing", "")

		sessionURL := fmt.Sprintf("%s/api/node/sessions?%s", baseURL, auth)
		sessionReq := map[string]string{
			"task_id":  taskID,
			"agent_id": agentProfileID,
			"queue_id": queueID,
		}
		sessionBody, _ := json.Marshal(sessionReq)
		resp, err := http.Post(sessionURL, "application/json", bytes.NewBuffer(sessionBody))
		if err != nil {
			log.Printf("[Runtime] Session creation failed: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusCreated {
			var sr struct {
				SessionID string `json:"session_id"`
			}
			json.NewDecoder(resp.Body).Decode(&sr)
			log.Printf("[Runtime] Session %s created for mentioned task %s", sr.SessionID[:8], taskID[:8])
		} else {
			log.Printf("[Runtime] Session creation returned %d", resp.StatusCode)
		}
	default:
		log.Printf("[Runtime] Unrecognized evaluation result, using as reply")
		reply := evalResult
		if len([]rune(reply)) > 2000 {
			reply = string([]rune(reply)[:2000]) + "\n\n...（过长已截断）"
		}
		// updateQueueStatus passes result_summary → MarkCompleted → handleInProgressToCompleted
		// which posts the comment. Do NOT also call postAgentComment — that causes duplicates.
		r.updateQueueStatus(queueID, "completed", reply)
	}
}

// postAgentComment posts a comment on a task on behalf of an agent profile.
func (r *Runtime) postAgentComment(taskID, agentProfileID, queueID, content string) {
	baseURL := "http://" + r.ServerURL
	auth := fmt.Sprintf("node_id=%s&node_secret=%s", r.NodeID, r.Secret)

	body := map[string]string{
		"content":          content,
		"agent_profile_id": agentProfileID,
		"queue_id":         queueID,
	}
	bodyBytes, _ := json.Marshal(body)

	u := fmt.Sprintf("%s/api/node/tasks/%s/comments?%s", baseURL, taskID, auth)
	resp, err := http.Post(u, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Printf("[Runtime] Post comment failed: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("[Runtime] Posted agent comment on task %s (status: %d)", taskID[:8], resp.StatusCode)
}

// updateQueueStatus updates a queue item's status on the server.
func (r *Runtime) updateQueueStatus(queueID, status, resultSummary string) {
	baseURL := "http://" + r.ServerURL
	auth := fmt.Sprintf("node_id=%s&node_secret=%s", r.NodeID, r.Secret)

	body := map[string]string{
		"status": status,
	}
	if resultSummary != "" {
		body["result_summary"] = resultSummary
	}
	bodyBytes, _ := json.Marshal(body)

	u := fmt.Sprintf("%s/api/node/queue/%s/status?%s", baseURL, queueID, auth)
	req5, _ := http.NewRequest("PUT", u, bytes.NewBuffer(bodyBytes))
	req5.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req5)
	if err != nil {
		log.Printf("[Runtime] Queue status update failed: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("[Runtime] Queue %s → %s (status: %d)", queueID[:8], status, resp.StatusCode)
}

// truncateStr truncates a string for logging.
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
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
