package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/google/uuid"

	"github.com/coaether/server/middleware"
	"github.com/coaether/server/models"

	"github.com/coaether/server/protocol"
)

const (
	tokenDuration = 15 * time.Minute

	binaryDir = "bin/agents"
)

type NodeHandler struct {
	DB *sql.DB

	Bus *protocol.MessageBus

	Hub *DashboardHub
}

func NewNodeHandler(db *sql.DB, bus *protocol.MessageBus) *NodeHandler {

	return &NodeHandler{DB: db, Bus: bus}

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

		ID: nodeID,

		UserID: userID.(string),

		Name: req.Name,

		OS: req.OS,

		Arch: req.Arch,

		Status: models.NodeStatusOnline,

		Version: req.Version,

		IP: c.ClientIP(),

		LastSeen: time.Now(),

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

	c.JSON(http.StatusOK, models.NodeRegisterResp{

		NodeID: nodeID,
	})

}

func (h *NodeHandler) List(c *gin.Context) {

	userID, _ := c.Get("user_id")
	wsID, _ := c.Get("validated_workspace_id")
	wsIDStr, _ := wsID.(string)

	var rows *sql.Rows
	var err error
	if wsIDStr != "" {
		rows, err = h.DB.Query(
			`SELECT n.id, n.user_id, n.name, n.os, n.arch, n.status, n.version, n.ip, n.max_sessions, n.last_seen, n.created_at
			 FROM nodes n
			 JOIN workspace_members wm ON wm.user_id = n.user_id
			 WHERE wm.workspace_id = $1
			 ORDER BY n.last_seen DESC`, wsIDStr,
		)
	} else {
		rows, err = h.DB.Query(
			`SELECT id, user_id, name, os, arch, status, version, ip, max_sessions, last_seen, created_at
			 FROM nodes WHERE user_id = $1 ORDER BY last_seen DESC`, userID,
		)
	}

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

		nodes = append(nodes, n)

	}

	if nodes == nil {

		nodes = []models.Node{}

	}

	// Determine which nodes can be managed (online bus nodes can be stopped)
	for i := range nodes {
		if h.Bus != nil && h.Bus.GetEndpoint("runtime://"+nodes[i].ID) != nil {
			nodes[i].CanManage = true
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

	// For UUID-based runtime nodes, return runtime capabilities
	var epID string
	if h.Bus != nil && h.Bus.GetEndpoint("runtime://"+nodeID) != nil {
		epID = "runtime://" + nodeID
	}
	if epID != "" && h.Bus != nil {

		ep := h.Bus.GetEndpoint(epID)

		if ep != nil {

			agents := make([]models.Agent, 0, len(ep.Capabilities))

			for _, cap := range ep.Capabilities {

				agents = append(agents, models.Agent{

					ID: epID + "/" + cap.ID,

					NodeID: nodeID,

					Name: cap.Name,

					Command: cap.ID,

					Version: cap.Version,

					Enabled: true,

					AutoDetected: true,

					CreatedAt: time.Now(),

					UpdatedAt: time.Now(),
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

// ==================== Scheme B: Token-based Node Registration ====================

func generateTokenHex() string {

	b := make([]byte, 32)

	rand.Read(b)

	return "TOKEN_" + hex.EncodeToString(b)

}

// getLocalIP returns the first non-loopback IPv4 address of this host.
func getServerIP() string {
	return "192.168.0.104"
}

// GenerateToken creates a one-time join token for a remote node.

func (h *NodeHandler) GenerateToken(c *gin.Context) {

	var req models.GenerateTokenReq

	if err := c.ShouldBindJSON(&req); err != nil {

		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})

		return

	}

	userID, _ := c.Get("user_id")

	workspaceID, _ := c.Get("validated_workspace_id")

	token := generateTokenHex()

	expiresAt := time.Now().Add(tokenDuration)

	wsID, _ := workspaceID.(string)
	var err error
	if wsID == "" {

		_, err = h.DB.Exec(

			`INSERT INTO node_join_tokens (token, user_id, node_name, status, expires_at, created_at)

			 VALUES ($1, $2, $3, 'pending', $4, NOW())`,

			token, userID, req.NodeName, expiresAt,
		)

	} else {

		_, err = h.DB.Exec(

			`INSERT INTO node_join_tokens (token, user_id, workspace_id, node_name, status, expires_at, created_at)

			 VALUES ($1, $2, $3, $4, 'pending', $5, NOW())`,

			token, userID, wsID, req.NodeName, expiresAt,
		)

	}

	if err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create token"})

		return

	}

	// Build the install command with the server's LAN IP
	// so remote machines can reach it.
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	ip := getServerIP()
	_, port, _ := net.SplitHostPort(c.Request.Host)
	if port == "" {
		port = "8088"
	}
	serverAddr := net.JoinHostPort(ip, port)
	command := fmt.Sprintf("curl -s '%s://%s/api/nodes/install.sh?token=%s' | bash",
		scheme, serverAddr, token)
	commandPS1 := fmt.Sprintf("powershell -c \"iex ((Invoke-WebRequest -Uri '%s://%s/api/nodes/install.ps1?token=%s').Content)\"",
		scheme, serverAddr, token)

	c.JSON(http.StatusOK, models.GenerateTokenResp{

		Token: token,

		ExpiresAt: expiresAt,

		Command: command,

		CommandPS1: commandPS1,
	})

}

// InstallScript returns a shell script that installs and starts the agent runtime on a remote machine.

func (h *NodeHandler) InstallScript(c *gin.Context) {

	token := c.Query("token")

	if token == "" {

		c.String(http.StatusBadRequest, "echo 'Missing token parameter'")

		return

	}

	// Validate token

	var status string

	err := h.DB.QueryRow(

		`SELECT status FROM node_join_tokens WHERE token = $1`, token,
	).Scan(&status)

	if err == sql.ErrNoRows {

		c.String(http.StatusNotFound, "echo 'Invalid token'")

		return

	}

	if err != nil {

		c.String(http.StatusInternalServerError, "echo 'Server error'")

		return

	}

	if status != "pending" {

		c.String(http.StatusGone, "echo 'Token already used or expired'")

		return

	}

	ip := getServerIP()
	_, port, _ := net.SplitHostPort(c.Request.Host)
	if port == "" {
		port = "8088"
	}
	serverAddr := net.JoinHostPort(ip, port)
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	script := fmt.Sprintf(`#!/bin/bash

set -e

TOKEN="%s"
SERVER_URL="%s"
SERVER_BASE="%s://%s"

# Check if already installed
if [ -f "$HOME/.coaether/env" ]; then
    echo "This machine already has a CoAether agent node configured."
    echo "To reinstall, remove ~/.coaether/ first."
    exit 1
fi

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    arm64)  ARCH="arm64" ;;
    *)      echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "Downloading agent-runtime for ${OS}/${ARCH}..."
mkdir -p "$HOME/.coaether"
curl -sL --connect-timeout 5 --max-time 30 "${SERVER_BASE}/api/nodes/bin/${OS}/${ARCH}" -o "$HOME/.coaether/agent-runtime" || { echo "Failed to download agent-runtime"; exit 1; }
chmod +x "$HOME/.coaether/agent-runtime"

# Install Claude Code CLI if not already installed
if ! command -v claude &>/dev/null; then
    echo "Installing Claude Code CLI via npm..."
    if command -v npm &>/dev/null; then
        npm install -g @anthropic-ai/claude-code 2>/dev/null || {
            echo "Warning: npm install failed. Trying npx..."
            npx @anthropic-ai/claude-code --install 2>/dev/null || {
                echo ""
                echo "WARNING: Could not install Claude Code CLI automatically."
                echo "To use Claude Code features, install it manually after setup:"
                echo "  npm install -g @anthropic-ai/claude-code"
                echo "Or set ANTHROPIC_API_KEY in ~/.coaether/env for the API backend."
            }
        }
    elif command -v npx &>/dev/null; then
        npx @anthropic-ai/claude-code --install 2>/dev/null || {
            echo ""
            echo "WARNING: Could not install Claude Code CLI automatically."
            echo "Install it manually: npm install -g @anthropic-ai/claude-code"
        }
    else
        echo ""
        echo "WARNING: npm not found. Install Node.js first, then run:"
        echo "  npm install -g @anthropic-ai/claude-code"
        echo "Or set ANTHROPIC_API_KEY in ~/.coaether/env for the API backend."
    fi
else
    echo "Claude Code CLI already installed."
fi

# Save config
cat > "$HOME/.coaether/env" <<CONFEOF
SERVER_URL=${SERVER_URL}
NODE_TOKEN=${TOKEN}
NODE_SECRET=
NODE_ID=
# Optional: set ANTHROPIC_API_KEY if you don't have Claude Code CLI installed
# ANTHROPIC_API_KEY=your_key_here
CONFEOF

# Detect npm global bin path for launchd PATH
NPM_BIN=""
if command -v npm &>/dev/null; then
    NPM_BIN=$(npm bin -g 2>/dev/null || echo "")
fi
# Common npm global bin locations on macOS
export PATH="/usr/local/bin:/opt/homebrew/bin:${HOME}/.npm-global/bin:${NPM_BIN}:${PATH}"

# Install as macOS LaunchAgent (persists across terminal closes and reboots)
PLIST_PATH="$HOME/Library/LaunchAgents/com.coaether.agent.plist"
mkdir -p "$HOME/Library/LaunchAgents"

cat > "$PLIST_PATH" <<PLISTEOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.coaether.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>${HOME}/.coaether/agent-runtime</string>
    </array>
    <key>WorkingDirectory</key>
    <string>${HOME}/.coaether</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>SERVER_URL</key>
        <string>${SERVER_URL}</string>
        <key>NODE_TOKEN</key>
        <string>${TOKEN}</string>
        <key>NODE_SECRET</key>
        <string></string>
        <key>NODE_ID</key>
        <string></string>
        <key>PATH</key>
        <string>/usr/local/bin:/opt/homebrew/bin:${HOME}/.npm-global/bin:${NPM_BIN}:/usr/bin:/bin</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${HOME}/.coaether/agent.log</string>
    <key>StandardErrorPath</key>
    <string>${HOME}/.coaether/agent.err</string>
</dict>
</plist>
PLISTEOF

# Unload any existing instance, then load and start
launchctl unload "$PLIST_PATH" 2>/dev/null || true
launchctl load "$PLIST_PATH"

echo "CoAether agent installed and started as a background service."
echo "Check status: launchctl list com.coaether.agent"
echo "View logs: tail -f $HOME/.coaether/agent.log"

`, token, serverAddr, scheme, serverAddr)

	c.Header("Content-Type", "text/x-shellscript")

	c.String(http.StatusOK, script)

}

// InstallScriptPS1 returns a PowerShell script that installs and starts the
// agent runtime on a remote Windows machine.
func (h *NodeHandler) InstallScriptPS1(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.String(http.StatusBadRequest, `Write-Host "Missing token parameter"`)
		return
	}

	// Validate token (same logic as InstallScript)
	var status string
	err := h.DB.QueryRow(
		`SELECT status FROM node_join_tokens WHERE token = $1`, token,
	).Scan(&status)
	if err == sql.ErrNoRows {
		c.String(http.StatusNotFound, `Write-Host "Invalid token"`)
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, `Write-Host "Server error"`)
		return
	}
	if status != "pending" {
		c.String(http.StatusGone, `Write-Host "Token already used or expired"`)
		return
	}

	ip := getServerIP()
	_, port, _ := net.SplitHostPort(c.Request.Host)
	if port == "" {
		port = "8088"
	}
	serverAddr := net.JoinHostPort(ip, port)
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	script := fmt.Sprintf(`$TOKEN="%s"
$SERVER_URL="%s"
$SERVER_BASE="%s://%s"

$ARCH="amd64"
$DIR="$env:USERPROFILE\\.coaether"

# Check if already installed
if (Test-Path "$DIR\env") {
    Write-Host "This machine already has a CoAether agent node configured."
    Write-Host "To reinstall, remove $DIR first."
    exit 1
}

# Create directory
New-Item -ItemType Directory -Force -Path $DIR | Out-Null

# Download binary
Write-Host "Downloading agent-runtime for windows/${ARCH}..."
try {
    Invoke-WebRequest -Uri "${SERVER_BASE}/api/nodes/bin/windows/${ARCH}" -OutFile "$DIR\\agent-runtime.exe" -TimeoutSec 30 -ErrorAction Stop
} catch {
    Write-Host "Failed to download agent-runtime: $_"
    exit 1
}

# Install Claude Code CLI if not already installed
$claude = Get-Command claude -ErrorAction SilentlyContinue
if (-not $claude) {
    Write-Host "Installing Claude Code CLI via npm..."
    $npm = Get-Command npm -ErrorAction SilentlyContinue
    if ($npm) {
        & npm install -g @anthropic-ai/claude-code 2>$null
        if ($LASTEXITCODE -ne 0) {
            Write-Host "WARNING: npm install failed. Trying npx..."
            & npx @anthropic-ai/claude-code --install 2>$null
        }
    } else {
        Write-Host "WARNING: npm not found. Install Node.js first, then run:"
        Write-Host "  npm install -g @anthropic-ai/claude-code"
    }
} else {
    Write-Host "Claude Code CLI already installed."
}

# Save config
@"
SERVER_URL=${SERVER_URL}
NODE_TOKEN=${TOKEN}
NODE_SECRET=
NODE_ID=
"@ | Out-File -FilePath "$DIR\env" -Encoding ascii

# Create startup shortcut (CurrentUser Startup folder)
$STARTUP = [Environment]::GetFolderPath("Startup")
$VBS_PATH = "$STARTUP\coaether-agent.vbs"
$VBS = @'
Set WshShell = CreateObject("WScript.Shell")
WshShell.Run chr(34) & "'" + "$DIR\\agent-runtime.exe" + "'" & chr(34), 0, False
'@
$VBS | Out-File -FilePath $VBS_PATH -Encoding ascii

# Start the agent
Write-Host "Starting agent-runtime..."
Start-Process -WindowStyle Hidden -FilePath "$DIR\\agent-runtime.exe"

Write-Host ""
Write-Host "CoAether agent installed and started as a background process."
Write-Host "It will auto-start on login (via Startup folder)."
Write-Host "View process: Get-Process agent-runtime"
`, token, serverAddr, scheme, serverAddr)

	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, script)
}

// DownloadBinary serves pre-compiled agent-runtime binaries for remote platforms.

func (h *NodeHandler) DownloadBinary(c *gin.Context) {

	osName := c.Param("os")

	arch := c.Param("arch")

	if arch != "amd64" && arch != "arm64" {

		c.String(http.StatusBadRequest, "unsupported arch: "+arch)

		return

	}

	// Try multiple possible locations for the binary

	paths := []string{

		filepath.Join(binaryDir, osName+"-"+arch, "agent-runtime"),

		filepath.Join(binaryDir, osName+"-"+arch, "agent-runtime.exe"),

		filepath.Join("..", binaryDir, osName+"-"+arch, "agent-runtime"),

		filepath.Join("..", binaryDir, osName+"-"+arch, "agent-runtime.exe"),
	}

	var foundPath string

	for _, p := range paths {

		if _, err := os.Stat(p); err == nil {

			foundPath = p

			break

		}

	}

	if foundPath == "" {

		c.String(http.StatusNotFound, "binary not found for "+osName+"/"+arch)

		return

	}

	c.File(foundPath)

}

// StartNode generates a command for the user to run agent-runtime on the target machine.
func (h *NodeHandler) StartNode(c *gin.Context) {
	nodeID := c.Param("id")

	// Generate a fresh node_secret for this node
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate secret"})
		return
	}
	nodeSecret := hex.EncodeToString(secretBytes)
	secretHash := sha256.Sum256([]byte(nodeSecret))
	secretHashHex := hex.EncodeToString(secretHash[:])

	if _, err := h.DB.Exec(
		`UPDATE nodes SET node_secret_hash = $1 WHERE id = $2`,
		secretHashHex, nodeID,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save secret"})
		return
	}

	// Determine server address for the runtime to connect to
	ip := getServerIP()
	_, port, _ := net.SplitHostPort(c.Request.Host)
	if port == "" {
		port = "8088"
	}
	serverAddr := net.JoinHostPort(ip, port)

	// Build command for user to run on the target machine
	command := fmt.Sprintf("nohup ~/.coaether/agent-runtime start --server %s --secret %s --node-id %s > ~/.coaether/agent.log 2>&1 &",
		serverAddr, nodeSecret, nodeID)
	commandPS1 := fmt.Sprintf("Start-Process -WindowStyle Hidden -FilePath \"$env:USERPROFILE\\.coaether\\agent-runtime.exe\" -ArgumentList \"start --server %s --secret %s --node-id %s\"",
		serverAddr, nodeSecret, nodeID)

	c.JSON(http.StatusOK, gin.H{
		"status":      "command_generated",
		"command":     command,
		"command_ps1": commandPS1,
		"server":      serverAddr,
		"node_id":     nodeID,
	})
}

