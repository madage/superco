import { useEffect, useState, useCallback } from 'react';
import { useLang } from '../i18n/context';
import { projects as projectsApi } from '../api/client';
import { useResourceSync } from '../hooks/useResourceSync';
import { ProjectCard } from './ProjectCard';
import { ProjectForm } from './ProjectForm';
import { ProjectDetail } from './ProjectDetail';
import type { Project } from '../types';

export function ProjectList() {
  const { t } = useLang();
  const [projectList, setProjectList] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [editingProject, setEditingProject] = useState<Project | null>(null);
  const [detailProject, setDetailProject] = useState<Project | null>(null);

  const fetchProjects = useCallback(async () => {
    try {
      const res = await projectsApi.list();
      setProjectList(res.projects);
    } catch {
      // silently fail
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchProjects();
  }, [fetchProjects]);

  useResourceSync('projects', fetchProjects);

  const handleCreate = useCallback(async (data: { name: string; description: string; color: string }) => {
    try {
      await projectsApi.create(data);
      setShowCreate(false);
      fetchProjects();
    } catch {
      alert('Failed to create project');
    }
  }, [fetchProjects]);

  const handleUpdate = useCallback(async (id: string, data: { name?: string; description?: string; color?: string }) => {
    try {
      await projectsApi.update(id, data);
      setEditingProject(null);
      setDetailProject(null);
      fetchProjects();
    } catch {
      alert('Failed to update project');
    }
  }, [fetchProjects]);

  const handleDelete = useCallback(async (id: string) => {
    try {
      await projectsApi.delete(id);
      setDetailProject(null);
      fetchProjects();
    } catch {
      alert('Failed to delete project');
    }
  }, [fetchProjects]);

  if (loading) {
    return (
      <div style={{ padding: '24px', color: '#999', textAlign: 'center' }}>
        {t('loading')}...
      </div>
    );
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px', margin: '0 auto' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '24px' }}>
        <h2 style={{ margin: 0 }}>{t('navProjects')}</h2>
        <button
          onClick={() => setShowCreate(true)}
          style={{
            padding: '8px 20px', background: '#1976d2', color: '#fff',
            border: 'none', borderRadius: '8px', cursor: 'pointer',
            fontSize: '0.95em', fontWeight: 600,
          }}
        >
          + {t('projectCreate')}
        </button>
      </div>

      {/* Grid */}
      {projectList.length === 0 ? (
        <div style={{ textAlign: 'center', color: '#999', marginTop: '48px', fontSize: '0.95em' }}>
          {t('projectEmpty')}
        </div>
      ) : (
        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
          gap: '20px',
        }}>
          {projectList.map((project) => (
            <ProjectCard
              key={project.id}
              project={project}
              onClick={() => setDetailProject(project)}
              onEdit={() => setEditingProject(project)}
              onDelete={handleDelete}
            />
          ))}
        </div>
      )}

      {/* Create form */}
      {showCreate && (
        <ProjectForm
          onClose={() => setShowCreate(false)}
          onSave={handleCreate}
        />
      )}

      {/* Edit form */}
      {editingProject && (
        <ProjectForm
          initial={editingProject}
          onClose={() => setEditingProject(null)}
          onSave={(data) => handleUpdate(editingProject.id, data)}
        />
      )}

      {/* Detail modal */}
      {detailProject && (
        <ProjectDetail
          project={detailProject}
          onClose={() => setDetailProject(null)}
          onDelete={handleDelete}
          onUpdate={handleUpdate}
        />
      )}
    </div>
  );
}
