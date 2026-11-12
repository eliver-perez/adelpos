import { useCallback, useEffect, useState } from 'react';
import { invoke, isTauri } from '@tauri-apps/api/core';
import { ariaShortcut, fullscreenShortcut, shortcutLabel } from '../commands/platform';
import { Icon } from './Icon';

// Aviso entre los controles de pantalla completa: el botón y la acción «Pantalla completa» del menú
// nativo, la paleta y el atajo (etapa 9C, fase C) cambian el modo con la misma función.
const fullscreenChangeEvent = 'adel-fullscreen-change';

// Cambia el modo de la ventana con la operación nativa de siempre (sin modo quiosco).
export async function setWindowFullscreen(fullscreen: boolean) {
  await invoke('window_set_fullscreen', { fullscreen });
  window.dispatchEvent(new CustomEvent(fullscreenChangeEvent, { detail: fullscreen }));
}

// Alterna el modo real de la ventana (lo consulta primero: el usuario pudo salir con el sistema).
export async function toggleWindowFullscreen() {
  if (!isTauri()) {
    return;
  }
  const fullscreen = await invoke<boolean>('window_fullscreen_state');
  await setWindowFullscreen(!fullscreen);
}

// Pantalla completa nativa (etapa 9C, §1.1): un solo botón de icono que cambia entre maximizar
// (entrar) y reducir (salir), con su descripción accesible y su atajo (⌃⌘F en macOS, F11 en
// Windows). Conserva el comportamiento de las etapas anteriores: consulta el modo real de la
// ventana al enfocarla o redimensionarla y alterna entre pantalla completa y ventana normal (sin
// modo quiosco). En la sesión vive en la barra superior; en el acceso y el asistente, fijo arriba a
// la derecha.
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
    const changed = (event: Event) => {
      setFullscreen(!!(event as CustomEvent<boolean>).detail);
    };
    window.addEventListener('resize', refresh);
    window.addEventListener('focus', refresh);
    window.addEventListener(fullscreenChangeEvent, changed);
    return () => {
      window.removeEventListener('resize', refresh);
      window.removeEventListener('focus', refresh);
      window.removeEventListener(fullscreenChangeEvent, changed);
    };
  }, [refresh]);

  async function toggle() {
    setError('');
    try {
      await setWindowFullscreen(!fullscreen);
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
        aria-keyshortcuts={ariaShortcut(fullscreenShortcut)}
        title={`${label} · ${shortcutLabel(fullscreenShortcut)}`}
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
