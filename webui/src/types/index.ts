// === Plugin Types ===
export interface PluginInfo {
  name: string;
  version: string;
  type: string;
  state: string;
  label?: Record<string, string>;
  description?: Record<string, string>;
  author?: string;
  pid: number;
  port: number;
  error?: string;
  permissions?: string[];
  hooks?: string[];
  api_routes?: string[];
  frontend_slots?: Record<string, string>;
  uptime_seconds?: number;
}

// === Agent Types ===
export interface Agent {
  id: string;
  node_id: string;
  name: string;
  command: string;
  version: string;
  enabled: boolean;
  auto_detected: boolean;
}

// === Agent Profile Types ===
export interface AgentProfile {
  id: string;
  user_id: string;
  name: string;
  avatar: string;
  description: string;
  system_prompt: string;
  instructions: string;
  agent_id: string;
  node_id?: string;
  version: string;
  model: string;
  backend: string;
  enabled: boolean;
  max_concurrency: number;
  current_load: number;
  tags: string[];
  skills: string[];
  last_active_at?: string;
  created_at: string;
  updated_at: string;
  // Harness fields
  protocol_version?: string;
  capabilities?: string[];
  permissions?: Record<string, unknown>;
  max_depth: number;
  max_review_loops: number;
  completion_behavior?: string;
  review_sample_rate: number;
  review_timeout: number;
}

export interface RuntimeEntity {
  id: string;
  name: string;
  description: string;
}

// === Node Types ===
export type NodeStatus = 'online' | 'offline' | 'busy';

export interface Node {
  id: string;
  user_id: string;
  name: string;
  os: string;
  arch: string;
  status: NodeStatus;
  version: string;
  ip: string;
  max_sessions: number;
  last_seen: string;
  created_at: string;
  agents?: Agent[];
  can_manage?: boolean;
}

// === Session Types ===
export type SessionStatus = 'pending' | 'running' | 'paused' | 'completed' | 'failed';

