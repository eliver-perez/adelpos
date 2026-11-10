import { useEffect, useRef, useState } from 'react';
import { OperationalPage } from './pages/restaurant/OperationalPage';
import { WindowControls } from './components/WindowControls';
import { VirtualKeyboard } from './components/VirtualKeyboard';
import { Notice } from './components/Forms';
import { AppToaster } from './components/AppToaster';
import { BrandLogo } from './components/BrandLogo';
import { CatalogPage } from './pages/catalog/CatalogPage';
import { Icon } from './components/Icon';
import { useSession } from './hooks/useSession';
import { useRealtime } from './hooks/useRealtime';
import { KitchenPage } from './pages/kitchen/KitchenPage';
import { AccessFrame, EnrollmentPage, LoginPage, SetupPage } from './pages/AccessPages';
import { AuditPage } from './pages/AuditPage';
import { DevicesPage } from './pages/DevicesPage';
import {
  PasswordPage,
  PreferencesPage,
  homeScreen,
  screenAuthorized,
  screens,
} from './pages/PreferencesPage';
import { PaymentsAdminPage } from './pages/payments/PaymentsAdminPage';
import { CashPage } from './pages/cash/CashPage';
import { PendingOperationsProvider } from './hooks/pendingOperations';
import { RegistersPage } from './pages/RegistersPage';
import { RolesPage } from './pages/RolesPage';
import { SessionsPage } from './pages/SessionsPage';
import { SettingsPage } from './pages/SettingsPage';
import { StatusPage } from './pages/StatusPage';
import { UsersPage } from './pages/UsersPage';
import product from './generated/product.json';
import { dismissNotice, notifyError, notifyWarning } from './services/notify';

const connectionNoticeID = 'connection';

export function App() {
  // El almacén de operaciones inciertas vive por encima de todas las pantallas: navegar no pierde
  // el operation_id de un cobro o una operación de cocina sin resultado confirmado.
  return (
    <PendingOperationsProvider>
      <WindowControls />
      <ApplicationContent />
      <AppToaster />
    </PendingOperationsProvider>
  );
}

