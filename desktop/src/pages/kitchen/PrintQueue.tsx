import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import { ReferenceField, TextField } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import { jobLabels, ticketKindLabels } from '../../services/kitchen';
import type { ExecuteKitchen, Printer, PrintJob, TicketDocument } from '../../services/kitchen';
import { serverDateTime } from '../../services/restaurant';

// Confirmación de cada decisión sobre la cola: «enviado a la cola», nunca «impreso».
const queueActionMessages: Record<'resolve' | 'retry' | 'reprint', string> = {
  reprint: 'Copia adicional autorizada y enviada a la cola de impresión.',
  retry: 'Reintento autorizado: el trabajo vuelve a la cola de impresión.',
  resolve: 'Decisión sobre el trabajo de impresión registrada.',
};

export function PrintQueue({
  can,
  execute,
  busy,
  timezone,
}: {
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
  timezone: string;
}) {
  const [state, setState] = useState('');
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<PrintJob | null>(null);
  const [action, setAction] = useState<'resolve' | 'retry' | 'reprint'>('resolve');
  const resource = useLiveResource<PrintJob[]>('kitchen_jobs', {
    query: { state, page: String(page) },
  });
  return (
    <div className="kitchen-editor-layout">
      <section className="panel form-stack">
        <h2>Cola persistente de impresión</h2>
        <p className="muted">
          Aceptado por CUPS o por la conexión de red no confirma que salió papel. Los resultados
          inciertos no se reenvían automáticamente.
        </p>
        <Field label="Filtrar trabajos por estado">
          <select
            value={state}
            onChange={(event) => {
              setState(event.target.value);
              setPage(1);
            }}
          >
            <option value="">Todos los estados</option>
            {Object.entries(jobLabels).map(([key, label]) => (
              <option key={key} value={key}>
                {label}
              </option>
            ))}
          </select>
        </Field>
        <Notice>{resource.error}</Notice>
        {resource.data?.map((job) => (
          <article className={`kitchen-job kitchen-job-${job.state}`} key={job.id}>
            <div className="section-heading">
              <strong>{job.printer_name}</strong>
              <span className="kitchen-state-label">{jobLabels[job.state]}</span>
            </div>
            <p className="muted text-sm">
              {serverDateTime(job.created_at, timezone)} · {job.attempts} intentos
              {job.transport_reference ? ` · ${job.transport_reference}` : ''}
            </p>
            {job.last_error && <p>{job.last_error}</p>}
            <div className="button-row">
              <button
                className="secondary-button"
                onClick={() => {
                  setSelected(job);
                  setAction('resolve');
                }}
              >
                Consultar trabajo e intentos
              </button>
              {(job.state === 'error' || job.state === 'intervention') &&
                can('kitchen.queue.retry') && (
                  <button
                    className="secondary-button"
                    disabled={busy}
                    onClick={() => {
                      setSelected(job);
                      setAction('retry');
                    }}
                  >
                    Reintentar con motivo
                  </button>
                )}
              {(job.state === 'submitted' || job.state === 'cancelled') &&
                can('kitchen.reprint') && (
                  <button
                    className="secondary-button"
                    disabled={busy}
                    onClick={() => {
                      setSelected(job);
                      setAction('reprint');
                    }}
                  >
                    Solicitar otra copia
                  </button>
                )}
            </div>
          </article>
        ))}
        {resource.data?.length === 0 && <p className="muted">No hay trabajos en este filtro.</p>}
        <div className="button-row">
          <button
            className="secondary-button"
            disabled={page === 1}
            onClick={() => setPage(page - 1)}
          >
            Más recientes
          </button>
          <span>Página {page}</span>
          <button
            className="secondary-button"
            disabled={(resource.data?.length ?? 0) < 100}
            onClick={() => setPage(page + 1)}
          >
            Más antiguos
          </button>
        </div>
      </section>
      {selected && (
        <JobEditor
          key={`${selected.id}-${action}`}
          job={selected}
          action={action}
          can={can}
          execute={execute}
          busy={busy}
          timezone={timezone}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

function JobEditor({
  job,
  action,
  can,
  execute,
  busy,
  timezone,
  onClose,
}: {
  job: PrintJob;
  action: 'resolve' | 'retry' | 'reprint';
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
  timezone: string;
  onClose: () => void;
}) {
  const detail = useLiveResource<{
    job: PrintJob;
    attempts: {
      id: string;
      attempt: number;
      outcome: string;
      detail: string;
      started_at: string;
    }[];
  }>('kitchen_job', { id: job.id });
  const document = useLiveResource<TicketDocument>('kitchen_ticket', {
    id: job.ticket_id,
    enabled: can('kitchen.tickets.read'),
  });
  const printers = useLiveResource<Printer[]>('kitchen_printers', {
    enabled: can('kitchen.printers.manage'),
  });
  const [reason, setReason] = useState('');
  const [decision, setDecision] = useState('cancel');
  const [printerID, setPrinterID] = useState('');
  const stale = !!detail.data && detail.data.job.revision !== job.revision;
  const permitted =
    action === 'reprint'
      ? can('kitchen.reprint')
      : action === 'retry'
        ? can('kitchen.queue.retry')
        : can('kitchen.queue.resolve');
  return (
    <section className="panel form-stack">
      <div className="section-heading">
        <h2>
          {action === 'reprint'
            ? 'Copia adicional'
            : action === 'retry'
              ? 'Reintento autorizado'
              : 'Revisión del trabajo'}
        </h2>
        <button className="secondary-button" onClick={onClose}>
          Cerrar revisión
        </button>
      </div>
      <Notice>{detail.error || document.error}</Notice>
      <Notice>
        {stale &&
          'El trabajo cambió. Cierra esta revisión y vuelve a abrir su resultado actual; tu motivo no se ha enviado.'}
      </Notice>
      {job.state === 'uncertain' && (
        <Notice>
          El destino pudo haber recibido el documento. Revisa la cola y el papel antes de autorizar
          otra copia.
        </Notice>
      )}
      {permitted && (
        <Form
          disabled={busy || stale || job.state === 'processing'}
          label={
            action === 'reprint'
              ? 'Autorizar copia adicional'
              : action === 'retry'
                ? 'Autorizar reintento'
                : 'Registrar decisión'
          }
          successMessage={queueActionMessages[action]}
          onSave={async () => {
            const body = { revision: job.revision, reason, printer_id: printerID || null };
            if (action === 'reprint')
              await execute('kitchen_reprint', {
                body: { job_id: job.id, reason, printer_id: printerID || null },
              });
            else
              await execute(action === 'retry' ? 'kitchen_retry_job' : 'kitchen_resolve_job', {
                id: job.id,
                body: action === 'retry' ? body : { ...body, decision },
              });
            onClose();
          }}
        >
          {action === 'resolve' && (
            <Field label="Decisión tras revisar el resultado">
              <select value={decision} onChange={(event) => setDecision(event.target.value)}>
                <option value="cancel">Cancelar administrativamente</option>
                <option value="accepted">Reconocer aceptación conocida del transporte</option>
                <option value="send">Autorizar envío, aceptando posible copia adicional</option>
              </select>
            </Field>
          )}
          <TextField
            label="Motivo de la decisión de impresión"
            required
            value={reason}
            onChange={setReason}
          />
          {can('kitchen.printers.manage') && (
            <ReferenceField
              optional
              label="Redirección expresa (vacío: destino guardado)"
              value={printerID}
              options={printers.data ?? []}
              onChange={setPrinterID}
            />
          )}
        </Form>
      )}
      <Accordion title="Historial de intentos" defaultOpen>
        {detail.data?.attempts.map((attempt) => (
          <div key={attempt.id}>
            <strong>
              Intento {attempt.attempt} · {attempt.outcome}
            </strong>
            <p className="muted">{serverDateTime(attempt.started_at, timezone)}</p>
            <p>{attempt.detail}</p>
          </div>
        ))}
      </Accordion>
      {document.data && (
        <Accordion
          title={`Documento original · ${ticketKindLabels[document.data.kind] ?? 'Documento'}`}
          summary="La copia conserva esta presentación"
        >
          <pre className="kitchen-document">{document.data.preview}</pre>
        </Accordion>
      )}
    </section>
  );
}
