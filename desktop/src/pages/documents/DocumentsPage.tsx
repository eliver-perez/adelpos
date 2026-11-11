import { useState } from 'react';
import { Notice, PageTitle } from '../../components/Forms';
import { useResource } from '../../hooks/useResource';
import type { DocumentCatalog } from '../../services/documents';
import { IdentityPanel } from './IdentityPanel';
import { TemplatesTab } from './templates/TemplatesTab';

// Pantalla «Documentos» (etapa 9B): plantillas de los diez tipos de documento con su editor visual
// y la identidad del establecimiento (fase B2). Se ve con documents.templates.read; cada acción
// aparece solo con su permiso y Go vuelve a comprobarlo.

type Tab = 'templates' | 'identity';

export function DocumentsPage({
  permissions,
  timezone,
  onOpenSettings,
}: {
  permissions: string[];
  timezone: string;
  onOpenSettings: () => void;
}) {
  const [tab, setTab] = useState<Tab>('templates');
  const [editing, setEditing] = useState(false);
  const catalog = useResource<DocumentCatalog>('documents_catalog');

  return (
    <div className={`documents-page ${editing ? 'documents-page-editing' : ''}`}>
      {!editing && (
        <>
          <PageTitle
            title="Documentos"
            description="Diseña con bloques tickets, precuentas, comandas, avisos de cocina, comprobantes y cortes, y configura la identidad que se imprime. Lo que ves en la vista previa es exactamente lo que genera el servidor."
          />
          <div className="catalog-tabs" role="tablist" aria-label="Secciones de documentos">
            <button
              type="button"
              role="tab"
              aria-selected={tab === 'templates'}
              className={tab === 'templates' ? 'tab-active' : ''}
              onClick={() => setTab('templates')}
            >
              Plantillas
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={tab === 'identity'}
              className={tab === 'identity' ? 'tab-active' : ''}
              onClick={() => setTab('identity')}
            >
              Identidad del establecimiento
            </button>
          </div>
        </>
      )}
      {tab === 'templates' && (
        <>
          <Notice>{catalog.error}</Notice>
          {catalog.loading && !catalog.data && (
            <p className="muted" role="status">
              Cargando el catálogo de documentos…
            </p>
          )}
          {catalog.data && (
            <TemplatesTab
              catalog={catalog.data}
              permissions={permissions}
              timezone={timezone}
              onEditing={setEditing}
            />
          )}
        </>
      )}
      {tab === 'identity' && (
        <IdentityPanel permissions={permissions} onOpenSettings={onOpenSettings} />
      )}
    </div>
  );
}
