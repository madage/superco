import { useState, useEffect } from 'react';
import { useLang } from '../i18n/context';

const presetColors = [
  '#1976d2', '#388e3c', '#f57c00', '#c62828',
  '#7b1fa2', '#00838f', '#e91e63', '#546e7a',
];

interface ProjectFormProps {
  initial?: { name: string; description: string; color: string };
  onClose: () => void;
  onSave: (data: { name: string; description: string; color: string }) => void;
}

export function ProjectForm({ initial, onClose, onSave }: ProjectFormProps) {
  const { t, lang } = useLang();
  const [name, setName] = useState(initial?.name || '');
  const [description, setDescription] = useState(initial?.description || '');
  const [color, setColor] = useState(initial?.color || presetColors[0]);

  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleEsc);
    return () => window.removeEventListener('keydown', handleEsc);
  }, [onClose]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    onSave({ name: name.trim(), description: description.trim(), color });
  };

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)',
        display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 1000,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: '#fff', borderRadius: '16px', padding: '32px',
          width: '420px', maxWidth: '90vw',
          boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
        }}
      >
        <h3 style={{ margin: '0 0 24px', color: '#333' }}>
          {initial ? t('profileEdit') : t('projectCreate')}
        </h3>

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: '16px' }}>
            <label style={{ display: 'block', fontSize: '0.85em', color: '#666', marginBottom: '4px' }}>
              {t('projectName')} *
            </label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              style={{
                width: '100%', padding: '10px', borderRadius: '6px',
                border: '1px solid #ddd', fontSize: '0.95em', boxSizing: 'border-box',
              }}
              required
              autoFocus
            />
          </div>

          <div style={{ marginBottom: '16px' }}>
            <label style={{ display: 'block', fontSize: '0.85em', color: '#666', marginBottom: '4px' }}>
              {t('projectDescription')}
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              style={{
                width: '100%', padding: '10px', borderRadius: '6px',
                border: '1px solid #ddd', fontSize: '0.95em', resize: 'vertical', boxSizing: 'border-box',
              }}
            />
          </div>

          <div style={{ marginBottom: '24px' }}>
            <label style={{ display: 'block', fontSize: '0.85em', color: '#666', marginBottom: '8px' }}>
              {t('projectColor')}
            </label>
            <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
              {presetColors.map((c) => (
                <div
                  key={c}
                  onClick={() => setColor(c)}
                  style={{
                    width: '36px', height: '36px', borderRadius: '50%',
                    background: c, cursor: 'pointer',
                    border: color === c ? '3px solid #333' : '3px solid transparent',
                    transition: 'border 0.15s',
                  }}
                />
              ))}
            </div>
          </div>

          <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
            <button type="button" onClick={onClose} style={{
              padding: '10px 20px', borderRadius: '6px', border: '1px solid #ddd',
              background: '#fff', cursor: 'pointer', color: '#666', fontSize: '0.95em',
            }}>
              {t('cancel')}
            </button>
            <button type="submit" style={{
              padding: '10px 24px', borderRadius: '6px', border: 'none',
              background: color, color: '#fff', cursor: 'pointer', fontSize: '0.95em', fontWeight: 600,
            }}>
              {t('saveAgent')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
