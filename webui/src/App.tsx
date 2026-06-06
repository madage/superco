import { useState, useCallback, useRef, useEffect } from 'react';
import { useMessageBus, type Envelope, type ContentBlock } from './hooks/useMessageBus';
import { NodeList } from './components/NodeList';
import { AgentList } from './components/AgentList';
import { TaskBoard } from './components/TaskBoard';
import { ProjectList } from './components/ProjectList';
import { TrashView } from './components/TrashView';
import { FloatingChat } from './components/FloatingChat';
import { LangSwitcher } from './components/LangSwitcher';
import { useDashboardWS } from './hooks/useDashboardWS';
import { useLang } from './i18n/context';
import { auth as authApi, workspaces as workspacesApi } from './api/client';
import type { Node, Session, AuthState, Workspace } from './types';

type Page = 'nodes' | 'tasks' | 'projects' | 'agents' | 'trash';

function App() {
  const { t, lang } = useLang();
  const [auth, setAuth] = useState<AuthState>(() => {
    const token = localStorage.getItem('token');
    const raw = localStorage.getItem('user');
    const wsId = localStorage.getItem('workspace_id');
    if (token && raw) {
      try {
        return { token, user: JSON.parse(raw), workspace_id: wsId };
      } catch {
        // corrupted user data, ignore
      }
    }
    return { token: null, user: null, workspace_id: null };
  });
  const [page, setPage] = useState<Page>('nodes');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [isRegister, setIsRegister] = useState(false);
  const [authError, setAuthError] = useState<string | null>(null);
  const { nodes, sessions, connected: dashboardConnected } = useDashboardWS();

  // Workspace state
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspaceKey, setWorkspaceKey] = useState(0);
  const [showWorkspaceManager, setShowWorkspaceManager] = useState(false);
  const [newWsName, setNewWsName] = useState('');
  const [newWsDesc, setNewWsDesc] = useState('');

  // Workspace delete verification
  const [wsDeleteVerify, setWsDeleteVerify] = useState<{
    id: string; a: number; b: number; op: '+' | '-'; answer: number;
  } | null>(null);
  const [wsVerifyInput, setWsVerifyInput] = useState('');
  const [wsVerifyError, setWsVerifyError] = useState(false);

  // Fetch workspaces when authenticated
  useEffect(() => {
    if (auth.token) {
      workspacesApi.list().then((res) => setWorkspaces(res.workspaces)).catch(() => {});
    }
  }, [auth.token]);

  // Queue of pending permission requests from claude
  const [pendingPermissions, setPendingPermissions] = useState<Envelope[]>([]);

  // Permission mode: 'auto' (auto-approve) or 'restricted' (require user input)
  const [permissionMode, setPermissionMode] = useState<'auto' | 'restricted'>('auto');
  const permissionModeRef = useRef(permissionMode);
  permissionModeRef.current = permissionMode;

  // Message Bus — only connect when authenticated
  const bus = useMessageBus({
    userID: auth.user?.id || 'anonymous',
    onMessage: useCallback((env: Envelope) => {
      if (env.type === 'permission.request') {
        if (permissionModeRef.current === 'auto') {
          // Auto-approve: send response directly, don't add to pending queue
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
            // Session not ready yet — queue for later handling
            setPendingPermissions((prev) => [...prev, env]);
          }
        } else {
          setPendingPermissions((prev) => [...prev, env]);
        }
      }
    }, []),
  });

  // Track sessionID via ref so the stable onMessage callback can access it
  const sessionIDRef = useRef<string | null>(null);
  sessionIDRef.current = bus.sessionID;

  const sendPermissionResponse = useCallback((approved: boolean) => {
    const queue = pendingPermissions;
    if (queue.length === 0 || !bus.sessionID) return;
    // Approve (or deny) ALL pending requests in one batch
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
    // Join existing session on the bus and load historical messages
    bus.joinSession(session.id);
  }, [bus]);

  const handleSessionCreated = useCallback(async (sessionID: string) => {
    // Session was created via REST API — join the bus session for real-time messaging
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
      // If deleted current workspace, switch to first available
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
            width: '360px',
          }}
        >
          <h1 style={{ margin: '0 0 24px', textAlign: 'center', color: '#1a1a2e' }}>{t('appTitle')}</h1>
          <p style={{ textAlign: 'center', color: '#666', marginBottom: '24px' }}>
            {t('appSubtitle')}
          </p>

          <form onSubmit={async (e) => {
            e.preventDefault();
            setAuthError(null);
            try {
              const fn = isRegister ? authApi.register : authApi.login;
              const data = await fn(username, password);
              localStorage.setItem('token', data.token);
              localStorage.setItem('user', JSON.stringify(data.user));
              if (data.workspace_id) {
                localStorage.setItem('workspace_id', data.workspace_id);
              }
              setAuth({ token: data.token, user: data.user, workspace_id: data.workspace_id || null });
            } catch (err) {
              setAuthError(err instanceof Error ? err.message : t('authFailed'));
            }
          }}>
            <div style={{ marginBottom: '16px' }}>
              <input
                type="text"
                placeholder={t('username')}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
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
        </div>
      </div>
    );
  }

  const busConnected = bus.connected;
  const hasSession = bus.sessionID !== null;

  return (
    <>
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
          <div style={{ fontSize: '0.85em', color: '#999', marginTop: '4px' }}>
            {auth.user?.username}
          </div>
        </div>

        <nav style={{ display: 'flex', flexDirection: 'column', padding: '8px' }}>
          {(['nodes', 'tasks', 'projects', 'agents', 'trash'] as Page[]).map((p) => (
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
                {p === 'nodes' ? '📡' : p === 'tasks' ? '📋' : p === 'projects' ? '📁' : p === 'agents' ? '🤖' : '🗑'}
              </span>
              {p === 'nodes' ? t('navNodes') : p === 'tasks' ? t('navTasks') : p === 'projects' ? t('navProjects') : p === 'agents' ? t('agents') : t('navTrash')}
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
        </div>

        <div style={{ marginTop: 'auto', padding: '16px', display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <LangSwitcher />
          <button
            onClick={() => {
              localStorage.removeItem('token');
              localStorage.removeItem('user');
              localStorage.removeItem('workspace_id');
              localStorage.removeItem('activeSessionID');
              setAuth({ token: null, user: null, workspace_id: null });
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
          <h2>{t('agentNodes')}</h2>
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
          <TaskBoard key={workspaceKey} />
        </div>

        {/* Projects page */}
        <div style={{ display: page === 'projects' ? 'block' : 'none', height: '100%', overflow: 'auto' }}>
          <ProjectList key={workspaceKey} />
        </div>

        {/* Trash page */}
        <div style={{ display: page === 'trash' ? 'block' : 'none', height: '100%', overflow: 'auto' }}>
          <TrashView key={workspaceKey} />
        </div>

        <FloatingChat
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
            width: '420px', maxWidth: '90vw',
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

          {/* Workspace list */}
          <div style={{ marginBottom: '16px' }}>
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
    </>
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
