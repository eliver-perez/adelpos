// @vitest-environment jsdom

// Etapa 9B, fase C: editor visual de plantillas. Panel de elementos filtrado por tipo; agregar,
// quitar con deshacer, elegir y reordenar (botones, teclado y arrastre); propiedades que cambian el
// borrador; vista previa con antirrebote y una sola petición en vuelo; elegir un bloque desde la
// vista previa; 58/80 mm; guardado, conflicto 409 con el borrador conservado y 422 con el bloque
// señalado; historial y restaurar; muestra impresa; solo lectura sin permiso y predeterminada no
// editable; aviso de cambios de otra persona.

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { templatesChangeEvent } from '../../../services/documents';
import type { TemplateDetail, TemplateLayout } from '../../../services/documents';
import { Designer } from './Designer';
import {
  catalog,
  kitchenKind,
  previewFor,
  printers,
  receiptKind,
  receiptLayout,
  template,
} from './designerTestData';
import { previewDelay } from './usePreview';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

type Response = { status: number; body: unknown };
type Handler = (body: Record<string, unknown>, id?: string) => Response | Promise<Response>;

const manager = [
  'documents.templates.read',
  'documents.templates.manage',
  'documents.samples.print',
  'kitchen.queue.read',
  'kitchen.stations.all',
];
const reader = ['documents.templates.read'];

let handlers: Record<string, Handler> = {};

function ok(body: unknown): Response {
  return { status: 200, body };
}

function fail(status: number, code: string, message: string, details?: unknown): Response {
  return { status, body: { error: { code, message, details } } };
}

function calls(operation: string) {
  return native.invoke.mock.calls
    .filter((call) => (call[1] as { operation?: string })?.operation === operation)
    .map((call) => call[1] as { body: Record<string, unknown>; resourceId: string | null });
}

function defaultHandlers(): Record<string, Handler> {
  return {
    documents_preview: (body) =>
      ok(previewFor(body.layout as TemplateLayout, body.paper_width as 58 | 80)),
    documents_recent: () => ok([]),
    documents_template_versions: () =>
      ok([
        { ...template().current_version },
        {
          id: 'v-1',
          number: 1,
          note: 'Primera',
          created_at: '2026-11-10T15:00:00Z',
          created_by_name: 'Administrador',
          block_count: 4,
        },
      ]),
    documents_version: (_body, id) =>
      ok({
        ...template().current_version,
        id,
        number: id === 'v-1' ? 1 : 2,
        template_id: 't-own',
        template_name: 'Ticket de la casa',
        kind: 'receipt',
        layout:
          id === 'v-1'
            ? { ...receiptLayout(), blocks: receiptLayout().blocks.slice(0, 2) }
            : receiptLayout(),
      }),
  };
}

beforeEach(() => {
  handlers = defaultHandlers();
  native.invoke.mockImplementation(async (command: string, args: Record<string, unknown>) => {
    if (command !== 'api_request') {
      return null;
    }
    const handler = handlers[String(args.operation)];
    if (!handler) {
      return fail(404, 'NOT_FOUND', 'Sin respuesta de prueba.');
    }
    return handler(
      (args.body ?? {}) as Record<string, unknown>,
      (args.resourceId as string) ?? undefined,
    );
  });
  HTMLElement.prototype.setPointerCapture = vi.fn();
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.clearAllMocks();
});

function renderDesigner(overrides: Partial<Parameters<typeof Designer>[0]> = {}) {
  const props = {
    catalog,
    kind: receiptKind,
    template: template(),
    permissions: manager,
    timezone: 'America/Monterrey',
    printers,
    actions: { activate: vi.fn(async () => undefined), duplicate: vi.fn() },
    onClose: vi.fn(),
    onTemplateChanged: vi.fn(),
    ...overrides,
  };
  render(<Designer {...props} />);
  return props;
}

function disabled(element: HTMLElement) {
  return (element as HTMLButtonElement).disabled;
}

function pressed(element: HTMLElement) {
  return element.getAttribute('aria-pressed');
}

function structure() {
  return screen.getByRole('list', { name: 'Estructura de la plantilla' });
}

function order() {
  return Array.from(structure().querySelectorAll('.designer-row-main')).map((row) =>
    (row.getAttribute('aria-label') ?? '').replace(/^\d+\. /, ''),
  );
}

