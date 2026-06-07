# CoAether — AI Agent Distributed Orchestration Platform

Cross-platform AI Agent distributed orchestration platform, connecting AI Runtimes with the Web frontend through the **Message Bus** protocol, providing multi-user workspaces, task/project management, real-time chat, Agent configuration, and more.

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Web UI (React)                      │
│  ┌────────────┐  ┌──────────────┐  ┌─────────────────┐  │
│  │  Dashboard  │  │  Chat Panel  │  │  Notification   │  │
│  │  (Tasks/Projects)│  (Floating Chat) │  (Bell/Toast)      │  │
│  └──────┬──────┘  └──────┬───────┘  └────────┬────────┘  │
│         │                │                    │           │
│  ┌──────┴────────────────┴────────────────────┴────────┐  │
│  │              WebSocket Client Layer                   │  │
│  │  Dashboard WS (/ws/dashboard)  +  Bus WS (/ws/bus)   │  │
│  └────────────────────────┬────────────────────────────┘  │
└───────────────────────────┼────────────────────────────────┘
                            │
                   HTTP REST + WebSocket
                            │
┌───────────────────────────┼────────────────────────────────┐
│                    Server (Go + Gin)                        │
│  ┌─────────────┐  ┌──────┴────────┐  ┌──────────────────┐  │
│  │ DashboardHub │  │  Message Bus  │  │  REST API        │  │
│  │ (Notifications/ │  (Message      │  │  (CRUD/Auth)      │  │
│  │  Signals)    │  │  Routing)     │  │                  │  │
│  └─────────────┘  └──────┬────────┘  └──────────────────┘  │
│                          │                                  │
│                    ┌─────┴──────┐                           │
│                    │ PostgreSQL  │                           │
│                    └────────────┘                           │
└───────────────────────────┬────────────────────────────────┘
                            │
                   Message Bus (WebSocket)
                            │
              ┌─────────────┼─────────────┐
              │             │             │
     ┌────────┴───┐  ┌─────┴──────┐  ┌───┴────────┐
     │ Agent      │  │ Agent      │  │ Agent      │
     │ Runtime    │  │ Runtime    │  │ Runtime    │
     │ (API mode) │  │ (CLI mode) │  │ (Remote)   │
     └────────────┘  └────────────┘  └────────────┘
