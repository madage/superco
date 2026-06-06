import { useEffect, useState, useCallback } from 'react';
import { useLang } from '../i18n/context';
import { tasks as tasksApi, projects as projectsApi } from '../api/client';
import { useResourceSync } from '../hooks/useResourceSync';
import type { Task, TaskStatus, Project } from '../types';

const statusKeys: Record<TaskStatus, string> = {
  todo: 'taskStatusTodo',
  in_progress: 'taskStatusInProgress',
  blocked: 'taskStatusBlocked',
  review: 'taskStatusReview',
  done: 'taskStatusDone',
};

const statusColors: Record<TaskStatus, { bg: string; color: string }> = {
  todo: { bg: '#e0e0e0', color: '#616161' },
  in_progress: { bg: '#bbdefb', color: '#1565c0' },
  blocked: { bg: '#d1c4e9', color: '#4527a0' },
  review: { bg: '#ffe0b2', color: '#e65100' },
  done: { bg: '#c8e6c9', color: '#2e7d32' },
};

export function TrashView() {
  const { t, lang } = useLang();
  const [trashList, setTrashList] = useState<Task[]>([]);
  const [projectTrash, setProjectTrash] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);

  // Permanent delete verification
  const [deleteVerify, setDeleteVerify] = useState<{
    taskId: string;
    a: number;
    b: number;
    op: '+' | '-';
    answer: number;
  } | null>(null);
  const [verifyInput, setVerifyInput] = useState('');
  const [verifyError, setVerifyError] = useState(false);

  const fetchTrash = useCallback(async () => {
    try {
      const [taskRes, projectRes] = await Promise.all([
        tasksApi.listTrash(),
        projectsApi.listTrash(),
      ]);
      setTrashList(taskRes.tasks);
      setProjectTrash(projectRes.projects);
    } catch {
      // silently fail
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTrash();
  }, [fetchTrash]);

  useResourceSync('tasks', fetchTrash);
  useResourceSync('projects', fetchTrash);

  const handleRestore = useCallback(async (id: string) => {
    try {
      await tasksApi.restore(id);
      setTrashList((prev) => prev.filter((t) => t.id !== id));
    } catch {
      alert('Failed to restore task');
    }
  }, []);

  const handleProjectRestore = useCallback(async (id: string) => {
    try {
      await projectsApi.restore(id);
      setProjectTrash((prev) => prev.filter((p) => p.id !== id));
    } catch {
      alert('Failed to restore project');
    }
  }, []);

  const handlePermanentDelete = useCallback((id: string) => {
    const a = Math.floor(Math.random() * 20) + 1;
    const b = Math.floor(Math.random() * 20) + 1;
    const op = Math.random() > 0.5 ? '+' : '-';
    const answer = op === '+' ? a + b : Math.max(a, b) - Math.min(a, b);
    const [na, nb] = op === '+' ? [a, b] : [Math.max(a, b), Math.min(a, b)];
    setDeleteVerify({ taskId: id, a: na, b: nb, op, answer });
    setVerifyInput('');
    setVerifyError(false);
  }, []);

  const handleDeleteConfirm = useCallback(async () => {
    if (!deleteVerify) return;
    const userAnswer = parseInt(verifyInput, 10);
    if (isNaN(userAnswer) || userAnswer !== deleteVerify.answer) {
      setVerifyError(true);
      return;
    }
    try {
      await tasksApi.permanentDelete(deleteVerify.taskId);
      setTrashList((prev) => prev.filter((t) => t.id !== deleteVerify.taskId));
      setDeleteVerify(null);
      setVerifyInput('');
      setVerifyError(false);
    } catch {
      alert('Failed to delete task');
    }
  }, [deleteVerify, verifyInput]);

  if (loading) {
    return (
      <div style={{ padding: '24px', color: '#999', textAlign: 'center' }}>{t('loading')}...</div>
    );
  }

  return (
    <div style={{ padding: '24px', maxWidth: '800px', margin: '0 auto' }}>
      <h2 style={{ margin: '0 0 20px' }}>{t('navTrash')}</h2>

      {trashList.length === 0 && (
        <div style={{ textAlign: 'center', color: '#999', marginTop: '48px', fontSize: '0.95em' }}>
          {t('taskTrashEmpty')}
        </div>
      )}

      {trashList.length > 0 && (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.9em' }}>
            <thead>
              <tr style={{ background: '#f5f5f5', textAlign: 'left' }}>
                <th style={{ padding: '10px 12px', borderBottom: '2px solid #ddd' }}>{t('taskTitle')}</th>
                <th style={{ padding: '10px 12px', borderBottom: '2px solid #ddd' }}>{t('taskStatus')}</th>
                <th style={{ padding: '10px 12px', borderBottom: '2px solid #ddd' }}>{t('taskActions')}</th>
              </tr>
            </thead>
            <tbody>
              {trashList.map((task) => {
                const sc = statusColors[task.status];
                return (
                  <tr key={task.id} style={{ borderBottom: '1px solid #eee' }}>
                    <td style={{ padding: '10px 12px' }}>
                      <div style={{ fontWeight: 500 }}>{task.title}</div>
                      {task.description && (
                        <div style={{ fontSize: '0.85em', color: '#999', marginTop: '2px' }}>
                          {task.description.length > 60 ? task.description.slice(0, 60) + '...' : task.description}
                        </div>
                      )}
                    </td>
                    <td style={{ padding: '10px 12px' }}>
                      <span
                        style={{
                          fontSize: '0.8em', padding: '2px 8px', borderRadius: '10px',
                          background: sc.bg, color: sc.color, fontWeight: 500,
                        }}
                      >
                        {t(statusKeys[task.status] as any)}
                      </span>
                    </td>
                    <td style={{ padding: '10px 12px' }}>
                      <div style={{ display: 'flex', gap: '6px' }}>
                        <button
                          onClick={() => handleRestore(task.id)}
                          style={{
                            padding: '3px 10px', fontSize: '0.75em', borderRadius: '4px',
                            border: '1px solid #ddd', background: '#fafafa', cursor: 'pointer', color: '#2e7d32',
                          }}
                        >
                          {t('taskRestore')}
                        </button>
                        <button
                          onClick={() => handlePermanentDelete(task.id)}
                          style={{
                            padding: '3px 10px', fontSize: '0.75em', borderRadius: '4px',
                            border: '1px solid #ddd', background: '#fafafa', cursor: 'pointer', color: '#c62828',
                          }}
                        >
                          {t('taskPermanentDelete')}
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Permanent delete verification modal */}
      {deleteVerify && (
        <div
          onClick={() => { setDeleteVerify(null); setVerifyError(false); }}
          style={{
            position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)',
            display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 1000,
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
            <h3 style={{ margin: '0 0 8px', color: '#c62828' }}>{t('taskPermanentDelete')}</h3>
            <p style={{ color: '#666', fontSize: '0.9em', marginBottom: '20px' }}>
              {lang === 'zh' ? '此操作不可恢复，请回答以下验证问题：' : 'This cannot be undone. Answer the following:'}
            </p>
            <div style={{ fontSize: '1.4em', fontWeight: 700, color: '#333', marginBottom: '16px' }}>
              {deleteVerify.a} {deleteVerify.op} {deleteVerify.b} = ?
            </div>
            <input
              value={verifyInput}
              onChange={(e) => { setVerifyInput(e.target.value); setVerifyError(false); }}
              onKeyDown={(e) => { if (e.key === 'Enter') handleDeleteConfirm(); }}
              style={{
                width: '100%', padding: '10px', borderRadius: '6px',
                border: verifyError ? '1px solid #c62828' : '1px solid #ddd',
                fontSize: '1.1em', textAlign: 'center', boxSizing: 'border-box', outline: 'none',
                marginBottom: '8px',
              }}
              autoFocus
            />
            {verifyError && (
              <div style={{ color: '#c62828', fontSize: '0.85em', marginBottom: '8px' }}>
                {lang === 'zh' ? '答案错误，请重试' : 'Wrong answer, try again'}
              </div>
            )}
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'center', marginTop: '12px' }}>
              <button
                onClick={() => { setDeleteVerify(null); setVerifyError(false); }}
                style={{
                  padding: '10px 20px', borderRadius: '6px', border: '1px solid #ddd',
                  background: '#fff', cursor: 'pointer', color: '#666', fontSize: '0.95em',
                }}
              >
                {t('cancel')}
              </button>
              <button
                onClick={handleDeleteConfirm}
                style={{
                  padding: '10px 20px', borderRadius: '6px', border: 'none',
                  background: '#c62828', color: '#fff', cursor: 'pointer', fontSize: '0.95em',
                }}
              >
                {t('taskPermanentDelete')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