function ApplicationContent() {
  const session = useSession();
  const [screen, setScreen] = useState('status');
  const [technical, setTechnical] = useState(false);
  const initializedUser = useRef<string | null>(null);
  const identity = session.identity;
  const realtimeStatus = useRealtime(
    identity?.user.id ?? '',
    !!identity?.user.permissions.includes('events.read'),
  );
  const operationMode = screen === 'restaurant' || screen === 'takeaway';
  const can = (permission: string) => identity?.user.permissions.includes(permission) ?? false;
  const available = screens.filter((entry) => {
    const authorized = screenAuthorized(entry, identity?.user.permissions ?? []);
    return authorized && (entry.id !== 'takeaway' || can('accounts.read'));
  });

  // La pérdida de conexión con una sesión abierta se avisa con un mensaje persistente que se
  // retira solo cuando el servidor vuelve a responder (la cabecera también indica «Sin conexión»).
  const authenticated = session.phase === 'authenticated';
  const connectionError = session.connectionError;
  useEffect(() => {
    if (!authenticated || !connectionError) {
      dismissNotice(connectionNoticeID);
      return;
    }
    notifyWarning(
      `${connectionError} Se reintentará automáticamente; los cambios deben confirmarse por el servidor.`,
      {
        id: connectionNoticeID,
        persistent: true,
      },
    );
  }, [authenticated, connectionError]);

  useEffect(() => {
    if (!identity) {
      initializedUser.current = null;
      return;
    }
    document.documentElement.dataset.theme = identity.preferences.theme;
    if (initializedUser.current !== identity.user.id) {
      setScreen(homeScreen(identity.preferences.home_screen));
      initializedUser.current = identity.user.id;
    }
    const current = screens.find((entry) => entry.id === screen);
    if (current && !screenAuthorized(current, identity.user.permissions)) {
      setScreen('status');
    }
  }, [identity, screen]);

  if (technical && session.phase !== 'authenticated') {
    return (
      <main className="p-8">
        <button className="secondary-button mb-6" onClick={() => setTechnical(false)}>
          Volver al acceso
        </button>
        <StatusPage />
      </main>
    );
  }
  if (
    session.phase === 'loading' ||
    session.phase === 'offline' ||
    session.phase === 'secure-storage'
  ) {
    return (
      <AccessFrame
        title={
          session.phase === 'loading'
            ? 'Conectando'
            : session.phase === 'secure-storage'
              ? 'Revisa el acceso seguro'
              : 'Revisa la conexión'
        }
        description={
          session.phase === 'secure-storage'
            ? 'El sistema operativo debe autorizar la recuperación de tus credenciales locales. Revisa su diálogo de acceso al Llavero.'
            : 'La aplicación consulta el servidor configurado y su estado real.'
        }
      >
        <Notice>{session.connectionError}</Notice>
        <div className="button-row">
          <button className="primary-button" onClick={() => void session.refresh()}>
            Volver a comprobar
          </button>
          <button className="secondary-button" onClick={() => setTechnical(true)}>
            Diagnóstico técnico
          </button>
        </div>
      </AccessFrame>
    );
  }
  if (session.phase === 'setup') {
    return (
      <SetupPage
        available={session.local?.local_setup_available ?? false}
        refresh={session.refresh}
      />
    );
  }
  if (session.phase === 'enroll') {
    return (
      <EnrollmentPage
        message={session.message}
        refresh={session.refresh}
        localAvailable={session.local?.local_enrollment_available}
      />
    );
  }
  if (session.phase === 'login' || !identity) {
    return <LoginPage message={session.message} refresh={session.refresh} />;
  }

  async function logout() {
    try {
      await session.logout();
    } catch (error) {
      notifyError(error, 'No se pudo cerrar la sesión.');
    }
  }

  function content() {
    if (!identity) {
      return null;
    }
    switch (screen) {
      case 'restaurant':
      case 'takeaway':
        return null;
      case 'kitchen':
        return <KitchenPage can={can} timezone={identity.establishment.timezone} />;
      case 'catalog':
        return <CatalogPage can={can} />;
      case 'users':
        return <UsersPage can={can} onChanged={session.refresh} />;
      case 'roles':
        return <RolesPage can={can} onChanged={session.refresh} />;
      case 'devices':
        return <DevicesPage can={can} />;
      case 'registers':
        return <RegistersPage can={can} />;
      case 'payments':
        return <PaymentsAdminPage can={can} timezone={identity.establishment.timezone} />;
      case 'cash':
        return (
          <CashPage
            can={can}
            userID={identity.user.id}
            timezone={identity.establishment.timezone}
          />
        );
      case 'sessions':
        return <SessionsPage can={can} />;
      case 'settings':
        return <SettingsPage can={can} onSaved={session.refresh} />;
      case 'audit':
        return <AuditPage />;
      case 'preferences':
        return <PreferencesPage identity={identity} refresh={session.refresh} />;
      case 'password':
        return <PasswordPage refresh={session.refresh} />;
      default:
        return <StatusPage />;
    }
  }

  return (
    <div className={`app-shell min-h-screen ${operationMode ? 'operation-mode' : ''}`}>
      <aside className="sidebar admin-sidebar">
        <div className="brand-heading">
          <div className="brand-heading-content">
            <BrandLogo />
            <p className="muted text-xs">Administración · v{product.version}</p>
          </div>
        </div>
        <nav aria-label="Navegación principal">
          {available.map((entry) => (
            <button
              key={entry.id}
              className={screen === entry.id ? 'nav-selected' : ''}
              onClick={() => setScreen(entry.id)}
              aria-current={screen === entry.id ? 'page' : undefined}
            >
              {entry.label}
            </button>
          ))}
        </nav>
        <div className="sidebar-user">
          <strong>{identity.user.display_name}</strong>
          <p className="muted text-xs">{identity.user.role_name}</p>
          <button onClick={() => setScreen('password')} className="secondary-button mt-4">
            Cambiar contraseña
          </button>
          <button className="secondary-button mt-2" onClick={() => void logout()}>
            Cerrar sesión
          </button>
        </div>
      </aside>
      <div className="min-w-0">
        <header className="application-header">
          <div>
            <strong>{identity.establishment.name}</strong>
            <p className="muted text-xs">
              {identity.terminal.name} · {identity.establishment.currency}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <Icon name="server" />
            <span className="text-xs">
              {session.connectionError ? 'Sin conexión' : 'Sesión verificada'}
            </span>
            <span className="realtime-status text-xs">
              {!session.connectionError && realtimeStatus}
            </span>
          </div>
        </header>
        <div className="mobile-navigation">
          <select
            aria-label="Pantalla"
            value={screen}
            onChange={(event) => setScreen(event.target.value)}
          >
            {available.map((entry) => (
              <option key={entry.id} value={entry.id}>
                {entry.label}
              </option>
            ))}
            <option value="password">Cambiar contraseña</option>
          </select>
          <button className="secondary-button" onClick={() => void logout()}>
            Salir
          </button>
        </div>
        <main className="admin-main">
          {identity.user.initial_password && (
            <div className="password-warning" role="status">
              <div>
                <strong>Recomendamos cambiar tu contraseña inicial.</strong>
                <p>Puedes seguir utilizando el sistema y cambiarla cuando lo decidas.</p>
              </div>
              <button className="secondary-button" onClick={() => setScreen('password')}>
                Cambiar contraseña
              </button>
            </div>
          )}
          {content()}
          {can('accounts.read') && (
            <div
              className="operation-root-host"
              style={{ display: operationMode ? 'block' : 'none' }}
            >
              <OperationalPage
                timezone={identity.establishment.timezone}
                key={identity.user.id}
                active={operationMode}
                requestedView={screen === 'takeaway' ? 'takeaway' : 'tables'}
                can={can}
                onOpenCash={() => setScreen('cash')}
              />
            </div>
          )}
        </main>
      </div>
      <VirtualKeyboard mode={identity.terminal.keyboard_mode} />
    </div>
  );
}