```

### Core Subsystems

| Subsystem | Role | Tech Stack |
|-----------|------|------------|
| **server/** | HTTP + WebSocket server, authentication, CRUD, message routing | Go + Gin + gorilla/websocket + PostgreSQL |
| **webui/** | React frontend SPA, Dashboard + Floating Chat | React 18 + TypeScript + Vite |
| **agent-runtime/** | AI Agent runtime, connects to the platform via Message Bus | Go, supports Claude CLI / API backends |

### Communication Architecture

The system uses a **dual WebSocket channel** architecture:

1. **Dashboard WebSocket** (`/ws/dashboard`) — Used for UI real-time updates (task/project change notifications, workspace signals, Toast popups). Authenticated with JWT token, connects to `DashboardHub`.
2. **Message Bus WebSocket** (`/ws/bus`) — Used for AI Agent message routing. Identifies frontend connections with the `type=ui` parameter, connects to `MessageBus`, does not rely on JWT (registers via `hello` message after connecting).

---

## Features

### Multi-user Workspace
- Role-based permission system: `owner` > `admin` > `worker` > `observer`
- Supports workspace switching (sidebar dropdown selector)
- Auto-creates default workspace for users without one
- Workspace-level resource isolation: tasks, projects, Agent configurations, and sessions are all bound to workspaces

### Role Permission Matrix

| Action | owner | admin | worker | observer |
|--------|-------|-------|--------|----------|
| View workspace content | ✅ | ✅ | ✅ | ✅ |
| Create/Edit tasks | ✅ | ✅ | ✅ | ❌ |
| Manage projects | ✅ | ✅ | ✅ | ❌ |
| Configure Agent | ✅ | ✅ | ❌ | ❌ |
| Manage workspace members | ✅ | ✅ | ❌ | ❌ |
| Delete workspace | ✅ | ❌ | ❌ | ❌ |
| Modify roles | ✅ | ❌ | ❌ | ❌ |

### AI Agent Chat
- Floating chat window (draggable), supports multi-session management
- Multi-Agent selection: users can configure multiple Agent Profiles, switching Agents automatically restores the corresponding session
- Session persistence: active sessions are automatically restored after page refresh
- Session isolation between Agents: sessions for different Agents are stored and restored independently
- Rich text message rendering: code blocks, tables, Markdown, images, progress indicators
- File/Image upload: supports paste or drag-and-drop upload
- Tool call permission control: auto mode (auto-approve) and restricted mode (manual confirmation)

### Agent Configuration System
- Custom Agent Profile: name, avatar, description, associated runtime, model selection
- Supports CLI and API backend modes
- Runtime auto-discovery and registration
- Supports workspace-scoped configuration

### Task Management
- **Kanban Board** — Supports status transitions: `todo` → `in_progress` → `blocked` → `review` → `done`
- **Task Detail** (GitHub Issue style) — Left sidebar shows editable title, read-only description, subtask list, and comments section; right sidebar editable fields for status, priority, assignee, delegated assignees, tags, due date, project, parent task
- **Three-level responsibility system** — Creator (immutable) → Assignee (changeable) → Delegated Assignees (appendable)
- **Subtasks** — Linked via `parent_id`, displayed as a list in the detail page
- **Tags** — Freely add/remove, supports filtering by tag
- **Priority** — `urgent` > `high` > `medium` > `low`
- **Task Comments** — Issue-style comments, postable by both users and agents, supports deletion
- Linked to projects, organize tasks by project
- Trash mechanism: soft delete + restore + permanent delete
- Isolated by workspace

### Project Management
- Supports color labels, description
- Linked task count
- Status transitions: `planning` → `active` → `completed` / `on_hold`
- Supports assignee (polymorphic: user or agent)
- Supports start/due dates
- Trash mechanism (soft delete/restore/permanent delete)
- Isolated by workspace

### Trash
- Both tasks and projects support soft delete
- Separate trash views (`/tasks/trash`, `/projects/trash`)
- Supports restore and permanent delete

### Workspace Invitation
- Email invitation: generates a unique token link
- Invitation management: list, cancel pending invitations
- In-app notification: invited users see a bell dot indicator after login
- Instant notification: WebSocket real-time push of invitation notifications
- Supports accept/decline invitation
- Auto-mark expired invitations

### WebSocket Real-time Push
- Instant notification on workspace deletion/member removal/role change
- Toast popup (auto-dismiss, 5 seconds)
- Dashboard data auto-refresh (`useResourceSync` hook)
- Real-time invitation change sync

### Remote Node Management
- **Token-based node registration** (sole method): generate a token through the Web UI, run the install script on the target machine to register
- Cross-platform support: **macOS** (bash install script + LaunchAgent auto-start) and **Windows** (PowerShell install script + Startup folder auto-start)
- Node card management: status indicator (online/offline/busy), scan Agents, start/stop Agents, **delete node**
- Platform selection UI: Add Node dialog supports macOS/Windows tabs, auto-displays corresponding install commands
- Cross-platform binary distribution: automatically downloads the agent-runtime for the corresponding OS/Arch
- Node join token mechanism: 15-minute validity, prevents unauthorized registration, auto-marked after use
- Node list status synced in real-time via WebSocket

### Multi-language
- Chinese / English bilingual interface
- Switch via `useLang()` hook

### User Management
- Admins can view all users list
- Supports user deletion
- JWT authentication (Access Token)

---

## Tech Stack

### Backend
- **Language**: Go 1.21+
- **Web Framework**: Gin
- **WebSocket**: gorilla/websocket (DashboardHub + MessageBus dual channel)
- **Database**: PostgreSQL (database/sql + lib/pq)
- **Authentication**: JWT (golang-jwt v5)
- **Email**: net/smtp-based email sending

### Frontend
- **Framework**: React 18 + TypeScript
- **Build Tool**: Vite
- **Communication**: REST API + WebSocket (Dashboard signals + Message Bus messages)
- **State Management**: React Hooks (useState/useEffect/useCallback/useRef)
- **Internationalization**: Custom useLang hook + JSON language packs

### AI Runtime
- **Language**: Go
- **Backend Support**: Claude API (api mode) / Claude CLI (cli mode)
- **Protocol**: Message Bus Protocol (JSON Envelope over WebSocket)
- **Session Management**: Runtime-level session isolation

---

## Message Bus Protocol

All communication is based on JSON `Envelope` format:

```json
{
  "id": "msg_1234_5678",
  "from": "ui://user123/conn456",
  "to": "session://session-id",
  "type": "message",
  "session_id": "session-id",
  "payload": {
    "content": [
      { "type": "text", "content": "Hello" },
      { "type": "code", "language": "go", "content": "fmt.Println()" }
    ],
    "metadata": {}
  },
  "timestamp": 1718000000000
}
```

### Message Types

| Type | Direction | Purpose |
|------|-----------|---------|
| `hello` / `bye` | Endpoint ↔ Bus | Connection register/unregister |
| `ping` / `pong` | Endpoint ↔ Bus | Heartbeat |
| `session.create` / `session.created` | UI → Bus → Runtime | Create new session |
| `session.join` / `session.joined` | UI → Bus → Runtime | Join existing session |
| `session.end` | Any → Bus | End session |
| `message` | Any → Bus → Target | Application messages (text/code/image/etc.) |
| `tool.use` / `tool.result` | Runtime → UI / UI → Runtime | AI tool calls and results |
| `permission.request` / `permission.response` | Runtime ↔ UI | Tool call permission confirmation |
| `event` | Runtime → Bus | Runtime event notifications |

### Address Format

| Endpoint Type | Format | Example |
|---------------|--------|---------|
| UI Frontend | `ui://{userID}/{connID}` | `ui://u001/cabc123` |
| Agent Runtime | `runtime://{nodeID}/{instance}` | `runtime://node-001/main` |
| System | `system://{service}` | `system://bus`, `system://api` |
| Session | `session://{sessionID}` | `session://abc-123-def` |

