import { useCallback, useEffect, useRef, useState } from 'react';
import { api, APIError } from '../services/api';
import type { Operation } from '../services/api';
import type { Lookups } from '../services/catalog';
import { notifyError, notifyWarning } from '../services/notify';
import {
  emptyCustomization,
  normalizeSelection,
  pendingAccountBill,
  pendingAccountPrefix,
} from '../services/restaurant';
import type {
  Account,
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
import { DraftSyncError, uncertainError } from '../pages/restaurant/capture/draftSync';

export type RestaurantView = 'tables' | 'takeaway' | 'accounts' | 'manage';
export type WorkspacePanel =
  | 'capture'
  | 'metadata'
  | 'history'
  | 'editor'
  | 'cancel'
  | 'kitchen'
  | 'checkout'
  | 'refund'
  | 'adjustments'
  | 'precheck'
  // Etapa 9D (§5): panel «Cancelar cuenta» con el motivo del catálogo y, si hace falta, supervisor.
  | 'cancelAccount'
  | AccountAction;

// Cuenta pendiente que se muestra en la captura de una mesa sin cuenta (etapa 7.1). Etapa 9D (§4.1,
// decisión 2): también el pedido para llevar y la cuenta sin mesa (sin tableID).
interface PendingAccount {
  localID: string;
  mode: Account['mode'];
  tableID: string | null;
  createdAt: string;
}

// Vista en que se captura una cuenta pendiente: la mesa en Mesas, el pedido en Para llevar y la
// cuenta sin mesa en Cuentas (las mismas vistas a las que llevaba la apertura inmediata).
function pendingView(pending: Pick<PendingAccount, 'mode' | 'tableID'>): RestaurantView {
  if (pending.mode === 'takeaway') {
    return 'takeaway';
  }
  return pending.tableID ? 'tables' : 'accounts';
}

// Cambia en todas las vistas la selección de una cuenta (o de una cuenta pendiente) por otra; con
// '' deja de seleccionarla.
function replaceSelection(current: Record<RestaurantView, string>, from: string, to: string) {
  const views = Object.keys(current) as RestaurantView[];
  if (!views.some((key) => current[key] === from)) {
    return current;
  }
  const next = { ...current };
  for (const key of views) {
    if (next[key] === from) next[key] = to;
  }
  return next;
}

const uncertainResultMessage =
  'La conexión no permitió conocer el resultado. Usa «Recuperar resultado» antes de realizar otra operación.';

// Paneles de solo consulta: no requieren sincronizar el borrador antes de abrirse.
const readOnlyPanels: WorkspacePanel[] = ['capture', 'history', 'kitchen', 'editor'];

function newID() {
  return crypto.randomUUID();
}

export function useRestaurantWorkspace(
  active: boolean,
  requestedView: 'tables' | 'takeaway',
  can: (permission: string) => boolean,
  // Etapa 9C: cambia cada vez que la persona elige Mesas o Para llevar en la barra superior, aunque
  // ya esté en esa sección, para volver a su vista (el mapa o el listado).
  requestKey = 0,
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
  // La captura táctil se abre expresamente: en Mesas desde el panel lateral de la mesa («Capturar»
  // o al abrir una cuenta) y en Para llevar y Cuentas desde el listado. Se recuerda por vista para
  // que llegar desde el menú principal muestre el mapa o el listado de esa vista.
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
  const [refresh, setRefresh] = useState(0);
  const [pending, setPending] = useState<PendingAccount | null>(null);
  // Cuentas que esta terminal creó desde una cuenta pendiente: mientras llega su consulta se
  // muestra la respuesta de la creación.
  const createdIDs = useRef(new Set<string>());
  // Cuentas que esta terminal acaba de descartar, cancelar o liberar: mientras el listado aún no se
  // actualiza, no se vuelven a elegir como primera cuenta del listado.
  const closedIDs = useRef(new Set<string>());
  const canMap = can('restaurant.areas.read') && can('restaurant.tables.read');
  const draft = useDraftSync();
  // Una cuenta pendiente ya creada se sigue por su identificador real.
  const selectedID = draft.engine.resolve(selections[view]);
  // Un identificador local nunca se consulta al servidor: la cuenta todavía no existe (por ejemplo,
  // mientras se crea después de volver al mapa).
  const localSelected = selectedID.startsWith(pendingAccountPrefix);
  const pendingSelected =
    localSelected && !!pending && view === pendingView(pending) && selectedID === pending.localID;
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
    id: selectedID && !localSelected ? selectedID : undefined,
    enabled: active && !!selectedID && !localSelected,
  });
  const fetched = resource.data?.account.id === selectedID ? resource.data : null;
  // La respuesta de un guardado propio del borrador puede ser más reciente que el último sondeo.
  const synced = selectedID && !localSelected ? draft.engine.latestBill(selectedID) : null;
  const pendingTable =
    pendingSelected && pending.tableID
      ? overview.data?.tables.find((entry) => entry.id === pending.tableID)
      : undefined;
  const defaultPriceList =
    lookups.data?.['price-lists'].find((list) => list.is_default && list.active)?.id ?? '';
  const pendingBill =
    pendingSelected && (!pending.tableID || pendingTable)
      ? pendingAccountBill({
          localID: pending.localID,
          mode: pending.mode,
          table: pendingTable ?? null,
          priceListID: defaultPriceList,
          createdAt: pending.createdAt,
          timezone: overview.data?.timezone ?? accounts.data?.timezone ?? 'UTC',
        })
      : null;
  const justCreated = !!selectedID && createdIDs.current.has(selectedID);
  const bill =
    pendingBill ??
    (synced && (fetched ? synced.account.revision > fetched.account.revision : justCreated)
      ? synced
      : fetched);
  const savedQuote = useLiveResource<OrderQuote>('restaurant_preview', {
    id: selectedID && !localSelected ? selectedID : undefined,
    enabled:
      active &&
      !!bill &&
      bill.account.state === 'open' &&
      bill.draft.length > 0 &&
      can('catalog.read') &&
      can('catalog.prices.read'),
  });
  const captureMode = view !== 'manage' && !!selectedID && captureViews[view];

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
        'restaurant_table_service',
      ].includes(operation)
    ) {
      setPanel('capture');
      const record = result as { account?: Bill['account']; target?: Bill };
      if (operation === 'restaurant_edit') {
        setEditor((current) => (current?.accountID === record.account?.id ? null : current));
      }
      const account = record.account ?? record.target?.account;
      if (account?.state === 'voided') {
        // Etapa 9D: la cuenta se descartó, se canceló o se liberó. Deja de mostrarse y se vuelve
        // al mapa o al listado de la vista en que se hizo, sin pasar la captura a otra cuenta.
        closedIDs.current.add(account.id);
        setSelections((current) => replaceSelection(current, account.id, ''));
        setCaptureOpen(view, false);
      } else if (account) {
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
  }, [active, requestedView, canMap, requestKey]);

  // Etapa 7.1: cuando la cuenta pendiente se crea (o sus productos se agregan a otra cuenta de la
  // mesa), la selección, la personalización abierta y el mapa pasan a la cuenta real.
  useEffect(
    () =>
      draft.engine.onAccountCreated((localID, created) => {
        const accountID = created.account.id;
        createdIDs.current.add(accountID);
        setSelections((current) => replaceSelection(current, localID, accountID));
        setEditor((current) =>
          current?.accountID === localID
            ? { ...current, accountID, revision: created.account.revision }
            : current,
        );
        setPending((current) => (current?.localID === localID ? null : current));
        setRefresh((current) => current + 1);
      }),
    [draft.engine],
  );

  useEffect(() => {
    if (!areaID && overview.data?.areas.length)
      setAreaID(
        overview.data.areas.find((area) => area.active)?.id ?? overview.data.areas[0]?.id ?? '',
      );
  }, [overview.data, areaID]);

  useEffect(() => {
    const first = accounts.data?.items.find(
      (entry) => entry.state !== 'open' || !closedIDs.current.has(entry.id),
    );
    if (!selectedID && first) {
      // Solo si la vista sigue sin cuenta elegida: un toque en el mismo instante («Nuevo pedido»,
      // que elige una cuenta pendiente) no se pisa con la primera del listado.
      setSelections((current) => (current[view] ? current : { ...current, [view]: first.id }));
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
      // Un resultado incierto conserva su código (la operación queda pendiente de recuperar),
      // pero su texto remite al panel «Recuperar resultado»: los formularios lo muestran como
      // aviso y no debe parecer un rechazo.
      if (problem instanceof APIError && uncertainError(problem)) {
        throw new APIError(problem.code, uncertainResultMessage, problem.status);
      }
      throw problem;
    }
  }

  // Rechazo de una acción del espacio de trabajo (sistema único de mensajes, etapa 7.1):
  // - una escritura con resultado incierto queda en su panel «Recuperar resultado», sin otro aviso;
  // - un borrador en conflicto o incierto ya tiene su panel en la pantalla: el aviso solo explica
  //   por qué no se continuó y se cierra solo;
  // - los demás errores permanecen hasta que se cierran.
  function reportError(problem: unknown, fallback: string) {
    if (uncertainError(problem) && mutation.isPending()) {
      return;
    }
    if (problem instanceof DraftSyncError && problem.reason !== 'error') {
      notifyWarning(problem.message);
      return;
    }
    notifyError(problem, fallback);
  }

  const capture = useCaptureDraft({
    bill,
    engine: draft.engine,
    can,
    savedQuote,
    execute,
    onError: reportError,
    onConflict: () => setRefresh((current) => current + 1),
  });

  // Envía de inmediato los toques pendientes de la cuenta que se deja de mostrar.
  function flushCurrent() {
    if (selectedID) draft.engine.flushNow(selectedID);
  }

  // Deja de mostrar la cuenta pendiente. Sin productos no queda ningún registro: la mesa sigue
  // libre. Si aún se está creando, la creación continúa y su resultado actualiza el mapa.
  function leavePending() {
    if (!pending) {
      return;
    }
    if (draft.engine.discardPendingIfIdle(pending.localID)) {
      setSelections((current) => replaceSelection(current, pending.localID, ''));
    }
    setPending(null);
  }

  function chooseAccount(id: string) {
    flushCurrent();
    leavePending();
    setSelections((current) => ({ ...current, [view]: id }));
    setPanel('capture');
  }

  function chooseTable(id: string) {
    flushCurrent();
    leavePending();
    setTableID(id);
    setSelections((current) => ({ ...current, tables: '' }));
    setPanel('capture');
    setPage(1);
    setSearch('');
  }

  function switchView(next: RestaurantView) {
    flushCurrent();
    leavePending();
    setView(next);
    setPanel('capture');
    setCaptureOpen(next, false);
    setSearch('');
    setPage(1);
    setState('open');
  }

  // «Mesas» vuelve al mapa con el panel lateral de la misma mesa; en Para llevar y Cuentas, al
  // listado.
  function leaveCapture() {
    flushCurrent();
    leavePending();
    setCaptureOpen(view, false);
    setPanel('capture');
  }

  // Etapa 7.1: «Capturar» en una mesa libre y «Nueva cuenta» en una ocupada entran a la captura
  // sin crear la cuenta. La cuenta se crea en Go con el primer producto guardado (motor del
  // borrador), sabiendo qué cuentas de la mesa veía esta terminal para no abrir una segunda en
  // silencio. Un doble toque solo vuelve a mostrar una cuenta pendiente vacía.
  //
  // Etapa 9D (§4.1 y decisión 2: «ninguna cuenta se guarda sin productos»): «Nuevo pedido» (para
  // llevar) y «Cuenta sin mesa» hacen lo mismo, sin mesa: el pedido o la cuenta nacen con el primer
  // producto y salir sin productos no deja nada guardado.
  function startPendingCapture(table: string | null, mode: Account['mode'] = 'dine_in') {
    flushCurrent();
    leavePending();
    const tableID = mode === 'takeaway' ? null : table;
    const seenAccountIDs = tableID
      ? (accounts.data?.items ?? [])
          .filter((entry) => entry.state === 'open' && entry.table_id === tableID)
          .map((entry) => entry.id)
      : [];
    const localID = `${pendingAccountPrefix}${newID()}`;
    draft.engine.preparePending(localID, { mode, tableID, seenAccountIDs });
    const target = pendingView({ mode, tableID });
    setPending({ localID, mode, tableID, createdAt: new Date().toISOString() });
    if (target !== view) {
      setSearch('');
      setPage(1);
      setState('open');
    }
    setView(target);
    if (tableID) setTableID(tableID);
    setSelections((current) => ({ ...current, [target]: localID }));
    setCaptureOpen(target, true);
    setPanel('capture');
  }

  // Decisión ante TABLE_HAS_ACCOUNT: agregar los productos a la cuenta que abrió otra terminal
  // (con su borrador vigente) o abrir otra cuenta en la mesa a sabiendas.
  const tableConflict = draft.engine.tableConflict();
  const [resolvingConflict, setResolvingConflict] = useState(false);

  async function addPendingToAccount(localID: string, accountID: string) {
    setResolvingConflict(true);
    try {
      const target = await api<Bill>('restaurant_account', { id: accountID });
      draft.engine.addToAccount(localID, target);
    } catch (problem) {
      notifyError(problem, 'No se pudo consultar la cuenta de la mesa.');
    } finally {
      setResolvingConflict(false);
    }
  }

  function openPendingAnyway(localID: string) {
    draft.engine.openNewAccount(localID);
  }

  function enterCapture() {
    setCaptureOpen(view, true);
    setPanel('capture');
  }

  // Abre un panel de la cuenta. Las acciones que escriben parten de la cuenta sincronizada: se
  // envían antes los toques pendientes para que su revisión sea la vigente.
  async function openPanel(next: WorkspacePanel) {
    if (bill && !readOnlyPanels.includes(next)) {
      try {
        await draft.engine.settle(bill.account.id);
      } catch (problem) {
        reportError(problem, 'No se pudo guardar.');
        return;
      }
    }
    setPanel(next);
  }

  function startEditor(
    line: DraftItem | ConfirmedLine | CaptureProduct,
    kind: 'new' | 'draft' | 'confirmed',
    revision?: number,
  ) {
    if (!bill || bill.account.state !== 'open') return;
    if (editor) {
      // La selección conservada sigue visible con «Retomar» y «Descartar»; el aviso explica por
      // qué no se abrió otro producto.
      notifyWarning(
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
  }

  // Modificar una partida confirmada parte de la cuenta sincronizada (revisión vigente).
  async function startConfirmedEdit(line: ConfirmedLine) {
    if (!bill) return;
    try {
      const synced = await draft.engine.settle(bill.account.id);
      startEditor(line, 'confirmed', synced?.baseRevision ?? bill.account.revision);
    } catch (problem) {
      reportError(problem, 'No se pudo guardar.');
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
    startPendingCapture,
    pendingAccount: pendingSelected,
    tableConflict,
    resolvingConflict,
    addPendingToAccount,
    openPendingAnyway,
    reportError,
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
