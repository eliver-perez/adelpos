import { useId, useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Notice } from '../../components/Forms';
import { useLiveResource } from '../../hooks/useLiveResource';
import { denominationLabel } from '../../services/cash';
import type { ClosureView, PrintOutcome } from '../../services/cash';
import { formatDollars, formatMoney, permissionName, textLength } from '../../services/payments';
import { serverDateTime } from '../../services/restaurant';
import { CashDocumentActions } from './CashDocumentActions';
import { PrintNotice } from './CashNotices';
import { currencyAmount } from './DenominationCount';
import { DifferenceFigures, ExpectedFigures } from './ExpectedBlock';
import { authorizationText } from './RecordedOperation';
import { useCashMutation } from './useCashMutation';

// Lo declarado en el arqueo: líneas por denominación con el importe de Go y los totales.
function DeclaredCount({ view }: { view: ClosureView }) {
  const count = view.count;
  return (
    <div className="form-stack">
      <div className="shift-summary" aria-label="Efectivo declarado">
        <div>
          Pesos declarados
          <strong>{formatMoney(count?.counted_mxn)}</strong>
        </div>
        <div>
          Dólares declarados
          <strong>{formatDollars(count?.counted_usd)}</strong>
        </div>
        <div>
          Cerrado por
          <strong>
            {view.closed_by_name} · {view.device_name}
          </strong>
        </div>
      </div>
      {count && count.lines.some((line) => line.quantity > 0) && (
        <ul className="cash-count-lines" aria-label="Conteo por denominaciones">
          {count.lines
            .filter((line) => line.quantity > 0)
            .map((line) => (
              <li key={line.denomination_id}>
                {line.quantity} × {denominationLabel(line)} ={' '}
                {currencyAmount(line.currency, line.amount)}
              </li>
            ))}
        </ul>
      )}
      {view.cashier_notes && <p className="muted text-sm">Observaciones: {view.cashier_notes}</p>}
    </div>
  );
}

// Comentarios posteriores del supervisor; agregarlos exige cash.closures.review y no cambia la
// diferencia registrada.
function ClosureComments({
  view,
  can,
  timezone,
}: {
  view: ClosureView;
  can: (permission: string) => boolean;
  timezone: string;
}) {
  const identifier = useId();
  const [comment, setComment] = useState('');
  const mutation = useCashMutation(view.shift.id, () => setComment(''));
  const reviewer = can('cash.closures.review');
  if (!view.comments && !reviewer) {
    return null;
  }
  const valid = textLength(comment) >= 1 && textLength(comment) <= 2000;
  return (
    <Accordion
      title="Comentarios del supervisor"
      summary={`${view.comments?.length ?? 0} registrados`}
    >
      {(view.comments ?? []).map((item) => (
        <p key={item.id} className="text-sm">
          <strong>{item.author_name}</strong> · {serverDateTime(item.created_at, timezone)}:{' '}
          {item.comment}
        </p>
      ))}
      {reviewer && (
        <div className="form-stack">
          <div className="field">
            <label htmlFor={identifier}>Nuevo comentario</label>
            <textarea
              id={identifier}
              rows={2}
              maxLength={2000}
              aria-describedby={`${identifier}-hint`}
              value={comment}
              onChange={(event) => setComment(event.target.value)}
            />
            <small id={`${identifier}-hint`} className="muted">
              El comentario queda auditado y no cambia la diferencia.
            </small>
          </div>
          {mutation.error && (
            <p className="checkout-inline-note" role="alert">
              {mutation.error}
            </p>
          )}
          <button
            type="button"
            className="secondary-button"
            disabled={!valid || mutation.locked}
            onClick={() =>
              void mutation
                .execute('payments_shift_comment_closure', {
                  id: view.shift.id,
                  body: { comment: comment.trim() },
                })
                .catch(() => undefined)
            }
          >
            Agregar comentario
          </button>
        </div>
      )}
    </Accordion>
  );
}

// Resultado de un turno cerrado según los permisos de quien consulta (§12 y §13): lo declarado y
// que el turno quedó cerrado siempre; el esperado, las diferencias por moneda y su equivalencia
// solo si Go los entrega (cash.expected.read).
export function ClosureResult({
  shiftID,
  can,
  timezone,
  printing,
}: {
  shiftID: string;
  can: (permission: string) => boolean;
  timezone: string;
  // Estado de la impresión del corte definitivo solicitada al cerrar (fase C).
  printing?: PrintOutcome;
}) {
  const closure = useLiveResource<ClosureView>('payments_shift_closure', {
    id: shiftID,
    interval: 30000,
  });
  if (closure.error) {
    return <Notice>{closure.error}</Notice>;
  }
  if (!closure.data) {
    return <p className="muted">Consultando el cierre…</p>;
  }
  const view = closure.data;
  const visible = view.expected_visible && !!view.expected && !!view.differences;
  return (
    <section className="cash-panel" aria-label="Resultado del cierre">
      <p className="checkout-result" role="status">
        El turno de {view.shift.register_name} quedó cerrado (
        {serverDateTime(view.closed_at, timezone)}) y ya no admite movimientos.
      </p>
      <PrintNotice outcome={printing} subject="el cierre" />
      <DeclaredCount view={view} />
      {visible && view.expected && <ExpectedFigures expected={view.expected} />}
      {visible && view.differences && <DifferenceFigures differences={view.differences} />}
      {!visible && (
        <p className="muted text-xs">
          El efectivo esperado y las diferencias se consultan con el permiso «
          {permissionName('cash.expected.read')}».
        </p>
      )}
      {view.regularizations.length > 0 && (
        <Accordion
          title="Regularizaciones enlazadas"
          summary={`${view.regularizations.length} registradas en turnos posteriores`}
        >
          <p className="muted text-xs">
            Corrigen movimientos de este turno sin cambiar su diferencia registrada.
          </p>
          <ul className="cash-count-lines">
            {view.regularizations.map((operation) => (
              <li key={operation.id}>
                {serverDateTime(operation.created_at, timezone)} · {operation.concept} ·{' '}
                {currencyAmount(operation.currency, operation.amount)} · {operation.reason} ·{' '}
                {authorizationText(operation)}
              </li>
            ))}
          </ul>
        </Accordion>
      )}
      <Accordion title="Corte definitivo" summary="Vista previa, impresión y reimpresión">
        <CashDocumentActions
          source={{ kind: 'report', id: view.final_report_id }}
          shiftID={view.shift.id}
          can={can}
        />
      </Accordion>
      <ClosureComments view={view} can={can} timezone={timezone} />
    </section>
  );
}
