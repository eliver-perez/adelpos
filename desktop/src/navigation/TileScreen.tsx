import { useId } from 'react';
import { Icon } from '../components/Icon';
import { PageTitle } from '../components/Forms';
import { sectionByID, tilesOf } from './sections';
import type { ScreenID, SectionEntry } from './sections';

const descriptions: Partial<Record<ScreenID, string>> = {
  administration: 'Elige un área para configurar o consultar el establecimiento.',
  staff: 'Usuarios, roles y sesiones del personal.',
  system: 'Configuración general, terminales, cajas físicas y registro de movimientos.',
};

function Tile({ entry, onOpen }: { entry: SectionEntry; onOpen: () => void }) {
  const id = useId();
  return (
    <li>
      <button
        type="button"
        className="admin-tile"
        aria-labelledby={`${id}-name`}
        aria-describedby={`${id}-description`}
        onClick={onOpen}
      >
        <span className="admin-tile-icon" aria-hidden="true">
          <Icon name={entry.icon} size={30} />
        </span>
        <span className="admin-tile-text">
          <strong id={`${id}-name`}>{entry.label}</strong>
          <span id={`${id}-description`}>{entry.description}</span>
        </span>
        <Icon name="chevron-right" size={20} className="admin-tile-chevron" />
      </button>
    </li>
  );
}

// Pantalla de mosaicos grandes (§1.3): Administración y, como segunda pantalla de mosaicos, Personal
// y Sistema. Solo aparecen los mosaicos que los permisos del usuario permiten.
export function TileScreen({
  parent,
  permissions,
  onNavigate,
}: {
  parent: ScreenID;
  permissions: string[];
  onNavigate: (next: ScreenID) => void;
}) {
  const entry = sectionByID(parent);
  const tiles = tilesOf(parent, permissions);
  return (
    <section className="tile-screen" aria-label={entry.label}>
      <PageTitle title={entry.label} description={descriptions[parent] ?? ''} />
      {tiles.length === 0 ? (
        <p className="muted">No tienes permisos para ninguna de estas pantallas.</p>
      ) : (
        <ul className="admin-tiles">
          {tiles.map((tile) => (
            <Tile key={tile.id} entry={tile} onOpen={() => onNavigate(tile.id)} />
          ))}
        </ul>
      )}
    </section>
  );
}
