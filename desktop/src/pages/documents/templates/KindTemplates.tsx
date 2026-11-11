import { useEffect, useRef } from 'react';
import { Notice } from '../../../components/Forms';
import { IconLabel } from '../../../components/Icon';
import { useResource } from '../../../hooks/useResource';
import { documentPermissions } from '../../../services/documents';
import type { CatalogKind, TemplateSummary } from '../../../services/documents';
import { serverDateTime } from '../../../services/restaurant';

// Plantillas de un tipo de documento: la activa primero (así las entrega Go), la predeterminada
// del sistema marcada como solo lectura y, con su permiso, las acciones de cada una. La lista se
// vuelve a consultar con cada cambio del servidor.
export function KindTemplates({
  kind,
  permissions,
  timezone,
  reloadKey,
  onOpen,
  onDuplicate,
  onRename,
  onActivate,
  onHistory,
  onCreate,
  onRestoreDefault,
}: {
  kind: CatalogKind;
  permissions: string[];
  timezone: string;
  // Cambia cuando una acción propia modificó las plantillas: vuelve a consultar al instante.
  reloadKey: number;
  onOpen: (template: TemplateSummary) => void;
  onDuplicate: (template: TemplateSummary) => void;
  onRename: (template: TemplateSummary) => void;
  onActivate: (template: TemplateSummary) => void;
  onHistory: (template: TemplateSummary) => void;
  onCreate: () => void;
  onRestoreDefault: () => void;
}) {
  const manage = permissions.includes(documentPermissions.manage);
  const templates = useResource<TemplateSummary[]>('documents_templates', {
    query: { kind: kind.kind },
  });
  const reload = templates.reload;
  const firstKey = useRef(reloadKey);
  useEffect(() => {
    if (reloadKey !== firstKey.current) {
      void reload();
    }
  }, [reloadKey, reload]);
  const list = templates.data ?? [];
  const active = list.find((template) => template.active);

  return (
    <section className="panel documents-kind" aria-label={`Plantillas de ${kind.name}`}>
      <div className="section-heading">
        <div>
          <h2>{kind.name}</h2>
          <p className="muted text-sm">
            {active
              ? `Plantilla activa: «${active.name}», versión ${active.current_version.number}.`
              : 'Consultando la plantilla activa…'}
          </p>
        </div>
        {manage && (
          <div className="button-row">
            <button type="button" className="secondary-button" onClick={onCreate}>
              <IconLabel icon="plus">Nueva plantilla</IconLabel>
            </button>
            {active && !active.system && (
              <button type="button" className="secondary-button" onClick={onRestoreDefault}>
                Restaurar predeterminada
              </button>
            )}
          </div>
        )}
      </div>
      <Notice>{templates.error}</Notice>
      {templates.loading && !templates.data && (
        <p className="muted" role="status">
          Cargando plantillas…
        </p>
      )}
      <ul className="documents-template-list">
        {list.map((template) => (
          <li
            key={template.id}
            className={`documents-template ${template.active ? 'documents-template-active' : ''}`}
          >
            <div className="documents-template-body">
              <div className="documents-template-title">
                <strong>{template.name}</strong>
                {template.active && (
                  <span className="designer-badge designer-badge-on">Activa</span>
                )}
                {template.system && (
                  <span className="designer-badge">Predeterminada · solo lectura</span>
                )}
              </div>
              <p className="muted text-xs">
                Versión {template.current_version.number} ·{' '}
                {serverDateTime(template.current_version.created_at, timezone)} ·{' '}
                {template.current_version.created_by_name ?? 'Sistema'}
                {template.current_version.note ? ` · ${template.current_version.note}` : ''}
              </p>
            </div>
            <div className="button-row documents-template-actions">
              <button
                type="button"
                className="primary-button"
                aria-label={`${template.system || !manage ? 'Ver' : 'Editar'} ${template.name}`}
                onClick={() => onOpen(template)}
              >
                {template.system || !manage ? 'Ver' : 'Editar'}
              </button>
              {manage && (
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Duplicar ${template.name}`}
                  onClick={() => onDuplicate(template)}
                >
                  Duplicar
                </button>
              )}
              {manage && !template.system && (
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Renombrar ${template.name}`}
                  onClick={() => onRename(template)}
                >
                  Renombrar
                </button>
              )}
              {manage && !template.active && (
                <button
                  type="button"
                  className="secondary-button"
                  aria-label={`Activar ${template.name}`}
                  onClick={() => onActivate(template)}
                >
                  Activar
                </button>
              )}
              <button
                type="button"
                className="secondary-button"
                aria-label={`Historial de ${template.name}`}
                onClick={() => onHistory(template)}
              >
                Historial
              </button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
