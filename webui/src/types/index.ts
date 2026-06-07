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
  agent_id: string;
  version: string;
  model: string;
  backend: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
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
export interface Project {
  id: string;
  user_id: string;
  name: string;
  description: string;
  color: string;
  task_count: number;
  created_at: string;
  updated_at: string;
}

export interface CreateProjectReq {
  name: string;
  description?: string;
  color?: string;
}

export interface UpdateProjectReq {
  name?: string;
  description?: string;
  color?: string;
}

// === Task Types ===
export type TaskStatus = 'todo' | 'in_progress' | 'blocked' | 'done' | 'review';

export interface Task {
  id: string;
  user_id: string;
  title: string;
  description: string;
  status: TaskStatus;
  project_id?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateTaskReq {
  title: string;
  description?: string;
  project_id?: string;
}

export interface UpdateTaskReq {
  title?: string;
  description?: string;
  status?: TaskStatus;
  project_id?: string | null;
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

// === User Management ===
export interface UserSummary {
  id: string;
  username: string;
  email: string;
  created_at: string;
}
