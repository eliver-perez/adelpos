import { useState } from 'react';
import { Field, Notice, PageTitle } from '../../components/Forms';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { notifyError, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { CatalogRecord } from './CatalogRecord';
import { useResource } from '../../hooks/useResource';
import { api } from '../../services/api';
import type {
  CatalogDocument,
  CatalogPage as Page,
  Category,
  Lookups,
  Product,
} from '../../services/catalog';
import { CatalogEditor } from './CatalogEditor';
import { CategoryQuickNotes } from './CategoryQuickNotes';
import { PreviewPage } from './PreviewPage';
import { catalogOperation, catalogTabs, newCatalogDocument } from './catalogState';
import { catalogRequestBody } from '../../services/catalogPositions';

// Recursos que suelen capturarse muchos seguidos y ofrecen «Guardar y crear otra».
const continuousTabs: (typeof catalogTabs)[number]['id'][] = [
  'categories',
  'products',
  'ingredients',
  'complements',
];

export function CatalogPage({ can }: { can: (permission: string) => boolean }) {
  const [tab, setTab] = useState<(typeof catalogTabs)[number]['id']>('products');
  const [search, setSearch] = useState('');
  const [active, setActive] = useState('');
  const [category, setCategory] = useState('');
  const [archived, setArchived] = useState('false');
  const [page, setPage] = useState(1);
  const [draft, setDraft] = useState<CatalogDocument | null>(null);
  const [savedDraft, setSavedDraft] = useState('');
  // «Vista previa» (antes «Probar personalización» en el menú principal) vive dentro del catálogo.
  const [view, setView] = useState<'records' | 'preview'>('records');
  // Cada «Guardar y crear otra» abre un editor nuevo: su estado interno (por ejemplo, si el código
  // ya se editó a mano) no pasa al registro siguiente.
  const [editorSession, setEditorSession] = useState(0);
  const [focusName, setFocusName] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const resource = tab === 'combos' ? 'products' : tab;
  const lookup = useResource<Lookups>('catalog_lookups');
  const query: Record<string, string> = { search, active, page: String(page), page_size: '20' };
  if (resource === 'products') {
    query.kind = tab === 'combos' ? 'combo' : 'product';
    query.archived = archived;
    if (category) query.category_id = category;
  }
  const list = useResource<Page>(catalogOperation(resource), { query });
  const commercial = ['products', 'groups', 'price_lists', 'promotions'].includes(resource);
  const editable =
    can(catalogTabs.find((item) => item.id === tab)?.permission ?? '') &&
    (tab !== 'combos' || can('catalog.products.manage')) &&
    (!commercial || (can('catalog.prices.read') && can('catalog.prices.manage')));
  // La vista previa calcula precios: exige administrar productos y consultar precios.
  const previewAllowed = can('catalog.products.manage') && can('catalog.prices.read');

  // Cierra el editor actual. Con cambios sin guardar pregunta dentro de la aplicación: el WebView
  // nativo no muestra window.confirm y lo resolvía como «no», por lo que «Volver al listado»
  // parecía no responder.
  async function closeEditor(): Promise<boolean> {
    if (draft && JSON.stringify(draft) !== savedDraft) {
      const discard = await confirm({
        title: 'Cambios sin guardar',
        message: 'Este registro tiene cambios que aún no se guardan. ¿Quieres descartarlos?',
        confirmLabel: 'Descartar cambios',
        cancelLabel: 'Seguir editando',
        tone: 'danger',
      });
      if (!discard) {
        return false;
      }
    }
    setDraft(null);
    setFocusName(false);
    return true;
  }

  // Guarda el borrador con la posición elegida (Go recalcula el orden visual) y recarga el
  // listado y los lookups para que el siguiente registro vea el estado actualizado.
  async function saveDraft(current: CatalogDocument) {
    if (!lookup.data) {
      throw new Error('Espera a que se carguen las referencias del catálogo.');
    }
    return api<CatalogDocument>(catalogOperation(resource, current.id ? 'save_' : 'create_'), {
      id: current.id || undefined,
      body: catalogRequestBody(resource, current, lookup.data),
    });
  }

  async function openEditor(id: string) {
    if (!(await closeEditor())) return;
    setFocusName(false);
    try {
      const record = await api<CatalogDocument>(catalogOperation(resource, 'get_'), { id });
      setDraft(record);
      setSavedDraft(JSON.stringify(record));
    } catch (problem) {
      notifyError(problem, 'No se pudo abrir el registro.');
    }
  }

  // Acciones rápidas de un registro del listado (estado, disponibilidad, duplicar, orden). El
  // resultado se confirma con un aviso y el listado se recarga; devuelve si Go la aceptó.
  async function runAction(action: () => Promise<unknown>, success?: string) {
    try {
      await action();
    } catch (problem) {
      notifyError(problem, 'No se pudo completar la acción.');
      return false;
    }
    if (success) {
      notifySuccess(success);
    }
    await list.reload();
    await lookup.reload();
    return true;
  }

  async function moveCategoryUp(index: number) {
    await runAction(async () => {
      const items = [...(list.data?.items ?? [])];
      const previous = items[index - 1];
      const current = items[index];
      if (!previous || !current) return;
      items[index - 1] = current;
      items[index] = previous;
      await api('catalog_reorder_categories', {
        body: {
          items: items.map((item, position) => ({
            id: item.id,
            revision: item.revision,
            sort_order: (page - 1) * 200 + position * 10,
          })),
        },
      });
    }, 'Orden de las categorías actualizado.');
  }

  return (
    <>
      <PageTitle
        title="Catálogo comercial"
        description="Productos, preparación y reglas de precio persistidos en el servidor."
      >
        {editable && !draft && view === 'records' && (
          <button
            className="primary-button"
            disabled={!lookup.data}
            onClick={async () => {
              if (!(await closeEditor())) return;
              // La categoría solo se propone si el listado está filtrado por una; si no, el
              // selector inicia vacío y exige una elección explícita.
              const record = newCatalogDocument(resource, tab === 'combos', category);
              setDraft(record);
              setSavedDraft(JSON.stringify(record));
              setFocusName(true);
            }}
          >
            Crear {tab === 'combos' ? 'combo' : 'registro'}
          </button>
        )}
      </PageTitle>
      <div className="catalog-tabs" role="tablist" aria-label="Secciones del catálogo">
        {catalogTabs.map((item) => (
          <button
            key={item.id}
            role="tab"
            aria-selected={view === 'records' && tab === item.id}
            className={view === 'records' && tab === item.id ? 'tab-active' : ''}
            onClick={async () => {
              if (!(await closeEditor())) return;
              setView('records');
              setTab(item.id);
              setPage(1);
              setSearch('');
            }}
          >
            {item.label}
          </button>
        ))}
        {previewAllowed && (
          <button
            role="tab"
            aria-selected={view === 'preview'}
            className={view === 'preview' ? 'tab-active' : ''}
            onClick={async () => {
              if (!(await closeEditor())) return;
              setView('preview');
            }}
          >
            Vista previa
          </button>
        )}
      </div>
      <Notice>{lookup.error || list.error}</Notice>
      {view === 'preview' && previewAllowed ? (
        <PreviewPage can={can} embedded />
      ) : draft && lookup.data ? (
        <section className="panel catalog-editor">
          <div className="section-heading">
            <div>
              <h2>{draft.id ? draft.name : 'Nuevo registro'}</h2>
              <p className="muted text-xs">
                {draft.id ? `Revisión ${draft.revision}` : 'Se guardará al confirmar'}
              </p>
            </div>
            <button type="button" className="secondary-button" onClick={() => void closeEditor()}>
              Volver al listado
            </button>
          </div>
          {!editable && (
            <Notice>
              Consulta disponible. La edición de esta configuración requiere sus permisos de
              administración y precios.
            </Notice>
          )}
          <CatalogEditor
            key={`${resource}-${draft.id}-${editorSession}`}
            resource={resource}
            value={draft}
            onChange={setDraft}
            lookups={lookup.data}
            can={can}
            editable={editable}
            focusName={focusName}
            onSave={async () => {
              const saved = await saveDraft(draft);
              setFocusName(false);
              setDraft(saved);
              setSavedDraft(JSON.stringify(saved));
              await list.reload();
              await lookup.reload();
            }}
            saveAndNewLabel={
              resource === 'categories' ? 'Guardar y crear otra' : 'Guardar y crear otro'
            }
            onSaveAndNew={
              continuousTabs.includes(tab)
                ? async () => {
                    const saved = await saveDraft(draft);
                    // En productos se conserva la categoría elegida para el siguiente registro.
                    const next = newCatalogDocument(
                      resource,
                      false,
                      resource === 'products' ? (saved as Product).category_id : category,
                    );
                    setDraft(next);
                    setSavedDraft(JSON.stringify(next));
                    setEditorSession((current) => current + 1);
                    setFocusName(true);
                    await list.reload();
                    await lookup.reload();
                    // El formulario muestra este texto como aviso de confirmación.
                    return `«${saved.name}» quedó guardado. Captura el siguiente registro.`;
                  }
                : undefined
            }
          />
          {resource === 'categories' && draft.id && (
            <CategoryQuickNotes
              category={draft as Category}
              notes={lookup.data.category_quick_notes ?? []}
              ingredients={lookup.data.ingredients}
              editable={can('catalog.categories.manage')}
              onChanged={lookup.reload}
            />
          )}
        </section>
      ) : (
        <>
          <section className="panel catalog-filters">
            <Field label="Buscar por nombre">
              <input
                type="search"
                maxLength={100}
                value={search}
                placeholder="Escribe para buscar…"
                onChange={(event) => {
                  setSearch(event.target.value);
                  setPage(1);
                }}
              />
            </Field>
            <Field label="Estado">
              <select
                value={active}
                onChange={(event) => {
                  setActive(event.target.value);
                  setPage(1);
                }}
              >
                <option value="">Todos</option>
                <option value="true">Activos</option>
                <option value="false">Inactivos</option>
              </select>
            </Field>
            {resource === 'products' && (
              <>
                <Field label="Categoría">
                  <select
                    value={category}
                    onChange={(event) => {
                      setCategory(event.target.value);
                      setPage(1);
                    }}
                  >
                    <option value="">Todas</option>
                    {lookup.data?.categories.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="Archivo">
                  <select
                    value={archived}
                    onChange={(event) => {
                      setArchived(event.target.value);
                      setPage(1);
                    }}
                  >
                    <option value="false">Sin archivar</option>
                    <option value="true">Archivados</option>
                    <option value="all">Todos</option>
                  </select>
                </Field>
              </>
            )}
            <button
              className="secondary-button"
              onClick={() => {
                void list.reload();
                void lookup.reload();
              }}
            >
              Actualizar
            </button>
          </section>
          <section className="panel">
            {list.loading && (
              <p className="muted" role="status">
                Consultando catálogo…
              </p>
            )}
            {!list.loading && list.data?.total === 0 && (
              <div className="catalog-empty">
                <h2>Aún no hay registros</h2>
                <p className="muted">
                  Crea la configuración de tu restaurante. Para productos, comienza por una
                  categoría.
                </p>
              </div>
            )}
            <div className="catalog-records">
              {!list.loading &&
                list.data?.items.map((record, index) => {
                  return (
                    <CatalogRecord
                      key={record.id}
                      record={record}
                      resource={resource}
                      categoryName={
                        lookup.data?.categories.find(
                          (item) => item.id === (record as Product).category_id,
                        )?.name ?? ''
                      }
                      editable={editable}
                      can={can}
                      onOpen={(id) => void openEditor(id)}
                      runAction={runAction}
                      existingCodes={lookup.data?.products.map((entry) => entry.code) ?? []}
                      onViewProducts={() => {
                        setCategory(record.id);
                        setTab('products');
                        setPage(1);
                      }}
                      onMoveUp={
                        resource === 'categories' && editable && index > 0 && !search
                          ? () => void moveCategoryUp(index)
                          : undefined
                      }
                    />
                  );
                })}
            </div>
            <div className="catalog-pagination">
              <span className="muted text-sm">
                {countLabel(list.data?.total ?? 0, 'registro', 'registros')} · Página {page}
              </span>
              <div className="button-row">
                <button
                  className="secondary-button"
                  disabled={page <= 1}
                  onClick={() => setPage(page - 1)}
                >
                  Anterior
                </button>
                <button
                  className="secondary-button"
                  disabled={!list.data || page * 20 >= list.data.total}
                  onClick={() => setPage(page + 1)}
                >
                  Siguiente
                </button>
              </div>
            </div>
          </section>
        </>
      )}
      {dialog}
    </>
  );
}