### Content Types

`ContentBlock` supports multiple content formats: `text`, `code`, `markdown`, `table`, `card`, `image`, `file`, `progress`, `tool_use`, `status`, `separator`.

---

## Database Tables

| Table | Purpose | Key Fields |
|-------|---------|------------|
| `users` | Users | id, username, email, password |
| `workspaces` | Workspaces | id, name, description |
| `workspace_members` | Membership relations | workspace_id, user_id, role |
| `pending_invitations` | Pending invitations | token, invitee_email, status, expires_at |
| `sessions` | AI sessions | node_id, agent_id, status, workspace |
| `messages` | Message history | session_id, envelope (JSONB) |
| `nodes` | Runtime nodes | id, name, status, ip, max_sessions |
| `agents` | Agent instances | node_id, name, command, enabled |
| `agent_profiles` | User Agent profiles | user_id, name, avatar, model, backend |
| `tasks` | Tasks | title, status, priority, project_id, parent_id, assignee_id, assignee_type, due_at, workspace_id, tags |
| `task_assignees` | Delegated assignees | task_id, assignee_id, assignee_type |
| `task_tags` | Task tags | task_id, tag |
| `task_comments` | Task comments | task_id, user_id, agent_profile_id, content |
| `projects` | Projects | name, color, status, assignee, started_at, due_at, workspace_id |

---

## API Overview

### Authentication
- `POST /api/auth/register` — Register
- `POST /api/auth/login` — Login

### Workspaces
- `GET /api/workspaces` — List
- `POST /api/workspaces` — Create
- `GET /api/workspaces/:id` — Detail
- `PUT /api/workspaces/:id` — Update
- `DELETE /api/workspaces/:id` — Delete

### Workspace Members
- `GET /api/workspaces/:id/members` — List
- `POST /api/workspaces/:id/members` — Add member
- `PUT /api/workspaces/:id/members/:userId` — Modify role
- `DELETE /api/workspaces/:id/members/:userId` — Remove member

