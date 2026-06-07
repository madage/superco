import { useState, useCallback, useRef, useEffect } from 'react';
import { useMessageBus, type Envelope, type ContentBlock } from './hooks/useMessageBus';
import { NodeList } from './components/NodeList';
import { AddNodeDialog } from './components/AddNodeDialog';
import { AgentList } from './components/AgentList';
import { TaskBoard } from './components/TaskBoard';
import { ProjectList } from './components/ProjectList';
import { TrashView } from './components/TrashView';
import { PluginList } from './components/PluginList';
import { RuleList } from './components/RuleList';
import { SkillList } from './components/SkillList';
import { AgentQueuePanel } from './components/AgentQueuePanel';
import { FloatingChat } from './components/FloatingChat';
import { LangSwitcher } from './components/LangSwitcher';
import WorkspaceMembers from './components/WorkspaceMembers';
import NotificationBell from './components/NotificationBell';
import { useDashboardWSContext } from './hooks/DashboardWSContext';
import { useResourceSync } from './hooks/useResourceSync';
import { useLang } from './i18n/context';
import { auth as authApi, workspaces as workspacesApi, workspaceMembers as workspaceMembersApi, invitations as invitationsApi, users as usersApi } from './api/client';
import type { Node, Session, AuthState, Workspace, WorkspaceRole, WorkspaceMember, UserSummary } from './types';
import WorkspaceContext from './hooks/WorkspaceContext';
import { pluginClient } from './plugin/PluginClient';

type Page = 'nodes' | 'tasks' | 'rules' | 'projects' | 'agents' | 'skills' | 'agent-queue' | 'plugins' | 'trash';

