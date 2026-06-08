import { useState, useRef, useEffect } from 'react';
import type { Task, TaskStatus, Priority, TaskAssignee } from '../types';
import { useLang, type TranslationKey } from '../i18n/context';

const statusColors: Record<TaskStatus, { bg: string; color: string }> = {
  todo: { bg: '#e0e0e0', color: '#616161' },
  in_progress: { bg: '#bbdefb', color: '#1565c0' },
  blocked: { bg: '#d1c4e9', color: '#4527a0' },
  review: { bg: '#ffe0b2', color: '#e65100' },
  done: { bg: '#c8e6c9', color: '#2e7d32' },
};

const priorityColors: Record<Priority, { bg: string; color: string }> = {
  urgent: { bg: '#ffcdd2', color: '#c62828' },
  high: { bg: '#ffe0b2', color: '#e65100' },
  medium: { bg: '#bbdefb', color: '#1565c0' },
  low: { bg: '#e0e0e0', color: '#757575' },
};

const statusKeys: Record<TaskStatus, TranslationKey> = {
  todo: 'taskStatusTodo',
  in_progress: 'taskStatusInProgress',
  blocked: 'taskStatusBlocked',
  review: 'taskStatusReview',
  done: 'taskStatusDone',
};

const priorityKeys: Record<Priority, TranslationKey> = {
  urgent: 'priorityUrgent',
  high: 'priorityHigh',
  medium: 'priorityMedium',
  low: 'priorityLow',
};

const allStatuses: TaskStatus[] = ['todo', 'in_progress', 'blocked', 'review', 'done'];

interface TaskCardProps {
  task: Task;
  onEdit: (task: Task) => void;
  onDelete: (id: string) => void;
  onStatusChange: (id: string, status: TaskStatus) => void;
  projectsMap?: Record<string, { name: string; color: string }>;
  subtaskCount?: number;
  assigneeName?: string;
  creatorName?: string;
  assigneeNamesMap?: Record<string, string>;
  processingTasks?: Set<string>;
}

