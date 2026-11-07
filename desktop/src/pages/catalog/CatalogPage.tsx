import { useState } from 'react';
import { Field, Notice, PageTitle } from '../../components/Forms';
import { CatalogRecord } from './CatalogRecord';
import { useResource } from '../../hooks/useResource';
import { api } from '../../services/api';
import type {
  CatalogDocument,
  CatalogPage as Page,
  Lookups,
  Product,
} from '../../services/catalog';
import { CatalogEditor } from './CatalogEditor';
import { catalogOperation, catalogTabs, newCatalogDocument } from './catalogState';

export function CatalogPage({ can }: { can: (permission: string) => boolean }) {
  const [tab, setTab] = useState<(typeof catalogTabs)[number]['id']>('products');
  const [search, setSearch] = useState('');
  const [active, setActive] = useState('');
  const [category, setCategory] = useState('');
  const [archived, setArchived] = useState('false');
  const [page, setPage] = useState(1);
  const [draft, setDraft] = useState<CatalogDocument | null>(null);
  const [savedDraft, setSavedDraft] = useState('');
  const [error, setError] = useState('');
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

  function closeEditor() {
    if (
      draft &&
      JSON.stringify(draft) !== savedDraft &&
      !window.confirm('Tienes cambios sin guardar. ¿Descartarlos?')
    )
      return false;
    setDraft(null);
    return true;
  }

  async function openEditor(id: string) {
    if (!closeEditor()) return;
    setError('');
    try {
      const record = await api<CatalogDocument>(catalogOperation(resource, 'get_'), { id });
      setDraft(record);
      setSavedDraft(JSON.stringify(record));
    } catch (problem) {
      setError(problem instanceof Error ? problem.message : 'No se pudo abrir el registro.');
    }
  }

  async function runAction(action: () => Promise<unknown>) {
    setError('');
    try {
      await action();
      await list.reload();
      await lookup.reload();
    } catch (problem) {
      setError(problem instanceof Error ? problem.message : 'No se pudo completar.');
    }
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
    });
  }

  return (
    <>
      <PageTitle
        title="Catálogo comercial"
        description="Productos, preparación y reglas de precio persistidos en el servidor."
      >
        {editable && !draft && (
          <button
            className="primary-button"
            disabled={!lookup.data}
            onClick={() => {
              if (!closeEditor()) return;
              const record = newCatalogDocument(
                resource,
                tab === 'combos',
                category || lookup.data?.categories[0]?.id || '',
              );
              setDraft(record);
              setSavedDraft(JSON.stringify(record));
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
            aria-selected={tab === item.id}
            className={tab === item.id ? 'tab-active' : ''}
            onClick={() => {
              if (!closeEditor()) return;
              setTab(item.id);
              setPage(1);
              setSearch('');
            }}
          >
            {item.label}
          </button>
        ))}
      </div>
      <Notice>{error || lookup.error || list.error}</Notice>
      {draft && lookup.data ? (
        <section className="panel catalog-editor">
          <div className="section-heading">
            <div>
              <h2>{draft.id ? draft.name : 'Nuevo registro'}</h2>
              <p className="muted text-xs">
                {draft.id ? `Revisión ${draft.revision}` : 'Se guardará al confirmar'}
              </p>
            </div>
            <button className="secondary-button" onClick={() => closeEditor()}>
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
            key={`${resource}-${draft.id}`}
            resource={resource}
            value={draft}
            onChange={setDraft}
            lookups={lookup.data}
            can={can}
            editable={editable}
            onSave={async () => {
              const saved = await api<CatalogDocument>(
                catalogOperation(resource, draft.id ? 'save_' : 'create_'),
                { id: draft.id || undefined, body: draft },
              );
              setDraft(saved);
              setSavedDraft(JSON.stringify(saved));
              await list.reload();
              await lookup.reload();
            }}
          />
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
                {list.data?.total ?? 0} registros · Página {page}
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
    </>
  );
}
