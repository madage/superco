# Superco - AI Agent 分布式调度平台

跨平台 AI Agent 分布式调度平台，通过 **Message Bus** 协议连接 AI Runtime 与 Web 前端，提供多用户工作区、任务/项目管理、实时聊天、Agent 配置等能力。

## 架构

```
┌─────────────────────────────────────────────────────────┐
│                      Web UI (React)                      │
│  ┌────────────┐  ┌──────────────┐  ┌─────────────────┐  │
│  │  Dashboard  │  │  Chat Panel  │  │  Notification   │  │
│  │  (任务/项目) │  │  (浮动聊天窗) │  │  (铃铛/Toast)    │  │
│  └──────┬──────┘  └──────┬───────┘  └────────┬────────┘  │
│         │                │                    │           │
│  ┌──────┴────────────────┴────────────────────┴────────┐  │
│  │              WebSocket 客户端层                       │  │
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
│  │ (通知/信号)   │  │  (消息路由)    │  │  (CRUD/认证)      │  │
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
     │ (API模式)   │  │ (CLI模式)   │  │ (远程)      │
     └────────────┘  └────────────┘  └────────────┘
```

### 核心子系统

| 子系统 | 定位 | 技术栈 |
|--------|------|--------|
| **server/** | HTTP + WebSocket 服务端，认证、CRUD、消息路由 | Go + Gin + gorilla/websocket + PostgreSQL |
| **webui/** | React 前端 SPA，Dashboard + 浮动聊天 | React 18 + TypeScript + Vite |
| **agent-runtime/** | AI Agent 运行时，通过 Message Bus 连接平台 | Go，支持 Claude CLI / API 后端 |

### 通信架构

系统采用 **双 WebSocket 通道** 架构：

1. **Dashboard WebSocket** (`/ws/dashboard`) — 用于 UI 实时更新（任务/项目变更通知、工作区信号、Toast 弹窗）。持有 JWT token 鉴权，连接到 `DashboardHub`。
2. **Message Bus WebSocket** (`/ws/bus`) — 用于 AI Agent 消息路由。以 `type=ui` 参数标识前端连接，连接到 `MessageBus`，不依赖 JWT（连接后通过 `hello` 消息注册）。

## 功能特性

### 多用户工作区
- 基于角色的权限体系：`owner` > `admin` > `worker` > `observer`
- 支持工作区切换（侧边栏下拉选择器）
- 自动为无工作区用户创建默认工作区
- 工作区级资源隔离：任务、项目、Agent 配置、会话均绑定工作区

### 角色权限矩阵

| 操作 | owner | admin | worker | observer |
|------|-------|-------|--------|----------|
| 查看工作区内容 | ✅ | ✅ | ✅ | ✅ |
| 创建/编辑任务 | ✅ | ✅ | ✅ | ❌ |
| 管理项目 | ✅ | ✅ | ✅ | ❌ |
| 配置 Agent | ✅ | ✅ | ❌ | ❌ |
| 管理工作区成员 | ✅ | ✅ | ❌ | ❌ |
| 删除工作区 | ✅ | ❌ | ❌ | ❌ |
| 修改角色 | ✅ | ❌ | ❌ | ❌ |

### AI Agent 聊天
- 浮动聊天窗口（可拖拽），支持多会话管理
- 多 Agent 选择：用户可配置多个 Agent Profile，切换 Agent 自动恢复对应会话
- 会话持久化：页面刷新后自动恢复活跃会话
- Agent 间会话隔离：不同 Agent 的会话独立存储、独立恢复
- 富文本消息渲染：代码块、表格、Markdown、图片、进度指示器
- 文件/图片上传：支持粘贴或拖拽上传
- 工具调用权限控制：auto 模式（自动批准）和 restricted 模式（手动确认）

### Agent 配置系统
- 自定义 Agent Profile：名称、头像、描述、关联运行时、模型选择
- 支持 CLI 和 API 两种后端模式
- 运行时自动发现和注册
- 支持按工作区隔离配置

### 任务管理
- 看板视图，支持状态流转：`todo` → `in_progress` → `blocked` → `done` → `review`
- 关联项目，按项目组织任务
- 回收站机制：软删除 + 可恢复 + 永久删除
- 按工作区隔离

### 项目管理
- 支持颜色标签、描述
- 关联任务计数
- 回收站机制（软删除/恢复/永久删除）
- 按工作区隔离

### 回收站
- 任务和项目均支持软删除
- 独立的回收站视图（`/tasks/trash`、`/projects/trash`）
- 支持恢复和永久删除

### 工作区邀请
- 邮箱邀请：生成唯一 token 链接
- 邀请管理：列出、取消待处理的邀请
- 站内通知：被邀请用户登录后铃铛红点提示
- 即时通知：WebSocket 实时推送邀请通知
- 支持接受/拒绝邀请
- 邀请过期自动标记

### WebSocket 实时推送
- 工作区删除/成员移除/角色变更即时通知用户
- Toast 弹窗提示（自动消失，5秒）
- Dashboard 数据自动刷新（`useResourceSync` hook）
- 邀请变更实时同步

### 多语言
- 中文 / English 双语界面
- 通过 `useLang()` hook 切换

### 用户管理
- 管理员可查看所有用户列表
- 支持删除用户
- JWT 认证（Access Token）

## 技术栈

### 后端
- **语言**: Go 1.21+
- **Web 框架**: Gin
- **WebSocket**: gorilla/websocket（DashboardHub + MessageBus 双通道）
- **数据库**: PostgreSQL（database/sql + lib/pq）
- **认证**: JWT（golang-jwt v5）
- **邮件**: 基于 net/smtp 的邮件发送

### 前端
- **框架**: React 18 + TypeScript
- **构建工具**: Vite
- **通信**: REST API + WebSocket（Dashboard 信号 + Message Bus 消息）
- **状态管理**: React Hooks (useState/useEffect/useCallback/useRef)
- **国际化**: 自定义 useLang hook + JSON 语言包

### AI 运行时
- **语言**: Go
- **后端支持**: Claude API（api 模式）/ Claude CLI（cli 模式）
- **协议**: Message Bus 协议（JSON Envelope over WebSocket）
- **会话管理**: 运行时级别会话隔离

## Message Bus 协议

所有通信基于 JSON `Envelope` 格式：

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

### 消息类型

| 类型 | 方向 | 用途 |
|------|------|------|
| `hello` / `bye` | 端点 ↔ Bus | 连接注册/注销 |
| `ping` / `pong` | 端点 ↔ Bus | 心跳检测 |
| `session.create` / `session.created` | UI → Bus → Runtime | 创建新会话 |
| `session.join` / `session.joined` | UI → Bus → Runtime | 加入已有会话 |
| `session.end` | 任意 → Bus | 结束会话 |
| `message` | 任意 → Bus → 目标 | 应用消息（文本/代码/图片等） |
| `tool.use` / `tool.result` | Runtime → UI / UI → Runtime | AI 工具调用与结果 |
| `permission.request` / `permission.response` | Runtime ↔ UI | 工具调用权限确认 |
| `event` | Runtime → Bus | 运行时事件通知 |

### 地址格式

| 端点类型 | 格式 | 示例 |
|----------|------|------|
| UI 前端 | `ui://{userID}/{connID}` | `ui://u001/cabc123` |
| Agent 运行时 | `runtime://{nodeID}/{instance}` | `runtime://node-001/main` |
| 系统 | `system://{service}` | `system://bus`、`system://api` |
| 会话 | `session://{sessionID}` | `session://abc-123-def` |

### 内容类型

`ContentBlock` 支持多种内容格式：`text`、`code`、`markdown`、`table`、`card`、`image`、`file`、`progress`、`tool_use`、`status`、`separator`。

## 数据库表

| 表名 | 用途 | 关键字段 |
|------|------|----------|
| `users` | 用户 | id, username, email, password |
| `workspaces` | 工作区 | id, name, description |
| `workspace_members` | 成员关系 | workspace_id, user_id, role |
| `pending_invitations` | 待处理邀请 | token, invitee_email, status, expires_at |
| `sessions` | AI 会话 | node_id, agent_id, status, workspace |
| `messages` | 消息历史 | session_id, envelope (JSONB) |
| `nodes` | 运行时节点 | id, name, status, ip, max_sessions |
| `agents` | Agent 实例 | node_id, name, command, enabled |
| `agent_profiles` | 用户 Agent 配置 | user_id, name, avatar, model, backend |
| `tasks` | 任务 | title, status, project_id, workspace_id |
| `projects` | 项目 | name, color, workspace_id |

## API 概览

### 认证
- `POST /api/auth/register` — 注册
- `POST /api/auth/login` — 登录

### 工作区
- `GET /api/workspaces` — 列表
- `POST /api/workspaces` — 创建
- `GET /api/workspaces/:id` — 详情
- `PUT /api/workspaces/:id` — 更新
- `DELETE /api/workspaces/:id` — 删除

### 工作区成员
- `GET /api/workspaces/:id/members` — 列表
- `POST /api/workspaces/:id/members` — 添加成员
- `PUT /api/workspaces/:id/members/:userId` — 修改角色
- `DELETE /api/workspaces/:id/members/:userId` — 移除成员

### 邀请
- `POST /api/workspaces/:id/invitations` — 创建邀请
- `GET /api/workspaces/:id/invitations` — 列表
- `DELETE /api/workspaces/:id/invitations/:invitationId` — 取消
- `GET /api/invitations/:token` — 查询（公开）
- `POST /api/invitations/:token/accept` — 接受
- `POST /api/invitations/:token/decline` — 拒绝（公开）
- `GET /api/invitations/pending` — 待处理邀请列表

### Agent 配置
- `GET /api/agents/profiles` — 列表
- `POST /api/agents/profiles` — 创建
- `GET /api/agents/profiles/:id` — 详情
- `PUT /api/agents/profiles/:id` — 更新
- `DELETE /api/agents/profiles/:id` — 删除
- `GET /api/agents/runtimes` — 可用运行时列表

### 会话
- `POST /api/sessions` — 创建
- `GET /api/sessions` — 列表（支持 `?workspace_id=` 过滤）
- `GET /api/sessions/:id` — 详情
- `GET /api/sessions/:id/messages` — 消息历史

### 任务
- `GET /api/tasks` — 列表
- `POST /api/tasks` — 创建
- `GET /api/tasks/trash` — 回收站
- `PUT /api/tasks/:id` — 更新
- `DELETE /api/tasks/:id` — 软删除
- `DELETE /api/tasks/:id/force` — 永久删除
- `POST /api/tasks/:id/restore` — 恢复
- `PATCH /api/tasks/:id/status` — 更新状态

### 项目
- `GET /api/projects` — 列表
- `POST /api/projects` — 创建
- `GET /api/projects/trash` — 回收站
- `PUT /api/projects/:id` — 更新
- `DELETE /api/projects/:id` — 软删除
- `DELETE /api/projects/:id/force` — 永久删除
- `POST /api/projects/:id/restore` — 恢复

### WebSocket
- `GET /ws/dashboard?token={jwt}` — Dashboard 实时通知
- `GET /ws/bus?type=ui&user_id={id}` — Message Bus 消息路由

### 用户管理
- `GET /api/users` — 用户列表（admin/owner）
- `DELETE /api/users/:id` — 删除用户（admin/owner）

## 快速开始

### 1. 依赖

- Go 1.21+
- Node.js 18+
- PostgreSQL 14+

### 2. 配置

```bash
cp .env.example .env
# 编辑 .env，填写必要的环境变量
```

### 3. 启动数据库

确保 PostgreSQL 运行，创建数据库：

```bash
createdb superco
```

### 4. 启动后端

```bash
cd server
go run .
# 监听 :8088
# 首次启动自动执行数据库迁移
```

### 5. 启动前端

```bash
cd webui
npm install
npm run dev
# 打开 http://localhost:5173
```

### 6. 启动 Agent Runtime

```bash
cd agent-runtime
go build -o agent-runtime .
./agent-runtime
# 自动连接 ws://localhost:8088/ws/bus
```

> Agent Runtime 需要配置 AI 后端。默认使用 Claude CLI（需要 `claude` 命令在 PATH 中），也可通过环境变量切换为 API 模式。

## 环境变量

### 后端 (server/.env)

| 变量 | 说明 | 默认值 | 必填 |
|------|------|--------|------|
| `POSTGRES_DSN` | PostgreSQL 连接字符串 | `postgres://postgres:postgres@localhost:5432/superco?sslmode=disable` | 是 |
| `JWT_SECRET` | JWT 签名密钥 | `superco-secret-key` | 是 |
| `PORT` | HTTP 服务端口 | `8088` | 否 |
| `SMTP_HOST` | SMTP 服务器地址 | - | 邀请功能需要 |
| `SMTP_PORT` | SMTP 端口 | `587` | 否 |
| `SMTP_USER` | SMTP 用户名 | - | 邀请功能需要 |
| `SMTP_PASS` | SMTP 密码 | - | 邀请功能需要 |
| `SMTP_FROM` | 发件人邮箱地址 | - | 邀请功能需要 |
| `PUBLIC_URL` | 公开访问地址（用于邀请链接） | `http://localhost:5173` | 否 |

> SMTP 未配置时，邀请链接会输出到服务端日志，仍可正常使用。

### Agent Runtime (agent-runtime/.env)

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `BUS_URL` | Message Bus WebSocket 地址 | `ws://localhost:8088/ws/bus` |
| `AGENT_BACKEND` | AI 后端模式 (`cli` / `api`) | `cli` |
| `API_KEY` | API 模式时的密钥 | - |
| `API_MODEL` | API 模式时的模型名 | `claude-sonnet-4-6` |

## 项目结构

```
superco/
├── server/                    # Go 后端
│   ├── main.go               # 入口：路由注册、依赖注入
│   ├── config/               # 配置加载
│   ├── database/             # 数据库连接 + 迁移 + schema
│   ├── handlers/             # HTTP + WebSocket 处理器
│   │   ├── auth.go           # 登录/注册
│   │   ├── workspace.go      # 工作区 CRUD + 成员管理 + 邀请
│   │   ├── session.go        # AI 会话管理
│   │   ├── task.go           # 任务 CRUD + 回收站
│   │   ├── project.go        # 项目 CRUD + 回收站
│   │   ├── agent_profile.go  # Agent 配置 CRUD
│   │   ├── node.go           # 运行时节点管理
│   │   ├── user.go           # 用户管理
│   │   ├── ws.go             # DashboardHub (通知/信号)
│   │   └── bus_handler.go    # Message Bus WebSocket 入口
│   ├── middleware/           # Gin 中间件
│   │   ├── auth.go           # JWT 认证
│   │   ├── roles.go          # 角色权限检查
│   │   └── workspace_auth.go # 工作区级权限
│   ├── protocol/             # Message Bus 协议定义 + 路由
│   │   ├── message.go        # Envelope、Payload、ContentBlock
│   │   ├── bus.go            # MessageBus 核心：端点/会话管理、消息路由
│   │   └── address.go        # 地址解析
│   ├── models/               # 数据模型
│   ├── store/                # 消息持久化 (PostgreSQL)
│   ├── mailer/               # 邮件发送
│   └── notifications/        # 通知系统
│
├── webui/                    # React 前端
│   ├── src/
│   │   ├── App.tsx           # 主应用：路由、认证、布局
│   │   ├── api/client.ts     # HTTP API 客户端
│   │   ├── components/       # 组件
│   │   │   ├── FloatingChat.tsx    # 浮动聊天窗口
│   │   │   ├── MessageStream.tsx   # 消息渲染流（富文本）
│   │   │   ├── InputArea.tsx       # 消息输入区
│   │   │   ├── TaskBoard.tsx       # 任务看板
│   │   │   ├── ProjectList.tsx     # 项目列表
│   │   │   ├── NotificationBell.tsx # 通知铃铛
│   │   │   ├── AgentList.tsx       # Agent 列表
│   │   │   ├── Sidebar.tsx         # 侧边栏
│   │   │   └── LoginForm.tsx       # 登录表单
│   │   ├── hooks/            # React Hooks
│   │   │   ├── useMessageBus.ts    # Message Bus WebSocket hook
│   │   │   ├── useDashboardWS.ts   # Dashboard WebSocket hook
│   │   │   ├── useResourceSync.ts  # 资源自动同步
│   │   │   └── useLang.ts          # 国际化
│   │   ├── i18n/             # 国际化语言包
│   │   └── types/            # TypeScript 类型定义
│   └── vite.config.ts
│
├── agent-runtime/            # AI Agent 运行时
│   ├── main.go               # 入口
│   ├── bus_client.go         # Message Bus 客户端
│   ├── session.go            # 会话管理
│   └── backends/             # AI 后端适配器
│       ├── cli.go            # Claude CLI 模式
│       └── api.go            # API 模式
│
└── README.md
```

## 开发指南

### 添加新 API 端点

1. 在 `server/handlers/` 中新增或修改 handler
2. 在 `server/main.go` 中注册路由
3. 如果需要工作区隔离，确保路由在 `api` 组中（自动应用 `WorkspaceAuthMiddleware`）
4. 前端在 `webui/src/api/client.ts` 中添加对应方法

### 添加新数据库表

1. 在 `server/database/database.go` 的 `Migrate()` 函数的 `schema` 常量中添加 `CREATE TABLE`
2. 如果是对已有表的修改，在 `alterations` 切片中添加 `ALTER TABLE`

### 添加新 WebSocket 消息类型

1. 在 `server/protocol/message.go` 中添加消息类型常量
2. 在对应的 handler 中处理新消息类型
3. 前端在 `useMessageBus` 或 `useDashboardWS` 中消费

### 国际化

在 `webui/src/i18n/en.ts` 和 `webui/src/i18n/zh.ts` 中添加对应翻译 key，前端通过 `t('key')` 使用。

## 常见问题

**Q: 启动后端提示数据库连接失败？**
A: 确保 PostgreSQL 已启动，并在 `.env` 中正确配置 `POSTGRES_DSN`。

**Q: 邀请邮件没有收到？**
A: 检查 SMTP 配置。未配置 SMTP 时，邀请链接会输出在服务端日志中，可以直接复制链接使用。

**Q: 浮动聊天窗口连接不上？**
A: 确保后端已启动，Message Bus WebSocket 路径正确（默认 `/ws/bus`），检查浏览器控制台 WebSocket 连接状态。

**Q: 工作区切换后数据没更新？**
A: 系统通过 WebSocket 实时推送变更（`resource_change` 消息），如果出现不同步，刷新页面即可。

**Q: Agent Runtime 频繁重连？**
A: 检查 `BUS_URL` 配置是否正确，以及网络是否稳定。Runtime 会自动重连。
