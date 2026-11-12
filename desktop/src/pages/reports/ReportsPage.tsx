import { useCallback, useRef, useState } from 'react';
import { Field, PageTitle } from '../../components/Forms';
import { defaultPeriod } from '../../components/reports/period';
import { useBreadcrumbTail } from '../../navigation/NavigationContext';
import type { PeriodSelection } from '../../components/reports/period';
import { fetchFilterOptions, fetchReportCatalog } from '../../services/reports';
import type { FilterOption, ReportCatalog } from '../../services/reports';
import { DashboardView } from './DashboardView';
import { ReportView } from './ReportView';
import { useReportData } from './useReportData';
import { Select } from '../../components/Select';

// Sección principal «Reportes» (etapa 9A, §4 y §26), con reports.read. La navegación sale del
// catálogo de Go: solo aparecen los grupos y reportes que el usuario puede consultar (Resumen
// ejecutivo, Ventas, Productos, Categorías, Pagos, Descuentos y cortesías, Caja, Cocina y operación,
// Inventarios, Compras, Proveedores, Costos y márgenes, Empleados…), de modo que un reporte nuevo
// del registro aparece sin cambiar esta pantalla. El periodo y los filtros se conservan al cambiar
// de reporte: se eligen en un solo lugar y cada reporte envía solo los que declara.

const summaryGroup = 'summary';
const dashboardEntry = '__dashboard__';

interface NavigationGroup {
  id: string;
  title: string;
  entries: { id: string; title: string }[];
}

function navigationGroups(catalog: ReportCatalog): NavigationGroup[] {
  const groups: NavigationGroup[] = [];
  const summary = catalog.groups.find((group) => group.id === summaryGroup);
  if (catalog.dashboard || summary) {
    groups.push({
      id: summaryGroup,
      title: summary?.title ?? 'Resumen ejecutivo',
      entries: [
        ...(catalog.dashboard ? [{ id: dashboardEntry, title: 'Indicadores' }] : []),
        ...(summary?.reports ?? []).map((report) => ({ id: report.id, title: report.title })),
      ],
    });
  }
  for (const group of catalog.groups) {
    if (group.id === summaryGroup) {
      continue;
    }
    groups.push({
      id: group.id,
      title: group.title,
      entries: group.reports.map((report) => ({ id: report.id, title: report.title })),
    });
  }
  return groups;
}

// En pantallas anchas los reportes del grupo se listan a la izquierda; por debajo de 1440 px de
// ancho la lista se sustituye por un selector «Reporte» (styles.css) para dejar todo el ancho a la
// tabla. Los dos controles eligen lo mismo y solo uno es visible.

// Columnas iguales en una o dos filas completas: hasta 7 grupos en una fila; con más, la mitad
// redondeada hacia arriba (por ejemplo 7 y 6), nunca una fila casi vacía.
function groupColumns(count: number) {
  return count <= 7 ? Math.max(1, count) : Math.ceil(count / 2);
}

