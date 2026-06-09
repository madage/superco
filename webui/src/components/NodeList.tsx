import { useState, useEffect, useCallback } from 'react';
import { useLang } from '../i18n/context';
import { agents as agentsApi, nodes as nodesApi, agentQueue as agentQueueApi, agentProfiles as agentProfilesApi } from '../api/client';
import { useResourceSync } from '../hooks/useResourceSync';
import type { Node, Agent, AgentProfile } from '../types';
import { MathConfirmDialog } from './MathConfirmDialog';

interface NodeListProps {
  nodes: Node[];
  onSelect?: (node: Node) => void;
}

export function NodeList({ nodes, onSelect }: NodeListProps) {
  const { t } = useLang();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [agentMap, setAgentMap] = useState<Record<string, Agent[]>>({});
  const [profileMap, setProfileMap] = useState<Record<string, AgentProfile[]>>({});
  const [scanning, setScanning] = useState<string | null>(null);
  const [removing, setRemoving] = useState<string | null>(null);
  const [managing, setManaging] = useState<string | null>(null);
  const [showOffline, setShowOffline] = useState(true);
  const [errorDialog, setErrorDialog] = useState<string | null>(null);
  const [commandDialog, setCommandDialog] = useState<{command: string; command_ps1: string} | null>(null);
  const [mathConfirmAction, setMathConfirmAction] = useState<{ type: 'stop' | 'remove'; nodeID: string } | null>(null);
  const [workingAgentIds, setWorkingAgentIds] = useState<Set<string>>(new Set());

  const fetchWorkingAgents = useCallback(async () => {
    try {
      const [queueRes, profilesRes] = await Promise.all([
        agentQueueApi.list(),
        agentProfilesApi.list(),
      ]);
      const activeProfileIds = new Set(
        queueRes.queue
          .filter(q => q.status === 'processing')
          .map(q => q.agent_profile_id)
      );
      const working = new Set<string>();
      for (const p of profilesRes.profiles) {
        if (activeProfileIds.has(p.id) && p.agent_id) {
          working.add(p.agent_id);
        }
      }
      setWorkingAgentIds(working);
    } catch {
      // silently fail
    }
  }, []);

  useEffect(() => { fetchWorkingAgents(); }, [fetchWorkingAgents]);

  useResourceSync('task_agent_queue', fetchWorkingAgents);

  const filteredNodes = showOffline ? nodes : nodes.filter(n => n.status === 'online' || n.status === 'busy');

  const handleRemove = useCallback((nodeID: string, _nodeName: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setMathConfirmAction({ type: 'remove', nodeID });
  }, []);

  const statusLabel: Record<string, string> = {
    online: t('nodeOnline'),
    offline: t('nodeOffline'),
    busy: t('nodeBusy'),
  };

  const statusColor: Record<string, string> = {
    online: '#4caf50',
    offline: '#9e9e9e',
    busy: '#ff9800',
  };

  // Fetch agents and profiles when a node is expanded
  useEffect(() => {
    if (expandedId) {
      if (!agentMap[expandedId]) {
        agentsApi.list(expandedId).then((data) => {
          setAgentMap((prev) => ({ ...prev, [expandedId]: data.agents }));
        }).catch(() => {});
      }
      if (!profileMap[expandedId]) {
        agentProfilesApi.list().then((data) => {
          const nodeProfiles = data.profiles.filter(p => p.node_id === expandedId);
          setProfileMap((prev) => ({ ...prev, [expandedId]: nodeProfiles }));
        }).catch(() => {});
      }
    }
  }, [expandedId, agentMap, profileMap]);

  const handleScan = useCallback(async (nodeID: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setScanning(nodeID);
    try {
      await nodesApi.scan(nodeID);
      // Re-fetch agents after short delay
      setTimeout(async () => {
        const data = await agentsApi.list(nodeID);
        setAgentMap((prev) => ({ ...prev, [nodeID]: data.agents }));
        setScanning(null);
      }, 2000);
    } catch {
      setScanning(null);
    }
  }, []);

  const handleStart = useCallback(async (nodeID: string, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      const res = await nodesApi.start(nodeID);
      if (res.command) {
        setCommandDialog({ command: res.command, command_ps1: res.command_ps1 || res.command });
      }
    } catch (err) {
      setErrorDialog(err instanceof Error ? err.message : 'Start failed');
    }
  }, [t]);

  const handleStop = useCallback((nodeID: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setMathConfirmAction({ type: 'stop', nodeID });
  }, []);

  const handleMathConfirm = useCallback(async () => {
    if (!mathConfirmAction) return;
    const { type, nodeID } = mathConfirmAction;
    setMathConfirmAction(null);
    if (type === 'stop') {
      const key = `stop:${nodeID}`;
      setManaging(key);
      try {
        await nodesApi.stop(nodeID);
      } catch (err) {
        setErrorDialog(err instanceof Error ? err.message : 'Stop failed');
        setManaging(null);
      }
    } else {
      setRemoving(nodeID);
      try {
        await nodesApi.remove(nodeID);
      } catch (err) {
        setErrorDialog(err instanceof Error ? err.message : t('nodeNoPermission'));
        setRemoving(null);
      }
    }
  }, [mathConfirmAction, t]);

  const handleToggleAgent = useCallback(async (agent: Agent, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await agentsApi.toggle(agent.id, !agent.enabled);
      setAgentMap((prev) => ({
        ...prev,
        [agent.node_id]: (prev[agent.node_id] || []).map((a) =>
          a.id === agent.id ? { ...a, enabled: !a.enabled } : a,
        ),
      }));
    } catch {}
  }, []);

  if (nodes.length === 0) {
    return <div className="empty">{t('noNodes')}</div>;
  }

  return (
    <div>
      <style>{`@keyframes agent-spin { to { transform: rotate(360deg); } }`}</style>
      {/* Error dialog modal */}
      {errorDialog && (
        <div
          style={{
            position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
            background: 'rgba(0,0,0,0.5)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            zIndex: 1000,
          }}
          onClick={() => setErrorDialog(null)}
        >
          <div
            style={{
              background: '#fff',
              borderRadius: '16px',
              padding: '40px',
              width: '420px',
              maxWidth: '90vw',
              boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
              textAlign: 'center',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div style={{ fontSize: '3em', marginBottom: '16px', color: '#e53935' }}>✕</div>
            <h3 style={{ margin: '0 0 12px', color: '#1a1a2e', fontSize: '1.2em' }}>Error</h3>
            <p style={{ margin: '0 0 24px', color: '#666', fontSize: '0.95em', lineHeight: 1.5 }}>{errorDialog}</p>
            <button
              onClick={() => setErrorDialog(null)}
              style={{
                padding: '10px 32px',
                background: '#1976d2',
                color: '#fff',
                border: 'none',
                borderRadius: '8px',
                cursor: 'pointer',
                fontSize: '0.95em',
                fontWeight: 600,
              }}
            >
              OK
            </button>
          </div>
        </div>
      )}
      {/* Start command dialog */}
      {commandDialog && (
        <div
          style={{
            position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
            background: 'rgba(0,0,0,0.5)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            zIndex: 1000,
          }}
          onClick={() => setCommandDialog(null)}
        >
          <div
            style={{
              background: '#fff',
              borderRadius: '16px',
              padding: '32px',
              width: '640px',
              maxWidth: '90vw',
              maxHeight: '80vh',
              overflow: 'auto',
              boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <h3 style={{ margin: '0 0 16px', color: '#1a1a2e', fontSize: '1.2em' }}>{t('startCommandTitle')}</h3>
            <p style={{ margin: '0 0 16px', color: '#666', fontSize: '0.9em' }}>{t('startCommandHint')}</p>

            {/* macOS / Linux */}
            <div style={{ marginBottom: '16px' }}>
              <div style={{ fontSize: '0.85em', fontWeight: 600, color: '#555', marginBottom: '6px' }}>{t('startCommandMac')}</div>
              <div style={{
                background: '#1a1a2e', color: '#e0e0e0', padding: '12px 16px',
                borderRadius: '8px', fontSize: '0.85em', fontFamily: 'monospace',
                wordBreak: 'break-all', lineHeight: 1.5, position: 'relative',
              }}>
                <div>{commandDialog.command}</div>
                <button
                  onClick={() => { navigator.clipboard.writeText(commandDialog.command); }}
                  style={{
                    position: 'absolute', top: '8px', right: '8px',
                    padding: '4px 10px', background: '#333', color: '#fff',
                    border: 'none', borderRadius: '4px', cursor: 'pointer', fontSize: '0.8em',
                  }}
                >
                  {t('copyCommand')}
                </button>
              </div>
            </div>

            {/* Windows (PowerShell) */}
            <div style={{ marginBottom: '20px' }}>
              <div style={{ fontSize: '0.85em', fontWeight: 600, color: '#555', marginBottom: '6px' }}>{t('startCommandWindows')}</div>
              <div style={{
                background: '#1a1a2e', color: '#e0e0e0', padding: '12px 16px',
                borderRadius: '8px', fontSize: '0.85em', fontFamily: 'monospace',
                wordBreak: 'break-all', lineHeight: 1.5, position: 'relative',
              }}>
                <div>{commandDialog.command_ps1}</div>
                <button
                  onClick={() => { navigator.clipboard.writeText(commandDialog.command_ps1); }}
                  style={{
                    position: 'absolute', top: '8px', right: '8px',
                    padding: '4px 10px', background: '#333', color: '#fff',
                    border: 'none', borderRadius: '4px', cursor: 'pointer', fontSize: '0.8em',
                  }}
                >
                  {t('copyCommand')}
                </button>
              </div>
            </div>

            <button
              onClick={() => setCommandDialog(null)}
              style={{
                padding: '10px 32px',
                background: '#1976d2',
                color: '#fff',
                border: 'none',
                borderRadius: '8px',
                cursor: 'pointer',
                fontSize: '0.95em',
                fontWeight: 600,
                float: 'right',
              }}
            >
              OK
            </button>
          </div>
        </div>
      )}
      {/* Math confirm dialog */}
      <MathConfirmDialog
        open={mathConfirmAction !== null}
        title={mathConfirmAction?.type === 'stop' ? t('nodeStop') : t('nodeRemove')}
        description={mathConfirmAction?.type === 'stop' ? t('nodeStopConfirm') : t('nodeRemoveConfirm')}
        confirmLabel={mathConfirmAction?.type === 'stop' ? t('nodeStop') : t('nodeRemove')}
        onConfirm={handleMathConfirm}
        onCancel={() => setMathConfirmAction(null)}
      />
      {/* Filter toggle */}
      <div style={{ marginBottom: '12px', display: 'flex', alignItems: 'center', gap: '8px' }}>
        <label style={{ fontSize: '0.85em', color: '#666', display: 'flex', alignItems: 'center', gap: '4px', cursor: 'pointer' }}>
          <input
            type="checkbox"
            checked={showOffline}
            onChange={(e) => setShowOffline(e.target.checked)}
          />
          {t('showOffline')}
        </label>
        <span style={{ fontSize: '0.8em', color: '#999' }}>
          {nodes.filter(n => n.status === 'online' || n.status === 'busy').length}/{nodes.length} online
        </span>
      </div>
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))',
        gap: '20px',
      }}>
      {filteredNodes.map((node) => {
        const agents = agentMap[node.id];
        const isExpanded = expandedId === node.id;

        return (
          <div key={node.id} style={{ display: 'flex', flexDirection: 'column' }}>
            <div
              onClick={() => {
                onSelect?.(node);
                setExpandedId(isExpanded ? null : node.id);
              }}
              style={{
                borderRadius: '12px',
                cursor: 'pointer',
                background: '#fff',
                boxShadow: '0 4px 6px rgba(0,0,0,0.1), 0 10px 20px rgba(0,0,0,0.06), 0 2px 4px rgba(0,0,0,0.08)',
                transition: 'transform 0.2s, boxShadow 0.2s',
                display: 'flex',
                flexDirection: 'column',
                minHeight: '200px',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.transform = 'translateY(-4px)';
                e.currentTarget.style.boxShadow = '0 12px 24px rgba(0,0,0,0.15), 0 4px 8px rgba(0,0,0,0.1)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.transform = '';
                e.currentTarget.style.boxShadow = '';
              }}
            >
              <div style={{ padding: '16px', display: 'flex', flexDirection: 'column', flex: 1 }}>
                {/* Top: status dot + name + scan button */}
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '10px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
                    <span style={{
                      width: '10px', height: '10px', borderRadius: '50%',
                      background: statusColor[node.status] || '#999',
                      display: 'inline-block', flexShrink: 0,
                    }} />
                    <div style={{ fontWeight: 600, color: '#1a1a2e', fontSize: '1em', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{node.name}</div>
                  </div>
                  <div style={{ display: 'flex', gap: '6px', flexShrink: 0 }}>
                    {node.status === 'offline' ? (
                      <span style={{ fontSize: '0.75em', color: '#999', whiteSpace: 'nowrap' }}>
                        {t('lastSeen')}: {new Date(node.last_seen).toLocaleString()}
                      </span>
                    ) : (
                    <button
                      onClick={(e) => handleScan(node.id, e)}
                      disabled={scanning === node.id}
                      style={{
                        padding: '3px 10px',
                        background: scanning === node.id ? '#ccc' : '#1976d2',
                        color: '#fff',
                        border: 'none',
                        borderRadius: '6px',
                        cursor: scanning === node.id ? 'not-allowed' : 'pointer',
                        fontSize: '0.75em',
                        fontWeight: 500,
                      }}
                    >
                      {scanning === node.id ? t('scanning') : t('scanAgents')}
                    </button>
                    )}
                    <button
                      onClick={(e) => handleRemove(node.id, node.name, e)}
                      disabled={removing === node.id}
                      style={{
                        padding: '3px 10px',
                        background: removing === node.id ? '#ccc' : '#fff',
                        color: '#e53935',
                        border: '1px solid #e53935',
                        borderRadius: '6px',
                        cursor: removing === node.id ? 'not-allowed' : 'pointer',
                        fontSize: '0.75em',
                        fontWeight: 500,
                      }}
                    >
                      {removing === node.id ? '...' : t('nodeRemove')}
                    </button>
                  </div>
                </div>

                {/* Info rows: side by side */}
                <div style={{ display: 'flex', gap: '12px', fontSize: '0.83em', color: '#888' }}>
                  <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '4px' }}>
                    <div><span style={{ color: '#aaa' }}>OS</span> {node.os} / {node.arch}</div>
                    <div><span style={{ color: '#aaa' }}>{t('maxSessions')}</span> {node.max_sessions}</div>
                  </div>
                  <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '4px' }}>
                    <div>
                      <span style={{
                        fontSize: '0.8em', padding: '1px 8px', borderRadius: '10px',
                        background: node.status === 'online' ? '#e8f5e9' : node.status === 'busy' ? '#fff3e0' : '#f5f5f5',
                        color: statusColor[node.status] || '#999', fontWeight: 500,
                      }}>
                        {statusLabel[node.status] || node.status}
                      </span>
                    </div>
                    <div style={{ color: '#aaa', fontSize: '0.9em', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {new Date(node.last_seen).toLocaleString()}
                    </div>
                  </div>
                </div>

                {/* Start/Stop buttons — always visible, permission checked on click */}
                <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '12px' }}>
                  {node.status === 'offline' ? (
                    <button
                      onClick={(e) => handleStart(node.id, e)}
                      disabled={managing === `start:${node.id}`}
                      style={{
                        padding: '10px 28px',
                        background: managing === `start:${node.id}` ? '#a5d6a7' : '#4caf50',
                        color: '#fff',
                        border: 'none',
                        borderRadius: '8px',
                        cursor: managing === `start:${node.id}` ? 'not-allowed' : 'pointer',
                        fontSize: '0.95em',
                        fontWeight: 600,
                      }}
                    >
                      {managing === `start:${node.id}` ? t('nodeStarting') : t('nodeStart')}
                    </button>
                  ) : (
                    <button
                      onClick={(e) => handleStop(node.id, e)}
                      disabled={managing === `stop:${node.id}`}
                      style={{
                        padding: '10px 28px',
                        background: managing === `stop:${node.id}` ? '#ef9a9a' : '#e53935',
                        color: '#fff',
                        border: 'none',
                        borderRadius: '8px',
                        cursor: managing === `stop:${node.id}` ? 'not-allowed' : 'pointer',
                        fontSize: '0.95em',
                        fontWeight: 600,
                      }}
                    >
                      {managing === `stop:${node.id}` ? t('nodeStopping') : t('nodeStop')}
                    </button>
                  )}
                </div>
              </div>
            </div>

            {/* Expanded agent list */}
            {isExpanded && (
              <div
                style={{
                  margin: '0 0 12px',
                  padding: '12px',
                  background: '#fafafa',
                  border: '1px solid #f0f0f0',
                  borderTop: 'none',
                  borderRadius: '0 0 12px 12px',
                }}
              >
                <div style={{ fontSize: '0.9em', fontWeight: 500, marginBottom: '6px' }}>
                  {t('agents')}:
                </div>
                {!agents ? (
                  <div style={{ fontSize: '0.85em', color: '#999' }}>{t('loading')}...</div>
                ) : agents.length === 0 ? (
                  <div style={{ fontSize: '0.85em', color: '#999' }}>{t('noAgents')}</div>
                ) : (
                  agents.map((agent) => (
                    <div
                      key={agent.id}
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        padding: '4px 8px',
                        margin: '2px 0',
                        background: '#fff',
                        borderRadius: '4px',
                        border: '1px solid #eee',
                      }}
                    >
                      <div>
                        <span style={{ fontWeight: 500 }}>
                          {workingAgentIds.has(agent.id) && (
                            <span
                              title="Agent working..."
                              style={{
                                display: 'inline-block', width: '10px', height: '10px',
                                borderRadius: '50%', border: '2px solid #e0e0e0',
                                borderTopColor: '#ff9800',
                                animation: 'agent-spin 0.8s linear infinite',
                                marginRight: '6px', verticalAlign: 'middle',
                              }}
                            />
                          )}
                          {agent.name}
                        </span>
                        <span style={{ fontSize: '0.8em', color: '#999', marginLeft: '8px' }}>
                          {agent.command} {agent.version ? `(${agent.version})` : ''}
                        </span>
                      </div>
                      <button
                        onClick={(e) => handleToggleAgent(agent, e)}
                        style={{
                          padding: '2px 8px',
                          background: agent.enabled ? '#4caf50' : '#e0e0e0',
                          color: agent.enabled ? '#fff' : '#666',
                          border: 'none',
                          borderRadius: '3px',
                          cursor: 'pointer',
                          fontSize: '0.8em',
                        }}
                      >
                        {agent.enabled ? t('enabled') : t('disabled')}
                      </button>
                    </div>
                  ))
                )}
                {agents && agents.length > 0 && (
                  <div style={{ fontSize: '0.75em', color: '#bbb', marginTop: '4px' }}>
                    {t('agentHint')}
                  </div>
                )}
                {/* Assigned agent profiles */}
                <div style={{ fontSize: '0.9em', fontWeight: 500, marginBottom: '6px', marginTop: '12px' }}>
                  {t('assignedAgents')}:
                </div>
                {!profileMap[node.id] ? (
                  <div style={{ fontSize: '0.85em', color: '#999' }}>{t('loading')}...</div>
                ) : profileMap[node.id].length === 0 ? (
                  <div style={{ fontSize: '0.85em', color: '#999' }}>{t('noAssignedAgents')}</div>
                ) : (
                  profileMap[node.id].map((profile) => (
                    <div
                      key={profile.id}
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        padding: '4px 8px',
                        margin: '2px 0',
                        background: '#fff',
                        borderRadius: '4px',
                        border: '1px solid #eee',
                      }}
                    >
                      <div>
                        <span style={{ fontWeight: 500 }}>
                          {profile.avatar} {profile.name}
                        </span>
                        <span style={{ fontSize: '0.8em', color: '#999', marginLeft: '8px' }}>
                          {profile.agent_id}
                        </span>
                      </div>
                      <span style={{ fontSize: '0.75em', padding: '2px 6px', borderRadius: '4px', background: profile.enabled ? '#e8f5e9' : '#f5f5f5', color: profile.enabled ? '#2e7d32' : '#999' }}>
                        {profile.enabled ? t('enabled') : t('disabled')}
                      </span>
                    </div>
                  ))
                )}
              </div>
            )}
          </div>
        );
      })}
    </div>
    </div>
  );
}
