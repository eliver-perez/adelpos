import { useCallback, useEffect, useState } from 'react';
import { invoke, isTauri } from '@tauri-apps/api/core';

export function WindowControls() {
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
  return (
    <div className="window-controls">
      <button
        type="button"
        className="secondary-button"
        onClick={() => void toggle()}
        title="Pantalla completa nativa de macOS · Control + ⌘ + F"
      >
        {fullscreen ? 'Salir de pantalla completa' : 'Pantalla completa'}
      </button>
      {error && <span role="status">{error}</span>}
    </div>
  );
}
