import type { Node, Session, CreateSessionReq, Agent, AgentProfile, RuntimeEntity, Task, CreateTaskReq, UpdateTaskReq, TaskStatus, TaskAssignee, AddAssigneeReq, Priority, Project, CreateProjectReq, UpdateProjectReq, ProjectStatus, Workspace, CreateWorkspaceReq, UpdateWorkspaceReq, WorkspaceMember, AddMemberReq, UpdateMemberRoleReq, PendingInvitation, InviteMemberReq, UserSummary, Comment, CreateCommentReq, PluginInfo, AppNotification, TaskRule, TaskRuleLog, CreateRuleReq, UpdateRuleReq, Skill, CreateSkillReq, ExtractSkillReq, AgentQueueItem, AgentLoadInfo } from '../types';



const BASE = '/api';



function getToken(): string | null {

  return localStorage.getItem('token');

}



function authHeaders(): Record<string, string> {

  const token = getToken();

  return token ? { Authorization: `Bearer ${token}` } : {};

}



// List of path prefixes that should NOT get workspace_id appended

const unscopedPrefixes = ['/workspaces', '/auth/', '/nodes', '/agents/runtimes', '/invitations/', '/users', '/invitations/pending'];



async function request<T>(path: string, options?: RequestInit): Promise<T> {

  const wsId = localStorage.getItem('workspace_id');

  let url = `${BASE}${path}`;

  if (wsId && !unscopedPrefixes.some(p => path.startsWith(p))) {

    const separator = path.includes('?') ? '&' : '?';

    url += `${separator}workspace_id=${encodeURIComponent(wsId)}`;

  }



  const res = await fetch(url, {

    ...options,

    headers: {

      'Content-Type': 'application/json',

      ...authHeaders(),

      ...options?.headers,

    },

  });



  if (res.status === 401) {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    localStorage.removeItem('workspace_id');
    localStorage.removeItem('activeSessionID');
    window.location.reload();
    throw new Error('Session expired');
  }

  if (!res.ok) {

    const err = await res.json().catch(() => ({ error: res.statusText }));

    throw new Error(err.error || 'Request failed');

  }



  return res.json();

}



// Auth

export const auth = {

  login: (email: string, password: string) =>

    request<{ token: string; user: { id: string; username: string; email: string }; workspace_id: string }>('/auth/login', {

      method: 'POST',

      body: JSON.stringify({ email, password }),

    }),



  register: (email: string, password: string, invitationToken?: string) => {

    const body: Record<string, string> = { email, password };

    if (invitationToken) body.invitation_token = invitationToken;

    return request<{ token: string; user: { id: string; username: string; email: string }; workspace_id: string }>('/auth/register', {

      method: 'POST',

      body: JSON.stringify(body),

    });

  },

};



// Nodes

export const nodes = {

  list: () => request<{ nodes: Node[] }>('/nodes'),



  getByID: (id: string) => request<Node>(`/nodes/${id}`),



  register: (data: { node_token: string; name: string; os: string; arch: string; version: string }) =>

    request<{ node_id: string; ws_token: string }>('/nodes/register', {

      method: 'POST',

      body: JSON.stringify(data),

    }),



  heartbeat: (nodeID: string, status: string) =>

    request<{ status: string }>('/nodes/heartbeat', {

      method: 'POST',

      body: JSON.stringify({ node_id: nodeID, status }),

    }),



  scan: (nodeID: string) =>

    request<{ status: string }>(`/nodes/${nodeID}/scan`, { method: 'POST' }),



  generateToken: (nodeName: string) =>

    request<{ token: string; expires_at: string; command: string }>('/nodes/token', {

      method: 'POST',

      body: JSON.stringify({ node_name: nodeName }),

    }),



  remove: (nodeID: string) =>

    request<{ status: string }>(`/nodes/${nodeID}`, { method: 'DELETE' }),

  start: (id: string) =>
    request<{ status: string; command?: string; command_ps1?: string; server?: string; node_id?: string }>(`/nodes/${id}/start`, { method: 'POST' }),

  stop: (id: string) =>
    request<{ status: string }>(`/nodes/${id}/stop`, { method: 'POST' }),
};



// Agents

export const agents = {

  list: (nodeID: string) => request<{ agents: Agent[] }>(`/nodes/${nodeID}/agents`),



  toggle: (agentID: string, enabled: boolean) =>

    request<{ status: string }>(`/agents/${encodeURIComponent(agentID)}`, {

      method: 'PATCH',

      body: JSON.stringify({ enabled }),

    }),

};



