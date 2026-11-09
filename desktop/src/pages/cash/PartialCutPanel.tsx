import { useState } from 'react';
import { Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { CashReport, CashReportSummary } from '../../services/cash';
import type { Shift } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { CashReportView, reportTitle } from './CashReportView';
import type { CashMutation } from './useCashMutation';

// Consulta un corte con las reglas de Go: el esperado solo llega con su permiso.
export function ReportLoader({ reportID, timezone }: { reportID: string; timezone: string }) {
  const report = useLiveResource<CashReport>('payments_cash_report', {
    id: reportID,
    interval: 60000,
  });
  if (report.error) {
    return <Notice>{report.error}</Notice>;
  }
  if (!report.data) {
    return <p className="muted">Consultando el corte…</p>;
  }
  return <CashReportView report={report.data} timezone={timezone} />;
}

// Lista de cortes del turno; cada uno se abre para consultarlo.
export function ReportList({
  reports,
  selectedID,
  timezone,
  onSelect,
}: {
  reports: CashReportSummary[];
  selectedID: string;
  timezone: string;
  onSelect: (id: string) => void;
}) {
  if (reports.length === 0) {
    return <p className="muted text-sm">Este turno no tiene cortes registrados.</p>;
  }
  return (
    <div className="cash-choice-grid" role="group" aria-label="Cortes del turno">
      {reports.map((report) => (
        <button
          type="button"
          key={report.id}
          className={selectedID === report.id ? 'cash-choice-selected' : ''}
          aria-pressed={selectedID === report.id}
          onClick={() => onSelect(report.id)}
        >
          {reportTitle(report)} · {serverDateTime(report.created_at, timezone)}
        </button>
      ))}
    </div>
  );
}

// Corte parcial: informe del turno hasta este momento. No retira dinero ni pausa la caja; para
// sacar efectivo se ofrece el acceso directo «Retirar efectivo» (terminología de §0-B).
export function PartialCutPanel({
  shift,
  reports,
  can,
  mutation,
  timezone,
  onWithdraw,
  onClose,
}: {
  shift: Shift;
  reports: CashReportSummary[];
  can: (permission: string) => boolean;
  mutation: CashMutation;
  timezone: string;
  onWithdraw: () => void;
  onClose: () => void;
}) {
  const [selectedID, setSelectedID] = useState('');
  const canCreate = can('cash.partial_cuts.create') && shift.state === 'open';
  const partials = reports.filter((report) => report.kind === 'partial');

  async function create() {
    if (!canCreate || mutation.isPending()) {
      return;
    }
    try {
      const report = await mutation.execute<CashReport>('payments_shift_partial_cut', {
        id: shift.id,
        body: {},
      });
      setSelectedID(report.id);
    } catch {
      // El mensaje de Go queda visible en el aviso de la operación.
    }
  }

  return (
    <section className="cash-panel" aria-label="Corte parcial">
      <header className="cash-panel-header">
        <h3>Corte parcial</h3>
        <button type="button" className="secondary-button" onClick={onClose}>
          Volver a la caja
        </button>
      </header>
      <p className="cash-terminology">
        Un <strong>corte parcial</strong> es un informe del turno hasta este momento: no mueve
        dinero, no cierra el turno y no pausa la caja. Para sacar dinero de la caja registra un{' '}
        <strong>retiro de efectivo</strong>.
      </p>
      <div className="button-row">
        {canCreate && (
          <button
            type="button"
            className="primary-button"
            disabled={mutation.locked}
            onClick={() => void create()}
          >
            {mutation.busy ? 'Generando corte…' : 'Generar corte parcial'}
          </button>
        )}
        {can('cash.withdrawals.record') && shift.state === 'open' && (
          <button type="button" className="secondary-button" onClick={onWithdraw}>
            Retirar efectivo
          </button>
        )}
      </div>
      {mutation.error && (
        <p className="checkout-inline-note" role="alert">
          {mutation.error}
        </p>
      )}
      <ReportList
        reports={partials}
        selectedID={selectedID}
        timezone={timezone}
        onSelect={setSelectedID}
      />
      {selectedID && <ReportLoader key={selectedID} reportID={selectedID} timezone={timezone} />}
    </section>
  );
}
