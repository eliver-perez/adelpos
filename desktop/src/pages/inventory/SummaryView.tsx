import type { ReactNode } from 'react';
import { Notice } from '../../components/Forms';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import { serverDateTime } from '../../services/restaurant';
import type { InventorySummary, Page, StockState } from '../../services/inventory';
import type {
  AdjustmentPage,
  CountSummary,
  PurchaseSuggestionPage,
} from '../../services/inventoryControl';
import type { PurchaseOrderSummary } from '../../services/inventoryPurchasing';
import type { CancellationPage, ConsumptionRequestPage } from '../../services/inventoryRecipes';
import { formatMoney } from '../../services/money';
import { countLabel } from '../../services/plural';
import { canReadAdjustments } from './AdjustmentRecords';
import { AlertsSection } from './AlertsSection';
import { inventoryPermissionName } from './inventoryFormat';
import type { InventoryNavigate } from './inventoryNavigation';
import { canReadPurchasing } from './purchasingFormat';

// Tarjeta de una cifra del resumen. Con permiso de existencias, las que corresponden a un filtro
// abren Existencias con ese filtro aplicado.
function SummaryFigure({
  label,
  value,
  detail,
  tone = 'default',
  onOpen,
}: {
  label: string;
  value: ReactNode;
  detail?: string;
  tone?: 'default' | 'warning';
  onOpen?: () => void;
}) {
  const className = `inventory-figure ${tone === 'warning' ? 'inventory-figure-warning' : ''}`;
  const body = (
    <>
      <span className="inventory-figure-label">{label}</span>
      <strong>{value}</strong>
      {detail && <span className="muted text-xs">{detail}</span>}
    </>
  );
  if (!onOpen) {
    return <div className={className}>{body}</div>;
  }
  return (
    <button type="button" className={className} onClick={onOpen}>
      {body}
    </button>
  );
}

// Estado de «Control de inventario». La función se activa en Configuración → Funciones del
// establecimiento (etapa 6.1); aquí solo se informa y se lleva a esa pantalla, sin un segundo
// interruptor.
function FeatureStatus({
  summary,
  can,
  timezone,
  onOpenSettings,
}: {
  summary: InventorySummary;
  can: (permission: string) => boolean;
  timezone: string;
  onOpenSettings: () => void;
}) {
  const feature = summary.feature;
  const since = feature.enabled_at ? serverDateTime(feature.enabled_at, timezone) : null;
  let state = 'Desactivado';
  if (feature.enabled) {
    state = since ? `Activado desde el ${since}` : 'Activado';
  }
  const manageable = can('settings.features.manage') && can('settings.read');
  return (
    <section className="panel inventory-feature" aria-labelledby="inventory-feature-title">
      <div className="section-heading">
        <div>
          <h2 id="inventory-feature-title">{feature.name}</h2>
          <p className={feature.enabled ? 'feature-state-on' : 'muted'}>{state}</p>
        </div>
        {manageable && (
          <button type="button" className="secondary-button" onClick={onOpenSettings}>
            Administrar funciones
          </button>
        )}
      </div>
      <p className="muted text-sm">
        Con el control activado, el inventario descuenta solo lo que se libera a cocina después de
        activarlo; lo liberado antes nunca genera consumo. Insumos, almacenes y existencias
        iniciales se pueden preparar antes de activarlo, y desactivarlo deja de generar consumos sin
        borrar el historial.
      </p>
      {!feature.enabled && since && <p className="muted text-xs">Última activación: {since}.</p>}
      {!manageable && (
        <p className="muted text-xs">
          Lo activa quien tiene el permiso «{inventoryPermissionName('settings.features.manage')}»
          en Configuración.
        </p>
      )}
    </section>
  );
}

