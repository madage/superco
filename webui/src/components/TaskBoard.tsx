import { useEffect, useState, useCallback } from 'react';
import { useLang, type TranslationKey } from '../i18n/context';
import { tasks as tasksApi, projects as projectsApi } from '../api/client';
import { useResourceSync } from '../hooks/useResourceSync';
import { TaskCard } from './TaskCard';
import { TaskForm } from './TaskForm';
import type { Task, TaskStatus } from '../types';

const columns: TaskStatus[] = ['todo', 'in_progress', 'blocked', 'review', 'done'];

const columnLabels: Record<TaskStatus, TranslationKey> = {
  todo: 'taskStatusTodo',
  in_progress: 'taskStatusInProgress',
  blocked: 'taskStatusBlocked',
  review: 'taskStatusReview',
  done: 'taskStatusDone',
};

const columnColors: Record<TaskStatus, string> = {
  todo: '#e0e0e0',
  in_progress: '#bbdefb',
  blocked: '#d1c4e9',
  review: '#ffe0b2',
  done: '#c8e6c9',
};

export function TaskBoard() {
  const { t, lang } = useLang();
  const [taskList, setTaskList] = useState<Task[]>([]);
  const [loading, setLoading] = useState(true);
  const [view, setView] = useState<'kanban' | 'list'>('kanban');
  const [showForm, setShowForm] = useState(false);
  const [editingTask, setEditingTask] = useState<Task | null>(null);
  const [filterProjectId, setFilterProjectId] = useState<string>("");
  const [projects, setProjects] = useState<{ id: string; name: string; color: string }[]>([]);

  // Delete verification state
  const [deleteVerify, setDeleteVerify] = useState<{
    taskId: string;
    a: number;
    b: number;
    op: '+' | '-';
    answer: number;
  } | null>(null);
  const [verifyInput, setVerifyInput] = useState('');
  const [verifyError, setVerifyError] = useState(false);

  const fetchTasks = useCallback(async (projectFilter?: string) => {
    try {
      const res = await tasksApi.list(projectFilter || undefined);
      setTaskList(res.tasks);
    } catch {
      // silently fail
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTasks();
  }, [fetchTasks]);

  useEffect(() => {
    fetchTasks(filterProjectId || undefined);
  }, [filterProjectId, fetchTasks]);

  useEffect(() => {
    projectsApi.list().then((res) => setProjects(res.projects)).catch(() => {});
  }, []);

  useResourceSync('tasks', fetchTasks);

  const grouped = taskList.reduce(
    (acc, task) => {
      if (!acc[task.status]) acc[task.status] = [];
      acc[task.status].push(task);
      return acc;
    },
    {} as Record<string, Task[]>,
  );

  const handleCreate = useCallback(async (data: { title: string; description: string; project_id?: string | null }) => {
    try {
      await tasksApi.create({ title: data.title, description: data.description || undefined, project_id: data.project_id || undefined });
      setShowForm(false);
      fetchTasks();
    } catch {
      alert('Failed to create task');
    }
  }, [fetchTasks]);

  useEffect(() => {
    fetchTasks(filterProjectId || undefined);
  }, [filterProjectId, fetchTasks]);

  const handleUpdate = useCallback(async (data: { title: string; description: string; status?: TaskStatus; project_id?: string | null }) => {
    if (!editingTask) return;
    try {
      const updateData: { title?: string; description?: string; status?: TaskStatus; project_id?: string } = {};
      if (data.title !== editingTask.title) updateData.title = data.title;
      if (data.description !== editingTask.description) updateData.description = data.description;
      if (data.status && data.status !== editingTask.status) updateData.status = data.status;
      if (data.project_id !== undefined && data.project_id !== editingTask.project_id) updateData.project_id = data.project_id || undefined;
      if (Object.keys(updateData).length > 0) {
        await tasksApi.update(editingTask.id, updateData);
      }
      setEditingTask(null);
      fetchTasks();
    } catch {
      alert('Failed to update task');
    }
  }, [editingTask, fetchTasks]);

  const handleDelete = useCallback(async (id: string) => {
    // Generate simple math verification (addition/subtraction only)
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
      await tasksApi.delete(deleteVerify.taskId);
      setTaskList((prev) => prev.filter((t) => t.id !== deleteVerify.taskId));
      setDeleteVerify(null);
      setVerifyInput('');
      setVerifyError(false);
    } catch {
      alert('Failed to delete task');
    }
  }, [deleteVerify, verifyInput]);

  const handleStatusChange = useCallback(async (id: string, status: TaskStatus) => {
    try {
      const updated = await tasksApi.setStatus(id, status);
      setTaskList((prev) => prev.map((t) => (t.id === id ? updated : t)));
    } catch {
      alert('Failed to update status');
    }
  }, []);

  if (loading) {
    return (
      <div style={{ padding: '24px', color: '#999', textAlign: 'center' }}>{t('loading')}...</div>
    );
  }

  return (
    <div style={{ padding: '24px', maxWidth: taskList.length === 0 ? '600px' : '1400px', margin: '0 auto' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
        <h2 style={{ margin: 0 }}>{t('navTasks')}</h2>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
          {/* Project filter */}
          <select
            value={filterProjectId}
            onChange={(e) => setFilterProjectId(e.target.value)}
            style={{
              padding: "6px 10px",
              borderRadius: "6px",
              border: "1px solid #ddd",
              fontSize: "0.85em",
              background: "#fff",
              maxWidth: "140px",
            }}
          >
            <option value="">{t("noProject")}</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
          {/* View toggle */}
          <div style={{ display: 'flex', borderRadius: '6px', overflow: 'hidden', border: '1px solid #ddd' }}>
            <button
              onClick={() => setView('kanban')}
              style={{
                padding: '6px 14px',
                border: 'none',
                background: view === 'kanban' ? '#1976d2' : '#fff',
                color: view === 'kanban' ? '#fff' : '#666',
                cursor: 'pointer',
                fontSize: '0.85em',
              }}
            >
              {t('taskViewKanban')}
            </button>
            <button
              onClick={() => setView('list')}
              style={{
                padding: '6px 14px',
                border: 'none',
                background: view === 'list' ? '#1976d2' : '#fff',
                color: view === 'list' ? '#fff' : '#666',
                cursor: 'pointer',
                fontSize: '0.85em',
              }}
            >
              {t('taskViewList')}
            </button>
          </div>
          <button
            onClick={() => setShowForm(true)}
            style={{
              padding: '6px 16px',
              background: '#1976d2',
              color: '#fff',
              border: 'none',
              borderRadius: '6px',
              cursor: 'pointer',
              fontSize: '0.95em',
              fontWeight: 600,
            }}
          >
            + {t('taskCreate')}
          </button>
        </div>
      </div>

      {/* Empty state */}
      {taskList.length === 0 && (
        <div style={{ textAlign: 'center', color: '#999', marginTop: '48px', fontSize: '0.95em' }}>
          {t('taskEmpty')}
        </div>
      )}

      {/* Kanban view */}
      {view === 'kanban' && taskList.length > 0 && (
        <div style={{ display: 'flex', gap: '12px', overflow: 'auto', paddingBottom: '12px', minHeight: '400px' }}>
          {columns.map((col) => {
            const tasks = grouped[col] || [];
            return (
              <div key={col} style={{ flex: '0 0 260px', minWidth: '240px' }}>
                <div
                  style={{
                    padding: '10px 14px',
                    borderRadius: '12px 12px 0 0',
                    background: columnColors[col],
                    fontWeight: 600,
                    fontSize: '0.85em',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                  }}
                >
                  <span>{t(columnLabels[col])}</span>
                  <span style={{ background: 'rgba(0,0,0,0.1)', borderRadius: '10px', padding: '0 8px', fontSize: '0.85em' }}>
                    {tasks.length}
                  </span>
                </div>
                <div
                  style={{
                    background: '#fff',
                    borderRadius: '0 0 12px 12px',
                    padding: '8px',
                    minHeight: '120px',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '8px',
                    boxShadow: '0 2px 8px rgba(0,0,0,0.06)',
                  }}
                >
                  {tasks.map((task) => (
                    <TaskCard
                      key={task.id}
                      task={task}
                      onEdit={(t) => setEditingTask(t)}
                      onDelete={handleDelete}
                      onStatusChange={handleStatusChange}
                      projectsMap={Object.fromEntries(projects.map(p => [p.id, { name: p.name, color: p.color }]))}
                    />
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* List view */}
      {view === 'list' && taskList.length > 0 && (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.9em' }}>
            <thead>
              <tr style={{ background: '#f5f5f5', textAlign: 'left' }}>
                <th style={{ padding: '10px 12px', borderBottom: '2px solid #ddd' }}>{t('taskTitle')}</th>
                <th style={{ padding: '10px 12px', borderBottom: '2px solid #ddd' }}>{t('taskStatus')}</th>
                <th style={{ padding: '10px 12px', borderBottom: '2px solid #ddd' }}>{t('created')}</th>
                <th style={{ padding: '10px 12px', borderBottom: '2px solid #ddd' }}>{t('taskActions')}</th>
              </tr>
            </thead>
            <tbody>
              {taskList.map((task) => {
                const sc = {
                  todo: { bg: '#e0e0e0', color: '#616161' },
                  in_progress: { bg: '#bbdefb', color: '#1565c0' },
                  blocked: { bg: '#d1c4e9', color: '#4527a0' },
                  review: { bg: '#ffe0b2', color: '#e65100' },
                  done: { bg: '#c8e6c9', color: '#2e7d32' },
                }[task.status];
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
                          fontSize: '0.8em',
                          padding: '2px 8px',
                          borderRadius: '10px',
                          background: sc.bg,
                          color: sc.color,
                          fontWeight: 500,
                        }}
                      >
                        {t(columnLabels[task.status])}
                      </span>
                    </td>
                    <td style={{ padding: '10px 12px', color: '#999', fontSize: '0.85em', whiteSpace: 'nowrap' }}>
                      {new Date(task.created_at).toLocaleDateString()}
                    </td>
                    <td style={{ padding: '10px 12px' }}>
                      <div style={{ display: 'flex', gap: '6px' }}>
                        <button
                          onClick={() => setEditingTask(task)}
                          style={{
                            padding: '3px 10px',
                            fontSize: '0.75em',
                            borderRadius: '4px',
                            border: '1px solid #ddd',
                            background: '#fafafa',
                            cursor: 'pointer',
                            color: '#555',
                          }}
                        >
                          {t('profileEdit')}
                        </button>
                        <button
                          onClick={() => handleDelete(task.id)}
                          style={{
                            padding: '3px 10px',
                            fontSize: '0.75em',
                            borderRadius: '4px',
                            border: '1px solid #ddd',
                            background: '#fafafa',
                            cursor: 'pointer',
                            color: '#c62828',
                          }}
                        >
                          {t('taskDelete')}
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

      {/* Delete verification modal */}
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
            <h3 style={{ margin: '0 0 8px', color: '#333' }}>{t('taskConfirmDelete')}</h3>
            <p style={{ color: '#666', fontSize: '0.9em', marginBottom: '20px' }}>
              {lang === 'zh' ? '请回答以下验证问题：' : 'Answer the following to confirm:'}
            </p>
            <div style={{ fontSize: '1.4em', fontWeight: 700, color: '#333', marginBottom: '16px' }}>
              {deleteVerify.a} {deleteVerify.op} {deleteVerify.b} = ?
            </div>
            <input
              value={verifyInput}
              onChange={(e) => { setVerifyInput(e.target.value); setVerifyError(false); }}
              onKeyDown={(e) => { if (e.key === 'Enter') handleDeleteConfirm(); }}
              style={{
                width: '100%', padding: '10px', borderRadius: '6px', border: verifyError ? '1px solid #c62828' : '1px solid #ddd',
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
                {t('taskDelete')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Create form */}
      {showForm && (
        <TaskForm
          onClose={() => setShowForm(false)}
          onSave={handleCreate}
        />
      )}

      {/* Edit form */}
      {editingTask && (
        <TaskForm
          task={editingTask}
          onClose={() => setEditingTask(null)}
          onSave={handleUpdate}
        />
      )}
    </div>
  );
}