export function ReportsPage({
  can,
  timezone,
  permissions,
}: {
  can: (permission: string) => boolean;
  timezone: string;
  // Los permisos efectivos: si cambian, el catálogo se vuelve a pedir a Go.
  permissions: string[];
}) {
  const catalogRequest = `catalog:${permissions.join(',')}`;
  const catalogResource = useReportData<ReportCatalog>(catalogRequest, 'catalog', () =>
    fetchReportCatalog(),
  );
  const catalog = catalogResource.data;
  const [groupID, setGroupID] = useState<string | null>(null);
  const [selected, setSelected] = useState<Record<string, string>>({});
  const [period, setPeriod] = useState<PeriodSelection>(defaultPeriod);
  const [filters, setFilters] = useState<Record<string, string>>({});
  const optionCache = useRef(new Map<string, Promise<FilterOption[]>>());

  // Opciones de una fuente de filtros, una sola vez por fuente mientras la pantalla está abierta.
  const loadOptions = useCallback((source: string) => {
    const cached = optionCache.current.get(source);
    if (cached) {
      return cached;
    }
    const pending = fetchFilterOptions(source).then((response) => response.options);
    optionCache.current.set(source, pending);
    pending.catch(() => optionCache.current.delete(source));
    return pending;
  }, []);

  const groups = catalog ? navigationGroups(catalog) : [];
  const currentGroup = groups.find((group) => group.id === groupID) ?? groups[0] ?? null;
  // Etapa 9C: el grupo visible es el último tramo de la ruta («… › Reportes › Ventas»).
  useBreadcrumbTail(currentGroup?.title ?? null);
  const entryID =
    currentGroup &&
    (currentGroup.entries.some((entry) => entry.id === selected[currentGroup.id])
      ? selected[currentGroup.id]!
      : (currentGroup.entries[0]?.id ?? null));
  const report =
    catalog && entryID && entryID !== dashboardEntry
      ? (catalog.groups.flatMap((group) => group.reports).find((entry) => entry.id === entryID) ??
        null)
      : null;

  function openReport(reportID: string) {
    const group = groups.find((entry) => entry.entries.some((item) => item.id === reportID));
    if (!group) {
      return;
    }
    setGroupID(group.id);
    setSelected((current) => ({ ...current, [group.id]: reportID }));
  }

  function content() {
    if (!catalog || !currentGroup || !entryID) {
      return null;
    }
    if (entryID === dashboardEntry) {
      return (
        <DashboardView
          catalog={catalog}
          period={period}
          onPeriod={setPeriod}
          onOpenReport={openReport}
          timezone={timezone}
        />
      );
    }
    if (!report) {
      return null;
    }
    return (
      <ReportView
        key={report.id}
        report={report}
        catalog={catalog}
        period={period}
        onPeriod={setPeriod}
        filters={filters}
        onFilter={(key, value) => setFilters((current) => ({ ...current, [key]: value }))}
        loadOptions={loadOptions}
        can={can}
        timezone={timezone}
      />
    );
  }

  return (
    <div className="reports-page form-stack">
      <PageTitle
        title="Reportes"
        description="Ventas, cobros, descuentos, caja, inventarios y compras por día comercial, semana, mes o rango. Cada cifra se calcula con la hora de inicio del día; consultar un reporte no detiene cobros, captura ni cocina."
      />
      {catalogResource.error && (
        <div className="report-state report-state-error" role="alert">
          <p>{catalogResource.error}</p>
          <button
            type="button"
            className="secondary-button"
            onClick={() => void catalogResource.reload()}
          >
            Reintentar
          </button>
        </div>
      )}
      {!catalog && !catalogResource.error && (
        <div className="report-state" role="status" aria-busy="true">
          Cargando reportes…
        </div>
      )}
      {catalog && groups.length === 0 && (
        <div className="report-state" role="status">
          Tu usuario no tiene reportes disponibles. Cada grupo de reportes requiere su permiso (por
          ejemplo, «Consultar reportes de ventas»).
        </div>
      )}
      {currentGroup && (
        <nav className="reports-navigation" aria-label="Reportes">
          <div
            className="catalog-tabs reports-groups"
            role="tablist"
            aria-label="Grupos de reportes"
            style={{
              gridTemplateColumns: `repeat(${groupColumns(groups.length)}, minmax(0, 1fr))`,
            }}
          >
            {groups.map((group) => (
              <button
                type="button"
                key={group.id}
                role="tab"
                aria-selected={currentGroup.id === group.id}
                className={currentGroup.id === group.id ? 'tab-active' : ''}
                onClick={() => setGroupID(group.id)}
              >
                {group.title}
              </button>
            ))}
          </div>
        </nav>
      )}
      {currentGroup && (
        <div
          className={`reports-layout ${currentGroup.entries.length > 1 ? 'reports-layout-list' : ''}`}
        >
          {currentGroup.entries.length > 1 && (
            <nav className="reports-list" aria-label={`Reportes de ${currentGroup.title}`}>
              {currentGroup.entries.map((entry) => (
                <button
                  type="button"
                  key={entry.id}
                  aria-current={entry.id === entryID ? 'page' : undefined}
                  className={entry.id === entryID ? 'reports-list-active' : ''}
                  onClick={() =>
                    setSelected((current) => ({ ...current, [currentGroup.id]: entry.id }))
                  }
                >
                  {entry.title}
                </button>
              ))}
            </nav>
          )}
          <div className="reports-content">
            {currentGroup.entries.length > 1 && (
              <div className="reports-picker">
                <Field label="Reporte">
                  <Select
                    value={entryID ?? ''}
                    options={currentGroup.entries.map((entry) => ({
                      value: entry.id,
                      label: entry.title,
                    }))}
                    onChange={(next) =>
                      setSelected((current) => ({ ...current, [currentGroup.id]: next }))
                    }
                  />
                </Field>
              </div>
            )}
            {content()}
          </div>
        </div>
      )}
    </div>
  );
}