// Cifras de la fase B que Go ya entrega en sus listados: cancelaciones por clasificar (con
// inventory.read) y solicitudes de consumo pendientes o con error (con inventory.movements.read).
// Si una consulta falla o falta el permiso, la cifra no se muestra: nunca se inventa.
function ConsumptionFigures({
  can,
  onNavigate,
}: {
  can: (permission: string) => boolean;
  onNavigate: InventoryNavigate;
}) {
  const movementsReadable = can('inventory.movements.read');
  const cancellations = useResource<CancellationPage>('inventory_cancellations', {
    query: { state: 'pending', page_size: '1' },
    reloadEvent: inventoryChangeEvent,
  });
  const requests = useResource<ConsumptionRequestPage>('inventory_consumption_requests', {
    query: { page_size: '1' },
    enabled: movementsReadable,
    reloadEvent: inventoryChangeEvent,
  });
  const pending = cancellations.data?.pending ?? null;
  const failed = movementsReadable ? (requests.data?.failed ?? null) : null;
  const waiting = movementsReadable ? (requests.data?.pending ?? null) : null;
  return (
    <>
      {pending !== null && (
        <SummaryFigure
          label="Cancelaciones por clasificar"
          value={pending}
          detail={pending > 0 ? 'Al cierre del día se consideran «No preparado»' : undefined}
          tone={pending > 0 ? 'warning' : 'default'}
          onOpen={() => onNavigate({ section: 'adjustments' })}
        />
      )}
      {failed !== null && (
        <SummaryFigure
          label="Solicitudes de consumo con error"
          value={failed}
          detail={waiting !== null ? countLabel(waiting, 'pendiente', 'pendientes') : undefined}
          tone={failed > 0 ? 'warning' : 'default'}
          onOpen={() => onNavigate({ section: 'movements' })}
        />
      )}
    </>
  );
}

// Cifras de las fases C y D tal como las entregan los listados de Go (su total): ajustes por
// autorizar, conteos en curso, órdenes de compra abiertas e insumos con sugerencia de compra. Cada
// una aparece solo con el permiso que Go exige para consultarla.
function ControlFigures({
  can,
  onNavigate,
}: {
  can: (permission: string) => boolean;
  onNavigate: InventoryNavigate;
}) {
  const adjustmentsReadable = canReadAdjustments(can);
  const countsReadable = can('inventory.counts.perform') || can('inventory.counts.approve');
  const purchasingReadable = canReadPurchasing(can);
  const adjustments = useResource<AdjustmentPage>('inventory_adjustments', {
    query: { state: 'pending_authorization', page_size: '1' },
    enabled: adjustmentsReadable,
    reloadEvent: inventoryChangeEvent,
  });
  const counts = useResource<Page<CountSummary>>('inventory_counts', {
    query: { state: 'active', page_size: '1' },
    enabled: countsReadable,
    reloadEvent: inventoryChangeEvent,
  });
  const orders = useResource<Page<PurchaseOrderSummary>>('inventory_purchase_orders', {
    query: { state: 'open', page_size: '1' },
    enabled: purchasingReadable,
    reloadEvent: inventoryChangeEvent,
  });
  const suggestions = useResource<PurchaseSuggestionPage>('inventory_purchase_suggestions', {
    query: { page_size: '1' },
    reloadEvent: inventoryChangeEvent,
  });
  const pendingAdjustments = adjustmentsReadable ? (adjustments.data?.total ?? null) : null;
  const activeCounts = countsReadable ? (counts.data?.total ?? null) : null;
  const openOrders = purchasingReadable ? (orders.data?.total ?? null) : null;
  const suggested = suggestions.data?.total ?? null;
  return (
    <>
      {pendingAdjustments !== null && (
        <SummaryFigure
          label="Ajustes por autorizar"
          value={pendingAdjustments}
          detail={pendingAdjustments > 0 ? 'No afectan existencias hasta autorizarse' : undefined}
          tone={pendingAdjustments > 0 ? 'warning' : 'default'}
          onOpen={() => onNavigate({ section: 'adjustments' })}
        />
      )}
      {activeCounts !== null && (
        <SummaryFigure
          label="Conteos en curso"
          value={activeCounts}
          detail="Abiertos, enviados o aprobados"
          onOpen={() => onNavigate({ section: 'counts' })}
        />
      )}
      {openOrders !== null && (
        <SummaryFigure
          label="Órdenes de compra abiertas"
          value={openOrders}
          detail="Emitidas o parcialmente recibidas"
          onOpen={() => onNavigate({ section: 'purchases', subsection: 'orders' })}
        />
      )}
      {suggested !== null && (
        <SummaryFigure
          label="Insumos por reabastecer"
          value={suggested}
          detail="Con sugerencia de compra"
          tone={suggested > 0 ? 'warning' : 'default'}
          onOpen={
            purchasingReadable
              ? () => onNavigate({ section: 'purchases', subsection: 'suggestions' })
              : undefined
          }
        />
      )}
    </>
  );
}

