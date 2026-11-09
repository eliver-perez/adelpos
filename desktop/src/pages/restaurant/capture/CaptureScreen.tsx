import { useState } from 'react';
import type { ReactNode } from 'react';
import { useConfirmDialog } from '../../../components/ConfirmDialog';
import type { useRestaurantWorkspace } from '../../../hooks/useRestaurantWorkspace';
import { isPendingAccount } from '../../../services/restaurant';
import type { ConfirmedLine } from '../../../services/restaurant';
import { CaptureBill } from './CaptureBill';
import type { BillAction } from './CaptureBill';
import { CaptureTopBar } from './CaptureTopBar';
import { CategoryColumn, captureCategories } from './CategoryColumn';
import { CustomizationPanel } from './CustomizationPanel';
import { useShowImages } from './imagePreference';
import { accountNoteShortcuts } from './KitchenNotes';
import { ProductGrid } from './ProductGrid';

type Workspace = ReturnType<typeof useRestaurantWorkspace>;

// Pantalla táctil de captura: barra superior, categorías a la izquierda, productos o
// personalización al centro y la cuenta a la derecha. La misma pantalla sirve para salón y para
// llevar; las acciones de la cuenta (dividir, historial, cobro…) ocupan las dos primeras columnas.
export function CaptureScreen({
  workspace,
  can,
  active,
  timezone,
  blocked,
  panelContent,
  onAction,
  onCheckout,
  onDeliverCheckout,
  onNewAccount,
}: {
  workspace: Workspace;
  can: (permission: string) => boolean;
  active: boolean;
  timezone: string;
  blocked: boolean;
  panelContent: ReactNode;
  onAction: (action: BillAction) => void;
  onCheckout: () => void;
  onDeliverCheckout: () => void;
  onNewAccount: () => void;
}) {
  const { bill, lookups, editor, view } = workspace;
  const [category, setCategory] = useState('');
  const [search, setSearch] = useState('');
  const [showImages, setShowImages] = useShowImages();
  const { confirm, dialog } = useConfirmDialog();
  const categories = captureCategories(lookups.data);
  const selectedCategory = categories.some((entry) => entry.category.id === category)
    ? category
    : (categories[0]?.category.id ?? '');
  const currentEditor = editor && bill && editor.accountID === bill.account.id ? editor : null;
  const customizing = workspace.panel === 'editor' && !!currentEditor && !!lookups.data;
  const captureEnabled = can('orders.capture') && can('catalog.read') && can('catalog.prices.read');
  const writable =
    !!bill &&
    bill.account.state === 'open' &&
    (bill.account.mode !== 'takeaway' || can('takeaway.manage')) &&
    captureEnabled;
  const table =
    workspace.overview.data?.tables.find((entry) => entry.id === bill?.account.table_id) ?? null;
  const tableAccounts =
    view === 'tables'
      ? (workspace.accounts.data?.items.filter((entry) => entry.state === 'open') ?? [])
      : [];
  const shortcuts = accountNoteShortcuts([
    ...workspace.localDraft.map((item) => item.customization.notes ?? ''),
    ...(bill?.lines ?? []).map((line) => line.request.customization?.notes ?? ''),
  ]);
  const linesLocked = captureLinesLocked(workspace);

  // «Volver» descarta la personalización; si hubo cambios, primero lo confirma.
  async function leaveEditor(): Promise<boolean> {
    if (!currentEditor) {
      return true;
    }
    const changed =
      JSON.stringify(currentEditor.line) !== JSON.stringify(currentEditor.initialLine) ||
      currentEditor.reason.trim() !== '';
    if (changed) {
      const discard = await confirm({
        title: 'Descartar la selección',
        message: 'Las opciones elegidas para este producto no se agregarán a la cuenta.',
        confirmLabel: 'Descartar selección',
        cancelLabel: 'Seguir eligiendo',
        tone: 'danger',
      });
      if (!discard) {
        return false;
      }
    }
    workspace.discardEditor();
    return true;
  }

  async function selectCategory(id: string) {
    if (customizing && !(await leaveEditor())) {
      return;
    }
    setSearch('');
    setCategory(id);
  }

  return (
    <div className="capture-screen">
      {dialog}
      <CaptureTopBar
        account={bill?.account ?? null}
        backLabel={view === 'tables' ? 'Mesas' : view === 'takeaway' ? 'Para llevar' : 'Cuentas'}
        table={table}
        tableAccounts={tableAccounts}
        timezone={bill?.timezone ?? timezone}
        canOpenAccount={
          view === 'tables' &&
          can('accounts.open') &&
          !!table &&
          table.active &&
          !table.out_of_service
        }
        canEditDetails={
          !!bill &&
          !isPendingAccount(bill.account) &&
          bill.account.state === 'open' &&
          (bill.account.mode !== 'takeaway' || can('takeaway.manage')) &&
          can('accounts.manage')
        }
        disabled={blocked}
        onBack={workspace.leaveCapture}
        onChooseAccount={workspace.chooseAccount}
        onNewAccount={onNewAccount}
        onDetails={() => onAction('metadata')}
        onRefresh={workspace.refresh}
      />
      <div className="capture-body">
        {panelContent ? (
          <section className="capture-panel-area">{panelContent}</section>
        ) : (
          <>
            <CategoryColumn
              categories={categories}
              selectedID={search.trim() ? '' : selectedCategory}
              onSelect={(id) => void selectCategory(id)}
            />
            <div className="capture-center">
              {customizing && currentEditor && bill && lookups.data ? (
                <CustomizationPanel
                  key={currentEditor.localID}
                  editor={currentEditor}
                  onChange={workspace.setEditor}
                  bill={bill}
                  localDraft={workspace.localDraft}
                  draftRevision={workspace.draftRevision}
                  lookups={lookups.data}
                  noteShortcuts={shortcuts}
                  execute={workspace.execute}
                  busy={blocked}
                  onBack={() => void leaveEditor()}
                  onSave={workspace.saveEditorLine}
                />
              ) : (
                <ProductGrid
                  account={bill?.account ?? null}
                  lookups={lookups.data}
                  categoryID={selectedCategory}
                  search={search}
                  onSearch={setSearch}
                  showImages={showImages}
                  onShowImages={setShowImages}
                  enabled={captureEnabled}
                  active={active}
                  writable={writable && !blocked}
                  onAdd={(product) => workspace.addProduct(product)}
                  onCustomize={(product) => workspace.startEditor(product, 'new')}
                />
              )}
            </div>
          </>
        )}
        <WorkspaceBill
          workspace={workspace}
          can={can}
          blocked={blocked}
          linesLocked={linesLocked}
          onAction={onAction}
          onCheckout={onCheckout}
          onDeliverCheckout={onDeliverCheckout}
        />
      </div>
    </div>
  );
}

