import { useEffect, useRef, useState } from 'react';
import { Accordion } from './Accordion';
import { Field } from './Forms';
import type { Permission } from '../services/models';
import {
  capabilityState,
  catalogPermissionName,
  matchesSearch,
  toggleCapability,
  withoutPermissions,
  withRequirements,
} from '../services/permissionCatalog';
import type {
  CapabilityState,
  CatalogPermission,
  PermissionCapability,
  PermissionCatalog,
  SelectionChange,
} from '../services/permissionCatalog';

// La lista de siempre: una casilla por permiso. Se usa con un servidor sin el catálogo agrupado
// (anterior a la etapa A3) y en las excepciones por usuario.
export function PermissionChecklist({
  registry,
  selected,
  onChange,
}: {
  registry: Permission[];
  selected: string[];
  onChange: (keys: string[]) => void;
}) {
  return (
    <div className="permission-grid">
      {registry.map((permission) => (
        <label className="permission-option" key={permission.key}>
          <input
            type="checkbox"
            checked={selected.includes(permission.key)}
            onChange={(event) =>
              onChange(
                event.target.checked
                  ? [...selected, permission.key]
                  : selected.filter((key) => key !== permission.key),
              )
            }
          />
          <span>{permission.description}</span>
        </label>
      ))}
    </div>
  );
}

type PermissionView = 'simple' | 'detailed';

interface ChangeMessage {
  tone: 'info' | 'warning';
  text: string;
}

function quotedNames(catalog: PermissionCatalog, keys: string[]) {
  return keys.map((key) => `«${catalogPermissionName(catalog, key)}»`).join(', ');
}

// changeMessage dice lo que la pantalla hizo sola o lo que dejará de funcionar (§3.2).
function changeMessage(catalog: PermissionCatalog, change: SelectionChange): ChangeMessage | null {
  if (change.broken.length > 0) {
    return {
      tone: 'warning',
      text: `Sin lo que quitaste dejarán de funcionar: ${quotedNames(catalog, change.broken)}.`,
    };
  }
  if (change.added.length > 0) {
    return {
      tone: 'info',
      text: `También se marcó lo que necesita: ${quotedNames(catalog, change.added)}.`,
    };
  }
  return null;
}

// Etapa A3 (§3.2): la pantalla de permisos de un rol con la vista sencilla por capacidades como
// pestaña inicial y la detallada al lado, agrupada por área con secciones plegables. Todo lo que
// decide (grupos, dependencias y capacidades) lo informa Go en el catálogo.
export function GroupedPermissionEditor({
  catalog,
  selected,
  onChange,
}: {
  catalog: PermissionCatalog;
  selected: string[];
  onChange: (keys: string[]) => void;
}) {
  const [view, setView] = useState<PermissionView>('simple');
  const [message, setMessage] = useState<ChangeMessage | null>(null);

  function apply(change: SelectionChange) {
    onChange(change.selected);
    setMessage(changeMessage(catalog, change));
  }

  return (
    <div className="permission-editor">
      <div className="catalog-tabs" role="tablist" aria-label="Vista de los permisos">
        <button
          type="button"
          role="tab"
          aria-selected={view === 'simple'}
          className={view === 'simple' ? 'tab-active' : ''}
          onClick={() => setView('simple')}
        >
          Vista sencilla
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={view === 'detailed'}
          className={view === 'detailed' ? 'tab-active' : ''}
          onClick={() => setView('detailed')}
        >
          Vista detallada
        </button>
      </div>
      {message && (
        <p className={`permission-change ${message.tone}`} role="status">
          {message.text}
        </p>
      )}
      {view === 'simple' ? (
        <CapabilityList catalog={catalog} selected={selected} onApply={apply} />
      ) : (
        <DetailedPermissions catalog={catalog} selected={selected} onApply={apply} />
      )}
    </div>
  );
}