// Agent Profiles

export const agentProfiles = {

  list: () => request<{ profiles: AgentProfile[] }>('/agents/profiles'),



  get: (id: string) => request<AgentProfile>(`/agents/profiles/${id}`),



  create: (data: { name: string; description?: string; system_prompt?: string; agent_id: string; node_id?: string; tags?: string[] }) =>

    request<{ id: string; status: string }>('/agents/profiles', {

      method: 'POST',

      body: JSON.stringify(data),

    }),



  update: (id: string, data: Partial<{ name: string; description: string; system_prompt: string; avatar: string; agent_id: string; node_id: string; enabled: boolean; max_concurrency: number; tags: string[] }>) =>

    request<{ status: string }>(`/agents/profiles/${id}`, {

      method: 'PUT',

      body: JSON.stringify(data),

    }),



  delete: (id: string) =>

    request<{ status: string }>(`/agents/profiles/${id}`, { method: 'DELETE' }),



  listRuntimes: () => request<{ runtimes: RuntimeEntity[] }>('/agents/runtimes'),

};



// Sessions

export const sessions = {

  list: () => request<{ sessions: Session[] }>('/sessions'),



  getByID: (id: string) => request<Session>(`/sessions/${id}`),



  create: (data: CreateSessionReq) =>

    request<{ id: string; status: string; prompt: string; workspace: string; node_id: string; created_at: string }>(

      '/sessions',

      { method: 'POST', body: JSON.stringify(data) }

    ),



  getMessages: (sessionID: string) =>

    request<{ messages: Record<string, unknown>[] }>(`/sessions/${sessionID}/messages`),

};



// Tasks

export const tasks = {

  list: (params?: { projectId?: string; parentId?: string; assigneeId?: string; delegatedAssigneeId?: string; priority?: string; tag?: string }) => {
    const query = new URLSearchParams();
    if (params?.projectId) query.set('project_id', params.projectId);
    if (params?.parentId) query.set('parent_id', params.parentId);
    if (params?.assigneeId) query.set('assignee_id', params.assigneeId);
    if (params?.delegatedAssigneeId) query.set('delegated_assignee_id', params.delegatedAssigneeId);
    if (params?.priority) query.set('priority', params.priority);
    if (params?.tag) query.set('tag', params.tag);
    const qs = query.toString();
    return request<{ tasks: Task[] }>(`/tasks${qs ? '?' + qs : ''}`);
  },

  get: (id: string) => request<Task>(`/tasks/${id}`),

  create: (data: CreateTaskReq) =>
    request<Task>('/tasks', { method: 'POST', body: JSON.stringify(data) }),

  update: (id: string, data: UpdateTaskReq) =>
    request<Task>(`/tasks/${id}`, { method: 'PUT', body: JSON.stringify(data) }),

  delete: (id: string) =>
    request<{ status: string }>(`/tasks/${id}`, { method: 'DELETE' }),

  setStatus: (id: string, status: TaskStatus) =>
    request<Task>(`/tasks/${id}/status`, { method: 'PATCH', body: JSON.stringify({ status }) }),

  // Trash
  listTrash: () => request<{ tasks: Task[] }>('/tasks/trash'),

  permanentDelete: (id: string) =>
    request<{ status: string }>(`/tasks/${id}/force`, { method: 'DELETE' }),

  restore: (id: string) =>
    request<{ status: string }>(`/tasks/${id}/restore`, { method: 'POST' }),

  // Assignees
  listAssignees: (id: string) =>
    request<{ assignees: TaskAssignee[] }>(`/tasks/${id}/assignees`),

  addAssignee: (id: string, data: AddAssigneeReq) =>
    request<{ status: string }>(`/tasks/${id}/assignees`, { method: 'POST', body: JSON.stringify(data) }),

  removeAssignee: (id: string, assigneeId: string) =>
    request<{ status: string }>(`/tasks/${id}/assignees/${assigneeId}`, { method: 'DELETE' }),

  // Subtasks
  listSubtasks: (id: string) =>
    request<{ tasks: Task[] }>(`/tasks/${id}/subtasks`),

};



// Projects