// Cuenta de la columna derecha conectada al espacio de trabajo. También se usa en los listados de
// Para llevar y Cuentas, donde ofrece abrir la captura.
export function WorkspaceBill({
  workspace,
  can,
  blocked,
  linesLocked,
  onAction,
  onCheckout,
  onDeliverCheckout,
  onCapture,
}: {
  workspace: Workspace;
  can: (permission: string) => boolean;
  blocked: boolean;
  linesLocked: boolean;
  onAction: (action: BillAction) => void;
  onCheckout: () => void;
  onDeliverCheckout: () => void;
  onCapture?: () => void;
}) {
  const { bill, lookups } = workspace;
  return (
    <CaptureBill
      bill={bill}
      error={workspace.resource.error}
      loading={workspace.resource.loading}
      lookups={lookups.data}
      quote={workspace.savedQuote.data}
      quoteError={workspace.savedQuote.error}
      localDraft={workspace.localDraft}
      persistedDraft={workspace.persistedDraft}
      persistedRevision={workspace.draftRevision}
      draftView={workspace.draftView}
      can={can}
      blocked={blocked}
      linesLocked={linesLocked}
      sending={workspace.sending}
      onIncrement={(item) => workspace.changeQuantity(item, 1)}
      onDecrement={(item) => workspace.changeQuantity(item, -1)}
      onCustomize={(item) => workspace.startEditor(item, 'draft')}
      onLineEdit={(line: ConfirmedLine) => void workspace.startConfirmedEdit(line)}
      onLineCancel={(line: ConfirmedLine) => {
        workspace.setCancelLine(line);
        void workspace.openPanel('cancel');
      }}
      onAction={onAction}
      onSend={() => void workspace.sendToKitchen()}
      onCheckout={onCheckout}
      onDeliverCheckout={onDeliverCheckout}
      onMergedTarget={workspace.chooseAccount}
      onKeepMine={() => bill && workspace.draft.keepMine(bill.account.id)}
      onDiscard={() => bill && workspace.draft.discard(bill.account.id)}
      onCapture={onCapture}
    />
  );
}

// Las partidas no se tocan mientras un panel de acción o una modificación confirmada parten de
// la revisión con que se abrieron.
export function captureLinesLocked(workspace: Workspace) {
  const editing =
    workspace.panel === 'editor' &&
    !!workspace.editor &&
    workspace.editor.accountID === workspace.bill?.account.id &&
    workspace.editor.kind !== 'confirmed';
  return workspace.panel !== 'capture' && !editing;
}