### Invitations
- `POST /api/workspaces/:id/invitations` — Create invitation
- `GET /api/workspaces/:id/invitations` — List
- `DELETE /api/workspaces/:id/invitations/:invitationId` — Cancel
- `GET /api/invitations/:token` — Lookup (public)
- `POST /api/invitations/:token/accept` — Accept
- `POST /api/invitations/:token/decline` — Decline (public)
- `GET /api/invitations/pending` — Pending invitations list

### Agent Configuration
- `GET /api/agents/profiles` — List
- `POST /api/agents/profiles` — Create
- `GET /api/agents/profiles/:id` — Detail
- `PUT /api/agents/profiles/:id` — Update
- `DELETE /api/agents/profiles/:id` — Delete
- `GET /api/agents/runtimes` — Available runtimes list

### Sessions
- `POST /api/sessions` — Create
- `GET /api/sessions` — List (supports `?workspace_id=` filtering)
- `GET /api/sessions/:id` — Detail
- `GET /api/sessions/:id/messages` — Message history

### Tasks
- `GET /api/tasks` — List (supports `?project_id=`, `?parent_id=`, `?assignee_id=`, `?priority=`, `?tag=` filtering)
- `POST /api/tasks` — Create
- `GET /api/tasks/trash` — Trash
- `GET /api/tasks/:id` — Detail
- `PUT /api/tasks/:id` — Update
- `DELETE /api/tasks/:id` — Soft delete
- `DELETE /api/tasks/:id/force` — Permanent delete
- `POST /api/tasks/:id/restore` — Restore
- `PATCH /api/tasks/:id/status` — Update status
- `POST /api/tasks/:id/assignees` — Add delegated assignee
- `DELETE /api/tasks/:id/assignees/:assigneeId` — Remove delegated assignee
- `GET /api/tasks/:id/assignees` — Delegated assignees list
- `GET /api/tasks/:id/subtasks` — Subtasks list
- `GET /api/tasks/:id/comments` — Comments list
- `POST /api/tasks/:id/comments` — Create comment
- `DELETE /api/tasks/:id/comments/:commentId` — Delete comment

### Projects
- `GET /api/projects` — List (supports `?status=` filtering)
- `POST /api/projects` — Create
- `GET /api/projects/trash` — Trash
- `GET /api/projects/:id` — Detail
- `PUT /api/projects/:id` — Update
- `DELETE /api/projects/:id` — Soft delete
- `DELETE /api/projects/:id/force` — Permanent delete
- `POST /api/projects/:id/restore` — Restore

### Node Management
- `POST /api/nodes/token` — Generate node join token
- `GET /api/nodes/install.sh?token=` — Get bash install script (macOS/Linux)
- `GET /api/nodes/install.ps1?token=` — Get PowerShell install script (Windows)
- `GET /api/nodes/bin/:os/:arch` — Download precompiled agent-runtime binary
- `POST /api/nodes/register` — Node registration
- `POST /api/nodes/heartbeat` — Node heartbeat
- `GET /api/nodes` — Node list
- `GET /api/nodes/:id` — Node detail
- `GET /api/nodes/:id/agents` — Node Agent list
- `POST /api/nodes/:id/scan` — Scan node Agents
- `PATCH /api/agents/:id` — Start/stop Agent
- `DELETE /api/nodes/:id` — Remove node

### WebSocket
- `GET /ws/dashboard?token={jwt}` — Dashboard real-time notifications
- `GET /ws/bus?type=ui&user_id={id}` — Message Bus message routing

### User Management
- `GET /api/users` — User list (admin/owner)
- `DELETE /api/users/:id` — Delete user (admin/owner)

---

## Quick Start

### 1. Prerequisites

- Go 1.21+
- Node.js 18+
- PostgreSQL 14+

### 2. Configuration

```bash
cp .env.example .env
# Edit .env, fill in the required environment variables
```

### 3. Start Database

Make sure PostgreSQL is running, create the database:

```bash
createdb coaether
```

### 4. Start Backend

```bash
cd server
go run .
# Listening on :8088
# Database migration runs automatically on first start
```

### 5. Start Frontend

```bash
cd webui
npm install
npm run dev
# Open http://localhost:5173
```

### 6. Add Remote Node

Click **Add Node** on the Web UI nodes page, enter a node name to generate the install command, then run it on the target machine (Mac/Windows) to automatically install and register:

