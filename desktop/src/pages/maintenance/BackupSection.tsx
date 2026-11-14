import { useRef, useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { IntegerField } from '../../components/catalog/EditorFields';
import { Form, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { SegmentedControl } from '../../components/SegmentedControl';
import { TimeField } from '../../components/TimeField';
import { ToggleButton } from '../../components/ToggleButton';
import { useLiveResource } from '../../hooks/useLiveResource';
import { APIError } from '../../services/api';
import {
  announceBackupChange,
  backupDateTime,
  backupNow,
  chooseBackupDirectory,
  failureLabel,
  fallbackLabel,
  folderLabel,
  kindLabels,
  maximumKeep,
  minimumKeep,
  restoreDefaultBackupDirectory,
  runResult,
  saveBackupSettings,
  sizeLabel,
} from '../../services/backups';
import type { BackupRun, BackupSettings, BackupState } from '../../services/backups';
import { notifyError, notifyInfo, notifySuccess } from '../../services/notify';
import { countLabel } from '../../services/plural';
import { BackupAlertNotice } from './BackupAlert';

// Sistema › Mantenimiento › «Respaldos» (etapa 9G, fase F; §7): respaldos automáticos (activos, hora y cuántos
// conservar), carpeta (elegida con el diálogo nativo, solo en la computadora principal), «Respaldar ahora» y los
// últimos respaldos con fecha, tamaño, si se revisaron y dónde están. Restaurar un respaldo sigue siendo una acción del
// administrador desde la línea de comandos (decisión D7): aquí no hay restauración.

type TimeMode = 'cutoff' | 'custom';

// Mientras hay un respaldo en curso la sección se consulta más seguido para mostrar el resultado.
const idleInterval = 15000;
const runningInterval = 3000;

function ScheduleForm({
  settings,
  onSaved,
}: {
  settings: BackupSettings;
  onSaved: () => Promise<void>;
}) {
  const [enabled, setEnabled] = useState(settings.enabled);
  const [mode, setMode] = useState<TimeMode>(settings.time === null ? 'cutoff' : 'custom');
  const [time, setTime] = useState(settings.time ?? settings.business_day_cutoff);
  const [keep, setKeep] = useState(settings.keep);
  return (
    <Form
      label="Guardar respaldos automáticos"
      successMessage="Respaldos automáticos guardados."
      onSave={async () => {
        try {
          await saveBackupSettings({ enabled, time: mode === 'custom' ? time : null, keep });
          announceBackupChange();
        } finally {
          await onSaved();
        }
      }}
    >
      <div className="backup-schedule">
        <ToggleButton label="Respaldo automático diario" pressed={enabled} onChange={setEnabled} />
        <SegmentedControl
          label="Hora del respaldo automático"
          value={mode}
          options={[
            {
              value: 'cutoff',
              label: `Hora de inicio del día (${settings.business_day_cutoff})`,
            },
            { value: 'custom', label: 'Otra hora' },
          ]}
          onChange={setMode}
        />
        {mode === 'custom' && (
          <TimeField
            label="Hora del respaldo"
            hint="En la zona horaria del restaurante."
            required
            value={time}
            onChange={setTime}
          />
        )}
        <IntegerField
          label="Respaldos automáticos que se conservan"
          min={minimumKeep}
          max={maximumKeep}
          hint={`Los más viejos se borran solos. De ${minimumKeep} a ${maximumKeep}.`}
          value={keep}
          onChange={setKeep}
        />
      </div>
      <p className="muted text-sm" aria-live="polite">
        {enabled
          ? `Cada día a las ${mode === 'custom' ? time : settings.business_day_cutoff} se crea un respaldo y se conservan los últimos ${countLabel(keep, 'respaldo automático', 'respaldos automáticos')}. Si la computadora estaba apagada a esa hora, se hace al encenderla.`
          : 'Sin respaldo automático: solo se crean respaldos con «Respaldar ahora».'}
      </p>
    </Form>
  );
}

function RunItem({ run }: { run: BackupRun }) {
  const size = sizeLabel(run.size_bytes);
  return (
    <li className={`backup-run backup-run-${run.status}`}>
      <div className="backup-run-main">
        <strong>{backupDateTime(run.started_at)}</strong>
        <span className="backup-run-kind">{kindLabels[run.kind]}</span>
        <span className={`backup-run-result backup-run-result-${run.status}`}>
          {runResult(run)}
        </span>
      </div>
      <div className="backup-run-detail muted">
        {run.status === 'failed' && <span>{failureLabel(run.failure_reason)}</span>}
        {run.status === 'succeeded' && (
          <>
            {size && <span>{size}</span>}
            <span>{`En ${folderLabel(run)}`}</span>
            {run.fallback_reason && (
              <span>{`La carpeta elegida ${fallbackLabel(run.fallback_reason)}.`}</span>
            )}
            {run.removed && <span>Se borró al conservar solo los más recientes.</span>}
            {!run.removed && run.available === false && <span>Ya no está en su carpeta.</span>}
          </>
        )}
      </div>
    </li>
  );
}

function scheduleSummary(state: BackupState) {
  if (state.running) {
    return 'Se está creando un respaldo.';
  }
  if (!state.settings.enabled) {
    return 'El respaldo automático diario está desactivado.';
  }
  if (state.pending) {
    return 'El respaldo de hoy está pendiente: se hará en unos minutos.';
  }
  if (state.next_run_at) {
    return `Próximo respaldo automático: ${backupDateTime(state.next_run_at)}.`;
  }
  return '';
}

export function BackupSection() {
  const [running, setRunning] = useState(false);
  const resource = useLiveResource<BackupState>('maintenance_backups', {
    interval: running ? runningInterval : idleInterval,
  });
  const [backingUp, setBackingUp] = useState(false);
  const [choosing, setChoosing] = useState(false);
  // La clave de «Respaldar ahora» se conserva ante una respuesta incierta (sin conexión): reenviarla devuelve el
  // mismo respaldo en lugar de crear otro.
  const pending = useRef<string | null>(null);
  const state = resource.data;
  if (state && state.running !== running) {
    setRunning(state.running);
  }

  async function backup() {
    if (!pending.current) {
      pending.current = crypto.randomUUID();
    }
    setBackingUp(true);
    try {
      const { run } = await backupNow(pending.current);
      pending.current = null;
      if (run.status === 'running') {
        notifyInfo('El respaldo sigue en curso.', {
          description: 'Aparecerá en la lista de respaldos al terminar.',
        });
      } else if (run.status === 'succeeded') {
        notifySuccess('Respaldo creado y revisado.', {
          description: `Se guardó en ${folderLabel(run)}.`,
        });
      } else {
        notifyError(failureLabel(run.failure_reason), 'No se pudo crear el respaldo.');
      }
    } catch (error) {
      const uncertain = error instanceof APIError && error.code === 'CONNECTION_ERROR';
      if (!uncertain) {
        pending.current = null;
      }
      notifyError(error, 'No se pudo crear el respaldo.');
    } finally {
      setBackingUp(false);
      announceBackupChange();
      await resource.reload();
    }
  }

  async function choose() {
    setChoosing(true);
    try {
      const outcome = await chooseBackupDirectory();
      if (outcome.result === 'saved') {
        notifySuccess('Carpeta de los respaldos guardada.', {
          description: `Los respaldos se guardarán en ${folderLabel(outcome.state.settings.directory)}.`,
        });
        announceBackupChange();
      }
    } catch (error) {
      notifyError(error, 'No se pudo guardar la carpeta de los respaldos.');
    } finally {
      setChoosing(false);
      await resource.reload();
    }
  }

  async function restoreDefault() {
    setChoosing(true);
    try {
      await restoreDefaultBackupDirectory();
      notifySuccess('Los respaldos se guardarán en la carpeta predeterminada.');
      announceBackupChange();
    } catch (error) {
      notifyError(error, 'No se pudo cambiar la carpeta de los respaldos.');
    } finally {
      setChoosing(false);
      await resource.reload();
    }
  }

  const busy = backingUp || choosing;
  const settings = state?.settings;
  return (
    <section
      className="surface panel maintenance-panel"
      aria-labelledby="maintenance-backups-title"
      aria-busy={busy}
    >
      <div className="section-heading">
        <h2 id="maintenance-backups-title">Respaldos</h2>
      </div>
      <p className="muted maintenance-help">
        Una copia de toda la base de datos del restaurante. Cada respaldo se revisa al crearse. Para
        restaurar uno, pide ayuda a soporte: se hace desde la computadora principal con el servidor
        detenido.
      </p>
      <Notice>{resource.error}</Notice>
      {state && settings && (
        <>
          {state.alert && <BackupAlertNotice alert={state.alert} />}
          <div className="backup-summary" role="status">
            <p>
              <strong>Último respaldo correcto: </strong>
              {state.last_success
                ? `${backupDateTime(state.last_success.started_at)}, en ${folderLabel(state.last_success)}.`
                : 'todavía no hay ninguno.'}
            </p>
            <p className="muted">{scheduleSummary(state)}</p>
          </div>
          <ScheduleForm
            key={`${settings.enabled}|${settings.time ?? ''}|${settings.keep}|${settings.business_day_cutoff}`}
            settings={settings}
            onSaved={resource.reload}
          />
          <div className="backup-folder">
            <h3>Carpeta de los respaldos</h3>
            <p>
              {settings.directory.custom
                ? `Se guardan en ${folderLabel(settings.directory)}.`
                : 'Se guardan en la carpeta predeterminada de la computadora principal.'}
            </p>
            {settings.directory.custom && !settings.directory.available && (
              <p className="backup-folder-missing">
                Ahora no se encuentra (¿está conectado el disco o la USB?). Mientras falte, los
                respaldos se guardan en la carpeta predeterminada.
              </p>
            )}
            <p className="muted text-sm">
              Puede ser otro disco o una USB. Si no está conectada al momento del respaldo, se usa
              la carpeta predeterminada y aparece un aviso.
            </p>
            {state.directory_change_available ? (
              <div className="button-row">
                <button
                  type="button"
                  className="secondary-button"
                  disabled={busy}
                  onClick={() => void choose()}
                >
                  <IconLabel icon="document">Elegir carpeta</IconLabel>
                </button>
                {settings.directory.custom && (
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={busy}
                    onClick={() => void restoreDefault()}
                  >
                    Usar la carpeta predeterminada
                  </button>
                )}
              </div>
            ) : (
              <p className="muted text-sm">
                La carpeta solo se cambia desde la computadora principal.
              </p>
            )}
          </div>
          <div className="button-row backup-now">
            <button
              type="button"
              className="secondary-button"
              disabled={busy || state.running}
              onClick={() => void backup()}
            >
              <IconLabel icon="database">
                {backingUp || state.running ? 'Respaldando…' : 'Respaldar ahora'}
              </IconLabel>
            </button>
          </div>
          <div className="backup-runs">
            <h3>Últimos respaldos</h3>
            {state.runs.length === 0 ? (
              <p className="muted">Todavía no hay respaldos.</p>
            ) : (
              <ul className="backup-run-list" aria-label="Últimos respaldos">
                {state.runs.map((run) => (
                  <RunItem key={run.id} run={run} />
                ))}
              </ul>
            )}
          </div>
          <Accordion
            title="Información para soporte"
            summary="Carpetas y archivos de los respaldos."
            className="support-information"
          >
            <dl>
              <div className="detail-row">
                <dt>Carpeta predeterminada</dt>
                <dd className="font-mono">{settings.default_directory.path}</dd>
              </div>
              <div className="detail-row">
                <dt>Carpeta elegida</dt>
                <dd className="font-mono">{settings.directory.path || 'Ninguna'}</dd>
              </div>
              {state.last_backup && (
                <>
                  <div className="detail-row">
                    <dt>Último respaldo</dt>
                    <dd className="font-mono">
                      {state.last_backup.file_name ||
                        `${state.last_backup.status} · ${state.last_backup.failure_reason}`}
                    </dd>
                  </div>
                  <div className="detail-row">
                    <dt>Carpeta del último respaldo</dt>
                    <dd className="font-mono">{state.last_backup.directory || 'Ninguna'}</dd>
                  </div>
                </>
              )}
            </dl>
          </Accordion>
        </>
      )}
    </section>
  );
}
