// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  activateDocumentTemplate,
  createDocumentTemplate,
  documentTemplates,
  previewDocument,
  printDocumentSample,
  recentDocuments,
  rerenderAccountDocument,
  rerenderCashDocument,
  rerenderKitchenJob,
  saveDocumentTemplate,
  type TemplateLayout,
} from './documents';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  vi.clearAllMocks();
});

const layout: TemplateLayout = {
  options: { text_size: 'normal' },
  blocks: [
    {
      id: 'fiscal',
      type: 'fiscal_notice',
      visible: true,
      align: 'center',
      size: 'normal',
      bold: false,
      uppercase: false,
      spacing_before: 0,
      spacing_after: 0,
      options: {},
    },
  ],
};

function lastCall() {
  return native.invoke.mock.calls[native.invoke.mock.calls.length - 1];
}

describe('contratos del diseñador de documentos', () => {
  it('usa las operaciones cerradas del puente con el tipo como único filtro', async () => {
    native.invoke.mockResolvedValue({ status: 200, body: [] });
    await documentTemplates('receipt');
    expect(lastCall()).toEqual([
      'api_request',
      {
        operation: 'documents_templates',
        resourceId: null,
        body: null,
        query: { kind: 'receipt' },
      },
    ]);
    await recentDocuments('cash_final_cut');
    expect(lastCall()?.[1]).toMatchObject({
      operation: 'documents_recent',
      query: { kind: 'cash_final_cut' },
    });
  });

  it('las escrituras conservan el operation_id y la revisión', async () => {
    native.invoke.mockResolvedValue({ status: 200, body: {} });
    await createDocumentTemplate('op-1', 'precheck', 'Precuenta', 'version-1');
    expect(lastCall()?.[1]).toMatchObject({
      operation: 'documents_create_template',
      body: {
        operation_id: 'op-1',
        kind: 'precheck',
        name: 'Precuenta',
        source_version_id: 'version-1',
      },
    });
    await createDocumentTemplate('op-2', 'precheck', 'Sin origen');
    expect(lastCall()?.[1].body).toEqual({
      operation_id: 'op-2',
      kind: 'precheck',
      name: 'Sin origen',
    });
    await saveDocumentTemplate('op-3', 'plantilla-1', 4, layout, 'Nota');
    expect(lastCall()?.[1]).toMatchObject({
      operation: 'documents_save_template',
      resourceId: 'plantilla-1',
      body: { operation_id: 'op-3', revision: 4, layout, note: 'Nota' },
    });
    await activateDocumentTemplate('op-4', 'plantilla-1', 5);
    expect(lastCall()?.[1]).toMatchObject({
      operation: 'documents_activate_template',
      body: { operation_id: 'op-4', revision: 5 },
    });
  });

  it('la vista previa envía el layout, el papel y la fuente', async () => {
    native.invoke.mockResolvedValue({ status: 200, body: { lines: [] } });
    await previewDocument('receipt', layout, 58, { sample: true });
    expect(lastCall()?.[1]).toMatchObject({
      operation: 'documents_preview',
      body: { kind: 'receipt', layout, paper_width: 58, source: { sample: true } },
    });
    await previewDocument('receipt', layout, 80, { ticket_id: 'ticket-1' });
    expect(lastCall()?.[1].body.source).toEqual({ ticket_id: 'ticket-1' });
  });

  it('muestras e impresión con la plantilla vigente usan sus propias operaciones', async () => {
    native.invoke.mockResolvedValue({ status: 200, body: {} });
    await printDocumentSample('op-5', 'kitchen_order', layout, 'impresora-1');
    expect(lastCall()?.[1]).toMatchObject({
      operation: 'documents_print_sample',
      body: { operation_id: 'op-5', kind: 'kitchen_order', printer_id: 'impresora-1' },
    });
    await rerenderKitchenJob('op-6', 'trabajo-1', 'Motivo de la copia');
    expect(lastCall()?.[1]).toMatchObject({
      operation: 'kitchen_rerender',
      body: { operation_id: 'op-6', job_id: 'trabajo-1', reason: 'Motivo de la copia' },
    });
    expect(lastCall()?.[1].body.printer_id).toBeUndefined();
    await rerenderAccountDocument('op-7', 'cuenta-1', 'ticket-1');
    expect(lastCall()?.[1]).toMatchObject({
      operation: 'payments_rerender',
      resourceId: 'cuenta-1',
      body: { operation_id: 'op-7', ticket_id: 'ticket-1' },
    });
    await rerenderCashDocument('op-8', 'documento-1');
    expect(lastCall()?.[1]).toMatchObject({
      operation: 'payments_rerender_cash_document',
      resourceId: 'documento-1',
      body: { operation_id: 'op-8' },
    });
  });

  it('un conflicto de revisión conserva su código y sus detalles', async () => {
    native.invoke.mockResolvedValue({
      status: 409,
      body: {
        error: {
          code: 'REVISION_CONFLICT',
          message: 'Otro administrador cambió esta plantilla.',
          details: { revision: 7 },
        },
      },
    });
    await expect(saveDocumentTemplate('op-9', 'plantilla-1', 6, layout)).rejects.toMatchObject({
      code: 'REVISION_CONFLICT',
      status: 409,
      details: { revision: 7 },
    });
  });
});
