import { useCallback, useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { OperationalPage } from './pages/restaurant/OperationalPage';
import { WindowControls } from './components/WindowControls';
import { VirtualKeyboard } from './components/VirtualKeyboard';
import { Notice } from './components/Forms';
import { AppToaster } from './components/AppToaster';
import { CatalogPage } from './pages/catalog/CatalogPage';
import { useSession } from './hooks/useSession';
import { useRealtime } from './hooks/useRealtime';
import type { RestaurantView } from './hooks/useRestaurantWorkspace';
import { KitchenPage } from './pages/kitchen/KitchenPage';
import { AccessFrame, EnrollmentPage, LoginPage, SetupPage } from './pages/AccessPages';
import { AuditPage } from './pages/AuditPage';
import { MaintenancePage } from './pages/MaintenancePage';
import { DevicesPage } from './pages/DevicesPage';
import { RepairTerminalPage, useRepairTerminal } from './pages/setup/RepairTerminal';
import { PasswordPage, PreferencesPage } from './pages/PreferencesPage';
import { PaymentsAdminPage } from './pages/payments/PaymentsAdminPage';
import { CashPage } from './pages/cash/CashPage';
import { CashShortcutSheet, shortcutOptions, useCashShortcut } from './pages/cash/CashShortcut';
import { InventoryPage } from './pages/inventory/InventoryPage';
import { ReportsPage } from './pages/reports/ReportsPage';
import { DocumentsPage } from './pages/documents/DocumentsPage';
import { useConfirmDialog } from './components/ConfirmDialog';
import type { CashOptionState } from './commands/actions';
import type { OperationCommandHandlers, OperationCommandState } from './commands/operationBridge';
import { useCommandCenter } from './commands/useCommandCenter';
import { leaveWarning } from './hooks/useLeaveGuard';
import { PendingOperationsProvider } from './hooks/pendingOperations';
import { RegistersPage } from './pages/RegistersPage';
import { RolesPage } from './pages/RolesPage';
import { SessionsPage } from './pages/SessionsPage';
import { SettingsPage } from './pages/SettingsPage';
import { StatusPage } from './pages/StatusPage';
import { UsersPage } from './pages/UsersPage';
import { Breadcrumb } from './navigation/Breadcrumb';
import { NavigationContext } from './navigation/NavigationContext';
import type { NavigationValue } from './navigation/NavigationContext';
import { ServerStatus } from './navigation/ServerStatus';
import { serverState } from './navigation/serverState';
import { TileScreen } from './navigation/TileScreen';
import { TopBar } from './navigation/TopBar';
import { UserMenu } from './navigation/UserMenu';
import { barHighlight, canOpen, resolveHome, sectionByID } from './navigation/sections';
import type { ScreenID } from './navigation/sections';
import { installationError, readInstallation } from './services/installation';
import type { InstallationState } from './services/installation';
import { ModalityChoicePage } from './pages/setup/ModalityChoicePage';
import { FindServerPage } from './pages/setup/FindServerPage';
import { ServerOnlyAccess, ServerOnlyBar, ServerOnlyPage } from './pages/server/ServerOnlyPage';
import { dismissNotice, notifyError, notifySuccess, notifyWarning } from './services/notify';
import { savePreferences } from './services/preferences';
import { setPrecheckName } from './services/documentNames';
import { registerPrinterSetup } from './services/printProblems';
import { requestPaymentPolicies } from './pages/payments/policiesRequest';

const connectionNoticeID = 'connection';

export function App() {
  // El almacén de operaciones inciertas vive por encima de todas las pantallas: navegar no pierde
  // el operation_id de un cobro o una operación de cocina sin resultado confirmado.
  return (
    <PendingOperationsProvider>
      <InstallationGate />
      <AppToaster />
    </PendingOperationsProvider>
  );
}

// Etapa 9G (§2.3): antes de conectarse se resuelve la modalidad de esta computadora. Sin elegirla (primer
// arranque en macOS) se pregunta; «Solo terminal» sin servidor pasa a «Buscar el servidor» (las fases B
// y C agregan ahí la búsqueda y el emparejamiento); con servidor se conecta al de esta computadora y
// «Solo servidor» muestra la pantalla de estado en lugar de la interfaz de venta.
function InstallationGate() {
  const [state, setState] = useState<InstallationState | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    readInstallation()
      .then((value) => {
        if (active) {
          setState(value);
        }
      })
      .catch((problem: unknown) => {
        if (active) {
          setError(installationError(problem));
        }
      });
    return () => {
      active = false;
    };
  }, []);
  if (error) {
    return (
      <AccessShell overlay={null}>
        <AccessFrame
          title="Revisa la instalación"
          description="No se pudo leer cómo se usa esta computadora."
        >
          <Notice>{error}</Notice>
        </AccessFrame>
      </AccessShell>
    );
  }
  if (!state) {
    return null;
  }
  if (!state.chosen) {
    return (
      <AccessShell overlay={null}>
        <ModalityChoicePage state={state} />
      </AccessShell>
    );
  }
  if (state.mode === 'terminal-only' && !state.server_configured) {
    return (
      <AccessShell overlay={null}>
        <FindServerPage state={state} />
      </AccessShell>
    );
  }
  return (
    <ApplicationContent serverOnly={state.mode === 'server-only'} paired={state.paired ?? false} />
  );
}

