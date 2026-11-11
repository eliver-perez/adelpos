// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest';
import { exportQuery, exportReport } from './reportExport';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

afterEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe('exportación de reportes por el puente nativo', () => {
  it('envía la consulta de la pantalla sin paginación y devuelve solo el resultado', async () => {
    native.invoke.mockResolvedValue({
      status: 200,
      body: { result: 'saved', file_name: 'ventas-por-dia_2026-11-10.pdf', size: 2048 },
    });
    const outcome = await exportReport(
      'sales.by_day',
      'pdf',
      { period: 'today', page: '3', page_size: '50', sort: 'net', direction: 'desc', mode: '' },
      { paper: 'a4' },
    );
    expect(outcome).toEqual({
      result: 'saved',
      file_name: 'ventas-por-dia_2026-11-10.pdf',
      size: 2048,
    });
    expect(native.invoke).toHaveBeenCalledWith('export_report', {
      report: 'sales.by_day',
      format: 'pdf',
      query: { period: 'today', sort: 'net', direction: 'desc', paper: 'a4' },
    });
  });

  it('el papel solo aplica al PDF', () => {
    expect(exportQuery({ period: 'today' }, 'xlsx', 'a4')).toEqual({ period: 'today' });
    expect(exportQuery({ period: 'today' }, 'pdf')).toEqual({ period: 'today' });
  });

  it('cancelar en el diálogo no es un error', async () => {
    native.invoke.mockResolvedValue({ status: 200, body: { result: 'cancelled' } });
    await expect(exportReport('payments.by_method', 'xlsx')).resolves.toEqual({
      result: 'cancelled',
    });
  });

  it('un rechazo de Go conserva su código y su mensaje', async () => {
    native.invoke.mockResolvedValue({
      status: 422,
      body: { error: { code: 'EXPORT_TOO_LARGE', message: 'El reporte tiene 61,234 renglones.' } },
    });
    const notification = vi.spyOn(window, 'dispatchEvent');
    await expect(exportReport('payments.list', 'pdf')).rejects.toMatchObject({
      code: 'EXPORT_TOO_LARGE',
      status: 422,
      message: 'El reporte tiene 61,234 renglones.',
    });
    native.invoke.mockResolvedValue({
      status: 403,
      body: { error: { code: 'PERMISSION_DENIED', message: 'No tienes permiso.' } },
    });
    await expect(exportReport('payments.list', 'xlsx')).rejects.toMatchObject({
      code: 'PERMISSION_DENIED',
      status: 403,
    });
    expect(notification).not.toHaveBeenCalled();
  });

  it('una sesión inválida avisa a la aplicación como cualquier operación', async () => {
    native.invoke.mockResolvedValue({
      status: 401,
      body: { error: { code: 'SESSION_EXPIRED', message: 'La sesión expiró.' } },
    });
    const notification = vi.spyOn(window, 'dispatchEvent');
    await expect(exportReport('sales.by_day', 'pdf')).rejects.toMatchObject({
      code: 'SESSION_EXPIRED',
    });
    expect(notification).toHaveBeenCalledOnce();
  });

  it('un rechazo local del puente llega como error de exportación con su mensaje', async () => {
    native.invoke.mockRejectedValue('Ya hay una exportación en curso.');
    await expect(exportReport('sales.by_day', 'pdf')).rejects.toMatchObject({
      code: 'EXPORT_FAILED',
      message: 'Ya hay una exportación en curso.',
    });
    native.invoke.mockResolvedValue({ status: 200, body: { result: 'otro' } });
    await expect(exportReport('sales.by_day', 'pdf')).rejects.toMatchObject({
      code: 'EXPORT_FAILED',
    });
  });
});
