// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AppToaster } from '../../../components/AppToaster';
import type { Account } from '../../../services/restaurant';
import type { CaptureProduct } from './captureRules';
import { ProductGrid } from './ProductGrid';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

// Etapa 9G (§5.4): «Agotado» desde la captura. El producto agotado se ve marcado y no se puede tocar para
// agregarlo; con catalog.sold_out.manage, «Marcar agotados» cambia el toque a «Agotado» / «Disponible otra vez» con
// la duración elegida. Go decide la duración y el rechazo; aquí solo se comprueba qué se envía y qué se ve.

function product(id: string, name: string, extra: Partial<CaptureProduct> = {}): CaptureProduct {
  return {
    id,
    name,
    description: '',
    active: true,
    revision: 1,
    category_id: 'category-1',
    code: id,
    barcode: null,
    kind: 'product',
    base_price: '120.00',
    tax_rate: null,
    price_can_decrease: false,
    image_id: null,
    sort_order: 0,
    archived: false,
    temporary_unavailable: false,
    availability: { days: [], windows: [] },
    variants: [],
    ingredients: [],
    groups: [],
    slots: [],
    available_now: true,
    availability_reason: '',
    effective_price: '120.00',
    sold_out: false,
    sold_out_indefinite: false,
    sold_out_until: null,
    ...extra,
  } as CaptureProduct;
}

let items: CaptureProduct[] = [];
const sent: {
  operation: string;
  resourceId: string | null;
  body: Record<string, unknown> | null;
}[] = [];

beforeEach(() => {
  sent.length = 0;
  items = [
    product('burger', 'Hamburguesa'),
    product('shake', 'Malteada de fresa', {
      available_now: false,
      availability_reason: 'Agotado.',
      sold_out: true,
      sold_out_until: '2026-11-14T11:00:00Z',
    }),
  ];
  native.invoke.mockImplementation(
    async (
      command: string,
      args: { operation: string; resourceId: string | null; body: Record<string, unknown> | null },
    ) => {
      if (command !== 'api_request') {
        return null;
      }
      if (args.operation === 'catalog_products') {
        return { status: 200, body: { items, total: items.length } };
      }
      if (args.operation === 'catalog_product_sold_out') {
        sent.push({ operation: args.operation, resourceId: args.resourceId, body: args.body });
        const target = items.find((entry) => entry.id === args.resourceId)!;
        const soldOut = args.body?.sold_out === true;
        items = items.map((entry) =>
          entry.id === target.id
            ? {
                ...entry,
                sold_out: soldOut,
                available_now: !soldOut,
                sold_out_indefinite: args.body?.indefinite === true,
              }
            : entry,
        );
        return {
          status: 200,
          body: {
            product_id: target.id,
            name: target.name,
            sold_out: soldOut,
            indefinite: args.body?.indefinite === true,
            sold_out_at: null,
            sold_out_until: null,
            catalog_version: 7,
          },
        };
      }
      return { status: 200, body: {} };
    },
  );
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const account = {
  id: 'account-1',
  price_list_id: 'list-1',
  state: 'open',
  mode: 'dine_in',
} as unknown as Account;

function renderGrid(canMarkSoldOut: boolean, onAdd = vi.fn()) {
  render(
    <>
      <AppToaster />
      <ProductGrid
        account={account}
        lookups={null}
        categoryID="category-1"
        search=""
        onSearch={() => undefined}
        showImages={false}
        onShowImages={() => undefined}
        enabled
        active
        writable
        canMarkSoldOut={canMarkSoldOut}
        onAdd={onAdd}
        onCustomize={() => undefined}
      />
    </>,
  );
  return onAdd;
}

describe('«Agotado» en la captura', () => {
  it('marca el producto agotado y no deja agregarlo; sin el permiso no ofrece «Marcar agotados»', async () => {
    const onAdd = renderGrid(false);
    await screen.findByText('Agotado');
    const shake = screen.getByRole('button', { name: /^Malteada de fresa/ });
    expect((shake as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByRole('button', { name: 'Marcar agotados' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /^Hamburguesa/ }));
    expect(onAdd).toHaveBeenCalledTimes(1);
  });

  it('con un toque marca «Agotado» hasta nuevo aviso y «Disponible otra vez», sin agregar a la cuenta', async () => {
    const onAdd = renderGrid(true);
    await screen.findByText('Agotado');
    fireEvent.click(screen.getByRole('button', { name: 'Marcar agotados' }));
    expect(screen.getByText(/Toca un producto para marcarlo como agotado/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Hasta nuevo aviso' }));
    fireEvent.click(screen.getByRole('button', { name: 'Marcar como agotado: Hamburguesa' }));
    await screen.findByText('«Hamburguesa» quedó agotado hasta nuevo aviso.');
    expect(sent[0]).toMatchObject({
      resourceId: 'burger',
      body: { sold_out: true, indefinite: true },
    });
    expect(typeof sent[0]?.body?.operation_id).toBe('string');
    await screen.findByText('Agotado hasta nuevo aviso');
    // El producto agotado también se vuelve a ofrecer con un toque.
    fireEvent.click(
      await screen.findByRole('button', { name: 'Disponible otra vez: Malteada de fresa' }),
    );
    await screen.findByText('«Malteada de fresa» está disponible otra vez.');
    expect(sent[1]).toMatchObject({
      resourceId: 'shake',
      body: { sold_out: false, indefinite: false },
    });
    expect(sent[0]?.body?.operation_id).not.toBe(sent[1]?.body?.operation_id);
    expect(onAdd).not.toHaveBeenCalled();
    // Al salir del modo, el toque vuelve a agregar productos.
    fireEvent.click(screen.getByRole('button', { name: 'Marcar agotados' }));
    await waitFor(() => expect(screen.queryByText(/Toca un producto para marcarlo/)).toBeNull());
    fireEvent.click(screen.getByRole('button', { name: /^Malteada de fresa/ }));
    expect(onAdd).toHaveBeenCalledTimes(1);
  });
});
