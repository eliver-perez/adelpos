import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Notice } from '../../components/Forms';
import { Icon } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { inventoryChangeEvent } from '../../hooks/useInventoryEvents';
import { useResource } from '../../hooks/useResource';
import type {
  AlertPage,
  AlertSeverity,
  AlertType,
  InventoryAlert,
} from '../../services/inventoryControl';
import { countLabel } from '../../services/plural';
import { serverDateTime } from '../../services/restaurant';
import { alertActionLabels, severityTone } from './controlFormat';
import { Pagination } from './InventoryFields';
import type { InventoryNavigate, InventoryTarget } from './inventoryNavigation';
import { canReadPurchasing } from './purchasingFormat';

// Go devuelve hasta 100 alertas por página (su máximo).
const alertPageSize = 100;

// Destino de una alerta según su referencia y los permisos de la sesión: la pestaña o el registro
// relacionado, sin identificadores a la vista. Si la sesión no puede ver el destino, la alerta se
// muestra sin acceso directo.
export function alertTarget(
  alert: InventoryAlert,
  can: (permission: string) => boolean,
): InventoryTarget | null {
  const reference = alert.reference;
  const countsReadable = can('inventory.counts.perform') || can('inventory.counts.approve');
  switch (reference.kind) {
    case 'stock':
      if (can('inventory.stock.read')) {
        return {
          section: 'stock',
          search: alert.item?.code ?? reference.label,
          warehouseID: alert.warehouse?.id ?? '',
        };
      }
      return alert.item ? { section: 'items', itemID: alert.item.id } : null;
    case 'item':
      return reference.id ? { section: 'items', itemID: reference.id } : null;
    case 'product':
      return { section: 'recipes', search: reference.label };
    case 'count':
      return countsReadable && reference.id ? { section: 'counts', countID: reference.id } : null;
    case 'warehouse':
      return countsReadable && reference.id
        ? { section: 'counts', warehouseID: reference.id }
        : null;
    case 'purchase_order':
      return canReadPurchasing(can) && reference.id
        ? { section: 'purchases', orderID: reference.id }
        : null;
    case 'receipt':
      return canReadPurchasing(can) && reference.id
        ? { section: 'receipts', receiptID: reference.id }
        : null;
    case 'cancellations':
      return { section: 'adjustments' };
    case 'consumption_requests':
      return can('inventory.movements.read') ? { section: 'movements' } : null;
    default:
      return null;
  }
}

// Texto del acceso directo según el destino que resultó (sin existencias, la alerta de una
// existencia abre el insumo; quien solo aprueba conteos ve los del almacén en lugar de contarlo).
function actionLabel(
  alert: InventoryAlert,
  target: InventoryTarget,
  can: (permission: string) => boolean,
) {
  if (alert.reference.kind === 'stock' && target.section === 'items') {
    return alertActionLabels.item;
  }
  if (alert.reference.kind === 'warehouse' && can('inventory.counts.perform')) {
    return 'Contar almacén';
  }
  return alertActionLabels[alert.reference.kind];
}

interface AlertGroup {
  type: AlertType;
  label: string;
  severity: AlertSeverity;
  severityLabel: string;
  alerts: InventoryAlert[];
}

// Agrupa por tipo conservando el orden de Go (gravedad, tipo y nombre del destino).
function groupAlerts(alerts: InventoryAlert[]): AlertGroup[] {
  const groups: AlertGroup[] = [];
  for (const alert of alerts) {
    const group = groups.find((entry) => entry.type === alert.type);
    if (group) {
      group.alerts.push(alert);
    } else {
      groups.push({
        type: alert.type,
        label: alert.type_label,
        severity: alert.severity,
        severityLabel: alert.severity_label,
        alerts: [alert],
      });
    }
  }
  return groups;
}

function AlertRow({
  alert,
  can,
  timezone,
  onNavigate,
}: {
  alert: InventoryAlert;
  can: (permission: string) => boolean;
  timezone: string;
  onNavigate: InventoryNavigate;
}) {
  const target = alertTarget(alert, can);
  const context: string[] = [];
  if (alert.warehouse) {
    context.push(alert.warehouse.name);
  } else if (alert.reference.kind === 'stock') {
    context.push('Existencia total');
  }
  if (alert.occurred_at) {
    context.push(serverDateTime(alert.occurred_at, timezone));
  }
  return (
    <li className={`inventory-list-row control-alert control-alert-${alert.severity}`}>
      <div className="inventory-list-text">
        <span className="control-alert-message">
          <span className={severityTone(alert.severity)}>{alert.severity_label}</span>
          <span>{alert.message}</span>
        </span>
        {context.length > 0 && <span className="muted text-xs">{context.join(' · ')}</span>}
      </div>
      {target && (
        <button
          type="button"
          className="secondary-button"
          aria-label={`${actionLabel(alert, target, can)}: ${alert.reference.label}`}
          onClick={() => onNavigate(target)}
        >
          {actionLabel(alert, target, can)}
        </button>
      )}
    </li>
  );
}

