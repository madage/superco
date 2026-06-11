import { useState, useEffect, useRef, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { useLang } from '../i18n/context';
import { useResourceSync } from '../hooks/useResourceSync';
import { notifications as notificationsApi, invitations as invitationsApi } from '../api/client';
import type { AppNotification, PendingInvitation } from '../types';

interface Props {
  onWorkspaceChange?: () => void;
  onOpenTask?: (taskId: string) => void;
}

function timeAgo(dateStr: string, lang: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return lang === 'zh' ? '刚刚' : 'just now';
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d`;
  return new Date(dateStr).toLocaleDateString(lang === 'zh' ? 'zh-CN' : 'en-US', { month: 'short', day: 'numeric' });
}

const notifTypeIcons: Record<string, string> = {
  task_assigned: '📋',
  task_status_changed: '🔄',
  task_comment: '💬',
  task_mention: '@',
};

export default function NotificationBell({ onWorkspaceChange, onOpenTask }: Props) {
  const { t, lang } = useLang();
  const [notifList, setNotifList] = useState<AppNotification[]>([]);
  const [invitations, setInvitations] = useState<PendingInvitation[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<'notifications' | 'invitations'>('notifications');
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [dropdownPos, setDropdownPos] = useState<{ top: number; left: number } | null>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const bellRef = useRef<HTMLButtonElement>(null);

  const fetchNotifications = useCallback(async () => {
    try {
      const [listRes, countRes] = await Promise.all([
        notificationsApi.list(),
        notificationsApi.unreadCount(),
      ]);
      setNotifList(listRes.notifications || []);
      setUnreadCount(countRes.count);
    } catch {
      // silently fail
    }
  }, []);

  const fetchInvitations = useCallback(async () => {
    try {
      const res = await invitationsApi.pending();
      setInvitations(res.invitations || []);
    } catch {
      // silently fail
    }
  }, []);

  const fetchAll = useCallback(async () => {
    setLoading(true);
    await Promise.all([fetchNotifications(), fetchInvitations()]);
    setLoading(false);
  }, [fetchNotifications, fetchInvitations]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  useResourceSync('notifications', fetchNotifications);
  useResourceSync('invitations', fetchInvitations);

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node) &&
        bellRef.current &&
        !bellRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [open]);

  const handleNotifClick = async (n: AppNotification) => {
    if (!n.is_read) {
      try {
        await notificationsApi.markRead(n.id);
        setUnreadCount((c) => Math.max(0, c - 1));
        setNotifList((prev) => prev.map((x) => (x.id === n.id ? { ...x, is_read: true } : x)));
      } catch {}
    }
    setOpen(false);
    if (n.task_id && onOpenTask) {
      onOpenTask(n.task_id);
    }
  };

  const handleMarkAllRead = async () => {
    try {
      const res = await notificationsApi.markAllRead();
      setUnreadCount(0);
      setNotifList((prev) => prev.map((n) => ({ ...n, is_read: true })));
      if (res.count > 0) {
        setMessage(t('notifMarkAllRead'));
        setTimeout(() => setMessage(null), 3000);
      }
    } catch {}
  };

  const handleAccept = async (inv: PendingInvitation) => {
    try {
      const res = await invitationsApi.accept(inv.token);
      if (res.workspace_id) {
        localStorage.setItem('workspace_id', res.workspace_id);
      }
      setInvitations(prev => prev.filter(i => i.id !== inv.id));
      setMessage(t('invitationAccepted'));
      setTimeout(() => setMessage(null), 3000);
      onWorkspaceChange?.();
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Failed to accept');
      setTimeout(() => setMessage(null), 3000);
    }
  };

  const handleDecline = async (inv: PendingInvitation) => {
    try {
      await invitationsApi.decline(inv.token);
      setInvitations(prev => prev.filter(i => i.id !== inv.id));
      setMessage(t('invitationDeclined'));
      setTimeout(() => setMessage(null), 3000);
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Failed to decline');
      setTimeout(() => setMessage(null), 3000);
    }
  };

  const totalBadge = unreadCount + invitations.length;

  const handleToggle = useCallback(() => {
    if (!open && bellRef.current) {
      const rect = bellRef.current.getBoundingClientRect();
      setDropdownPos({ top: rect.bottom + 4, left: rect.left });
    }
    setOpen(!open);
  }, [open]);

  // Recalculate position on window resize while open
  useEffect(() => {
    if (!open) return;
    const onResize = () => {
      if (bellRef.current) {
        const rect = bellRef.current.getBoundingClientRect();
        setDropdownPos({ top: rect.bottom + 4, left: rect.left });
      }
    };
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, [open]);

  return (
    <div style={{ position: 'relative', display: 'inline-block' }}>
      <button
        ref={bellRef}
        onClick={handleToggle}
        title={t('notifInbox')}
        style={{
          background: 'none',
          border: 'none',
          color: '#ccc',
          cursor: 'pointer',
          fontSize: '1.1em',
          padding: '4px',
          position: 'relative',
          lineHeight: 1,
        }}
      >
        🔔
        {totalBadge > 0 && (
          <span
            style={{
              position: 'absolute',
              top: '-2px',
              right: '-6px',
              background: '#f44336',
              color: '#fff',
              borderRadius: '50%',
              width: '16px',
              height: '16px',
              fontSize: '10px',
              fontWeight: 700,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              lineHeight: 1,
            }}
          >
            {totalBadge > 9 ? '9+' : totalBadge}
          </span>
        )}
      </button>

      {message && (
        <div style={{
          position: 'absolute',
          bottom: '-28px',
          left: '50%',
          transform: 'translateX(-50%)',
          background: '#333',
          color: '#fff',
          padding: '4px 10px',
          borderRadius: '4px',
          fontSize: '0.75em',
          whiteSpace: 'nowrap',
          zIndex: 100,
        }}>
          {message}
        </div>
      )}

      {open && dropdownPos && createPortal(
        <div
          ref={dropdownRef}
          style={{
            position: 'fixed',
            top: dropdownPos.top,
            left: dropdownPos.left,
            width: '380px',
            maxHeight: '480px',
            display: 'flex',
            flexDirection: 'column',
            background: '#fff',
            borderRadius: '10px',
            boxShadow: '0 8px 32px rgba(0,0,0,0.2)',
            zIndex: 3000,
            color: '#333',
            overflow: 'hidden',
          }}
        >
          {/* Tabs */}
          <div style={{ display: 'flex', borderBottom: '1px solid #eee' }}>
            <button
              onClick={() => setTab('notifications')}
              style={{
                flex: 1, padding: '10px', border: 'none', background: 'none',
                cursor: 'pointer', fontSize: '0.85em', fontWeight: tab === 'notifications' ? 600 : 400,
                color: tab === 'notifications' ? '#1976d2' : '#999',
                borderBottom: tab === 'notifications' ? '2px solid #1976d2' : '2px solid transparent',
              }}
            >
              {t('notifInbox')}
              {unreadCount > 0 && (
                <span style={{ marginLeft: '6px', background: '#f44336', color: '#fff', borderRadius: '10px', padding: '0 6px', fontSize: '0.8em' }}>
                  {unreadCount}
                </span>
              )}
            </button>
            <button
              onClick={() => setTab('invitations')}
              style={{
                flex: 1, padding: '10px', border: 'none', background: 'none',
                cursor: 'pointer', fontSize: '0.85em', fontWeight: tab === 'invitations' ? 600 : 400,
                color: tab === 'invitations' ? '#1976d2' : '#999',
                borderBottom: tab === 'invitations' ? '2px solid #1976d2' : '2px solid transparent',
              }}
            >
              {t('notifInvitations')}
              {invitations.length > 0 && (
                <span style={{ marginLeft: '6px', background: '#f44336', color: '#fff', borderRadius: '10px', padding: '0 6px', fontSize: '0.8em' }}>
                  {invitations.length}
                </span>
              )}
            </button>
          </div>

          {/* Notifications tab */}
          {tab === 'notifications' && (
            <>
              {/* Mark all read button */}
              {unreadCount > 0 && (
                <div style={{ padding: '8px 12px', borderBottom: '1px solid #f5f5f5', textAlign: 'right' }}>
                  <button
                    onClick={handleMarkAllRead}
                    style={{
                      background: 'none', border: 'none', color: '#1976d2',
                      cursor: 'pointer', fontSize: '0.78em', fontWeight: 500,
                    }}
                  >
                    {t('notifMarkAllRead')}
                  </button>
                </div>
              )}

              <div style={{ overflow: 'auto', flex: 1 }}>
                {loading && notifList.length === 0 ? (
                  <div style={{ padding: '32px', textAlign: 'center', color: '#999', fontSize: '0.85em' }}>
                    {t('loading')}...
                  </div>
                ) : notifList.length === 0 ? (
                  <div style={{ padding: '32px', textAlign: 'center', color: '#999', fontSize: '0.85em' }}>
                    {t('notifEmpty')}
                  </div>
                ) : (
                  notifList.map((n) => (
                    <div
                      key={n.id}
                      onClick={() => handleNotifClick(n)}
                      style={{
                        padding: '10px 14px',
                        borderBottom: '1px solid #f5f5f5',
                        cursor: 'pointer',
                        background: n.is_read ? '#fff' : '#f0f7ff',
                        transition: 'background 0.15s',
                        display: 'flex',
                        gap: '10px',
                        alignItems: 'flex-start',
                      }}
                      onMouseEnter={(e) => { e.currentTarget.style.background = '#f5f5f5'; }}
                      onMouseLeave={(e) => { e.currentTarget.style.background = n.is_read ? '#fff' : '#f0f7ff'; }}
                    >
                      <span style={{ fontSize: '1.1em', flexShrink: 0, marginTop: '1px' }}>
                        {notifTypeIcons[n.type] || '🔔'}
                      </span>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontWeight: n.is_read ? 400 : 600, fontSize: '0.85em', color: '#333', marginBottom: '2px' }}>
                          {n.title}
                        </div>
                        <div style={{ fontSize: '0.8em', color: '#888', lineHeight: 1.3, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {n.message}
                        </div>
                        <div style={{ fontSize: '0.72em', color: '#aaa', marginTop: '4px' }}>
                          {timeAgo(n.created_at, lang)}
                        </div>
                      </div>
                      {!n.is_read && (
                        <span style={{ width: '8px', height: '8px', borderRadius: '50%', background: '#1976d2', flexShrink: 0, marginTop: '6px' }} />
                      )}
                    </div>
                  ))
                )}
              </div>
            </>
          )}

          {/* Invitations tab */}
          {tab === 'invitations' && (
            <div style={{ overflow: 'auto', flex: 1 }}>
              {loading && invitations.length === 0 ? (
                <div style={{ padding: '32px', textAlign: 'center', color: '#999', fontSize: '0.85em' }}>
                  {t('loading')}...
                </div>
              ) : invitations.length === 0 ? (
                <div style={{ padding: '32px', textAlign: 'center', color: '#999', fontSize: '0.85em' }}>
                  {t('noPendingInvitations')}
                </div>
              ) : (
                invitations.map((inv) => (
                  <div key={inv.id} style={{
                    padding: '12px 16px',
                    borderBottom: '1px solid #f5f5f5',
                  }}>
                    <div style={{ fontSize: '0.85em', marginBottom: '8px', lineHeight: 1.4 }}>
                      <span style={{ fontWeight: 500 }}>{inv.inviter_name}</span>
                      {lang === 'zh' ? ' 邀请你加入工作区 ' : ' invited you to '}
                      <span style={{ fontWeight: 500 }}>{inv.workspace_name}</span>
                    </div>
                    <div style={{ fontSize: '0.75em', color: '#999', marginBottom: '8px' }}>
                      {new Date(inv.created_at).toLocaleDateString(lang === 'zh' ? 'zh-CN' : 'en-US', {
                        month: 'short',
                        day: 'numeric',
                        hour: '2-digit',
                        minute: '2-digit',
                      })}
                    </div>
                    <div style={{ display: 'flex', gap: '8px' }}>
                      <button
                        onClick={() => handleAccept(inv)}
                        style={{
                          flex: 1,
                          padding: '6px 12px',
                          borderRadius: '6px',
                          border: 'none',
                          background: '#1976d2',
                          color: '#fff',
                          cursor: 'pointer',
                          fontSize: '0.82em',
                          fontWeight: 600,
                        }}
                      >
                        {t('accept')}
                      </button>
                      <button
                        onClick={() => handleDecline(inv)}
                        style={{
                          flex: 1,
                          padding: '6px 12px',
                          borderRadius: '6px',
                          border: '1px solid #ddd',
                          background: '#fff',
                          color: '#666',
                          cursor: 'pointer',
                          fontSize: '0.82em',
                        }}
                      >
                        {t('decline')}
                      </button>
                    </div>
                  </div>
                ))
              )}
            </div>
          )}
        </div>
      , document.body)}
    </div>
  );
}