function row(label: string) {
  return within(structure()).getByRole('button', { name: new RegExp(`^\\d+\\. ${label}`) });
}

async function previewReady() {
  await waitFor(() => expect(calls('documents_preview').length).toBeGreaterThan(0));
  await screen.findByRole('figure');
}

describe('panel de elementos', () => {
  it('ofrece solo los bloques compatibles con el tipo, con los atajos de datos', () => {
    renderDesigner();
    const palette = screen.getByRole('region', { name: 'Elementos disponibles' });
    expect(
      disabled(within(palette).getByRole('button', { name: 'Agregar Lista de productos' })),
    ).toBe(true);
    expect(within(palette).queryByRole('button', { name: 'Agregar Lista de cocina' })).toBeNull();
    expect(disabled(within(palette).getByRole('button', { name: 'Agregar Fecha' }))).toBe(false);
    expect(disabled(within(palette).getByRole('button', { name: 'Agregar Cajero' }))).toBe(false);
    // El aviso fiscal es obligatorio y ya está en la plantilla.
    expect(disabled(within(palette).getByRole('button', { name: 'Agregar Aviso fiscal' }))).toBe(
      true,
    );
    expect(within(palette).getByText('Encabezado')).toBeTruthy();
    expect(within(palette).getByText('Texto y formato')).toBeTruthy();
    cleanup();

    renderDesigner({
      kind: kitchenKind,
      template: template({
        kind: 'kitchen_order',
        layout: { options: { text_size: 'inherit' }, blocks: [] },
      }),
    });
    const kitchen = screen.getByRole('region', { name: 'Elementos disponibles' });
    expect(disabled(within(kitchen).getByRole('button', { name: 'Agregar Lista de cocina' }))).toBe(
      false,
    );
    expect(
      within(kitchen).queryByRole('button', { name: 'Agregar Lista de productos' }),
    ).toBeNull();
    expect(within(kitchen).queryByRole('button', { name: 'Agregar Aviso fiscal' })).toBeNull();
    expect(within(kitchen).getByRole('button', { name: 'Agregar Mesa' })).toBeTruthy();
  });

  it('agrega después del bloque elegido y lo deja elegido', () => {
    renderDesigner();
    fireEvent.click(row('Texto fijo'));
    fireEvent.click(screen.getByRole('button', { name: 'Agregar Separador' }));
    expect(order()).toEqual([
      'Nombre del establecimiento',
      'Texto fijo · «Gracias»',
      'Separador · Con guiones',
      'Lista de productos',
      'Total',
      'Aviso fiscal',
    ]);
    expect(pressed(row('Separador'))).toBe('true');
    expect(screen.getByRole('heading', { name: 'Separador · Con guiones' })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Agregar Cajero' }));
    expect(order()[3]).toBe('Campo de datos · Cajero');
    expect(screen.getByText('Cambios sin guardar')).toBeTruthy();
  });

  it('agrega arrastrando desde el panel hasta la estructura', () => {
    renderDesigner();
    const rows = Array.from(structure().querySelectorAll<HTMLElement>('[data-structure-row]'));
    rows.forEach((item, index) => {
      item.getBoundingClientRect = () =>
        ({
          top: 100 + index * 60,
          height: 60,
          bottom: 160 + index * 60,
          left: 600,
          right: 900,
        }) as DOMRect;
    });
    structure().getBoundingClientRect = () =>
      ({ top: 100, bottom: 400, left: 600, right: 900, height: 300, width: 300 }) as DOMRect;
    const item = screen.getByRole('button', { name: 'Agregar Espacio' });
    fireEvent.pointerDown(item, { clientX: 100, clientY: 100, buttons: 1 });
    fireEvent.pointerMove(item, { clientX: 140, clientY: 110, buttons: 1 });
    act(() => {
      window.dispatchEvent(new MouseEvent('pointermove', { clientX: 700, clientY: 110 }));
    });
    act(() => {
      window.dispatchEvent(new MouseEvent('pointerup', { clientX: 700, clientY: 110 }));
    });
    expect(order()[0]).toBe('Espacio · 1 línea en blanco');
  });
});

describe('estructura', () => {
  it('quita con deshacer y no permite quitar el aviso fiscal', () => {
    renderDesigner();
    fireEvent.click(row('Texto fijo'));
    fireEvent.click(screen.getByRole('button', { name: 'Quitar Texto fijo · «Gracias»' }));
    expect(order()).not.toContain('Texto fijo · «Gracias»');
    expect(screen.getByText('Se quitó «Texto fijo · «Gracias»».')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Deshacer' }));
    expect(order()[1]).toBe('Texto fijo · «Gracias»');
    fireEvent.click(row('Aviso fiscal'));
    expect(
      disabled(
        screen.getByRole('button', { name: 'Aviso fiscal: obligatorio, no se puede quitar' }),
      ),
    ).toBe(true);
    expect(within(structure()).getByText('Obligatorio')).toBeTruthy();
    // Tampoco se puede ocultar.
    expect(disabled(screen.getByRole('button', { name: 'Visible' }))).toBe(true);
  });

  it('reordena con los botones, con el teclado y arrastrando el asa', () => {
    renderDesigner();
    fireEvent.click(screen.getByRole('button', { name: 'Bajar Nombre del establecimiento' }));
    expect(order().slice(0, 2)).toEqual(['Texto fijo · «Gracias»', 'Nombre del establecimiento']);
    fireEvent.keyDown(row('Nombre del establecimiento'), { key: 'ArrowUp', altKey: true });
    expect(order()[0]).toBe('Nombre del establecimiento');
    // Flecha sola: solo mueve el foco.
    fireEvent.keyDown(row('Nombre del establecimiento'), { key: 'ArrowDown' });
    expect(document.activeElement).toBe(row('Texto fijo'));
    expect(order()[0]).toBe('Nombre del establecimiento');

    const rows = Array.from(structure().querySelectorAll<HTMLElement>('[data-structure-row]'));
    rows.forEach((item, index) => {
      item.getBoundingClientRect = () =>
        ({ top: index * 60, height: 60, bottom: index * 60 + 60 }) as DOMRect;
    });
    const handle = rows[0]?.querySelector('.designer-handle');
    expect(handle).toBeTruthy();
    fireEvent.pointerDown(handle!, { pointerId: 1, clientY: 10 });
    fireEvent.pointerMove(handle!, { pointerId: 1, clientY: 250 });
    fireEvent.pointerUp(handle!, { pointerId: 1, clientY: 250 });
    expect(order()).toEqual([
      'Texto fijo · «Gracias»',
      'Lista de productos',
      'Total',
      'Nombre del establecimiento',
      'Aviso fiscal',
    ]);
  });
});

describe('vista previa', () => {
  it('pide la vista previa con antirrebote y una sola petición en vuelo', async () => {
    vi.useFakeTimers();
    const pending: ((response: Response) => void)[] = [];
    handlers.documents_preview = () =>
      new Promise<Response>((resolve) => {
        pending.push(resolve);
      });
    renderDesigner();
    await act(async () => {
      vi.advanceTimersByTime(previewDelay + 10);
    });
    expect(calls('documents_preview')).toHaveLength(1);
    // Se escribe varias veces en el texto fijo mientras la primera petición sigue en vuelo.
    fireEvent.click(row('Texto fijo'));
    const text = screen.getByLabelText('Texto');
    fireEvent.change(text, { target: { value: 'G' } });
    fireEvent.change(text, { target: { value: 'Gr' } });
    await act(async () => {
      vi.advanceTimersByTime(previewDelay / 2);
    });
    fireEvent.change(text, { target: { value: 'Gracias por venir' } });
    await act(async () => {
      vi.advanceTimersByTime(previewDelay + 10);
    });
    expect(calls('documents_preview')).toHaveLength(1);
    // Al responder la primera, se envía solo la más reciente.
    await act(async () => {
      pending[0]?.(ok(previewFor(receiptLayout())));
    });
    expect(calls('documents_preview')).toHaveLength(2);
    const last = calls('documents_preview')[1]?.body.layout as TemplateLayout;
    expect(last.blocks[1]?.options.text).toBe('Gracias por venir');
    await act(async () => {
      pending[1]?.(ok(previewFor(last)));
    });
    expect(calls('documents_preview')).toHaveLength(2);
  });

  it('elige el bloque al tocar sus líneas y las resalta', async () => {
    renderDesigner();
    await previewReady();
    fireEvent.click(screen.getByRole('button', { name: 'Elegir Total' }));
    expect(pressed(row('Total'))).toBe('true');
    expect(pressed(screen.getByRole('button', { name: 'Elegir Total' }))).toBe('true');
    expect(screen.getByRole('heading', { name: 'Total' })).toBeTruthy();
    expect(screen.getByText('Vista previa con datos de muestra')).toBeTruthy();
  });

  it('cambia entre 58 y 80 mm con el ancho real de Go', async () => {
    renderDesigner();
    await previewReady();
    expect(calls('documents_preview')[0]?.body.paper_width).toBe(80);
    fireEvent.click(screen.getByRole('button', { name: '58 mm' }));
    await waitFor(() => expect(calls('documents_preview').at(-1)?.body.paper_width).toBe(58));
    await screen.findByText('TOTAL (58)');
    expect(screen.getByRole('figure').getAttribute('data-paper')).toBe('58');
    expect(screen.getByText('30 columnas')).toBeTruthy();
  });

  it('las propiedades cambian el borrador que se previsualiza', async () => {
    renderDesigner();
    await previewReady();
    fireEvent.click(row('Total'));
    fireEvent.click(
      within(screen.getByRole('group', { name: 'Alineación' })).getByRole('button', {
        name: 'Centro',
      }),
    );
    fireEvent.click(
      within(screen.getByRole('group', { name: 'Tamaño' })).getByRole('button', {
        name: 'Doble alto',
      }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Negrita' }));
    fireEvent.click(screen.getByRole('button', { name: 'Espacio antes: más' }));
    await waitFor(() => {
      const layout = calls('documents_preview').at(-1)?.body.layout as TemplateLayout;
      expect(layout.blocks[3]).toMatchObject({
        align: 'center',
        size: 'double_height',
        bold: true,
        spacing_before: 1,
      });
    });
    expect(screen.getByText(/requieren una impresora ESC\/POS por red/)).toBeTruthy();
    // La impresora controlada no reproduce el formato: se avisa.
    await screen.findByText('Impresoras que no reproducen todo el formato');
  });

  it('muestra el error de Go en la vista previa y señala el bloque', async () => {
    handlers.documents_preview = () =>
      fail(422, 'TEMPLATE_INVALID', 'El texto fijo no puede quedar vacío.', {
        block_id: 'greeting',
      });
    renderDesigner();
    await screen.findByText('El texto fijo no puede quedar vacío.');
    fireEvent.click(screen.getByRole('button', { name: 'Ir a Texto fijo · «Gracias»' }));
    expect(pressed(row('Texto fijo'))).toBe('true');
    expect(within(structure()).getByText('Revisa este bloque')).toBeTruthy();
  });
});

describe('guardar y versiones', () => {
  function savedTemplate(layout: TemplateLayout): TemplateDetail {
    return template({
      revision: 4,
      layout,
      current_version: { ...template().current_version, id: 'v-3', number: 3 },
    });
  }

  async function makeChangeAndOpenSave() {
    fireEvent.click(row('Total'));
    fireEvent.click(screen.getByRole('button', { name: 'Negrita' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar versión' }));
    return screen.findByRole('dialog', { name: 'Guardar la versión 3' });
  }

  it('guarda una versión nueva con nota y revisión', async () => {
    handlers.documents_save_template = (body) => ok(savedTemplate(body.layout as TemplateLayout));
    const props = renderDesigner();
    expect(disabled(screen.getByRole('button', { name: 'Guardar versión' }))).toBe(true);
    const dialog = await makeChangeAndOpenSave();
    fireEvent.change(within(dialog).getByLabelText('Nota de la versión (opcional)'), {
      target: { value: 'Total en negrita' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Guardar versión' }));
    await screen.findByText('Sin cambios pendientes');
    const save = calls('documents_save_template')[0];
    expect(save?.resourceId).toBe('t-own');
    expect(save?.body).toMatchObject({ revision: 3, note: 'Total en negrita' });
    expect(typeof save?.body.operation_id).toBe('string');
    expect((save?.body.layout as TemplateLayout).blocks[3]?.bold).toBe(true);
    expect(props.onTemplateChanged).toHaveBeenCalled();
    expect(screen.getByText(/versión 3 guardada/)).toBeTruthy();
  });

  it('conflicto 409: conserva el borrador y guarda sobre la versión más reciente', async () => {
    let attempts = 0;
    handlers.documents_save_template = (body) => {
      attempts += 1;
      if (attempts === 1) {
        return fail(409, 'REVISION_CONFLICT', 'Otra persona guardó la plantilla.', { revision: 5 });
      }
      return ok(savedTemplate(body.layout as TemplateLayout));
    };
    const latestLayout = { ...receiptLayout(), blocks: receiptLayout().blocks.slice(1) };
    handlers.documents_template = () =>
      ok(
        template({
          revision: 5,
          layout: latestLayout,
          current_version: {
            ...template().current_version,
            id: 'v-other',
            number: 3,
            created_by_name: 'Cajera Ana',
          },
        }),
      );
    renderDesigner();
    const dialog = await makeChangeAndOpenSave();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Guardar versión' }));
    const conflict = await screen.findByRole('alertdialog', {
      name: 'Otra persona guardó esta plantilla',
    });
    expect(within(conflict).getByText(/Cajera Ana guardó la versión 3/)).toBeTruthy();
    expect(within(conflict).getByText(/Tu borrador no se perdió/)).toBeTruthy();
    // El borrador sigue intacto.
    expect(screen.getByText('Cambios sin guardar')).toBeTruthy();
    // Ver la versión nueva y volver.
    fireEvent.click(within(conflict).getByRole('button', { name: 'Ver la versión nueva' }));
    await screen.findByRole('dialog', { name: 'Versión 3 guardada por Cajera Ana' });
    fireEvent.click(screen.getAllByRole('button', { name: 'Volver a las opciones' })[1]!);
    fireEvent.click(
      await screen.findByRole('button', { name: 'Guardar mi borrador como versión más reciente' }),
    );
    await screen.findByText('Sin cambios pendientes');
    const second = calls('documents_save_template')[1];
    expect(second?.body.revision).toBe(5);
    const layout = second?.body.layout as TemplateLayout;
    expect(layout.blocks).toHaveLength(5);
    expect(layout.blocks[3]?.bold).toBe(true);
  });

  it('conflicto 409: descartar el borrador muestra la versión de la otra persona', async () => {
    handlers.documents_save_template = () =>
      fail(409, 'REVISION_CONFLICT', 'Otra persona guardó la plantilla.', { revision: 5 });
    handlers.documents_template = () =>
      ok(
        template({
          revision: 5,
          layout: { ...receiptLayout(), blocks: receiptLayout().blocks.slice(1) },
          current_version: { ...template().current_version, id: 'v-other', number: 3 },
        }),
      );
    renderDesigner();
    const dialog = await makeChangeAndOpenSave();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Guardar versión' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Descartar mi borrador' }));
    await waitFor(() => expect(order()).toHaveLength(4));
    expect(order()[0]).toBe('Texto fijo · «Gracias»');
    expect(screen.getByText('Sin cambios pendientes')).toBeTruthy();
    expect(calls('documents_save_template')).toHaveLength(1);
  });

  it('422: el bloque señalado por Go queda marcado y elegido', async () => {
    handlers.documents_save_template = () =>
      fail(422, 'TEMPLATE_INVALID', 'El texto fijo tiene más de 8 líneas.', {
        block_id: 'greeting',
      });
    renderDesigner();
    const dialog = await makeChangeAndOpenSave();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Guardar versión' }));
    await waitFor(() => expect(pressed(row('Texto fijo'))).toBe('true'));
    expect(within(structure()).getByText('Revisa este bloque')).toBeTruthy();
    expect(screen.getAllByText('El texto fijo tiene más de 8 líneas.').length).toBeGreaterThan(0);
    expect(screen.getByText('Cambios sin guardar')).toBeTruthy();
  });

  it('historial: vista previa de una versión y restaurarla como versión nueva', async () => {
    handlers.documents_save_template = (body) => ok(savedTemplate(body.layout as TemplateLayout));
    renderDesigner();
    fireEvent.click(screen.getByRole('button', { name: 'Historial' }));
    const history = await screen.findByRole('dialog', { name: 'Historial de «Ticket de la casa»' });
    fireEvent.click(await within(history).findByRole('button', { name: /Versión 1/ }));
    await waitFor(() => expect(calls('documents_version').at(-1)?.resourceId).toBe('v-1'));
    fireEvent.click(await within(history).findByRole('button', { name: 'Restaurar esta versión' }));
    const confirm = await screen.findByRole('alertdialog', { name: 'Restaurar la versión 1' });
    fireEvent.click(within(confirm).getByRole('button', { name: 'Restaurar esta versión' }));
    await waitFor(() => expect(calls('documents_save_template')).toHaveLength(1));
    const save = calls('documents_save_template')[0];
    expect(save?.body).toMatchObject({ revision: 3, note: 'Restaurada desde la versión 1' });
    expect((save?.body.layout as TemplateLayout).blocks).toHaveLength(2);
  });

  it('avisa si otra persona guardó mientras se edita, sin tocar el borrador', async () => {
    handlers.documents_template = () =>
      ok(
        template({
          revision: 5,
          current_version: { ...template().current_version, id: 'v-other', number: 3 },
        }),
      );
    renderDesigner();
    fireEvent.click(row('Total'));
    fireEvent.click(screen.getByRole('button', { name: 'Negrita' }));
    act(() => {
      window.dispatchEvent(
        new CustomEvent(templatesChangeEvent, { detail: { templates: ['t-own'] } }),
      );
    });
    await screen.findByText(/Otra persona guardó la versión 3 mientras editabas/);
    expect(screen.getByText('Cambios sin guardar')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Ver opciones' }));
    expect(
      await screen.findByRole('alertdialog', { name: 'Otra persona guardó esta plantilla' }),
    ).toBeTruthy();
  });
});

describe('permisos y plantillas del sistema', () => {
  it('sin documents.templates.manage todo es de solo lectura, con vista previa', async () => {
    renderDesigner({ permissions: reader });
    await previewReady();
    expect(screen.queryByRole('button', { name: 'Guardar versión' })).toBeNull();
    expect(screen.queryByRole('region', { name: 'Elementos disponibles' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Duplicar' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Imprimir muestra' })).toBeNull();
    expect(screen.getByText('Solo lectura')).toBeTruthy();
    fireEvent.click(row('Total'));
    expect(disabled(screen.getByRole('button', { name: 'Negrita' }))).toBe(true);
    expect(screen.queryByRole('button', { name: /^Quitar/ })).toBeNull();
  });

  it('la predeterminada del sistema no se edita: se duplica', async () => {
    const props = renderDesigner({
      template: template({ id: 'system', name: 'Predeterminada', system: true, active: true }),
    });
    await previewReady();
    expect(
      screen.getByText(/La plantilla predeterminada del sistema reproduce el formato/),
    ).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Guardar versión' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Activar' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Duplicar' }));
    await waitFor(() => expect(props.actions.duplicate).toHaveBeenCalled());
  });
});

describe('muestra impresa', () => {
  it('elige impresora, confirma y muestra el estado de la cola sin decir «impreso»', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    handlers.documents_print_sample = (body) =>
      ok({
        status: 'pending',
        job_id: 'job-1',
        ticket_id: 'tk-1',
        printer_id: body.printer_id,
        message: '',
      });
    handlers.kitchen_job = () => ok({ job: { id: 'job-1', state: 'intervention' } });
    renderDesigner();
    fireEvent.click(screen.getByRole('button', { name: 'Imprimir muestra' }));
    const dialog = await screen.findByRole('dialog', { name: 'Imprimir una muestra' });
    expect(
      within(dialog).getByText(/No crea ventas, pagos, movimientos de caja ni documentos/),
    ).toBeTruthy();
    expect(disabled(within(dialog).getByRole('button', { name: 'Continuar' }))).toBe(true);
    fireEvent.click(within(dialog).getByRole('button', { name: /Destino controlado/ }));
    expect(within(dialog).getByText(/Negrita y tamaños: No admitido/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Continuar' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Imprimir muestra' }));
    await within(dialog).findByText('Muestra enviada a la cola de impresión.');
    const sent = calls('documents_print_sample')[0];
    expect(sent?.body).toMatchObject({ kind: 'receipt', printer_id: 'p-agent' });
    await act(async () => {
      vi.advanceTimersByTime(1600);
    });
    await within(dialog).findByText(
      /Requiere intervención: revisa el trabajo en Cocina → Cola de impresión/,
    );
    expect(document.body.textContent ?? '').not.toMatch(/\bimpres[oa]\b/i);
  });
});
