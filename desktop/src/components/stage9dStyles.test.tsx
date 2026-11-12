// @vitest-environment jsdom

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PendingOperationsProvider } from '../hooks/pendingOperations';
import { TablePanel } from '../pages/restaurant/TablePanel';
import { accountMoreActions } from '../pages/restaurant/capture/BillFooter';
import { MoreActions } from '../pages/restaurant/capture/MoreActions';
import type { Account, RestaurantTable, TableState } from '../services/restaurant';
import { notifyError, notifyInfo, notifySuccess, notifyWarning } from '../services/notify';
import { ChoiceDialog, ConfirmDialog } from './ConfirmDialog';
import { Form } from './Forms';
import { TableMap } from './restaurant/TableMap';
import { tableStateIcons } from './restaurant/tableStateIcons';

vi.mock('@tauri-apps/api/core', () => ({ invoke: vi.fn(), isTauri: () => true }));

// Etapa 9D, fase A (§1): avisos por tipo con su icono, estados de mesa con su icono (también
// «Libre») y estilo destructivo rojo en las acciones que no se pueden deshacer. Los colores de
// cada clase los comprueba src/styles.test.ts sobre los tokens de styles.css.

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

async function advance(milliseconds: number) {
  for (let elapsed = 0; elapsed < milliseconds; elapsed += 25) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(25);
    });
  }
}

describe('avisos por tipo (§1.3)', () => {
  const cases = [
    {
      tone: 'success',
      notify: () => notifySuccess('Cuenta guardada'),
      role: 'status',
      name: 'Operación completada',
      icon: 'success',
    },
    {
      tone: 'info',
      notify: () => notifyInfo('Elige una cuenta con productos'),
      role: 'status',
      name: 'Información',
      icon: 'info',
    },
    {
      tone: 'warning',
      notify: () => notifyWarning('Conservas una selección sin guardar'),
      role: 'status',
      name: 'Atención',
      icon: 'alert',
    },
    {
      tone: 'error',
      notify: () => notifyError(new Error('La confirmación no coincide.')),
      role: 'alert',
      name: 'Error',
      icon: 'error',
    },
  ] as const;

  for (const entry of cases) {
    it(`${entry.tone}: su clase de color y su icono (${entry.icon})`, async () => {
      vi.useFakeTimers({
        toFake: ['setTimeout', 'clearTimeout', 'Date', 'requestAnimationFrame'],
      });
      entry.notify();
      await advance(50);
      const toast = screen.getByRole(entry.role, { name: entry.name });
      expect(toast.className).toContain(`app-toast-${entry.tone}`);
      expect(toast.getAttribute('data-tone')).toBe(entry.tone);
      const icon = toast.querySelector('svg.app-toast-icon');
      expect(icon?.getAttribute('data-icon')).toBe(entry.icon);
    });
  }
});

const area = {
  id: '00000000-0000-4000-8000-00000000d9a1',
  name: 'Salón 9D',
  description: '',
  sort_order: 10,
  active: true,
  revision: 1,
  table_count: 5,
};

const states: TableState[] = [
  'free',
  'occupied',
  'awaiting_payment',
  'partial_payment',
  'out_of_service',
];

// La aserción de tipo admite los campos que agreguen otras fases (p. ej. la marca de cuenta abierta
// por mucho tiempo de §3.4) sin cambiar esta prueba de colores.
function tableIn(state: TableState, index: number): RestaurantTable {
  const busy = state !== 'free' && state !== 'out_of_service';
  const table = {
    id: `00000000-0000-4000-8000-00000000d9b${index}`,
    area_id: area.id,
    name: `Mesa ${index + 1}`,
    shape: 'rectangle',
    x: 20 + index * 180,
    y: 20,
    width: 140,
    height: 110,
    seats: 4,
    active: true,
    out_of_service: state === 'out_of_service',
    revision: 1,
    account_count: busy ? 1 : 0,
    state,
    occupied_since: busy ? '2026-11-12T15:00:00Z' : null,
    occupied_seconds: busy ? 1800 : 0,
    balance: state === 'partial_payment' ? '120.00' : '0.00',
    long_open: false,
  };
  return table as RestaurantTable;
}

