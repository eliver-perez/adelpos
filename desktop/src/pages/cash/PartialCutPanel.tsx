import { useState } from 'react';
import { Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { CashReport, CashReportSummary, CreatedCashReport } from '../../services/cash';
import type { Shift } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { CashDocumentActions } from './CashDocumentActions';
import { CashReportView, reportTitle } from './CashReportView';
import { PrintToggle } from './PrintToggle';
import type { CashMutation } from './useCashMutation';

// Consulta un corte con las reglas de Go (el esperado solo llega con su permiso) y ofrece su vista
// previa, impresión y reimpresión según permisos.
export function ReportLoader({
  reportID,
  shiftID,
  can,
  timezone,
}: {
  reportID: string;
  shiftID: string;
  can: (permission: string) => boolean;
  timezone: string;
}) {
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
  return (
    <div className="form-stack">
      <CashReportView report={report.data} timezone={timezone} />
      <CashDocumentActions source={{ kind: 'report', id: reportID }} shiftID={shiftID} can={can} />
    </div>
  );
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
// sacar efectivo se ofrece el acceso directo «Retirar efectivo» (terminología de §0-B). Desde la
// etapa 7.1, generar el corte lo anuncia la escritura del turno (useCashRecording) y quien abrió
// el panel lo cierra; los cortes anteriores se consultan desde la lista.
export function PartialCutPanel({
  shift,
  reports,
  can,
  mutation,
  timezone,
  onWithdraw,
  onClose,
  backLabel = 'Volver a la caja',
}: {
  shift: Shift;
  reports: CashReportSummary[];
  can: (permission: string) => boolean;
  mutation: CashMutation;
  timezone: string;
  onWithdraw: () => void;
  onClose: () => void;
  backLabel?: string;
}) {
  const [selectedID, setSelectedID] = useState('');
  const [print, setPrint] = useState(false);
  const canCreate = can('cash.partial_cuts.create') && shift.state === 'open';
  const partials = reports.filter((report) => report.kind === 'partial');

  async function create() {
    if (!canCreate || mutation.isPending()) {
      return;
    }
    try {
      // El aviso confirma el corte sin cifras; el informe se consulta en la lista, con el
      // esperado solo si Go lo entrega a quien tiene permiso.
      const report = await mutation.execute<CreatedCashReport>('payments_shift_partial_cut', {
        id: shift.id,
        body: print ? { print: true } : {},
      });
      setSelectedID(report.id);
    } catch (problem) {
      mutation.reportFailure(problem, 'No se pudo generar el corte parcial.');
    }
  }

  return (
    <section className="cash-panel" aria-label="Corte parcial">
      <header className="cash-panel-header">
        <h3>Corte parcial</h3>
        <button type="button" className="secondary-button" onClick={onClose}>
          <IconLabel icon="back">{backLabel}</IconLabel>
        </button>
      </header>
      <p className="cash-terminology">
        Un <strong>corte parcial</strong> es un informe del turno hasta este momento: no mueve
        dinero, no cierra el turno y no pausa la caja. Para sacar dinero de la caja registra un{' '}
        <strong>retiro de efectivo</strong>.
      </p>
      {canCreate && (
        <PrintToggle
          label="Imprimir el corte"
          pressed={print}
          onChange={setPrint}
          can={can}
          disabled={mutation.locked}
        />
      )}
      <div className="button-row">
        {canCreate && (
          <button
            type="button"
            className="primary-button"
            disabled={mutation.locked}
            onClick={() => void create()}
          >
            <IconLabel icon="report">
              {mutation.busy ? 'Generando corte…' : 'Generar corte parcial'}
            </IconLabel>
          </button>
        )}
        {can('cash.withdrawals.record') && shift.state === 'open' && (
          <button type="button" className="secondary-button" onClick={onWithdraw}>
            <IconLabel icon="withdrawal">Retirar efectivo</IconLabel>
          </button>
        )}
      </div>
      <ReportList
        reports={partials}
        selectedID={selectedID}
        timezone={timezone}
        onSelect={setSelectedID}
      />
      {selectedID && (
        <ReportLoader
          key={selectedID}
          reportID={selectedID}
          shiftID={shift.id}
          can={can}
          timezone={timezone}
        />
      )}
    </section>
  );
}
