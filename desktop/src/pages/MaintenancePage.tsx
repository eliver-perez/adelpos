import { useRef, useState } from 'react';
import { useConfirmDialog } from '../components/ConfirmDialog';
import { Notice, PageTitle } from '../components/Forms';
import { IconLabel } from '../components/Icon';
import { APIError } from '../services/api';
import {
  hasTestData,
  readTestData,
  removeTestData,
  testDataKindLabels,
  testDataSummary,
} from '../services/maintenance';
import type { TestDataEntry, TestDataPreview, TestDataResult } from '../services/maintenance';
import { errorMessage, notifyError, notifySuccess } from '../services/notify';
import { countLabel } from '../services/plural';

// Mantenimiento del administrador (etapa 9F, §3): «Quitar datos de prueba». La lista la arma Go con la
// firma exacta de cada archivo de prueba; la pantalla solo la muestra, pide la confirmación y avisa el
// resultado con el nombre del respaldo que Go creó antes de quitar nada.

type PreviewGroup = 'remove' | 'deactivate' | 'unlink' | 'keep';

const groups: { key: PreviewGroup; title: string; description: string }[] = [
  {
    key: 'remove',
    title: 'Se eliminará',
    description: 'Datos de prueba sin operaciones registradas.',
  },
  {
    key: 'deactivate',
    title: 'Se desactivará',
    description:
      'Datos de prueba que ya tienen operaciones registradas: se conservan desactivados para no alterar ventas, cortes ni historial.',
  },
  {
    key: 'unlink',
    title: 'Se desvinculará',
    description: 'Configuración del restaurante que apunta a datos de prueba.',
  },
  {
    key: 'keep',
    title: 'Se conserva',
    description: 'Coinciden con un dato de prueba, pero el restaurante los usa.',
  },
];

function EntryList({ title, entries }: { title: string; entries: TestDataEntry[] }) {
  return (
    <ul className="maintenance-list" aria-label={title}>
      {entries.map((entry) => (
        <li key={`${entry.kind}:${entry.id}`} className="maintenance-entry">
          <span className="maintenance-kind">{testDataKindLabels[entry.kind]}</span>
          <strong>{entry.name}</strong>
          <span className="muted">{entry.reason}</span>
        </li>
      ))}
    </ul>
  );
}

function PreviewGroups({ preview }: { preview: TestDataPreview }) {
  return (
    <div className="maintenance-groups">
      {groups
        .filter((group) => preview[group.key].length > 0)
        .map((group) => (
          <section key={group.key} className="maintenance-group" aria-label={group.title}>
            <h3>
              {group.title} ({preview[group.key].length})
            </h3>
            <p className="muted">{group.description}</p>
            <EntryList title={group.title} entries={preview[group.key]} />
          </section>
        ))}
    </div>
  );
}

// Texto de la confirmación con lo que se va a hacer.
function confirmationMessage(preview: TestDataPreview) {
  const counts = [
    countLabel(preview.remove.length, 'dato se elimina', 'datos se eliminan'),
    countLabel(preview.deactivate.length, 'dato se desactiva', 'datos se desactivan'),
    countLabel(preview.unlink.length, 'ajuste se desvincula', 'ajustes se desvinculan'),
  ];
  return (
    <>
      <p>
        Primero se crea un respaldo de la base de datos; si no se puede crear, no se quita nada.
      </p>
      <p>{`${counts.join(', ')}. Eliminar no se puede deshacer salvo restaurando el respaldo.`}</p>
      <p>Las ventas, los pagos, los cortes y los movimientos de inventario no se tocan.</p>
    </>
  );
}

export function MaintenancePage() {
  const [preview, setPreview] = useState<TestDataPreview | null>(null);
  const [result, setResult] = useState<TestDataResult | null>(null);
  const [loadError, setLoadError] = useState('');
  const [reviewing, setReviewing] = useState(false);
  const [removing, setRemoving] = useState(false);
  // El operation_id se conserva ante una respuesta incierta (sin conexión) para no repetir el respaldo ni
  // la operación; solo vale para la misma lista revisada.
  const pending = useRef<{ operationID: string; planID: string } | null>(null);
  const { confirm, dialog } = useConfirmDialog();

  async function review() {
    setReviewing(true);
    setLoadError('');
    try {
      const current = await readTestData();
      setPreview(current);
      if (pending.current && pending.current.planID !== current.plan_id) {
        pending.current = null;
      }
    } catch (error) {
      setPreview(null);
      setLoadError(errorMessage(error, 'No se pudieron revisar los datos de prueba.'));
    } finally {
      setReviewing(false);
    }
  }

  async function remove(current: TestDataPreview) {
    const confirmed = await confirm({
      title: 'Quitar datos de prueba',
      message: confirmationMessage(current),
      confirmLabel: 'Quitar datos de prueba',
      cancelLabel: 'Conservar',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    if (!pending.current || pending.current.planID !== current.plan_id) {
      pending.current = { operationID: crypto.randomUUID(), planID: current.plan_id };
    }
    setRemoving(true);
    try {
      const done = await removeTestData(pending.current.operationID, current.plan_id);
      pending.current = null;
      setResult(done);
      notifySuccess('Datos de prueba quitados.', { description: testDataSummary(done) });
      await review();
    } catch (error) {
      const uncertain = error instanceof APIError && error.code === 'CONNECTION_ERROR';
      if (!uncertain) {
        pending.current = null;
      }
      notifyError(error, 'No se pudieron quitar los datos de prueba.');
      if (error instanceof APIError && error.code === 'TEST_DATA_CHANGED') {
        await review();
      }
    } finally {
      setRemoving(false);
    }
  }

  const busy = reviewing || removing;
  return (
    <>
      <PageTitle
        title="Mantenimiento"
        description="Tareas del administrador para conservar en orden los datos del restaurante."
      />
      <section
        className="surface panel maintenance-panel"
        aria-labelledby="maintenance-test-data-title"
        aria-busy={busy}
      >
        <div className="section-heading">
          <h2 id="maintenance-test-data-title">Quitar datos de prueba</h2>
        </div>
        <p className="muted maintenance-help">
          Busca las impresoras, estaciones, rutas de preparación, productos y categorías que crearon
          las pruebas automáticas, reconocidos por sus datos exactos. Lo que ya tiene operaciones
          registradas se desactiva en lugar de eliminarse, y antes de cambiar algo se crea un
          respaldo.
        </p>
        <div className="button-row">
          <button
            type="button"
            className="primary-button"
            disabled={busy}
            onClick={() => void review()}
          >
            <IconLabel icon="search">Revisar datos de prueba</IconLabel>
          </button>
          {preview && hasTestData(preview) && (
            <button
              type="button"
              className="danger-button"
              disabled={busy}
              onClick={() => void remove(preview)}
            >
              <IconLabel icon="trash">Quitar datos de prueba</IconLabel>
            </button>
          )}
        </div>
        <Notice>{loadError}</Notice>
        {preview && !hasTestData(preview) && (
          <p className="maintenance-empty" role="status">
            No hay datos de prueba.
          </p>
        )}
        {preview && <PreviewGroups preview={preview} />}
        {result?.backup && (
          <p className="muted maintenance-backup" role="status">
            Respaldo creado antes de quitar los datos de prueba: <strong>{result.backup}</strong>
          </p>
        )}
      </section>
      {dialog}
    </>
  );
}
