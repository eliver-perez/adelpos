import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form } from '../../components/Forms';
import { PositionField } from '../../components/PositionField';
import { SegmentedControl } from '../../components/SegmentedControl';
import { ToggleButton } from '../../components/ToggleButton';
import { useRestaurantMutation } from '../../hooks/useRestaurantMutation';
import { acceptAmountText, toWireAmount } from '../../services/amountInput';
import { currencyLabels, denominationLabel } from '../../services/cash';
import { notifyError, notifySuccess } from '../../services/notify';
import type { CashCatalog, CashCurrency, Denomination, ExpenseCategory } from '../../services/cash';
import { orderSiblings, positionPayload } from '../../services/positions';
import { APIError } from '../../services/api';
import type { Operation } from '../../services/api';
import { CashLimitAdmin, DrawerAdmin } from './CashHardwareSettings';

// Escrituras de configuración de caja con su propia clave de pendientes.
const settingsPendingKey = 'cash:settings';

type Execute = <T>(
  operation: Operation,
  options: { id?: string; body?: Record<string, unknown> },
) => Promise<T>;

function changed() {
  window.dispatchEvent(new CustomEvent('adel-server-change'));
}

// Alta de una denominación: moneda, billete o moneda y valor. Valor, moneda y tipo no cambian
// después; una denominación en desuso se desactiva.
function NewDenomination({ execute }: { execute: Execute }) {
  const [currency, setCurrency] = useState<CashCurrency>('MXN');
  const [kind, setKind] = useState<'bill' | 'coin'>('coin');
  const [value, setValue] = useState('');
  return (
    <Form
      label="Agregar denominación"
      successMessage="Denominación agregada."
      onSave={async () => {
        await execute('payments_create_denomination', {
          body: { currency, kind, value: toWireAmount(value), active: true },
        });
        setValue('');
      }}
    >
      <div className="form-grid">
        <div className="field">
          <span>Moneda</span>
          <SegmentedControl<CashCurrency>
            label="Moneda de la denominación"
            value={currency}
            onChange={setCurrency}
            options={[
              { value: 'MXN', label: 'Pesos' },
              { value: 'USD', label: 'Dólares' },
            ]}
          />
        </div>
        <div className="field">
          <span>Tipo</span>
          <SegmentedControl<'bill' | 'coin'>
            label="Tipo de denominación"
            value={kind}
            onChange={setKind}
            options={[
              { value: 'bill', label: 'Billete' },
              { value: 'coin', label: 'Moneda' },
            ]}
          />
        </div>
        <Field label="Valor">
          <input
            required
            type="text"
            inputMode="decimal"
            autoComplete="off"
            placeholder="0.00"
            value={value}
            onChange={(event) => setValue(acceptAmountText(event.target.value, value))}
          />
        </Field>
      </div>
    </Form>
  );
}

function DenominationsAdmin({
  denominations,
  execute,
  busy,
}: {
  denominations: Denomination[];
  execute: Execute;
  busy: boolean;
}) {
  const currencies: CashCurrency[] = ['MXN', 'USD'];

  async function toggle(denomination: Denomination, active: boolean) {
    const label = denominationLabel(denomination);
    try {
      await execute('payments_save_denomination', {
        id: denomination.id,
        body: { revision: denomination.revision, active },
      });
      notifySuccess(
        active ? `Denominación activada: ${label}.` : `Denominación desactivada: ${label}.`,
      );
    } catch (problem) {
      notifyError(problem, 'No se pudo guardar la denominación.');
    }
  }

  return (
    <div className="form-stack">
      {currencies.map((currency) => (
        <div key={currency} className="field">
          <span>{currencyLabels[currency]}</span>
          <div
            className="cash-choice-grid"
            role="group"
            aria-label={`Denominaciones en ${currencyLabels[currency]}`}
          >
            {denominations
              .filter((denomination) => denomination.currency === currency)
              .map((denomination) => (
                <ToggleButton
                  key={denomination.id}
                  label={denominationLabel(denomination)}
                  pressed={denomination.active}
                  disabled={busy}
                  onChange={(active) => void toggle(denomination, active)}
                />
              ))}
          </div>
        </div>
      ))}
      <p className="muted text-xs">
        Una denominación activa aparece en la apertura y en el arqueo. Desactivarla no cambia los
        conteos registrados.
      </p>
      <NewDenomination execute={execute} />
    </div>
  );
}

