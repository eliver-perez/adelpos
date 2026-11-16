import { useEffect, useReducer, useRef, useState } from 'react';
import { Field } from '../../components/Forms';
import { Icon, IconLabel } from '../../components/Icon';
import {
  cancelLegacyPreparation,
  chooseLegacyBackup,
  chooseLegacyOutput,
  formatCount,
  passphraseProblem,
  preparationError,
  preparationSteps,
  readerDate,
  sizeText,
  startLegacyPreparation,
  watchLegacyPreparation,
} from '../../services/legacyMigration';
import type {
  ChosenFile,
  LegacySelection,
  PreparationEvent,
  PreparationStep,
} from '../../services/legacyMigration';
import { errorMessage, notifyError, notifySuccess } from '../../services/notify';
import { ProgressBar } from './LegacySummary';

// Paso 1 del asistente (etapa 9H, fase D; §5.1): preparar el paquete con el respaldo del sistema anterior.
// Solo en una computadora con Docker Desktop. Los archivos se eligen con los diálogos nativos y la
// preparación la hace el lector (Rust lanza legacy-prepare y entrega sus eventos por un canal): aquí solo
// se escribe la frase de paso y se ve el avance. La preparación sigue aunque se salga de la pantalla.

type Result = Extract<PreparationEvent, { event: 'result' }>;
type Failure = Extract<PreparationEvent, { event: 'error' }>;

interface PreparationView {
  running: boolean;
  steps: Partial<Record<PreparationStep, 'started' | 'finished'>>;
  progress: { step: PreparationStep; current: number; total: number } | null;
  table: { done: number; total: number } | null;
  result: Result | null;
  error: Failure | null;
}

const idle: PreparationView = {
  running: false,
  steps: {},
  progress: null,
  table: null,
  result: null,
  error: null,
};

type Action = { type: 'start' } | { type: 'event'; event: PreparationEvent } | { type: 'reset' };

// reduce arma la vista con los eventos del lector (los mismos al volver a entrar, con el resumen de Rust).
export function reducePreparation(view: PreparationView, action: Action): PreparationView {
  if (action.type === 'reset') {
    return idle;
  }
  if (action.type === 'start') {
    return { ...idle, running: true };
  }
  const event = action.event;
  switch (event.event) {
    case 'step':
      return {
        ...view,
        running: true,
        steps: { ...view.steps, [event.step]: event.status },
        progress: event.status === 'started' ? null : view.progress,
      };
    case 'progress':
      return { ...view, running: true, progress: event };
    case 'table':
      return { ...view, running: true, table: { done: event.done, total: event.total } };
    case 'result':
      return { ...view, running: false, result: event, error: null };
    case 'error':
      return { ...view, running: false, error: event, result: null };
    default:
      return view;
  }
}

function percentOf(view: PreparationView) {
  if (view.table && view.table.total > 0) {
    return (view.table.done * 100) / view.table.total;
  }
  if (view.progress && view.progress.total > 0) {
    return (view.progress.current * 100) / view.progress.total;
  }
  return 0;
}

function StepList({ view }: { view: PreparationView }) {
  return (
    <ol className="legacy-steps" aria-label="Avance de la preparación">
      {preparationSteps.map(({ step, label }) => {
        const state = view.steps[step];
        return (
          <li key={step} className={`legacy-step legacy-step-${state ?? 'pending'}`}>
            <Icon
              name={state === 'finished' ? 'check' : state === 'started' ? 'clock' : 'arrow'}
              size={16}
            />
            <span>{label}</span>
            <span className="muted">
              {state === 'finished' ? 'Listo' : state === 'started' ? 'En curso' : 'Pendiente'}
            </span>
          </li>
        );
      })}
    </ol>
  );
}

// Tablas del sistema anterior que se muestran en el resumen de la preparación, con su nombre para el restaurante.
const preparedTables: { name: string; label: string }[] = [
  { name: 'productos', label: 'Productos' },
  { name: 'categorias', label: 'Categorías' },
  { name: 'mesas', label: 'Mesas' },
  { name: 'usuarios', label: 'Usuarios' },
  { name: 'ventas', label: 'Ventas' },
  { name: 'pagos', label: 'Pagos' },
  { name: 'cortes', label: 'Cortes' },
];

