import { useCallback, useEffect, useState } from 'react';
import { APIError } from '../services/api';
import type { Operation } from '../services/api';
import type { Lookups } from '../services/catalog';
import { emptyCustomization, normalizeSelection } from '../services/restaurant';
import type {
  AccountPage,
  Bill,
  ConfirmedLine,
  DraftItem,
  EditorState,
  OrderQuote,
  Overview,
} from '../services/restaurant';
import { useCaptureDraft } from './useCaptureDraft';
import { useDraftSync } from './useDraftSync';
import { useLiveResource } from './useLiveResource';
import { useRestaurantMutation } from './useRestaurantMutation';
import type { AccountAction } from '../pages/restaurant/AccountActions';
import type { CaptureProduct } from '../pages/restaurant/capture/captureRules';

export type RestaurantView = 'tables' | 'takeaway' | 'accounts' | 'manage';
export type WorkspacePanel =
  | 'capture'
  | 'new'
  | 'metadata'
  | 'history'
  | 'editor'
  | 'cancel'
  | 'kitchen'
  | 'checkout'
  | 'refund'
  | 'adjustments'
  | 'precheck'
  | AccountAction;

// Paneles de solo consulta: no requieren sincronizar el borrador antes de abrirse.
const readOnlyPanels: WorkspacePanel[] = ['capture', 'history', 'kitchen', 'editor'];

function newID() {
  return crypto.randomUUID();
}

