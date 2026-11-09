import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { ShiftDetail, ShiftSummary } from '../../services/cash';
import type { Shift } from '../../services/payments';
import { ClosureResult } from './ClosureResult';
import { businessDate } from './movementRows';
import { MovementsTimeline } from './MovementsView';
import { ReportList, ReportLoader } from './PartialCutPanel';
import { ShiftStatusCard } from './ShiftStatusCard';
import { TotalsTables } from './TotalsTables';

// Totales del turno con el mismo cálculo de los cortes (resumen de Go).
function ShiftTotalsSection({ shiftID }: { shiftID: string }) {
  const summary = useLiveResource<ShiftSummary>('payments_shift_summary', {
    id: shiftID,
    interval: 60000,
  });
  if (summary.error) {
    return <Notice>{summary.error}</Notice>;
  }
  if (!summary.data) {
    return <p className="muted">Consultando totales…</p>;
  }
  if (summary.data.blind || !summary.data.accumulated) {
    return (
      <p className="muted text-sm">
        Arqueo de cierre en curso: los totales se consultan al terminar el arqueo.
      </p>
    );
  }
  return <TotalsTables totals={summary.data.accumulated} />;
}

// Cortes parciales y corte definitivo del turno.
function ShiftReports({
  shiftID,
  can,
  timezone,
}: {
  shiftID: string;
  can: (permission: string) => boolean;
  timezone: string;
}) {
  const [selectedID, setSelectedID] = useState('');
  const detail = useLiveResource<ShiftDetail>('payments_shift', { id: shiftID, interval: 60000 });
  if (detail.error) {
    return <Notice>{detail.error}</Notice>;
  }
  return (
    <div className="form-stack">
      <ReportList
        reports={detail.data?.reports ?? []}
        selectedID={selectedID}
        timezone={timezone}
        onSelect={setSelectedID}
      />
      {selectedID && (
        <ReportLoader
          key={selectedID}
          reportID={selectedID}
          shiftID={shiftID}
          can={can}
          timezone={timezone}
        />
      )}
    </div>
  );
}

// Detalle de un turno del historial (§19): apertura, cierre, responsables, totales, movimientos,
// cortes parciales, corte definitivo, regularizaciones enlazadas y comentarios del supervisor.
// Las diferencias solo aparecen si Go las entrega.
export function ShiftHistoryDetail({
  shift,
  ownShift,
  can,
  timezone,
  onClose,
}: {
  shift: Shift;
  ownShift: Shift | null;
  can: (permission: string) => boolean;
  timezone: string;
  onClose: () => void;
}) {
  return (
    <section className="cash-panel" aria-label={`Turno del ${businessDate(shift.business_date)}`}>
      <header className="cash-panel-header">
        <h3>
          {shift.register_name} · turno del {businessDate(shift.business_date)}
        </h3>
        <button type="button" className="secondary-button" onClick={onClose}>
          Volver al historial
        </button>
      </header>
      <ShiftStatusCard shift={shift} timezone={timezone} />
      {shift.state === 'closed' && (
        <ClosureResult shiftID={shift.id} can={can} timezone={timezone} />
      )}
      <Accordion
        title="Totales del turno"
        summary="Ventas por método, propinas, devoluciones y operaciones"
      >
        <ShiftTotalsSection shiftID={shift.id} />
      </Accordion>
      <Accordion title="Cortes" summary="Cortes parciales y corte definitivo">
        <ShiftReports shiftID={shift.id} can={can} timezone={timezone} />
      </Accordion>
      <Accordion title="Movimientos" summary="Cobros, devoluciones y operaciones de efectivo">
        <MovementsTimeline shiftID={shift.id} ownShift={ownShift} can={can} timezone={timezone} />
      </Accordion>
    </section>
  );
}
