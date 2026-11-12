import { useState } from 'react';
import { Form, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { IntegerField, ReferenceField, TextField } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import { formatMoney } from '../../services/money';
import { pluralWord } from '../../services/plural';
import { accountCustomName, accountLabel } from '../../services/restaurant';
import type { AccountPage, Bill, Overview } from '../../services/restaurant';
import type { ExecuteMutation } from './OperationsAdmin';

export type AccountAction = 'split' | 'transfer' | 'merge' | 'move' | 'assign' | 'void';
const titles = {
  split: 'Dividir cuenta',
  transfer: 'Transferir productos',
  merge: 'Combinar cuentas',
  move: 'Cambiar mesa',
  assign: 'Cambiar responsable',
  void: 'Anular cuenta vacía',
};

// Confirmación que muestra el aviso de éxito al terminar cada acción.
const successMessages = {
  split: 'Cuenta dividida',
  transfer: 'Productos transferidos',
  merge: 'Cuentas combinadas',
  move: 'Mesa de la cuenta cambiada',
  assign: 'Responsable de la cuenta cambiado',
  void: 'Cuenta anulada',
};

export function AccountActions({
  action,
  bill,
  overview,
  execute,
  busy,
  onClose,
}: {
  action: AccountAction;
  bill: Bill;
  overview: Overview | null;
  execute: ExecuteMutation;
  busy: boolean;
  onClose: () => void;
}) {
  const [revision, setRevision] = useState(bill.account.revision);
  const [targetID, setTargetID] = useState('');
  const [tableID, setTableID] = useState(bill.account.table_id ?? '');
  const [staffID, setStaffID] = useState(bill.account.assigned_user_id);
  // Una cuenta sin nombre capturado (referencia neutra) propone un nombre a partir de su folio.
  const [name, setName] = useState(() => {
    const custom = accountCustomName(bill.account);
    return custom ? `${custom} · separada` : `Separada de la cuenta #${bill.account.folio}`;
  });
  const [reason, setReason] = useState('');
  const [quantities, setQuantities] = useState<Record<string, number>>({});
  const [search, setSearch] = useState('');
  const candidates = useLiveResource<AccountPage>('restaurant_accounts', {
    enabled: action === 'transfer' || action === 'merge',
    query: { mode: bill.account.mode, search, page_size: '100' },
  });
  const staff = useLiveResource<{ id: string; name: string }[]>('restaurant_staff', {
    enabled: action === 'assign',
  });
  const targets =
    candidates.data?.items.filter(
      (account) =>
        account.id !== bill.account.id && account.price_list_id === bill.account.price_list_id,
    ) ?? [];
  const target = targets.find((account) => account.id === targetID);
  const stale = revision !== bill.account.revision;
  const activeLines = bill.lines.filter((line) => line.state === 'active');

  return (
    <section className="operation-inline-panel">
      <div className="section-heading">
        <h2>
          {titles[action]} · #{bill.account.folio}
        </h2>
        <button className="secondary-button" onClick={onClose}>
          <IconLabel icon="back">Volver</IconLabel>
        </button>
      </div>
      <Notice>{candidates.error || staff.error}</Notice>
      {stale && (
        <Notice>
          La cuenta cambió. Revisa sus partidas; las cantidades seleccionadas se conservan.
        </Notice>
      )}
      {stale && (
        <button className="secondary-button" onClick={() => setRevision(bill.account.revision)}>
          Revisé los cambios de esta cuenta
        </button>
      )}
      <Form
        disabled={busy || stale}
        label={titles[action]}
        successMessage={successMessages[action]}
        tone={action === 'void' ? 'danger' : 'default'}
        onSave={async () => {
          const body: Record<string, unknown> = { revision };
          if (action === 'split' || action === 'transfer') {
            body.selections = Object.entries(quantities)
              .filter(([, quantity]) => quantity > 0)
              .map(([version_id, quantity]) => ({ version_id, quantity }));
          }
          if (action === 'split') body.new_account = { name };
          if (action === 'transfer' || action === 'merge') {
            if (!target) throw new Error('Selecciona una cuenta de destino vigente.');
            body.target_account_id = target.id;
            body.target_revision = target.revision;
          }
          if (action === 'move') body.table_id = tableID || null;
          if (action === 'assign') body.assigned_user_id = staffID;
          if (action === 'void') body.reason = reason;
          await execute(`restaurant_${action}`, { id: bill.account.id, body });
          onClose();
        }}
      >
        {(action === 'split' || action === 'transfer') && (
          <>
            <p className="muted text-sm">
              Selecciona unidades enteras. Conservan sus personalizaciones, identidad y centavos
              históricos; esta operación no confirma otro pedido.
            </p>
            {activeLines.map((line) => (
              <IntegerField
                key={line.version_id}
                label={`${line.quote_line.name} · ${line.quantity} ${pluralWord(line.quantity, 'disponible', 'disponibles')} · ${formatMoney(line.totals.total)}`}
                value={quantities[line.version_id] ?? 0}
                max={line.quantity}
                onChange={(quantity) =>
                  setQuantities({ ...quantities, [line.version_id]: quantity })
                }
              />
            ))}
          </>
        )}
        {action === 'split' && (
          <TextField
            required
            label="Nombre de la cuenta resultante"
            value={name}
            onChange={setName}
          />
        )}
        {(action === 'transfer' || action === 'merge') && (
          <>
            <TextField
              label="Buscar cuenta de destino por folio o referencia"
              value={search}
              onChange={setSearch}
            />
            <ReferenceField
              label="Cuenta de destino compatible"
              value={targetID}
              options={targets.map((account) => ({
                id: account.id,
                name: `${accountLabel(account)} · ${account.table_name ?? 'sin mesa'} · ${formatMoney(account.totals.total)}`,
              }))}
              onChange={setTargetID}
            />
            {action === 'merge' && (
              <Notice>
                Se trasladarán todas las unidades activas y borradores. La cuenta de origen quedará
                identificada como combinada y conservará su historial.
              </Notice>
            )}
          </>
        )}
        {action === 'move' && (
          <ReferenceField
            optional
            label="Mesa de destino (vacío: sin mesa)"
            value={tableID}
            options={
              overview?.tables.filter((table) => table.active && !table.out_of_service) ?? []
            }
            onChange={setTableID}
          />
        )}
        {action === 'assign' && (
          <ReferenceField
            label="Responsable de la cuenta"
            value={staffID}
            options={staff.data ?? []}
            onChange={setStaffID}
          />
        )}
        {action === 'void' && (
          <TextField
            required
            label="Motivo de anulación de cuenta vacía"
            value={reason}
            onChange={setReason}
          />
        )}
      </Form>
    </section>
  );
}