// Alertas de inventario del resumen: las calcula Go en cada consulta según los permisos de la
// sesión (cantidades solo con inventory.stock.read, costos y compras con sus permisos) y nunca
// bloquean ventas ni operaciones. Se agrupan por tipo con su gravedad y cada una lleva a la pestaña
// o al registro relacionado.
export function AlertsSection({
  can,
  timezone,
  onNavigate,
}: {
  can: (permission: string) => boolean;
  timezone: string;
  onNavigate: InventoryNavigate;
}) {
  const [severity, setSeverity] = useState<AlertSeverity | ''>('');
  const [page, setPage] = useState(1);
  const query: Record<string, string> = {
    page: String(page),
    page_size: String(alertPageSize),
  };
  if (severity !== '') {
    query.severity = severity;
  }
  const resource = useResource<AlertPage>('inventory_alerts', {
    query,
    reloadEvent: inventoryChangeEvent,
  });
  const data = resource.data;
  const counts = data?.counts;
  const groups = data ? groupAlerts(data.items) : [];
  const options: { value: AlertSeverity | ''; label: string }[] = counts
    ? [
        { value: '', label: `Todas (${counts.total})` },
        { value: 'critical', label: `Críticas (${counts.critical})` },
        { value: 'warning', label: `Advertencias (${counts.warning})` },
        { value: 'info', label: `Avisos (${counts.info})` },
      ]
    : [];
  return (
    <section className="panel form-stack" aria-labelledby="inventory-alerts-title">
      <div className="section-heading">
        <div>
          <h2 id="inventory-alerts-title">Alertas de inventario</h2>
          <p className="muted text-xs">
            Se calculan al consultar. Ninguna bloquea ventas ni operaciones.
          </p>
        </div>
        {counts && (
          <div className="inventory-tags">
            {counts.critical > 0 && (
              <span className="tag tag-danger">
                {countLabel(counts.critical, 'crítica', 'críticas')}
              </span>
            )}
            {counts.warning > 0 && (
              <span className="tag tag-warning">
                {countLabel(counts.warning, 'advertencia', 'advertencias')}
              </span>
            )}
            {counts.info > 0 && (
              <span className="tag tag-info">{countLabel(counts.info, 'aviso', 'avisos')}</span>
            )}
            {counts.total === 0 && <span className="tag tag-good">Sin alertas</span>}
          </div>
        )}
      </div>
      <Notice>{resource.error}</Notice>
      {!data && !resource.error && <p className="muted">Consultando alertas…</p>}
      {data && counts && counts.total > 0 && (
        <div className="inventory-subnav">
          <SegmentedControl
            label="Gravedad de las alertas"
            value={severity}
            options={options}
            onChange={(next) => {
              setSeverity(next);
              setPage(1);
            }}
          />
        </div>
      )}
      {data && counts && counts.total === 0 && (
        <p className="muted control-alerts-empty">
          <Icon name="success" size={16} /> No hay alertas de inventario.
        </p>
      )}
      {data && counts && counts.total > 0 && data.items.length === 0 && (
        <p className="muted">No hay alertas con esta gravedad.</p>
      )}
      {groups.map((group) => (
        <Accordion
          key={`${severity}:${group.type}`}
          className={`control-alert-group control-alert-group-${group.severity}`}
          title={group.label}
          summary={`${group.severityLabel} · ${countLabel(group.alerts.length, 'alerta', 'alertas')}`}
          defaultOpen={group.severity !== 'info'}
        >
          <ul className="inventory-list" aria-label={group.label}>
            {group.alerts.map((alert, index) => (
              <AlertRow
                key={`${alert.reference.kind}:${alert.reference.id ?? ''}:${alert.warehouse?.id ?? ''}:${index}`}
                alert={alert}
                can={can}
                timezone={timezone}
                onNavigate={onNavigate}
              />
            ))}
          </ul>
        </Accordion>
      ))}
      {data && data.total > data.page_size && (
        <Pagination
          page={data.page}
          pageSize={data.page_size}
          total={data.total}
          singular="alerta"
          plural="alertas"
          onPage={setPage}
        />
      )}
    </section>
  );
}
