import { useState, useEffect, useCallback } from 'react';
import { useLang } from '../i18n/context';
import { agents as agentsApi, nodes as nodesApi } from '../api/client';
import type { Node, Agent } from '../types';

interface NodeListProps {
  nodes: Node[];
  onSelect?: (node: Node) => void;
}

export function NodeList({ nodes, onSelect }: NodeListProps) {
  const { t } = useLang();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [agentMap, setAgentMap] = useState<Record<string, Agent[]>>({});
  const [scanning, setScanning] = useState<string | null>(null);
  const [removing, setRemoving] = useState<string | null>(null);
  const [managing, setManaging] = useState<string | null>(null);
  const [showOffline, setShowOffline] = useState(true);

  const filteredNodes = showOffline ? nodes : nodes.filter(n => n.status === 'online' || n.status === 'busy');

  const handleRemove = useCallback(async (nodeID: string, nodeName: string, e: React.MouseEvent) => {
    e.stopPropagation();
    if (!window.confirm(t('nodeRemoveConfirm'))) return;
    setRemoving(nodeID);
    try {
      await nodesApi.remove(nodeID);
    } catch {
      setRemoving(null);
    }
  }, [t]);

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

  // Fetch agents when a node is expanded
  useEffect(() => {
    if (expandedId && !agentMap[expandedId]) {
      agentsApi.list(expandedId).then((data) => {
        setAgentMap((prev) => ({ ...prev, [expandedId]: data.agents }));
      }).catch(() => {});
    }
  }, [expandedId, agentMap]);

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
    const key = `start:${nodeID}`;
    setManaging(key);
    try {
      await nodesApi.start(nodeID);
      // Leave button showing "启动中..." until WebSocket updates node status
    } catch {
      setManaging(null);
    }
  }, []);

  const handleStop = useCallback(async (nodeID: string, e: React.MouseEvent) => {
    e.stopPropagation();
    const key = `stop:${nodeID}`;
    setManaging(key);
    try {
      await nodesApi.stop(nodeID);
      // Leave button showing "停止中..." until WebSocket updates node status
    } catch {
      setManaging(null);
    }
  }, []);

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

                {/* Start/Stop buttons at bottom-right */}
                {node.can_manage ? (
                  <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '12px' }}>
                    {node.can_manage && node.status === 'offline' && (
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
                    )}
                    {node.can_manage && node.status !== 'offline' && (
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
                ) : null}
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
                        <span style={{ fontWeight: 500 }}>{agent.name}</span>
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
              </div>
            )}
          </div>
        );
      })}
    </div>
    </div>
  );
}
