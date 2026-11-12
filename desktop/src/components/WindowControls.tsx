import { useCallback, useEffect, useState } from 'react';
import { invoke, isTauri } from '@tauri-apps/api/core';
import { Icon } from './Icon';

// Pantalla completa nativa (etapa 9C, §1.1): un solo botón de icono que cambia entre maximizar
// (entrar) y reducir (salir), con su descripción accesible. Conserva el comportamiento de las
// etapas anteriores: consulta el modo real de la ventana al enfocarla o redimensionarla y alterna
// entre pantalla completa y ventana normal (sin modo quiosco). En la sesión vive en la barra
// superior; en el acceso y el asistente, fijo arriba a la derecha.
export function WindowControls({ placement = 'floating' }: { placement?: 'floating' | 'bar' }) {
  const [fullscreen, setFullscreen] = useState(true);
  const [error, setError] = useState('');
  const refresh = useCallback(() => {
    if (isTauri()) {
      void invoke<boolean>('window_fullscreen_state')
        .then(setFullscreen)
        .catch(() => undefined);
    }
  }, []);

  useEffect(() => {
    refresh();
    window.addEventListener('resize', refresh);
    window.addEventListener('focus', refresh);
    return () => {
      window.removeEventListener('resize', refresh);
      window.removeEventListener('focus', refresh);
    };
  }, [refresh]);

  async function toggle() {
    setError('');
    try {
      await invoke('window_set_fullscreen', { fullscreen: !fullscreen });
      setFullscreen(!fullscreen);
    } catch {
      setError('No se pudo cambiar el modo de ventana.');
    }
  }

  if (!isTauri()) return null;
  const label = fullscreen ? 'Salir de pantalla completa' : 'Pantalla completa';
  return (
    <div className={placement === 'bar' ? 'window-controls-bar' : 'window-controls'}>
      <button
        type="button"
        className={placement === 'bar' ? 'topbar-button topbar-icon-button' : 'secondary-button'}
        aria-label={label}
        title={`${label} · Control + ⌘ + F`}
        data-fullscreen={fullscreen}
        onClick={() => void toggle()}
      >
        <Icon name={fullscreen ? 'minimize' : 'maximize'} size={22} />
      </button>
      {error && (
        <span className="window-controls-error" role="status">
          {error}
        </span>
      )}
    </div>
  );
}