export const projects = {

  list: (status?: string) => {
    const path = status ? `/projects?status=${status}` : '/projects';
    return request<{ projects: Project[] }>(path);
  },

  get: (id: string) => request<Project>(`/projects/${id}`),

  create: (req: CreateProjectReq) =>
    request<Project>('/projects', { method: 'POST', body: JSON.stringify(req) }),

  update: (id: string, req: UpdateProjectReq) =>
    request<Project>(`/projects/${id}`, { method: 'PUT', body: JSON.stringify(req) }),

  delete: (id: string) =>
    request<{ status: string }>(`/projects/${id}`, { method: 'DELETE' }),

  listTrash: () => request<{ projects: Project[] }>('/projects/trash'),

  permanentDelete: (id: string) =>
    request<{ status: string }>(`/projects/${id}/force`, { method: 'DELETE' }),

  restore: (id: string) =>
    request<{ status: string }>(`/projects/${id}/restore`, { method: 'POST' }),

};



// Workspaces

export const workspaces = {

  list: () => request<{ workspaces: Workspace[] }>('/workspaces'),



  create: (req: CreateWorkspaceReq) =>

    request<Workspace>('/workspaces', { method: 'POST', body: JSON.stringify(req) }),



  update: (id: string, req: UpdateWorkspaceReq) =>

    request<Workspace>(`/workspaces/${id}`, { method: 'PUT', body: JSON.stringify(req) }),



  delete: (id: string) =>

    request<{ status: string }>(`/workspaces/${id}`, { method: 'DELETE' }),

};



// Workspace Members

export const workspaceMembers = {

  list: (workspaceId: string) =>

    request<{ members: WorkspaceMember[] }>(`/workspaces/${workspaceId}/members`),



  add: (workspaceId: string, data: AddMemberReq) =>

    request<{ status: string }>(`/workspaces/${workspaceId}/members`, {

      method: 'POST',

      body: JSON.stringify(data),

    }),



  updateRole: (workspaceId: string, userId: string, data: UpdateMemberRoleReq) =>

    request<{ status: string }>(`/workspaces/${workspaceId}/members/${userId}`, {

      method: 'PUT',

      body: JSON.stringify(data),

    }),



  remove: (workspaceId: string, userId: string) =>

    request<{ status: string }>(`/workspaces/${workspaceId}/members/${userId}`, {

      method: 'DELETE',

    }),

};



// Invitations

export const invitations = {

  get: (token: string) =>

    request<PendingInvitation>(`/invitations/${token}`),



  pending: () =>

    request<{ invitations: PendingInvitation[] }>('/invitations/pending'),



  accept: (token: string) =>

    request<{ status: string; workspace_id?: string; invitee_email?: string; token?: string }>(`/invitations/${token}/accept`, {

      method: 'POST',

    }),



  decline: (token: string) =>

    request<{ status: string }>(`/invitations/${token}/decline`, {

      method: 'POST',

    }),



  create: (workspaceId: string, data: InviteMemberReq) =>

    request<{ status: string; invitation: PendingInvitation; redirect_url: string }>(`/workspaces/${workspaceId}/invitations`, {

      method: 'POST',

      body: JSON.stringify(data),

    }),



  list: (workspaceId: string) =>

    request<{ invitations: PendingInvitation[] }>(`/workspaces/${workspaceId}/invitations`),



  cancel: (workspaceId: string, invitationId: string) =>

    request<{ status: string }>(`/workspaces/${workspaceId}/invitations/${invitationId}`, {

      method: 'DELETE',

    }),

};



// Comments

export const comments = {
  list: (taskId: string) =>
    request<{ comments: Comment[] }>(`/tasks/${taskId}/comments`),

  create: (taskId: string, data: CreateCommentReq) =>
    request<Comment>(`/tasks/${taskId}/comments`, { method: 'POST', body: JSON.stringify(data) }),

  delete: (taskId: string, commentId: string) =>
    request<{ status: string }>(`/tasks/${taskId}/comments/${commentId}`, { method: 'DELETE' }),
};

// Notifications

export const notifications = {
  list: (before?: string) => {
    const qs = before ? `?before=${before}` : '';
    return request<{ notifications: AppNotification[] }>(`/notifications${qs}`);
  },
  unreadCount: () => request<{ count: number }>('/notifications/unread-count'),
  markRead: (id: string) =>
    request<{ status: string }>(`/notifications/${id}/read`, { method: 'PATCH' }),
  markAllRead: () =>
    request<{ status: string; count: number }>('/notifications/read-all', { method: 'PATCH' }),
  delete: (id: string) =>
    request<{ status: string }>(`/notifications/${id}`, { method: 'DELETE' }),
};