// Pantallas de acceso y del asistente: sin barra superior, con el control de pantalla completa fijo
// arriba a la derecha (como en las etapas anteriores). «Atajos de teclado» y «Acerca de AD-EL POS»
// del menú nativo también se abren aquí (overlay).
function AccessShell({ children, overlay }: { children: ReactNode; overlay: ReactNode }) {
  return (
    <>
      <WindowControls placement="floating" />
      {children}
      {overlay}
    </>
  );
}

function isOperation(screen: ScreenID) {
  return screen === 'restaurant' || screen === 'takeaway';
}

// paired (etapa 9G, fase B): terminal emparejada con la computadora principal; si pierde su autorización
// o el servidor cambia de identidad, se ofrece emparejarla de nuevo.
function ApplicationContent({ serverOnly, paired }: { serverOnly: boolean; paired: boolean }) {
  const session = useSession();
  const repairTerminal = useRepairTerminal();
  // Pantalla visible. Al iniciar sesión se sustituye por el inicio de la terminal (en «Solo servidor»,
  // el estado del servidor).
  const [screen, setScreen] = useState<ScreenID>(serverOnly ? 'server-status' : 'restaurant');
  // «Solo servidor» (etapa 9G): antes de iniciar sesión se ve el estado del servidor; el acceso aparece
  // al pedir «Entrar a Administración».
  const [serverLogin, setServerLogin] = useState(false);
  // Cada elección de Mesas o Para llevar en la barra vuelve a la vista de esa sección.
  const [operationRequest, setOperationRequest] = useState(0);
  // Vista operativa visible y captura abierta (las informa OperationalPage).
  const [operationView, setOperationView] = useState<RestaurantView>('tables');
  const [captureActive, setCaptureActive] = useState(false);
  const [tail, setTailState] = useState<{ screen: ScreenID; label: string } | null>(null);
  const [technical, setTechnical] = useState(false);
  // Contexto y botones de la pantalla operativa para el registro de acciones (etapa 9C, fase C).
  const [operationCommands, setOperationCommands] = useState<OperationCommandState | null>(null);
  const operationHandlers = useRef<OperationCommandHandlers | null>(null);
  const initializedUser = useRef<string | null>(null);
  const { confirm, dialog: leaveDialog } = useConfirmDialog();
  const identity = session.identity;
  const permissions = identity?.user.permissions ?? [];
  const realtimeEnabled = permissions.includes('events.read');
  const realtimeStatus = useRealtime(identity?.user.id ?? '', realtimeEnabled);
  const operationMode = isOperation(screen);
  const can = (permission: string) => identity?.user.permissions.includes(permission) ?? false;
  const authenticated = session.phase === 'authenticated';
  const home = serverOnly
    ? 'server-status'
    : resolveHome(identity?.terminal.home_screen, permissions);
  // Atajo «Caja» de la barra superior (etapa 9C): el gancho y la hoja de la 7.1 suben aquí para
  // que el botón Caja muestre el turno en todas las pantallas con una sola consulta.
  const cashShortcut = useCashShortcut({
    active: authenticated && !!identity && !serverOnly,
    can,
    onOpenCash: () => void requestScreen('cash'),
  });
  // Opciones del menú rápido de Caja que el menú nativo, la paleta y los atajos ofrecen igual.
  const cashOptions: CashOptionState[] = cashShortcut.visible
    ? shortcutOptions(cashShortcut, can).map((option) => ({
        id: option.id,
        disabled: option.disabled,
      }))
    : [];
  // Menú nativo, atajos y paleta (etapa 9C, fase C): ejecutan las mismas funciones de los botones. En
  // «Solo servidor» no hay interfaz de venta: los atajos de las secciones y la paleta no se ofrecen.
  const commands = useCommandCenter({
    authenticated: authenticated && !!identity && !serverOnly,
    permissions,
    screen: authenticated && identity && !serverOnly ? screen : null,
    operation: can('accounts.read') && !serverOnly ? operationCommands : null,
    operationHandlers,
    cashOptions,
    requestScreen,
    openCashSheet: (sheet) => cashShortcut.openSheet(sheet),
    logout,
    toggleTheme,
  });

  // Etapa 9F (§4.5): el nombre visible de la precuenta llega con la sesión y lo leen los botones, los
  // avisos, la paleta y el menú nativo.
  const precheckNameKey = identity?.establishment.precheck_name;
  useEffect(() => {
    setPrecheckName(precheckNameKey);
  }, [precheckNameKey]);

  // Etapa 9F (§4.1): «Configurar impresora» de los avisos sin impresora lleva a Cobros › Políticas de
  // cobro con el único requestScreen, solo si el usuario puede elegir allí la impresora de tickets.
  const requestScreenRef = useRef(requestScreen);
  requestScreenRef.current = requestScreen;
  const printerSetupAllowed =
    canOpen('payments', permissions) &&
    permissions.includes('payments.settings.manage') &&
    permissions.includes('kitchen.printers.manage');
  useEffect(() => {
    registerPrinterSetup(
      printerSetupAllowed
        ? () => {
            requestPaymentPolicies();
            void requestScreenRef.current('payments');
          }
        : null,
    );
    return () => registerPrinterSetup(null);
  }, [printerSetupAllowed]);

  const setTail = useCallback((owner: ScreenID, label: string | null) => {
    setTailState((current) => {
      if (label === null) {
        return current?.screen === owner ? null : current;
      }
      if (current?.screen === owner && current.label === label) {
        return current;
      }
      return { screen: owner, label };
    });
  }, []);

  // La pérdida de conexión con una sesión abierta se avisa con un mensaje persistente que se
  // retira solo cuando el servidor vuelve a responder (la barra también indica «Sin conexión»).
  const connectionError = session.connectionError;
  useEffect(() => {
    if (!authenticated || !connectionError) {
      dismissNotice(connectionNoticeID);
      return;
    }
    notifyWarning(
      `${connectionError} Se reintenta automáticamente; espera a que vuelva la conexión para guardar cambios.`,
      {
        id: connectionNoticeID,
        persistent: true,
      },
    );
  }, [authenticated, connectionError]);

  // Inicio por terminal (§1.5 y §4.1): al iniciar sesión se abre la pantalla de la terminal (o la
  // primera sección permitida), nunca «Estado del sistema». Si la pantalla visible deja de estar
  // permitida, se vuelve al inicio.
  useEffect(() => {
    if (!identity) {
      initializedUser.current = null;
      return;
    }
    document.documentElement.dataset.theme = identity.preferences.theme;
    const start = serverOnly
      ? 'server-status'
      : resolveHome(identity.terminal.home_screen, identity.user.permissions);
    if (initializedUser.current !== identity.user.id) {
      initializedUser.current = identity.user.id;
      setScreen(start);
      setOperationRequest((current) => current + 1);
      return;
    }
    if (!canOpen(screen, identity.user.permissions) || (serverOnly && isOperation(screen))) {
      setScreen(start);
    }
  }, [identity, screen, serverOnly]);

  // Navegación pedida por la persona: si una pantalla tiene cambios sin guardar (editor de
  // plantillas, etapa 9B), se confirma antes de salir de ella.
  async function confirmLeave() {
    const warning = leaveWarning();
    if (!warning) {
      return true;
    }
    return confirm({
      title: 'Salir sin guardar',
      message: <p>{warning} Si sales ahora, se descartan.</p>,
      confirmLabel: 'Salir sin guardar',
      cancelLabel: 'Seguir editando',
      tone: 'danger',
    });
  }

  // Único punto de navegación (etapa 9C): barra superior, logotipo, mosaicos, ruta, menú del
  // usuario, «Ir a Caja», Inventarios → Configuración y, desde la fase C, el menú nativo, la paleta
  // y los atajos. Sin cambios pendientes la navegación es inmediata; con ellos espera la
  // confirmación. Devuelve si se navegó (las acciones que siguen en la pantalla nueva lo esperan).
  async function requestScreen(next: ScreenID): Promise<boolean> {
    // «Solo servidor» no abre la interfaz de venta (Mesas, Para llevar, Cocina ni Caja).
    if (serverOnly && sectionByID(next).group === 'operation') {
      return false;
    }
    const go = () => {
      setScreen(next);
      if (isOperation(next)) {
        setOperationRequest((current) => current + 1);
      }
    };
    if (next === screen || !leaveWarning()) {
      go();
      return true;
    }
    const confirmed = await confirmLeave();
    if (confirmed) {
      go();
    }
    return confirmed;
  }

  async function logout() {
    if (leaveWarning() && !(await confirmLeave())) {
      return;
    }
    try {
      await session.logout();
      setServerLogin(false);
    } catch (error) {
      notifyError(error, 'No se pudo cerrar la sesión.');
    }
  }

  // «Tema claro u oscuro» del menú nativo y la paleta: guarda el tema con la misma operación de
  // Mis preferencias y lo aplica al recargar la identidad.
  async function toggleTheme() {
    if (!identity) {
      return;
    }
    const theme = identity.preferences.theme === 'dark' ? 'light' : 'dark';
    try {
      await savePreferences({ theme, home_screen: identity.preferences.home_screen });
      await session.refresh();
      notifySuccess(
        theme === 'dark'
          ? 'Tema oscuro guardado en tus preferencias.'
          : 'Tema claro guardado en tus preferencias.',
      );
    } catch (error) {
      notifyError(error, 'No se pudo cambiar el tema.');
    }
  }

  if (technical && session.phase !== 'authenticated') {
    return (
      <AccessShell overlay={commands.overlay}>
        <main className="p-8">
          <button className="secondary-button mb-6" onClick={() => setTechnical(false)}>
            Volver al acceso
          </button>
          {/* Antes de iniciar sesión no se sabe quién usa la terminal: el diagnóstico conserva su
              función con el mismo lenguaje sencillo, sin «Información para soporte», que §4.1
              reserva al administrador (revisión 9C). */}
          <StatusPage />
        </main>
      </AccessShell>
    );
  }
  if (
    session.phase === 'loading' ||
    session.phase === 'offline' ||
    session.phase === 'secure-storage'
  ) {
    return (
      <AccessShell overlay={commands.overlay}>
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
              : 'La terminal se comunica con el equipo servidor del restaurante.'
          }
        >
          <Notice>{session.connectionError}</Notice>
          <Notice>{repairTerminal.error}</Notice>
          <div className="button-row">
            <button className="primary-button" onClick={() => void session.refresh()}>
              Volver a comprobar
            </button>
            <button className="secondary-button" onClick={() => setTechnical(true)}>
              Ver estado del sistema
            </button>
            {/* Etapa 9G: una terminal emparejada cuyo servidor cambió de identidad (o desapareció)
                puede volver a emparejarse; nunca se acepta otro certificado en silencio. */}
            {paired && session.phase === 'offline' && (
              <button
                className="secondary-button"
                disabled={repairTerminal.busy}
                onClick={() => void repairTerminal.repair()}
              >
                Emparejar de nuevo
              </button>
            )}
          </div>
          {repairTerminal.dialog}
        </AccessFrame>
      </AccessShell>
    );
  }
  if (session.phase === 'setup') {
    return (
      <AccessShell overlay={commands.overlay}>
        <SetupPage
          available={session.local?.local_setup_available ?? false}
          refresh={session.refresh}
        />
      </AccessShell>
    );
  }
  // Etapa 9G: una terminal emparejada sin autorización (revocada) se empareja de nuevo con código.
  if (session.phase === 'enroll' && (paired || session.local?.paired)) {
    return (
      <AccessShell overlay={commands.overlay}>
        <RepairTerminalPage message={session.message} />
      </AccessShell>
    );
  }
  if (session.phase === 'enroll') {
    return (
      <AccessShell overlay={commands.overlay}>
        <EnrollmentPage
          message={session.message}
          refresh={session.refresh}
          localAvailable={session.local?.local_enrollment_available}
        />
      </AccessShell>
    );
  }
  if (serverOnly && session.phase === 'login' && !serverLogin) {
    return (
      <AccessShell overlay={commands.overlay}>
        <ServerOnlyAccess onLogin={() => setServerLogin(true)} />
      </AccessShell>
    );
  }
  if (session.phase === 'login' || !identity) {
    return (
      <AccessShell overlay={commands.overlay}>
        <LoginPage message={session.message} refresh={session.refresh} />
      </AccessShell>
    );
  }

  function content() {
    if (!identity) {
      return null;
    }
    switch (screen) {
      case 'restaurant':
      case 'takeaway':
        return null;
      case 'administration':
      case 'staff':
      case 'system':
        return (
          <TileScreen
            parent={screen}
            permissions={identity.user.permissions}
            onNavigate={requestScreen}
          />
        );
      case 'kitchen':
        return (
          <KitchenPage
            can={can}
            timezone={identity.establishment.timezone}
            connectionLost={server === 'offline'}
            onRetryConnection={() => void session.refresh()}
          />
        );
      case 'catalog':
        return <CatalogPage can={can} />;
      case 'users':
        return <UsersPage can={can} onChanged={session.refresh} />;
      case 'roles':
        return <RolesPage can={can} onChanged={session.refresh} />;
      case 'devices':
        return <DevicesPage can={can} onSaved={session.refresh} />;
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
      case 'inventory':
        return (
          <InventoryPage
            can={can}
            timezone={identity.establishment.timezone}
            onOpenSettings={() => requestScreen('settings')}
          />
        );
      case 'documents':
        return (
          <DocumentsPage
            permissions={identity.user.permissions}
            timezone={identity.establishment.timezone}
            onOpenSettings={() => requestScreen('settings')}
          />
        );
      case 'reports':
        return (
          <ReportsPage
            can={can}
            timezone={identity.establishment.timezone}
            permissions={identity.user.permissions}
          />
        );
      case 'sessions':
        return <SessionsPage can={can} />;
      case 'settings':
        return (
          <SettingsPage
            can={can}
            onSaved={session.refresh}
            onOpenStatus={() => requestScreen('status')}
          />
        );
      case 'status':
        // «Información para soporte» solo para el administrador: quien administra la
        // configuración general del establecimiento.
        return <StatusPage support={can('settings.manage')} />;
      case 'audit':
        return <AuditPage />;
      case 'maintenance':
        return <MaintenancePage />;
      case 'preferences':
        return <PreferencesPage identity={identity} refresh={session.refresh} />;
      case 'password':
        return <PasswordPage refresh={session.refresh} />;
      default:
        return null;
    }
  }

  const server = serverState(session.connectionError, realtimeEnabled, realtimeStatus);
  // En la operación la barra resalta la vista visible (Mesas o Para llevar).
  const highlight = operationMode
    ? operationView === 'takeaway'
      ? 'takeaway'
      : 'restaurant'
    : barHighlight(screen);
  // Durante la captura (§1.2) la barra superior se reduce a la de la captura: regreso, mesa y
  // cuenta, Caja y el usuario.
  const reducedBar = operationMode && captureActive;
  const userMenu = (compact: boolean) => (
    <UserMenu
      name={identity.user.display_name}
      role={identity.user.role_name}
      compact={compact}
      onPreferences={() => requestScreen('preferences')}
      onPassword={() => requestScreen('password')}
      onLogout={() => void logout()}
    />
  );
  const navigation: NavigationValue = { screen, home, requestScreen, setTail };

  // «Solo servidor» (etapa 9G, §2.3): barra reducida, pantalla de estado y, para quien la puede abrir,
  // Administración; nunca la interfaz de venta.
  if (serverOnly) {
    return (
      <NavigationContext.Provider value={navigation}>
        <div className="app-shell min-h-screen server-only-shell">
          <ServerOnlyBar
            screen={screen}
            canAdminister={canOpen('administration', identity.user.permissions)}
            server={server}
            userMenu={userMenu(false)}
            onNavigate={(next) => void requestScreen(next)}
          />
          <main className="admin-main">
            <Breadcrumb
              screen={screen}
              tail={tail?.screen === screen ? tail.label : null}
              onNavigate={requestScreen}
            />
            {screen === 'server-status' ? (
              <ServerOnlyPage
                canManage={can('settings.manage')}
                canAdminister={canOpen('administration', identity.user.permissions)}
                onAdministration={() => void requestScreen('administration')}
              />
            ) : (
              content()
            )}
          </main>
          <VirtualKeyboard mode={identity.terminal.keyboard_mode} />
          {commands.overlay}
          {leaveDialog}
        </div>
      </NavigationContext.Provider>
    );
  }

  return (
    <NavigationContext.Provider value={navigation}>
      <div
        className={`app-shell min-h-screen ${operationMode ? 'operation-mode' : ''} ${
          reducedBar ? 'capture-active' : ''
        }`}
      >
        {!reducedBar && (
          <TopBar
            permissions={identity.user.permissions}
            highlight={highlight}
            home={home}
            onNavigate={requestScreen}
            cashShortcut={cashShortcut}
            can={can}
            server={server}
            userMenu={userMenu(false)}
          />
        )}
        <main className="admin-main">
          {/* La recomendación no ocupa espacio de la captura (§1.2): reaparece al salir de ella. */}
          {identity.user.initial_password && !reducedBar && (
            <div className="password-warning" role="status">
              <div>
                <strong>Recomendamos cambiar tu contraseña inicial.</strong>
                <p>Puedes seguir utilizando el sistema y cambiarla cuando lo decidas.</p>
              </div>
              <button className="secondary-button" onClick={() => requestScreen('password')}>
                Cambiar contraseña
              </button>
            </div>
          )}
          <Breadcrumb
            screen={screen}
            tail={tail?.screen === screen ? tail.label : null}
            onNavigate={requestScreen}
          />
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
                requestKey={operationRequest}
                can={can}
                cashShortcut={cashShortcut}
                onViewChange={setOperationView}
                onCaptureChange={setCaptureActive}
                onCommandState={setOperationCommands}
                commandHandlers={operationHandlers}
                shellControls={
                  <>
                    {server !== 'online' && <ServerStatus state={server} />}
                    {userMenu(true)}
                  </>
                }
                connectionLost={server === 'offline'}
                onRetryConnection={() => void session.refresh()}
              />
            </div>
          )}
        </main>
        <CashShortcutSheet
          shortcut={cashShortcut}
          can={can}
          timezone={identity.establishment.timezone}
        />
        <VirtualKeyboard mode={identity.terminal.keyboard_mode} />
        {commands.overlay}
        {leaveDialog}
      </div>
    </NavigationContext.Provider>
  );
}
