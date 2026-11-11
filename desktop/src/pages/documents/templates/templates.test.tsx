// @vitest-environment jsdom

// Etapa 9B, fase C: pantalla Documentos y plantillas por tipo. Tipos agrupados, predeterminada de
// solo lectura, activar, duplicar, renombrar y restaurar la predeterminada con confirmación, la
// pestaña de identidad y la lectura sin permiso de administrar. También comprueba que el editor no
// usa campos type="number" ni window.confirm.

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { TemplateSummary } from '../../../services/documents';
import { catalog, template } from '../designer/designerTestData';
import { DocumentsPage } from '../DocumentsPage';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));
vi.mock('../IdentityPanel', () => ({
  IdentityPanel: (props: { onOpenSettings: () => void }) => (
    <button type="button" onClick={props.onOpenSettings}>
      Identidad simulada
    </button>
  ),
}));

type Response = { status: number; body: unknown };
type Handler = (body: Record<string, unknown>, id?: string) => Response;

const manager = ['documents.templates.read', 'documents.templates.manage'];
const reader = ['documents.templates.read'];

function summary(overrides: Partial<TemplateSummary>): TemplateSummary {
  const { layout: _layout, ...rest } = template(overrides);
  return rest;
}

const systemReceipt = summary({
  id: 'system-receipt',
  name: 'Predeterminada',
  system: true,
  active: false,
  revision: 1,
  current_version: { ...template().current_version, id: 'sv-1', number: 1, created_by_name: null },
});
const ownReceipt = summary({ active: true });

let handlers: Record<string, Handler> = {};

function ok(body: unknown): Response {
  return { status: 200, body };
}

function calls(operation: string) {
  return native.invoke.mock.calls
    .filter((call) => (call[1] as { operation?: string })?.operation === operation)
    .map(
      (call) =>
        call[1] as {
          body: Record<string, unknown>;
          resourceId: string | null;
          query: Record<string, string> | null;
        },
    );
}

