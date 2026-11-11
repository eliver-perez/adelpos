// @vitest-environment jsdom

// Etapa 9B, fase B2: pestaña «Identidad del establecimiento». Carga con los datos de Configuración
// general, vista previa del original y del papel de 58 y 80 mm, vista previa de otra presentación
// con antirrebote y una sola petición, carga del logotipo en base64, retiro con confirmación, solo
// lectura sin documents.identity.manage, conflicto de revisión sin perder lo escrito, reintento con
// el mismo operation_id y ningún campo type="number".

import { readFileSync } from 'node:fs';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DocumentIdentity, IdentityPresentation } from '../../../services/documentIdentity';
import { IdentityPanel } from '../IdentityPanel';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const manager = ['documents.identity.manage', 'documents.templates.read', 'settings.read'];
const reader = ['documents.templates.read'];
const pixel = 'data:image/png;base64,iVBORw0KGgo=';

const limits = {
  text_max_characters: 300,
  text_max_lines: 8,
  logo_max_bytes: 1048576,
  logo_max_dimension: 2048,
  width_percent_min: 25,
  width_percent_max: 100,
  threshold_min: 1,
  threshold_max: 255,
  max_height_dots: 320,
  paper_widths: [
    { paper_width: 58, width_dots: 384 },
    { paper_width: 80, width_dots: 576 },
  ],
};

const defaultPresentation: IdentityPresentation = {
  width_percent: 60,
  align: 'center',
  method: 'threshold',
  threshold: 128,
};

function identity(overrides: Partial<DocumentIdentity> = {}): DocumentIdentity {
  return {
    revision: 3,
    establishment: {
      name: 'Fonda Doña Rosa',
      address: 'Calle 1, Centro',
      phone: '81 1234 5678',
      email: '',
    },
    additional_info: 'RFC: XAXX010101000',
    header_text: '',
    footer_text: 'Gracias por su visita',
    presentation: { ...defaultPresentation },
    logo: null,
    updated_at: '2026-11-11T15:00:00Z',
    updated_by: { id: 'u1', name: 'Administrador' },
    limits,
    warnings: [],
    ...overrides,
  };
}

function withLogo(overrides: Partial<DocumentIdentity> = {}) {
  return identity({
    logo: {
      id: 'l1',
      media_type: 'image/png',
      width: 200,
      height: 80,
      size_bytes: 2048,
      sha256: 'a'.repeat(64),
      uploaded_at: '2026-11-11T15:00:00Z',
      uploaded_by: { id: 'u1', name: 'Administrador' },
      data_uri: pixel,
      rasters: [
        { paper_width: 58, raster_id: 'r58', width_dots: 384, height_dots: 92, data_uri: pixel },
        { paper_width: 80, raster_id: 'r80', width_dots: 576, height_dots: 138, data_uri: pixel },
      ],
    },
    ...overrides,
  });
}

interface Call {
  operation: string;
  body: Record<string, unknown> | null;
}

interface Response {
  status: number;
  body: unknown;
}

type Handler = (body: Record<string, unknown> | null) => Response | Promise<Response>;

let calls: Call[] = [];
let handlers: Record<string, Handler> = {};

function respond(status: number, body: unknown): Response {
  return { status, body };
}

function serve(current: () => DocumentIdentity, extra: Record<string, Handler> = {}) {
  handlers = {
    documents_identity: () => respond(200, current()),
    ...extra,
  };
}

beforeEach(() => {
  calls = [];
  handlers = {};
  native.invoke.mockImplementation(
    async (_command: string, args: { operation: string; body: Record<string, unknown> | null }) => {
      calls.push({ operation: args.operation, body: args.body });
      const handler = handlers[args.operation];
      if (!handler) {
        return respond(404, { error: { code: 'NOT_FOUND', message: 'Ruta no disponible.' } });
      }
      return handler(args.body);
    },
  );
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  native.invoke.mockReset();
});

function callsTo(operation: string) {
  return calls.filter((call) => call.operation === operation);
}

// Cuerpo de la llamada número index (desde 0) a una operación.
function bodyOf(operation: string, index: number) {
  const call = callsTo(operation)[index];
  if (!call) {
    throw new Error(`No hubo llamada ${index} a ${operation}`);
  }
  return call.body ?? {};
}

