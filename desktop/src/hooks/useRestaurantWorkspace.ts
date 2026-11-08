import { useCallback, useEffect, useState } from 'react';
import { APIError } from '../services/api';
import type { Operation } from '../services/api';
import type { Lookups, Product } from '../services/catalog';
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
import { useLiveResource } from './useLiveResource';
import { useRestaurantMutation } from './useRestaurantMutation';
import type { AccountAction } from '../pages/restaurant/AccountActions';

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
  | AccountAction;

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
  const [search, setSearch] = useState('');
  const [state, setState] = useState('open');
  const [page, setPage] = useState(1);
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [tentativeQuote, setTentativeQuote] = useState<OrderQuote | null>(null);
  const [cancelLine, setCancelLine] = useState<ConfirmedLine | null>(null);
  const [newMode, setNewMode] = useState<'dine_in' | 'takeaway'>('dine_in');
  const [actionError, setActionError] = useState('');
  const [refresh, setRefresh] = useState(0);
  const canMap = can('restaurant.areas.read') && can('restaurant.tables.read');
  const selectedID = selections[view];
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
  const bill = resource.data?.account.id === selectedID ? resource.data : null;
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
      if (operation === 'restaurant_save_draft' || operation === 'restaurant_edit') {
        setEditor((current) => (current?.accountID === record.account?.id ? null : current));
        setTentativeQuote(null);
      }
      const account = record.account ?? record.target?.account;
      if (account) {
        const destination: RestaurantView =
          account.mode === 'takeaway' ? 'takeaway' : account.table_id ? 'tables' : 'accounts';
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

  const receivePreview = useCallback((value: OrderQuote | null) => setTentativeQuote(value), []);

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

  function chooseAccount(id: string) {
    setSelections((current) => ({ ...current, [view]: id }));
    setPanel('capture');
    setActionError('');
  }

  function chooseTable(id: string) {
    setTableID(id);
    setSelections((current) => ({ ...current, tables: '' }));
    setPanel('capture');
    setPage(1);
    setSearch('');
  }

  function switchView(next: RestaurantView) {
    setView(next);
    setPanel('capture');
    setSearch('');
    setPage(1);
    setState('open');
    setActionError('');
  }

  function startEditor(
    line: DraftItem | ConfirmedLine | Product,
    kind: 'new' | 'draft' | 'confirmed',
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
            product_id: (line as Product).id,
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
      revision: bill.account.revision,
      baseDraft: bill.draft,
      localID: crypto.randomUUID(),
      draftID: kind === 'draft' ? (line as DraftItem).id : null,
      confirmedVersionID: kind === 'confirmed' ? (line as ConfirmedLine).version_id : null,
      line: request,
      reason: '',
    });
    setPanel('editor');
    setActionError('');
    setTentativeQuote(null);
  }

  async function confirm() {
    if (
      !bill ||
      !savedQuote.data?.fingerprint ||
      savedQuote.data.revision !== bill.account.revision
    )
      return;
    try {
      await execute('restaurant_confirm', {
        id: bill.account.id,
        body: {
          revision: bill.account.revision,
          expected_fingerprint: savedQuote.data.fingerprint,
        },
      });
    } catch (problem) {
      setActionError(problem instanceof Error ? problem.message : 'No se pudo confirmar.');
    }
  }

  async function removeDraft(item: DraftItem) {
    if (!bill) return;
    if (editor) {
      setActionError('Guarda o descarta la selección pendiente antes de quitar una partida.');
      return;
    }
    try {
      await execute('restaurant_save_draft', {
        id: bill.account.id,
        body: {
          revision: bill.account.revision,
          items: bill.draft.filter((entry) => entry.id !== item.id),
        },
      });
    } catch (problem) {
      setActionError(
        problem instanceof Error ? problem.message : 'No se pudo retirar del borrador.',
      );
    }
  }

  function resumeEditor() {
    if (!editor) return;
    const destination: RestaurantView =
      editor.mode === 'takeaway' ? 'takeaway' : editor.tableID ? 'tables' : 'accounts';
    setView(destination);
    setSelections((current) => ({ ...current, [destination]: editor.accountID }));
    if (editor.tableID) setTableID(editor.tableID);
    setPanel('editor');
  }

  return {
    view,
    panel,
    setPanel,
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
    tentativeQuote,
    receivePreview,
    startEditor,
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
    confirm,
    removeDraft,
  };
}