// Resumen de inventario: estado de la función, cifras de todas las fases y alertas, todo calculado
// por Go. El valor total solo aparece si Go lo entrega (inventory.valuation.read) y los insumos con
// costo pendiente solo con inventory.costs.read.
export function SummaryView({
  can,
  timezone,
  onNavigate,
  onOpenSettings,
}: {
  can: (permission: string) => boolean;
  timezone: string;
  onNavigate: InventoryNavigate;
  onOpenSettings: () => void;
}) {
  const resource = useResource<InventorySummary>('inventory_summary', {
    reloadEvent: inventoryChangeEvent,
  });
  const summary = resource.data;
  if (!summary) {
    return <Notice>{resource.error || 'Consultando el resumen de inventario…'}</Notice>;
  }
  const stockReadable = can('inventory.stock.read');
  const open = (state: StockState | '') =>
    stockReadable ? () => onNavigate({ section: 'stock', state }) : undefined;
  return (
    <div className="form-stack">
      <Notice>{resource.error}</Notice>
      <FeatureStatus
        summary={summary}
        can={can}
        timezone={timezone}
        onOpenSettings={onOpenSettings}
      />
      <section className="inventory-figures" aria-label="Cifras de inventario">
        <SummaryFigure
          label="Insumos activos"
          value={summary.active_items}
          detail={countLabel(summary.inactive_items, 'inactivo', 'inactivos')}
        />
        <SummaryFigure label="Almacenes activos" value={summary.active_warehouses} />
        <SummaryFigure
          label="Existencias negativas"
          value={summary.negative_stock}
          detail={summary.negative_stock > 0 ? 'No bloquean la operación' : undefined}
          tone={summary.negative_stock > 0 ? 'warning' : 'default'}
          onOpen={open('negative')}
        />
        <SummaryFigure
          label="Bajo mínimo"
          value={summary.below_minimum}
          tone={summary.below_minimum > 0 ? 'warning' : 'default'}
          onOpen={open('below_minimum')}
        />
        <SummaryFigure
          label="En punto de reabastecimiento"
          value={summary.at_reorder_point}
          tone={summary.at_reorder_point > 0 ? 'warning' : 'default'}
          onOpen={open('reorder')}
        />
        {summary.costs_visible && summary.pending_cost_items !== null && (
          <SummaryFigure
            label="Con costo pendiente"
            value={summary.pending_cost_items}
            detail="Salidas sin costo conocido"
            tone={summary.pending_cost_items > 0 ? 'warning' : 'default'}
          />
        )}
        {summary.valuation_visible && summary.total_value_display !== null && (
          <SummaryFigure
            label="Valor del inventario"
            value={formatMoney(summary.total_value_display)}
            detail="Costo promedio de todos los insumos"
          />
        )}
        <ConsumptionFigures can={can} onNavigate={onNavigate} />
        <ControlFigures can={can} onNavigate={onNavigate} />
      </section>
      <AlertsSection can={can} timezone={timezone} onNavigate={onNavigate} />
    </div>
  );
}
