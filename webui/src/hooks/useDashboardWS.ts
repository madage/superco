import { useEffect, useRef, useState, useCallback } from 'react';
import type { Node, Session } from '../types';

interface DashboardState {
  nodes: Node[];
  sessions: Session[];
  connected: boolean;
}

interface WsMessage {
  type: string;
  payload: Record<string, unknown>;
}

export function useDashboardWS() {
  const [state, setState] = useState<DashboardState>({
    nodes: [],
    sessions: [],
    connected: false,
  });
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>();
  const shouldReconnect = useRef(true);

  const connect = useCallback(() => {
    const token = localStorage.getItem('token');
    if (!token) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const url = `${protocol}//${host}/ws/dashboard?token=${encodeURIComponent(token)}`;

    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      console.log('[DashboardWS] Connected');
      setState((prev) => ({ ...prev, connected: true }));
    };

    ws.onmessage = (event) => {
      try {
        const msg: WsMessage = JSON.parse(event.data);

        switch (msg.type) {
          case 'init': {
            const p = msg.payload as { nodes: Node[]; sessions: Session[] };
            setState({
              nodes: p.nodes || [],
              sessions: p.sessions || [],
              connected: true,
            });
            break;
          }

          case 'node_status': {
            const p = msg.payload as { node_id: string; name: string; status: string };
            setState((prev) => {
              const exists = prev.nodes.find((n) => n.id === p.node_id);
              if (!exists) {
                // Only add new nodes that are actually online — ignore stale offline status
                if (p.status !== 'online') return prev;
                const newNode: Node = {
                  id: p.node_id,
                  user_id: '',
                  name: p.name || p.node_id,
                  os: 'bus',
                  arch: '',
                  status: p.status as Node['status'],
                  version: '',
                  ip: 'bus',
                  max_sessions: 3,
                  last_seen: new Date().toISOString(),
                  created_at: new Date().toISOString(),
                };
                return { ...prev, nodes: [...prev.nodes, newNode] };
              }
              return {
                ...prev,
                nodes: prev.nodes.map((n) =>
                  n.id === p.node_id ? { ...n, status: p.status as Node['status'] } : n,
                ),
              };
            });
            break;
          }

          case 'session_update': {
            const p = msg.payload as {
              id: string;
              status: string;
              prompt?: string;
              workspace?: string;
              node_id?: string;
            };
            setState((prev) => {
              const exists = prev.sessions.find((s) => s.id === p.id);
              if (exists) {
                // Update existing session
                return {
                  ...prev,
                  sessions: prev.sessions.map((s) =>
                    s.id === p.id ? { ...s, status: p.status as Session['status'] } : s,
                  ),
                };
              }
              // New session — prepend (prompt/workspace/node_id only available on create)
              const newSession: Session = {
                id: p.id,
                user_id: '',
                node_id: p.node_id || '',
                status: p.status as Session['status'],
                prompt: p.prompt || '',
                workspace: p.workspace || '',
                created_at: new Date().toISOString(),
                updated_at: new Date().toISOString(),
              };
              return {
                ...prev,
                sessions: [newSession, ...prev.sessions],
              };
            });
            break;
          }
        }
      } catch (e) {
        console.warn('[DashboardWS] Failed to parse message:', e);
      }
    };

    ws.onclose = () => {
      console.log('[DashboardWS] Disconnected');
      setState((prev) => ({ ...prev, connected: false }));
      if (shouldReconnect.current) {
        reconnectTimer.current = setTimeout(connect, 3000);
      }
    };

    ws.onerror = () => {
      ws.close();
    };
  }, []);

  useEffect(() => {
    shouldReconnect.current = true;
    connect();

    return () => {
      shouldReconnect.current = false;
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [connect]);

  return state;
}
