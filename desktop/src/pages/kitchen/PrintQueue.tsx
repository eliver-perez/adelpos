import { useEffect, useRef, useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { ReferenceField, TextField } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import { jobDocumentLabels, jobLabels, ticketKindLabels } from '../../services/kitchen';
import type {
  ExecuteKitchen,
  Printer,
  PrintAttempt,
  PrintJob,
  TicketDocument,
} from '../../services/kitchen';
import { notifySuccess } from '../../services/notify';
import {
  printProblemHint,
  printProblemMessage,
  retryableProblem,
} from '../../services/printProblems';
import { serverDateTime } from '../../services/restaurant';
import { renderCurrentLabel, renderCurrentPermission } from '../documents/rerender';
import { RerenderJob } from './RerenderJob';
import { Select } from '../../components/Select';
import { isWindows } from '../../commands/platform';

type QueueAction = 'resolve' | 'reprint' | 'rerender';

// Confirmación de cada decisión sobre la cola: «enviado a la cola», nunca «impreso».
const queueActionMessages: Record<QueueAction, string> = {
  reprint: 'Copia adicional autorizada y enviada a la cola de impresión.',
  rerender: 'Comanda con la plantilla vigente enviada a la cola de impresión.',
  resolve: 'Decisión sobre el trabajo de impresión registrada.',
};

// Trabajos repetidos de lo mismo (etapa 9F, §4.3): las precuentas de una misma cuenta y las copias o
// reintentos de un mismo documento, seguidos en la página, con la misma impresora y el mismo estado, se
// muestran juntos con «×N». La paginación de Go (100 por página) no cambia.
export interface JobGroup {
  key: string;
  jobs: PrintJob[];
}

function groupKey(job: PrintJob) {
  const documentKey =
    job.ticket_kind === 'precheck' && job.folio
      ? `precheck:${job.folio}`
      : `ticket:${job.ticket_id}`;
  return `${documentKey}|${job.printer_id}|${job.state}|${job.problem ?? ''}`;
}

export function groupJobs(jobs: PrintJob[]): JobGroup[] {
  const groups: JobGroup[] = [];
  for (const job of jobs) {
    const key = groupKey(job);
    const last = groups[groups.length - 1];
    if (last && last.key === key) {
      last.jobs.push(job);
    } else {
      groups.push({ key, jobs: [job] });
    }
  }
  return groups;
}

// Qué documento es, sin identificadores internos: «Precuenta · #12», «Comanda · #7», «Corte parcial #1».
export function jobDocument(job: PrintJob) {
  const kind = jobDocumentLabels[job.ticket_kind ?? ''] ?? 'Documento';
  if (job.document_title) {
    return `${kind} · ${job.document_title}`;
  }
  return job.folio ? `${kind} · #${job.folio}` : kind;
}

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
  const [action, setAction] = useState<QueueAction>('resolve');
  const [expanded, setExpanded] = useState<string | null>(null);
  const resource = useLiveResource<PrintJob[]>('kitchen_jobs', {
    query: { state, page: String(page) },
  });
  const groups = groupJobs(resource.data ?? []);

  function open(job: PrintJob, next: QueueAction) {
    setSelected(job);
    setAction(next);
  }

  // Etapa 9F (decisión D3): un trabajo que nunca salió en papel se reintenta con un toque, sin motivo.
  async function retry(job: PrintJob) {
    try {
      await execute('kitchen_retry_job', {
        id: job.id,
        body: { revision: job.revision, reason: '', printer_id: null },
      });
      notifySuccess('Reintento enviado: el trabajo vuelve a la cola de impresión.');
    } catch {
      // execute ya avisó el rechazo de Go (o dejó el resultado incierto en su panel de recuperación).
    }
  }

  return (
    <div className="kitchen-editor-layout print-queue-layout">
      <section className="panel form-stack print-queue-panel">
        <h2>Cola de impresión</h2>
        {/* Etapa 9E: en Windows acepta la cola de impresión de Windows; en macOS no cambia. */}
        {isWindows() ? (
          <p className="muted">
            «Enviado a la impresora» significa que la cola de impresión de Windows o la impresora de
            red lo aceptó; no confirma que salió papel. Si no se sabe si salió, no se reenvía solo.
          </p>
        ) : (
          <p className="muted">
            «Enviado a la impresora» significa que el sistema de impresión o la impresora de red lo
            aceptó; no confirma que salió papel. Si no se sabe si salió, no se reenvía solo.
          </p>
        )}
        <Field label="Filtrar trabajos por estado">
          <Select
            value={state}
            options={[
              { value: '', label: 'Todos los estados' },
              ...Object.entries(jobLabels).map(([key, label]) => ({ value: key, label })),
            ]}
            onChange={(next) => {
              setState(next);
              setPage(1);
              setExpanded(null);
            }}
          />
        </Field>
        <Notice>{resource.error}</Notice>
        <div className="print-queue-list" role="list" aria-label="Trabajos de impresión">
          {groups.map((group) => (
            <JobCard
              key={group.key + group.jobs[0]!.id}
              group={group}
              expanded={expanded === group.key}
              selectedID={selected?.id ?? null}
              can={can}
              busy={busy}
              timezone={timezone}
              onToggle={() => setExpanded(expanded === group.key ? null : group.key)}
              onOpen={open}
              onRetry={(job) => void retry(job)}
            />
          ))}
          {resource.data?.length === 0 && <p className="muted">No hay trabajos en este filtro.</p>}
        </div>
        <div className="button-row">
          <button
            className="secondary-button"
            disabled={page === 1}
            onClick={() => {
              setPage(page - 1);
              setExpanded(null);
            }}
          >
            Más recientes
          </button>
          <span>Página {page}</span>
          <button
            className="secondary-button"
            disabled={(resource.data?.length ?? 0) < 100}
            onClick={() => {
              setPage(page + 1);
              setExpanded(null);
            }}
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

// Un trabajo (o varios repetidos) de la lista: el documento, la impresora, el estado en palabras del
// restaurante, qué pasó y sus acciones. Sin la referencia técnica del transporte.
function JobCard({
  group,
  expanded,
  selectedID,
  can,
  busy,
  timezone,
  onToggle,
  onOpen,
  onRetry,
}: {
  group: JobGroup;
  expanded: boolean;
  selectedID: string | null;
  can: (key: string) => boolean;
  busy: boolean;
  timezone: string;
  onToggle: () => void;
  onOpen: (job: PrintJob, action: QueueAction) => void;
  onRetry: (job: PrintJob) => void;
}) {
  const job = group.jobs[0]!;
  const count = group.jobs.length;
  const problem = printProblemMessage(job.problem, job.printer_name);
  return (
    <article
      className={`kitchen-job kitchen-job-${job.state} ${selectedID && group.jobs.some((entry) => entry.id === selectedID) ? 'kitchen-job-selected' : ''}`}
      role="listitem"
      aria-label={`${jobDocument(job)} · ${job.printer_name}`}
    >
      <div className="section-heading">
        <strong>
          {jobDocument(job)}
          {count > 1 && <span className="print-queue-count"> ×{count}</span>}
        </strong>
        <span className="kitchen-state-label">{jobLabels[job.state]}</span>
      </div>
      <p className="muted text-sm">
        {job.printer_name} · {serverDateTime(job.created_at, timezone)} ·{' '}
        {job.attempts === 1 ? '1 intento' : `${job.attempts} intentos`}
      </p>
      {problem && (
        <p>
          {problem} {printProblemHint(job.problem, job.printer_name)}
        </p>
      )}
      <div className="button-row">
        <button className="secondary-button" onClick={() => onOpen(job, 'resolve')}>
          Consultar trabajo e intentos
        </button>
        {retryableProblem(job.state) && can('kitchen.queue.retry') && (
          <button className="secondary-button" disabled={busy} onClick={() => onRetry(job)}>
            <IconLabel icon="printer">Reintentar</IconLabel>
          </button>
        )}
        {(job.state === 'submitted' || job.state === 'cancelled') && can('kitchen.reprint') && (
          <button
            className="secondary-button"
            disabled={busy}
            onClick={() => onOpen(job, 'reprint')}
          >
            Solicitar otra copia
          </button>
        )}
        {/* Etapa 9B: otra representación con la plantilla activa, distinta de la copia. */}
        {(job.state === 'submitted' || job.state === 'cancelled') &&
          can('kitchen.reprint') &&
          can(renderCurrentPermission) && (
            <button
              className="secondary-button"
              disabled={busy}
              onClick={() => onOpen(job, 'rerender')}
            >
              {renderCurrentLabel}
            </button>
          )}
        {count > 1 && (
          <button className="secondary-button" aria-expanded={expanded} onClick={onToggle}>
            {expanded ? 'Ocultar los repetidos' : `Ver los ${count} trabajos`}
          </button>
        )}
      </div>
      {expanded && (
        <ol className="print-queue-repeats" aria-label="Trabajos repetidos">
          {group.jobs.map((entry) => (
            <li key={entry.id}>
              <span>
                {serverDateTime(entry.created_at, timezone)} · {jobLabels[entry.state]}
              </span>
              <button className="secondary-button" onClick={() => onOpen(entry, 'resolve')}>
                Consultar
              </button>
            </li>
          ))}
        </ol>
      )}
    </article>
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
  action: QueueAction;
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
  timezone: string;
  onClose: () => void;
}) {
  const detail = useLiveResource<{
    job: PrintJob;
    attempts: PrintAttempt[];
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
  const panel = useRef<HTMLElement>(null);
  const stale = !!detail.data && detail.data.job.revision !== job.revision;
  const permitted =
    action === 'rerender'
      ? false
      : action === 'reprint'
        ? can('kitchen.reprint')
        : can('kitchen.queue.resolve');
  const current = detail.data?.job ?? job;

  // Etapa 9F (§4.3): al elegir un trabajo, el detalle aparece a la vista junto a la lista (o debajo de
  // ella en pantallas angostas), en lugar de quedar arriba de la página sin que se note.
  useEffect(() => {
    panel.current?.scrollIntoView?.({ block: 'nearest' });
  }, []);

  return (
    <section
      ref={panel}
      className="panel form-stack print-queue-detail"
      aria-label="Detalle del trabajo de impresión"
    >
      <div className="section-heading">
        <h2>
          {action === 'rerender'
            ? renderCurrentLabel
            : action === 'reprint'
              ? 'Copia adicional'
              : 'Revisión del trabajo'}
        </h2>
        <button className="secondary-button" onClick={onClose}>
          Cerrar revisión
        </button>
      </div>
      <p className="muted">
        {jobDocument(current)} · {current.printer_name} · {jobLabels[current.state]}
      </p>
      {current.problem && (
        <p>
          {printProblemMessage(current.problem, current.printer_name)}{' '}
          {printProblemHint(current.problem, current.printer_name)}
        </p>
      )}
      <Notice>{detail.error || document.error}</Notice>
      <Notice>
        {stale &&
          'El trabajo cambió. Cierra esta revisión y vuelve a abrir su resultado actual; tu motivo no se ha enviado.'}
      </Notice>
      {job.state === 'uncertain' && (
        <Notice>
          La impresora pudo haber recibido el documento. Revisa el papel antes de autorizar otra
          copia.
        </Notice>
      )}
      {action === 'rerender' && can('kitchen.reprint') && can(renderCurrentPermission) && (
        <RerenderJob
          job={job}
          document={document.data}
          printers={printers.data ?? []}
          canRedirect={can('kitchen.printers.manage')}
          execute={execute}
          busy={busy || stale}
          onClose={onClose}
        />
      )}
      {permitted && (
        <Form
          disabled={busy || stale || job.state === 'processing'}
          label={action === 'reprint' ? 'Autorizar copia adicional' : 'Registrar decisión'}
          successMessage={queueActionMessages[action]}
          onSave={async () => {
            if (action === 'reprint') {
              await execute('kitchen_reprint', {
                body: { job_id: job.id, reason, printer_id: printerID || null },
              });
            } else {
              await execute('kitchen_resolve_job', {
                id: job.id,
                body: { revision: job.revision, reason, printer_id: printerID || null, decision },
              });
            }
            onClose();
          }}
        >
          {action === 'resolve' && (
            <Field label="Decisión tras revisar el resultado">
              <Select
                value={decision}
                options={[
                  { value: 'cancel', label: 'Cancelar administrativamente' },
                  { value: 'accepted', label: 'Reconocer que la impresora lo recibió' },
                  {
                    value: 'send',
                    label: 'Autorizar envío, aceptando una posible copia adicional',
                  },
                ]}
                onChange={setDecision}
              />
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
              label="Enviar a otra impresora (vacío: la misma)"
              value={printerID}
              options={printers.data ?? []}
              onChange={setPrinterID}
            />
          )}
        </Form>
      )}
      <Accordion title="Historial de intentos" defaultOpen>
        {detail.data?.attempts.length === 0 && <p className="muted">Todavía no hay intentos.</p>}
        {detail.data?.attempts.map((attempt) => (
          <div key={attempt.id} className="print-queue-attempt">
            <strong>
              Intento {attempt.attempt} ·{' '}
              {attempt.outcome
                ? (jobLabels[attempt.outcome as PrintJob['state']] ?? 'Sin resultado')
                : 'En proceso'}
            </strong>
            <p className="muted">{serverDateTime(attempt.started_at, timezone)}</p>
            {attempt.outcome && attempt.outcome !== 'submitted' && (
              <p>
                {printProblemMessage(
                  attempt.result?.code ??
                    (attempt.outcome === 'error'
                      ? 'unreachable'
                      : attempt.outcome === 'uncertain'
                        ? 'uncertain'
                        : 'setup'),
                  current.printer_name,
                )}
              </p>
            )}
          </div>
        ))}
      </Accordion>
      {document.data && (
        <Accordion
          title={`Documento original · ${ticketKindLabels[document.data.kind] ?? jobDocumentLabels[document.data.kind] ?? 'Documento'}`}
          summary="La copia conserva esta presentación"
        >
          <pre className="kitchen-document">{document.data.preview}</pre>
        </Accordion>
      )}
      {/* Datos técnicos solo para soporte (etapa 9F, §4.3): la referencia del sistema de impresión y
          el mensaje exacto de cada intento. */}
      <Accordion title="Información para soporte">
        <dl className="print-queue-support">
          <dt>Referencia del sistema de impresión</dt>
          <dd>{current.transport_reference || 'Sin referencia'}</dd>
          <dt>Último mensaje</dt>
          <dd>{current.last_error || 'Sin mensaje'}</dd>
          {detail.data?.attempts.map((attempt) => (
            <div key={attempt.id}>
              <dt>Intento {attempt.attempt}</dt>
              <dd>
                {attempt.outcome ?? 'en proceso'} · {attempt.detail || 'Sin detalle'}
              </dd>
            </div>
          ))}
        </dl>
      </Accordion>
    </section>
  );
}