// StopNode stops the agent-runtime on the local machine.
func (h *NodeHandler) StopNode(c *gin.Context) {
	nodeID := c.Param("id")

	// Server-side enforcement: disable all agent profiles for this node
	if _, err := h.DB.Exec(
		`UPDATE agent_profiles SET enabled = false, current_load = 0 WHERE node_id = $1`,
		nodeID,
	); err != nil {
		log.Printf("[StopNode] Failed to disable profiles for node %s: %v", nodeID[:8], err)
	}

	// Fail all active queue items for agents on this node
	if _, err := h.DB.Exec(
		`UPDATE task_agent_queue SET status = 'failed', result_summary = '节点已停止', completed_at = NOW()
		 WHERE agent_profile_id IN (SELECT id FROM agent_profiles WHERE node_id = $1)
		 AND status IN ('queued', 'claimed', 'processing')`,
		nodeID,
	); err != nil {
		log.Printf("[StopNode] Failed to fail queue items for node %s: %v", nodeID[:8], err)
	}

	// Signal changes via websocket
	if h.Hub != nil {
		h.Hub.SignalChange("agent_profiles")
		h.Hub.SignalChange("task_agent_queue")
	}

	// Stop via Message Bus (works for all connected runtime nodes)
	if h.Bus == nil {
		c.JSON(http.StatusOK, gin.H{"status": "stopped"})
		return
	}

	env := protocol.NewEnvelope("system://bus", "runtime://"+nodeID, protocol.MsgNodeStop, nil)
	delivered := h.Bus.Deliver(env)
	if delivered > 0 {
		log.Printf("[StopNode] Sent stop command via bus to %s", nodeID)
	}

	c.JSON(http.StatusOK, gin.H{"status": "stopped"})
}

