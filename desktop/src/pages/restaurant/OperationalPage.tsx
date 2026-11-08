import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
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
import { BillPanel } from './BillPanel';
import { CancelItem } from './CancelItem';
import { OperationsAdmin } from './OperationsAdmin';
import { OrderItemEditor } from './OrderItemEditor';
import { ProductSelector } from './ProductSelector';
import { CheckoutPanel } from './checkout/CheckoutPanel';
import { CheckoutToolbar } from './checkout/CheckoutToolbar';
import { AccountAdjustments } from './adjustments/AccountAdjustments';
import { paymentStateBadges } from '../../services/payments';

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
  const blocked = mutation.busy || mutation.uncertain;
  const selectedTable = overview.data?.tables.find((table) => table.id === workspace.tableID);
  const editable =
    !!bill &&
    bill.account.state === 'open' &&
    (bill.account.mode !== 'takeaway' || can('takeaway.manage'));
  const currentEditor = editor?.accountID === bill?.account.id ? editor : null;
  // El cobro (y sus devoluciones) sustituye las áreas de captura y cuenta dentro de la misma vista.
  const checkoutOpen = view !== 'manage' && (panel === 'checkout' || panel === 'refund') && !!bill;
  const actionNames: AccountAction[] = ['split', 'transfer', 'merge', 'move', 'assign', 'void'];
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

  function openAccountAction(
    action: AccountAction | 'metadata' | 'history' | 'kitchen' | 'checkout' | 'adjustments',
    origin: WorkspacePanel,
  ) {
    setActionReturn(origin);
    workspace.setPanel(action);
  }

  function startAccount(takeaway = view === 'takeaway') {
    workspace.setNewMode(takeaway ? 'takeaway' : 'dine_in');
    workspace.setPanel('new');
  }

  function leftPanel() {
    if (panel === 'kitchen' && bill) {
      return (
        <AccountKitchen
          bill={bill}
          allStations={can('kitchen.stations.all')}
          onClose={() => workspace.setPanel('capture')}
        />
      );
    }
    if (panel === 'new' || (panel === 'metadata' && bill)) {
      return (
        <AccountForm
          timezone={timezone}
          key={panel === 'new' ? `new-${workspace.newMode}` : bill?.account.id}
          mode={workspace.newMode}
          tableID={view === 'tables' ? workspace.tableID || null : null}
          current={panel === 'metadata' ? bill?.account : undefined}
          overview={overview.data}
          lookups={lookups.data}
          can={can}
          execute={workspace.execute}
          busy={blocked}
          onClose={() => workspace.setPanel('capture')}
        />
      );
    }
    if (panel === 'editor' && currentEditor && bill && lookups.data) {
      return (
        <OrderItemEditor
          value={currentEditor}
          onChange={workspace.setEditor}
          bill={bill}
          lookups={lookups.data}
          execute={workspace.execute}
          busy={blocked || !!workspace.resource.error}
          onClose={() => workspace.setPanel('capture')}
          onPreview={workspace.receivePreview}
        />
      );
    }
    if (panel === 'cancel' && bill && workspace.cancelLine) {
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
    if (panel === 'adjustments' && bill) {
      return (
        <AccountAdjustments
          key={bill.account.id}
          bill={bill}
          active={active}
          can={can}
          onClose={() => {
            workspace.setPanel(actionReturn);
            setActionReturn('capture');
          }}
        />
      );
    }
    if (panel === 'history' && bill)
      return (
        <AccountHistory
          key={bill.account.id}
          bill={bill}
          onClose={() => workspace.setPanel('capture')}
        />
      );
    if (bill && actionNames.includes(panel as AccountAction)) {
      return (
        <AccountActions
          key={`${panel}-${bill.account.id}`}
          action={panel as AccountAction}
          bill={bill}
          overview={overview.data}
          execute={workspace.execute}
          busy={blocked}
          onClose={() => {
            workspace.setPanel(actionReturn);
            setActionReturn('capture');
          }}
        />
      );
    }
    if (view === 'takeaway' || view === 'accounts') {
      return (
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
      );
    }
    return (
      <ProductSelector
        account={bill?.account ?? null}
        lookups={lookups.data}
        enabled={can('orders.capture') && can('catalog.read') && can('catalog.prices.read')}
        active={active}
        onSelect={(product) => workspace.startEditor(product, 'new')}
      />
    );
  }

  return (
    <div className="operational-page">
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
          {can('accounts.open') && !checkoutOpen && (
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
      {editor && (
        <div className="pending-selection">
          <span>Selección sin guardar conservada.</span>
          <button className="secondary-button" onClick={workspace.resumeEditor}>
            Retomar selección
          </button>
          <button
            className="secondary-button"
            disabled={mutation.busy}
            onClick={() => {
              workspace.setEditor(null);
              workspace.receivePreview(null);
              workspace.setPanel('capture');
            }}
          >
            Descartar selección
          </button>
        </div>
      )}
      {checkoutOpen && bill && (
        <CheckoutPanel
          key={bill.account.id}
          bill={bill}
          active={active}
          can={can}
          mode={panel === 'refund' ? 'refund' : 'collect'}
          onModeChange={(mode) => workspace.setPanel(mode === 'refund' ? 'refund' : 'checkout')}
          onClose={() => workspace.setPanel('capture')}
        />
      )}
      {!checkoutOpen && view === 'manage' && (
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
      )}
      {!checkoutOpen && view !== 'manage' && (
        <div
          className={`operation-workspace ${view === 'takeaway' ? 'operation-takeaway-board' : ''}`}
        >
          <div className="operation-left">
            {view === 'tables' && overview.data && (
              <Accordion
                key={`map-${!!bill}`}
                title={selectedTable ? `Mesas · ${selectedTable.name}` : 'Mapa del restaurante'}
                summary="Selecciona una mesa; sus cuentas permanecen en esta pantalla"
                defaultOpen={!bill}
              >
                <TableMap
                  areas={overview.data.areas}
                  tables={overview.data.tables}
                  areaID={workspace.areaID}
                  selectedTableID={workspace.tableID}
                  onAreaChange={workspace.setAreaID}
                  onSelect={(table) => workspace.chooseTable(table.id)}
                />
              </Accordion>
            )}
            <Notice>{overview.error}</Notice>
            {view === 'tables' && (
              <div className="table-account-switcher">
                <label>
                  <span>Cuentas de {selectedTable?.name ?? 'la mesa seleccionada'}</span>
                  <select
                    aria-label="Cuenta de la mesa"
                    value={workspace.selectedID}
                    onChange={(event) => workspace.chooseAccount(event.target.value)}
                  >
                    <option value="">Selecciona una cuenta…</option>
                    {workspace.accounts.data?.items.map((account) => (
                      <option key={account.id} value={account.id}>
                        #{account.folio} · {account.name} · ${account.totals.total} ·{' '}
                        {paymentStateBadges[account.payment_state] ?? account.payment_state}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
            )}
            {(view === 'takeaway' || view === 'accounts') &&
              bill &&
              panel === 'capture' &&
              editable &&
              can('orders.capture') && (
                <button
                  className="secondary-button capture-products-button"
                  onClick={() => workspace.setPanel('editor' as const)}
                >
                  Capturar productos en esta cuenta
                </button>
              )}
            <div className="operation-left-content">
              {panel === 'editor' &&
                (view === 'takeaway' || view === 'accounts') &&
                !currentEditor && (
                  <button
                    className="secondary-button"
                    onClick={() => workspace.setPanel('capture')}
                  >
                    Volver al listado de cuentas
                  </button>
                )}
              {panel === 'editor' &&
              !currentEditor &&
              (view === 'takeaway' || view === 'accounts') ? (
                <ProductSelector
                  account={bill?.account ?? null}
                  lookups={lookups.data}
                  enabled={
                    can('orders.capture') && can('catalog.read') && can('catalog.prices.read')
                  }
                  active={active}
                  onSelect={(product) => workspace.startEditor(product, 'new')}
                />
              ) : (
                leftPanel()
              )}
            </div>
          </div>
          <BillPanel
            bill={bill}
            error={workspace.resource.error || workspace.savedQuote.error}
            loading={workspace.resource.loading}
            lookups={lookups.data}
            quote={currentEditor ? workspace.tentativeQuote : workspace.savedQuote.data}
            editing={!!currentEditor}
            busy={blocked}
            can={can}
            onAction={(action) => openAccountAction(action, 'capture')}
            onCheckout={() => openAccountAction('checkout', 'capture')}
            onDraftEdit={(item) => workspace.startEditor(item, 'draft')}
            onDraftRemove={(item) => void workspace.removeDraft(item)}
            onLineEdit={(line) => workspace.startEditor(line, 'confirmed')}
            onLineCancel={(line) => {
              workspace.setCancelLine(line);
              workspace.setPanel('cancel');
            }}
            onConfirm={() => void workspace.confirm()}
            onRefresh={workspace.refresh}
            onMergedTarget={workspace.chooseAccount}
          />
        </div>
      )}
    </div>
  );
}
