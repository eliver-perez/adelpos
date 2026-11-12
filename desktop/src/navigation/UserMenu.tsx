import { useCallback, useId, useRef, useState } from 'react';
import { Icon, IconLabel } from '../components/Icon';
import { useDismiss } from './useDismiss';

// Menú del usuario de la barra superior (§1.1): nombre y rol, con Mis preferencias (sale de
// Administración porque es de cada usuario), Cambiar contraseña y Cerrar sesión. En la barra
// reducida de la captura se muestra compacto (solo el nombre).
export function UserMenu({
  name,
  role,
  compact = false,
  onPreferences,
  onPassword,
  onLogout,
}: {
  name: string;
  role: string;
  compact?: boolean;
  onPreferences: () => void;
  onPassword: () => void;
  onLogout: () => void;
}) {
  const [open, setOpen] = useState(false);
  const menuID = useId();
  const root = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(open, close, root);

  function choose(action: () => void) {
    setOpen(false);
    action();
  }

  return (
    <div className={`user-menu ${compact ? 'user-menu-compact' : ''}`} ref={root}>
      <button
        type="button"
        className="topbar-button user-menu-button"
        aria-label={`Menú del usuario · ${name} · ${role}`}
        aria-expanded={open}
        aria-controls={menuID}
        onClick={() => setOpen(!open)}
      >
        <Icon name="user" size={20} />
        <span className="user-menu-identity" aria-hidden="true">
          <strong>{name}</strong>
          {!compact && <small>{role}</small>}
        </span>
        <Icon name="chevron-down" size={16} className="user-menu-chevron" />
      </button>
      {open && (
        <div id={menuID} className="topbar-menu" role="group" aria-label="Menú del usuario">
          <p className="topbar-menu-heading">
            <strong>{name}</strong>
            <span className="muted">{role}</span>
          </p>
          <button type="button" className="secondary-button" onClick={() => choose(onPreferences)}>
            <IconLabel icon="user">Mis preferencias</IconLabel>
          </button>
          <button type="button" className="secondary-button" onClick={() => choose(onPassword)}>
            <IconLabel icon="key">Cambiar contraseña</IconLabel>
          </button>
          <button type="button" className="secondary-button" onClick={() => choose(onLogout)}>
            <IconLabel icon="logout">Cerrar sesión</IconLabel>
          </button>
        </div>
      )}
    </div>
  );
}
