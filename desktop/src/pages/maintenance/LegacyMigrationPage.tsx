import { useCallback, useEffect, useState } from 'react';
import { Notice, PageTitle } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { APIError } from '../../services/api';
import {
  cleanLegacyLeftovers,
  readLegacyBatch,
  readLegacySelection,
  readLegacyStatus,
} from '../../services/legacyMigration';
import type {
  LegacyBatchDetail,
  LegacyRun,
  LegacySelection,
  LegacyStatus,
} from '../../services/legacyMigration';
import { errorMessage, notifyError } from '../../services/notify';
import { ImportReportView, LegacyImportStep } from './LegacyImportStep';
import { LegacyPrepareStep } from './LegacyPrepareStep';
import { BatchFacts, ValidationReport } from './LegacySummary';

// Asistente de migración del sistema anterior (etapa 9H, fase D; §5): Administración › Sistema ›
// Mantenimiento › «Migrar del sistema anterior», también desde Archivo › «Migrar del sistema anterior…» y
// la paleta. Solo para quien administra la configuración (settings.manage) y solo en la computadora
// principal (Go exige la escucha local). Dos pasos sencillos: preparar el paquete (con Docker Desktop) e
// importarlo; si ya hay un lote importado lo dice al abrir y ofrece reemplazarlo con confirmación.

const emptySelection: LegacySelection = { backup: null, output: null, package: null };

// Última importación que no se completó (para decirlo al volver a entrar).
function lastFailedImport(status: LegacyStatus): LegacyRun | null {
  if (status.active_run) {
    return null;
  }
  const last = status.recent_runs.find((run) => run.kind === 'import');
  return last && (last.status === 'failed' || last.status === 'interrupted') ? last : null;
}

function CurrentBatch({ status }: { status: LegacyStatus }) {
  const batch = status.current_batch;
  const [detail, setDetail] = useState<LegacyBatchDetail | null>(null);
  const [loading, setLoading] = useState(false);
  if (!batch) {
    return null;
  }
  async function show() {
    if (!batch) {
      return;
    }
    setLoading(true);
    try {
      setDetail(await readLegacyBatch(batch.id));
    } catch (error) {
      notifyError(error, 'No se pudo leer el informe del lote.');
    } finally {
      setLoading(false);
    }
  }
  return (
    <section
      className="surface panel maintenance-panel legacy-current"
      aria-labelledby="legacy-current-title"
    >
      <div className="section-heading">
        <h2 id="legacy-current-title">Ya hay datos importados del sistema anterior</h2>
      </div>
      <BatchFacts batch={batch} />
      <p className="muted maintenance-help">
        Para actualizarlos con un respaldo más reciente, prepara o elige otro paquete e impórtalo:
        lo importado antes se reemplaza completo, con confirmación.
      </p>
      {detail?.batch.id === batch.id ? (
        detail.report ? (
          <ImportReportView report={detail.report} batchID={batch.id} />
        ) : (
          detail.validation && <ValidationReport validation={detail.validation} />
        )
      ) : (
        <div className="button-row">
          <button
            type="button"
            className="secondary-button"
            disabled={loading}
            onClick={() => void show()}
          >
            <IconLabel icon="report">Ver el informe</IconLabel>
          </button>
        </div>
      )}
    </section>
  );
}

export function LegacyMigrationPage({ mainComputer }: { mainComputer: boolean }) {
  const [status, setStatus] = useState<LegacyStatus | null>(null);
  const [loadError, setLoadError] = useState('');
  const [localRequired, setLocalRequired] = useState(false);
  const [selection, setSelection] = useState<LegacySelection>(emptySelection);

  const loadStatus = useCallback(async () => {
    try {
      setStatus(await readLegacyStatus());
      setLoadError('');
    } catch (error) {
      if (error instanceof APIError && error.code === 'LOCAL_REQUIRED') {
        setLocalRequired(true);
        return;
      }
      setLoadError(errorMessage(error, 'No se pudo leer el estado de la migración.'));
    }
  }, []);

  const loadSelection = useCallback(async () => {
    try {
      setSelection(await readLegacySelection());
    } catch {
      setSelection(emptySelection);
    }
  }, []);

  useEffect(() => {
    if (!mainComputer) {
      return;
    }
    void loadStatus();
    void loadSelection();
    // Restos de una preparación que terminó a la fuerza: se borran sin esperar para dibujar la pantalla.
    cleanLegacyLeftovers().catch(() => undefined);
  }, [mainComputer, loadStatus, loadSelection]);

  const title = (
    <PageTitle
      title="Migrar del sistema anterior"
      description="Prepara un paquete con el respaldo del sistema anterior e impórtalo con todo su historial. Se puede repetir: cada importación reemplaza completa la anterior."
    />
  );
  if (!mainComputer || localRequired) {
    return (
      <>
        {title}
        <section className="surface panel maintenance-panel">
          <p className="maintenance-help">
            La migración del sistema anterior se hace en la computadora principal del restaurante,
            la que guarda los datos. Ábrela ahí, en Sistema › Mantenimiento.
          </p>
        </section>
      </>
    );
  }
  const failed = status ? lastFailedImport(status) : null;
  return (
    <>
      {title}
      <Notice>{loadError}</Notice>
      {failed && (
        <Notice>
          La última importación no se completó: {failed.error_message || 'se interrumpió.'} No quedó
          nada a medias.
        </Notice>
      )}
      {status && <CurrentBatch status={status} />}
      <LegacyPrepareStep selection={selection} onSelectionChange={loadSelection} />
      <LegacyImportStep
        selection={selection}
        resumeRun={status?.active_run ?? null}
        onSelectionChange={loadSelection}
        onImported={loadStatus}
      />
    </>
  );
}