describe('estados de mesa con icono (§1.2)', () => {
  it('cada estado tiene icono propio, también «Libre»', () => {
    const icons = states.map((state) => tableStateIcons[state]);
    expect(new Set(icons).size).toBe(states.length);
    expect(tableStateIcons.free).toBe('check');
  });

  it('el mosaico y la leyenda muestran el icono de cada estado', () => {
    render(
      <TableMap
        areas={[area]}
        tables={states.map(tableIn)}
        areaID={area.id}
        onAreaChange={() => undefined}
        onSelect={() => undefined}
      />,
    );
    const canvas = screen.getByLabelText('Mapa de mesas');
    states.forEach((state, index) => {
      const tile = within(canvas).getByRole('button', { name: new RegExp(`^Mesa ${index + 1}, `) });
      expect(tile.className).toContain(`table-${state}`);
      const icon = tile.querySelector('.table-tile-state svg');
      expect(icon?.getAttribute('data-icon'), state).toBe(tableStateIcons[state]);
    });
    const legend = screen.getByRole('list', { name: 'Estados de las mesas' });
    const entries = within(legend).getAllByRole('listitem');
    expect(entries).toHaveLength(states.length);
    states.forEach((state, index) => {
      const entry = entries[index]!;
      expect(entry.querySelector(`.legend-${state}`)).not.toBeNull();
      expect(entry.querySelector('svg')?.getAttribute('data-icon')).toBe(tableStateIcons[state]);
    });
  });

  it('el panel de la mesa muestra el estado con su icono', () => {
    for (const state of ['free', 'out_of_service'] as const) {
      const table = tableIn(state, 0);
      const view = render(
        <PendingOperationsProvider>
          <TablePanel
            table={table}
            accounts={[]}
            bill={null}
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
          />
        </PendingOperationsProvider>,
      );
      const chip = view.container.querySelector(`.table-panel-state-${state}`);
      expect(chip?.querySelector('svg')?.getAttribute('data-icon')).toBe(tableStateIcons[state]);
      view.unmount();
    }
  });
});

describe('acciones destructivas en rojo (§1.5)', () => {
  it('el botón de confirmar del diálogo de peligro es rojo y el normal azul', () => {
    for (const tone of ['danger', 'default'] as const) {
      const view = render(
        <ConfirmDialog
          title="Descartar cambios"
          message="Se perderán los cambios."
          confirmLabel="Descartar"
          tone={tone}
          onConfirm={() => undefined}
          onCancel={() => undefined}
        />,
      );
      const confirm = screen.getByRole('button', { name: 'Descartar' });
      expect(confirm.className).toContain(
        tone === 'danger' ? 'danger-solid-button' : 'primary-button',
      );
      view.unmount();
    }
  });

  it('una opción destructiva de ChoiceDialog va en rojo', () => {
    render(
      <ChoiceDialog
        title="Otra persona guardó esta plantilla"
        message="Elige qué hacer."
        choices={[
          { label: 'Ver la versión nueva', onSelect: () => undefined },
          { label: 'Descartar mi borrador', onSelect: () => undefined, tone: 'danger' },
        ]}
      />,
    );
    expect(screen.getByRole('button', { name: 'Ver la versión nueva' }).className).toContain(
      'primary-button',
    );
    expect(screen.getByRole('button', { name: 'Descartar mi borrador' }).className).toContain(
      'danger-button',
    );
  });

  it('el formulario que confirma una acción irreversible envía con el botón rojo', async () => {
    let finish: () => void = () => undefined;
    render(
      <Form
        label="Anular cuenta vacía"
        tone="danger"
        onSave={() =>
          new Promise<void>((resolve) => {
            finish = resolve;
          })
        }
      >
        <input aria-label="Motivo" defaultValue="Cliente se retiró" />
      </Form>,
    );
    const submit = screen.getByRole('button', { name: 'Anular cuenta vacía' });
    expect(submit.className).toContain('danger-solid-button');
    const form = submit.closest('form')!;
    expect(form.getAttribute('aria-busy')).toBeNull();
    // Mientras Go responde, el formulario indica la espera real (cursor de progreso).
    fireEvent.click(submit);
    await act(async () => {});
    expect(form.getAttribute('aria-busy')).toBe('true');
    await act(async () => {
      finish();
    });
    expect(form.getAttribute('aria-busy')).toBeNull();
  });

  it('el formulario ordinario conserva el botón principal azul', () => {
    render(
      <Form label="Guardar cambios" onSave={async () => undefined}>
        <input aria-label="Nombre" />
      </Form>,
    );
    expect(screen.getByRole('button', { name: 'Guardar cambios' }).className).toContain(
      'primary-button',
    );
  });

  it('«Anular cuenta vacía» del menú «Más…» va en rojo y las demás opciones no', () => {
    const account = {
      id: '00000000-0000-4000-8000-00000000d9c1',
      state: 'open',
      mode: 'takeaway',
      table_id: null,
      confirmed_quantity: 0,
      draft_quantity: 0,
      releasable: false,
    } as unknown as Account;
    const actions = accountMoreActions(account, () => true, 0);
    expect(actions.find((action) => action.id === 'void')?.tone).toBe('danger');
    render(<MoreActions actions={actions} disabled={false} onAction={() => undefined} />);
    fireEvent.click(screen.getByRole('button', { name: 'Más…' }));
    const menu = screen.getByRole('group', { name: 'Más acciones de la cuenta' });
    expect(within(menu).getByRole('button', { name: 'Anular cuenta vacía' }).className).toContain(
      'danger-button',
    );
    expect(within(menu).getByRole('button', { name: 'Historial de cambios' }).className).toContain(
      'secondary-button',
    );
  });
});

