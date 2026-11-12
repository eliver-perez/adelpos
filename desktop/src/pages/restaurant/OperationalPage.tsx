import { useEffect, useState } from 'react';
import type { ReactNode, RefObject } from 'react';
import type {
  OperationCommandHandlers,
  OperationCommandState,
} from '../../commands/operationBridge';
import { ChoiceDialog, useConfirmDialog } from '../../components/ConfirmDialog';
import { Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { TableMap } from '../../components/restaurant/TableMap';
import { useRestaurantWorkspace } from '../../hooks/useRestaurantWorkspace';
import type { RestaurantView, WorkspacePanel } from '../../hooks/useRestaurantWorkspace';
import { notifyError, notifySuccess } from '../../services/notify';
import { accountLabel, isPendingAccount } from '../../services/restaurant';
import type { Account, RestaurantTable } from '../../services/restaurant';
import {
  CashShortcut,
  CashShortcutSheet,
  NoShiftNotice,
  useCashShortcut,
} from '../cash/CashShortcut';
import type { CashShortcutState } from '../cash/CashShortcut';
import { uncertainError } from './capture/draftSync';
import { AccountActions } from './AccountActions';
import type { AccountAction } from './AccountActions';
import { AccountDetails } from './AccountDetails';
import { AccountHistory } from './AccountHistory';
import { AccountKitchen } from './AccountKitchen';
import { AccountList } from './AccountList';
import { CancelItem } from './CancelItem';
import { OperationsAdmin } from './OperationsAdmin';
import { TablePanel } from './TablePanel';
import { CheckoutPanel } from './checkout/CheckoutPanel';
import { CheckoutToolbar } from './checkout/CheckoutToolbar';
import { AccountAdjustments } from './adjustments/AccountAdjustments';
import { PrecheckPanel } from './documents/PrecheckPanel';
import type { BillAction } from './capture/CaptureBill';
import { CaptureScreen, WorkspaceBill, captureLinesLocked } from './capture/CaptureScreen';

const accountActions: AccountAction[] = ['split', 'transfer', 'merge', 'move', 'assign', 'void'];

export function OperationalPage({
  active,
  timezone,
  requestedView,
  can,
  onOpenCash,
  cashShortcut: sharedCashShortcut,
  requestKey = 0,
  onViewChange,
  onCaptureChange,
  onCommandState,
  commandHandlers,
  shellControls,
}: {
  active: boolean;
  timezone: string;
  requestedView: 'tables' | 'takeaway';
  can: (permission: string) => boolean;
  // «Ir a Caja» del atajo «Caja» (etapa 7.1) cuando la pantalla crea su propio atajo.
  onOpenCash?: () => void;
  // Etapa 9C: atajo «Caja» compartido con la barra superior de App (una sola consulta del turno).
  cashShortcut?: CashShortcutState;
  // Cambia cada vez que se elige Mesas o Para llevar en la barra superior.
  requestKey?: number;
  // Vista operativa visible (la barra resalta Mesas o Para llevar según ella).
  onViewChange?: (view: RestaurantView) => void;
  // Indica si la captura está abierta: App reduce entonces la barra superior (§1.2).
  onCaptureChange?: (capture: boolean) => void;
  // Etapa 9C, fase C: contexto y botones de esta pantalla para el menú nativo, la paleta y los
  // atajos (las mismas funciones de los botones).
  onCommandState?: (state: OperationCommandState) => void;
  commandHandlers?: RefObject<OperationCommandHandlers | null>;
  // Controles del armazón para la barra reducida de la captura (usuario y estado del servidor).
  shellControls?: ReactNode;
}) {
  const workspace = useRestaurantWorkspace(active, requestedView, can, requestKey);
  // Atajo «Caja» (etapa 7.1): una sola consulta del turno de la terminal para la barra, la captura
  // y la hoja superpuesta. En la aplicación lo crea App (barra superior, etapa 9C); montada sola,
  // la pantalla crea el suyo y lo muestra en su barra operativa.
  const ownCashShortcut = useCashShortcut({
    active: active && !sharedCashShortcut,
    can,
    onOpenCash,
  });
  const cashShortcut = sharedCashShortcut ?? ownCashShortcut;
  // Dentro del armazón de App (con la barra superior) la pantalla recibe el atajo compartido.
  const inShell = !!sharedCashShortcut;
  const { view, panel, bill, overview, lookups, editor, mutation } = workspace;
  const captureMode = workspace.captureMode;
  useEffect(() => {
    onViewChange?.(view);
  }, [view, onViewChange]);
  useEffect(() => {
    onCaptureChange?.(captureMode);
  }, [captureMode, onCaptureChange]);
  // Panel al que vuelve una acción de cuenta: dividir o transferir desde el cobro regresa al cobro.
  const [actionReturn, setActionReturn] = useState<WorkspacePanel>('capture');
  // El cobro abierto desde «Marcar como entregado» inicia con la entrega activada.
  const [checkoutDeliver, setCheckoutDeliver] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const draftUncertain = workspace.draft.uncertain;
  const blocked = mutation.busy || mutation.uncertain || draftUncertain;
  const selectedTable = overview.data?.tables.find((table) => table.id === workspace.tableID);
  // El cobro (y sus devoluciones) sustituye la captura y la cuenta dentro de la misma vista.
  const checkoutOpen = view !== 'manage' && (panel === 'checkout' || panel === 'refund') && !!bill;
  const editorVisible =
    workspace.captureMode &&
    panel === 'editor' &&
    !!editor &&
    editor.accountID === bill?.account.id;
  // Etapa 9C: Mesas y Para llevar son secciones de la barra superior. Dentro de Mesas siguen como
  // pestañas «Cuentas» y «Áreas y distribución» (decisión 5 del coordinador); Para llevar no lleva
  // pestañas. Montada sola (pruebas), la pantalla conserva las cuatro vistas.
  const section: RestaurantView = view === 'takeaway' ? 'takeaway' : 'tables';
  const views: { id: RestaurantView; label: string; allowed: boolean }[] = [
    {
      id: 'tables',
      label: 'Mesas',
      allowed:
        can('restaurant.tables.read') &&
        can('restaurant.areas.read') &&
        (!inShell || section === 'tables'),
    },
    {
      id: 'takeaway',
      label: 'Para llevar',
      allowed: can('takeaway.read') && !inShell,
    },
    {
      id: 'accounts',
      label: 'Cuentas',
      allowed: can('accounts.read') && (!inShell || section === 'tables'),
    },
    {
      id: 'manage',
      label: 'Áreas y distribución',
      allowed:
        (can('restaurant.areas.manage') ||
          can('restaurant.tables.manage') ||
          can('restaurant.settings.manage')) &&
        (!inShell || section === 'tables'),
    },
  ];
  const visibleViews = views.filter((entry) => entry.allowed);

  // Registro de acciones (etapa 9C, fase C). «Nueva cuenta en mesa» actúa como «Capturar» / «Nueva
  // cuenta» del panel de la mesa elegida o como «Nueva cuenta» de la captura de una mesa;
  // «Imprimir precuenta», como «Precuenta» de la cuenta a la vista.
  const captureTable =
    captureMode && view === 'tables' && bill && !isPendingAccount(bill.account)
      ? overview.data?.tables.find((table) => table.id === bill.account.table_id)
      : undefined;
  const newAccountTable = captureMode
    ? captureTable
    : view === 'tables'
      ? selectedTable
      : undefined;
  const tableForNewAccount =
    active &&
    !blocked &&
    !checkoutOpen &&
    can('accounts.open') &&
    !!newAccountTable &&
    newAccountTable.active &&
    !newAccountTable.out_of_service
      ? newAccountTable.id
      : null;
  const precheckVisible =
    !!bill && (captureMode || view !== 'tables' || bill.account.table_id === workspace.tableID);
  const precheckReady =
    active &&
    !blocked &&
    !checkoutOpen &&
    view !== 'manage' &&
    can('payments.read') &&
    precheckVisible &&
    !!bill &&
    bill.account.state === 'open' &&
    !isPendingAccount(bill.account) &&
    bill.account.confirmed_quantity > 0;
  useEffect(() => {
    onCommandState?.({
      active,
      view,
      captureMode,
      checkoutOpen,
      blocked,
      tableForNewAccount,
      precheckReady,
    });
  }, [
    onCommandState,
    active,
    view,
    captureMode,
    checkoutOpen,
    blocked,
    tableForNewAccount,
    precheckReady,
  ]);

  function openAccountAction(action: BillAction | 'checkout', origin: WorkspacePanel) {
    setActionReturn(origin);
    void workspace.openPanel(action);
  }

  function closeAction() {
    workspace.setPanel(actionReturn);
    setActionReturn('capture');
  }

  // Abrir una cuenta no muestra formulario (etapa 6.2). En Mesas (etapa 7.1) se entra a la
  // captura sin crear la cuenta: Go la crea con el primer producto guardado. Para llevar y
  // Cuentas («Cuenta sin mesa») siguen abriendo la cuenta al instante.
  function openAccount(mode: 'dine_in' | 'takeaway', table: string | null) {
    if (mode === 'dine_in' && table) {
      workspace.startPendingCapture(table);
      return;
    }
    void workspace.quickOpen(mode, table);
  }

  // «Liberar mesa» (etapa 7.1): una cuenta vacía se anula sin motivo después de confirmarlo.
  async function releaseAccount(table: RestaurantTable, account: Account) {
    // Con otras cuentas abiertas la mesa sigue ocupada: solo se libera esta cuenta.
    const others =
      workspace.accounts.data?.items.filter(
        (entry) => entry.state === 'open' && entry.table_id === table.id && entry.id !== account.id,
      ).length ?? 0;
    const release = await confirm({
      title: `¿Liberar ${table.name}?`,
      message:
        others > 0
          ? `${accountLabel(account)} no tiene productos, pagos ni registros de cocina. Se anulará; las demás cuentas de la mesa no cambian.`
          : `${accountLabel(account)} no tiene productos, pagos ni registros de cocina. Se anulará y la mesa quedará libre.`,
      confirmLabel: 'Liberar mesa',
      cancelLabel: 'Conservar la cuenta',
    });
    if (!release) {
      return;
    }
    try {
      await workspace.execute('restaurant_release_account', {
        id: account.id,
        body: { revision: account.revision },
      });
      notifySuccess(
        others > 0 ? `${accountLabel(account)} liberada.` : `${table.name} quedó libre.`,
      );
    } catch (problem) {
      workspace.reportError(problem, 'No se pudo liberar la mesa.');
    }
  }

  // Fuera de servicio desde la operación (etapa 7.1); Go rechaza una mesa con cuentas abiertas.
  async function changeTableService(table: RestaurantTable, outOfService: boolean) {
    try {
      await workspace.execute('restaurant_table_service', {
        id: table.id,
        body: { revision: table.revision, out_of_service: outOfService },
      });
      notifySuccess(
        outOfService ? `${table.name} quedó fuera de servicio.` : `${table.name} quedó habilitada.`,
      );
    } catch (problem) {
      workspace.reportError(problem, 'No se pudo cambiar el servicio de la mesa.');
    }
  }

  function openCheckout(deliver: boolean) {
    setCheckoutDeliver(deliver);
    openAccountAction('checkout', 'capture');
  }

  // Paneles de la cuenta que se cierran volviendo a donde se abrieron (como sus botones).
  const returningPanels: WorkspacePanel[] = [
    'metadata',
    'adjustments',
    'precheck',
    ...accountActions,
  ];

  // Escape (etapa 9C, fase C): cierra el panel de la cuenta, sale de la captura («‹ Mesas») o
  // cierra el panel lateral de la mesa. El cobro conserva su Escape (limpia el importe) y la
  // personalización sus propias acciones.
  function back() {
    if (!active || checkoutOpen || panel === 'editor' || view === 'manage') {
      return false;
    }
    if (bill && panel !== 'capture') {
      if (returningPanels.includes(panel)) {
        closeAction();
      } else {
        workspace.setPanel('capture');
      }
      return true;
    }
    if (captureMode) {
      workspace.leaveCapture();
      return true;
    }
    if (view === 'tables' && workspace.tableID) {
      workspace.chooseTable('');
      return true;
    }
    return false;
  }

  // Elegir una mesa desde la paleta o desde el buscador del mapa es tocarla en el mapa (con su
  // área a la vista).
  function showTable(id: string) {
    const table = overview.data?.tables.find((entry) => entry.id === id);
    if (!table) {
      return;
    }
    if (workspace.captureMode) {
      workspace.leaveCapture();
    }
    if (view !== 'tables') {
      workspace.switchView('tables');
    }
    if (table.area_id !== workspace.areaID) {
      workspace.setAreaID(table.area_id);
    }
    workspace.chooseTable(id);
  }

  const handlers: OperationCommandHandlers = {
    newTableAccount: () => {
      if (tableForNewAccount) {
        openAccount('dine_in', tableForNewAccount);
      }
    },
    accountWithoutTable: () => openAccount('dine_in', null),
    newTakeaway: () => openAccount('takeaway', null),
    openPrecheck: () => openAccountAction('precheck', 'capture'),
    leaveCapture: workspace.leaveCapture,
    chooseTable: showTable,
    tables: () => {
      const areas = overview.data?.areas ?? [];
      return (overview.data?.tables ?? [])
        .filter((table) => table.active)
        .map((table) => ({
          id: table.id,
          name: table.name,
          areaName: areas.find((area) => area.id === table.area_id)?.name ?? '',
        }));
    },
    back,
  };
  // Las funciones cambian en cada render (cierran sobre el estado vigente): se publican después de
  // cada render y se retiran al desmontar la pantalla.
  useEffect(() => {
    if (commandHandlers) {
      commandHandlers.current = handlers;
    }
  });
  useEffect(
    () => () => {
      if (commandHandlers) {
        commandHandlers.current = null;
      }
    },
    [commandHandlers],
  );

  // «Recuperar resultado» repite la misma operación con su operation_id. Si Go la confirma se
  // avisa; si la rechaza, el error permanece; si sigue sin respuesta, el panel continúa.
  async function recoverOperation() {
    const problem = await mutation.retry();
    if (!problem) {
      notifySuccess('Resultado recuperado: la operación quedó registrada.');
      return;
    }
    if (!uncertainError(problem)) {
      notifyError(problem, 'No se pudo recuperar el resultado de la operación.');
    }
  }

  // Descartar la selección conservada pierde las opciones elegidas: se confirma antes.
  async function discardPendingSelection() {
    const discard = await confirm({
      title: '¿Descartar la selección sin guardar?',
      message: 'Las opciones elegidas para ese producto no se agregarán a la cuenta.',
      confirmLabel: 'Descartar selección',
      cancelLabel: 'Conservarla',
      tone: 'danger',
    });
    if (discard) {
      workspace.discardEditor();
    }
  }

  // Recupera el guardado incierto del borrador. Un rechazo queda indicado en la cuenta.
  async function recoverDraft() {
    const accountID = workspace.draft.uncertainAccountID;
    await workspace.recoverDraft();
    const stored = accountID ? workspace.draft.view(accountID) : null;
    if (accountID && !workspace.draft.uncertain && !stored?.error) {
      notifySuccess('Productos nuevos guardados en la cuenta.');
    }
  }

  // Paneles de la cuenta (formularios y consultas). En la captura ocupan las columnas de
  // categorías y productos; en los listados, la columna izquierda.
  function accountPanel(): ReactNode {
    if (!bill) {
      return null;
    }
    if (panel === 'kitchen') {
      return (
        <AccountKitchen
          bill={bill}
          allStations={can('kitchen.stations.all')}
          onClose={() => workspace.setPanel('capture')}
        />
      );
    }
    if (panel === 'metadata') {
      return (
        <AccountDetails
          key={bill.account.id}
          account={bill.account}
          lookups={lookups.data}
          can={can}
          execute={workspace.execute}
          busy={blocked}
          onClose={closeAction}
        />
      );
    }
    if (panel === 'cancel' && workspace.cancelLine) {
      return (
        <CancelItem
          key={workspace.cancelLine.version_id}
          bill={bill}
          line={workspace.cancelLine}
          execute={workspace.execute}
          busy={blocked}
          onClose={() => workspace.setPanel('capture')}
        />
      );
    }
    if (panel === 'adjustments') {
      return (
        <AccountAdjustments
          key={bill.account.id}
          bill={bill}
          active={active}
          can={can}
          onClose={closeAction}
        />
      );
    }
    if (panel === 'precheck') {
      return (
        <PrecheckPanel
          key={bill.account.id}
          bill={bill}
          active={active}
          can={can}
          onClose={closeAction}
        />
      );
    }
    if (panel === 'history') {
      return (
        <AccountHistory
          key={bill.account.id}
          bill={bill}
          onClose={() => workspace.setPanel('capture')}
        />
      );
    }
    if (accountActions.includes(panel as AccountAction)) {
      return (
        <AccountActions
          key={`${panel}-${bill.account.id}`}
          action={panel as AccountAction}
          bill={bill}
          overview={overview.data}
          execute={workspace.execute}
          busy={blocked}
          onClose={closeAction}
        />
      );
    }
    return null;
  }

  const toolbar = (
    <div className="operation-toolbar">
      {visibleViews.length > 1 && (
        <div className="operation-view-tabs" role="group" aria-label="Vistas operativas">
          {visibleViews.map((entry) => (
            <button
              key={entry.id}
              type="button"
              aria-pressed={view === entry.id}
              className={view === entry.id ? 'selected-operation-view' : ''}
              onClick={() => workspace.switchView(entry.id)}
            >
              {entry.label}
            </button>
          ))}
        </div>
      )}
      {/* Atajo «Caja» junto a las vistas cuando la pantalla está sola; en la aplicación está en la
          barra superior (etapa 9C). */}
      {!inShell && <CashShortcut shortcut={cashShortcut} can={can} />}
      {checkoutOpen && bill && (
        <CheckoutToolbar
          bill={bill}
          mode={panel === 'refund' ? 'refund' : 'collect'}
          can={can}
          onMode={(mode) => workspace.setPanel(mode === 'refund' ? 'refund' : 'checkout')}
          onShortcut={(shortcut) => openAccountAction(shortcut, panel)}
        />
      )}
      <div className={`button-row ${checkoutOpen ? 'operation-toolbar-secondary' : ''}`}>
        <button className="secondary-button" onClick={workspace.refresh}>
          <IconLabel icon="refresh">Actualizar</IconLabel>
        </button>
        {/* En Mesas, la cuenta se abre desde el panel lateral de la mesa. */}
        {can('accounts.open') && !checkoutOpen && (view === 'takeaway' || view === 'accounts') && (
          <button
            className="primary-button"
            disabled={blocked || (view === 'takeaway' && !can('takeaway.manage'))}
            onClick={() => openAccount(view === 'takeaway' ? 'takeaway' : 'dine_in', null)}
          >
            <IconLabel icon="plus">
              {view === 'takeaway' ? 'Nuevo pedido' : 'Cuenta sin mesa'}
            </IconLabel>
          </button>
        )}
      </div>
    </div>
  );

  // Estados persistentes del flujo (etapa 7.1): siguen en pantalla mientras duran. Los resultados
  // de las acciones se informan con avisos (services/notify).
  const conflict = workspace.tableConflict;
  const conflictTable = conflict
    ? overview.data?.tables.find((table) => table.id === conflict.tableID)
    : undefined;
  const banners = (
    <>
      {dialog}
      {/* Con el atajo compartido, App muestra la hoja una sola vez. */}
      {!inShell && <CashShortcutSheet shortcut={cashShortcut} can={can} timezone={timezone} />}
      {conflict && (
        <ChoiceDialog
          title="Esta mesa ya tiene una cuenta"
          message={
            <p>
              Otra terminal abrió{' '}
              {conflict.accounts.map((entry) => `la Cuenta #${entry.folio}`).join(' y ')}
              {conflictTable ? ` en ${conflictTable.name}` : ''} mientras capturabas. Tus productos
              se conservan en esta terminal: agrégalos a esa cuenta o abre una cuenta nueva en la
              mesa.
            </p>
          }
          busy={workspace.resolvingConflict}
          choices={[
            ...conflict.accounts.map((entry) => ({
              label: `Agregar a la Cuenta #${entry.folio}`,
              onSelect: () => void workspace.addPendingToAccount(conflict.localID, entry.id),
            })),
            {
              label: 'Abrir una cuenta nueva',
              onSelect: () => workspace.openPendingAnyway(conflict.localID),
            },
          ]}
        />
      )}
      {mutation.uncertain && (
        <div className="operation-recovery">
          <p>
            La conexión no permitió conocer el resultado. Recupera la misma operación antes de
            realizar otra.
          </p>
          <button
            className="primary-button"
            disabled={mutation.busy}
            onClick={() => void recoverOperation()}
          >
            <IconLabel icon="refresh">Recuperar resultado</IconLabel>
          </button>
        </div>
      )}
      {draftUncertain && (
        <div className="operation-recovery">
          <p>
            No se pudo confirmar el guardado de los productos nuevos. Recupera la misma operación
            antes de continuar; tus toques se conservan en esta terminal.
          </p>
          <button
            className="primary-button"
            disabled={workspace.draft.busy}
            onClick={() => void recoverDraft()}
          >
            <IconLabel icon="refresh">Recuperar resultado del borrador</IconLabel>
          </button>
        </div>
      )}
      {editor && !editorVisible && (
        <div className="pending-selection">
          <span>Selección sin guardar conservada.</span>
          <button className="secondary-button" onClick={workspace.resumeEditor}>
            Retomar selección
          </button>
          <button
            className="secondary-button"
            disabled={mutation.busy}
            onClick={() => void discardPendingSelection()}
          >
            Descartar selección
          </button>
        </div>
      )}
    </>
  );

  if (checkoutOpen && bill) {
    return (
      <div className="operational-page">
        {toolbar}
        {banners}
        <CheckoutPanel
          key={bill.account.id}
          bill={bill}
          active={active}
          can={can}
          mode={panel === 'refund' ? 'refund' : 'collect'}
          onModeChange={(mode) => workspace.setPanel(mode === 'refund' ? 'refund' : 'checkout')}
          onClose={() => workspace.setPanel('capture')}
          initialDeliver={checkoutDeliver}
        />
      </div>
    );
  }

  if (view === 'manage') {
    return (
      <div className="operational-page">
        {toolbar}
        {banners}
        <div className="operation-admin-scroll">
          <Notice>{overview.error}</Notice>
          {overview.data ? (
            <OperationsAdmin
              overview={overview.data}
              can={can}
              execute={workspace.execute}
              busy={blocked}
            />
          ) : (
            <p className="muted">Consultando áreas y mesas…</p>
          )}
        </div>
      </div>
    );
  }

  if (captureMode) {
    const content = panel === 'capture' || panel === 'editor' ? null : accountPanel();
    // Con la operación oculta (otra pantalla visible) el botón Caja de la captura no se monta:
    // comparte el estado del menú con el de la barra superior y, al tocar ese menú, su «tocar
    // fuera» lo cerraría.
    return (
      <div className="operational-page">
        {banners}
        <CaptureScreen
          workspace={workspace}
          can={can}
          active={active}
          timezone={timezone}
          blocked={blocked}
          panelContent={content}
          onAction={(action) => openAccountAction(action, 'capture')}
          onCheckout={() => openCheckout(false)}
          onDeliverCheckout={() => openCheckout(true)}
          onNewAccount={() =>
            openAccount('dine_in', bill?.account.table_id ?? (workspace.tableID || null))
          }
          cashShortcut={active ? <CashShortcut shortcut={cashShortcut} can={can} /> : undefined}
          shiftNotice={<NoShiftNotice shortcut={cashShortcut} can={can} />}
          shellControls={shellControls}
        />
      </div>
    );
  }

  if (view === 'tables') {
    // Mapa operativo (sin controles de edición) a la izquierda y panel lateral de la mesa a la
    // derecha. Una acción de la cuenta (dividir, precuenta, historial…) ocupa el lugar del mapa
    // y al cerrarse vuelve a él; el panel permanece.
    const tablePanelAction = panel === 'capture' || panel === 'editor' ? null : accountPanel();
    const tableAccounts =
      selectedTable && workspace.accounts.data
        ? workspace.accounts.data.items.filter(
            (entry) => entry.state === 'open' && entry.table_id === selectedTable.id,
          )
        : null;
    return (
      <div className="operational-page">
        {toolbar}
        {banners}
        <div className={`operation-map-screen ${selectedTable ? 'operation-map-split' : ''}`}>
          {tablePanelAction ? (
            <section className="operation-map-card operation-map-action">
              {tablePanelAction}
            </section>
          ) : overview.data ? (
            <section className="operation-map-card" aria-label="Mapa del restaurante">
              <Notice>{overview.error}</Notice>
              {!selectedTable && (
                <p className="muted text-sm">
                  Toca una mesa para ver sus cuentas, abrir una nueva o capturar productos.
                </p>
              )}
              <TableMap
                areas={overview.data.areas}
                tables={overview.data.tables}
                areaID={workspace.areaID}
                selectedTableID={workspace.tableID}
                onAreaChange={workspace.setAreaID}
                onSelect={(table) => showTable(table.id)}
              />
            </section>
          ) : (
            <section className="operation-map-card">
              <Notice>{overview.error}</Notice>
              <p className="muted">Consultando áreas y mesas…</p>
            </section>
          )}
          {selectedTable && (
            <TablePanel
              table={selectedTable}
              accounts={tableAccounts}
              bill={bill}
              billError={workspace.resource.error}
              localDraft={workspace.localDraft}
              lookups={lookups.data}
              can={can}
              blocked={blocked}
              onChooseAccount={workspace.chooseAccount}
              onCapture={workspace.enterCapture}
              onOpenAccount={() => openAccount('dine_in', selectedTable.id)}
              onAction={(action) => openAccountAction(action, 'capture')}
              onCheckout={() => openCheckout(false)}
              onRelease={(account) => void releaseAccount(selectedTable, account)}
              onService={(outOfService) => void changeTableService(selectedTable, outOfService)}
            />
          )}
        </div>
      </div>
    );
  }

  // Para llevar y Cuentas: listado a la izquierda y la cuenta seleccionada a la derecha.
  const listPanel = accountPanel();
  return (
    <div className="operational-page">
      {toolbar}
      {banners}
      <div
        className={`operation-workspace ${view === 'takeaway' ? 'operation-takeaway-board' : ''}`}
      >
        <div className="operation-left">
          <div className="operation-left-content">
            {listPanel ?? (
              <AccountList
                value={workspace.accounts.data}
                selectedID={workspace.selectedID}
                onSelect={workspace.chooseAccount}
                search={workspace.search}
                onSearch={workspace.setSearch}
                state={workspace.state}
                onStateChange={workspace.setState}
                page={workspace.page}
                onPageChange={workspace.setPage}
                error={workspace.accounts.error}
                takeaway={view === 'takeaway'}
              />
            )}
          </div>
        </div>
        <WorkspaceBill
          workspace={workspace}
          can={can}
          blocked={blocked}
          linesLocked={captureLinesLocked(workspace)}
          onAction={(action) => openAccountAction(action, 'capture')}
          onCheckout={() => openCheckout(false)}
          onDeliverCheckout={() => openCheckout(true)}
          onCapture={workspace.enterCapture}
        />
      </div>
    </div>
  );
}