**Mac:**
```bash
curl -s 'http://<server>:8088/api/nodes/install.sh?token=TOKEN' | bash
```

**Windows (PowerShell):**
```powershell
powershell -c "iex ((Invoke-WebRequest -Uri 'http://<server>:8088/api/nodes/install.ps1?token=TOKEN').Content)"
```

The install script will automatically:
- Download the agent-runtime binary for the corresponding OS/Arch
- Install Claude Code CLI (if not already installed and npm is available)
- Create auto-start service (LaunchAgent / Startup folder)
- Start agent-runtime and connect to Message Bus

### 7. Node Runtime CLI Management

agent-runtime supports command-line management. After installation, use the following commands:

```bash
# Start (use token for first registration, saved key used automatically afterwards)
agent-runtime start -s <server>:8088 -t <token>

# View runtime status
agent-runtime status

# Graceful shutdown
agent-runtime stop

# Test server connection
agent-runtime connect -s <server>:8088 -t <token>

# Manage configuration
agent-runtime config list          # View all configuration
agent-runtime config set KEY=VALUE # Modify configuration

# View version
agent-runtime version
```

> Agent Runtime backend registration order: Claude CLI → Claude API (ANTHROPIC_API_KEY) → Echo (testing)

---

## Environment Variables

### Backend (server/.env)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `POSTGRES_DSN` | PostgreSQL connection string | `postgres://postgres:postgres@localhost:5432/coaether?sslmode=disable` | Yes |
| `JWT_SECRET` | JWT signing key | `coaether-secret-key` | Yes |
| `PORT` | HTTP service port | `8088` | No |
| `SMTP_HOST` | SMTP server address | - | Required for invitations |
| `SMTP_PORT` | SMTP port | `587` | No |
| `SMTP_USER` | SMTP username | - | Required for invitations |
| `SMTP_PASS` | SMTP password | - | Required for invitations |
| `SMTP_FROM` | Sender email address | - | Required for invitations |
| `PUBLIC_URL` | Public access URL (used for invitation links) | `http://localhost:5173` | No |

> When SMTP is not configured, invitation links are printed to server logs and can still be used normally.

### Agent Runtime (~/.coaether/env)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `SERVER_URL` | Server address | `localhost:8088` | No |
| `NODE_TOKEN` | Node registration token | - | **Yes** on first registration / No afterwards |
| `NODE_SECRET` | Persistent connection key (auto-saved after first registration) | - | No |
| `NODE_ID` | Node ID (used for key reconnection) | - | No |
| `RUNTIME_NAME` | Node display name | hostname | No |

> All configuration items can be overridden via CLI parameters, e.g., `agent-runtime start -s <addr> -t <token>`.

---

## Project Structure