export function useRestaurantWorkspace(
  active: boolean,
  requestedView: 'tables' | 'takeaway',
  can: (permission: string) => boolean,
) {
  const [view, setView] = useState<RestaurantView>(requestedView);
  const [panel, setPanel] = useState<WorkspacePanel>('capture');
  const [areaID, setAreaID] = useState('');
  const [tableID, setTableID] = useState('');
  const [selections, setSelections] = useState<Record<RestaurantView, string>>({
    tables: '',
    takeaway: '',
    accounts: '',
    manage: '',
  });
  // En Para llevar y Cuentas, la captura táctil se abre expresamente desde el listado; se recuerda
  // por vista para que llegar desde el menú principal muestre el listado de esa vista.
  const [captureViews, setCaptureViews] = useState<Record<RestaurantView, boolean>>({
    tables: false,
    takeaway: false,
    accounts: false,
    manage: false,
  });
  const [search, setSearch] = useState('');
  const [state, setState] = useState('open');
  const [page, setPage] = useState(1);
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [cancelLine, setCancelLine] = useState<ConfirmedLine | null>(null);
  const [newMode, setNewMode] = useState<'dine_in' | 'takeaway'>('dine_in');
  const [actionError, setActionError] = useState('');
  const [refresh, setRefresh] = useState(0);
  const canMap = can('restaurant.areas.read') && can('restaurant.tables.read');
  const selectedID = selections[view];
  const draft = useDraftSync();
  const overview = useLiveResource<Overview>('restaurant_overview', { enabled: active && canMap });
  const lookups = useLiveResource<Lookups>('catalog_lookups', {
    enabled: active && can('catalog.read'),
    interval: 15000,
  });
  const query: Record<string, string> = {
    mode: view === 'takeaway' ? 'takeaway' : 'dine_in',
    search,
    state,
    page: String(page),
    page_size: '50',
  };
  if (view === 'tables') query.table_id = tableID;
  const accounts = useLiveResource<AccountPage>('restaurant_accounts', {
    enabled: active && view !== 'manage' && (view !== 'tables' || !!tableID),
    query,
  });
  const resource = useLiveResource<Bill>('restaurant_account', {
    id: selectedID || undefined,
    enabled: active && !!selectedID,
  });
  const fetched = resource.data?.account.id === selectedID ? resource.data : null;
  // La respuesta de un guardado propio del borrador puede ser más reciente que el último sondeo.
  const synced = selectedID ? draft.engine.latestBill(selectedID) : null;
  const bill =
    fetched && synced && synced.account.revision > fetched.account.revision ? synced : fetched;
  const savedQuote = useLiveResource<OrderQuote>('restaurant_preview', {
    id: selectedID || undefined,
    enabled:
      active &&
      !!bill &&
      bill.account.state === 'open' &&
      bill.draft.length > 0 &&
      can('catalog.read') &&
      can('catalog.prices.read'),
  });
  const captureMode =
    view !== 'manage' && !!selectedID && (view === 'tables' || captureViews[view]);

  function setCaptureOpen(target: RestaurantView, open: boolean) {
    setCaptureViews((current) => ({ ...current, [target]: open }));
  }

  const mutation = useRestaurantMutation((operation, result) => {
    if (
      operation.startsWith('restaurant_') &&
      ![
        'restaurant_create_area',
        'restaurant_save_area',
        'restaurant_create_table',
        'restaurant_save_table',
        'restaurant_save_layout',
        'restaurant_save_settings',
      ].includes(operation)
    ) {
      setPanel('capture');
      const record = result as { account?: Bill['account']; target?: Bill };
      if (operation === 'restaurant_edit') {
        setEditor((current) => (current?.accountID === record.account?.id ? null : current));
      }
      const account = record.account ?? record.target?.account;
      if (account) {
        const destination: RestaurantView =
          account.mode === 'takeaway' ? 'takeaway' : account.table_id ? 'tables' : 'accounts';
        if (operation === 'restaurant_open_account') {
          // «Abrir cuenta y capturar» lleva directamente a la captura táctil.
          setCaptureOpen(destination, true);
        }
        setView(destination);
        setSelections((current) => ({
          ...current,
          [destination]: account.state === 'open' ? account.id : '',
        }));
        if (account.table_id) setTableID(account.table_id);
      }
    }
    setRefresh((current) => current + 1);
  });

  useEffect(() => {
    if (active) setView(requestedView === 'tables' && !canMap ? 'accounts' : requestedView);
  }, [active, requestedView, canMap]);

  useEffect(() => {
    if (!areaID && overview.data?.areas.length)
      setAreaID(
        overview.data.areas.find((area) => area.active)?.id ?? overview.data.areas[0]?.id ?? '',
      );
  }, [overview.data, areaID]);

  useEffect(() => {
    if (!selectedID && accounts.data?.items[0]) {
      const first = accounts.data.items[0];
      setSelections((current) => ({ ...current, [view]: first.id }));
    }
  }, [selectedID, accounts.data, view]);

  useEffect(() => {
    if (view === 'tables' && bill?.account.table_id && bill.account.table_id !== tableID) {
      setTableID(bill.account.table_id);
      const table = overview.data?.tables.find((entry) => entry.id === bill.account.table_id);
      if (table) setAreaID(table.area_id);
    }
  }, [view, bill?.account.table_id, tableID, overview.data]);

  // Cada cuenta que llega del servidor se concilia con los toques locales (conflicto explícito si
  // otra terminal la cambió mientras había toques sin guardar).
  useEffect(() => {
    if (fetched) draft.engine.observe(fetched);
  }, [fetched, draft.engine]);

  useEffect(() => {
    if (refresh === 0) return;
    void overview.reload();
    void accounts.reload();
    void resource.reload();
    void savedQuote.reload();
    void lookups.reload();
  }, [
    refresh,
    overview.reload,
    accounts.reload,
    resource.reload,
    savedQuote.reload,
    lookups.reload,
  ]);

  useEffect(() => {
    if (bill?.account.revision) void savedQuote.reload();
  }, [bill?.account.revision, savedQuote.reload]);

  async function execute<T>(
    operation: Operation,
    options: { id?: string; body?: Record<string, unknown> },
  ): Promise<T> {
    try {
      return await mutation.execute<T>(operation, options);
    } catch (problem) {
      if (problem instanceof APIError && problem.status === 409)
        setRefresh((current) => current + 1);
      throw problem;
    }
  }

  const capture = useCaptureDraft({
    bill,
    engine: draft.engine,
    can,
    savedQuote,
    execute,
    onError: setActionError,
    onConflict: () => setRefresh((current) => current + 1),
  });

  // Envía de inmediato los toques pendientes de la cuenta que se deja de mostrar.
  function flushCurrent() {
    if (selectedID) draft.engine.flushNow(selectedID);
  }

  function chooseAccount(id: string) {
    flushCurrent();
    setSelections((current) => ({ ...current, [view]: id }));
    setPanel('capture');
    setActionError('');
  }

  function chooseTable(id: string) {
    flushCurrent();
    setTableID(id);
    setSelections((current) => ({ ...current, tables: '' }));
    setPanel('capture');
    setPage(1);
    setSearch('');
  }

  function switchView(next: RestaurantView) {
    flushCurrent();
    setView(next);
    setPanel('capture');
    setCaptureOpen(next, false);
    setSearch('');
    setPage(1);
    setState('open');
    setActionError('');
  }

  // «Mesas» vuelve al mapa; en Para llevar y Cuentas, al listado.
  function leaveCapture() {
    flushCurrent();
    setActionError('');
    if (view === 'tables') {
      chooseTable('');
      return;
    }
    setCaptureOpen(view, false);
    setPanel('capture');
  }

  function enterCapture() {
    setCaptureOpen(view, true);
    setPanel('capture');
    setActionError('');
  }

  // Abre un panel de la cuenta. Las acciones que escriben parten de la cuenta sincronizada: se
  // envían antes los toques pendientes para que su revisión sea la vigente.
  async function openPanel(next: WorkspacePanel) {
    if (bill && !readOnlyPanels.includes(next)) {
      try {
        await draft.engine.settle(bill.account.id);
      } catch (problem) {
        setActionError(problem instanceof Error ? problem.message : 'No se pudo guardar.');
        return;
      }
    }
    setActionError('');
    setPanel(next);
  }

  function startEditor(
    line: DraftItem | ConfirmedLine | CaptureProduct,
    kind: 'new' | 'draft' | 'confirmed',
    revision?: number,
  ) {
    if (!bill || bill.account.state !== 'open') return;
    if (editor) {
      setActionError(
        'Conservas una selección sin guardar. Retómala o descártala antes de editar otro producto.',
      );
      return;
    }
    const request =
      kind === 'new'
        ? {
            product_id: (line as CaptureProduct).id,
            variant_id: null,
            quantity: 1,
            customization: emptyCustomization(),
          }
        : kind === 'draft'
          ? normalizeSelection(line as DraftItem)
          : normalizeSelection({
              ...(line as ConfirmedLine).request,
              quantity: (line as ConfirmedLine).quantity,
            });
    setEditor({
      accountID: bill.account.id,
      mode: bill.account.mode,
      tableID: bill.account.table_id,
      kind,
      revision: revision ?? bill.account.revision,
      localID: newID(),
      draftID: kind === 'draft' ? (line as DraftItem).id : null,
      confirmedVersionID: kind === 'confirmed' ? (line as ConfirmedLine).version_id : null,
      line: request,
      initialLine: request,
      reason: '',
      basePriceUnavailable: kind === 'new' && !!(line as CaptureProduct).pricing_error,
    });
    // La personalización vive en el panel central de la captura, también desde un listado.
    setCaptureOpen(view, true);
    setPanel('editor');
    setActionError('');
  }

  // Modificar una partida confirmada parte de la cuenta sincronizada (revisión vigente).
  async function startConfirmedEdit(line: ConfirmedLine) {
    if (!bill) return;
    try {
      const synced = await draft.engine.settle(bill.account.id);
      startEditor(line, 'confirmed', synced?.baseRevision ?? bill.account.revision);
    } catch (problem) {
      setActionError(problem instanceof Error ? problem.message : 'No se pudo guardar.');
    }
  }

  // Guarda la personalización del panel central en el borrador local.
  function saveEditorLine() {
    if (!editor || !bill || editor.accountID !== bill.account.id) return;
    if (capture.addEditorLine(editor)) {
      setEditor(null);
      setPanel('capture');
    }
  }

  function discardEditor() {
    setEditor(null);
    setPanel('capture');
  }

  function resumeEditor() {
    if (!editor) return;
    flushCurrent();
    const destination: RestaurantView =
      editor.mode === 'takeaway' ? 'takeaway' : editor.tableID ? 'tables' : 'accounts';
    setView(destination);
    setSelections((current) => ({ ...current, [destination]: editor.accountID }));
    if (editor.tableID) setTableID(editor.tableID);
    setCaptureOpen(destination, true);
    setPanel('editor');
  }

  const recoverDraft = useCallback(() => draft.engine.recover(), [draft.engine]);

  return {
    view,
    panel,
    setPanel,
    openPanel,
    areaID,
    setAreaID: (id: string) => {
      setAreaID(id);
      chooseTable('');
    },
    tableID,
    chooseTable,
    selectedID,
    chooseAccount,
    switchView,
    captureMode,
    enterCapture,
    leaveCapture,
    search,
    setSearch,
    state,
    setState,
    page,
    setPage,
    overview,
    lookups,
    accounts,
    resource,
    bill,
    savedQuote,
    editor,
    setEditor,
    startEditor,
    startConfirmedEdit,
    saveEditorLine,
    discardEditor,
    resumeEditor,
    cancelLine,
    setCancelLine,
    newMode,
    setNewMode,
    actionError,
    setActionError,
    refresh: () => setRefresh((current) => current + 1),
    mutation,
    execute,
    draft: draft.engine,
    draftView: capture.draftView,
    localDraft: capture.localDraft,
    persistedDraft: capture.persistedDraft,
    draftRevision: capture.draftRevision,
    addProduct: capture.addProduct,
    changeQuantity: capture.changeQuantity,
    sending: capture.sending,
    sendToKitchen: capture.sendToKitchen,
    recoverDraft,
  };
}
