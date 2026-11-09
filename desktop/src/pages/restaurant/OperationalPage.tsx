import { useState } from 'react';
import type { ReactNode } from 'react';
import { Notice } from '../../components/Forms';
import { TableMap } from '../../components/restaurant/TableMap';
import { useRestaurantWorkspace } from '../../hooks/useRestaurantWorkspace';
import type { RestaurantView, WorkspacePanel } from '../../hooks/useRestaurantWorkspace';
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
}: {
  active: boolean;
  timezone: string;
  requestedView: 'tables' | 'takeaway';
  can: (permission: string) => boolean;
}) {
  const workspace = useRestaurantWorkspace(active, requestedView, can);
  const { view, panel, bill, overview, lookups, editor, mutation } = workspace;
  // Panel al que vuelve una acción de cuenta: dividir o transferir desde el cobro regresa al cobro.
  const [actionReturn, setActionReturn] = useState<WorkspacePanel>('capture');
  // El cobro abierto desde «Marcar como entregado» inicia con la entrega activada.
  const [checkoutDeliver, setCheckoutDeliver] = useState(false);
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
  const views: { id: RestaurantView; label: string; allowed: boolean }[] = [
    {
      id: 'tables',
      label: 'Mesas',
      allowed: can('restaurant.tables.read') && can('restaurant.areas.read'),
    },
    { id: 'takeaway', label: 'Para llevar', allowed: can('takeaway.read') },
    { id: 'accounts', label: 'Cuentas', allowed: can('accounts.read') },
    {
      id: 'manage',
      label: 'Áreas y distribución',
      allowed:
        can('restaurant.areas.manage') ||
        can('restaurant.tables.manage') ||
        can('restaurant.settings.manage'),
    },
  ];

  function openAccountAction(action: BillAction | 'checkout', origin: WorkspacePanel) {
    setActionReturn(origin);
    void workspace.openPanel(action);
  }

  function closeAction() {
    workspace.setPanel(actionReturn);
    setActionReturn('capture');
  }

  // Abrir una cuenta no muestra formulario (etapa 6.2): Mesas abre en la mesa elegida, Para llevar
  // abre un pedido y Cuentas una cuenta sin mesa; las tres entran directo a la captura.
  function openAccount(mode: 'dine_in' | 'takeaway', table: string | null) {
    void workspace.quickOpen(mode, table);
  }

  function openCheckout(deliver: boolean) {
    setCheckoutDeliver(deliver);
    openAccountAction('checkout', 'capture');
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
          timezone={bill.timezone || timezone}
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
      <div className="operation-view-tabs" role="group" aria-label="Vistas operativas">
        {views
          .filter((entry) => entry.allowed)
          .map((entry) => (
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
          Actualizar
        </button>
        {/* En Mesas, la cuenta se abre desde el panel lateral de la mesa. */}
        {can('accounts.open') && !checkoutOpen && (view === 'takeaway' || view === 'accounts') && (
          <button
            className="primary-button"
            disabled={blocked || (view === 'takeaway' && !can('takeaway.manage'))}
            onClick={() => openAccount(view === 'takeaway' ? 'takeaway' : 'dine_in', null)}
          >
            {view === 'takeaway' ? 'Nuevo pedido' : 'Cuenta sin mesa'}
          </button>
        )}
      </div>
    </div>
  );

  const banners = (
    <>
      <Notice>{workspace.actionError || mutation.error}</Notice>
      {mutation.uncertain && (
        <div className="operation-recovery">
          <p>
            La conexión no permitió conocer el resultado. Recupera la misma operación antes de
            realizar otra.
          </p>
          <button
            className="primary-button"
            disabled={mutation.busy}
            onClick={() => void mutation.retry()}
          >
            Recuperar resultado
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
            onClick={() => void workspace.recoverDraft()}
          >
            Recuperar resultado del borrador
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
            onClick={workspace.discardEditor}
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

  if (workspace.captureMode) {
    const content = panel === 'capture' || panel === 'editor' ? null : accountPanel();
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
                onSelect={(table) => workspace.chooseTable(table.id)}
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