```
coaether/
├── server/                    # Go Backend
│   ├── main.go               # Entry: route registration, dependency injection
│   ├── config/               # Configuration loading
│   ├── database/             # Database connection + migration + schema
│   ├── handlers/             # HTTP + WebSocket handlers
│   │   ├── auth.go           # Login/Register
│   │   ├── workspace.go      # Workspace CRUD + member management + invitations
│   │   ├── session.go        # AI session management
│   │   ├── task.go           # Task CRUD + trash
│   │   ├── project.go        # Project CRUD + trash
│   │   ├── agent_profile.go  # Agent profile CRUD
│   │   ├── node.go           # Runtime node management
│   │   ├── user.go           # User management
│   │   ├── ws.go             # DashboardHub (notifications/signals)
│   │   └── bus_handler.go    # Message Bus WebSocket entry
│   ├── middleware/           # Gin middleware
│   │   ├── auth.go           # JWT authentication
│   │   ├── roles.go          # Role permission checks
│   │   └── workspace_auth.go # Workspace-level permissions
│   ├── protocol/             # Message Bus protocol definitions + routing
│   │   ├── message.go        # Envelope, Payload, ContentBlock
│   │   ├── bus.go            # MessageBus core: endpoint/session management, message routing
│   │   └── address.go        # Address parsing
│   ├── models/               # Data models
│   ├── store/                # Message persistence (PostgreSQL)
│   ├── mailer/               # Email sending
│   └── notifications/        # Notification system
│
├── webui/                    # React Frontend
│   ├── src/
│   │   ├── App.tsx           # Main app: routing, authentication, layout
│   │   ├── api/client.ts     # HTTP API client
│   │   │   ├── components/       # Components
│   │   │   ├── FloatingChat.tsx     # Floating chat window
│   │   │   ├── MessageStream.tsx    # Message rendering stream (rich text)
│   │   │   ├── InputArea.tsx        # Message input area
│   │   │   ├── TaskBoard.tsx        # Task Kanban board
│   │   │   ├── TaskDetail.tsx       # Task detail (GitHub Issue style inline editing)
│   │   │   ├── TaskCard.tsx         # Task card
│   │   │   ├── TaskForm.tsx         # Task creation form
│   │   │   ├── ProjectList.tsx      # Project list
│   │   │   ├── ProjectCard.tsx      # Project card
│   │   │   ├── ProjectForm.tsx      # Project create/edit form
│   │   │   ├── ProjectDetail.tsx    # Project detail (with task list)
│   │   │   ├── NotificationBell.tsx # Notification bell
│   │   │   ├── AgentList.tsx        # Agent list
│   │   │   ├── Sidebar.tsx          # Sidebar
│   │   │   ├── LoginForm.tsx        # Login form
│   │   │   ├── AddNodeDialog.tsx    # Add Node dialog (platform selection/command copy)
│   │   │   └── NodeList.tsx         # Node card list (status/Agent/delete)
│   │   ├── hooks/            # React Hooks
│   │   │   ├── useMessageBus.ts    # Message Bus WebSocket hook
│   │   │   ├── useDashboardWS.ts   # Dashboard WebSocket hook
│   │   │   ├── useResourceSync.ts  # Resource auto-sync
│   │   │   └── useLang.ts          # Internationalization
│   │   ├── i18n/             # Internationalization language packs
│   │   └── types/            # TypeScript type definitions
│   └── vite.config.ts
│
├── agent-runtime/            # AI Agent Runtime
│   ├── main.go               # CLI entry point (Cobra)
│   ├── runtime.go             # Core: connect Message Bus, register backends
│   ├── root.go                # Root command definition
│   ├── start.go               # start command: start and connect
│   ├── stop.go                # stop command: graceful shutdown
│   ├── status.go              # status command: view status
│   ├── connect.go             # connect command: connection diagnostics
│   ├── config.go              # config command: configuration management
│   ├── backends/              # AI backend adapters
│   │   ├── claude_cli.go      # Claude CLI mode (stream-json, preferred)
│   │   ├── claude.go          # Claude API mode (ANTHROPIC_API_KEY)
│   │   └── echo.go            # Testing Echo backend (fallback)
│   └── bin/                   # Local build output
│       ├── darwin-arm64/
│       └── darwin-amd64/
│
├── server/
│   └── bin/
│       ├── myai-server*      # Server binary
│       ├── myai-server.exe   # Windows server binary
│       └── agents/           # Node distribution binaries
│           ├── darwin-arm64/agent-runtime
│           ├── darwin-amd64/agent-runtime
│           └── windows-amd64/agent-runtime.exe
│
└── README.md
```

---

## Development Guide

### Adding New API Endpoints

1. Add or modify handler in `server/handlers/`
2. Register the route in `server/main.go`
3. If workspace isolation is needed, ensure the route is in the `api` group (automatically applies `WorkspaceAuthMiddleware`)
4. Add the corresponding method on the frontend in `webui/src/api/client.ts`

### Adding New Database Tables

1. Add `CREATE TABLE` in the `schema` constant of the `Migrate()` function in `server/database/database.go`
2. If modifying an existing table, add `ALTER TABLE` in the `alterations` slice

### Adding New WebSocket Message Types

1. Add message type constants in `server/protocol/message.go`
2. Handle the new message type in the corresponding handler
3. Consume on the frontend in `useMessageBus` or `useDashboardWS`

### Internationalization

Add corresponding translation keys in `webui/src/i18n/en.ts` and `webui/src/i18n/zh.ts`, use on the frontend via `t('key')`.

---

## License

[Apache-2.0](LICENSE)