beforeEach(() => {
  handlers = {
    documents_catalog: () => ok(catalog),
    documents_printers: () => ok([]),
    documents_templates: () => ok([ownReceipt, systemReceipt]),
    documents_template: (_body, id) =>
      ok(template(id === 'system-receipt' ? { ...systemReceipt } : { ...ownReceipt })),
    documents_preview: () =>
      ok({
        kind: 'receipt',
        paper_width: 80,
        columns: 42,
        text_size: 'normal',
        sample: true,
        includes_expected: false,
        lines: [],
        text: '',
        warnings: [],
        empty_blocks: [],
      }),
  };
  native.invoke.mockImplementation(async (command: string, args: Record<string, unknown>) => {
    if (command !== 'api_request') {
      return null;
    }
    const handler = handlers[String(args.operation)];
    if (!handler) {
      return { status: 404, body: { error: { code: 'NOT_FOUND', message: 'Sin respuesta.' } } };
    }
    return handler(
      (args.body ?? {}) as Record<string, unknown>,
      (args.resourceId as string) ?? undefined,
    );
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderPage(permissions = manager) {
  const onOpenSettings = vi.fn();
  render(
    <DocumentsPage
      permissions={permissions}
      timezone="America/Monterrey"
      onOpenSettings={onOpenSettings}
    />,
  );
  return { onOpenSettings };
}

async function receiptPanel() {
  const panel = await screen.findByRole('region', { name: 'Plantillas de Ticket de venta' });
  await within(panel).findByText('Ticket de la casa');
  return panel;
}

describe('pantalla Documentos', () => {
  it('agrupa los tipos y marca la predeterminada como solo lectura', async () => {
    renderPage();
    const kinds = await screen.findByRole('navigation', { name: 'Tipos de documento' });
    for (const group of ['Venta', 'Cocina', 'Caja']) {
      expect(within(kinds).getByRole('heading', { name: group })).toBeTruthy();
    }
    expect(within(kinds).getByRole('button', { name: 'Comanda' })).toBeTruthy();
    const panel = await receiptPanel();
    expect(within(panel).getByText('Predeterminada · solo lectura')).toBeTruthy();
    expect(within(panel).getByText('Activa')).toBeTruthy();
    expect(within(panel).getByRole('button', { name: 'Ver Predeterminada' })).toBeTruthy();
    expect(within(panel).getByRole('button', { name: 'Editar Ticket de la casa' })).toBeTruthy();
    // La predeterminada no se renombra.
    expect(within(panel).queryByRole('button', { name: 'Renombrar Predeterminada' })).toBeNull();
    fireEvent.click(within(kinds).getByRole('button', { name: 'Comanda' }));
    await waitFor(() =>
      expect(calls('documents_templates').at(-1)?.query).toEqual({ kind: 'kitchen_order' }),
    );
  });

  it('activar se confirma y envía la revisión', async () => {
    handlers.documents_activate_template = (_body, id) =>
      ok(template({ ...systemReceipt, id: id ?? '', active: true }));
    renderPage();
    const panel = await receiptPanel();
    fireEvent.click(within(panel).getByRole('button', { name: 'Activar Predeterminada' }));
    let dialog = await screen.findByRole('alertdialog', { name: 'Activar plantilla' });
    expect(within(dialog).getByText(/conservan su formato al reimprimirse/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancelar' }));
    expect(calls('documents_activate_template')).toHaveLength(0);
    fireEvent.click(within(panel).getByRole('button', { name: 'Activar Predeterminada' }));
    dialog = await screen.findByRole('alertdialog', { name: 'Activar plantilla' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Activar' }));
    await waitFor(() => expect(calls('documents_activate_template')).toHaveLength(1));
    expect(calls('documents_activate_template')[0]).toMatchObject({
      resourceId: 'system-receipt',
      body: { revision: 1 },
    });
  });

  it('duplicar pide el nombre, copia la versión vigente y abre el editor', async () => {
    handlers.documents_create_template = (body) =>
      ok(template({ id: 't-copy', name: String(body.name), revision: 1 }));
    renderPage();
    const panel = await receiptPanel();
    fireEvent.click(within(panel).getByRole('button', { name: 'Duplicar Predeterminada' }));
    const dialog = await screen.findByRole('dialog', { name: 'Duplicar plantilla' });
    const name = within(dialog).getByRole('textbox') as HTMLInputElement;
    expect(name.value).toBe('Copia de Predeterminada');
    fireEvent.change(name, { target: { value: 'Ticket con logotipo' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Duplicar' }));
    await screen.findByRole('region', { name: 'Editor de Ticket con logotipo' });
    expect(calls('documents_create_template')[0]?.body).toMatchObject({
      kind: 'receipt',
      name: 'Ticket con logotipo',
      source_version_id: 'sv-1',
    });
    // Mientras se edita, el título de la página y las pestañas se ocultan.
    expect(screen.queryByRole('tablist', { name: 'Secciones de documentos' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Volver a plantillas' }));
    await screen.findByRole('tablist', { name: 'Secciones de documentos' });
  });

  it('renombrar muestra el rechazo de Go sin cerrar el diálogo', async () => {
    handlers.documents_rename_template = () => ({
      status: 409,
      body: {
        error: { code: 'TEMPLATE_NAME_TAKEN', message: 'Ya existe una plantilla con ese nombre.' },
      },
    });
    renderPage();
    const panel = await receiptPanel();
    fireEvent.click(within(panel).getByRole('button', { name: 'Renombrar Ticket de la casa' }));
    const dialog = await screen.findByRole('dialog', { name: 'Renombrar plantilla' });
    fireEvent.change(within(dialog).getByRole('textbox'), { target: { value: 'Predeterminada' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Renombrar' }));
    await within(dialog).findByText('Ya existe una plantilla con ese nombre.');
    expect(calls('documents_rename_template')[0]).toMatchObject({
      resourceId: 't-own',
      body: { name: 'Predeterminada', revision: 3 },
    });
  });

  it('restaurar la predeterminada explica que conserva las demás y se confirma', async () => {
    handlers.documents_restore_default = () => ok(template({ ...systemReceipt, active: true }));
    renderPage();
    const panel = await receiptPanel();
    fireEvent.click(within(panel).getByRole('button', { name: 'Restaurar predeterminada' }));
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Restaurar la plantilla predeterminada',
    });
    expect(
      within(dialog).getByText(/Las demás plantillas y sus versiones se conservan/),
    ).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restaurar predeterminada' }));
    await waitFor(() => expect(calls('documents_restore_default')).toHaveLength(1));
    expect(calls('documents_restore_default')[0]?.body).toMatchObject({ kind: 'receipt' });
  });

  it('sin permiso de administrar solo se consulta', async () => {
    renderPage(reader);
    const panel = await receiptPanel();
    expect(within(panel).getByRole('button', { name: 'Ver Ticket de la casa' })).toBeTruthy();
    expect(within(panel).queryByRole('button', { name: /^Duplicar/ })).toBeNull();
    expect(within(panel).queryByRole('button', { name: /^Activar/ })).toBeNull();
    expect(within(panel).queryByRole('button', { name: 'Nueva plantilla' })).toBeNull();
    expect(within(panel).queryByRole('button', { name: 'Restaurar predeterminada' })).toBeNull();
    expect(
      within(panel).getByRole('button', { name: 'Historial de Ticket de la casa' }),
    ).toBeTruthy();
  });

  it('la pestaña de identidad monta el panel de la fase B2', async () => {
    const { onOpenSettings } = renderPage();
    fireEvent.click(await screen.findByRole('tab', { name: 'Identidad del establecimiento' }));
    fireEvent.click(screen.getByRole('button', { name: 'Identidad simulada' }));
    expect(onOpenSettings).toHaveBeenCalled();
  });
});

// Recorre los archivos de la pantalla (sin las pruebas) buscando campos numéricos nativos o
// diálogos del navegador, que el WebView de macOS no muestra.
function sources(directory: string): string[] {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) {
      return entry === 'identity' ? [] : sources(path);
    }
    return /\.tsx?$/.test(entry) && !entry.includes('.test.') ? [path] : [];
  });
}

describe('controles del editor', () => {
  it('no usa type="number" ni window.confirm', () => {
    const files = sources(join(__dirname, '..'));
    expect(files.length).toBeGreaterThan(10);
    for (const file of files) {
      const source = readFileSync(file, 'utf8');
      expect(source, file).not.toMatch(/type=["']number["']/);
      expect(source, file).not.toMatch(/window\.(confirm|alert|prompt)\(/);
    }
  });
});
