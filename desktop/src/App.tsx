import { useEffect, useRef, useState } from 'react';
import { Notice } from './components/Forms';
import { Icon } from './components/Icon';
import { useSession } from './hooks/useSession';
import { AccessFrame, EnrollmentPage, LoginPage, SetupPage } from './pages/AccessPages';
import { AuditPage } from './pages/AuditPage';
import { DevicesPage } from './pages/DevicesPage';
import { PasswordPage, PreferencesPage, screens } from './pages/PreferencesPage';
import { RegistersPage } from './pages/RegistersPage';
import { RolesPage } from './pages/RolesPage';
import { SessionsPage } from './pages/SessionsPage';
import { SettingsPage } from './pages/SettingsPage';
import { StatusPage } from './pages/StatusPage';
import { UsersPage } from './pages/UsersPage';
import product from './generated/product.json';

export function App() {
  const session = useSession();
  const [screen, setScreen] = useState('status');
  const [technical, setTechnical] = useState(false);
  const [actionError, setActionError] = useState('');
  const initializedUser = useRef<string | null>(null);
  const identity = session.identity;
  const can = (permission: string) => identity?.user.permissions.includes(permission) ?? false;
  const available = screens.filter((entry) => !entry.permission || can(entry.permission));

  useEffect(() => {
    if (!identity) {
      initializedUser.current = null;
      return;
    }
    document.documentElement.dataset.theme = identity.preferences.theme;
    if (initializedUser.current !== identity.user.id) {
      setScreen(identity.preferences.home_screen);
      initializedUser.current = identity.user.id;
    }
    const current = screens.find((entry) => entry.id === screen);
    if (current?.permission && !identity.user.permissions.includes(current.permission)) {
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
  if (session.phase === 'loading' || session.phase === 'offline') {
    return (
      <AccessFrame
        title={session.phase === 'loading' ? 'Conectando' : 'Revisa la conexión'}
        description="La aplicación consulta el servidor configurado y su estado real."
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
    setActionError('');
    try {
      await session.logout();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'No se pudo cerrar la sesión.');
    }
  }

  function content() {
    if (!identity) {
      return null;
    }
    switch (screen) {
      case 'users':
        return <UsersPage can={can} onChanged={session.refresh} />;
      case 'roles':
        return <RolesPage can={can} onChanged={session.refresh} />;
      case 'devices':
        return <DevicesPage can={can} />;
      case 'registers':
        return <RegistersPage can={can} />;
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
    <div className="app-shell min-h-screen">
      <aside className="sidebar admin-sidebar">
        <div className="brand-heading">
          <span className="brand-mark">
            A<span>·</span>
          </span>
          <div>
            <strong>{product.name}</strong>
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
          <Notice>
            {session.connectionError &&
              `${session.connectionError} Se reintentará automáticamente; los cambios deben confirmarse por el servidor.`}
          </Notice>
          <Notice>{actionError}</Notice>
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
        </main>
      </div>
    </div>
  );
}