function App() {
  const { t, lang } = useLang();
  const [auth, setAuth] = useState<AuthState>(() => {
    const token = localStorage.getItem('token');
    const raw = localStorage.getItem('user');
    const wsId = localStorage.getItem('workspace_id');
    if (token && raw) {
      try {
        return { token, user: JSON.parse(raw), workspace_id: wsId, workspace_role: null };
      } catch {
        // corrupted user data, ignore
      }
    }
    return { token: null, user: null, workspace_id: null, workspace_role: null };
  });
  const [page, setPage] = useState<Page>(() => {
    const saved = localStorage.getItem('page') as Page | null;
    return saved || 'nodes';
  });
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isRegister, setIsRegister] = useState(false);
  const [authError, setAuthError] = useState<string | null>(null);
  const { nodes, sessions, connected: dashboardConnected, subscribeNotification } = useDashboardWSContext();
  const [toast, setToast] = useState<{ title: string; message: string } | null>(null);
  const [showAddNode, setShowAddNode] = useState(false);
  const [targetTaskId, setTargetTaskId] = useState<string | null>(null);

  useEffect(() => {
    localStorage.setItem('page', page);
  }, [page]);

  // Invitation token from URL
  const [invitationToken, setInvitationToken] = useState<string | null>(null);
  const [invitationInfo, setInvitationInfo] = useState<{
    workspace_name?: string;
    inviter_name?: string;
    status?: string;
  } | null>(null);

  // Check URL for invitation token on mount
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const token = params.get('token');
    if (token) {
      setInvitationToken(token);
      invitationsApi.get(token).then((info) => {
        setInvitationInfo({
          workspace_name: info.workspace_name,
          inviter_name: info.inviter_name,
          status: 'valid',
        });
      }).catch((err) => {
        setInvitationInfo({ status: err.message || 'invalid' });
      });
      // Clean URL
      window.history.replaceState({}, '', window.location.pathname);
    }
  }, []);

  // Handle invitation accept when user is logged in
  useEffect(() => {
    if (auth.token && invitationToken && invitationInfo?.status === 'valid') {
      invitationsApi.accept(invitationToken).then((res) => {
        if (res.workspace_id) {
          localStorage.setItem('workspace_id', res.workspace_id);
          setInvitationToken(null);
          setInvitationInfo(null);
          // Refresh workspaces to get updated list
          workspacesApi.list().then((wsRes) => {
            setWorkspaces(wsRes.workspaces);
            const ws = wsRes.workspaces.find(w => w.id === res.workspace_id);
            setAuth(prev => ({
              ...prev,
              workspace_id: res.workspace_id || prev.workspace_id,
              workspace_role: (ws?.role as WorkspaceRole) || prev.workspace_role,
            }));
          }).catch(() => {});
        }
      }).catch(() => {});
    }
  }, [auth.token, invitationToken, invitationInfo]);

  // Workspace state
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspaceKey, setWorkspaceKey] = useState(0);
  const [showWorkspaceManager, setShowWorkspaceManager] = useState(false);
  const [wsManagerTab, setWsManagerTab] = useState<'workspaces' | 'members' | 'users'>('workspaces');
  const [newWsName, setNewWsName] = useState('');
  const [newWsDesc, setNewWsDesc] = useState('');

  // User management state
  const [userList, setUserList] = useState<UserSummary[]>([]);
  const [userDeleteVerify, setUserDeleteVerify] = useState<{
    userId: string; email: string; a: number; b: number; op: '+' | '-'; answer: number;
  } | null>(null);
  const [userVerifyInput, setUserVerifyInput] = useState('');

  // Workspace delete verification
  const [wsDeleteVerify, setWsDeleteVerify] = useState<{
    id: string; a: number; b: number; op: '+' | '-'; answer: number;
  } | null>(null);
  const [wsVerifyInput, setWsVerifyInput] = useState('');
  const [wsVerifyError, setWsVerifyError] = useState(false);

  // Fetch workspaces when authenticated
  useEffect(() => {
    if (auth.token) {
      workspacesApi.list().then((res) => {
        setWorkspaces(res.workspaces);
        const currentWsId = localStorage.getItem('workspace_id');
        const currentWs = res.workspaces.find(w => w.id === currentWsId);
        if (currentWs?.role) {
          setAuth(prev => ({ ...prev, workspace_role: currentWs.role as WorkspaceRole }));
        }
      }).catch(() => {});
    }
  }, [auth.token]);

  const [pendingPermissions, setPendingPermissions] = useState<Envelope[]>([]);

  // Permission mode: 'auto' (auto-approve) or 'restricted' (require user input)
  const [permissionMode, setPermissionMode] = useState<'auto' | 'restricted'>('auto');
  const permissionModeRef = useRef(permissionMode);
  permissionModeRef.current = permissionMode;

  // Message Bus — only connect when authenticated
  const bus = useMessageBus({
    userID: auth.user?.id || 'anonymous',
    workspaceID: auth.workspace_id || undefined,
    onMessage: useCallback((env: Envelope) => {
      if (env.type === 'permission.request') {
        if (permissionModeRef.current === 'auto') {
          const sid = sessionIDRef.current;
          if (sid) {
            bus.send({
              type: 'permission.response',
              to: `session://${sid}`,
              session_id: sid,
              payload: {
                tool_use_id: env.payload?.tool_use_id,
                approved: true,
              },
            });
          } else {
            setPendingPermissions((prev) => [...prev, env]);
          }
        } else {
          setPendingPermissions((prev) => [...prev, env]);
        }
      }
    }, []),
  });

  const sessionIDRef = useRef<string | null>(null);
  sessionIDRef.current = bus.sessionID;

  const sendPermissionResponse = useCallback((approved: boolean) => {
    const queue = pendingPermissions;
    if (queue.length === 0 || !bus.sessionID) return;
    for (const req of queue) {
      bus.send({
        type: 'permission.response',
        to: `session://${bus.sessionID}`,
        session_id: bus.sessionID,
        payload: {
          tool_use_id: req.payload?.tool_use_id,
          approved,
        },
      });
    }
    setPendingPermissions([]);
  }, [pendingPermissions, bus]);

  const handleNodeSelect = useCallback((node: Node) => {
    void node;
  }, []);

  const handleSessionSelect = useCallback((session: Session) => {
    bus.joinSession(session.id);
  }, [bus]);

  const handleSessionCreated = useCallback(async (sessionID: string) => {
    bus.joinSession(sessionID);
  }, [bus]);

  const handleSendBlocks = useCallback((blocks: ContentBlock[]) => {
    if (!bus.sessionID) return false;
    return bus.send({
      type: 'message',
      to: `session://${bus.sessionID}`,
      session_id: bus.sessionID,
      payload: { content: blocks },
    });
  }, [bus.sessionID, bus.send]);

  const handleWorkspaceChange = useCallback(async () => {
    try {
      const res = await workspacesApi.list();
      setWorkspaces(res.workspaces);
      const currentWsId = localStorage.getItem('workspace_id');
      const currentWs = res.workspaces.find(w => w.id === currentWsId);
      if (currentWs?.role) {
        setAuth(prev => ({ ...prev, workspace_role: currentWs.role as WorkspaceRole }));
      }
      setWorkspaceKey((k) => k + 1);
    } catch {}
  }, []);

  const handleOpenTask = useCallback((taskId: string) => {
    setTargetTaskId(taskId);
    setPage('tasks');
  }, []);

  // Auto-refresh workspaces on WebSocket resource_change signal
  useResourceSync("workspaces", handleWorkspaceChange);

  // In-app notification toast from WebSocket
  useEffect(() => {
    if (!auth.token) return;
    const unsub = subscribeNotification((n) => {
      setToast(n);
      setTimeout(() => setToast(null), 5000);
    });
    return unsub;
  }, [auth.token, subscribeNotification]);

  // Initialize plugin client on auth
  useEffect(() => {
    if (auth.token) {
      pluginClient.init('/api');
    }
  }, [auth.token]);

  const handleCreateWorkspace = useCallback(async () => {
    if (!newWsName.trim()) return;
    try {
      await workspacesApi.create({ name: newWsName, description: newWsDesc });
      setNewWsName('');
      setNewWsDesc('');
      const res = await workspacesApi.list();
      setWorkspaces(res.workspaces);
    } catch {
      alert('Failed to create workspace');
    }
  }, [newWsName, newWsDesc]);

  const handleDeleteWorkspace = useCallback(async (id: string) => {
    try {
      await workspacesApi.delete(id);
      const res = await workspacesApi.list();
      setWorkspaces(res.workspaces);
      if (id === localStorage.getItem('workspace_id')) {
        const firstWs = res.workspaces[0];
        if (firstWs) {
          localStorage.setItem('workspace_id', firstWs.id);
          setWorkspaceKey((k) => k + 1);
        }
      }
    } catch {
      alert('Failed to delete workspace');
    }
  }, []);

  const handleWsDeleteClick = useCallback((id: string) => {
    const a = Math.floor(Math.random() * 20) + 1;
    const b = Math.floor(Math.random() * 20) + 1;
    const op = Math.random() > 0.5 ? '+' : '-';
    const answer = op === '+' ? a + b : Math.max(a, b) - Math.min(a, b);
    const [na, nb] = op === '+' ? [a, b] : [Math.max(a, b), Math.min(a, b)];
    setWsDeleteVerify({ id, a: na, b: nb, op, answer });
    setWsVerifyInput('');
    setWsVerifyError(false);
  }, []);

  const handleWsDeleteConfirm = useCallback(async () => {
    if (!wsDeleteVerify) return;
    const userAnswer = parseInt(wsVerifyInput, 10);
    if (isNaN(userAnswer) || userAnswer !== wsDeleteVerify.answer) {
      setWsVerifyError(true);
      return;
    }
    const id = wsDeleteVerify.id;
    setWsDeleteVerify(null);
    setWsVerifyInput('');
    setWsVerifyError(false);
    await handleDeleteWorkspace(id);
  }, [wsDeleteVerify, wsVerifyInput, handleDeleteWorkspace]);

  // User management
  const fetchUsers = useCallback(async () => {
    try {
      const res = await usersApi.list();
      setUserList(res.users);
    } catch {
      // silently fail
    }
  }, []);

  const handleUserDeleteClick = useCallback((userId: string, userEmail: string) => {
    const a = Math.floor(Math.random() * 20) + 1;
    const b = Math.floor(Math.random() * 20) + 1;
    const op = Math.random() > 0.5 ? '+' : '-';
    const answer = op === '+' ? a + b : Math.max(a, b) - Math.min(a, b);
    const [na, nb] = op === '+' ? [a, b] : [Math.max(a, b), Math.min(a, b)];
    setUserDeleteVerify({ userId, email: userEmail, a: na, b: nb, op, answer });
    setUserVerifyInput('');
  }, []);

  const handleUserDeleteConfirm = useCallback(async () => {
    if (!userDeleteVerify) return;
    const userAnswer = parseInt(userVerifyInput, 10);
    if (isNaN(userAnswer) || userAnswer !== userDeleteVerify.answer) {
      alert(lang === 'zh' ? '答案错误' : 'Wrong answer');
      return;
    }
    try {
      await usersApi.delete(userDeleteVerify.userId);
      setUserDeleteVerify(null);
      setUserVerifyInput('');
      fetchUsers();
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to delete user');
    }
  }, [userDeleteVerify, userVerifyInput, fetchUsers, lang]);

  // Login screen
  if (!auth.token) {
    return (
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          height: '100vh',
          background: 'linear-gradient(135deg, #1a1a2e 0%, #16213e 100%)',
          position: 'relative',
        }}
      >
        <div style={{ position: 'absolute', top: 16, right: 16 }}>
          <LangSwitcher />
        </div>
        <div
          style={{
            background: '#fff',
            padding: '40px',
            borderRadius: '12px',
            boxShadow: '0 8px 32px rgba(0,0,0,0.3)',
            width: '400px',
          }}
        >
          <h1 style={{ margin: '0 0 24px', textAlign: 'center', color: '#1a1a2e' }}>{t('appTitle')}</h1>
          <p style={{ textAlign: 'center', color: '#666', marginBottom: '24px' }}>
            {t('appSubtitle')}
          </p>

          {/* Invitation info banner */}
          {invitationInfo?.status === 'valid' && (
            <div style={{
              background: '#e8f5e9', borderRadius: '8px', padding: '12px',
              marginBottom: '16px', fontSize: '0.9em', color: '#2e7d32',
            }}>
              {lang === 'zh' ? (
                <>你已被 <strong>{invitationInfo.inviter_name}</strong> 邀请加入工作区 <strong>{invitationInfo.workspace_name}</strong></>
              ) : (
                <>You've been invited by <strong>{invitationInfo.inviter_name}</strong> to join workspace <strong>{invitationInfo.workspace_name}</strong></>
              )}
              <div style={{ marginTop: '4px', fontSize: '0.85em' }}>
                {lang === 'zh' ? '登录或注册后将自动接受邀请' : 'Login or register to accept the invitation'}
              </div>
            </div>
          )}

          {invitationInfo?.status && invitationInfo.status !== 'valid' && (
            <div style={{
              background: '#fbe9e7', borderRadius: '8px', padding: '12px',
              marginBottom: '16px', fontSize: '0.9em', color: '#c62828',
            }}>
              {invitationInfo.status}
            </div>
          )}

          <form onSubmit={async (e) => {
            e.preventDefault();
            setAuthError(null);
            try {
              const fn = isRegister
                ? (email: string, password: string) => authApi.register(email, password, invitationToken || undefined)
                : authApi.login;
              const data = await fn(email, password);
              localStorage.setItem('token', data.token);
              localStorage.setItem('user', JSON.stringify(data.user));
              if (data.workspace_id) {
                localStorage.setItem('workspace_id', data.workspace_id);
              }
              setAuth({ token: data.token, user: data.user, workspace_id: data.workspace_id || null, workspace_role: null });
            } catch (err) {
              setAuthError(err instanceof Error ? err.message : t('authFailed'));
            }
          }}>
            <div style={{ marginBottom: '16px' }}>
              <input
                type="email"
                placeholder={lang === 'zh' ? '邮箱' : 'Email'}
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                style={inputStyle}
                required
              />
            </div>
            <div style={{ marginBottom: '16px' }}>
              <input
                type="password"
                placeholder={t('password')}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                style={inputStyle}
                required
              />
            </div>

            {authError && (
              <div style={{ color: '#f44336', marginBottom: '12px', fontSize: '0.9em' }}>{authError}</div>
            )}

            <button type="submit" style={buttonStyle}>
              {isRegister ? t('register') : t('login')}
            </button>

            <div style={{ textAlign: 'center', marginTop: '16px' }}>
              <button
                type="button"
                onClick={() => setIsRegister(!isRegister)}
                style={{
                  background: 'none',
                  border: 'none',
                  color: '#1976d2',
                  cursor: 'pointer',
                  fontSize: '0.9em',
                }}
              >
                {isRegister ? t('alreadyHasAccount') : t('noAccount')}
              </button>
            </div>
          </form>

          {invitationToken && (
            <div style={{ marginTop: '12px', textAlign: 'center' }}>
              <button
                onClick={() => { setInvitationToken(null); setInvitationInfo(null); }}
                style={{
                  background: 'none', border: 'none', color: '#999',
                  cursor: 'pointer', fontSize: '0.85em',
                }}
              >
                {lang === 'zh' ? '忽略邀请' : 'Dismiss invitation'}
              </button>
            </div>
          )}
        </div>
      </div>
    );
  }

  const busConnected = bus.connected;
  const hasSession = bus.sessionID !== null;

  return (
    <WorkspaceContext.Provider value={{ role: auth.workspace_role, workspaceId: auth.workspace_id }}>
    <div style={{ display: 'flex', height: '100vh', background: '#f5f5f5' }}>
      {/* Sidebar */}
      <div
        style={{
          width: '280px',
          background: '#1a1a2e',
          color: '#fff',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <div style={{ padding: '20px', borderBottom: '1px solid #333' }}>
          <h2 style={{ margin: 0, fontSize: '1.3em' }}>{t('appTitle')}</h2>
          {/* Workspace selector */}
          {workspaces.length > 0 && (
            <div style={{ marginTop: '10px', display: 'flex', gap: '4px', alignItems: 'center' }}>
              <select
                value={localStorage.getItem('workspace_id') || ''}
                onChange={(e) => {
                  const newId = e.target.value;
                  localStorage.setItem('workspace_id', newId);
                  localStorage.removeItem('activeSessionID');
                  const ws = workspaces.find(w => w.id === newId);
                  setAuth(prev => ({ ...prev, workspace_id: newId, workspace_role: (ws?.role as WorkspaceRole) || null }));
                  setWorkspaceKey((k) => k + 1);
                }}
                style={{
                  flex: 1, padding: '6px 8px', borderRadius: '6px', border: '1px solid #444',
                  background: '#2a2a3e', color: '#fff', fontSize: '0.82em', cursor: 'pointer', outline: 'none',
                }}
              >
                {workspaces.map((ws) => (
                  <option key={ws.id} value={ws.id}>{ws.name}</option>
                ))}
              </select>
              <button onClick={() => setShowWorkspaceManager(true)} style={{
                background: 'none', border: 'none', color: '#888', cursor: 'pointer', fontSize: '0.9em', padding: '4px',
              }} title={t('manageWorkspaces')}>⚙</button>
            </div>
          )}
          <div style={{ fontSize: '0.85em', color: '#999', marginTop: '4px', display: 'flex', alignItems: 'center', gap: '6px' }}>
            <span>{auth.user?.username} ({auth.user?.email})</span>
            <NotificationBell onWorkspaceChange={handleWorkspaceChange} onOpenTask={handleOpenTask} />
          </div>
        </div>

        <nav style={{ display: 'flex', flexDirection: 'column', padding: '8px' }}>
          {(['nodes', 'tasks', 'rules', 'projects', 'agents', 'skills', 'agent-queue', ...(auth.workspace_role === 'owner' || auth.workspace_role === 'admin' ? ['plugins'] : []), 'trash'] as Page[]).map((p) => (
            <button
              key={p}
              onClick={() => setPage(p)}
              style={{
                padding: '12px 16px',
                textAlign: 'left',
                background: page === p ? 'rgba(255,255,255,0.1)' : 'transparent',
                color: '#fff',
                border: 'none',
                borderRadius: '6px',
                cursor: 'pointer',
                fontSize: '0.95em',
                marginBottom: '2px',
                ...(p === 'trash' ? { marginTop: 'auto', marginBottom: 0 } : {}),
              }}
            >
              <span style={{ display: 'inline-block', width: '24px', textAlign: 'center', marginRight: '4px' }}>
                {p === 'nodes' ? '📡' : p === 'tasks' ? '📋' : p === 'projects' ? '📁' : p === 'agents' ? '🤖' : p === 'rules' ? '⚡' : p === 'skills' ? '📚' : p === 'agent-queue' ? '⏳' : p === 'plugins' ? '🧩' : '🗑'}
              </span>
              {p === 'nodes' ? t('navNodes') : p === 'tasks' ? t('navTasks') : p === 'projects' ? t('navProjects') : p === 'agents' ? t('agents') : p === 'rules' ? t('navAutomation') : p === 'skills' ? t('navSkills') : p === 'agent-queue' ? t('navAgentQueue') || 'Queue' : p === 'plugins' ? t('navPlugins') : t('navTrash')}
            </button>
          ))}
        </nav>

        {/* Connection status */}
        <div style={{ padding: '8px 16px', fontSize: '0.75em', color: '#666' }}>
          <span style={{
            display: 'inline-block',
            width: '8px',
            height: '8px',
            borderRadius: '50%',
            background: busConnected ? '#4caf50' : '#f44336',
            marginRight: '4px',
          }} />
          Bus: {busConnected ? 'connected' : 'offline'}
          {hasSession && (
            <span style={{ marginLeft: '8px', color: '#4caf50' }}>
              | Session: {bus.sessionID?.slice(0, 6)}...
            </span>
          )}
          <span style={{ marginLeft: '8px' }}>
            <span style={{
              display: 'inline-block',
              width: '6px',
              height: '6px',
              borderRadius: '50%',
              background: dashboardConnected ? '#4caf50' : '#f44336',
              marginRight: '2px',
            }} />
            Dash: {dashboardConnected ? 'on' : 'off'}
          </span>
        </div>

        <div style={{ marginTop: 'auto', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <LangSwitcher />
          <button
            onClick={() => {
              localStorage.removeItem('token');
              localStorage.removeItem('user');
              localStorage.removeItem('workspace_id');
              localStorage.removeItem('activeSessionID');
              setAuth({ token: null, user: null, workspace_id: null, workspace_role: null });
            }}
            style={{
              width: '100%',
              padding: '8px',
              background: 'transparent',
              color: '#999',
              border: '1px solid #333',
              borderRadius: '4px',
              cursor: 'pointer',
            }}
          >
            {t('logout')}
          </button>
        </div>
      </div>

      {/* Main content */}
      <div style={{ flex: 1, overflow: 'hidden', position: 'relative' }}>
        {/* Nodes page */}
        <div style={{ display: page === 'nodes' ? 'block' : 'none', padding: '24px', maxWidth: '800px', height: '100%', overflow: 'auto' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
            <h2 style={{ margin: 0 }}>{t('agentNodes')}</h2>
            <button
              onClick={() => setShowAddNode(true)}
              style={{
                padding: '8px 16px',
                background: '#1976d2',
                color: '#fff',
                border: 'none',
                borderRadius: '6px',
                cursor: 'pointer',
                fontSize: '0.85em',
                fontWeight: 600,
              }}
            >
              + {t('addNode')}
            </button>
          </div>
          <NodeList nodes={nodes} onSelect={handleNodeSelect} />
          <div style={{ fontSize: '0.85em', color: '#999', marginTop: '8px' }}>
            {sessions.length} {t('sessions').toLowerCase()} | {t('navNodes').toLowerCase()}: {nodes.length}
          </div>
        </div>

        {/* Agents page */}
        <div style={{ display: page === 'agents' ? 'block' : 'none', height: '100%', overflow: 'auto' }}>
          <h2 style={{ padding: '24px 24px 0' }}>{t('agents')}</h2>
          <AgentList key={workspaceKey} />
        </div>

        {/* Tasks page */}
        <div style={{ display: page === 'tasks' ? 'block' : 'none', height: '100%', overflow: 'auto' }}>
          <TaskBoard key={workspaceKey} initialTaskId={targetTaskId} onTaskOpened={() => setTargetTaskId(null)} />
        </div>

        {/* Projects page */}
        <div style={{ display: page === 'projects' ? 'block' : 'none', height: '100%', overflow: 'auto' }}>
          <ProjectList key={workspaceKey} />
        </div>

        {/* Rules page */}
        <div style={{ display: page === 'rules' ? 'block' : 'none', height: '100%', overflow: 'auto' }}>
          <RuleList key={workspaceKey} />
        </div>

        {/* Skills page */}
        <div style={{ display: page === 'skills' ? 'block' : 'none', height: '100%', overflow: 'auto' }}>
          <SkillList key={workspaceKey} />
        </div>

        {/* Agent Queue page */}
        <div style={{ display: page === 'agent-queue' ? 'block' : 'none', height: '100%', overflow: 'auto' }}>
          <AgentQueuePanel key={workspaceKey} />
        </div>

        {/* Plugins page */}
        <div style={{ display: page === 'plugins' ? 'block' : 'none', height: '100%', overflow: 'auto' }}>
          <PluginList key={workspaceKey} />
        </div>

        {/* Trash page */}
        <div style={{ display: page === 'trash' ? 'block' : 'none', height: '100%', overflow: 'auto' }}>
          <TrashView key={workspaceKey} />
        </div>

        <FloatingChat key={workspaceKey}
          messages={bus.messages}
          sessionID={bus.sessionID}
          sessionActive={bus.sessionActive}
          sessionEnded={bus.sessionEnded}
          connected={busConnected}
          loadingHistory={bus.loadingHistory}
          onCreateSession={bus.createSession}
          onJoinSession={bus.joinSession}
          onSendMessage={bus.sendMessage}
          onSendBlocks={handleSendBlocks}
          onClearMessages={() => bus.sendMessage('/clear')}
          pendingPermissions={pendingPermissions.length}
          onPermissionResponse={(approved) => sendPermissionResponse(approved)}
          permissionMode={permissionMode}
          onTogglePermissionMode={() => setPermissionMode(prev => prev === 'auto' ? 'restricted' : 'auto')}
        />
      </div>
    </div>

    {/* Workspace Manager Modal */}
    {showWorkspaceManager && (
      <div
        onClick={() => setShowWorkspaceManager(false)}
        style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)',
          display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 2000,
        }}
      >
        <div
          onClick={(e) => e.stopPropagation()}
          style={{
            background: '#fff', borderRadius: '16px', padding: '28px',
            width: '480px', maxWidth: '90vw',
            boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
            <h3 style={{ margin: 0 }}>{t('manageWorkspaces')}</h3>
            <button onClick={() => setShowWorkspaceManager(false)} style={{
              width: '32px', height: '32px', borderRadius: '50%', border: 'none',
              background: '#f5f5f5', cursor: 'pointer', fontSize: '1em',
              display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#666',
            }}>✕</button>
          </div>

          {/* Tabs */}
          <div style={{ display: 'flex', gap: '4px', marginBottom: '16px', borderBottom: '1px solid #eee' }}>
            <button
              onClick={() => setWsManagerTab('workspaces')}
              style={{
                padding: '8px 16px', border: 'none', background: 'none',
                cursor: 'pointer', fontSize: '0.9em', color: wsManagerTab === 'workspaces' ? '#1976d2' : '#999',
                borderBottom: wsManagerTab === 'workspaces' ? '2px solid #1976d2' : '2px solid transparent',
                fontWeight: wsManagerTab === 'workspaces' ? 600 : 400,
              }}
            >
              {t('workspaceLabel')}
            </button>
            <button
              onClick={() => setWsManagerTab('members')}
              style={{
                padding: '8px 16px', border: 'none', background: 'none',
                cursor: 'pointer', fontSize: '0.9em', color: wsManagerTab === 'members' ? '#1976d2' : '#999',
                borderBottom: wsManagerTab === 'members' ? '2px solid #1976d2' : '2px solid transparent',
                fontWeight: wsManagerTab === 'members' ? 600 : 400,
              }}
            >
              {lang === 'zh' ? '成员' : 'Members'}
            </button>
            {(auth.workspace_role === 'admin' || auth.workspace_role === 'owner') && (
              <button
                onClick={() => { setWsManagerTab('users'); fetchUsers(); }}
                style={{
                  padding: '8px 16px', border: 'none', background: 'none',
                  cursor: 'pointer', fontSize: '0.9em', color: wsManagerTab === 'users' ? '#1976d2' : '#999',
                  borderBottom: wsManagerTab === 'users' ? '2px solid #1976d2' : '2px solid transparent',
                  fontWeight: wsManagerTab === 'users' ? 600 : 400,
                }}
              >
                {lang === 'zh' ? '用户管理' : 'Users'}
              </button>
            )}
          </div>

          {wsManagerTab === 'workspaces' ? (
          <>
          {/* Workspace list */}
          <div style={{ marginBottom: '16px', maxHeight: '300px', overflow: 'auto' }}>
            {workspaces.map((ws) => (
              <div key={ws.id} style={{
                display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                padding: '10px 12px', borderRadius: '8px', marginBottom: '6px',
                background: '#f9f9f9',
              }}>
                <div>
                  <div style={{ fontWeight: 500, fontSize: '0.95em' }}>{ws.name}</div>
                  {ws.description && <div style={{ fontSize: '0.8em', color: '#999' }}>{ws.description}</div>}
                </div>
                {ws.name === 'Default' ? (
                  <span style={{
                    padding: '4px 10px', borderRadius: '4px', background: '#e8e8e8',
                    color: '#999', fontSize: '0.75em',
                  }}>{t('workspaceDefaultName')}</span>
                ) : (
                  <button
                    onClick={() => handleWsDeleteClick(ws.id)}
                    style={{
                      padding: '4px 12px', borderRadius: '4px', border: '1px solid #e0e0e0',
                      background: '#fff', cursor: 'pointer', color: '#c62828', fontSize: '0.8em',
                    }}
                  >
                    {t('taskDelete')}
                  </button>
                )}
              </div>
            ))}
          </div>

          {/* Add workspace form */}
          <div style={{ borderTop: '1px solid #eee', paddingTop: '16px' }}>
            <input
              placeholder={t('workspaceName')}
              value={newWsName}
              onChange={(e) => setNewWsName(e.target.value)}
              style={{
                width: '100%', padding: '8px 10px', borderRadius: '6px', border: '1px solid #ddd',
                fontSize: '0.9em', boxSizing: 'border-box', marginBottom: '8px',
              }}
            />
            <input
              placeholder={t('workspaceDescription')}
              value={newWsDesc}
              onChange={(e) => setNewWsDesc(e.target.value)}
              style={{
                width: '100%', padding: '8px 10px', borderRadius: '6px', border: '1px solid #ddd',
                fontSize: '0.9em', boxSizing: 'border-box', marginBottom: '8px',
              }}
            />
            <button
              onClick={handleCreateWorkspace}
              style={{
                width: '100%', padding: '8px', borderRadius: '6px', border: 'none',
                background: '#1976d2', color: '#fff', cursor: 'pointer', fontSize: '0.9em',
              }}
            >
              + {t('addWorkspace')}
            </button>
          </div>
          </>
          ) : wsManagerTab === 'members' ? (
            <WorkspaceMembers workspaceId={localStorage.getItem('workspace_id') || ''} />
          ) : (
            <>
            {/* User management tab */}
            <div style={{ maxHeight: '350px', overflow: 'auto' }}>
              {userList.length === 0 ? (
                <div style={{ textAlign: 'center', color: '#999', padding: '24px', fontSize: '0.9em' }}>
                  {lang === 'zh' ? '暂无用户' : 'No users'}
                </div>
              ) : (
                userList.map((u) => (
                  <div key={u.id} style={{
                    display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                    padding: '10px 12px', borderRadius: '8px', marginBottom: '6px',
                    background: '#f9f9f9',
                  }}>
                    <div>
                      <div style={{ fontWeight: 500, fontSize: '0.95em' }}>{u.username}</div>
                      <div style={{ fontSize: '0.8em', color: '#999' }}>
                        {u.email} · {new Date(u.created_at).toLocaleDateString()}
                      </div>
                    </div>
                    {u.id !== auth.user?.id && (
                      <button
                        onClick={() => handleUserDeleteClick(u.id, u.email)}
                        style={{
                          padding: '4px 12px', borderRadius: '4px', border: '1px solid #e0e0e0',
                          background: '#fff', cursor: 'pointer', color: '#c62828', fontSize: '0.8em',
                        }}
                      >
                        {t('taskDelete')}
                      </button>
                    )}
                  </div>
                ))
              )}
            </div>
            </>
          )}
        </div>
      </div>
    )}
    {/* Workspace delete verification modal */}
    {wsDeleteVerify && (
      <div
        onClick={() => { setWsDeleteVerify(null); setWsVerifyError(false); }}
        style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)',
          display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 2100,
        }}
      >
        <div
          onClick={(e) => e.stopPropagation()}
          style={{
            background: '#fff', borderRadius: '12px', padding: '28px',
            width: '360px', maxWidth: '90vw',
            boxShadow: '0 8px 32px rgba(0,0,0,0.2)', textAlign: 'center',
          }}
        >
          <h3 style={{ margin: '0 0 8px', color: '#333' }}>{t('taskConfirmDelete')}</h3>
          <p style={{ color: '#666', fontSize: '0.9em', marginBottom: '20px' }}>
            {lang === 'zh' ? '请回答以下验证问题：' : 'Answer the following to confirm:'}
          </p>
          <div style={{ fontSize: '1.4em', fontWeight: 700, color: '#333', marginBottom: '16px' }}>
            {wsDeleteVerify.a} {wsDeleteVerify.op} {wsDeleteVerify.b} = ?
          </div>
          <input
            value={wsVerifyInput}
            onChange={(e) => { setWsVerifyInput(e.target.value); setWsVerifyError(false); }}
            onKeyDown={(e) => { if (e.key === 'Enter') handleWsDeleteConfirm(); }}
            style={{
              width: '100%', padding: '10px', borderRadius: '6px',
              border: wsVerifyError ? '1px solid #c62828' : '1px solid #ddd',
              fontSize: '1.1em', textAlign: 'center', boxSizing: 'border-box', outline: 'none',
              marginBottom: '8px',
            }}
            autoFocus
          />
          {wsVerifyError && (
            <div style={{ color: '#c62828', fontSize: '0.85em', marginBottom: '8px' }}>
              {lang === 'zh' ? '答案错误，请重试' : 'Wrong answer, try again'}
            </div>
          )}
          <div style={{ display: 'flex', gap: '10px', justifyContent: 'center', marginTop: '12px' }}>
            <button
              onClick={() => { setWsDeleteVerify(null); setWsVerifyError(false); }}
              style={{
                padding: '10px 20px', borderRadius: '6px', border: '1px solid #ddd',
                background: '#fff', cursor: 'pointer', color: '#666', fontSize: '0.95em',
              }}
            >
              {t('cancel')}
            </button>
            <button
              onClick={handleWsDeleteConfirm}
              style={{
                padding: '10px 20px', borderRadius: '6px', border: 'none',
                background: '#c62828', color: '#fff', cursor: 'pointer', fontSize: '0.95em',
              }}
            >
              {t('taskDelete')}
            </button>
          </div>
        </div>
      </div>
    )}

    {/* User delete verification modal */}
    {userDeleteVerify && (
      <div
        onClick={() => setUserDeleteVerify(null)}
        style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)',
          display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 2100,
        }}
      >
        <div
          onClick={(e) => e.stopPropagation()}
          style={{
            background: '#fff', borderRadius: '12px', padding: '28px',
            width: '360px', maxWidth: '90vw',
            boxShadow: '0 8px 32px rgba(0,0,0,0.2)', textAlign: 'center',
          }}
        >
          <h3 style={{ margin: '0 0 8px', color: '#333' }}>
            {lang === 'zh' ? '删除用户' : 'Delete User'}
          </h3>
          <p style={{ color: '#666', fontSize: '0.9em', marginBottom: '8px' }}>
            {lang === 'zh' ? `确定删除用户 ${userDeleteVerify.email}？` : `Delete user ${userDeleteVerify.email}?`}
          </p>
          <p style={{ color: '#999', fontSize: '0.85em', marginBottom: '20px' }}>
            {lang === 'zh' ? '此操作将删除该用户及其所有数据，不可恢复。请回答验证问题：' : 'This permanently removes the user and all their data.'}
          </p>
          <div style={{ fontSize: '1.4em', fontWeight: 700, color: '#333', marginBottom: '16px' }}>
            {userDeleteVerify.a} {userDeleteVerify.op} {userDeleteVerify.b} = ?
          </div>
          <input
            value={userVerifyInput}
            onChange={(e) => setUserVerifyInput(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') handleUserDeleteConfirm(); }}
            style={{
              width: '100%', padding: '10px', borderRadius: '6px', border: '1px solid #ddd',
              fontSize: '1.1em', textAlign: 'center', boxSizing: 'border-box', outline: 'none',
              marginBottom: '8px',
            }}
            autoFocus
          />
          <div style={{ display: 'flex', gap: '10px', justifyContent: 'center', marginTop: '12px' }}>
            <button
              onClick={() => setUserDeleteVerify(null)}
              style={{
                padding: '10px 20px', borderRadius: '6px', border: '1px solid #ddd',
                background: '#fff', cursor: 'pointer', color: '#666', fontSize: '0.95em',
              }}
            >
              {t('cancel')}
            </button>
            <button
              onClick={handleUserDeleteConfirm}
              style={{
                padding: '10px 20px', borderRadius: '6px', border: 'none',
                background: '#c62828', color: '#fff', cursor: 'pointer', fontSize: '0.95em',
              }}
            >
              {t('taskDelete')}
            </button>
          </div>
        </div>
      </div>
    )}

    {/* In-app notification toast */}
    {toast && (
      <div
        onClick={() => setToast(null)}
        style={{
          position: 'fixed',
          top: '20px',
          right: '20px',
          zIndex: 10000,
          background: '#323232',
          color: '#fff',
          padding: '14px 20px',
          borderRadius: '10px',
          boxShadow: '0 8px 32px rgba(0,0,0,0.3)',
          maxWidth: '400px',
          cursor: 'pointer',
        }}
      >
        <div style={{ fontWeight: 600, fontSize: '0.9em', marginBottom: '4px' }}>{toast.title}</div>
        <div style={{ fontSize: '0.82em', opacity: 0.9 }}>{toast.message}</div>
      </div>
    )}
      {showAddNode && <AddNodeDialog onClose={() => setShowAddNode(false)} />}
    </WorkspaceContext.Provider>
  );
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '10px',
  borderRadius: '6px',
  border: '1px solid #ddd',
  fontSize: '1em',
  boxSizing: 'border-box',
};

const buttonStyle: React.CSSProperties = {
  width: '100%',
  padding: '12px',
  background: '#1976d2',
  color: '#fff',
  border: 'none',
  borderRadius: '6px',
  cursor: 'pointer',
  fontSize: '1em',
  fontWeight: 600,
};

export default App;
