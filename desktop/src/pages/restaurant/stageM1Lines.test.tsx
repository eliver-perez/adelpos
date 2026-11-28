// @vitest-environment jsdom

import { cleanup, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Task } from '../../services/kitchen';
import type {
  Account,
  AccountPage,
  Bill,
  ConfirmedLine,
  RestaurantTable,
} from '../../services/restaurant';
import { PreparationTaskCard } from '../kitchen/PreparationTaskCard';
import { AccountList } from './AccountList';
import { ConfirmedLines } from './capture/BillLines';
import { TablePanel } from './TablePanel';

vi.mock('@tauri-apps/api/core', () => ({ invoke: vi.fn(), isTauri: () => true }));

// Etapa M1 (§5.1, §5.2 y §5.4; M1-D15, M1-D18 y M1-D34). Empezó como la prueba de reproducción de la Fase 0: Go ya
// mandaba el estado de cocina de cada renglón (preparation_state: released, preparing, ready…) y Android lo mostraba
// («Listo»), pero la captura ponía siempre la etiqueta «En cocina» con el estado real en gris al lado
// (BillLines.tsx) y el panel de la mesa ni lo leía («En cocina» fijo en TablePanel.tsx). El caso del propietario: la
// cuenta de la Mesa 1 después de «Pedido listo» en la pantalla de cocina. Ahora las dos vistas usan la etiqueta de
// Go (lineKitchenTag) y lo que no necesita preparación no lleva ninguna.

afterEach(cleanup);

const accountID = '00000000-0000-4000-8000-0000000000c1';
const tableID = '00000000-0000-4000-8000-0000000000b1';

function line(name: string, state: string, values: Partial<ConfirmedLine> = {}): ConfirmedLine {
  return {
    preparation_state: state,
    version_id: `00000000-0000-4000-8000-${name.length.toString().padStart(4, '0')}${state.length
      .toString()
      .padStart(8, '0')}`,
    item_id: `item-${name}`,
    original_order_id: 'order-1',
    version: 1,
    state: 'active',
    quantity: 1,
    request: { product_id: `product-${name}`, quantity: 1, customization: {} },
    quote_line: {
      product_id: `product-${name}`,
      revision: 1,
      name,
      base_price: '120.00',
      price_origin: 'base',
      unit_price: '120.00',
      ingredient_extras: '0.00',
      group_extras: '0.00',
      component_extras: '0.00',
      notes: '',
      ingredients: [],
      groups: [],
      components: [],
      quantity: 1,
      subtotal: '120.00',
      discount: '0.00',
      total: '120.00',
    },
    totals: { subtotal: '120.00', discount: '0.00', total: '120.00' },
    created_at: '2026-11-27T18:00:00Z',
    ...values,
  } as unknown as ConfirmedLine;
}

function tagOf(container: HTMLElement, name: string) {
  const row = within(container).getByText(new RegExp(name)).closest('article, li') as HTMLElement;
  return row.querySelector('.capture-tag')?.textContent ?? '(sin etiqueta)';
}

const account = {
  id: accountID,
  folio: 26,
  name: 'Mesa 1',
  mode: 'dine_in',
  state: 'open',
  table_id: tableID,
  table_name: 'Mesa 1',
  price_list_id: 'list',
  customer_name: '',
  phone: '',
  origin: '',
  notes: '',
  scheduled_at: null,
  scheduled_local: '',
  schedule_kind: null,
  assigned_user_id: 'user',
  assigned_name: 'Laura',
  created_at: '2026-11-27T18:00:00Z',
  updated_at: '2026-11-27T18:00:00Z',
  revision: 3,
  merged_into_id: null,
  elapsed_seconds: 600,
  urgency: 'normal',
  preparation_state: 'ready',
  payment_state: 'not_recorded',
  delivery_state: 'not_delivered',
  finance: null,
  confirmed_quantity: 2,
  draft_quantity: 0,
  totals: { subtotal: '240.00', discount: '0.00', total: '240.00' },
  releasable: false,
  discardable: false,
  cancel_blocker: '',
  cancel_requires_authorization: false,
  long_open: false,
} as unknown as Account;