export interface Session {
  id: string;
  user_id: string;
  node_id: string;
  agent_id?: string;
  status: SessionStatus;
  prompt: string;
  workspace: string;
  output_log?: string;
  error_log?: string;
  pid?: number;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

export interface CreateSessionReq {
  prompt?: string;
  workspace: string;
  node_id: string;
  agent_id: string;
}

// === Project Types ===
export type ProjectStatus = 'planning' | 'active' | 'completed' | 'on_hold';

export interface Project {
  id: string;
  user_id: string;
  name: string;
  description: string;
  color: string;
  task_count: number;
  assignee_id?: string;
  assignee_type?: AssigneeType;
  status: ProjectStatus;
  started_at?: string;
  due_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateProjectReq {
  name: string;
  description?: string;
  color?: string;
  assignee_id?: string;
  assignee_type?: AssigneeType;
  status?: ProjectStatus;
  started_at?: string;
  due_at?: string;
}

export interface UpdateProjectReq {
  name?: string;
  description?: string;
  color?: string;
  assignee_id?: string | null;
  assignee_type?: AssigneeType | null;
  status?: ProjectStatus;
  started_at?: string | null;
  due_at?: string | null;
}

// === Task Types ===
export type TaskStatus = 'todo' | 'in_progress' | 'blocked' | 'completed' | 'done' | 'review' | 'stuck';
export type Priority = 'urgent' | 'high' | 'medium' | 'low';
export type AssigneeType = 'user' | 'agent_profile';
export type CompletionBehavior = 'auto_done' | 'auto_review' | 'sample_review' | 'needs_review';
export type ReviewAction = 'approved' | 'rejected';

export interface Task {
  id: string;
  user_id: string;
  creator_name?: string;
  title: string;
  description: string;
  status: TaskStatus;
  project_id?: string;
  parent_id?: string;
  assignee_id?: string;
  assignee_type?: AssigneeType;
  priority: Priority;
  tags: string[];
  assignees?: TaskAssignee[];
  due_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
  workflow_id?: string;
  depth?: number;
  max_depth?: number;
  max_agent_loops?: number;
  agent_loop_count?: number;
  completion_behavior?: CompletionBehavior;
  parallel_group?: string | null;
}

export interface CreateTaskReq {
  title: string;
  description?: string;
  project_id?: string;
  parent_id?: string;
  assignee_id?: string;
  assignee_type?: AssigneeType;
  priority?: Priority;
  tags?: string[];
  due_at?: string;
}

export interface UpdateTaskReq {
  title?: string;
  description?: string;
  status?: TaskStatus;
  project_id?: string | null;
  parent_id?: string | null;
  assignee_id?: string | null;
  assignee_type?: AssigneeType | null;
  priority?: Priority;
  tags?: string[];
  due_at?: string | null;
}

export interface TaskAssignee {
  task_id: string;
  assignee_id: string;
  assignee_type: AssigneeType;
  role: string;
}

export interface AddAssigneeReq {
  assignee_id: string;
  assignee_type: AssigneeType;
}

export interface ReviewTaskReq {
  action: ReviewAction;
  comment?: string;
  reviewer_agent_id?: string;
}

// === Workflow Types ===
export type WorkflowStatus = 'active' | 'paused' | 'done' | 'stuck';

export interface Workflow {
  id: string;
  title: string;
  description: string;
  status: WorkflowStatus;
  token_budget: number;
  tokens_used: number;
  created_by: string;
  workspace_id: string;
  created_at: string;
  updated_at: string;
}

export interface CreateWorkflowReq {
  title: string;
  description?: string;
  token_budget?: number;
}

export interface AttachToWorkflowReq {
  task_id: string;
  workflow_id: string;
  depends_on?: string[];
  depth?: number;
}

// === Workspace Types ===
export type WorkspaceRole = 'owner' | 'admin' | 'worker' | 'observer';

export interface Workspace {
  id: string;
  user_id: string;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
  role?: WorkspaceRole;
}

export interface CreateWorkspaceReq {
  name: string;
  description?: string;
}

export interface UpdateWorkspaceReq {
  name?: string;
  description?: string;
}

export interface WorkspaceMember {
  workspace_id: string;
  user_id: string;
  role: WorkspaceRole;
  joined_at: string;
  username: string;
}

export interface AddMemberReq {
  user_id: string;
  role: WorkspaceRole;
}

export interface UpdateMemberRoleReq {
  role: WorkspaceRole;
}

// === Invitation Types ===
export type InvitationStatus = 'pending' | 'accepted' | 'declined' | 'expired';

export interface PendingInvitation {
  id: string;
  workspace_id: string;
  inviter_id: string;
  invitee_email: string;
  token: string;
  role: WorkspaceRole;
  status: InvitationStatus;
  created_at: string;
  expires_at: string;
  workspace_name?: string;
  inviter_name?: string;
}

export interface InviteMemberReq {
  email: string;
  role: WorkspaceRole;
}

// === Auth Types ===
export interface AuthState {
  token: string | null;
  user: { id: string; username: string; email?: string } | null;
  workspace_id: string | null;
  workspace_role: WorkspaceRole | null;
}

// === Comment Types ===
export interface Comment {
  id: string;
  task_id: string;
  user_id: string;
  username: string;
  agent_profile_id?: string;
  agent_name?: string;
  agent_avatar?: string;
  content: string;
  parent_id?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateCommentReq {
  content: string;
  agent_profile_id?: string;
  parent_id?: string;
}

// === Notification Types ===
export type NotificationType = 'task_assigned' | 'task_status_changed' | 'task_comment' | 'task_mention';

export interface AppNotification {
  id: string;
  user_id: string;
  type: NotificationType;
  title: string;
  message: string;
  task_id?: string | null;
  is_read: boolean;
  created_at: string;
}

// === Skill Types ===
export interface Skill {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  content: string;
  tags: string[];
  source_task_id?: string;
  source_agent_id?: string;
  usage_count: number;
  created_at: string;
  updated_at: string;
}

export interface CreateSkillReq {
  name: string;
  description?: string;
  content: string;
  tags?: string[];
}

export interface ExtractSkillReq {
  task_id: string;
  agent_profile_id?: string;
}

// === Task Rule Types ===
export interface TaskRule {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  trigger_type: string;
  conditions: Record<string, unknown>;
  actions: Record<string, unknown>[];
  enabled: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface TaskRuleLog {
  id: string;
  rule_id: string;
  task_id: string;
  trigger_event: string;
  matched: boolean;
  result: string;
  log: string;
  created_at: string;
}

export interface CreateRuleReq {
  name: string;
  description?: string;
  trigger_type: string;
  conditions?: Record<string, unknown>;
  actions?: Record<string, unknown>[];
  enabled?: boolean;
}

export interface UpdateRuleReq {
  name?: string;
  description?: string;
  trigger_type?: string;
  conditions?: Record<string, unknown>;
  actions?: Record<string, unknown>[];
  enabled?: boolean;
}

// === Agent Queue Types ===
export interface AgentQueueItem {
  id: string;
  task_id: string;
  agent_profile_id: string;
  status: 'queued' | 'claimed' | 'processing' | 'completed' | 'failed';
  assigned_at?: string;
  claimed_at?: string;
  completed_at?: string;
  result_summary: string;
  snapshot?: Record<string, unknown>;
  created_at: string;
}

export interface AgentLoadInfo {
  id: string;
  name: string;
  avatar: string;
  description: string;
  max_concurrency: number;
  current_load: number;
  available: boolean;
}

// === User Management ===
export interface UserSummary {
  id: string;
  username: string;
  email: string;
  created_at: string;
}
