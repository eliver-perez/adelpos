import { IconLabel } from '../../components/Icon';
import { useLiveResource } from '../../hooks/useLiveResource';
import { alertMessage } from '../../services/backups';
import type { BackupAlert, BackupSummary } from '../../services/backups';

// Aviso persistente de respaldos (etapa 9G, fase F; §7): si el último respaldo falló, si no hay un respaldo correcto
// reciente (atrasado) o si se guardó en la carpeta predeterminada porque la elegida no estaba disponible, quien
// administra la configuración (settings.manage, el mismo permiso de Mantenimiento: es quien puede corregir la carpeta o
// la hora) lo ve en la pantalla de inicio hasta que haya un respaldo correcto posterior. Go decide si hay aviso; no se
// puede cerrar.

// Cada cuánto se vuelve a consultar el aviso (además de al volver a la pantalla y tras un cambio de respaldos).
const alertInterval = 60000;

export function BackupAlertNotice({
  alert,
  onOpen,
}: {
  alert: BackupAlert;
  // Abre Sistema › Mantenimiento (no se ofrece dentro de la propia sección).
  onOpen?: () => void;
}) {
  const message = alertMessage(alert);
  return (
    <div
      className={`backup-warning ${alert.kind === 'fallback' ? '' : 'backup-warning-failed'}`}
      role="alert"
    >
      <div>
        <strong>{message.title}</strong>
        <p>{message.description}</p>
      </div>
      {onOpen && (
        <button type="button" className="secondary-button" onClick={onOpen}>
          <IconLabel icon="database">Ver respaldos</IconLabel>
        </button>
      )}
    </div>
  );
}

// Aviso de la pantalla de inicio: el componente que lo usa decide que el usuario tiene settings.manage (Go lo vuelve
// a exigir).
export function BackupAlertBanner({ onOpen }: { onOpen?: () => void }) {
  const summary = useLiveResource<BackupSummary>('maintenance_backups_status', {
    interval: alertInterval,
  });
  const alert = summary.data?.alert;
  if (!alert) {
    return null;
  }
  return <BackupAlertNotice alert={alert} onOpen={onOpen} />;
}