async function renderPanel(permissions = manager, onOpenSettings = vi.fn()) {
  render(<IdentityPanel permissions={permissions} onOpenSettings={onOpenSettings} />);
  await screen.findByText('Datos del establecimiento');
  return onOpenSettings;
}

describe('IdentityPanel', () => {
  it('carga la identidad con los datos de Configuración general en solo consulta', async () => {
    serve(() => identity());
    const onOpenSettings = await renderPanel();
    expect(screen.getByText('Fonda Doña Rosa')).toBeTruthy();
    expect(screen.getByText('Calle 1, Centro')).toBeTruthy();
    // El correo vacío se indica sin inventar un valor.
    expect(screen.getByText('Sin capturar')).toBeTruthy();
    expect((screen.getByLabelText('Información adicional') as HTMLTextAreaElement).value).toBe(
      'RFC: XAXX010101000',
    );
    expect(
      screen.getAllByText(/La marca AD-EL POS de la aplicación no cambia/).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText(/prueba física pendiente/)).toBeTruthy();
    expect(screen.getAllByText(/no imprime nada/).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole('button', { name: 'Editar en Configuración general' }));
    expect(onOpenSettings).toHaveBeenCalledTimes(1);
    // Ningún campo numérico nativo.
    expect(document.querySelector('input[type="number"]')).toBeNull();
  });

  it('muestra el original y cómo saldrá en papel de 58 y 80 mm', async () => {
    serve(() => withLogo());
    await renderPanel();
    expect(screen.getByAltText('Logotipo original del restaurante').getAttribute('src')).toBe(
      pixel,
    );
    expect(screen.getByAltText('Logotipo convertido para papel de 58 mm')).toBeTruthy();
    expect(screen.getByAltText('Logotipo convertido para papel de 80 mm')).toBeTruthy();
    expect(screen.getByText('384 × 92 puntos')).toBeTruthy();
    expect(screen.getByText('576 × 138 puntos')).toBeTruthy();
    expect(screen.getByText(/PNG · 200 × 80 píxeles/)).toBeTruthy();
    expect(screen.getByText('Presentación guardada.')).toBeTruthy();
  });

  it('pide la vista previa de otra presentación con antirrebote y una sola petición en vuelo', async () => {
    let release: (() => void) | null = null;
    serve(() => withLogo(), {
      documents_preview_identity_logo: (body) =>
        new Promise<Response>((resolve) => {
          release = () =>
            resolve(
              respond(200, {
                presentation: body?.presentation,
                rasters: [
                  { paper_width: 58, width_dots: 384, height_dots: 120, data_uri: pixel },
                  { paper_width: 80, width_dots: 576, height_dots: 180, data_uri: pixel },
                ],
              }),
            );
        }),
    });
    await renderPanel();
    vi.useFakeTimers();
    fireEvent.click(screen.getByRole('button', { name: 'Derecha' }));
    fireEvent.click(screen.getByRole('button', { name: '80 %' }));
    // Antes del antirrebote no hay petición.
    expect(callsTo('documents_preview_identity_logo')).toHaveLength(0);
    await act(async () => {
      vi.advanceTimersByTime(350);
    });
    expect(callsTo('documents_preview_identity_logo')).toHaveLength(1);
    expect(bodyOf('documents_preview_identity_logo', 0)).toEqual({
      presentation: { width_percent: 80, align: 'right', method: 'threshold', threshold: 128 },
    });
    // Otro cambio mientras la primera sigue en vuelo: no sale una segunda petición todavía.
    fireEvent.click(screen.getByRole('button', { name: 'Difuminado' }));
    await act(async () => {
      vi.advanceTimersByTime(350);
    });
    expect(callsTo('documents_preview_identity_logo')).toHaveLength(1);
    expect(screen.getByText('Preparando la vista previa…')).toBeTruthy();
    vi.useRealTimers();
    await act(async () => {
      release?.();
    });
    // Al terminar, la última presentación pedida gana.
    await waitFor(() => expect(callsTo('documents_preview_identity_logo')).toHaveLength(2));
    expect(bodyOf('documents_preview_identity_logo', 1)).toEqual({
      presentation: { width_percent: 80, align: 'right', method: 'dither', threshold: 128 },
    });
    await act(async () => {
      release?.();
    });
    await screen.findByText(/Vista previa sin guardar/);
    expect(screen.getByText('384 × 120 puntos')).toBeTruthy();
    expect(screen.getByText('El difuminado no usa el umbral.')).toBeTruthy();
    expect(screen.getByText('Hay cambios sin guardar.')).toBeTruthy();
  });

  it('sube el logotipo en base64 con la revisión vigente y vuelve a cargar la identidad', async () => {
    let current = identity();
    serve(() => current, {
      documents_upload_identity_logo: () => {
        current = withLogo({ revision: 4 });
        return respond(200, { ...current, logo: { ...current.logo, data_uri: undefined } });
      },
    });
    await renderPanel();
    const input = screen.getByLabelText('Subir logotipo') as HTMLInputElement;
    expect(input.getAttribute('accept')).toBe('image/png,image/jpeg');
    const file = new File([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])], 'logo.png', {
      type: 'image/png',
    });
    fireEvent.change(input, { target: { files: [file] } });
    await waitFor(() => expect(callsTo('documents_upload_identity_logo')).toHaveLength(1));
    const body = bodyOf('documents_upload_identity_logo', 0);
    expect(body.content_base64).toBe('iVBORw0KGgo=');
    expect(body.revision).toBe(3);
    expect(String(body.operation_id)).toMatch(/^[0-9a-f-]{36}$/);
    await screen.findByAltText('Logotipo original del restaurante');
    expect(await screen.findByText(/Logotipo cargado/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Retirar logotipo' })).toBeTruthy();
  });

  it('rechaza en pantalla un archivo de más de 1 MiB sin enviarlo', async () => {
    serve(() => identity());
    await renderPanel();
    const big = new File([new Uint8Array(1048577)], 'grande.png', { type: 'image/png' });
    fireEvent.change(screen.getByLabelText('Subir logotipo'), { target: { files: [big] } });
    expect(await screen.findByText('El logotipo debe ocupar como máximo 1 MiB.')).toBeTruthy();
    expect(callsTo('documents_upload_identity_logo')).toHaveLength(0);
  });

  it('retira el logotipo solo después de confirmarlo', async () => {
    let current = withLogo();
    serve(() => current, {
      documents_remove_identity_logo: () => {
        current = identity({ revision: 4 });
        return respond(200, current);
      },
    });
    await renderPanel();
    fireEvent.click(screen.getByRole('button', { name: 'Retirar logotipo' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText(/conservan su logotipo al reimprimirse/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancelar' }));
    expect(callsTo('documents_remove_identity_logo')).toHaveLength(0);
    fireEvent.click(screen.getByRole('button', { name: 'Retirar logotipo' }));
    fireEvent.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Retirar logotipo',
      }),
    );
    await waitFor(() => expect(callsTo('documents_remove_identity_logo')).toHaveLength(1));
    expect(bodyOf('documents_remove_identity_logo', 0).revision).toBe(3);
    await waitFor(() =>
      expect(screen.queryByAltText('Logotipo original del restaurante')).toBeNull(),
    );
    expect(await screen.findByText(/Logotipo retirado/)).toBeTruthy();
  });

  it('sin documents.identity.manage todo es de solo lectura', async () => {
    serve(() => withLogo());
    await renderPanel(reader);
    expect(screen.getByText(/Solo lectura/)).toBeTruthy();
    expect(screen.queryByLabelText('Reemplazar logotipo')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Retirar logotipo' })).toBeNull();
    expect(screen.queryByRole('button', { name: /Guardar cambios/ })).toBeNull();
    const textarea = screen.getByLabelText('Texto de pie') as HTMLTextAreaElement;
    expect(textarea.readOnly).toBe(true);
    for (const name of ['Derecha', 'Difuminado', '80 %']) {
      expect((screen.getByRole('button', { name }) as HTMLButtonElement).disabled).toBe(true);
    }
    // La vista previa guardada sí se consulta.
    expect(screen.getByAltText('Logotipo convertido para papel de 58 mm')).toBeTruthy();
    expect(callsTo('documents_preview_identity_logo')).toHaveLength(0);
  });

  it('ante un conflicto de revisión conserva lo escrito y lo guarda sobre la versión nueva', async () => {
    let current = identity();
    let saves = 0;
    serve(() => current, {
      documents_save_identity: (body) => {
        saves++;
        if (saves === 1) {
          // Otra persona guardó antes: la revisión vigente ya es la 4.
          current = identity({ revision: 4, header_text: 'Encabezado de otra persona' });
          return respond(409, {
            error: { code: 'REVISION_CONFLICT', message: 'Otra persona guardó la identidad.' },
          });
        }
        current = identity({
          revision: 5,
          header_text: 'Encabezado de otra persona',
          footer_text: String(body?.footer_text),
        });
        return respond(200, current);
      },
    });
    await renderPanel();
    const footer = screen.getByLabelText('Texto de pie') as HTMLTextAreaElement;
    fireEvent.change(footer, { target: { value: 'Vuelva pronto' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText(
      /Otra persona guardó la identidad del establecimiento mientras editabas/,
    );
    expect((screen.getByLabelText('Texto de pie') as HTMLTextAreaElement).value).toBe(
      'Vuelva pronto',
    );
    expect(bodyOf('documents_save_identity', 0).revision).toBe(3);
    fireEvent.click(
      screen.getByRole('button', { name: 'Guardar mis cambios sobre la versión nueva' }),
    );
    await waitFor(() => expect(callsTo('documents_save_identity')).toHaveLength(2));
    const second = bodyOf('documents_save_identity', 1);
    expect(second.revision).toBe(4);
    expect(second.footer_text).toBe('Vuelva pronto');
    expect(second.operation_id).not.toBe(bodyOf('documents_save_identity', 0).operation_id);
    await screen.findByText('Sin cambios pendientes.');
    expect((screen.getByLabelText('Texto de pie') as HTMLTextAreaElement).value).toBe(
      'Vuelva pronto',
    );
  });

  it('descartar ante el conflicto muestra la versión guardada', async () => {
    let current = identity();
    serve(() => current, {
      documents_save_identity: () => {
        current = identity({ revision: 4, footer_text: 'Pie de otra persona' });
        return respond(409, { error: { code: 'REVISION_CONFLICT', message: 'Conflicto.' } });
      },
    });
    await renderPanel();
    fireEvent.change(screen.getByLabelText('Texto de pie'), { target: { value: 'Mío' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Descartar mis cambios y ver la versión guardada',
      }),
    );
    expect((screen.getByLabelText('Texto de pie') as HTMLTextAreaElement).value).toBe(
      'Pie de otra persona',
    );
    expect(callsTo('documents_save_identity')).toHaveLength(1);
  });

  it('una respuesta incierta se reintenta con el mismo operation_id', async () => {
    let attempts = 0;
    let current = identity();
    serve(() => current, {
      documents_save_identity: (body) => {
        attempts++;
        if (attempts === 1) {
          return Promise.reject(new Error('No se pudo conectar con el servidor.'));
        }
        current = identity({ revision: 4, footer_text: String(body?.footer_text) });
        return respond(200, current);
      },
    });
    await renderPanel();
    fireEvent.change(screen.getByLabelText('Texto de pie'), { target: { value: 'Pie nuevo' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar cambios' }));
    await screen.findByText(/No se confirmó si se pudo guardar la identidad/);
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }));
    await waitFor(() => expect(callsTo('documents_save_identity')).toHaveLength(2));
    expect(bodyOf('documents_save_identity', 1).operation_id).toBe(
      bodyOf('documents_save_identity', 0).operation_id,
    );
    await screen.findByText('Sin cambios pendientes.');
  });

  it('el código del panel no usa campos numéricos ni diálogos nativos', () => {
    const sources = [
      '../IdentityPanel.tsx',
      './IdentityTexts.tsx',
      './LogoSection.tsx',
      './PresentationControls.tsx',
      './PaperPreview.tsx',
      './EstablishmentSummary.tsx',
      './useIdentityEditor.ts',
    ].map((path) => readFileSync(new URL(path, import.meta.url), 'utf8'));
    for (const source of sources) {
      expect(source).not.toContain('type="number"');
      expect(source).not.toContain('window.confirm');
      expect(source).not.toContain('window.alert');
    }
  });
});
