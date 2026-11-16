import { useEffect, useRef, useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { APIError } from '../../services/api';
import {
  analyzeLegacyPackage,
  cancelLegacyUpload,
  cancelLegacyUploadInProgress,
  chooseLegacyPackage,
  importLegacyPackage,
  legacyDateTime,
  passphraseProblem,
  phaseLabel,
  readLegacyRun,
  runFinished,
  saveLegacyReport,
  sizeText,
  summarySentence,
  uploadLegacyPackage,
} from '../../services/legacyMigration';
import type {
  LegacyPreview,
  LegacyReport,
  LegacyRun,
  LegacySelection,
} from '../../services/legacyMigration';
import { errorMessage, notifyError, notifyInfo, notifySuccess } from '../../services/notify';
import { PackageSummary, PreviewStatus, ProgressBar, ValidationReport } from './LegacySummary';

// Paso 2 del asistente (etapa 9H, fase D; §5.2): importar el paquete. Rust sube el archivo a Go por partes
// (React nunca recibe su contenido), Go lo analiza en segundo plano con la frase de paso y devuelve el
// resumen previo; con la confirmación, la importación corre en segundo plano (la aplicación sigue usable y
// se puede salir y volver a entrar) y al terminar muestra el informe con la validación y su PDF.

// La consulta del avance de una corrida mientras está en cola o en curso.
const pollInterval = 800;

type Stage = 'idle' | 'uploading' | 'analyzing' | 'preview' | 'importing' | 'report';

// useRunPolling consulta una corrida hasta que termina.
function useRunPolling(run: LegacyRun | null, onUpdate: (run: LegacyRun) => void) {
  const latest = useRef(onUpdate);
  latest.current = onUpdate;
  const id = run && !runFinished(run) ? run.id : null;
  useEffect(() => {
    if (!id) {
      return;
    }
    let active = true;
    let timer = 0;
    const next = () => {
      timer = window.setTimeout(() => {
        readLegacyRun(id)
          .then((current) => {
            if (!active) {
              return;
            }
            latest.current(current);
            if (!runFinished(current)) {
              next();
            }
          })
          .catch(() => {
            // Sin respuesta (por ejemplo, sin conexión) se vuelve a consultar; la capa de conexión lo avisa.
            if (active) {
              next();
            }
          });
      }, pollInterval);
    };
    next();
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [id]);
}

function RunProgress({ run, label }: { run: LegacyRun; label: string }) {
  return (
    <div className="legacy-running" aria-live="polite">
      <p>
        <strong>{phaseLabel(run.phase)}</strong>
      </p>
      <ProgressBar label={label} percent={run.progress?.percent ?? 0} />
      <p className="muted text-sm">
        La importación corre aparte: puedes salir de esta pantalla y seguir usando la aplicación; al
        volver verás el avance. Si se interrumpe, no deja datos a medias.
      </p>
    </div>
  );
}

export function ImportReportView({ report, batchID }: { report: LegacyReport; batchID: string }) {
  const [saving, setSaving] = useState(false);
  async function download() {
    setSaving(true);
    try {
      const saved = await saveLegacyReport(batchID);
      if (saved.result === 'saved') {
        notifySuccess('Informe guardado.', { description: saved.file_name });
      }
    } catch (error) {
      notifyError(error, 'No se pudo guardar el informe.');
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className="legacy-report">
      <h3>Informe de la importación</h3>
      <p className="legacy-sentence">{summarySentence(report)}</p>
      <ValidationReport validation={report.validation} />
      {report.backup_file && (
        <p className="muted text-sm">
          Antes de importar se hizo un respaldo de la base de datos: {report.backup_file}
        </p>
      )}
      <div className="button-row">
        <button
          type="button"
          className="secondary-button"
          disabled={saving}
          onClick={() => void download()}
        >
          <IconLabel icon="document">Descargar informe (PDF)</IconLabel>
        </button>
      </div>
    </div>
  );
}

export function LegacyImportStep({
  selection,
  resumeRun,
  onSelectionChange,
  onImported,
}: {
  selection: LegacySelection;
  // Corrida en curso al entrar (la de otra visita a esta pantalla).
  resumeRun: LegacyRun | null;
  onSelectionChange: () => Promise<void>;
  onImported: () => Promise<void>;
}) {
  const [stage, setStage] = useState<Stage>('idle');
  const [passphrase, setPassphrase] = useState('');
  const [problem, setProblem] = useState('');
  const [upload, setUpload] = useState<{ id: string; fileName: string } | null>(null);
  const [uploadPercent, setUploadPercent] = useState(0);
  const [analysis, setAnalysis] = useState<LegacyRun | null>(null);
  const [importRun, setImportRun] = useState<LegacyRun | null>(null);
  const { confirm, dialog } = useConfirmDialog();
  const resumed = useRef(false);

  // Al volver a entrar con una corrida en curso se muestra su avance.
  useEffect(() => {
    if (!resumeRun || resumed.current) {
      return;
    }
    resumed.current = true;
    if (resumeRun.kind === 'import') {
      setImportRun(resumeRun);
      setStage('importing');
    } else {
      setAnalysis(resumeRun);
      setUpload({ id: resumeRun.upload_id, fileName: selection.package?.file_name ?? '' });
      setStage('analyzing');
    }
  }, [resumeRun, selection.package]);

  // Lo que se hace con cada estado de una corrida (de la consulta periódica o de la respuesta al pedirla).
  function analysisUpdated(run: LegacyRun) {
    setAnalysis(run);
    if (!runFinished(run)) {
      return;
    }
    if (run.status === 'succeeded') {
      setStage('preview');
      return;
    }
    setStage('idle');
    if (run.error_code === 'LEGACY_UPLOAD_MISSING') {
      setUpload(null);
    }
    setProblem(run.error_message || 'No se pudo revisar el paquete.');
  }

  function importUpdated(run: LegacyRun) {
    setImportRun(run);
    if (!runFinished(run)) {
      setStage('importing');
      return;
    }
    setPassphrase('');
    if (run.status === 'succeeded') {
      setUpload(null);
      setStage('report');
      if (run.with_differences) {
        notifyInfo('Importación terminada con diferencias. Revisa el informe.');
      } else {
        notifySuccess('Importación terminada.');
      }
      void onImported();
      void onSelectionChange();
      return;
    }
    setStage('preview');
    setProblem(run.error_message || 'No se pudo importar el paquete.');
    void onImported();
  }

  useRunPolling(stage === 'analyzing' ? analysis : null, analysisUpdated);
  useRunPolling(stage === 'importing' ? importRun : null, importUpdated);

  async function choose() {
    try {
      const choice = await chooseLegacyPackage();
      if (choice.result !== 'chosen') {
        return;
      }
      await forgetUpload();
      setStage('idle');
      setAnalysis(null);
      setImportRun(null);
      setProblem('');
      await onSelectionChange();
    } catch (error) {
      notifyError(error, 'No se pudo elegir el paquete.');
    }
  }

  // forgetUpload cancela en Go la subida del paquete anterior (si la hay): ya no se va a usar.
  async function forgetUpload() {
    const previous = upload;
    setUpload(null);
    if (previous) {
      try {
        await cancelLegacyUpload(previous.id);
      } catch {
        // Si ya no existe (o se usó), no hay nada que cancelar; vence sola a las 2 horas.
      }
    }
  }

  async function review() {
    const invalid = passphraseProblem(passphrase);
    setProblem(invalid);
    const file = selection.package;
    if (invalid || !file) {
      return;
    }
    let current = upload && upload.fileName === file.file_name ? upload : null;
    try {
      if (!current) {
        setStage('uploading');
        setUploadPercent(0);
        const outcome = await uploadLegacyPackage((progress) => setUploadPercent(progress.percent));
        if (outcome.result === 'cancelled') {
          setStage('idle');
          return;
        }
        current = { id: outcome.upload.id, fileName: file.file_name };
        setUpload(current);
      }
      setStage('analyzing');
      analysisUpdated(await analyzeLegacyPackage(current.id, passphrase));
    } catch (error) {
      setStage('idle');
      if (error instanceof APIError && error.code === 'LEGACY_UPLOAD_MISSING') {
        setUpload(null);
      }
      setProblem(errorMessage(error, 'No se pudo revisar el paquete.'));
    }
  }

  async function startImport(preview: LegacyPreview) {
    if (!analysis || !upload) {
      return;
    }
    const invalid = passphraseProblem(passphrase);
    setProblem(invalid);
    if (invalid) {
      return;
    }
    const replacing = preview.will_replace && preview.current_batch !== null;
    const confirmed = await confirm(
      replacing
        ? {
            title: 'Reemplazar lo importado del sistema anterior',
            message: (
              <>
                <p>
                  Se borra completo el lote importado el{' '}
                  {legacyDateTime(preview.current_batch?.completed_at)} (catálogo, mesas, usuarios e
                  historial) y se importa este paquete en su lugar.
                </p>
                <p>
                  Antes se hace un respaldo de la base de datos. Reemplazar no se puede deshacer
                  salvo restaurando ese respaldo.
                </p>
              </>
            ),
            confirmLabel: 'Reemplazar lo importado',
            cancelLabel: 'Conservar lo importado',
            tone: 'danger',
          }
        : {
            title: 'Importar el paquete',
            message: (
              <>
                <p>{summarySentence(preview)}</p>
                <p>
                  Antes se hace un respaldo de la base de datos. Lo importado se puede reemplazar
                  después con un paquete más reciente.
                </p>
              </>
            ),
            confirmLabel: 'Importar',
            cancelLabel: 'Revisar otra vez',
          },
    );
    if (!confirmed) {
      return;
    }
    try {
      const run = await importLegacyPackage({
        uploadID: upload.id,
        passphrase,
        analysisRunID: analysis.id,
        replaceBatchID: preview.current_batch?.id ?? null,
      });
      setProblem('');
      importUpdated(run);
    } catch (error) {
      if (error instanceof APIError && error.code === 'LEGACY_BATCH_CHANGED') {
        setStage('idle');
      }
      setProblem(errorMessage(error, 'No se pudo empezar la importación.'));
    }
  }

  async function cancelUpload() {
    try {
      await cancelLegacyUploadInProgress();
    } catch (error) {
      notifyError(error, 'No se pudo cancelar la subida.');
    }
  }

  const preview =
    stage === 'preview' && analysis?.summary ? (analysis.summary as LegacyPreview) : null;
  const report =
    stage === 'report' && importRun?.summary && importRun.batch_id
      ? { summary: importRun.summary as LegacyReport, batchID: importRun.batch_id }
      : null;
  const working = stage === 'uploading' || stage === 'analyzing' || stage === 'importing';
  return (
    <section
      className="surface panel maintenance-panel legacy-step-panel"
      aria-labelledby="legacy-import-title"
      aria-busy={working || undefined}
    >
      <div className="section-heading">
        <h2 id="legacy-import-title">2. Importar el paquete</h2>
      </div>
      <p className="muted maintenance-help">
        Elige el paquete preparado (puede estar en una USB) y escribe su frase de paso. Antes de
        importar verás un resumen; nada se guarda hasta que lo confirmes.
      </p>
      <div className="legacy-files">
        <p className="legacy-file">
          <span className="muted">Paquete</span>
          <strong>{selection.package ? selection.package.file_name : 'Sin elegir'}</strong>
          {selection.package?.size !== undefined && (
            <span className="muted">{sizeText(selection.package.size)}</span>
          )}
        </p>
        <button
          type="button"
          className="secondary-button"
          disabled={working}
          onClick={() => void choose()}
        >
          <IconLabel icon="search">
            {selection.package ? 'Elegir otro paquete' : 'Elegir el paquete'}
          </IconLabel>
        </button>
      </div>
      {(stage === 'idle' || stage === 'preview') && selection.package && (
        <form
          className="legacy-passphrase"
          onSubmit={(event) => {
            event.preventDefault();
            if (stage === 'idle') {
              void review();
            }
          }}
        >
          <div className="form-grid">
            <Field label="Frase de paso del paquete">
              <input
                type="password"
                autoComplete="off"
                maxLength={256}
                value={passphrase}
                onChange={(event) => setPassphrase(event.target.value)}
              />
            </Field>
          </div>
          {problem && (
            <p className="legacy-problem" role="alert">
              {problem}
            </p>
          )}
          {stage === 'idle' && (
            <div className="button-row">
              <button type="submit" className="primary-button">
                <IconLabel icon="search">Revisar el paquete</IconLabel>
              </button>
            </div>
          )}
        </form>
      )}
      {stage === 'uploading' && (
        <div className="legacy-running">
          <p>
            <strong>Subiendo el paquete</strong>
          </p>
          <ProgressBar label="Avance de la subida" percent={uploadPercent} />
          <div className="button-row">
            <button type="button" className="secondary-button" onClick={() => void cancelUpload()}>
              Cancelar la subida
            </button>
          </div>
        </div>
      )}
      {stage === 'analyzing' && analysis && (
        <RunProgress run={analysis} label="Avance de la revisión" />
      )}
      {stage === 'analyzing' && !analysis && (
        <p className="muted" role="status">
          Revisando el paquete…
        </p>
      )}
      {preview && (
        <div className="legacy-preview">
          <h3>Resumen previo</h3>
          <PreviewStatus preview={preview} />
          <PackageSummary summary={preview} />
          {preview.clean && (
            <div className="button-row">
              {preview.will_replace ? (
                <button
                  type="button"
                  className="danger-button"
                  onClick={() => void startImport(preview)}
                >
                  <IconLabel icon="alert">Reemplazar lo importado</IconLabel>
                </button>
              ) : (
                <button
                  type="button"
                  className="primary-button"
                  onClick={() => void startImport(preview)}
                >
                  <IconLabel icon="database">Importar</IconLabel>
                </button>
              )}
            </div>
          )}
        </div>
      )}
      {stage === 'importing' && importRun && (
        <RunProgress run={importRun} label="Avance de la importación" />
      )}
      {report && <ImportReportView report={report.summary} batchID={report.batchID} />}
      {dialog}
    </section>
  );
}
