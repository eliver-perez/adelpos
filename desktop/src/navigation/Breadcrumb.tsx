import { Fragment } from 'react';
import { Icon } from '../components/Icon';
import { breadcrumbOf } from './sections';
import type { ScreenID } from './sections';

// Ruta de navegación de Administración (§1.4): «‹ Administración › Sistema › Terminales». Cada
// pantalla anterior es un botón que lleva a ella por el requestScreen de App (con la protección de
// cambios sin guardar); la pantalla actual y, si la informa, su pestaña son el último tramo.
export function Breadcrumb({
  screen,
  tail,
  onNavigate,
}: {
  screen: ScreenID;
  tail: string | null;
  onNavigate: (next: ScreenID) => void;
}) {
  const chain = breadcrumbOf(screen);
  if (chain.length < 2) {
    return null;
  }
  const ancestors = chain.slice(0, -1);
  const current = chain[chain.length - 1]!;
  return (
    <nav className="breadcrumb" aria-label="Ruta de navegación">
      <ol>
        {ancestors.map((entry, index) => (
          <Fragment key={entry.id}>
            <li>
              <button
                type="button"
                className={`breadcrumb-link ${index === 0 ? 'breadcrumb-root' : ''}`}
                onClick={() => onNavigate(entry.id)}
              >
                {index === 0 && <Icon name="chevron-left" size={18} />}
                <span>{entry.label}</span>
              </button>
            </li>
            <li className="breadcrumb-separator" aria-hidden="true">
              ›
            </li>
          </Fragment>
        ))}
        <li>
          <span className="breadcrumb-current" aria-current={tail ? undefined : 'page'}>
            {current.label}
          </span>
        </li>
        {tail && (
          <>
            <li className="breadcrumb-separator" aria-hidden="true">
              ›
            </li>
            <li>
              <span className="breadcrumb-current breadcrumb-tail" aria-current="page">
                {tail}
              </span>
            </li>
          </>
        )}
      </ol>
    </nav>
  );
}
