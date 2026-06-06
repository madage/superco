import { useState, useRef, useEffect } from 'react';
import type { Task, TaskStatus } from '../types';
import { useLang, type TranslationKey } from '../i18n/context';

const statusColors: Record<TaskStatus, { bg: string; color: string }> = {
  todo: { bg: '#e0e0e0', color: '#616161' },
  in_progress: { bg: '#bbdefb', color: '#1565c0' },
  blocked: { bg: '#d1c4e9', color: '#4527a0' },
  review: { bg: '#ffe0b2', color: '#e65100' },
  done: { bg: '#c8e6c9', color: '#2e7d32' },
};

const statusKeys: Record<TaskStatus, TranslationKey> = {
  todo: 'taskStatusTodo',
  in_progress: 'taskStatusInProgress',
  blocked: 'taskStatusBlocked',
  review: 'taskStatusReview',
  done: 'taskStatusDone',
};

const allStatuses: TaskStatus[] = ['todo', 'in_progress', 'blocked', 'review', 'done'];

interface TaskCardProps {
  task: Task;
  onEdit: (task: Task) => void;
  onDelete: (id: string) => void;
  onStatusChange: (id: string, status: TaskStatus) => void;
  projectsMap?: Record<string, { name: string; color: string }>;
}

export function TaskCard({ task, onEdit, onDelete, onStatusChange, projectsMap }: TaskCardProps) {
  const { t } = useLang();
  const sc = statusColors[task.status];
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    };
    if (menuOpen) {
      document.addEventListener('mousedown', handleClick);
    }
    return () => document.removeEventListener('mousedown', handleClick);
  }, [menuOpen]);

  return (
    <div
      style={{
        background: '#fff',
        borderRadius: '12px',
        padding: '10px 14px',
        boxShadow: '0 4px 6px rgba(0,0,0,0.1), 0 10px 20px rgba(0,0,0,0.06), 0 2px 4px rgba(0,0,0,0.08)',
        transition: 'transform 0.2s, boxShadow 0.2s',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        cursor: 'pointer',
        position: 'relative',
      }}
      onClick={() => onEdit(task)}
      onMouseEnter={(e) => {
        e.currentTarget.style.transform = 'translateY(-4px)';
        e.currentTarget.style.boxShadow = '0 12px 24px rgba(0,0,0,0.15), 0 4px 8px rgba(0,0,0,0.1)';
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.transform = '';
        e.currentTarget.style.boxShadow = '';
      }}
    >
      {/* Top row: menu + project dot + status badge */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div ref={menuRef} style={{ position: 'relative' }}>
          <button
            onClick={(e) => { e.stopPropagation(); setMenuOpen(!menuOpen); }}
            style={{
              background: 'none',
              border: 'none',
              cursor: 'pointer',
              padding: '2px 6px',
              borderRadius: '4px',
              color: '#999',
              fontSize: '1.1em',
              lineHeight: 1,
              fontWeight: 700,
              letterSpacing: '1px',
            }}
          >
            ···
          </button>

          {menuOpen && (
            <div
              style={{
                position: 'absolute',
                top: '100%',
                left: 0,
                zIndex: 100,
                background: '#fff',
                borderRadius: '8px',
                boxShadow: '0 4px 16px rgba(0,0,0,0.15)',
                minWidth: '140px',
                padding: '4px 0',
                border: '1px solid #eee',
              }}
            >
              <div style={{ padding: '6px 12px', fontSize: '0.75em', color: '#999', fontWeight: 500 }}>
                {t('taskStatus')}
              </div>
              {allStatuses.map((s) => (
                <button
                  key={s}
                  onClick={(e) => { e.stopPropagation(); setMenuOpen(false); onStatusChange(task.id, s); }}
                  style={{
                    display: 'block',
                    width: '100%',
                    textAlign: 'left',
                    padding: '6px 12px',
                    border: 'none',
                    background: task.status === s ? '#f0f0f0' : 'transparent',
                    cursor: 'pointer',
                    fontSize: '0.85em',
                    color: task.status === s ? statusColors[s].color : '#333',
                    fontWeight: task.status === s ? 600 : 400,
                  }}
                >
                  {task.status === s ? '✓ ' : '  '}{t(statusKeys[s])}
                </button>
              ))}
              <div style={{ borderTop: '1px solid #eee', margin: '4px 0' }} />
              <button
                onClick={(e) => { e.stopPropagation(); setMenuOpen(false); onDelete(task.id); }}
                style={{
                  display: 'block',
                  width: '100%',
                  textAlign: 'left',
                  padding: '6px 12px',
                  border: 'none',
                  background: 'transparent',
                  cursor: 'pointer',
                  fontSize: '0.85em',
                  color: '#c62828',
                }}
              >
                {t('taskDelete')}
              </button>
            </div>
          )}
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          {task.project_id && projectsMap?.[task.project_id] && (
            <span
              title={projectsMap[task.project_id].name}
              style={{
                width: '8px', height: '8px', borderRadius: '50%',
                background: projectsMap[task.project_id].color,
                display: 'inline-block', flexShrink: 0,
              }}
            />
          )}
          <span
            style={{
              fontSize: '0.75em',
              padding: '2px 8px',
              borderRadius: '10px',
              background: sc.bg,
              color: sc.color,
              fontWeight: 500,
              whiteSpace: 'nowrap',
            }}
          >
            {t(statusKeys[task.status])}
          </span>
        </div>
      </div>

      {/* Title */}
      <h4 style={{ margin: 0, fontSize: '0.95em', fontWeight: 600, color: '#333', wordBreak: 'break-word' }}>
        {task.title}
      </h4>

      {/* Description */}
      {task.description && (
        <p style={{ margin: 0, fontSize: '0.85em', color: '#666', lineHeight: 1.4 }}>
          {task.description.length > 100
            ? task.description.slice(0, 100) + '...'
            : task.description}
        </p>
      )}
    </div>
  );
}
