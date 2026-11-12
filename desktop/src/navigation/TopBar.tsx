import type { ReactNode } from 'react';
import { BrandLogo } from '../components/BrandLogo';
import { Icon } from '../components/Icon';
import { WindowControls } from '../components/WindowControls';
import { shortcutProps } from '../commands/hints';
import type { ActionID } from '../commands/actions';
import { CashShortcut } from '../pages/cash/CashShortcut';
import type { CashShortcutState } from '../pages/cash/CashShortcut';
import { ServerStatus } from './ServerStatus';
import type { ServerState } from './serverState';
import { barSections, canOpen, sectionByID } from './sections';
import type { ScreenID, SectionEntry } from './sections';

// Acción del registro que hace lo mismo que cada botón de la barra (su atajo va en la descripción).
const sectionActions: Partial<Record<ScreenID, ActionID>> = {
  restaurant: 'view.tables',
  takeaway: 'view.takeaway',
  kitchen: 'view.kitchen',
  cash: 'view.cash',
  administration: 'view.administration',
};

function SectionButton({
  entry,
  active,
  onNavigate,
}: {
  entry: SectionEntry;
  active: boolean;
  onNavigate: (next: ScreenID) => void;
}) {
  const action = sectionActions[entry.id];
  return (
    <button
      type="button"
      className={`topbar-button topbar-section ${active ? 'topbar-section-active' : ''}`}
      aria-current={active ? 'page' : undefined}
      {...(action ? shortcutProps(action, entry.label) : {})}
      onClick={() => onNavigate(entry.id)}
    >
      <Icon name={entry.icon} size={20} />
      <span>{entry.label}</span>
    </button>
  );
}

// Barra superior fija de una sola fila (etapa 9C, §1.1), de izquierda a derecha: logotipo (vuelve
// al inicio de la terminal), Mesas, Para llevar, Cocina y Caja (con el estado del turno y su menú
// rápido de la 7.1), Administración, estado del servidor, menú del usuario y pantalla completa.
// Cada usuario ve solo lo que su rol permite; sin opciones de administración no hay botón
// Administración. Toda navegación pasa por onNavigate (el requestScreen de App).
export function TopBar({
  permissions,
  highlight,
  home,
  onNavigate,
  cashShortcut,
  can,
  server,
  userMenu,
}: {
  permissions: string[];
  // Sección resaltada (la pantalla visible o, en la operación, la vista de mesas o para llevar).
  highlight: ScreenID | null;
  home: ScreenID;
  onNavigate: (next: ScreenID) => void;
  cashShortcut: CashShortcutState;
  can: (permission: string) => boolean;
  server: ServerState;
  userMenu: ReactNode;
}) {
  const operation = barSections(permissions);
  const administration = canOpen('administration', permissions);
  return (
    <header className="app-topbar">
      <button
        type="button"
        className="topbar-logo"
        aria-label={`AD-EL POS · Ir al inicio (${sectionByID(home).label})`}
        onClick={() => onNavigate(home)}
      >
        <BrandLogo />
      </button>
      <nav className="topbar-sections" aria-label="Navegación principal">
        {operation.map((entry) => {
          // Caja conserva el menú rápido de la 7.1 cuando el usuario puede ver el turno; si solo ve
          // la pantalla de Caja, el botón lleva directamente a ella.
          if (entry.id === 'cash' && cashShortcut.visible) {
            return (
              <CashShortcut
                key={entry.id}
                shortcut={cashShortcut}
                can={can}
                placement="bar"
                active={highlight === 'cash'}
              />
            );
          }
          return (
            <SectionButton
              key={entry.id}
              entry={entry}
              active={highlight === entry.id}
              onNavigate={onNavigate}
            />
          );
        })}
        {administration && (
          <SectionButton
            entry={sectionByID('administration')}
            active={highlight === 'administration'}
            onNavigate={onNavigate}
          />
        )}
      </nav>
      <div className="topbar-end">
        <ServerStatus state={server} />
        {userMenu}
        <WindowControls placement="bar" />
      </div>
    </header>
  );
}
