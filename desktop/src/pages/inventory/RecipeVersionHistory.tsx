import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type { Recipe, RecipeLine, RecipeVersionDetail } from '../../services/inventoryRecipes';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { formatQuantity } from './inventoryFormat';

// Nombre del componente de un renglón: el insumo, la preparación o «ingrediente → insumo».
export function lineComponentName(line: RecipeLine) {
  const consumed = line.item?.name ?? line.preparation_name ?? '—';
  if (line.component_kind === 'ingredient') {
    return `${line.ingredient_name ?? 'Ingrediente'} → ${consumed}`;
  }
  return consumed;
}

// Renglones de una versión tal como los guardó Go: cantidad capturada y su conversión a la unidad
// base. Sin costos: el historial no los lleva (los costos teóricos son siempre los de hoy).
export function RecipeLinesTable({ lines }: { lines: RecipeLine[] }) {
  return (
    <div className="table-container cash-table">
      <table className="inventory-table">
        <thead>
          <tr>
            <th scope="col">Renglón</th>
            <th scope="col">Componente</th>
            <th scope="col">Tipo</th>
            <th scope="col" className="inventory-number">
              Cantidad
            </th>
            <th scope="col" className="inventory-number">
              En unidad base
            </th>
            <th scope="col">Consumo</th>
          </tr>
        </thead>
        <tbody>
          {lines.map((line) => (
            <tr key={line.line_no}>
              <td>{line.line_no}</td>
              <td className="inventory-cell-name">
                <strong>{lineComponentName(line)}</strong>
              </td>
              <td>{line.component_kind_label}</td>
              <td className="inventory-number">{formatQuantity(line.quantity, line.unit)}</td>
              <td className="inventory-number">
                {formatQuantity(line.base_quantity, line.base_unit)}
              </td>
              <td>{line.takeaway_only ? 'Solo para llevar' : 'Siempre'}</td>
            </tr>
          ))}
          {lines.length === 0 && (
            <tr>
              <td colSpan={6} className="muted">
                Sin componentes.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

function VersionDetail({
  versionID,
  timezone,
  onClose,
}: {
  versionID: string;
  timezone: string;
  onClose: () => void;
}) {
  const resource = useResource<RecipeVersionDetail>('inventory_recipe_version', {
    id: versionID,
    reloadEvent: inventoryChangeEvent,
  });
  const version = resource.data;
  if (!version) {
    return <Notice>{resource.error || 'Consultando la versión…'}</Notice>;
  }
  const details = [
    `Rendimiento: ${formatQuantity(version.yield_quantity, version.yield_unit)}`,
    `vigente desde el ${serverDateTime(version.effective_from, timezone)}`,
  ];
  if (version.production_waste_percent !== '0') {
    details.push(`merma de elaboración ${version.production_waste_percent} %`);
  }
  if (version.created_by_name) {
    details.push(version.created_by_name);
  }
  return (
    <section
      className="recipe-version-detail form-stack"
      aria-label={`Versión ${version.version} de la receta`}
    >
      <div className="section-heading">
        <div>
          <h4>
            Versión {version.version} · {version.name}
          </h4>
          <p className="muted text-xs">{details.join(' · ')}</p>
        </div>
        <div className="button-row">
          <span className={version.active ? 'tag tag-good' : 'tag'}>
            {version.active ? 'Activa' : 'Inactiva'}
          </span>
          <button type="button" className="secondary-button" onClick={onClose}>
            Cerrar versión
          </button>
        </div>
      </div>
      {version.notes && <p className="text-sm">{version.notes}</p>}
      <RecipeLinesTable lines={version.lines} />
    </section>
  );
}

// Historial de versiones (las 50 más recientes). Cada versión es inmutable: las partidas liberadas
// conservan la versión con que se consumieron y aquí se consultan tal como se guardaron.
export function RecipeVersionHistory({ recipe, timezone }: { recipe: Recipe; timezone: string }) {
  const [selected, setSelected] = useState<string | null>(null);
  return (
    <Accordion
      title="Historial de versiones"
      summary={countLabel(recipe.versions.length, 'versión', 'versiones')}
    >
      <div className="form-stack">
        <ol className="inventory-list" aria-label="Versiones de la receta">
          {recipe.versions.map((entry) => {
            const current = entry.version === recipe.revision;
            const author = entry.created_by_name ? ` · ${entry.created_by_name}` : '';
            return (
              <li key={entry.id} className="inventory-list-row">
                <div className="inventory-list-text">
                  <strong>Versión {entry.version}</strong>
                  <span className="muted text-xs">
                    Desde el {serverDateTime(entry.effective_from, timezone)}
                    {author} · {countLabel(entry.line_count, 'componente', 'componentes')}
                  </span>
                </div>
                <div className="button-row">
                  {current && <span className="tag tag-good">Vigente</span>}
                  {!entry.active && <span className="tag">Inactiva</span>}
                  <button
                    type="button"
                    className="secondary-button"
                    aria-label={`Ver la versión ${entry.version}`}
                    aria-pressed={selected === entry.id}
                    onClick={() => setSelected(selected === entry.id ? null : entry.id)}
                  >
                    Ver versión
                  </button>
                </div>
              </li>
            );
          })}
        </ol>
        {selected && (
          <VersionDetail
            key={selected}
            versionID={selected}
            timezone={timezone}
            onClose={() => setSelected(null)}
          />
        )}
      </div>
    </Accordion>
  );
}
