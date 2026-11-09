import { useState } from 'react';
import type { ReactNode } from 'react';
import { Notice } from '../../components/Forms';
import { TableMap } from '../../components/restaurant/TableMap';
import { useRestaurantWorkspace } from '../../hooks/useRestaurantWorkspace';
import type { RestaurantView, WorkspacePanel } from '../../hooks/useRestaurantWorkspace';
import { AccountActions } from './AccountActions';
import type { AccountAction } from './AccountActions';
import { AccountForm } from './AccountForm';
import { AccountHistory } from './AccountHistory';
import { AccountKitchen } from './AccountKitchen';
import { AccountList } from './AccountList';
import { CancelItem } from './CancelItem';
import { OperationsAdmin } from './OperationsAdmin';
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

  function startAccount(takeaway = view === 'takeaway') {
    workspace.setNewMode(takeaway ? 'takeaway' : 'dine_in');
    workspace.setPanel('new');
  }

  function openCheckout(deliver: boolean) {
    setCheckoutDeliver(deliver);
    openAccountAction('checkout', 'capture');
  }

  // Paneles de la cuenta (formularios y consultas). En la captura ocupan las columnas de
  // categorías y productos; en los listados, la columna izquierda.
  function accountPanel(): ReactNode {
    if (panel === 'new') {
      return (
        <AccountForm
          timezone={timezone}
          key={`new-${workspace.newMode}`}
          mode={workspace.newMode}
          tableID={view === 'tables' ? workspace.tableID || null : null}
          overview={overview.data}
          lookups={lookups.data}
          can={can}
          execute={workspace.execute}
          busy={blocked}
          onClose={() => workspace.setPanel('capture')}
        />
      );
    }
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
        <AccountForm
          timezone={timezone}
          key={bill.account.id}
          mode={workspace.newMode}
          tableID={null}
          current={bill.account}
          overview={overview.data}
          lookups={lookups.data}
          can={can}
          execute={workspace.execute}
          busy={blocked}
          onClose={() => workspace.setPanel('capture')}
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
        {can('accounts.open') && !checkoutOpen && view !== 'manage' && (
          <button
            className="primary-button"
            disabled={
              blocked ||
              (view === 'tables' &&
                !!selectedTable &&
                (!selectedTable.active || selectedTable.out_of_service)) ||
              (view === 'takeaway' && !can('takeaway.manage'))
            }
            onClick={() => startAccount()}
          >
            {view === 'takeaway'
              ? 'Nuevo para llevar'
              : view === 'tables'
                ? 'Abrir cuenta en mesa'
                : 'Cuenta sin mesa'}
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
          onNewAccount={() => startAccount(false)}
        />
      </div>
    );
  }

  if (view === 'tables') {
    return (
      <div className="operational-page">
        {toolbar}
        {banners}
        <div className="operation-map-screen">
          <Notice>{overview.error}</Notice>
          {panel === 'new' ? (
            <div className="operation-map-form">{accountPanel()}</div>
          ) : overview.data ? (
            <section className="operation-map-card" aria-label="Mapa del restaurante">
              <p className="muted text-sm">
                {selectedTable?.state === 'free'
                  ? `${selectedTable.name} está libre. Abre una cuenta para capturar.`
                  : 'Toca una mesa para abrir su cuenta y capturar productos.'}
              </p>
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
            <p className="muted">Consultando áreas y mesas…</p>
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