function PreparationResult({ result }: { result: Result }) {
  const rows = new Map(result.tables.map((table) => [table.name, table.rows]));
  return (
    <div className="legacy-alert legacy-alert-good" role="status">
      <strong>El paquete está listo.</strong>
      <p>
        {result.package_file_name ? `«${result.package_file_name}», ` : ''}
        {sizeText(result.package_bytes)}, cifrado con la frase de paso.
        {result.backup.finished_at ? ` Respaldo del ${readerDate(result.backup.finished_at)}.` : ''}
        {result.sales?.first && result.sales.last
          ? ` Ventas del ${readerDate(result.sales.first)} al ${readerDate(result.sales.last)}.`
          : ''}
      </p>
      <ul className="legacy-counts" aria-label="Conteos del paquete preparado">
        {preparedTables
          .filter((table) => rows.has(table.name))
          .map((table) => (
            <li key={table.name}>
              <strong>{formatCount(rows.get(table.name) ?? 0)}</strong>
              <span>{table.label}</span>
            </li>
          ))}
      </ul>
      <p className="muted text-sm">
        El paquete quedó elegido en el paso 2 para importarlo en esta computadora. También puedes
        llevarlo en una USB a la computadora principal del restaurante e importarlo ahí.
      </p>
      {result.recorded === false && (
        <p className="muted text-sm">
          No se pudo anotar la preparación en el Registro de movimientos.
        </p>
      )}
    </div>
  );
}

function PreparationFailure({ failure }: { failure: Failure }) {
  const { title, message } = preparationError(failure.code, failure.message);
  return (
    <div className="legacy-alert legacy-alert-danger" role="alert">
      <strong>{title}</strong>
      <p>{message}</p>
    </div>
  );
}

function FileLine({
  label,
  file,
  empty,
}: {
  label: string;
  file: ChosenFile | null;
  empty: string;
}) {
  return (
    <p className="legacy-file">
      <span className="muted">{label}</span>
      <strong>{file ? file.file_name : empty}</strong>
      {file?.size !== undefined && <span className="muted">{sizeText(file.size)}</span>}
    </p>
  );
}

