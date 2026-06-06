import { useEffect, useState } from 'react';
import type { Project, Task, TaskStatus } from '../types';
import { tasks as tasksApi } from '../api/client';
import { useLang } from '../i18n/context';
import { TaskCard } from './TaskCard';
import { TaskForm } from './TaskForm';

interface ProjectDetailProps {
  project: Project;
  onClose: () => void;
  onDelete: (id: string) => void;
  onUpdate: (id: string, data: { name?: string; description?: string; color?: string }) => void;
}

const statusKeys: Record<TaskStatus, string> = {
  todo: 'taskStatusTodo',
  in_progress: 'taskStatusInProgress',
  blocked: 'taskStatusBlocked',
  review: 'taskStatusReview',
  done: 'taskStatusDone',
};

export function ProjectDetail({ project, onClose, onDelete, onUpdate }: ProjectDetailProps) {
  const { t, lang } = useLang();
  const [taskList, setTaskList] = useState<Task[]>([]);
  const [editingTask, setEditingTask] = useState<Task | null>(null);
  const [deleteVerify, setDeleteVerify] = useState<{
    taskId: string; a: number; b: number; op: '+' | '-'; answer: number;
  } | null>(null);
  const [verifyInput, setVerifyInput] = useState('');
  const [verifyError, setVerifyError] = useState(false);

  useEffect(() => {
    tasksApi.list(project.id).then((res) => setTaskList(res.tasks)).catch(() => {});
  }, [project.id]);

  const handleTaskUpdate = async (data: { title: string; description: string; status?: TaskStatus }) => {
    if (!editingTask) return;
    try {
      const updateData: { title?: string; description?: string; status?: TaskStatus } = {};
      if (data.title !== editingTask.title) updateData.title = data.title;
      if (data.description !== editingTask.description) updateData.description = data.description;
      if (data.status && data.status !== editingTask.status) updateData.status = data.status;
      if (Object.keys(updateData).length > 0) {
        await tasksApi.update(editingTask.id, updateData);
      }
      setEditingTask(null);
      const res = await tasksApi.list(project.id);
      setTaskList(res.tasks);
    } catch {
      alert('Failed to update task');
    }
  };

  const handleTaskDelete = (id: string) => {
    const a = Math.floor(Math.random() * 20) + 1;
    const b = Math.floor(Math.random() * 20) + 1;
    const op = Math.random() > 0.5 ? '+' : '-';
    const answer = op === '+' ? a + b : Math.max(a, b) - Math.min(a, b);
    const [na, nb] = op === '+' ? [a, b] : [Math.max(a, b), Math.min(a, b)];
    setDeleteVerify({ taskId: id, a: na, b: nb, op, answer });
    setVerifyInput('');
    setVerifyError(false);
  };

  const handleDeleteConfirm = async () => {
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
  };

  const handleTaskStatusChange = async (id: string, status: TaskStatus) => {
    try {
      const updated = await tasksApi.setStatus(id, status);
      setTaskList((prev) => prev.map((t) => (t.id === id ? updated : t)));
    } catch {
      alert('Failed to update status');
    }
  };

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)',
        display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 1000,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: '#fff', borderRadius: '16px', padding: '32px',
          width: '640px', maxWidth: '90vw', maxHeight: '85vh', overflow: 'auto',
          boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
        }}
      >
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '24px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{
              width: '48px', height: '48px', borderRadius: '12px',
              background: project.color + '20',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: '1.5em',
            }}>
              📁
            </div>
            <div>
              <h2 style={{ margin: 0, color: '#1a1a2e' }}>{project.name}</h2>
              {project.description && (
                <p style={{ margin: '4px 0 0', color: '#888', fontSize: '0.9em' }}>{project.description}</p>
              )}
            </div>
          </div>
          <button onClick={onClose} style={{
            width: '36px', height: '36px', borderRadius: '50%', border: 'none',
            background: '#f5f5f5', cursor: 'pointer', fontSize: '1.2em',
            display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#666',
          }}>✕</button>
        </div>

        {/* Project info bar */}
        <div style={{
          background: '#f9f9f9', borderRadius: '8px', padding: '12px 16px',
          display: 'flex', gap: '24px', fontSize: '0.85em', marginBottom: '20px',
        }}>
          <div><span style={{ color: '#999' }}>{t('projectColor')}</span> <span style={{ color: '#333', fontWeight: 500 }}>●</span></div>
          <div><span style={{ color: '#999' }}>{lang === 'zh' ? '任务' : 'Tasks'}</span> <span style={{ color: '#333', fontWeight: 500 }}>{taskList.length}</span></div>
        </div>

        {/* Section title */}
        <h3 style={{ margin: '0 0 12px', color: '#333', fontSize: '1em' }}>{lang === 'zh' ? '任务' : 'Tasks'}</h3>

        {/* Task list */}
        {taskList.length === 0 ? (
          <div style={{ textAlign: 'center', color: '#999', padding: '24px', fontSize: '0.9em' }}>
            {t('taskEmpty')}
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
            {taskList.map((task) => (
              <TaskCard
                key={task.id}
                task={task}
                onEdit={(t) => setEditingTask(t)}
                onDelete={handleTaskDelete}
                onStatusChange={handleTaskStatusChange}
                projectsMap={{ [project.id]: { name: project.name, color: project.color } }}
              />
            ))}
          </div>
        )}

        {/* Actions */}
        <div style={{ display: 'flex', gap: '8px', borderTop: '1px solid #eee', paddingTop: '16px', marginTop: '16px' }}>
          <button onClick={() => onDelete(project.id)} style={{
            padding: '8px 20px', background: '#fbe9e7', color: '#d32f2f',
            border: 'none', borderRadius: '6px', cursor: 'pointer', fontSize: '0.9em',
          }}>
            {t('taskDelete')}
          </button>
        </div>
      </div>

      {/* Task edit form */}
      {editingTask && (
        <TaskForm
          task={editingTask}
          onClose={() => setEditingTask(null)}
          onSave={handleTaskUpdate}
        />
      )}

      {/* Delete verification */}
      {deleteVerify && (
        <div
          onClick={() => { setDeleteVerify(null); setVerifyError(false); }}
          style={{
            position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)',
            display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 1100,
          }}
        >
          <div onClick={(e) => e.stopPropagation()} style={{
            background: '#fff', borderRadius: '12px', padding: '28px',
            width: '360px', maxWidth: '90vw',
            boxShadow: '0 8px 32px rgba(0,0,0,0.2)', textAlign: 'center',
          }}>
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
              <button onClick={() => { setDeleteVerify(null); setVerifyError(false); }} style={{
                padding: '10px 20px', borderRadius: '6px', border: '1px solid #ddd',
                background: '#fff', cursor: 'pointer', color: '#666', fontSize: '0.95em',
              }}>{t('cancel')}</button>
              <button onClick={handleDeleteConfirm} style={{
                padding: '10px 20px', borderRadius: '6px', border: 'none',
                background: '#c62828', color: '#fff', cursor: 'pointer', fontSize: '0.95em',
              }}>{t('taskDelete')}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
