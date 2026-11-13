import type { MouseEvent, ReactNode } from 'react';
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
  className = '',
}: {
  entry: SectionEntry;
  active: boolean;
  onNavigate: (next: ScreenID) => void;
  className?: string;
}) {
  const action = sectionActions[entry.id];
  return (
    <button
      type="button"
      className={`topbar-button topbar-section ${className} ${active ? 'topbar-section-active' : ''}`}
      aria-current={active ? 'page' : undefined}
      {...(action ? shortcutProps(action, entry.label) : {})}
      onClick={() => onNavigate(entry.id)}
    >
      <Icon name={entry.icon} size={20} />
      <span>{entry.label}</span>
    </button>
  );
}

// Barra superior fija de una sola fila (etapa 9C, §1.1; orden de la etapa 9D, §2). A la izquierda,
// el logotipo (vuelve al inicio de la terminal) y las secciones Mesas, Para llevar, Cocina y
// Administración, seguidas y del mismo ancho. A la derecha, Caja en dos renglones («Caja» y el
// estado del turno, con el menú rápido de la 7.1), el estado del servidor, el menú del usuario y
// pantalla completa. Cada usuario ve solo lo que su rol permite; sin opciones de administración no
// hay botón Administración. Toda navegación pasa por onNavigate (el requestScreen de App). Los
// atajos no cambian: ⌘/Ctrl + 1–4 siguen siendo Mesas, Para llevar, Cocina y Caja. Debajo de
// 1280 px la barra se compacta solo con estilos (etapa 9F, §6.1): isotipo de la marca, estado corto
// del turno («Abierto», «En arqueo», «Cerrado»), «En línea» con su texto y el nombre del usuario
// acortado con puntos suspensivos; debajo de 1150 px, el turno y «En línea» como punto de color con
// su ayuda emergente y el usuario como icono.
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
  const sectionsLeft = operation.filter((entry) => entry.id !== 'cash');
  const cash = operation.find((entry) => entry.id === 'cash');
  const administration = canOpen('administration', permissions);

  // Etapa 9F (§6.1): el logotipo solo muestra el contorno de foco al navegar con el teclado. En
  // Chromium (WebView2 en Windows) el botón conservaba el foco después de tocarlo o de hacer clic, y
  // la siguiente tecla (Escape, una flecha o un atajo como ⌘2) le encendía :focus-visible aunque ya
  // se hubiera vuelto al inicio; lo mismo al cerrar la paleta, que devuelve el foco al control
  // anterior. Con el dedo o el ratón (detail > 0) el foco se suelta al volver al inicio; con Intro o
  // Espacio (detail = 0) se queda y el contorno se ve como siempre.
  function goHome(event: MouseEvent<HTMLButtonElement>) {
    if (event.detail > 0) {
      event.currentTarget.blur();
    }
    onNavigate(home);
  }

  return (
    <header className="app-topbar">
      <button
        type="button"
        className="topbar-logo"
        aria-label={`AD-EL POS · Ir al inicio (${sectionByID(home).label})`}
        onClick={goHome}
      >
        <BrandLogo />
      </button>
      <nav className="topbar-sections" aria-label="Navegación principal">
        {sectionsLeft.map((entry) => (
          <SectionButton
            key={entry.id}
            entry={entry}
            active={highlight === entry.id}
            onNavigate={onNavigate}
          />
        ))}
        {administration && (
          <SectionButton
            entry={sectionByID('administration')}
            active={highlight === 'administration'}
            onNavigate={onNavigate}
          />
        )}
      </nav>
      <div className="topbar-end">
        {/* Caja conserva el menú rápido de la 7.1 cuando el usuario puede ver el turno; si solo ve
            la pantalla de Caja, el botón lleva directamente a ella. */}
        {cash &&
          (cashShortcut.visible ? (
            <CashShortcut
              shortcut={cashShortcut}
              can={can}
              placement="bar"
              active={highlight === 'cash'}
            />
          ) : (
            <SectionButton
              entry={cash}
              active={highlight === 'cash'}
              onNavigate={onNavigate}
              className="topbar-cash"
            />
          ))}
        <ServerStatus state={server} />
        {userMenu}
        <WindowControls placement="bar" />
      </div>
    </header>
  );
}