// Edición de una categoría: nombre, estado y posición en la lista (orden por posiciones).
function CategoryEditor({
  category,
  categories,
  execute,
  onClose,
}: {
  category: ExpenseCategory | null;
  categories: ExpenseCategory[];
  execute: Execute;
  onClose: () => void;
}) {
  const [name, setName] = useState(category?.name ?? '');
  const [active, setActive] = useState(category?.active ?? true);
  const [after, setAfter] = useState<string | undefined>(undefined);
  const siblings = categories.map((item) => ({
    id: item.id,
    name: item.name,
    note: item.active ? undefined : 'inactiva',
  }));
  return (
    <Form
      label={category ? 'Guardar categoría' : 'Crear categoría'}
      successMessage={category ? 'Categoría de gasto guardada.' : 'Categoría de gasto creada.'}
      onSave={async () => {
        const body: Record<string, unknown> = { name, active };
        const position = positionPayload(categories, category?.id ?? '', after);
        if (position) {
          body.position = position;
        }
        if (category) {
          await execute('payments_save_category', {
            id: category.id,
            body: { ...body, revision: category.revision },
          });
        } else {
          await execute('payments_create_category', { body });
        }
        onClose();
      }}
    >
      <div className="form-grid">
        <Field label="Nombre de la categoría">
          <input
            required
            maxLength={80}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <PositionField
          label="Posición de la categoría"
          siblings={siblings}
          recordID={category?.id ?? ''}
          value={after}
          onChange={setAfter}
        />
      </div>
      <ToggleButton label="Categoría activa" pressed={active} onChange={setActive} />
      <button type="button" className="secondary-button" onClick={onClose}>
        Cancelar
      </button>
    </Form>
  );
}

function CategoriesAdmin({
  categories,
  execute,
}: {
  categories: ExpenseCategory[];
  execute: Execute;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const ordered = orderSiblings(categories);
  const current = ordered.find((category) => category.id === editing) ?? null;
  return (
    <div className="form-stack">
      <div className="cash-choice-grid" role="group" aria-label="Categorías de gasto">
        {ordered.map((category) => (
          <button
            type="button"
            key={category.id}
            className={editing === category.id ? 'cash-choice-selected' : ''}
            aria-pressed={editing === category.id}
            onClick={() => setEditing(category.id)}
          >
            {category.name}
            {!category.active && <span className="muted text-xs"> · inactiva</span>}
          </button>
        ))}
        <button type="button" onClick={() => setEditing('')}>
          + Nueva categoría
        </button>
      </div>
      {editing !== null && (
        <CategoryEditor
          key={editing}
          category={current}
          categories={ordered}
          execute={execute}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}

// Configuración de caja (cash.settings.manage): denominaciones, categorías de gasto, tope de caja
// (función activable) y cajón de dinero por caja.
export function CashSettingsView({ catalog }: { catalog: CashCatalog | null }) {
  const mutation = useRestaurantMutation(changed, settingsPendingKey);
  if (!catalog) {
    return <p className="muted">Consultando la configuración de caja…</p>;
  }
  // Reintento del cambio incierto: su resultado definitivo se avisa como el de cualquier acción.
  async function retry() {
    const problem = await mutation.retry();
    if (!problem) {
      notifySuccess('Cambio de configuración confirmado en el servidor.');
      return;
    }
    if (!mutation.isPending()) {
      notifyError(problem, 'No se pudo confirmar el cambio.');
    }
  }
  const execute: Execute = async (operation, options) => {
    try {
      return await mutation.execute(operation, options);
    } catch (problem) {
      // Una revisión obsoleta o un nombre repetido se resuelven consultando la versión vigente.
      if (problem instanceof APIError && problem.status === 409) {
        changed();
      }
      throw problem;
    }
  };
  return (
    <div className="form-stack">
      {mutation.uncertain && (
        <div className="operation-recovery">
          <p>No se pudo confirmar el último cambio. Reintenta con el mismo identificador.</p>
          <button
            type="button"
            className="primary-button"
            disabled={mutation.busy}
            onClick={() => void retry()}
          >
            Reintentar con el mismo identificador
          </button>
        </div>
      )}
      <Accordion
        title="Denominaciones"
        summary="Billetes y monedas del arqueo y de la apertura"
        defaultOpen
      >
        <DenominationsAdmin
          denominations={catalog.denominations}
          execute={execute}
          busy={mutation.busy || !!mutation.pending}
        />
      </Accordion>
      <Accordion title="Categorías de gasto" summary="Nombre, estado y orden de las categorías">
        <CategoriesAdmin categories={catalog.expense_categories} execute={execute} />
      </Accordion>
      <Accordion title="Tope de caja" summary="Aviso de efectivo excedido; nunca bloquea cobros">
        <CashLimitAdmin execute={execute} />
      </Accordion>
      <Accordion
        title="Cajón de dinero"
        summary="Impresora ESC/POS, conector y activación por caja"
      >
        <DrawerAdmin execute={execute} />
      </Accordion>
    </div>
  );
}