export function TaskCard({ task, onEdit, onDelete, onStatusChange, projectsMap, subtaskCount, assigneeName, creatorName, assigneeNamesMap, processingTasks }: TaskCardProps) {
  const isProcessing = processingTasks?.has(task.id);
  const { t } = useLang();
  const pc = priorityColors[task.priority] || priorityColors.medium;
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  const isOverdue = task.due_at && new Date(task.due_at) < new Date() && task.status !== 'done';

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
    <>
      <style>{`
        @keyframes task-card-spin {
          to { transform: rotate(360deg); }
        }
        @keyframes task-card-pulse {
          0%, 100% { border-color: #e0e0e0; }
          50% { border-color: #1976d2; }
        }
      `}</style>
    <div
      style={{
        background: '#fff',
        borderRadius: '12px',
        padding: '14px 16px',
        border: isProcessing ? '1px solid #1976d2' : '1px solid #e0e0e0',
        boxShadow: isProcessing ? '0 0 8px rgba(25,118,210,0.3)' : '0 1px 3px rgba(0,0,0,0.06)',
        animation: isProcessing ? 'task-card-pulse 2s ease-in-out infinite' : undefined,
        transition: 'transform 0.2s, boxShadow 0.2s',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        cursor: 'pointer',
        position: 'relative',
        zIndex: menuOpen ? 1 : undefined,
      }}
      onClick={() => onEdit(task)}
      onMouseEnter={(e) => {
        e.currentTarget.style.transform = 'translateY(-4px)';
        e.currentTarget.style.borderColor = '#bbb';
        e.currentTarget.style.boxShadow = '0 12px 24px rgba(0,0,0,0.15), 0 4px 8px rgba(0,0,0,0.1)';
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.transform = '';
        e.currentTarget.style.borderColor = '#e0e0e0';
        e.currentTarget.style.boxShadow = '';
      }}
    >
      {/* Top row: menu + priority + project dot + status badge */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div ref={menuRef} style={{ position: 'relative' }}>
          <button
            onClick={(e) => { e.stopPropagation(); setMenuOpen(!menuOpen); }}
            style={{
              background: 'none', border: 'none', cursor: 'pointer',
              padding: '2px 6px', borderRadius: '4px', color: '#999',
              fontSize: '1.1em', lineHeight: 1, fontWeight: 700, letterSpacing: '1px',
            }}
          >
            ···
          </button>
          {menuOpen && (
            <div
              style={{
                position: 'absolute', top: '100%', left: 0, zIndex: 3000,
                background: '#fff', borderRadius: '8px',
                boxShadow: '0 4px 16px rgba(0,0,0,0.15)', minWidth: '140px',
                padding: '4px 0', border: '1px solid #eee',
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
                    display: 'block', width: '100%', textAlign: 'left',
                    padding: '6px 12px', border: 'none',
                    background: task.status === s ? '#f0f0f0' : 'transparent',
                    cursor: 'pointer', fontSize: '0.85em',
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
                  display: 'block', width: '100%', textAlign: 'left',
                  padding: '6px 12px', border: 'none', background: 'transparent',
                  cursor: 'pointer', fontSize: '0.85em', color: '#c62828',
                }}
              >
                {t('taskDelete')}
              </button>
            </div>
          )}
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          {/* Priority badge */}
          <span
            style={{
              fontSize: '0.7em', padding: '1px 6px', borderRadius: '8px',
              background: pc.bg, color: pc.color, fontWeight: 600,
              whiteSpace: 'nowrap', textTransform: 'uppercase',
            }}
          >
            {t(priorityKeys[task.priority])}
          </span>
          {/* Project dot */}
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
          {/* Agent processing spinner */}
          {isProcessing && (
            <span
              title="Agent working..."
              style={{
                width: '14px', height: '14px', borderRadius: '50%',
                border: '2px solid #e0e0e0',
                borderTopColor: '#1976d2',
                animation: 'task-card-spin 0.8s linear infinite',
                display: 'inline-block', flexShrink: 0,
              }}
            />
          )}
        </div>
      </div>

      {/* Title */}
      <h4 style={{ margin: 0, fontSize: '0.95em', fontWeight: 600, color: '#333', wordBreak: 'break-word' }}>
        {task.title}
      </h4>

      {/* Description */}
      {task.description && (
        <p style={{ margin: 0, fontSize: '0.85em', color: '#666', lineHeight: 1.4 }}>
          {task.description.replace(/<[^>]*>/g, '').length > 100
            ? task.description.replace(/<[^>]*>/g, '').slice(0, 100) + '...'
            : task.description.replace(/<[^>]*>/g, '')}
        </p>
      )}

      {/* Bottom row: tags, assignee, due date, subtasks */}
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: '6px', marginTop: '4px' }}>
        {/* Tags */}
        {task.tags && task.tags.length > 0 && task.tags.slice(0, 3).map(tag => (
          <span
            key={tag}
            style={{
              fontSize: '0.7em', padding: '1px 6px', borderRadius: '6px',
              background: '#e3f2fd', color: '#1565c0', whiteSpace: 'nowrap',
            }}
          >
            {tag}
          </span>
        ))}
        {task.tags && task.tags.length > 3 && (
          <span style={{ fontSize: '0.7em', color: '#999' }}>+{task.tags.length - 3}</span>
        )}

        {/* Creator */}
        {creatorName && (
          <span style={{ fontSize: '0.75em', color: '#888' }}>
            ✏️ {creatorName}
          </span>
        )}

        {/* Assignee */}
        {assigneeName && (
          <span style={{ fontSize: '0.75em', color: '#555' }}>
            👤 {assigneeName}
          </span>
        )}

        {/* Delegated assignees */}
        {task.assignees && task.assignees.length > 0 && (
          <span style={{ fontSize: '0.75em', color: '#777' }}>
            👥 {task.assignees.map(a => assigneeNamesMap?.[a.assignee_id] || a.assignee_id.slice(0, 6)).join(', ')}
          </span>
        )}

        {/* Due date */}
        {task.due_at && (
          <span
            style={{
              fontSize: '0.75em', color: isOverdue ? '#c62828' : '#555',
              fontWeight: isOverdue ? 600 : 400,
            }}
          >
            {isOverdue ? '⚠ ' : '📅 '}{new Date(task.due_at).toLocaleDateString()}
          </span>
        )}

        {/* Subtask count */}
        {subtaskCount !== undefined && subtaskCount > 0 && (
          <span style={{ fontSize: '0.75em', color: '#888' }}>
            📋 {subtaskCount}
          </span>
        )}
      </div>
    </div>
    </>
  );
}