// Rules
export const rules = {
  list: () => request<{ rules: TaskRule[] }>('/rules'),
  get: (id: string) => request<TaskRule>(`/rules/${id}`),
  create: (data: CreateRuleReq) =>
    request<{ id: string; status: string }>('/rules', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: UpdateRuleReq) =>
    request<{ status: string }>(`/rules/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    request<{ status: string }>(`/rules/${id}`, { method: 'DELETE' }),
  listLogs: (id: string) =>
    request<{ logs: TaskRuleLog[] }>(`/rules/${id}/logs`),
};

// Plugins

export const plugins = {
  list: () => request<{ plugins: PluginInfo[] }>('/plugins'),
  get: (id: string) => request<{ plugin: PluginInfo }>(`/plugins/${id}`),
  start: (id: string) => request<{ status: string; plugin: string }>(`/plugins/${id}/start`, { method: 'POST' }),
  stop: (id: string) => request<{ status: string; plugin: string }>(`/plugins/${id}/stop`, { method: 'POST' }),
  reload: (id: string) => request<{ status: string; plugin: string }>(`/plugins/${id}/reload`, { method: 'POST' }),
  remove: (id: string) => request<{ status: string; plugin: string }>(`/plugins/${id}/remove`, { method: 'POST' }),

  installUpload: async (file: File) => {
    const token = getToken();
    const wsId = localStorage.getItem('workspace_id');
    const formData = new FormData();
    formData.append('plugin', file);

    let url = `${BASE}/plugins/install/upload`;
    if (wsId) url += `?workspace_id=${encodeURIComponent(wsId)}`;

    const res = await fetch(url, {
      method: 'POST',
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: formData,
    });

    if (res.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      localStorage.removeItem('workspace_id');
      localStorage.removeItem('activeSessionID');
      window.location.reload();
      throw new Error('Session expired');
    }
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || 'Request failed');
    }
    return res.json() as Promise<{ status: string; plugin: string; version: string }>;
  },

  installGit: (url: string, branch?: string) =>
    request<{ status: string; plugin: string; version: string }>('/plugins/install/git', {
      method: 'POST',
      body: JSON.stringify({ url, branch }),
    }),
};

// Skills
export const skills = {
  list: () => request<{ skills: Skill[] }>('/skills'),
  get: (id: string) => request<Skill>(`/skills/${id}`),
  create: (data: CreateSkillReq) =>
    request<{ id: string; status: string }>('/skills', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Partial<CreateSkillReq>) =>
    request<{ status: string }>(`/skills/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    request<{ status: string }>(`/skills/${id}`, { method: 'DELETE' }),
  extractFromTask: (data: ExtractSkillReq) =>
    request<{ id: string; status: string; name: string }>('/skills/extract-from-task', { method: 'POST', body: JSON.stringify(data) }),
};

// Agent Queue
export const agentQueue = {
  list: (params?: { agent_profile_id?: string; status?: string }) => {
    const query = new URLSearchParams();
    if (params?.agent_profile_id) query.set('agent_profile_id', params.agent_profile_id);
    if (params?.status) query.set('status', params.status);
    const qs = query.toString();
    return request<{ queue: AgentQueueItem[] }>(`/agents/queue${qs ? '?' + qs : ''}`);
  },
  autoAssign: (taskId: string) =>
    request<{ id: string; task_id: string; agent_profile_id: string; agent_name: string; status: string }>(`/agents/auto-assign/${taskId}`, { method: 'POST' }),
  claim: (id: string) =>
    request<{ status: string }>(`/agents/queue/${id}/claim`, { method: 'POST' }),
  updateStatus: (id: string, data: { status: string; result_summary?: string; snapshot?: Record<string, unknown> }) =>
    request<{ status: string }>(`/agents/queue/${id}/status`, { method: 'PUT', body: JSON.stringify(data) }),
  listAgentsWithLoad: () =>
    request<{ agents: AgentLoadInfo[] }>('/agents/queue/agents'),
};

// User Management

export const users = {

  list: () =>

    request<{ users: UserSummary[] }>('/users'),



  delete: (id: string) =>

    request<{ status: string }>(`/users/${id}`, { method: 'DELETE' }),

};