func (h *NodeHandler) canControlNode(c *gin.Context, nodeID string) (bool, string) {
	userID, _ := c.Get("user_id")

	// Case 1: Node owner → always allowed
	var ownerID string
	if err := h.DB.QueryRow(`SELECT user_id FROM nodes WHERE id = $1`, nodeID).Scan(&ownerID); err != nil {
		return false, "node not found"
	}
	if sUserID, ok := userID.(string); ok && sUserID == ownerID {
		return true, ""
	}

	// Case 2: Workspace admin (when workspace context exists)
	wsID, _ := c.Get("validated_workspace_id")
	if wsIDStr, ok := wsID.(string); ok && wsIDStr != "" {
		role, _ := c.Get("workspace_role")
		if roleStr, ok := role.(string); ok {
			if middleware.RoleAtLeastByRole(roleStr, "admin") {
				return true, ""
			}
		}
		return false, "insufficient permissions: only admins and node owners can control nodes"
	}

	return false, "insufficient permissions: only node owners and workspace admins can control nodes"
}

// RemoveNode deletes a registered node and disconnects it from the bus.

func (h *NodeHandler) RemoveNode(c *gin.Context) {
	nodeID := c.Param("id")

	// Permission check: node owner or workspace admin
	if ok, msg := h.canControlNode(c, nodeID); !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": msg})
		return
	}

	// Disconnect from bus if connected
	if h.Bus != nil {
		if ep := h.Bus.GetEndpoint("runtime://" + nodeID); ep != nil {
			h.Bus.Unregister("runtime://" + nodeID)
		}
	}

	// Delete from DB (cascades to agents)

	_, err := h.DB.Exec(`DELETE FROM nodes WHERE id = $1`, nodeID)

	if err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete node"})

		return

	}

	c.JSON(http.StatusOK, gin.H{"status": "removed"})

}

// ValidateJoinToken checks if a token is valid and marks it as used.

// Returns user_id, node_name, and an error if invalid.

func (h *NodeHandler) ValidateJoinToken(token string) (string, string, error) {

	var userID, nodeName string

	var expiresAt time.Time

	err := h.DB.QueryRow(

		`SELECT user_id, node_name, expires_at FROM node_join_tokens

		 WHERE token = $1 AND status = 'pending'`,

		token,
	).Scan(&userID, &nodeName, &expiresAt)

	if err != nil {

		return "", "", err

	}

	if time.Now().After(expiresAt) {

		h.DB.Exec(`UPDATE node_join_tokens SET status = 'expired' WHERE token = $1`, token)

		return "", "", fmt.Errorf("token expired")

	}

	return userID, nodeName, nil

}

// UseJoinToken marks a token as used and returns the node info.

func (h *NodeHandler) UseJoinToken(token string) error {

	_, err := h.DB.Exec(

		`UPDATE node_join_tokens SET status = 'used', used_at = NOW() WHERE token = $1`,

		token,
	)

	return err

}
