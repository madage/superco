import type { Node, Session, CreateSessionReq, Agent, AgentProfile, RuntimeEntity, Task, CreateTaskReq, UpdateTaskReq, TaskStatus, Project, CreateProjectReq, UpdateProjectReq, Workspace, CreateWorkspaceReq, UpdateWorkspaceReq, WorkspaceMember, AddMemberReq, UpdateMemberRoleReq, PendingInvitation, InviteMemberReq, UserSummary } from '../types';



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
    request<{ status: string; pid?: number }>(`/nodes/${id}/start`, { method: 'POST' }),

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



  create: (data: { name: string; description?: string; agent_id: string }) =>

    request<{ id: string; status: string }>('/agents/profiles', {

      method: 'POST',

      body: JSON.stringify(data),

    }),



  update: (id: string, data: Partial<{ name: string; description: string; avatar: string; agent_id: string; enabled: boolean }>) =>

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

  list: (projectId?: string) => {

    const path = projectId ? `/tasks?project_id=${projectId}` : '/tasks';

    return request<{ tasks: Task[] }>(path);

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

};



// Projects

export const projects = {

  list: () => request<{ projects: Project[] }>('/projects'),



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



// User Management

export const users = {

  list: () =>

    request<{ users: UserSummary[] }>('/users'),



  delete: (id: string) =>

    request<{ status: string }>(`/users/${id}`, { method: 'DELETE' }),

};