const table = {
  id: tableID,
  area_id: 'area',
  name: 'Mesa 1',
  shape: 'rectangle',
  x: 20,
  y: 20,
  width: 100,
  height: 72,
  seats: 4,
  active: true,
  out_of_service: false,
  revision: 1,
  account_count: 1,
  state: 'occupied',
  occupied_since: '2026-11-27T18:00:00Z',
  occupied_seconds: 600,
  balance: '240.00',
  long_open: false,
} as RestaurantTable;

function panel(lines: ConfirmedLine[]) {
  const bill = {
    account,
    draft: [],
    lines,
    server_time: '2026-11-27T18:10:00Z',
    timezone: 'America/Monterrey',
    cancellation: null,
  } as unknown as Bill;
  return render(
    <TablePanel
      table={table}
      accounts={[account]}
      bill={bill}
      billError=""
      localDraft={[]}
      lookups={null}
      can={() => true}
      blocked={false}
      onChooseAccount={() => undefined}
      onCapture={() => undefined}
      onOpenAccount={() => undefined}
      onAction={() => undefined}
      onCheckout={() => undefined}
      onRelease={() => undefined}
      onService={() => undefined}
    />,
  );
}

describe('§5.1: la etiqueta de cada renglón sigue el estado real de cocina', () => {
  const lines = [
    line('Hamburguesa sencilla', 'ready'),
    line('Machacado Natural', 'preparing'),
    line('Molletes', 'released'),
  ];

  it('en la captura, un renglón listo dice «Listo» (no «En cocina»)', () => {
    const { container } = render(
      <ConfirmedLines
        lines={lines}
        editable
        can={() => true}
        disabled={false}
        onEdit={() => undefined}
        onCancel={() => undefined}
      />,
    );
    expect(tagOf(container, 'Hamburguesa sencilla')).toBe('Listo');
    expect(tagOf(container, 'Machacado Natural')).toBe('En preparación');
    expect(tagOf(container, 'Molletes')).toBe('En cocina');
  });

  it('en el panel de la mesa, un renglón listo dice «Listo» (no «En cocina»)', () => {
    panel(lines);
    const list = screen.getByRole('list', { name: 'Productos de la cuenta' });
    expect(tagOf(list, 'Hamburguesa sencilla')).toBe('Listo');
    expect(tagOf(list, 'Machacado Natural')).toBe('En preparación');
    expect(tagOf(list, 'Molletes')).toBe('En cocina');
  });
});

describe('§5.2: lo que no necesita preparación no lleva etiqueta de cocina', () => {
  // Go dice por renglón si necesita preparación (preparation_required, Decisión propuesta de la Fase 0); el estado
  // de hoy de esos renglones es «ready» desde el envío (reproducido en la Fase 0).
  const lines = [
    line('Hamburguesa sencilla', 'released', {
      preparation_required: true,
    } as Partial<ConfirmedLine>),
    line('Coca Cola 600ml', 'ready', { preparation_required: false } as Partial<ConfirmedLine>),
  ];

  it('en la captura', () => {
    const { container } = render(
      <ConfirmedLines
        lines={lines}
        editable
        can={() => true}
        disabled={false}
        onEdit={() => undefined}
        onCancel={() => undefined}
      />,
    );
    expect(tagOf(container, 'Hamburguesa sencilla')).toBe('En cocina');
    expect(tagOf(container, 'Coca Cola 600ml')).toBe('(sin etiqueta)');
    expect(within(container).queryByText('Listo')).toBeNull();
  });

  it('en el panel de la mesa', () => {
    panel(lines);
    const list = screen.getByRole('list', { name: 'Productos de la cuenta' });
    expect(tagOf(list, 'Hamburguesa sencilla')).toBe('En cocina');
    expect(tagOf(list, 'Coca Cola 600ml')).toBe('(sin etiqueta)');
  });
});