// Recorrido del código: todo botón cuyo texto empieza con un verbo destructivo («Quitar»,
// «Descartar», «Eliminar», «Borrar», «Anular», «Revocar», «Retirar logotipo/de prueba», «Liberar
// mesa», «Cancelar unidades/ajuste/orden/conteo/arqueo») lleva la clase destructiva. Las
// excepciones no destruyen nada: teclas de borrado del teclado, «Retirar efectivo» (operación de
// caja), «Retirar todos los complementos» (elección del cliente en la captura), «Liberar a
// preparación» (envía a cocina) y «Cancelar» a secas o «Cancelar corrección/apertura del cajón»
// (cierran un formulario sin registrar nada).
const destructiveLabel =
  /^(Quitar|Descartar|Eliminar|Borrar|Anular|Revocar|Retirar (logotipo|de prueba)|Liberar mesa|Cancelar (unidades|ajuste|orden|conteo|arqueo|el arqueo))\b/;
const notDestructive = [
  'components/VirtualKeyboard.tsx: Borrar',
  'components/NumericKeypad.tsx: Borrar último carácter',
];

function sources(directory: string): string[] {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) {
      return sources(path);
    }
    return entry.endsWith('.tsx') && !entry.includes('.test.') ? [path] : [];
  });
}

describe('recorrido de acciones destructivas', () => {
  it('todo botón con un verbo destructivo usa el estilo rojo', () => {
    const root = join(__dirname, '..');
    const missing: string[] = [];
    let checked = 0;
    for (const path of sources(root)) {
      const file = relative(root, path).split(sep).join('/');
      const lines = readFileSync(path, 'utf8').split('\n');
      lines.forEach((line, index) => {
        // Texto JSX: la línea completa o el contenido de <IconLabel>…</IconLabel>.
        const text = /^\s*([^<>{}]+?)\s*$/.exec(line)?.[1] ?? />([^<>{}]+)<\//.exec(line)?.[1];
        if (!text || !destructiveLabel.test(text.trim()) || line.includes('//')) {
          return;
        }
        if (notDestructive.includes(`${file}: ${text.trim()}`)) {
          return;
        }
        const context = lines.slice(Math.max(0, index - 16), index + 1).reverse();
        const className = context.find((candidate) => candidate.includes('className='));
        if (className?.includes('section-heading')) {
          return;
        }
        checked += 1;
        if (!className?.includes('danger')) {
          missing.push(`${file}:${index + 1} ${text.trim()}`);
        }
      });
    }
    expect(missing).toEqual([]);
    expect(checked).toBeGreaterThan(25);
  });
});
