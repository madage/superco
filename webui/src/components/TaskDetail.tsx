import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { useLang } from '../i18n/context';
import { tasks as tasksApi, projects as projectsApi, workspaceMembers as workspaceMembersApi, agentProfiles as agentProfilesApi, comments as commentsApi, agentQueue as agentQueueApi, workflows as workflowsApi } from '../api/client';
import { useWorkspace } from '../hooks/WorkspaceContext';
import type { Task, TaskStatus, Project, Priority, AssigneeType, WorkspaceMember, AgentProfile, Comment, Workflow, AgentQueueItem, DecompositionPlan, DecompositionPlanItem, ApprovePlanReq } from '../types';

interface TaskDetailProps {
  task: Task;
  onClose: () => void;
  onDelete: (id: string) => void;
  onRefresh: () => void;
}

const statusOptions: TaskStatus[] = ['todo', 'in_progress', 'blocked', 'review', 'done'];
const priorityOptions: Priority[] = ['urgent', 'high', 'medium', 'low'];

function highlightMentions(html: string, agentNames: Set<string> = new Set()): string {
  return html.replace(/@([\w一-鿿]{2,48})/g, (match, name) => {
    const color = agentNames.has(name) ? '#2e7d32' : '#1976d2';
    return `<span style="color:${color};font-weight:500;">@${name}</span>`;
  });
}

function formatCommentContent(content: string, isAgent: boolean, agentNames: Set<string>): string {
  const escaped = content.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  if (!isAgent) return highlightMentions(escaped.replace(/\n/g, '<br>'), agentNames);
  let html = escaped
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code style="background:#e8e8e8;padding:1px 5px;border-radius:3px;font-size:0.9em">$1</code>');
  const lines = html.split('\n');
  const out: string[] = [];
  let inUl = false, inOl = false;
  for (const line of lines) {
    const ulMatch = line.match(/^(\s*)[-*]\s+(.+)/);
    const olMatch = line.match(/^(\s*)(\d+)\.\s+(.+)/);
    if (ulMatch) {
      if (inOl) { out.push('</ol>'); inOl = false; }
      if (!inUl) { out.push('<ul style="margin:4px 0;padding-left:20px">'); inUl = true; }
      out.push(`<li style="margin:2px 0">${ulMatch[2]}</li>`);
    } else if (olMatch) {
      if (inUl) { out.push('</ul>'); inUl = false; }
      if (!inOl) { out.push('<ol style="margin:4px 0;padding-left:20px">'); inOl = true; }
      out.push(`<li style="margin:2px 0">${olMatch[3]}</li>`);
    } else {
      if (inUl) { out.push('</ul>'); inUl = false; }
      if (inOl) { out.push('</ol>'); inOl = false; }
      if (line.trim() === '') out.push('<br>');
      else out.push(`<div style="margin:2px 0">${line}</div>`);
    }
  }
  if (inUl) out.push('</ul>');
  if (inOl) out.push('</ol>');
  return highlightMentions(out.join('\n'), agentNames);
}

