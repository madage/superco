# Coaether 项目全部 API 接口文档

> 本文档描述 Coaether 系统的所有 HTTP API 端点，供外部系统集成调用。
>
> 配套文档：[本机运行环境说明](./本机运行环境说明.md)

---

## 目录

1. [通用说明](#通用说明)
2. [认证与用户](#1-认证与用户)
3. [任务管理](#2-任务管理)
4. [项目（列表）管理](#3-项目列表管理)
5. [工作区管理](#4-工作区管理)
6. [智能体 Profile 管理](#5-智能体-profile-管理)
7. [智能体调度与队列](#6-智能体调度与队列)
8. [节点管理](#7-节点管理)
9. [运行时节点 API（Node Agent）](#8-运行时节点-api-node-agent)
10. [技能库（Skills）](#9-技能库-skills)
11. [任务规则引擎](#10-任务规则引擎)
12. [通知系统](#11-通知系统)
13. [会话管理](#12-会话管理)
14. [插件管理](#13-插件管理)
15. [工作流管理](#14-工作流管理)
16. [Harness 智能体工具调用](#15-harness-智能体工具调用)
17. [辅助接口](#16-辅助接口)
18. [接口一览表](#17-接口一览表)

---

## 通用说明

### Base URL

```
http://<server>:8088/api
```

### 认证方式

| 接口分组 | 认证方式 |
|---------|---------|
| `/api/auth/*` | 公开（无需认证） |
| `/api/invitations/:token/*` | 公开（仅查看和拒绝邀请） |
| `/api/health` | 公开 |
| `/ws/*` | WebSocket（公开） |
| `/api/nodes/install.*` | 公开（安装脚本） |
| `/api/nodes/bin/*` | 公开（二进制下载） |
| `/api/node/*` | `node_secret` + `node_id` 查询参数 |
| 其余 `/api/*` | JWT Bearer Token |

### JWT 认证请求头

```
Content-Type: application/json
Authorization: Bearer <jwt_token>
```

### node_secret 认证

所有 `/api/node/*` 接口使用查询参数认证：

```
GET /api/node/queue?node_id=<node_id>&node_secret=<secret>
```

### 通用响应状态码

| 状态码 | 说明 |
|--------|------|
| `200` | 成功 |
| `201` | 创建成功 |
| `204` | 无内容（OPTIONS 预检） |
| `400` | 参数错误 |
| `403` | 权限不足 |
| `404` | 未找到 |
| `409` | 冲突（如邮箱重复） |
| `429` | 请求过多（并发超限） |
| `500` | 服务端错误 |

### 通用查询参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `workspace_id` | string | 通常必填 | 工作区 ID，大多数业务接口需要 |

---

## 1. 认证与用户

### 1.1 用户登录

```
POST /api/auth/login
```

**请求体：**

```json
{
  "email": "user@example.com",
  "password": "your-password"
}
```

**响应（200）：**

```json
{
  "token": "jwt-token-string",
  "user": {
    "id": "user-uuid",
    "username": "username",
    "email": "user@example.com"
  },
  "workspace_id": "default-workspace-id"
}
```

---

### 1.2 用户注册

```
POST /api/auth/register
```

**请求体：**

```json
{
  "email": "newuser@example.com",
  "password": "your-password",
  "invitation_token": "optional-invitation-token"
}
```

**说明：** `invitation_token` 可选。如果有邀请 Token，则直接加入对应工作区；否则自动创建默认工作区。

**响应（201）：**

```json
{
  "token": "jwt-token-string",
  "user": { "id": "user-uuid", "username": "username", "email": "newuser@example.com" },
  "workspace_id": "workspace-uuid"
}
```

---

### 1.3 用户列表（管理员/所有者）

```
GET /api/users
```

**认证：** JWT，需要 `admin` 或 `owner` 角色。

**响应：**

```json
{
  "users": [
    { "id": "user-uuid", "username": "user1", "email": "user1@example.com", "created_at": "2026-01-01T00:00:00Z" }
  ]
}
```

---

### 1.4 删除用户（管理员/所有者）

```
DELETE /api/users/:id
```

**认证：** JWT，需要 `admin` 或 `owner` 角色。

**说明：** 不能删除自己，不能删除工作区的最后一个所有者。

**响应：**

```json
{ "status": "deleted" }
```

---

## 2. 任务管理

### 2.1 任务模型

```json
{
  "id": "uuid",
  "user_id": "uuid",
  "creator_name": "创建者用户名",
  "title": "任务标题",
  "description": "任务描述",
  "status": "todo | in_progress | blocked | completed | done | review | stuck",
  "project_id": "uuid | null",
  "parent_id": "uuid | null",
  "assignee_id": "uuid | null",
  "assignee_type": "user | agent_profile | null",
  "priority": "urgent | high | medium | low",
  "tags": ["标签1", "标签2"],
  "assignees": [{"task_id": "uuid", "assignee_id": "uuid", "assignee_type": "user", "role": "..."}],
  "due_at": "datetime | null",
  "completed_at": "datetime | null",
  "created_at": "datetime",
  "updated_at": "datetime",
  // 工作流字段
  "workflow_id": "uuid | null",
  "depth": 0,
  "max_depth": 5,
  "max_agent_loops": 3,
  "agent_loop_count": 0,
  "completion_behavior": "auto_done | auto_review | sample_review | needs_review",
  "parallel_group": "group_name | null"
}
```

### 2.2 列出任务

```
GET /api/tasks?workspace_id=<workspace_id>
```

**查询参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `workspace_id` | string | 是 | 工作区 ID |
| `project_id` | string | 否 | 按项目过滤 |
| `status` | string | 否 | 按状态过滤 |
| `parent_id` | string | 否 | 按父任务过滤（`none` 表示无父任务） |
| `assignee_id` | string | 否 | 按负责人过滤 |
| `delegated_assignee_id` | string | 否 | 按受托人过滤 |
| `priority` | string | 否 | 按优先级过滤 |
| `tag` | string | 否 | 按标签过滤 |

**响应：**

```json
{ "tasks": [ /* Task 对象数组 */ ] }
```

---

### 2.3 创建任务

```
POST /api/tasks?workspace_id=<workspace_id>
```

**请求体：**

```json
{
  "title": "任务标题（必填）",
  "description": "任务描述",
  "project_id": "所属项目 ID",
  "parent_id": "父任务 ID",
  "assignee_id": "负责人 ID",
  "assignee_type": "user | agent_profile",
  "priority": "medium",
  "tags": ["前端", "bug"],
  "due_at": "2026-06-15T00:00:00Z",
  "auto_assign": false
}
```

**说明：** `auto_assign: true` 时，系统自动匹配智能体。

**响应（201）：** Task 对象

---

### 2.4 获取任务

```
GET /api/tasks/:id?workspace_id=<workspace_id>
```

**响应：** Task 对象

---

### 2.5 更新任务

```
PUT /api/tasks/:id?workspace_id=<workspace_id>
```

**请求体（全部可选，至少传一个）：**

```json
{
  "title": "新标题",
  "description": "新描述",
  "status": "in_progress",
  "project_id": "新项目 ID",
  "parent_id": "新父任务 ID",
  "assignee_id": "新负责人 ID",
  "assignee_type": "user | agent_profile",
  "priority": "high",
  "tags": ["新标签"],
  "due_at": "2026-06-20T00:00:00Z"
}
```

**响应：** 完整的 Task 对象

```json
{
  "id": "uuid",
  "title": "新标题",
  "status": "in_progress",
  ...
}
```

---

### 2.6 删除任务（软删除）

```
DELETE /api/tasks/:id?workspace_id=<workspace_id>
```

**响应：**

```json
{ "status": "deleted" }
```

---

### 2.7 永久删除任务

```
DELETE /api/tasks/:id/force?workspace_id=<workspace_id>
```

**响应：**

```json
{ "status": "permanently_deleted" }
```

---

### 2.8 恢复任务

```
POST /api/tasks/:id/restore?workspace_id=<workspace_id>
```

**响应：**

```json
{ "status": "restored" }
```

---

### 2.9 列出回收站

```
GET /api/tasks/trash?workspace_id=<workspace_id>
```

**响应：**

```json
{ "tasks": [ /* 已软删除的 Task 对象 */ ] }
```

---

### 2.10 设置任务状态

```
PATCH /api/tasks/:id/status?workspace_id=<workspace_id>
```

**请求体：**

```json
{ "status": "in_progress" }
```

**合法值：** `todo`、`in_progress`、`blocked`、`completed`、`done`、`review`、`stuck`

**说明：**
- 如果新状态为 `in_progress` 且负责人是智能体（`assignee_type=agent_profile`），系统自动创建队列条目并通知运行时处理。
- 如果新状态为 `completed`，系统根据任务的 `completion_behavior` 自动路由：`auto_done`→直接完成、`auto_review`→交给智能体审核、`sample_review`→抽检、`needs_review`→人工审核。

**响应：**

```json
{ "status": "updated" }
```

---

### 2.11 添加受托人

```
POST /api/tasks/:id/assignees?workspace_id=<workspace_id>
```

**请求体：**

```json
{
  "assignee_id": "user-uuid",
  "assignee_type": "user"
}
```

**响应（201）：**

```json
{ "status": "added" }
```

---

### 2.12 移除受托人

```
DELETE /api/tasks/:id/assignees/:assigneeId?workspace_id=<workspace_id>
```

**响应：**

```json
{ "status": "removed" }
```

---

### 2.13 列出受托人

```
GET /api/tasks/:id/assignees?workspace_id=<workspace_id>
```

**响应：**

```json
{
  "assignees": [
    { "task_id": "uuid", "assignee_id": "uuid", "assignee_type": "user", "role": "member" }
  ]
}
```

---

### 2.14 列出子任务

```
GET /api/tasks/:id/subtasks?workspace_id=<workspace_id>
```

**响应：**

```json
{ "tasks": [ /* Task 对象数组 */ ] }
```

---

### 2.15 列出评论

```
GET /api/tasks/:id/comments?workspace_id=<workspace_id>
```

**响应：**

```json
{
  "comments": [
    {
      "id": "uuid",
      "task_id": "uuid",
      "user_id": "uuid",
      "username": "用户名",
      "agent_profile_id": "uuid | null",
      "agent_name": "智能体名称",
      "agent_avatar": "🤖",
      "content": "评论内容（支持 HTML）",
      "parent_id": "uuid | null",
      "is_agent_comment": false,
      "created_at": "datetime",
      "updated_at": "datetime"
    }
  ]
}
```

---

### 2.16 创建评论

```
POST /api/tasks/:id/comments?workspace_id=<workspace_id>
```

**请求体：**

```json
{
  "content": "评论内容（支持 HTML）",
  "parent_id": "回复的评论 ID（可选）",
  "is_agent_comment": false,
  "agent_profile_id": "智能体 ID（智能体评论时必填）"
}
```

**说明：** `is_agent_comment=true` 时，表示为智能体自动发布的评论，跳过用户权限检查。

**响应（201）：** 创建的 Comment 对象

---

### 2.17 删除评论

```
DELETE /api/tasks/:id/comments/:commentId?workspace_id=<workspace_id>
```

**响应：**

```json
{ "status": "deleted" }
```

---

### 2.18 审核任务

提交对任务的审核结果（批准或驳回）。

```
POST /api/tasks/:id/review
```

**请求体：**

```json
{
  "action": "approved | rejected",
  "comment": "审核意见",
  "reviewer_agent_id": "智能体 ID（可选，智能体审核时传入）"
}
```

**响应：**

```json
{ "status": "approved" }
```

**说明：**
- `approved` → 任务状态改为 `done`，触发 DAG 推进
- `rejected` → 任务回到 `in_progress`，`agent_loop_count` +1
- 当驳回次数达到 `max_agent_loops` 时触发熔断（`stuck`），通知工作区管理员

---

## 3. 项目（列表）管理

### 3.1 项目模型

```json
{
  "id": "uuid",
  "user_id": "uuid",
  "name": "项目名称",
  "description": "描述",
  "color": "#1976d2",
  "assignee_id": "uuid | null",
  "assignee_type": "user | agent_profile | null",
  "status": "active | archived",
  "started_at": "datetime | null",
  "due_at": "datetime | null",
  "task_count": 0,
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### 3.2 列出项目

```
GET /api/projects?workspace_id=<workspace_id>&status=<status>
```

**响应：**

```json
{ "projects": [ /* Project 对象数组 */ ] }
```

---

### 3.3 创建项目

```
POST /api/projects?workspace_id=<workspace_id>
```

**请求体：**

```json
{
  "name": "项目名称（必填）",
  "description": "描述",
  "color": "#1976d2",
  "status": "active"
}
```

**响应（201）：** Project 对象

---

### 3.4 获取项目

```
GET /api/projects/:id?workspace_id=<workspace_id>
```

**响应：** Project 对象

---

### 3.5 更新项目

```
PUT /api/projects/:id?workspace_id=<workspace_id>
```

**请求体（全部可选）：**

```json
{
  "name": "新名称",
  "description": "新描述",
  "color": "#e91e63",
  "status": "archived"
}
```

**响应：** Project 对象

---

### 3.6 删除项目（软删除）

```
DELETE /api/projects/:id?workspace_id=<workspace_id>
```

**响应：**

```json
{ "status": "deleted" }
```

---

### 3.7 永久删除项目

```
DELETE /api/projects/:id/force?workspace_id=<workspace_id>
```

**响应：**

```json
{ "status": "permanently_deleted" }
```

---

### 3.8 恢复项目

```
POST /api/projects/:id/restore?workspace_id=<workspace_id>
```

**响应：**

```json
{ "status": "restored" }
```

---

### 3.9 列出项目回收站

```
GET /api/projects/trash?workspace_id=<workspace_id>
```

**响应：**

```json
{ "projects": [ /* 已软删除的 Project 对象 */ ] }
```

---

## 4. 工作区管理

### 4.1 工作区模型

```json
{
  "id": "uuid",
  "user_id": "uuid",
  "name": "工作区名称",
  "description": "描述",
  "role": "owner | admin | member",
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### 4.2 列出工作区

```
GET /api/workspaces
```

**响应：**

```json
{ "workspaces": [ /* Workspace 对象数组 */ ] }
```

---

### 4.3 创建工作区

```
POST /api/workspaces
```

**请求体：**

```json
{
  "name": "工作区名称（必填）",
  "description": "描述"
}
```

**说明：** 创建者自动成为 `owner`。

**响应（201）：** Workspace 对象

---

### 4.4 获取工作区

```
GET /api/workspaces/:id
```

**响应：** Workspace 对象

---

### 4.5 更新工作区

```
PUT /api/workspaces/:id
```

**请求体（全部可选）：**

```json
{
  "name": "新名称",
  "description": "新描述"
}
```

**响应：** Workspace 对象

---

### 4.6 删除工作区

```
DELETE /api/workspaces/:id
```

**响应：**

```json
{ "status": "deleted" }
```

---

### 4.7 列出工作区成员

```
GET /api/workspaces/:id/members
```

**响应：**

```json
{
  "members": [
    { "user_id": "uuid", "username": "user1", "email": "user1@example.com", "role": "owner", "joined_at": "datetime" }
  ]
}
```

---

### 4.8 添加成员

```
POST /api/workspaces/:id/members
```

**请求体：**

```json
{
  "user_id": "用户 ID（必填）",
  "role": "member（默认）"
}
```

**角色可选值：** `owner`、`admin`、`member`

**响应（201）：**

```json
{ "status": "added" }
```

---

### 4.9 更新成员角色

```
PUT /api/workspaces/:id/members/:userId
```

**请求体：**

```json
{ "role": "admin" }
```

**响应：**

```json
{ "status": "updated" }
```

---

### 4.10 移除成员

```
DELETE /api/workspaces/:id/members/:userId
```

**响应：**

```json
{ "status": "removed" }
```

---

### 4.11 创建邀请

```
POST /api/workspaces/:id/invitations
```

**请求体：**

```json
{
  "email": "invitee@example.com（必填）",
  "role": "member（默认）"
}
```

**响应（201）：** 包含邀请 Token 的邀请对象

---

### 4.12 列出工作区邀请

```
GET /api/workspaces/:id/invitations
```

**响应：**

```json
{ "invitations": [ /* PendingInvitation 对象数组 */ ] }
```

---

### 4.13 取消邀请

```
DELETE /api/workspaces/:id/invitations/:invitationId
```

**响应：**

```json
{ "status": "cancelled" }
```

---

### 4.14 通过 Token 获取邀请信息

```
GET /api/invitations/:token
```

**认证：** 公开（无需 JWT）

**响应：** 邀请信息对象

---

### 4.15 接受邀请

```
POST /api/invitations/:token/accept
```

**认证：** JWT

**响应：**

```json
{ "status": "accepted", "workspace_id": "uuid" }
```

---

### 4.16 拒绝邀请

```
POST /api/invitations/:token/decline
```

**认证：** 公开（无需 JWT）

**响应：**

```json
{ "status": "declined" }
```

---

### 4.17 列出待处理邀请

```
GET /api/invitations/pending
```

**认证：** JWT

**响应：**

```json
{ "invitations": [ /* PendingInvitation 对象数组 */ ] }
```

---

## 5. 智能体 Profile 管理

### 5.1 智能体 Profile 模型

```json
{
  "id": "uuid",
  "user_id": "uuid",
  "name": "显示名称",
  "avatar": "Emoji 头像",
  "description": "描述",
  "system_prompt": "系统提示词",
  "instructions": "行为指令",
  "agent_id": "运行时标识符（如 claude）",
  "node_id": "uuid（关联节点）",
  "version": "版本号",
  "model": "模型标识",
  "backend": "后端类型（如 cli）",
  "enabled": true,
  "max_concurrency": 3,
  "current_load": 1,
  "tags": ["前端", "React"],
  "skills": ["typescript", "css"],
  "capabilities": ["create_sub_task", "assign_task", "get_task_detail", "list_sub_tasks", "update_task_status"],
  "last_active_at": "datetime",
  "created_at": "datetime",
  "updated_at": "datetime",
  // Harness 扩展字段
  "protocol_version": "legacy | v1.0",
  "permissions": {},
  "max_depth": 5,
  "max_review_loops": 3,
  "completion_behavior": "auto_done",
  "review_sample_rate": 0.2,
  "review_timeout": 240
}
```

### 5.2 列出所有智能体

```
GET /api/agents/profiles?workspace_id=<workspace_id>
```

**响应：**

```json
{ "profiles": [ /* AgentProfile 对象数组 */ ] }
```

---

### 5.3 查询单个智能体

```
GET /api/agents/profiles/:id?workspace_id=<workspace_id>
```

**响应：** AgentProfile 对象

---

### 5.4 创建智能体

```
POST /api/agents/profiles?workspace_id=<workspace_id>
```

**请求体：**

```json
{
  "name": "后端工程师（必填）",
  "description": "负责后端开发和维护",
  "system_prompt": "你是一个资深后端开发工程师...",
  "instructions": "回复风格：专业、简洁、中文回复",
  "agent_id": "claude（必填）",
  "node_id": "关联的节点 ID（可选）",
  "avatar": "🖥️",
  "tags": ["后端", "Go"],
  "max_concurrency": 3,
  "capabilities": ["create_sub_task", "assign_task", "get_task_detail"]
}
```

**响应（201）：**

```json
{ "id": "newly-created-uuid", "status": "created" }
```

---

### 5.5 更新智能体

```
PUT /api/agents/profiles/:id?workspace_id=<workspace_id>
```

**请求体（全部可选，至少传一个）：**

```json
{
  "name": "新名称",
  "description": "新描述",
  "system_prompt": "新系统提示词",
  "instructions": "新行为指令",
  "avatar": "👾",
  "agent_id": "claude",
  "node_id": "新节点 ID",
  "enabled": true,
  "max_concurrency": 5,
  "tags": ["新标签"],
  "skills": ["typescript"],
  "review_sample_rate": 0.5,
  "review_timeout": 240,
  "max_review_loops": 3,
  "max_depth": 5,
  "completion_behavior": "auto_review",
  "capabilities": ["create_sub_task", "assign_task", "get_task_detail"]
}
```

**响应：**

```json
{ "status": "updated" }
```

---

### 5.6 删除智能体

```
DELETE /api/agents/profiles/:id?workspace_id=<workspace_id>
```

**权限要求：** 仅创建者或 `admin`/`owner` 角色可删除。

**响应：**

```json
{ "status": "deleted" }
```

---

### 5.7 列出可用运行时

```
GET /api/agents/runtimes
```

**响应：**

```json
{
  "runtimes": [
    { "id": "claude", "name": "Claude Code", "description": "AI programming assistant" },
    { "id": "echo", "name": "Echo", "description": "Simple echo backend for testing" }
  ]
}
```

---

## 6. 智能体调度与队列

### 6.1 队列项模型

```json
{
  "id": "queue-uuid",
  "task_id": "task-uuid",
  "agent_profile_id": "profile-uuid",
  "status": "queued | claimed | processing | completed | failed",
  "trigger_type": "status_change | mention | sub_task",
  "metadata": { "comment_id": "uuid", "comment_content": "..." },
  "assigned_at": "datetime",
  "claimed_at": "datetime | null",
  "completed_at": "datetime | null",
  "result_summary": "处理结果",
  "snapshot": {},
  "created_at": "datetime"
}
```

### 6.2 查询队列

```
GET /api/agents/queue?workspace_id=<workspace_id>&agent_profile_id=<id>&status=<status>
```

**响应：**

```json
{ "queue": [ /* TaskAgentQueue 对象数组 */ ] }
```

---

### 6.3 自动分配智能体

```
POST /api/agents/auto-assign/:taskId?workspace_id=<workspace_id>
```

**请求体：**

```json
{
  "required_tags": ["前端"],
  "exclude_ids": ["不想分配的智能体 ID"]
}
```

**响应：**

```json
{
  "assigned": true,
  "agent_profile_id": "assigned-agent-id",
  "queue_id": "created-queue-id",
  "agent_name": "前端程序员"
}
```

---

### 6.4 认领队列项

```
POST /api/agents/queue/:id/claim
```

**响应：**

```json
{ "status": "claimed" }
```

---

### 6.5 更新队列状态

```
PUT /api/agents/queue/:id/status
```

**请求体：**

```json
{
  "status": "processing",
  "result_summary": "处理结果描述（可选）"
}
```

**合法状态流转：** `queued → claimed → processing → completed/failed`

**响应：**

```json
{ "status": "updated" }
```

---

### 6.6 查询智能体负载

```
GET /api/agents/queue/agents?workspace_id=<workspace_id>
```

**响应：**

```json
{
  "agents": [
    { "id": "profile-uuid", "name": "前端程序员", "avatar": "🤖", "description": "负责前端开发", "current_load": 2, "max_concurrency": 5, "available": true }
  ]
}
```

---

## 7. 节点管理

### 7.1 节点模型

```json
{
  "id": "uuid",
  "user_id": "uuid",
  "name": "节点名称",
  "os": "linux | windows | darwin",
  "arch": "amd64 | arm64",
  "status": "online | offline",
  "version": "1.0.0",
  "ip": "192.168.1.1",
  "max_sessions": 5,
  "last_seen": "datetime",
  "created_at": "datetime"
}
```

### 7.2 列出节点

```
GET /api/nodes
```

**响应：**

```json
{ "nodes": [ /* Node 对象数组 */ ] }
```

---

### 7.3 获取节点

```
GET /api/nodes/:id
```

**响应：** Node 对象

---

### 7.4 注册节点

```
POST /api/nodes/register
```

**请求体：**

```json
{
  "name": "节点名称（必填）",
  "os": "linux",
  "arch": "amd64",
  "version": "1.0.0"
}
```

**响应（201）：** Node 对象

---

### 7.5 心跳

```
POST /api/nodes/heartbeat
```

**请求体：**

```json
{ "node_id": "uuid（必填）" }
```

**响应：**

```json
{ "status": "ok" }
```

---

### 7.6 生成节点 Token

```
POST /api/nodes/token
```

**请求体：**

```json
{ "node_id": "uuid（必填）" }
```

**说明：** 生成一个 15 分钟有效的临时 Token。

**响应：**

```json
{ "token": "temporary-token-string", "expires_in": 900 }
```

---

### 7.7 删除节点

```
DELETE /api/nodes/:id
```

**响应：**

```json
{ "status": "deleted" }
```

---

### 7.8 列出节点上的 Agent

```
GET /api/nodes/:id/agents
```

**响应：**

```json
{
  "agents": [
    { "id": "agent-runtime-id", "name": "claude", "enabled": true, "version": "1.0.0" }
  ]
}
```

---

### 7.9 扫描节点

```
POST /api/nodes/:id/scan
```

**说明：** 触发节点扫描，发现可用的 Agent 运行时。

**响应：**

```json
{ "status": "scanning" }
```

---

### 7.10 启动节点

```
POST /api/nodes/:id/start
```

**响应：**

```json
{ "status": "started" }
```

---

### 7.11 停止节点

```
POST /api/nodes/:id/stop
```

**响应：**

```json
{ "status": "stopped" }
```

---

### 7.12 启用/禁用 Agent

```
PATCH /api/agents/:id
```

**请求体：**

```json
{ "enabled": false }
```

**响应：**

```json
{ "status": "updated" }
```

---

## 8. 运行时节点 API（Node Agent）

> 这些接口供 Agent 运行时进程调用，认证方式为 `node_secret` + `node_id` 查询参数，而非 JWT。适用于外部 Agent 进程轮询和交互。

### 8.1 拉取待处理队列

```
GET /api/node/queue?node_id=<node_id>&node_secret=<secret>
```

**响应：**

```json
{
  "queue": [
    {
      "id": "queue-uuid",
      "task_id": "task-uuid",
      "agent_profile_id": "profile-uuid",
      "status": "queued",
      "trigger_type": "mention",
      "agent_name": "前端程序员",
      "snapshot": {},
      "created_at": "datetime"
    }
  ]
}
```

---

### 8.2 认领队列项（节点侧）

```
POST /api/node/queue/:id/claim?node_id=<node_id>&node_secret=<secret>
```

**响应：**

```json
{ "status": "claimed" }
```

---

### 8.3 更新队列状态（节点侧）

```
PUT /api/node/queue/:id/status?node_id=<node_id>&node_secret=<secret>
```

**请求体：**

```json
{
  "status": "completed",
  "result_summary": "任务已完成，输出内容..."
}
```

**说明：** 状态设为 `completed` 时，服务端会自动：
- 在任务下发布 Agent 评论（附 `result_summary`）
- 根据任务的 `completion_behavior` 决定目标状态：
  - `auto_done` → 设为 `done`，并触发 DAGEngine 推进依赖任务
  - 其他值（含 `auto_review`/`sample_review`/`needs_review`）→ 设为 `review`
- 如果目标状态为 `review` 且负责人是其他智能体，自动创建 review 队列条目

---

### 8.4 获取任务详情（节点侧）

```
GET /api/node/tasks/:id?node_id=<node_id>&node_secret=<secret>
```

**响应：**

```json
{
  "task": {
    "id": "task-uuid",
    "title": "任务标题",
    "description": "任务描述",
    "status": "in_progress"
  }
}
```

---

### 8.5 创建会话（节点侧）

```
POST /api/node/sessions?node_id=<node_id>&node_secret=<secret>
```

**请求体：**

```json
{
  "task_id": "task-uuid",
  "agent_id": "claude",
  "queue_id": "queue-uuid（可选，用于完成回调）"
}
```

**响应：**

```json
{
  "session_id": "session-uuid",
  "status": "pending",
  "prompt": "Task: 标题\n\nDescription: 描述\n\nPlease work on this task."
}
```

---

### 8.6 发布 Agent 评论（节点侧）

```
POST /api/node/tasks/:id/comments?node_id=<node_id>&node_secret=<secret>
```

**请求体：**

```json
{
  "content": "评论内容，支持 HTML 格式",
  "agent_profile_id": "agent-profile-uuid",
  "queue_id": "queue-uuid（可选，关联的队列项）"
}
```

**响应（201）：** 创建的评论对象

---

### 8.7 上报 Token 用量

```
POST /api/node/token-usage?node_id=<node_id>&node_secret=<secret>
```

**请求体：**

```json
{
  "task_id": "task-uuid（必填）",
  "agent_profile_id": "profile-uuid（必填）",
  "session_id": "session-uuid（可选）",
  "prompt_tokens": 1000,
  "completion_tokens": 500,
  "total_tokens": 1500,
  "stage": "work"
}
```

**说明：** 智能体在处理任务期间上报 Token 消耗。`stage` 可选值：`work`、`review`、`evaluate`，默认为 `work`。上报后自动累加到关联工作流的 `tokens_used` 计数。

**响应：**

```json
{ "status": "recorded" }
```

---

### 8.8 工具调用（节点侧）

```
POST /api/node/tool-call?node_id=<node_id>&node_secret=<secret>
```

**请求体：**

```json
{
  "task_id": "task-uuid（必填）",
  "queue_id": "queue-uuid（可选）",
  "tool": "create_sub_task（必填）",
  "params": { "title": "子任务标题", "depends_on": ["前置任务ID"] },
  "call_id": "optional-call-id",
  "agent_profile_id": "profile-uuid"
}
```

**说明：** 运行时调用此接口执行工具。服务端通过 Harness Policy Engine 进行权限检查、执行注册的 Executor，并记录审计日志。支持的工具有：`create_sub_task`、`assign_task`、`review_task`、`add_comment`、`get_task_detail`、`list_sub_tasks`、`update_task_status`。

**响应：**

```json
{
  "type": "tool_result",
  "tool": "create_sub_task",
  "status": "success",
  "result": { "task_id": "created-task-uuid" }
}
```

**错误响应：**

```json
{
  "type": "tool_result",
  "tool": "create_sub_task",
  "status": "denied",
  "error": {
    "code": "permission_denied",
    "message": "Agent lacks permission: task.write",
    "suggestion": "请更新智能体的 permissions 配置"
  }
}
```

---

## 9. 技能库（Skills）

### 9.1 技能模型

```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "name": "技能名称",
  "description": "描述",
  "content": "技能内容/提示词",
  "tags": ["前端", "React"],
  "source_task_id": "来源任务 ID",
  "source_agent_id": "来源智能体 ID",
  "usage_count": 5,
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### 9.2 列出技能

```
GET /api/skills?workspace_id=<workspace_id>&tag=<tag>
```

**响应：**

```json
{ "skills": [ /* Skill 对象数组 */ ] }
```

---

### 9.3 获取技能

```
GET /api/skills/:id
```

**响应：** Skill 对象

---

### 9.4 创建技能

```
POST /api/skills?workspace_id=<workspace_id>
```

**请求体：**

```json
{
  "name": "技能名称（必填）",
  "description": "描述",
  "content": "技能内容（必填）",
  "tags": ["前端", "React"]
}
```

**响应（201）：**

```json
{ "id": "new-uuid", "status": "created" }
```

---

### 9.5 更新技能

```
PUT /api/skills/:id
```

**请求体（全部可选）：**

```json
{
  "name": "新名称",
  "description": "新描述",
  "content": "新内容",
  "tags": ["新标签"]
}
```

**响应：**

```json
{ "status": "updated" }
```

---

### 9.6 删除技能

```
DELETE /api/skills/:id
```

**响应：**

```json
{ "status": "deleted" }
```

---

### 9.7 从任务提取技能

```
POST /api/skills/extract-from-task
```

**请求体：**

```json
{
  "task_id": "task-uuid（必填）",
  "agent_profile_id": "agent-profile-uuid（必填）"
}
```

**说明：** 从任务的完成结果中提取知识作为技能保存。

**响应（201）：**

```json
{ "id": "new-skill-uuid", "status": "created" }
```

---

## 10. 任务规则引擎

### 10.1 规则模型

```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "name": "规则名称",
  "trigger_type": "on_task_create | on_status_change | on_assignee_change | on_comment",
  "conditions": [{"field": "comment_content", "op": "contains", "value": "@urgent"}],
  "actions": [{"type": "set_priority", "value": "urgent"}],
  "enabled": true,
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### 10.2 列出规则

```
GET /api/rules?workspace_id=<workspace_id>
```

**响应：**

```json
{ "rules": [ /* Rule 对象数组 */ ] }
```

---

### 10.3 创建规则

```
POST /api/rules?workspace_id=<workspace_id>
```

**请求体：**

```json
{
  "name": "规则名称（必填）",
  "trigger_type": "on_comment（必填）",
  "conditions": [{"field": "comment_content", "op": "contains", "value": "@urgent"}],
  "actions": [{"type": "set_priority", "value": "urgent"}],
  "enabled": true
}
```

**触发类型可选值：** `on_task_create`、`on_status_change`、`on_assignee_change`、`on_comment`

**条件操作符可选值：** `equals`、`contains`、`matches`（正则）、`exists`、`not_exists`

**动作类型可选值：** `set_status`、`set_priority`、`set_assignee`、`add_tag`、`remove_tag`、`notify`、`comment`

**响应（201）：**

```json
{ "id": "new-uuid", "status": "created" }
```

---

### 10.4 获取规则

```
GET /api/rules/:id
```

**响应：** Rule 对象

---

### 10.5 更新规则

```
PUT /api/rules/:id
```

**请求体（全部可选）：**

```json
{
  "name": "新名称",
  "trigger_type": "on_status_change",
  "conditions": [{"field": "status", "op": "equals", "value": "done"}],
  "actions": [{"type": "notify", "value": "task completed"}],
  "enabled": false
}
```

**响应：**

```json
{ "status": "updated" }
```

---

### 10.6 删除规则

```
DELETE /api/rules/:id
```

**响应：**

```json
{ "status": "deleted" }
```

---

### 10.7 查看规则执行日志

```
GET /api/rules/:id/logs
```

**响应：**

```json
{
  "logs": [
    {
      "id": "uuid",
      "rule_id": "uuid",
      "task_id": "uuid",
      "trigger_type": "on_comment",
      "matched": true,
      "result": "actions executed",
      "exec_log": "details",
      "created_at": "datetime"
    }
  ]
}
```

---

## 11. 通知系统

### 11.1 通知模型

```json
{
  "id": "uuid",
  "user_id": "uuid",
  "type": "task_assigned | task_updated | comment_added | mention | ...",
  "title": "通知标题",
  "message": "通知内容",
  "task_id": "uuid | null",
  "is_read": false,
  "created_at": "datetime"
}
```

### 11.2 列出通知

```
GET /api/notifications?before=<notification_id>
```

**说明：** 支持游标分页，`before` 参数为上一页最后一条通知的 ID。

**响应：**

```json
{ "notifications": [ /* AppNotification 对象数组 */ ] }
```

---

### 11.3 获取未读数

```
GET /api/notifications/unread-count
```

**响应：**

```json
{ "count": 5 }
```

---

### 11.4 标记已读

```
PATCH /api/notifications/:id/read
```

**响应：**

```json
{ "status": "ok" }
```

---

### 11.5 全部标记已读

```
PATCH /api/notifications/read-all
```

**响应：**

```json
{ "status": "ok" }
```

---

### 11.6 删除通知

```
DELETE /api/notifications/:id
```

**响应：**

```json
{ "status": "deleted" }
```

---

## 12. 会话管理

### 12.1 会话模型

```json
{
  "id": "uuid",
  "node_id": "uuid",
  "user_id": "uuid",
  "agent_id": "claude",
  "task_id": "uuid | null",
  "status": "pending | running | completed | failed",
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### 12.2 创建会话

```
POST /api/sessions
```

**请求体：**

```json
{
  "node_id": "节点 ID（必填）",
  "agent_id": "claude（必填）",
  "task_id": "关联任务 ID（可选）"
}
```

**说明：** 创建后会通过 MessageBus 通知运行时加入会话。

**响应（201）：**

```json
{
  "session_id": "uuid",
  "status": "pending"
}
```

---

### 12.3 列出会话

```
GET /api/sessions
```

**响应：**

```json
{ "sessions": [ /* Session 对象数组 */ ] }
```

---

### 12.4 获取会话

```
GET /api/sessions/:id
```

**响应：** Session 对象

---

### 12.5 获取会话消息

```
GET /api/sessions/:id/messages
```

**响应：**

```json
{
  "messages": [
    {
      "id": "uuid",
      "session_id": "uuid",
      "sender": "system://api | runtime://nodeId",
      "content": "消息内容",
      "created_at": "datetime"
    }
  ]
}
```

---

## 13. 插件管理

### 13.1 插件模型

```json
{
  "name": "plugin-name",
  "version": "1.0.0",
  "type": "builtin | external",
  "state": "registered | running | stopped | error",
  "label": { "zh_CN": "插件名称", "en_US": "Plugin Name" },
  "description": { "zh_CN": "插件描述", "en_US": "Plugin Description" },
  "author": "author-name",
  "pid": 12345,
  "port": 9090,
  "error": "错误信息（如有）",
  "permissions": ["read_tasks", "write_tasks"],
  "hooks": ["on_task_create", "on_comment"],
  "api_routes": ["/api/plugin/xx"],
  "frontend_slots": { "slot_name": "ComponentName" },
  "uptime_seconds": 3600
}
```

### 13.2 列出插件

```
GET /api/plugins
```

**认证：** JWT

**响应：**

```json
{ "plugins": [ /* PluginView 对象数组 */ ] }
```

---

### 13.3 获取插件详情

```
GET /api/plugins/:name
```

**认证：** JWT

**响应：**

```json
{ "plugin": { /* Plugin 完整信息 */ } }
```

---

### 13.4 启动插件

```
POST /api/plugins/:name/start
```

**认证：** JWT

**响应：**

```json
{ "status": "started", "plugin": "plugin-name" }
```

---

### 13.5 停止插件

```
POST /api/plugins/:name/stop
```

**认证：** JWT

**响应：**

```json
{ "status": "stopped", "plugin": "plugin-name" }
```

---

### 13.6 移除插件

```
POST /api/plugins/:name/remove
```

**认证：** JWT

**说明：** 从插件管理器和磁盘上删除插件。

**响应：**

```json
{ "status": "removed", "plugin": "plugin-name" }
```

---

### 13.7 重新加载插件

```
POST /api/plugins/:name/reload
```

**认证：** JWT

**说明：** 停止并重新注册启动插件。

**响应：**

```json
{ "status": "reloaded", "plugin": "plugin-name" }
```

---

### 13.8 插件健康检查

```
GET /api/plugins/:name/health
```

**认证：** JWT

**响应：**

```json
{ "health": { "status": "ok", "uptime_seconds": 3600 } }
```

---

### 13.9 上传安装插件

```
POST /api/plugins/install/upload
```

**认证：** JWT

**说明：** 通过上传 ZIP 包安装插件。

**请求体：** `multipart/form-data`，包含插件 ZIP 文件。

**响应（201）：**

```json
{ "status": "installed", "plugin": "plugin-name" }
```

---

### 13.10 从 Git 安装插件

```
POST /api/plugins/install/git
```

**认证：** JWT

**请求体：**

```json
{
  "url": "https://github.com/user/plugin-repo.git",
  "branch": "main（可选）"
}
```

**响应（201）：**

```json
{ "status": "installed", "plugin": "plugin-name" }
```

---

### 13.11 插件宿主内部 API（供插件内部使用）

> 这些接口仅供插件运行时内部调用，端点位于 `/api/__plugin_host/*`。

| 操作 | 方法 | 路径 | 说明 |
|------|------|------|------|
| 查询任务 | `GET` | `/api/__plugin_host/tasks` | 查询任务列表 |
| 查询项目 | `GET` | `/api/__plugin_host/projects` | 查询项目列表 |
| 创建任务 | `POST` | `/api/__plugin_host/tasks` | 创建新任务 |
| 更新任务 | `PUT` | `/api/__plugin_host/tasks/:id` | 更新任务 |
| 删除任务 | `DELETE` | `/api/__plugin_host/tasks/:id` | 删除任务 |
| 发送消息 | `POST` | `/api/__plugin_host/message` | 发送系统消息 |
| 检查权限 | `GET` | `/api/__plugin_host/permission` | 检查操作权限 |
| 写入日志 | `POST` | `/api/__plugin_host/log` | 写入日志 |
| 读取 KV | `GET` | `/api/__plugin_host/kv/:key` | 读取键值存储 |
| 写入 KV | `POST` | `/api/__plugin_host/kv/:key` | 写入键值存储 |
| 删除 KV | `DELETE` | `/api/__plugin_host/kv/:key` | 删除键值存储 |

---

## 14. 工作流管理

### 14.1 工作流模型

```json
{
  "id": "uuid",
  "title": "工作流标题",
  "description": "描述",
  "status": "active | paused | done | stuck",
  "token_budget": 100000,
  "tokens_used": 5000,
  "created_by": "uuid",
  "workspace_id": "uuid",
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### 14.2 列出工作流

```
GET /api/workflows?workspace_id=<workspace_id>
```

**响应：**

```json
{
  "workflows": [
    {
      "id": "uuid",
      "title": "示例工作流",
      "status": "active",
      "token_budget": 100000,
      "tokens_used": 5000
    }
  ]
}
```

---

### 14.3 创建工作流

```
POST /api/workflows?workspace_id=<workspace_id>
```

**请求体：**

```json
{
  "title": "工作流标题（必填）",
  "description": "描述",
  "token_budget": 100000
}
```

**响应（201）：**

```json
{ "id": "uuid", "status": "active" }
```

---

### 14.4 获取工作流详情

```
GET /api/workflows/:id
```

**响应：**

```json
{
  "workflow": { /* Workflow 对象 */ },
  "task_summary": [
    { "status": "todo", "count": 5 },
    { "status": "done", "count": 3 }
  ]
}
```

---

### 14.5 更新工作流状态

```
PATCH /api/workflows/:id/status
```

**请求体：**

```json
{ "status": "paused" }
```

**合法值：** `active`、`paused`、`done`、`stuck`

**响应：**

```json
{ "status": "paused" }
```

---

### 14.6 列出工作流任务

```
GET /api/workflows/:id/tasks
```

**响应：**

```json
{
  "tasks": [
    {
      "id": "uuid",
      "title": "任务标题",
      "status": "done",
      "depth": 0,
      "completion_behavior": "auto_done",
      "assignee_id": "uuid",
      "assignee_type": "agent_profile",
      "agent_loop_count": 0,
      "max_agent_loops": 3,
      "parallel_group": null,
      "dependencies": ["dep-task-id"],
      "created_at": "datetime"
    }
  ]
}
```

---

### 14.7 将任务附加到工作流

```
POST /api/workflows/attach
```

**请求体：**

```json
{
  "task_id": "task-uuid（必填）",
  "workflow_id": "workflow-uuid（必填）",
  "depends_on": ["前置任务 ID"],
  "depth": 0
}
```

**说明：** 如果指定了 `depends_on`，系统会检查是否会产生循环依赖（DAG 合法性校验）。

**响应：**

```json
{ "status": "attached", "task_id": "uuid", "workflow_id": "uuid" }
```

---

### 14.8 DAG 自动推进

DAGEngine 在工作流执行过程中自动管理任务依赖：

**自动解阻塞：** 当某个任务完成时，DAGEngine 查找所有依赖该任务的任务（`status=blocked`），逐一检查其所有前置依赖是否已完成。若全部完成，自动将状态更新为 `todo`。

**自动派发：** 解阻塞时，如果任务的 `assignee_type=agent_profile` 且智能体有空闲容量，自动创建 `task_agent_queue` 条目（`trigger_type=status_change`）并递增负载计数。

**子任务自动入队：** 使用 `create_sub_task` 创建子任务时，如果 `assignee_type=agent_profile` 且未设置 `depends_on`（无前置依赖），系统自动创建队列条目（`trigger_type=sub_task`），无需等待 DAG 解阻塞。

**自动关闭父任务：** 解阻塞后检查当前任务的父任务的所有子任务是否均已 `done`。若是，自动将父任务设为 `done`，并递归调用 DAGEngine 继续推进父任务的依赖链。

**入口：** DAGEngine.OnTaskCompleted(taskID) 在以下场景被调用：
- 节点侧队列项完成且 `completion_behavior=auto_done`（见 8.3）
- 审核通过（见 2.18）

---

## 15. Harness 智能体工具调用

> Harness 是智能体在工作流中调用系统功能的权限安全层。智能体通过在回复中嵌入 JSON 格式的 `tool_call` 来调用以下工具。

### 15.1 工具列表

| 工具 | 描述 | 所需权限 | 所需能力 |
|------|------|---------|---------|
| `create_sub_task` | 在当前工作流下创建子任务，支持设置依赖关系和并行分组 | `task.write` | `create_sub_task` |
| `assign_task` | 分配任务给用户或智能体 | `task.assign` | `assign_task` |
| `review_task` | 审核已完成的任务，批准或打回 | `task.review` | `review_task` |
| `add_comment` | 在任务下添加评论 | `comment.write` | `add_comment` |
| `get_task_detail` | 查看任务详情（含依赖关系） | `task.read` | `get_task_detail` |
| `list_sub_tasks` | 列出任务的子任务列表 | `task.read` | `list_sub_tasks` |
| `update_task_status` | 更新任务状态（仅工作流内有效） | `task.write` | `update_task_status` |

### 15.2 Tool Call 格式

智能体在回复中输出以下 JSON 块：

```json
{"type": "tool_call", "tool": "create_sub_task", "params": {"title": "编写API文档"}, "id": "optional-call-id"}
```

### 15.3 工具调用流程

```
智能体回复 → 运行时提取 tool_call JSON → Harness.HandleToolCall()
  ├─ 1. Policy Engine 权限检查
  │   ├─ 有权限 → 继续执行
  │   └─ 无权限 → 返回 denied + 审计日志
  ├─ 2. 执行注册的 Executor
  │   ├─ create_sub_task → 创建任务 + DAG 循环检测
  │   ├─ assign_task → 更新负责人
  │   └─ ...
  └─ 3. Auditor 审计日志记录
```

### 15.4 各工具参数

**create_sub_task:**

```json
{
  "title": "子任务标题（必填，最长200字符）",
  "description": "任务描述",
  "depends_on": ["前置任务ID"],
  "parallel_group": "并行分组名称",
  "assignee_id": "负责人ID",
  "assignee_type": "user | agent_profile",
  "completion_behavior": "auto_done | auto_review | sample_review | needs_review"
}
```

**说明：** 创建的子任务自动继承父任务的 `workspace_id`。如果 `assignee_type=agent_profile` 且未指定 `depends_on`，系统自动将子任务加入智能体队列（`trigger_type=sub_task`），智能体将立即开始处理。

**assign_task:**

```json
{
  "task_id": "任务ID（必填）",
  "assignee_id": "负责人ID（必填）",
  "assignee_type": "user | agent_profile（必填）"
}
```

**review_task:**

```json
{
  "task_id": "任务ID（必填）",
  "action": "approved | rejected（必填）",
  "comment": "审核意见"
}
```

** add_comment:**

```json
{
  "task_id": "任务ID（必填）",
  "content": "评论内容（必填，最长10000字符）"
}
```

**get_task_detail:**

```json
{
  "task_id": "任务ID（必填）"
}
```

**list_sub_tasks:**

```json
{
  "task_id": "任务ID（必填）"
}
```

**update_task_status:**

```json
{
  "task_id": "任务ID（必填）",
  "status": "todo | in_progress | completed | blocked（必填）"
}
```

### 15.5 错误码

| 错误码 | 说明 |
|--------|------|
| `schema_invalid` | 参数格式错误 |
| `tool_not_found` | 工具名称不存在 |
| `permission_denied` | 权限不足 |
| `depth_exceeded` | 超过工作流最大深度 |
| `loop_exceeded` | 超过最大审核轮次 |
| `budget_exceeded` | 超过 Token 预算 |
| `internal_error` | 服务端错误 |

---

## 16. 辅助接口

### 16.1 健康检查

```
GET /api/health
```

**认证：** 公开

**响应：**

```json
{ "status": "ok" }
```

---

### 16.2 WebSocket 仪表盘

```
GET /ws/dashboard
```

**认证：** WebSocket 连接

**说明：** 连接后服务端会推送实时变更事件（`tasks`、`projects`、`workspaces`、`notifications` 等）。

---

### 16.3 WebSocket 消息总线

```
GET /ws/bus
```

**认证：** WebSocket 连接

**说明：** 连接后接收 MessageBus 的系统消息。

---

### 16.4 下载节点安装脚本（Linux）

```
GET /api/nodes/install.sh
```

**认证：** 公开

**说明：** 返回 Linux 节点的安装 Shell 脚本。

---

### 16.5 下载节点安装脚本（Windows）

```
GET /api/nodes/install.ps1
```

**认证：** 公开

**说明：** 返回 Windows 节点的安装 PowerShell 脚本。

---

### 16.6 下载节点二进制

```
GET /api/nodes/bin/:os/:arch
```

**认证：** 公开

**说明：** 下载指定操作系统和架构的节点运行时二进制文件。`:os` 可选 `linux`/`windows`/`darwin`，`:arch` 可选 `amd64`/`arm64`。

---

## 17. 接口一览表

| # | 操作 | 方法 | 路径 | 认证 |
|---|------|------|------|------|
| 1.1 | 用户登录 | `POST` | `/api/auth/login` | 公开 |
| 1.2 | 用户注册 | `POST` | `/api/auth/register` | 公开 |
| 1.3 | 用户列表 | `GET` | `/api/users` | JWT（admin/owner） |
| 1.4 | 删除用户 | `DELETE` | `/api/users/:id` | JWT（admin/owner） |
| 2.2 | 列出任务 | `GET` | `/api/tasks` | JWT |
| 2.3 | 创建任务 | `POST` | `/api/tasks` | JWT |
| 2.4 | 获取任务 | `GET` | `/api/tasks/:id` | JWT |
| 2.5 | 更新任务 | `PUT` | `/api/tasks/:id` | JWT |
| 2.6 | 删除任务 | `DELETE` | `/api/tasks/:id` | JWT |
| 2.7 | 永久删除任务 | `DELETE` | `/api/tasks/:id/force` | JWT |
| 2.8 | 恢复任务 | `POST` | `/api/tasks/:id/restore` | JWT |
| 2.9 | 列出回收站 | `GET` | `/api/tasks/trash` | JWT |
| 2.10 | 设置任务状态 | `PATCH` | `/api/tasks/:id/status` | JWT |
| 2.11 | 添加受托人 | `POST` | `/api/tasks/:id/assignees` | JWT |
| 2.12 | 移除受托人 | `DELETE` | `/api/tasks/:id/assignees/:assigneeId` | JWT |
| 2.13 | 列出受托人 | `GET` | `/api/tasks/:id/assignees` | JWT |
| 2.14 | 列出子任务 | `GET` | `/api/tasks/:id/subtasks` | JWT |
| 2.15 | 列出评论 | `GET` | `/api/tasks/:id/comments` | JWT |
| 2.16 | 创建评论 | `POST` | `/api/tasks/:id/comments` | JWT |
| 2.17 | 删除评论 | `DELETE` | `/api/tasks/:id/comments/:commentId` | JWT |
| 2.18 | 审核任务 | `POST` | `/api/tasks/:id/review` | JWT |
| 3.2 | 列出项目 | `GET` | `/api/projects` | JWT |
| 3.3 | 创建项目 | `POST` | `/api/projects` | JWT |
| 3.4 | 获取项目 | `GET` | `/api/projects/:id` | JWT |
| 3.5 | 更新项目 | `PUT` | `/api/projects/:id` | JWT |
| 3.6 | 删除项目 | `DELETE` | `/api/projects/:id` | JWT |
| 3.7 | 永久删除项目 | `DELETE` | `/api/projects/:id/force` | JWT |
| 3.8 | 恢复项目 | `POST` | `/api/projects/:id/restore` | JWT |
| 3.9 | 项目回收站 | `GET` | `/api/projects/trash` | JWT |
| 4.2 | 列出工作区 | `GET` | `/api/workspaces` | JWT |
| 4.3 | 创建工作区 | `POST` | `/api/workspaces` | JWT |
| 4.4 | 获取工作区 | `GET` | `/api/workspaces/:id` | JWT |
| 4.5 | 更新工作区 | `PUT` | `/api/workspaces/:id` | JWT |
| 4.6 | 删除工作区 | `DELETE` | `/api/workspaces/:id` | JWT |
| 4.7 | 列出成员 | `GET` | `/api/workspaces/:id/members` | JWT |
| 4.8 | 添加成员 | `POST` | `/api/workspaces/:id/members` | JWT |
| 4.9 | 更新成员角色 | `PUT` | `/api/workspaces/:id/members/:userId` | JWT |
| 4.10 | 移除成员 | `DELETE` | `/api/workspaces/:id/members/:userId` | JWT |
| 4.11 | 创建邀请 | `POST` | `/api/workspaces/:id/invitations` | JWT |
| 4.12 | 列出邀请 | `GET` | `/api/workspaces/:id/invitations` | JWT |
| 4.13 | 取消邀请 | `DELETE` | `/api/workspaces/:id/invitations/:invitationId` | JWT |
| 4.14 | 获取邀请信息 | `GET` | `/api/invitations/:token` | 公开 |
| 4.15 | 接受邀请 | `POST` | `/api/invitations/:token/accept` | JWT |
| 4.16 | 拒绝邀请 | `POST` | `/api/invitations/:token/decline` | 公开 |
| 4.17 | 待处理邀请 | `GET` | `/api/invitations/pending` | JWT |
| 5.2 | 列出智能体 | `GET` | `/api/agents/profiles` | JWT |
| 5.3 | 查询智能体 | `GET` | `/api/agents/profiles/:id` | JWT |
| 5.4 | 创建智能体 | `POST` | `/api/agents/profiles` | JWT |
| 5.5 | 更新智能体 | `PUT` | `/api/agents/profiles/:id` | JWT |
| 5.6 | 删除智能体 | `DELETE` | `/api/agents/profiles/:id` | JWT |
| 5.7 | 列出运行时 | `GET` | `/api/agents/runtimes` | JWT |
| 6.2 | 查询队列 | `GET` | `/api/agents/queue` | JWT |
| 6.3 | 自动分配 | `POST` | `/api/agents/auto-assign/:taskId` | JWT |
| 6.4 | 认领队列项 | `POST` | `/api/agents/queue/:id/claim` | JWT |
| 6.5 | 更新队列状态 | `PUT` | `/api/agents/queue/:id/status` | JWT |
| 6.6 | 查询负载 | `GET` | `/api/agents/queue/agents` | JWT |
| 7.2 | 列出节点 | `GET` | `/api/nodes` | JWT |
| 7.3 | 获取节点 | `GET` | `/api/nodes/:id` | JWT |
| 7.4 | 注册节点 | `POST` | `/api/nodes/register` | JWT |
| 7.5 | 心跳 | `POST` | `/api/nodes/heartbeat` | JWT |
| 7.6 | 生成 Token | `POST` | `/api/nodes/token` | JWT |
| 7.7 | 删除节点 | `DELETE` | `/api/nodes/:id` | JWT |
| 7.8 | 列出节点 Agent | `GET` | `/api/nodes/:id/agents` | JWT |
| 7.9 | 扫描节点 | `POST` | `/api/nodes/:id/scan` | JWT |
| 7.10 | 启动节点 | `POST` | `/api/nodes/:id/start` | JWT |
| 7.11 | 停止节点 | `POST` | `/api/nodes/:id/stop` | JWT |
| 7.12 | 启用/禁用 Agent | `PATCH` | `/api/agents/:id` | JWT |
| 8.1 | 拉取队列（节点） | `GET` | `/api/node/queue` | node_secret |
| 8.2 | 认领（节点） | `POST` | `/api/node/queue/:id/claim` | node_secret |
| 8.3 | 更新状态（节点） | `PUT` | `/api/node/queue/:id/status` | node_secret |
| 8.4 | 获取任务（节点） | `GET` | `/api/node/tasks/:id` | node_secret |
| 8.5 | 创建会话（节点） | `POST` | `/api/node/sessions` | node_secret |
| 8.6 | 发布评论（节点） | `POST` | `/api/node/tasks/:id/comments` | node_secret |
| 8.7 | 上报 Token 用量 | `POST` | `/api/node/token-usage` | node_secret |
| 8.8 | 工具调用 | `POST` | `/api/node/tool-call` | node_secret |
| 9.2 | 列出技能 | `GET` | `/api/skills` | JWT |
| 9.3 | 获取技能 | `GET` | `/api/skills/:id` | JWT |
| 9.4 | 创建技能 | `POST` | `/api/skills` | JWT |
| 9.5 | 更新技能 | `PUT` | `/api/skills/:id` | JWT |
| 9.6 | 删除技能 | `DELETE` | `/api/skills/:id` | JWT |
| 9.7 | 从任务提取技能 | `POST` | `/api/skills/extract-from-task` | JWT |
| 10.2 | 列出规则 | `GET` | `/api/rules` | JWT |
| 10.3 | 创建规则 | `POST` | `/api/rules` | JWT |
| 10.4 | 获取规则 | `GET` | `/api/rules/:id` | JWT |
| 10.5 | 更新规则 | `PUT` | `/api/rules/:id` | JWT |
| 10.6 | 删除规则 | `DELETE` | `/api/rules/:id` | JWT |
| 10.7 | 规则执行日志 | `GET` | `/api/rules/:id/logs` | JWT |
| 11.2 | 列出通知 | `GET` | `/api/notifications` | JWT |
| 11.3 | 未读数 | `GET` | `/api/notifications/unread-count` | JWT |
| 11.4 | 标记已读 | `PATCH` | `/api/notifications/:id/read` | JWT |
| 11.5 | 全部标记已读 | `PATCH` | `/api/notifications/read-all` | JWT |
| 11.6 | 删除通知 | `DELETE` | `/api/notifications/:id` | JWT |
| 12.2 | 创建会话 | `POST` | `/api/sessions` | JWT |
| 12.3 | 列出会话 | `GET` | `/api/sessions` | JWT |
| 12.4 | 获取会话 | `GET` | `/api/sessions/:id` | JWT |
| 12.5 | 获取会话消息 | `GET` | `/api/sessions/:id/messages` | JWT |
| 13.2 | 列出插件 | `GET` | `/api/plugins` | JWT |
| 13.3 | 获取插件详情 | `GET` | `/api/plugins/:name` | JWT |
| 13.4 | 启动插件 | `POST` | `/api/plugins/:name/start` | JWT |
| 13.5 | 停止插件 | `POST` | `/api/plugins/:name/stop` | JWT |
| 13.6 | 移除插件 | `POST` | `/api/plugins/:name/remove` | JWT |
| 13.7 | 重新加载插件 | `POST` | `/api/plugins/:name/reload` | JWT |
| 13.8 | 插件健康检查 | `GET` | `/api/plugins/:name/health` | JWT |
| 13.9 | 上传安装插件 | `POST` | `/api/plugins/install/upload` | JWT |
| 13.10 | 从 Git 安装插件 | `POST` | `/api/plugins/install/git` | JWT |
| 13.11 | 插件宿主 API 列表 | `GET/POST/PUT/DELETE` | `/api/__plugin_host/*` | 内部使用 |
| 14.1 | 列出工作流 | `GET` | `/api/workflows` | JWT |
| 14.2 | 创建工作流 | `POST` | `/api/workflows` | JWT |
| 14.3 | 获取工作流 | `GET` | `/api/workflows/:id` | JWT |
| 14.4 | 更新工作流状态 | `PATCH` | `/api/workflows/:id/status` | JWT |
| 14.5 | 列出工作流任务 | `GET` | `/api/workflows/:id/tasks` | JWT |
| 14.6 | 附加任务到工作流 | `POST` | `/api/workflows/attach` | JWT |
| 16.1 | 健康检查 | `GET` | `/api/health` | 公开 |
| 16.2 | WebSocket 仪表盘 | `GET` | `/ws/dashboard` | 公开 |
| 16.3 | WebSocket 消息总线 | `GET` | `/ws/bus` | 公开 |
| 16.4 | 安装脚本（Linux） | `GET` | `/api/nodes/install.sh` | 公开 |
| 16.5 | 安装脚本（Windows） | `GET` | `/api/nodes/install.ps1` | 公开 |
| 16.6 | 下载节点二进制 | `GET` | `/api/nodes/bin/:os/:arch` | 公开 |
