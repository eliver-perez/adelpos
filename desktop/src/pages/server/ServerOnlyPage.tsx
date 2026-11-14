import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { BrandLogo } from '../../components/BrandLogo';
import { Notice, PageTitle } from '../../components/Forms';
import { Icon, IconLabel } from '../../components/Icon';
import { StatusCard } from '../../components/StatusCard';
import { WindowControls } from '../../components/WindowControls';
import { useLiveResource } from '../../hooks/useLiveResource';
import { ServerStatus } from '../../navigation/ServerStatus';
import type { ServerState } from '../../navigation/serverState';
import type { ScreenID } from '../../navigation/sections';
import { connectionConfig, getDiagnostics } from '../../services/diagnostics';
import type { Diagnostics } from '../../services/diagnostics';
import { lastBackupCard } from '../../services/backups';
import { terminalsLabel, uptimeLabel } from '../../services/serverService';
import type { ServiceStatus } from '../../services/serverService';
import { AccessFrame } from '../AccessPages';
import { BackupAlertBanner } from '../maintenance/BackupAlert';

// Modalidad «Solo servidor» (etapa 9G, §2.3): una computadora apartada que guarda los datos y atiende a
// las terminales. No abre la interfaz de venta: muestra una pantalla de estado pequeña y, a quien lo
// administra, el acceso a Administración. Go calcula las terminales conectadas (última petición en los
// últimos 2 minutos) y el último respaldo (fase F: si fue correcto y dónde está, con el aviso si falló).

// Estado del servidor visible antes de iniciar sesión (solo lo que dicen las consultas públicas de salud).
export function ServerOnlyAccess({ onLogin }: { onLogin: () => void }) {
  const [diagnostics, setDiagnostics] = useState<Diagnostics | null>(null);
  useEffect(() => {
    let active = true;
    async function check() {
      try {
        const config = await connectionConfig();
        const next = await getDiagnostics(config);
        if (active) {
          setDiagnostics(next);
        }
      } catch {
        if (active) {
          setDiagnostics(null);
        }
      }
    }
    void check();
    const timer = window.setInterval(() => void check(), 15000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, []);
  const ready = diagnostics?.backend === 'connected' && diagnostics.postgres === 'available';
  return (
    <AccessFrame
      title="Servidor del restaurante"
      description="Esta computadora guarda los datos del restaurante y atiende a las terminales. En ella no se vende."
    >
      <p className="server-only-state" role="status">
        <span className={`status-dot ${ready ? 'good' : 'warning'}`} aria-hidden="true" />
        {diagnostics === null
          ? 'Comprobando el servidor…'
          : ready
            ? 'El servidor está activo.'
            : 'El servidor no responde. Si acaba de encender la computadora, espera un momento.'}
      </p>
      <div className="button-row">
        <button type="button" className="primary-button" onClick={onLogin}>
          <IconLabel icon="grid">Entrar a Administración</IconLabel>
        </button>
      </div>
    </AccessFrame>
  );
}

// Barra superior reducida de «Solo servidor»: estado del servidor, Administración (si el usuario puede
// abrirla), el estado de la conexión, el menú del usuario y pantalla completa. Toda navegación pasa por
// onNavigate (el requestScreen de App).
export function ServerOnlyBar({
  screen,
  canAdminister,
  server,
  userMenu,
  onNavigate,
}: {
  screen: ScreenID;
  canAdminister: boolean;
  server: ServerState;
  userMenu: ReactNode;
  onNavigate: (next: ScreenID) => void;
}) {
  const onStatus = screen === 'server-status';
  return (
    <header className="app-topbar server-only-bar">
      <button
        type="button"
        className="topbar-logo"
        aria-label="Estado del servidor"
        onClick={() => onNavigate('server-status')}
      >
        <BrandLogo />
      </button>
      <nav className="topbar-sections" aria-label="Secciones del servidor">
        <button
          type="button"
          className={`topbar-button topbar-section ${onStatus ? 'topbar-section-active' : ''}`}
          aria-current={onStatus ? 'page' : undefined}
          onClick={() => onNavigate('server-status')}
        >
          <Icon name="server" size={20} />
          <span>Estado del servidor</span>
        </button>
        {canAdminister && (
          <button
            type="button"
            className={`topbar-button topbar-section ${!onStatus ? 'topbar-section-active' : ''}`}
            aria-current={!onStatus ? 'page' : undefined}
            onClick={() => onNavigate('administration')}
          >
            <Icon name="grid" size={20} />
            <span>Administración</span>
          </button>
        )}
      </nav>
      <div className="topbar-end">
        <ServerStatus state={server} />
        {userMenu}
        <WindowControls placement="bar" />
      </div>
    </header>
  );
}

// Pantalla de estado de «Solo servidor» después de iniciar sesión.
export function ServerOnlyPage({
  canManage,
  canAdminister,
  onAdministration,
  onMaintenance,
}: {
  // settings.manage: ver las terminales conectadas, el servicio y los respaldos (Go vuelve a exigirlo).
  canManage: boolean;
  canAdminister: boolean;
  onAdministration: () => void;
  // Abre Sistema › Mantenimiento desde el aviso de respaldos.
  onMaintenance?: () => void;
}) {
  const status = useLiveResource<ServiceStatus>('maintenance_service', {
    enabled: canManage,
    interval: 15000,
  });
  const data = status.data;
  // Etapa 9G, fase F: el último respaldo (si fue correcto y dónde está) y el aviso si falló.
  const backup = data
    ? lastBackupCard(data.last_backup)
    : { value: '—', description: '', state: 'neutral' as const };
  return (
    <>
      <PageTitle
        title="Servidor del restaurante"
        description="Esta computadora guarda los datos del restaurante y atiende a las terminales. En ella no se vende."
      />
      {canManage && <BackupAlertBanner onOpen={canAdminister ? onMaintenance : undefined} />}
      <div className="server-only-cards">
        <StatusCard
          icon="server"
          label="Servidor"
          value="Activo"
          state="good"
          description={
            data
              ? `Activo desde hace ${uptimeLabel(data.uptime_seconds)}.`
              : 'Atiende a las terminales del restaurante.'
          }
        />
        <StatusCard
          icon="monitor"
          label="Terminales"
          value={data ? String(data.terminals.connected) : '—'}
          state={data && data.terminals.connected > 0 ? 'good' : 'neutral'}
          description={
            data
              ? terminalsLabel(data)
              : canManage
                ? 'Consultando las terminales…'
                : 'Solo quien administra la configuración ve las terminales conectadas.'
          }
        />
        <StatusCard
          icon="database"
          label="Último respaldo"
          value={backup.value}
          state={backup.state}
          description={
            canManage
              ? backup.description
              : 'Los respaldos los revisa quien administra la configuración.'
          }
        />
      </div>
      <Notice>{status.error}</Notice>
      {canAdminister && (
        <div className="button-row">
          <button type="button" className="primary-button" onClick={onAdministration}>
            <IconLabel icon="grid">Abrir Administración</IconLabel>
          </button>
        </div>
      )}
    </>
  );
}