export function TaskDetail({ task, onClose, onDelete, onRefresh }: TaskDetailProps) {
  const { t, lang } = useLang();
  const { workspaceId } = useWorkspace();
  const [currentUser] = useState(() => {
    try {
      const u = JSON.parse(localStorage.getItem('user') || '{}');
      return (u as { id?: string }).id || '';
    } catch { return ''; }
  });
  const [currentTask, setCurrentTask] = useState<Task>(task);
  const [title, setTitle] = useState(task.title);
  const titleRef = useRef<HTMLInputElement>(null);

  // Reference data
  const [projects, setProjects] = useState<Project[]>([]);
  const [members, setMembers] = useState<WorkspaceMember[]>([]);
  const [agentProfiles, setAgentProfiles] = useState<AgentProfile[]>([]);
  const [allTasks, setAllTasks] = useState<Task[]>([]);
  const [subtasks, setSubtasks] = useState<Task[]>([]);
  const [nameMap, setNameMap] = useState<Record<string, string>>({});
  const [workflows, setWorkflows] = useState<Workflow[]>([]);

  // Comments
  const [comments, setComments] = useState<Comment[]>([]);
  const [commentSortAsc, setCommentSortAsc] = useState(true);
  const [commentInput, setCommentInput] = useState('');
  const commentEditorRef = useRef<HTMLDivElement>(null);
  const [posting, setPosting] = useState(false);
  const [replyToId, setReplyToId] = useState<string | null>(null);
  const replyEditorRef = useRef<HTMLDivElement>(null);

  // @mention autocomplete
  const [mentionOpen, setMentionOpen] = useState(false);
  const [mentionSearch, setMentionSearch] = useState('');
  const [mentionIndex, setMentionIndex] = useState(0);
  const [mentionItems, setMentionItems] = useState<{ id: string; name: string; type: 'user' | 'agent' }[]>([]);
  const [mentionEditor, setMentionEditor] = useState<'main' | 'reply'>('main');

  // Assignee picker state
  const [showAddAssignee, setShowAddAssignee] = useState(false);
  const [newAssigneeType, setNewAssigneeType] = useState<AssigneeType>('user');
  const [newAssigneeId, setNewAssigneeId] = useState('');

  // Tag input
  const [tagInput, setTagInput] = useState('');

  // Saving indicator
  const [saving, setSaving] = useState(false);

  // Delete verification
  const [showDeleteVerify, setShowDeleteVerify] = useState(false);
  const [confirmDeleteComment, setConfirmDeleteComment] = useState<string | null>(null);

  const [isProcessing, setIsProcessing] = useState(false);
  const [taskQueueItems, setTaskQueueItems] = useState<AgentQueueItem[]>([]);
  const [reviewComment, setReviewComment] = useState('');
  const [reviewing, setReviewing] = useState(false);

  // Decomposition plan state
  const [decompPlan, setDecompPlan] = useState<DecompositionPlan | null>(null);
  const [decompItems, setDecompItems] = useState<DecompositionPlanItem[]>([]);
  const [decompLoading, setDecompLoading] = useState(false);
  const [selectedItems, setSelectedItems] = useState<Set<string>>(new Set());
  const [approving, setApproving] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [showRejectInput, setShowRejectInput] = useState(false);
  const [rejectComment, setRejectComment] = useState('');

  const isOverdue = currentTask.due_at && new Date(currentTask.due_at) < new Date() && !['done', 'completed', 'stuck'].includes(currentTask.status);

  useEffect(() => {
    const fetchQueue = () => {
      agentQueueApi.list().then(res => {
        const items = res.queue.filter((q: AgentQueueItem) => q.task_id === currentTask.id);
        setTaskQueueItems(items);
        setIsProcessing(items.some(q => q.status === 'processing'));
      }).catch(() => {});
    };
    fetchQueue();
    const interval = setInterval(fetchQueue, 10000);
    return () => clearInterval(interval);
  }, [currentTask.id]);

  useEffect(() => {
    const load = async () => {
      const [projRes, taskRes] = await Promise.all([
        projectsApi.list().catch(() => ({ projects: [] as Project[] })),
        tasksApi.list({ parentId: 'none' }).catch(() => ({ tasks: [] as Task[] })),
      ]);
      setProjects(projRes.projects);
      setAllTasks(taskRes.tasks.filter(t => t.id !== task.id));

      tasksApi.listSubtasks(task.id).then(res => setSubtasks(res.tasks)).catch(() => {});
      commentsApi.list(task.id).then(res => setComments(res.comments)).catch(() => {});

      // Load decomposition plan
      setDecompLoading(true);
      tasksApi.getDecompositionPlan(task.id).then(res => {
        setDecompPlan(res.plan);
        setDecompItems(res.items);
        setSelectedItems(new Set());
      }).catch(() => {}).finally(() => setDecompLoading(false));

      const names: Record<string, string> = {};
      if (workspaceId) {
        const membersRes = await workspaceMembersApi.list(workspaceId).catch(() => ({ members: [] as WorkspaceMember[] }));
        setMembers(membersRes.members);
        membersRes.members.forEach(m => { names[m.user_id] = m.username; });
      }
      const profilesRes = await agentProfilesApi.list().catch(() => ({ profiles: [] as AgentProfile[] }));
      setAgentProfiles(profilesRes.profiles);
      profilesRes.profiles.forEach(p => { names[p.id] = p.name; });
      const wfRes = await workflowsApi.list().catch(() => ({ workflows: [] as Workflow[] }));
      setWorkflows(wfRes.workflows);
      setNameMap(names);
    };
    load();
  }, [workspaceId, task.id]);

  // Build mention candidates when members/agents change
  const mentionCandidates = useRef<{ id: string; name: string; type: 'user' | 'agent' }[]>([]);
  mentionCandidates.current = [
    ...members.map(m => ({ id: m.user_id, name: m.username, type: 'user' as const })),
    ...agentProfiles.filter(a => a.enabled).map(a => ({ id: a.id, name: a.name, type: 'agent' as const })),
  ];

  // Agent name set for mention highlighting
  const agentNames = useMemo(() => new Set(agentProfiles.filter(a => a.enabled).map(a => a.name)), [agentProfiles]);

  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (confirmDeleteComment) { setConfirmDeleteComment(null); return; }
        if (replyToId) { setReplyToId(null); return; }
        if (!showAddAssignee && !showDeleteVerify) onClose();
      }
    };
    window.addEventListener('keydown', handleEsc);
    return () => window.removeEventListener('keydown', handleEsc);
  }, [onClose, showAddAssignee, showDeleteVerify, replyToId]);

  const saveField = useCallback(async (update: Record<string, unknown>) => {
    setSaving(true);
    try {
      const updated = await tasksApi.update(currentTask.id, update);
      // The API might not return creator_name in the response, preserve it
      setCurrentTask(prev => ({ ...updated, creator_name: prev.creator_name }));
      onRefresh();
    } catch (err) {
      console.error('Failed to update task', err);
      alert('Failed to update task');
    } finally {
      setSaving(false);
    }
  }, [currentTask.id, onRefresh]);

  const handleTitleSave = useCallback(() => {
    const trimmed = title.trim();
    if (trimmed && trimmed !== currentTask.title) {
      saveField({ title: trimmed });
    } else {
      setTitle(currentTask.title);
    }
  }, [title, currentTask.title, saveField]);

  const handleTitleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      titleRef.current?.blur();
    }
  };

  const handleStatusChange = (status: TaskStatus) => {
    setCurrentTask(prev => ({ ...prev, status }));
    saveField({ status });
  };

  const handlePriorityChange = (priority: Priority) => {
    setCurrentTask(prev => ({ ...prev, priority }));
    saveField({ priority });
  };

  const handleAssigneeChange = (assigneeId: string | null, assigneeType: AssigneeType | null) => {
    setCurrentTask(prev => ({ ...prev, assignee_id: assigneeId ?? undefined, assignee_type: assigneeType ?? undefined }));
    saveField({
      assignee_id: assigneeId ?? null,
      assignee_type: assigneeType ?? null,
    });
  };

  const handleProjectChange = (projectId: string | null) => {
    setCurrentTask(prev => ({ ...prev, project_id: projectId ?? undefined }));
    saveField({ project_id: projectId ?? null });
  };

  const handleWorkflowChange = async (workflowId: string | null) => {
    if (!workflowId) return;
    try {
      await workflowsApi.attach({ task_id: currentTask.id, workflow_id: workflowId });
      const refreshed = await tasksApi.get(currentTask.id);
      setCurrentTask(refreshed);
      onRefresh();
    } catch (err) {
      console.error('Failed to attach task to workflow', err);
      alert('Failed to attach task to workflow');
    }
  };

  const [triggeringAgent, setTriggeringAgent] = useState(false);
  const handleTriggerAgent = async () => {
    setTriggeringAgent(true);
    try {
      await agentQueueApi.autoAssign(currentTask.id);
      const refreshed = await tasksApi.get(currentTask.id);
      setCurrentTask(refreshed);
      onRefresh();
    } catch (err) {
      console.error('Failed to trigger agent', err);
      alert('Failed to trigger agent');
    } finally {
      setTriggeringAgent(false);
    }
  };

  const handleParentChange = (parentId: string | null) => {
    setCurrentTask(prev => ({ ...prev, parent_id: parentId ?? undefined }));
    saveField({ parent_id: parentId ?? null });
  };

  const handleDueAtChange = (dueAt: string) => {
    const val = dueAt ? dueAt + 'T00:00:00Z' : null;
    setCurrentTask(prev => ({ ...prev, due_at: val ?? undefined }));
    saveField({ due_at: val });
  };

  const handleReview = async (action: 'approved' | 'rejected') => {
    setReviewing(true);
    try {
      await tasksApi.review(currentTask.id, { action, comment: reviewComment || undefined });
      const refreshed = await tasksApi.get(currentTask.id);
      setCurrentTask(refreshed);
      setReviewComment('');
      onRefresh();
    } catch (err) {
      console.error('Failed to review task', err);
      alert('Failed to review task');
    } finally {
      setReviewing(false);
    }
  };

  const handleApprovePlan = async (itemIds?: string[]) => {
    setApproving(true);
    try {
      const res = await tasksApi.approveDecompositionPlan(currentTask.id, { item_ids: itemIds });
      const refreshed = await tasksApi.get(currentTask.id);
      setCurrentTask(refreshed);
      setDecompPlan(null);
      setDecompItems([]);
      setSelectedItems(new Set());
      onRefresh();
    } catch (err) {
      console.error('Failed to approve plan', err);
      alert('Failed to approve plan');
    } finally {
      setApproving(false);
    }
  };

  const handleRejectPlan = async () => {
    setRejecting(true);
    try {
      await tasksApi.rejectDecompositionPlan(currentTask.id, { comment: rejectComment || undefined });
      setDecompPlan(null);
      setDecompItems([]);
      setShowRejectInput(false);
      setRejectComment('');
      onRefresh();
    } catch (err) {
      console.error('Failed to reject plan', err);
      alert('Failed to reject plan');
    } finally {
      setRejecting(false);
    }
  };

  const toggleItemSelection = (itemId: string) => {
    setSelectedItems(prev => {
      const next = new Set(prev);
      if (next.has(itemId)) {
        next.delete(itemId);
      } else {
        next.add(itemId);
      }
      return next;
    });
  };

  const toggleSelectAll = () => {
    const unapproved = decompItems.filter(i => i.is_approved === null);
    if (selectedItems.size === unapproved.length) {
      setSelectedItems(new Set());
    } else {
      setSelectedItems(new Set(unapproved.map(i => i.id)));
    }
  };

  const handleAddAssignee = async () => {
    if (!newAssigneeId) return;
    try {
      await tasksApi.addAssignee(currentTask.id, {
        assignee_id: newAssigneeId,
        assignee_type: newAssigneeType,
      });
      const refreshed = await tasksApi.get(currentTask.id);
      setCurrentTask(refreshed);
      setShowAddAssignee(false);
      setNewAssigneeId('');
      onRefresh();
    } catch (err) {
      console.error('Failed to add assignee', err);
    }
  };

  const handleRemoveAssignee = async (assigneeId: string) => {
    try {
      await tasksApi.removeAssignee(currentTask.id, assigneeId);
      const refreshed = await tasksApi.get(currentTask.id);
      setCurrentTask(refreshed);
      onRefresh();
    } catch (err) {
      console.error('Failed to remove assignee', err);
    }
  };

  const handleAddTag = () => {
    const tag = tagInput.trim();
    if (!tag || (currentTask.tags || []).includes(tag)) {
      setTagInput('');
      return;
    }
    const newTags = [...(currentTask.tags || []), tag];
    setCurrentTask(prev => ({ ...prev, tags: newTags }));
    saveField({ tags: newTags });
    setTagInput('');
  };

  const handleRemoveTag = (tag: string) => {
    const newTags = (currentTask.tags || []).filter(t => t !== tag);
    setCurrentTask(prev => ({ ...prev, tags: newTags }));
    saveField({ tags: newTags });
  };

  const handlePostComment = async (parentId?: string) => {
    const ref = parentId ? replyEditorRef.current : commentEditorRef.current;
    let text = (ref?.innerHTML || '').trim();
    // Strip mention color spans to plain @text before saving
    text = text.replace(/<span[^>]*>(@[\w一-鿿]+)<\/span>/g, '$1');
    const plain = text.replace(/<[^>]*>/g, '').trim();
    if (!plain || plain === '<br>' || posting) return;
    setPosting(true);
    try {
      const content = text === '<br>' ? '' : text;
      const newComment = await commentsApi.create(currentTask.id, { content, parent_id: parentId });
      setComments(prev => [...prev, newComment]);
      if (ref) ref.innerHTML = '';
      if (!parentId) setCommentInput('');
      setReplyToId(null);
    } catch {
      alert('Failed to post comment');
    } finally {
      setPosting(false);
    }
  };

  const startReply = (commentId: string) => {
    setReplyToId(replyToId === commentId ? null : commentId);
  };

  const renderComment = (c: Comment, all: Comment[], depth: number) => {
    const isAgent = !!c.agent_profile_id;
    const authorName = c.agent_name || c.username || t('taskDetailUnknown');
    const avatar = isAgent ? (c.agent_avatar || '🤖') : '👤';
    const isOwn = c.user_id === currentUser;
    const replies = all.filter(r => r.parent_id === c.id);
    const showReplyEditor = replyToId === c.id;
    return (
      <div key={c.id} style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
        <div style={{
          display: 'flex', gap: '10px', padding: '10px 12px',
          background: isAgent ? '#f0f7ff' : '#fafafa',
          borderRadius: '8px', border: '1px solid #eee',
          marginLeft: depth > 0 ? '32px' : 0,
        }}>
          <div style={{
            width: '32px', height: '32px', borderRadius: '50%',
            background: isAgent ? '#e3f2fd' : '#e0e0e0',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            fontSize: '1em', flexShrink: 0,
          }}>
            {avatar}
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '4px' }}>
              <span style={{ fontWeight: 600, fontSize: '0.85em', color: '#333' }}>
                {authorName}
              </span>
              {isAgent && (
                <span style={{
                  fontSize: '0.65em', padding: '1px 5px', borderRadius: '4px',
                  background: '#e3f2fd', color: '#1565c0', fontWeight: 500,
                }}>{t('taskDetailAgentBadge')}</span>
              )}
              <span style={{ fontSize: '0.75em', color: '#aaa' }}>
                {new Date(c.created_at).toLocaleString()}
              </span>
              <div style={{ flex: 1 }} />
              {isOwn && !isAgent && (
                <button onClick={(e) => { e.stopPropagation(); handleDeleteComment(c.id); }}
                  title={confirmDeleteComment === c.id ? t('taskDetailDeleteCommentHint') : t('taskDetailDeleteCommentTitle')}
                  style={{
                    background: confirmDeleteComment === c.id ? '#ffcdd2' : 'none',
                    border: 'none', cursor: 'pointer',
                    color: confirmDeleteComment === c.id ? '#c62828' : '#bbb',
                    padding: '0 6px', fontSize: confirmDeleteComment === c.id ? '0.75em' : '0.85em',
                    lineHeight: 1, borderRadius: '4px',
                    opacity: confirmDeleteComment === c.id ? 1 : 0.4,
                    transition: 'opacity 0.15s, background 0.15s',
                  }}
                  onMouseEnter={e => { if (confirmDeleteComment !== c.id) { e.currentTarget.style.opacity = '1'; e.currentTarget.style.color = '#c62828'; } }}
                  onMouseLeave={e => { if (confirmDeleteComment !== c.id) { e.currentTarget.style.opacity = '0.4'; e.currentTarget.style.color = '#bbb'; } }}
                >{confirmDeleteComment === c.id ? t('taskDetailDeleteCommentHint') : '✕'}</button>
              )}
            </div>
            <div style={{
              fontSize: '0.9em', lineHeight: 1.5, color: '#333',
              wordBreak: 'break-word',
            }} dangerouslySetInnerHTML={{ __html: formatCommentContent(c.content, isAgent, agentNames) }} />
            <div style={{ marginTop: '6px' }}>
              <button onClick={() => startReply(c.id)}
                style={{
                  background: 'none', border: 'none', cursor: 'pointer',
                  color: '#1976d2', padding: '0 4px', fontSize: '0.78em',
                  lineHeight: 1, opacity: 0.6, transition: 'opacity 0.15s',
                }}
                onMouseEnter={e => { e.currentTarget.style.opacity = '1'; }}
                onMouseLeave={e => { e.currentTarget.style.opacity = '0.6'; }}
              >{t('commentReply')}</button>
            </div>
            {showReplyEditor && (
              <div style={{ marginTop: '10px', paddingLeft: '8px', borderLeft: '2px solid #1976d2', position: 'relative' }}>
                {mentionOpen && mentionEditor === 'reply' && (
                  <div style={{
                    position: 'absolute', bottom: '100%', left: 0, zIndex: 999,
                    background: '#fff', border: '1px solid #ddd', borderRadius: '8px',
                    boxShadow: '0 4px 16px rgba(0,0,0,0.15)', maxHeight: '200px', overflow: 'auto',
                    minWidth: '200px',
                  }}>
                    {mentionItems.length === 0 ? (
                      <div style={{ padding: '8px 12px', color: '#999', fontSize: '0.85em' }}>No results</div>
                    ) : mentionItems.map((item, i) => (
                      <div key={item.id}
                        onClick={() => insertMention(item)}
                        onMouseEnter={() => setMentionIndex(i)}
                        style={{
                          padding: '8px 12px', cursor: 'pointer', fontSize: '0.85em',
                          background: i === mentionIndex ? '#e3f2fd' : 'transparent',
                          display: 'flex', alignItems: 'center', gap: '6px',
                        }}
                      >
                        <span>{item.type === 'user' ? '👤' : '🤖'}</span>
                        <span style={{ color: item.type === 'agent' ? '#2e7d32' : '#1976d2' }}>{item.name}</span>
                        <span style={{ marginLeft: 'auto', fontSize: '0.75em', color: item.type === 'agent' ? '#2e7d32' : '#1976d2' }}>
                          {item.type === 'user' ? 'user' : 'agent'}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
                <div
                  ref={replyEditorRef}
                  contentEditable
                  suppressContentEditableWarning
                  onInput={() => handleMentionInput('reply')}
                  onKeyDown={(e) => {
                    if (mentionOpen && handleMentionKeyDown(e)) return;
                    if (e.key === 'Enter' && e.ctrlKey) { e.preventDefault(); handlePostComment(c.id); }
                  }}
                  onPaste={(e) => { e.preventDefault(); document.execCommand('insertText', false, e.clipboardData.getData('text/plain')); }}
                  style={{
                    width: '100%', boxSizing: 'border-box', padding: '8px 12px', borderRadius: '8px',
                    border: '1px solid #ddd', fontSize: '0.85em', fontFamily: 'inherit',
                    outline: 'none', lineHeight: 1.4, minHeight: '80px',
                    background: '#fff',
                  }}
                />
                <div style={{ display: 'flex', gap: '6px', marginTop: '6px' }}>
                  <button onClick={() => handlePostComment(c.id)}
                    disabled={posting}
                    style={{
                      padding: '4px 14px', borderRadius: '4px', border: 'none',
                      background: posting ? '#ccc' : '#1976d2',
                      color: '#fff', cursor: posting ? 'default' : 'pointer',
                      fontSize: '0.8em', fontWeight: 600,
                    }}
                  >{posting ? '...' : t('commentPost')}</button>
                  <button onClick={() => setReplyToId(null)}
                    style={{
                      padding: '4px 14px', borderRadius: '4px', border: '1px solid #ddd',
                      background: '#fff', cursor: 'pointer', fontSize: '0.8em', color: '#666',
                    }}
                  >{t('cancel')}</button>
                </div>
              </div>
            )}
          </div>
        </div>
        {replies.map(r => renderComment(r, all, depth + 1))}
      </div>
    );
  };

  // --- @mention autocomplete ---
  const closeMention = () => {
    setMentionOpen(false);
    setMentionSearch('');
    setMentionIndex(0);
  };

  const handleMentionInput = (editor: 'main' | 'reply') => {
    const ref = editor === 'main' ? commentEditorRef.current : replyEditorRef.current;
    if (!ref) return;

    const sel = window.getSelection();
    if (!sel || !sel.rangeCount) { closeMention(); return; }
    const range = sel.getRangeAt(0);

    // Get text before cursor
    const preRange = document.createRange();
    preRange.selectNodeContents(ref);
    preRange.setEnd(range.startContainer, range.startOffset);
    const textBefore = preRange.toString();

    const atIdx = textBefore.lastIndexOf('@');
    if (atIdx === -1 || textBefore.length - atIdx > 30) { closeMention(); return; }

    const search = textBefore.slice(atIdx + 1);
    if (search.includes(' ')) { closeMention(); return; }

    setMentionEditor(editor);
    setMentionSearch(search);
    setMentionIndex(0);
    setMentionItems(
      mentionCandidates.current.filter(c =>
        c.name.toLowerCase().includes(search.toLowerCase())
      ).slice(0, 20)
    );
    setMentionOpen(true);
  };

  const insertMention = (item: { id: string; name: string; type: 'user' | 'agent' }) => {
    const ref = mentionEditor === 'main' ? commentEditorRef.current : replyEditorRef.current;
    if (!ref) return;

    const sel = window.getSelection();
    if (!sel || !sel.rangeCount) { closeMention(); return; }
    const range = sel.getRangeAt(0);

    // Walk backwards from cursor to find the @ character in the same text node
    const node = range.startContainer;
    if (node.nodeType === Node.TEXT_NODE) {
      const text = node.textContent || '';
      const offset = range.startOffset;
      const atIdx = text.lastIndexOf('@', offset - 1);
      if (atIdx !== -1) {
        range.setStart(node, atIdx);
      }
    }
    range.deleteContents();

    // Insert @mention as colored span in the editor
    const span = document.createElement('span');
    span.style.color = item.type === 'agent' ? '#2e7d32' : '#1976d2';
    span.style.fontWeight = '500';
    span.textContent = `@${item.name}`;
    range.insertNode(span);
    // Add a trailing space after the colored mention
    const space = document.createTextNode(' ');
    span.parentNode?.insertBefore(space, span.nextSibling);
    range.setStartAfter(space);
    range.collapse(true);
    sel.removeAllRanges();
    sel.addRange(range);

    // Notify React of content change
    const evt = new Event('input', { bubbles: true });
    ref.dispatchEvent(evt);
    closeMention();
  };

  const handleMentionKeyDown = (e: React.KeyboardEvent): boolean => {
    if (!mentionOpen) return false;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setMentionIndex(i => Math.min(i + 1, mentionItems.length - 1));
      return true;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      setMentionIndex(i => Math.max(i - 1, 0));
      return true;
    }
    if (e.key === 'Enter' || e.key === 'Tab') {
      if (mentionItems[mentionIndex]) {
        e.preventDefault();
        insertMention(mentionItems[mentionIndex]);
        return true;
      }
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      closeMention();
      return true;
    }
    return false;
  };
  // --- end @mention ---

  const handleDeleteComment = async (commentId: string) => {
    if (confirmDeleteComment !== commentId) {
      setConfirmDeleteComment(commentId);
      return;
    }
    setConfirmDeleteComment(null);
    try {
      await commentsApi.delete(currentTask.id, commentId);
      setComments(prev => prev.filter(c => c.id !== commentId));
    } catch {
      alert('Failed to delete comment');
    }
  };

  const handleCommentKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (mentionOpen && handleMentionKeyDown(e)) return;
    if (e.key === 'Enter' && e.ctrlKey) {
      e.preventDefault();
      handlePostComment();
    }
  };

  const handleDeleteClick = () => {
    setShowDeleteVerify(true);
  };

  const handleDeleteConfirm = async () => {
    setShowDeleteVerify(false);
    onDelete(currentTask.id);
  };

  const sc = (() => {
    const map: Record<TaskStatus, { bg: string; color: string }> = {
      todo: { bg: '#e0e0e0', color: '#616161' },
      in_progress: { bg: '#bbdefb', color: '#1565c0' },
      blocked: { bg: '#d1c4e9', color: '#4527a0' },
      completed: { bg: '#d4edda', color: '#155724' },
      review: { bg: '#ffe0b2', color: '#e65100' },
      done: { bg: '#c8e6c9', color: '#2e7d32' },
      stuck: { bg: '#f8d7da', color: '#721c24' },
    };
    return map[currentTask.status] || map.todo;
  })();

  const pc = (() => {
    const map: Record<Priority, { bg: string; color: string }> = {
      urgent: { bg: '#ffcdd2', color: '#c62828' },
      high: { bg: '#ffe0b2', color: '#e65100' },
      medium: { bg: '#bbdefb', color: '#1565c0' },
      low: { bg: '#e0e0e0', color: '#757575' },
    };
    return map[currentTask.priority] || map.medium;
  })();

  const statusLabel: Record<TaskStatus, string> = {
    todo: t('taskStatusTodo'),
    in_progress: t('taskStatusInProgress'),
    blocked: t('taskStatusBlocked'),
    completed: t('taskStatusCompleted'),
    review: t('taskStatusReview'),
    done: t('taskStatusDone'),
    stuck: t('taskStatusStuck'),
  };

  const statusColors: Record<TaskStatus, { bg: string; color: string }> = {
    todo: { bg: '#e0e0e0', color: '#616161' },
    in_progress: { bg: '#bbdefb', color: '#1565c0' },
    blocked: { bg: '#d1c4e9', color: '#4527a0' },
    completed: { bg: '#d4edda', color: '#155724' },
    review: { bg: '#ffe0b2', color: '#e65100' },
    done: { bg: '#c8e6c9', color: '#2e7d32' },
    stuck: { bg: '#f8d7da', color: '#721c24' },
  };

  const priorityColors: Record<Priority, { bg: string; color: string }> = {
    urgent: { bg: '#ffcdd2', color: '#c62828' },
    high: { bg: '#ffe0b2', color: '#e65100' },
    medium: { bg: '#bbdefb', color: '#1565c0' },
    low: { bg: '#e0e0e0', color: '#757575' },
  };

  const priorityLabel: Record<Priority, string> = {
    urgent: t('priorityUrgent'),
    high: t('priorityHigh'),
    medium: t('priorityMedium'),
    low: t('priorityLow'),
  };

  const editableSelectStyle: React.CSSProperties = {
    width: '100%',
    padding: '6px 8px',
    borderRadius: '6px',
    border: '1px solid #ddd',
    fontSize: '0.85em',
    background: '#fff',
    boxSizing: 'border-box',
  };

  const sidebarLabelStyle: React.CSSProperties = {
    fontSize: '0.75em',
    fontWeight: 600,
    color: '#999',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
    marginBottom: '4px',
  };

  return (
    <>
      <style>{`
        @keyframes task-card-spin {
          to { transform: rotate(360deg); }
        }
      `}</style>
    <div
      onClick={onClose}
      style={{
        position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)',
        display: 'flex', justifyContent: 'center', alignItems: 'flex-start',
        zIndex: 1000, overflowY: 'auto', padding: '30px 20px',
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: '#fff', borderRadius: '16px',
          width: '880px', maxWidth: '95vw',
          boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
          overflow: 'hidden', marginTop: '20px',
        }}
      >
        {/* Header bar */}
        <div style={{
          display: 'flex', justifyContent: 'space-between', alignItems: 'center',
          padding: '16px 24px', borderBottom: '1px solid #eee',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span style={{ fontSize: '0.85em', color: '#999' }}>
              {saving ? t('taskDetailSaving') : `#${currentTask.id.slice(0, 8)}`}
            </span>
            <span style={{
              fontSize: '0.75em', padding: '2px 8px', borderRadius: '10px',
              background: sc.bg, color: sc.color, fontWeight: 500,
            }}>
              {statusLabel[currentTask.status]}
            </span>
            <span style={{
              fontSize: '0.7em', padding: '2px 8px', borderRadius: '10px',
              background: pc.bg, color: pc.color, fontWeight: 600,
              textTransform: 'uppercase',
            }}>
              {priorityLabel[currentTask.priority]}
            </span>
            {isProcessing && (
              <span
                title="Agent processing..."
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
          <button onClick={onClose}
            style={{
              width: '32px', height: '32px', borderRadius: '50%', border: 'none',
              background: '#f5f5f5', cursor: 'pointer', fontSize: '1.1em',
              display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#666',
            }}
          >✕</button>
        </div>

        {/* Main content: two columns */}
        <div style={{ display: 'flex', gap: '0', minHeight: '400px' }}>
          {/* LEFT COLUMN: Title, creator, description, subtasks */}
          <div style={{ flex: '1', padding: '24px', minWidth: 0 }}>
            {/* Editable Title */}
            <input
              ref={titleRef}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onBlur={handleTitleSave}
              onKeyDown={handleTitleKeyDown}
              style={{
                width: '100%', fontSize: '1.5em', fontWeight: 700, color: '#1a1a2e',
                border: 'none', outline: 'none', padding: '0', marginBottom: '8px',
                background: 'transparent', fontFamily: 'inherit',
                borderBottom: '2px solid transparent',
              }}
              onFocus={(e) => { e.currentTarget.style.borderBottomColor = '#1976d2'; }}
              onBlurCapture={(e) => { e.currentTarget.style.borderBottomColor = 'transparent'; handleTitleSave(); }}
            />

            {/* Creator info (read-only) */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '20px', fontSize: '0.85em', color: '#888' }}>
              <span>✏️ {currentTask.creator_name || t('taskDetailUnknown')}</span>
              <span>·</span>
              <span>📅 {new Date(currentTask.created_at).toLocaleDateString()}</span>
              {currentTask.updated_at !== currentTask.created_at && (
                <>
                  <span>·</span>
                  <span>{t('taskDetailUpdated')} {new Date(currentTask.updated_at).toLocaleDateString()}</span>
                </>
              )}
            </div>

            {/* Description (read-only) */}
            <div style={{ marginBottom: '24px' }}>
              <h4 style={{ margin: '0 0 8px', fontSize: '0.85em', color: '#999', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                {t('taskDescription')}
              </h4>
              <div style={{
                background: '#f9f9f9', borderRadius: '8px', padding: '16px',
                fontSize: '0.95em', lineHeight: 1.6, color: '#333',
                wordBreak: 'break-word',
                border: '1px solid #eee',
                minHeight: '60px',
              }}>
                {currentTask.description
                  ? <span dangerouslySetInnerHTML={{ __html: currentTask.description }} />
                  : <span style={{ color: '#ccc', fontStyle: 'italic' }}>{t('taskDetailNoDescription')}</span>}
              </div>
            </div>

            {/* Decomposition Plan Review Panel */}
            {decompPlan && decompPlan.status === 'pending' && (
              <div style={{ marginBottom: '24px', padding: '16px', background: '#fff8e1', borderRadius: '12px', border: '1px solid #ffe0b2' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
                  <span style={{ fontSize: '1.1em' }}>📋</span>
                  <span style={{ fontWeight: 700, fontSize: '0.95em', color: '#e65100' }}>{t('decompPlanTitle')}</span>
                  <span style={{
                    fontSize: '0.7em', padding: '2px 8px', borderRadius: '8px',
                    background: '#fff3e0', color: '#e65100', fontWeight: 500, border: '1px solid #ffe0b2',
                  }}>
                    {t('decompPlanPending')}
                  </span>
                  <span style={{ fontSize: '0.8em', color: '#888', marginLeft: 'auto' }}>
                    {t('decompPlanBy')} {decompPlan.created_by_name || decompPlan.created_by.slice(0, 8)}
                  </span>
                </div>

                {/* Summary */}
                {decompPlan.summary && (
                  <div style={{
                    fontSize: '0.85em', color: '#555', marginBottom: '12px',
                    padding: '8px 12px', background: '#fff', borderRadius: '8px',
                    border: '1px solid #ffe0b2',
                  }}>
                    <span style={{ fontWeight: 600, color: '#e65100' }}>{t('decompPlanSummary')}: </span>
                    {decompPlan.summary}
                  </div>
                )}

                {/* Items */}
                {decompItems.length === 0 ? (
                  <div style={{ textAlign: 'center', color: '#999', fontSize: '0.85em', padding: '12px' }}>
                    {t('decompPlanNoItems')}
                  </div>
                ) : (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', marginBottom: '12px' }}>
                    {decompItems.map(item => {
                      const isPending = item.is_approved === null;
                      const isApproved = item.is_approved === true;
                      const isRejected = item.is_approved === false;
                      const isSelected = selectedItems.has(item.id);
                      const dependsDisplay = Array.isArray(item.depends_on) ? item.depends_on : [];
                      return (
                        <div key={item.id} style={{
                          display: 'flex', alignItems: 'flex-start', gap: '10px',
                          padding: '10px 12px', background: isApproved ? '#e8f5e9' : isRejected ? '#ffebee' : '#fff',
                          borderRadius: '8px', border: isSelected ? '2px solid #1976d2' : '1px solid #e0e0e0',
                          opacity: isRejected ? 0.6 : 1,
                          transition: 'border-color 0.15s',
                        }}>
                          {/* Checkbox — only for pending items */}
                          {isPending ? (
                            <input
                              type="checkbox"
                              checked={isSelected}
                              onChange={() => toggleItemSelection(item.id)}
                              style={{ marginTop: '2px', cursor: 'pointer', flexShrink: 0 }}
                            />
                          ) : (
                            <span style={{
                              width: '16px', height: '16px', borderRadius: '3px', flexShrink: 0,
                              display: 'flex', alignItems: 'center', justifyContent: 'center',
                              fontSize: '0.75em', marginTop: '2px',
                              background: isApproved ? '#c8e6c9' : '#ffcdd2',
                              color: isApproved ? '#2e7d32' : '#c62828',
                            }}>
                              {isApproved ? '✓' : '✕'}
                            </span>
                          )}
                          {/* Item content */}
                          <div style={{ flex: 1, minWidth: 0 }}>
                            <div style={{ fontWeight: 600, fontSize: '0.9em', color: '#333', marginBottom: '2px' }}>
                              {item.title}
                            </div>
                            {item.description && (
                              <div style={{
                                fontSize: '0.8em', color: '#666', marginBottom: '4px',
                                overflow: 'hidden', textOverflow: 'ellipsis',
                                maxHeight: '2.8em', lineHeight: 1.4,
                              }}>
                                {item.description.length > 200
                                  ? item.description.slice(0, 200) + '...'
                                  : item.description}
                              </div>
                            )}
                            {/* Tags */}
                            <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px' }}>
                              {item.assignee_name && (
                                <span style={{
                                  fontSize: '0.7em', padding: '1px 6px', borderRadius: '4px',
                                  background: '#e3f2fd', color: '#1565c0',
                                }}>
                                  👤 {item.assignee_name}
                                </span>
                              )}
                              {dependsDisplay.length > 0 && (
                                <span style={{
                                  fontSize: '0.7em', padding: '1px 6px', borderRadius: '4px',
                                  background: '#f3e5f5', color: '#7b1fa2',
                                }}>
                                  ⛓ {dependsDisplay.length} dep{dependsDisplay.length > 1 ? 's' : ''}
                                </span>
                              )}
                              {item.parallel_group && (
                                <span style={{
                                  fontSize: '0.7em', padding: '1px 6px', borderRadius: '4px',
                                  background: '#fff3e0', color: '#e65100',
                                }}>
                                  ⇄ {item.parallel_group}
                                </span>
                              )}
                              {item.completion_behavior === 'auto_done' && (
                                <span style={{
                                  fontSize: '0.7em', padding: '1px 6px', borderRadius: '4px',
                                  background: '#e8f5e9', color: '#2e7d32',
                                }}>
                                  auto
                                </span>
                              )}
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}

                {/* Actions */}
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                  <button onClick={toggleSelectAll}
                    style={{
                      padding: '5px 12px', borderRadius: '6px', border: '1px solid #1976d2',
                      background: '#e3f2fd', color: '#1976d2', cursor: 'pointer',
                      fontSize: '0.8em', fontWeight: 500,
                    }}
                  >
                    {selectedItems.size === decompItems.filter(i => i.is_approved === null).length
                      ? t('decompPlanDeselectAll') : t('decompPlanSelectAll')}
                  </button>
                  <button onClick={() => handleApprovePlan(Array.from(selectedItems))}
                    disabled={approving || selectedItems.size === 0}
                    style={{
                      padding: '5px 12px', borderRadius: '6px', border: 'none',
                      background: (approving || selectedItems.size === 0) ? '#ccc' : '#2e7d32',
                      color: '#fff', cursor: (approving || selectedItems.size === 0) ? 'default' : 'pointer',
                      fontSize: '0.8em', fontWeight: 600,
                    }}
                  >
                    {approving ? '...' : t('decompPlanApproveSelected') + ` (${selectedItems.size})`}
                  </button>
                  <button onClick={() => handleApprovePlan(undefined)}
                    disabled={approving}
                    style={{
                      padding: '5px 12px', borderRadius: '6px', border: '1px solid #2e7d32',
                      background: approving ? '#ccc' : '#e8f5e9', color: '#2e7d32',
                      cursor: approving ? 'default' : 'pointer', fontSize: '0.8em', fontWeight: 500,
                    }}
                  >
                    {approving ? '...' : t('decompPlanApproveAll')}
                  </button>
                  <div style={{ flex: 1 }} />
                  {showRejectInput ? (
                    <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
                      <input
                        value={rejectComment}
                        onChange={e => setRejectComment(e.target.value)}
                        placeholder={t('decompPlanRejectPlaceholder')}
                        style={{
                          padding: '5px 8px', borderRadius: '6px', border: '1px solid #ddd',
                          fontSize: '0.8em', width: '180px',
                        }}
                      />
                      <button onClick={handleRejectPlan}
                        disabled={rejecting}
                        style={{
                          padding: '5px 12px', borderRadius: '6px', border: 'none',
                          background: rejecting ? '#ccc' : '#c62828',
                          color: '#fff', cursor: rejecting ? 'default' : 'pointer',
                          fontSize: '0.8em', fontWeight: 600,
                        }}
                      >
                        {rejecting ? '...' : t('decompPlanConfirmReject')}
                      </button>
                      <button onClick={() => { setShowRejectInput(false); setRejectComment(''); }}
                        style={{
                          padding: '5px 8px', borderRadius: '6px', border: '1px solid #ddd',
                          background: '#fff', cursor: 'pointer', fontSize: '0.8em', color: '#666',
                        }}
                      >{t('cancel')}</button>
                    </div>
                  ) : (
                    <button onClick={() => setShowRejectInput(true)}
                      disabled={rejecting}
                      style={{
                        padding: '5px 12px', borderRadius: '6px', border: '1px solid #c62828',
                        background: rejecting ? '#ccc' : '#fff', color: '#c62828',
                        cursor: rejecting ? 'default' : 'pointer', fontSize: '0.8em', fontWeight: 500,
                      }}
                    >
                      {rejecting ? '...' : t('decompPlanReject')}
                    </button>
                  )}
                </div>
              </div>
            )}

            {/* Subtasks */}
            {subtasks.length > 0 && (
              <div style={{ marginBottom: '24px' }}>
                <h4 style={{ margin: '0 0 8px', fontSize: '0.85em', color: '#999', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                  {t('taskDetailSubtasks')} ({subtasks.length})
                </h4>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                  {subtasks.map(st => {
                    const ssc = statusColors[st.status];
                    return (
                      <div key={st.id} style={{
                        display: 'flex', alignItems: 'center', gap: '8px',
                        padding: '8px 12px', background: '#f9f9f9', borderRadius: '8px',
                        border: '1px solid #eee',
                      }}>
                        <span style={{
                          width: '10px', height: '10px', borderRadius: '50%',
                          background: ssc.color, flexShrink: 0,
                        }} />
                        <span style={{ flex: 1, fontSize: '0.9em', color: '#333' }}>
                          {st.title}
                        </span>
                        {st.assignee_id && (
                          <span style={{ fontSize: '0.8em', color: '#888' }}>
                            👤 {nameMap[st.assignee_id] || st.assignee_id.slice(0, 6)}
                          </span>
                        )}
                        <span style={{
                          fontSize: '0.7em', padding: '1px 6px', borderRadius: '6px',
                          background: ssc.bg, color: ssc.color, fontWeight: 500,
                        }}>
                          {statusLabel[st.status]}
                        </span>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}

            {/* Agent Report — hidden per user request */}
            {false && taskQueueItems.filter(q => q.status === 'completed' && q.result_summary).length > 0 && (
              <div style={{ marginBottom: '24px' }}>
                <h4 style={{ margin: '0 0 8px 0', fontSize: '0.85em', color: '#999', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                  🤖 {t('taskDetailAgentReport') || 'Agent Report'}
                </h4>
                {taskQueueItems.filter(q => q.status === 'completed' && q.result_summary).map(item => (
                  <div key={item.id} style={{
                    padding: '12px 16px', background: '#f8fdf5', border: '1px solid #d4edda',
                    borderRadius: '8px', marginBottom: '8px',
                  }}>
                    <div style={{ fontSize: '0.75em', color: '#888', marginBottom: '6px' }}>
                      {new Date(item.completed_at || '').toLocaleString()}
                    </div>
                    <div style={{
                      fontSize: '0.9em', lineHeight: 1.7, color: '#333',
                      whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                    }}>
                      {item.result_summary}
                    </div>
                  </div>
                ))}
              </div>
            )}

            {/* Comments */}
            <div style={{ marginBottom: '24px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
                <h4 style={{ margin: 0, fontSize: '0.85em', color: '#999', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                  {t('taskDetailComments')} ({comments.length})
                </h4>
                <button
                  onClick={() => setCommentSortAsc(!commentSortAsc)}
                  title={commentSortAsc ? 'Switch to newest first' : 'Switch to oldest first'}
                  style={{
                    background: 'none', border: '1px solid #ddd', borderRadius: '4px',
                    cursor: 'pointer', padding: '2px 6px', fontSize: '0.75em', color: '#888',
                    display: 'flex', alignItems: 'center', gap: '3px',
                    transition: 'background 0.15s',
                  }}
                  onMouseEnter={e => { e.currentTarget.style.background = '#f5f5f5'; }}
                  onMouseLeave={e => { e.currentTarget.style.background = 'none'; }}
                >
                  {commentSortAsc ? '↑ Oldest' : '↓ Newest'}
                </button>
              </div>

              {/* Comments list — threaded */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '12px' }}>
                {comments.length === 0 ? (
                  <div style={{ textAlign: 'center', color: '#ccc', padding: '16px', fontSize: '0.85em' }}>
                    {t('commentEmpty')}
                  </div>
                ) : (
                  [...comments]
                    .filter(c => !c.parent_id)
                    .sort((a, b) => {
                      const diff = new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
                      return commentSortAsc ? diff : -diff;
                    })
                    .map(c => renderComment(c, comments, 0))
                )}
              </div>

              {/* Comment input — Rich Text */}
              <div>
                {/* Mini toolbar */}
                <div style={{ display: 'flex', gap: '3px', marginBottom: '4px' }}>
                  <button type="button" onMouseDown={(e) => { e.preventDefault(); document.execCommand('bold'); commentEditorRef.current?.focus(); }}
                    style={{ padding: '2px 8px', borderRadius: '4px', border: '1px solid #ddd', background: '#fafafa', cursor: 'pointer', fontSize: '0.8em', lineHeight: 1.4 }} title="Bold"><b>B</b></button>
                  <button type="button" onMouseDown={(e) => { e.preventDefault(); document.execCommand('italic'); commentEditorRef.current?.focus(); }}
                    style={{ padding: '2px 8px', borderRadius: '4px', border: '1px solid #ddd', background: '#fafafa', cursor: 'pointer', fontSize: '0.8em', lineHeight: 1.4 }} title="Italic"><i>I</i></button>
                  <button type="button" onMouseDown={(e) => { e.preventDefault(); document.execCommand('underline'); commentEditorRef.current?.focus(); }}
                    style={{ padding: '2px 8px', borderRadius: '4px', border: '1px solid #ddd', background: '#fafafa', cursor: 'pointer', fontSize: '0.8em', lineHeight: 1.4 }} title="Underline"><u>U</u></button>
                  <button type="button" onMouseDown={(e) => { e.preventDefault(); document.execCommand('insertUnorderedList'); commentEditorRef.current?.focus(); }}
                    style={{ padding: '2px 8px', borderRadius: '4px', border: '1px solid #ddd', background: '#fafafa', cursor: 'pointer', fontSize: '0.8em', lineHeight: 1.4 }} title="Bullet List">•</button>
                </div>
                <div style={{ position: 'relative' }}>
                  {mentionOpen && mentionEditor === 'main' && (
                    <div style={{
                      position: 'absolute', bottom: '100%', left: 0, zIndex: 999,
                      background: '#fff', border: '1px solid #ddd', borderRadius: '8px',
                      boxShadow: '0 4px 16px rgba(0,0,0,0.15)', maxHeight: '200px', overflow: 'auto',
                      minWidth: '200px',
                    }}>
                      {mentionItems.length === 0 ? (
                        <div style={{ padding: '8px 12px', color: '#999', fontSize: '0.85em' }}>No results</div>
                      ) : mentionItems.map((item, i) => (
                        <div key={item.id}
                          onClick={() => insertMention(item)}
                          onMouseEnter={() => setMentionIndex(i)}
                          style={{
                            padding: '8px 12px', cursor: 'pointer', fontSize: '0.85em',
                            background: i === mentionIndex ? '#e3f2fd' : 'transparent',
                            display: 'flex', alignItems: 'center', gap: '6px',
                          }}
                        >
                          <span>{item.type === 'user' ? '👤' : '🤖'}</span>
                          <span style={{ color: item.type === 'agent' ? '#2e7d32' : '#1976d2' }}>{item.name}</span>
                          <span style={{ marginLeft: 'auto', fontSize: '0.75em', color: item.type === 'agent' ? '#2e7d32' : '#1976d2' }}>
                            {item.type === 'user' ? 'user' : 'agent'}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                  <div
                    ref={commentEditorRef}
                    contentEditable
                    suppressContentEditableWarning
                    onInput={() => {
                      setCommentInput(commentEditorRef.current?.innerHTML || '');
                      handleMentionInput('main');
                    }}
                    onKeyDown={handleCommentKeyDown}
                    onPaste={(e) => {
                      e.preventDefault();
                      document.execCommand('insertText', false, e.clipboardData.getData('text/plain'));
                    }}
                    data-placeholder={t('commentPlaceholder')}
                    style={{
                      width: '100%', boxSizing: 'border-box', padding: '8px 12px', borderRadius: '8px',
                      border: '1px solid #ddd', fontSize: '0.85em', fontFamily: 'inherit',
                      outline: 'none', lineHeight: 1.4, minHeight: '160px',
                      whiteSpace: 'pre-wrap', wordBreak: 'break-word', overflowWrap: 'break-word',
                    }}
                  />
                  <div style={{ display: 'flex', justifyContent: 'center', marginTop: '6px' }}>
                    <button onClick={(e) => { e.preventDefault(); handlePostComment(); }}
                      disabled={posting}
                      style={{
                        padding: '6px 20px', borderRadius: '6px', border: 'none',
                        background: posting ? '#ccc' : '#1976d2',
                        color: '#fff', cursor: posting ? 'default' : 'pointer',
                        fontSize: '0.85em', fontWeight: 600,
                      }}
                    >{posting ? '...' : t('commentPost')}</button>
                  </div>
                </div>
              </div>
            </div>

            {/* Due date for overdue tasks */}
            {isOverdue && (
              <div style={{
                padding: '10px 14px', background: '#fff3e0', borderRadius: '8px',
                border: '1px solid #ffe0b2', fontSize: '0.85em', color: '#e65100',
                marginBottom: '16px',
              }}>
                ⚠ {t('taskDetailOverdue')} ({new Date(currentTask.due_at!).toLocaleDateString()})
              </div>
            )}
          </div>

          {/* RIGHT COLUMN: Sidebar with editable fields */}
          <div style={{
            width: '280px', padding: '24px', borderLeft: '1px solid #eee',
            display: 'flex', flexDirection: 'column', gap: '16px', flexShrink: 0,
          }}>
            {/* Status */}
            <div>
              <div style={sidebarLabelStyle}>{t('taskStatus')}</div>
              <div style={{ position: 'relative', display: 'flex', alignItems: 'center', gap: '6px' }}>
                <span style={{
                  width: '10px', height: '10px', borderRadius: '50%',
                  background: sc.color, flexShrink: 0,
                }} />
                <select
                  value={currentTask.status}
                  onChange={(e) => handleStatusChange(e.target.value as TaskStatus)}
                  style={editableSelectStyle}
                >
                  {statusOptions.map(s => (
                    <option key={s} value={s} style={{
                      background: statusColors[s].bg,
                      color: statusColors[s].color,
                      fontWeight: currentTask.status === s ? 600 : 400,
                    }}>{statusLabel[s]}</option>
                  ))}
                </select>
              </div>
            </div>

            {/* Review actions — shown when status is review */}
            {currentTask.status === 'review' && (
              <div style={{
                padding: '12px', borderRadius: '8px',
                background: '#fff8e1', border: '1px solid #ffe0b2',
              }}>
                <div style={sidebarLabelStyle}>{t('reviewComment')}</div>
                <textarea
                  value={reviewComment}
                  onChange={(e) => setReviewComment(e.target.value)}
                  placeholder={t('reviewCommentPlaceholder')}
                  style={{
                    width: '100%', boxSizing: 'border-box', padding: '6px 8px',
                    borderRadius: '6px', border: '1px solid #ddd',
                    fontSize: '0.85em', fontFamily: 'inherit', minHeight: '60px',
                    marginBottom: '8px', resize: 'vertical',
                  }}
                />
                <div style={{ display: 'flex', gap: '6px' }}>
                  <button
                    onClick={() => handleReview('approved')}
                    disabled={reviewing}
                    style={{
                      flex: 1, padding: '6px 0', borderRadius: '6px', border: 'none',
                      background: reviewing ? '#ccc' : '#2e7d32',
                      color: '#fff', cursor: reviewing ? 'default' : 'pointer',
                      fontSize: '0.85em', fontWeight: 600,
                    }}
                  >{t('reviewApprove')}</button>
                  <button
                    onClick={() => handleReview('rejected')}
                    disabled={reviewing}
                    style={{
                      flex: 1, padding: '6px 0', borderRadius: '6px', border: 'none',
                      background: reviewing ? '#ccc' : '#c62828',
                      color: '#fff', cursor: reviewing ? 'default' : 'pointer',
                      fontSize: '0.85em', fontWeight: 600,
                    }}
                  >{t('reviewReject')}</button>
                </div>
                {/* Agent loop info */}
                {typeof currentTask.agent_loop_count === 'number' && (
                  <div style={{
                    marginTop: '8px', fontSize: '0.75em', color: '#e65100',
                    textAlign: 'center',
                  }}>
                    {t('reviewAgentLoopCount')}: {currentTask.agent_loop_count}
                    {typeof currentTask.max_agent_loops === 'number' && ` / ${currentTask.max_agent_loops}`}
                  </div>
                )}
              </div>
            )}

            {/* Stuck info */}
            {currentTask.status === 'stuck' && (
              <div style={{
                padding: '10px 12px', borderRadius: '8px',
                background: '#f8d7da', border: '1px solid #f5c6cb',
                fontSize: '0.85em', color: '#721c24',
              }}>
                {t('reviewMeltdown')}
                {typeof currentTask.agent_loop_count === 'number' && (
                  <span> — {t('reviewAgentLoopCount')}: {currentTask.agent_loop_count}</span>
                )}
              </div>
            )}

            {/* Priority */}
            <div>
              <div style={sidebarLabelStyle}>{t('taskDetailPriority')}</div>
              <div style={{ position: 'relative', display: 'flex', alignItems: 'center', gap: '6px' }}>
                <span style={{
                  width: '10px', height: '10px', borderRadius: '50%',
                  background: pc.color, flexShrink: 0,
                }} />
                <select
                  value={currentTask.priority}
                  onChange={(e) => handlePriorityChange(e.target.value as Priority)}
                  style={editableSelectStyle}
                >
                  {priorityOptions.map(p => (
                    <option key={p} value={p} style={{
                      background: priorityColors[p].bg,
                      color: priorityColors[p].color,
                      fontWeight: currentTask.priority === p ? 600 : 400,
                    }}>{priorityLabel[p]}</option>
                  ))}
                </select>
              </div>
            </div>

            {/* Assignee (Responsible Person) */}
            <div>
              <div style={sidebarLabelStyle}>{t('taskDetailAssignee')}</div>
              <select
                value={currentTask.assignee_id ? `${currentTask.assignee_type || 'user'}:${currentTask.assignee_id}` : ''}
                onChange={(e) => {
                  const val = e.target.value;
                  if (!val) {
                    handleAssigneeChange(null, null);
                  } else {
                    const [type, id] = val.split(':');
                    handleAssigneeChange(id, type as AssigneeType);
                  }
                }}
                style={editableSelectStyle}
              >
                <option value="">{t('taskDetailUnassigned')}</option>
                <optgroup label={t('taskDetailUser') + 's'}>
                  {members.map(m => (
                    <option key={`user:${m.user_id}`} value={`user:${m.user_id}`}>
                      👤 {m.username}
                    </option>
                  ))}
                </optgroup>
                <optgroup label={t('agents')}>
                  {agentProfiles.filter(a => a.enabled).map(a => (
                    <option key={`agent_profile:${a.id}`} value={`agent_profile:${a.id}`}>
                      {a.avatar || '🤖'} {a.name} ({a.current_load}/{a.max_concurrency})
                    </option>
                  ))}
                </optgroup>
              </select>
              {/* Trigger agent button */}
              {currentTask.assignee_type === 'agent_profile' && (
                <button
                  onClick={handleTriggerAgent}
                  disabled={triggeringAgent || isProcessing}
                  style={{
                    width: '100%', marginTop: '6px', padding: '5px 0',
                    borderRadius: '6px', border: '1px solid #1976d2',
                    background: '#e3f2fd', color: '#1976d2',
                    cursor: (triggeringAgent || isProcessing) ? 'default' : 'pointer',
                    fontSize: '0.8em', fontWeight: 500,
                    opacity: (triggeringAgent || isProcessing) ? 0.6 : 1,
                  }}
                >
                  {triggeringAgent ? '...' : isProcessing ? (lang === 'zh' ? '处理中...' : 'Processing...') : (lang === 'zh' ? '触发智能体处理' : 'Trigger Agent')}
                </button>
              )}
            </div>

            {/* Agent queue status */}
            {taskQueueItems.length > 0 && (
              <div>
                <div style={sidebarLabelStyle}>{t('agents')}</div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '3px' }}>
                  {taskQueueItems.map(item => {
                    const statusColorMap: Record<string, string> = {
                      queued: '#9e9e9e',
                      claimed: '#1976d2',
                      processing: '#f9a825',
                      completed: '#388e3c',
                      failed: '#d32f2f',
                    };
                    const statusKeyMap: Record<string, string> = {
                      queued: t('agentQueueQueued'),
                      claimed: t('agentQueueClaimed'),
                      processing: t('agentQueueProcessing'),
                      completed: t('agentQueueCompleted'),
                      failed: t('agentQueueFailed'),
                    };
                    return (
                      <div key={item.id} style={{
                        display: 'flex', alignItems: 'center', gap: '6px',
                        padding: '4px 8px', borderRadius: '6px', background: '#f5f5f5',
                        fontSize: '0.8em',
                      }}>
                        <span style={{
                          width: '8px', height: '8px', borderRadius: '50%',
                          background: statusColorMap[item.status] || '#999',
                          flexShrink: 0,
                        }} />
                        <span style={{ fontWeight: 500, color: '#555' }}>
                          {statusKeyMap[item.status] || item.status}
                        </span>
                        {item.status === 'completed' && item.result_summary && (
                          <span style={{
                            color: '#888', flex: 1, overflow: 'hidden',
                            textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                            fontSize: '0.9em',
                          }} title={item.result_summary}>
                            {item.result_summary.slice(0, 120)}
                          </span>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            )}

            {/* Delegated Assignees */}
            <div>
              <div style={sidebarLabelStyle}>{t('taskDetailDelegatedAssignees')}</div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                {(currentTask.assignees || []).map(a => (
                  <div key={a.assignee_id} style={{
                    display: 'flex', alignItems: 'center', gap: '4px',
                    padding: '4px 8px', borderRadius: '6px', background: '#f5f5f5',
                    fontSize: '0.85em',
                  }}>
                    <span style={{ flex: 1, color: '#333' }}>
                      {nameMap[a.assignee_id] || a.assignee_id.slice(0, 8)}
                    </span>
                    <button
                      onClick={() => handleRemoveAssignee(a.assignee_id)}
                      style={{
                        background: 'none', border: 'none', cursor: 'pointer',
                        color: '#c62828', padding: '0 2px', fontSize: '1em', lineHeight: 1,
                      }}
                      title={t('taskDetailRemoveAssignee')}
                    >✕</button>
                  </div>
                ))}
                {showAddAssignee ? (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', marginTop: '4px' }}>
                    <select
                      value={newAssigneeType}
                      onChange={(e) => { setNewAssigneeType(e.target.value as AssigneeType); setNewAssigneeId(''); }}
                      style={editableSelectStyle}
                    >
                      <option value="user">{t('taskDetailUser')}</option>
                      <option value="agent_profile">{t('agent')}</option>
                    </select>
                    <select
                      value={newAssigneeId}
                      onChange={(e) => setNewAssigneeId(e.target.value)}
                      style={editableSelectStyle}
                    >
                      <option value="">{t('taskDetailSelect')}</option>
                      {(newAssigneeType === 'user' ? members : agentProfiles.filter(a => a.enabled)).map((item: WorkspaceMember | AgentProfile) => {
                        const id = 'user_id' in item ? (item as WorkspaceMember).user_id : (item as AgentProfile).id;
                        const name = 'username' in item ? (item as WorkspaceMember).username : (item as AgentProfile).name;
                        const icon = 'username' in item ? '👤' : ((item as AgentProfile).avatar || '🤖');
                        return (
                          <option key={id} value={id}>{icon} {name}</option>
                        );
                      })}
                    </select>
                    <div style={{ display: 'flex', gap: '4px' }}>
                      <button onClick={handleAddAssignee}
                        style={{
                          flex: 1, padding: '4px 8px', borderRadius: '4px', border: 'none',
                          background: '#1976d2', color: '#fff', cursor: 'pointer', fontSize: '0.8em',
                        }}
                      >{t('taskDetailAdd')}</button>
                      <button onClick={() => setShowAddAssignee(false)}
                        style={{
                          padding: '4px 8px', borderRadius: '4px', border: '1px solid #ddd',
                          background: '#fff', cursor: 'pointer', fontSize: '0.8em',
                        }}
                      >{t('cancel')}</button>
                    </div>
                  </div>
                ) : (
                  <button onClick={() => setShowAddAssignee(true)}
                    style={{
                      padding: '4px 8px', borderRadius: '4px', border: '1px dashed #ccc',
                      background: 'transparent', cursor: 'pointer', fontSize: '0.8em', color: '#888',
                      textAlign: 'center', marginTop: '2px',
                    }}
                  >{t('taskDetailAddAssignee')}</button>
                )}
              </div>
            </div>

            {/* Tags */}
            <div>
              <div style={sidebarLabelStyle}>{t('taskDetailTags')}</div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px', marginBottom: '4px' }}>
                {(currentTask.tags || []).map(tag => (
                  <span key={tag} style={{
                    display: 'inline-flex', alignItems: 'center', gap: '3px',
                    padding: '2px 6px', borderRadius: '6px', background: '#e3f2fd',
                    color: '#1565c0', fontSize: '0.8em',
                  }}>
                    {tag}
                    <button onClick={() => handleRemoveTag(tag)}
                      style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#1565c0', padding: 0, fontSize: '1em', lineHeight: 1 }}
                    >×</button>
                  </span>
                ))}
              </div>
              <div style={{ display: 'flex', gap: '4px' }}>
                <input
                  value={tagInput}
                  onChange={(e) => setTagInput(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); handleAddTag(); } }}
                  placeholder={t('taskDetailAddTag')}
                  style={{
                    flex: 1, padding: '4px 8px', borderRadius: '4px', border: '1px solid #ddd',
                    fontSize: '0.8em', outline: 'none',
                  }}
                />
                <button onClick={handleAddTag}
                  style={{
                    padding: '4px 8px', borderRadius: '4px', border: '1px solid #ddd',
                    background: '#f5f5f5', cursor: 'pointer', fontSize: '0.8em',
                  }}
                >{t('taskDetailAdd')}</button>
              </div>
            </div>

            {/* Due Date */}
            <div>
              <div style={sidebarLabelStyle}>{t('taskDetailDueDate')}</div>
              <input
                type="date"
                value={currentTask.due_at ? currentTask.due_at.slice(0, 10) : ''}
                onChange={(e) => handleDueAtChange(e.target.value)}
                style={{
                  ...editableSelectStyle,
                  color: isOverdue ? '#c62828' : '#333',
                }}
              />
            </div>

            {/* Project */}
            <div>
              <div style={sidebarLabelStyle}>{t('taskDetailProject')}</div>
              <select
                value={currentTask.project_id || ''}
                onChange={(e) => handleProjectChange(e.target.value || null)}
                style={editableSelectStyle}
              >
                <option value="">{t('defaultProject')}</option>
                {projects.map(p => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </select>
            </div>

            {/* Parent Task */}
            <div>
              <div style={sidebarLabelStyle}>{t('taskDetailParentTask')}</div>
              <select
                value={currentTask.parent_id || ''}
                onChange={(e) => handleParentChange(e.target.value || null)}
                style={editableSelectStyle}
              >
                <option value="">{t('taskDetailNoneTopLevel')}</option>
                {allTasks.map(t => (
                  <option key={t.id} value={t.id}>{t.title}</option>
                ))}
              </select>
            </div>

            {/* Workflow selector */}
            <div>
              <div style={sidebarLabelStyle}>{t('taskWorkflow')}</div>
              <select
                value={currentTask.workflow_id || ''}
                onChange={(e) => handleWorkflowChange(e.target.value || null)}
                style={editableSelectStyle}
              >
                <option value="">{(lang === 'zh' ? '无工作流' : 'No workflow')}</option>
                {workflows.filter(w => w.status === 'active').map(w => (
                  <option key={w.id} value={w.id}>{w.title}</option>
                ))}
              </select>
            </div>

            {/* Workflow / Agent info */}
            {(currentTask.workflow_id || typeof currentTask.agent_loop_count === 'number' || currentTask.completion_behavior) && (
              <div style={{ borderTop: '1px solid #eee', paddingTop: '12px' }}>
                {currentTask.workflow_id && (
                  <div style={{ marginBottom: '8px' }}>
                    <div style={sidebarLabelStyle}>{t('taskWorkflow')}</div>
                    <div style={{
                      fontSize: '0.8em', color: '#1565c0', background: '#e3f2fd',
                      padding: '4px 8px', borderRadius: '4px', wordBreak: 'break-all',
                    }}>
                      {currentTask.workflow_id}
                      {typeof currentTask.depth === 'number' && typeof currentTask.max_depth === 'number' && (
                        <span style={{ marginLeft: '8px', color: '#666' }}>
                          Lv.{currentTask.depth}/{currentTask.max_depth}
                        </span>
                      )}
                    </div>
                  </div>
                )}
                {typeof currentTask.agent_loop_count === 'number' && (
                  <div style={{ marginBottom: '8px' }}>
                    <div style={sidebarLabelStyle}>{t('taskAgentLoops')}</div>
                    <div style={{ fontSize: '0.85em', color: '#666' }}>
                      {t('taskAgentLoopCount')}: <strong>{currentTask.agent_loop_count}</strong>
                      {typeof currentTask.max_agent_loops === 'number' && (
                        <span> / {t('taskMaxAgentLoops')}: <strong>{currentTask.max_agent_loops}</strong></span>
                      )}
                    </div>
                  </div>
                )}
                {currentTask.completion_behavior && (
                  <div>
                    <div style={sidebarLabelStyle}>{t('taskCompletionBehavior')}</div>
                    <span style={{
                      fontSize: '0.8em', padding: '2px 6px', borderRadius: '4px',
                      background: '#e8f5e9', color: '#2e7d32',
                    }}>
                      {({
                        auto_done: t('completionBehaviorAutoDone'),
                        auto_review: t('completionBehaviorAutoReview'),
                        sample_review: t('completionBehaviorSampleReview'),
                        needs_review: t('completionBehaviorNeedsReview'),
                      })[currentTask.completion_behavior] || currentTask.completion_behavior}
                    </span>
                  </div>
                )}
              </div>
            )}

            {/* Timestamps (read-only) */}
            <div style={{ borderTop: '1px solid #eee', paddingTop: '12px', marginTop: '4px' }}>
              <div style={{ fontSize: '0.8em', color: '#999', lineHeight: 1.6 }}>
                <div>{t('taskDetailCreated')} {new Date(currentTask.created_at).toLocaleString()}</div>
                <div>{t('taskDetailUpdatedTime')} {new Date(currentTask.updated_at).toLocaleString()}</div>
              </div>
            </div>
          </div>
        </div>

        {/* Footer actions */}
        <div style={{
          display: 'flex', justifyContent: 'space-between', alignItems: 'center',
          padding: '12px 24px', borderTop: '1px solid #eee', background: '#fafafa',
        }}>
          <button onClick={handleDeleteClick}
            style={{
              padding: '6px 16px', borderRadius: '6px', border: '1px solid #ffcdd2',
              background: '#fff', color: '#c62828', cursor: 'pointer', fontSize: '0.85em',
            }}
          >{t('taskDetailDeleteTask')}</button>
          <button onClick={onClose}
            style={{
              padding: '6px 16px', borderRadius: '6px', border: '1px solid #ddd',
              background: '#fff', color: '#666', cursor: 'pointer', fontSize: '0.85em',
            }}
          >{t('taskDetailClose')}</button>
        </div>
      </div>

      {/* Delete verification modal */}
      {showDeleteVerify && (
        <div onClick={() => setShowDeleteVerify(false)}
          style={{
            position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)',
            display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 1100,
          }}
        >
          <div onClick={(e) => e.stopPropagation()}
            style={{
              background: '#fff', borderRadius: '12px', padding: '28px',
              width: '360px', maxWidth: '90vw', boxShadow: '0 8px 32px rgba(0,0,0,0.2)', textAlign: 'center',
            }}
          >
            <h3 style={{ margin: '0 0 8px', color: '#333' }}>{t('taskDetailDeleteTask')}</h3>
            <p style={{ color: '#666', fontSize: '0.9em', marginBottom: '20px' }}>
              {t('taskDetailConfirmDeleteMsg')}
            </p>
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'center' }}>
              <button onClick={() => setShowDeleteVerify(false)}
                style={{
                  padding: '10px 20px', borderRadius: '6px', border: '1px solid #ddd',
                  background: '#fff', cursor: 'pointer', color: '#666', fontSize: '0.95em',
                }}
              >{t('cancel')}</button>
              <button onClick={handleDeleteConfirm}
                style={{
                  padding: '10px 20px', borderRadius: '6px', border: 'none',
                  background: '#c62828', color: '#fff', cursor: 'pointer', fontSize: '0.95em',
                }}
              >{t('taskDelete')}</button>
            </div>
          </div>
        </div>
      )}
    </div>
    </>
  );
}