describe('M1-D34: un renglón cancelado conserva «Cancelado» aunque no necesite preparación', () => {
  it('en la captura, como Android', () => {
    const { container } = render(
      <ConfirmedLines
        lines={[
          line('Coca Cola 600ml', 'cancelled', {
            state: 'cancelled',
            preparation_required: false,
          } as Partial<ConfirmedLine>),
          line('Papas a la francesa', 'ready', {
            preparation_required: false,
          } as Partial<ConfirmedLine>),
        ]}
        editable
        can={() => true}
        disabled={false}
        onEdit={() => undefined}
        onCancel={() => undefined}
      />,
    );
    expect(tagOf(container, 'Coca Cola 600ml')).toBe('Cancelado');
    expect(tagOf(container, 'Papas a la francesa')).toBe('(sin etiqueta)');
  });
});

describe('§5.2 en la lista de cuentas: una cuenta sin preparación no lleva etiqueta de cocina', () => {
  function list(accounts: Account[]) {
    const value: AccountPage = {
      items: accounts,
      total: accounts.length,
      page: 1,
      page_size: 50,
      server_time: '2026-11-27T18:10:00Z',
      timezone: 'America/Monterrey',
      settings: { warning_minutes: 15, overdue_minutes: 30, revision: 1 },
    };
    return render(
      <AccountList
        value={value}
        selectedID=""
        onSelect={() => undefined}
        search=""
        onSearch={() => undefined}
        state="open"
        onStateChange={() => undefined}
        page={1}
        onPageChange={() => undefined}
        error=""
      />,
    );
  }

  it('solo refrescos sin ruta: sin «Listo»; una cuenta mixta sigue con su estado', () => {
    const { container } = list([
      { ...account, folio: 30, preparation_required: false } as Account,
      { ...account, id: 'otra', folio: 31, preparation_state: 'released' } as Account,
    ]);
    const cards = container.querySelectorAll('.account-list-card');
    expect(cards).toHaveLength(2);
    expect(cards[0]!.querySelector('.account-card-preparation')).toBeNull();
    expect(cards[1]!.querySelector('.account-card-preparation')?.textContent).toBe('En cocina');
  });
});

describe('§5.4: Preparación en lenguaje sencillo', () => {
  function task(state: Task['state']): Task {
    return {
      id: `tarea-${state}`,
      unit_id: 'unidad',
      release_id: 'envio',
      station_id: 'estacion',
      state,
      instructions: {
        item_id: 'item',
        version_id: 'version',
        name: 'Hamburguesa sencilla',
        notes: '',
        parts: [],
      },
      revision: 1,
      released_at: '2026-11-27T18:00:00Z',
      folio: 26,
      account_name: 'Mesa 1',
      table_name: 'Mesa 1',
      station_name: 'Cocina',
      warning_minutes: 10,
      overdue_minutes: 20,
      target_minutes: 15,
      elapsed_seconds: 60,
      release_revision: 1,
      tracking: 'item',
    } as unknown as Task;
  }

  it('la tarjeta que llegó y nadie ha empezado dice «Por preparar» (no «Liberado a preparación»)', () => {
    render(
      <PreparationTaskCard
        tasks={[task('released')]}
        canManage
        disabled={false}
        onAdvance={() => undefined}
      />,
    );
    expect(screen.getByText('Por preparar')).toBeTruthy();
    expect(screen.queryByText(/Liberado a preparación/)).toBeNull();
  });

  it('en preparación y lista conservan sus textos', () => {
    render(
      <>
        <PreparationTaskCard
          tasks={[task('preparing')]}
          canManage={false}
          disabled={false}
          onAdvance={() => undefined}
        />
        <PreparationTaskCard
          tasks={[task('ready')]}
          canManage={false}
          disabled={false}
          onAdvance={() => undefined}
        />
      </>,
    );
    expect(screen.getAllByText('En preparación')).toHaveLength(1);
    expect(screen.getByText('Listo')).toBeTruthy();
  });
});