// Casilla de una capacidad: marcada, sin marcar o parcial («mixta»), que se ve y se anuncia.
function CapabilityCheckbox({
  state,
  label,
  onToggle,
}: {
  state: CapabilityState;
  label: string;
  onToggle: () => void;
}) {
  const box = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (box.current) {
      box.current.indeterminate = state === 'some';
    }
  }, [state]);
  return (
    <input
      ref={box}
      type="checkbox"
      aria-label={label}
      aria-checked={state === 'some' ? 'mixed' : state === 'all'}
      checked={state === 'all'}
      onChange={onToggle}
    />
  );
}

function CapabilityList({
  catalog,
  selected,
  onApply,
}: {
  catalog: PermissionCatalog;
  selected: string[];
  onApply: (change: SelectionChange) => void;
}) {
  return (
    <div className="permission-grid" aria-label="Lo que puede hacer el rol" role="group">
      {catalog.capabilities.map((capability: PermissionCapability) => {
        const state = capabilityState(capability, selected);
        const held = capability.permissions.filter((key) => selected.includes(key)).length;
        return (
          <label className="permission-option" key={capability.code}>
            <CapabilityCheckbox
              state={state}
              label={capability.name}
              onToggle={() => onApply(toggleCapability(catalog, selected, capability))}
            />
            <span>
              <span className="block">{capability.name}</span>
              <small className="muted block">{capability.description}</small>
              {state === 'some' && (
                <small className="permission-partial block">
                  Parcial: {held} de {capability.permissions.length}
                </small>
              )}
            </span>
          </label>
        );
      })}
    </div>
  );
}

function DetailedPermissions({
  catalog,
  selected,
  onApply,
}: {
  catalog: PermissionCatalog;
  selected: string[];
  onApply: (change: SelectionChange) => void;
}) {
  const [search, setSearch] = useState('');
  const searching = search.trim() !== '';
  const byKey = new Map(catalog.permissions.map((permission) => [permission.key, permission]));
  const sections = catalog.groups
    .map((group) => {
      const permissions = group.permissions
        .map((key) => byKey.get(key))
        .filter((permission): permission is CatalogPermission => permission !== undefined);
      return {
        group,
        visible: permissions.filter((permission) => matchesSearch(permission, search)),
        count: group.permissions.filter((key) => selected.includes(key)).length,
      };
    })
    .filter((section) => section.visible.length > 0);
  return (
    <>
      <Field label="Buscar permiso" hint="Por nombre o por descripción.">
        <input type="search" value={search} onChange={(event) => setSearch(event.target.value)} />
      </Field>
      {sections.length === 0 && <p className="muted">Ningún permiso coincide con la búsqueda.</p>}
      {sections.map(({ group, visible, count }) => (
        // Con una búsqueda, las secciones con coincidencias se abren solas.
        <Accordion
          key={`${group.code}-${searching ? 'busqueda' : 'todo'}`}
          title={group.name}
          summary={`${count} de ${group.permissions.length}`}
          defaultOpen={searching}
        >
          <div className="button-row mb-4">
            <button
              type="button"
              className="secondary-button"
              onClick={() => onApply(withRequirements(catalog, selected, group.permissions))}
            >
              Marcar todo el grupo
            </button>
            <button
              type="button"
              className="secondary-button"
              onClick={() => onApply(withoutPermissions(catalog, selected, group.permissions))}
            >
              Quitar todo el grupo
            </button>
          </div>
          <div className="permission-grid">
            {visible.map((permission) => (
              <label className="permission-option" key={permission.key}>
                <input
                  type="checkbox"
                  aria-label={permission.name}
                  aria-describedby={`permiso-${permission.key}`}
                  checked={selected.includes(permission.key)}
                  onChange={(event) =>
                    onApply(
                      event.target.checked
                        ? withRequirements(catalog, selected, [permission.key])
                        : withoutPermissions(catalog, selected, [permission.key]),
                    )
                  }
                />
                <span>
                  <span className="block">{permission.name}</span>
                  <small className="muted block" id={`permiso-${permission.key}`}>
                    {permission.description}
                  </small>
                </span>
              </label>
            ))}
          </div>
        </Accordion>
      ))}
    </>
  );
}