export function LegacyPrepareStep({
  selection,
  onSelectionChange,
}: {
  selection: LegacySelection;
  onSelectionChange: () => Promise<void>;
}) {
  const [view, dispatch] = useReducer(reducePreparation, idle);
  const [passphrase, setPassphrase] = useState('');
  const [repeated, setRepeated] = useState('');
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState('');
  const announced = useRef(false);

  // Al entrar se vuelve a escuchar la preparación (si sigue en curso, su avance; si terminó, su resultado).
  useEffect(() => {
    let active = true;
    const onEvent = (event: PreparationEvent) => {
      if (active) {
        dispatch({ type: 'event', event });
      }
    };
    watchLegacyPreparation(onEvent)
      .then((snapshot) => {
        if (!active) {
          return;
        }
        for (const event of snapshot.events) {
          dispatch({ type: 'event', event });
        }
        announced.current = !snapshot.running;
      })
      .catch(() => {
        // Sin puente nativo no hay preparación que mostrar.
      });
    return () => {
      active = false;
    };
  }, []);

  // Al terminar una preparación iniciada aquí se avisa y se actualizan los archivos elegidos (el paquete nuevo).
  const finished = view.result ?? view.error;
  useEffect(() => {
    if (!finished || announced.current) {
      return;
    }
    announced.current = true;
    if (finished.event === 'result') {
      notifySuccess('Paquete preparado.');
    }
    void onSelectionChange();
  }, [finished, onSelectionChange]);

  async function choose(kind: 'backup' | 'output') {
    setBusy(true);
    try {
      const choice = kind === 'backup' ? await chooseLegacyBackup() : await chooseLegacyOutput();
      if (choice.result === 'chosen') {
        await onSelectionChange();
      }
    } catch (error) {
      notifyError(error, 'No se pudo elegir el archivo.');
    } finally {
      setBusy(false);
    }
  }

  async function start() {
    const invalid = passphraseProblem(passphrase, repeated);
    setProblem(invalid);
    if (invalid) {
      return;
    }
    setBusy(true);
    announced.current = false;
    dispatch({ type: 'start' });
    try {
      await startLegacyPreparation(passphrase, (event) => dispatch({ type: 'event', event }));
      setPassphrase('');
      setRepeated('');
    } catch (error) {
      dispatch({ type: 'reset' });
      setProblem(errorMessage(error, 'No se pudo empezar a preparar el paquete.'));
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    try {
      await cancelLegacyPreparation();
    } catch (error) {
      notifyError(error, 'No se pudo cancelar la preparación.');
    }
  }

  const ready = !!selection.backup && !!selection.output && !view.running;
  return (
    <section
      className="surface panel maintenance-panel legacy-step-panel"
      aria-labelledby="legacy-prepare-title"
      aria-busy={view.running || undefined}
    >
      <div className="section-heading">
        <h2 id="legacy-prepare-title">1. Preparar el paquete</h2>
      </div>
      <p className="muted maintenance-help">
        Lee el respaldo del sistema anterior y guarda sus datos en un paquete cifrado. Solo hace
        falta en una computadora con Docker Desktop (gratuito); importar el paquete no lo necesita.
        Si el paquete ya está preparado, pasa al paso 2.
      </p>
      <div className="legacy-files">
        <FileLine label="Respaldo" file={selection.backup} empty="Sin elegir" />
        <button
          type="button"
          className="secondary-button"
          disabled={busy || view.running}
          onClick={() => void choose('backup')}
        >
          <IconLabel icon="search">Elegir el respaldo</IconLabel>
        </button>
        <FileLine label="Guardar el paquete en" file={selection.output} empty="Sin elegir" />
        <button
          type="button"
          className="secondary-button"
          disabled={busy || view.running}
          onClick={() => void choose('output')}
        >
          <IconLabel icon="document">Elegir dónde guardarlo</IconLabel>
        </button>
      </div>
      {!view.running && (
        <form
          className="legacy-passphrase"
          onSubmit={(event) => {
            event.preventDefault();
            void start();
          }}
        >
          <p className="muted text-sm">
            La frase de paso protege los datos reales del restaurante que lleva el paquete. Elige
            una de al menos 8 caracteres y guárdala en un lugar seguro: si se olvida, no se puede
            recuperar y habrá que preparar el paquete otra vez.
          </p>
          <div className="form-grid">
            <Field label="Frase de paso del paquete">
              <input
                type="password"
                autoComplete="new-password"
                maxLength={256}
                value={passphrase}
                onChange={(event) => setPassphrase(event.target.value)}
              />
            </Field>
            <Field label="Escríbela otra vez">
              <input
                type="password"
                autoComplete="new-password"
                maxLength={256}
                value={repeated}
                onChange={(event) => setRepeated(event.target.value)}
              />
            </Field>
          </div>
          {problem && (
            <p className="legacy-problem" role="alert">
              {problem}
            </p>
          )}
          <div className="button-row">
            <button type="submit" className="primary-button" disabled={!ready || busy}>
              <IconLabel icon="database">Preparar el paquete</IconLabel>
            </button>
          </div>
        </form>
      )}
      {(view.running || Object.keys(view.steps).length > 0) && !view.result && !view.error && (
        <div className="legacy-running">
          <StepList view={view} />
          <ProgressBar label="Avance de la lectura" percent={percentOf(view)} />
          <p className="muted text-sm">
            Puedes salir de esta pantalla y seguir usando la aplicación; al volver verás el avance.
          </p>
          <div className="button-row">
            <button type="button" className="secondary-button" onClick={() => void cancel()}>
              Cancelar la preparación
            </button>
          </div>
        </div>
      )}
      {view.result && <PreparationResult result={view.result} />}
      {view.error && <PreparationFailure failure={view.error} />}
    </section>
  );
}
