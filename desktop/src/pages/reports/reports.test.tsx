// @vitest-environment jsdom

// Etapa 9A, parte C2: módulo Reportes. Navegación según el catálogo de Go y los permisos, filtro de
// periodo (cada opción, semana ISO con su rango y rango personalizado), tabla con los totales de Go
// en el pie, paginación sin cambiar totales y columnas que Go no entrega, estados de carga, vacío y
// error, resumen ejecutivo sin recarga por eventos SSE y con «Actualizar», y exportación por
// permiso. Ningún campo usa type="number".

import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PeriodFilter } from '../../components/reports/PeriodFilter';
import {
  defaultPeriod,
  isoWeekRange,
  isoWeeksInYear,
  periodQuery,
  shiftIsoWeek,
} from '../../components/reports/period';
import type { PeriodSelection } from '../../components/reports/period';
import { ReportTable } from '../../components/reports/ReportTable';
import {
  formatReportValue,
  formatSignedPercent,
  formatSignedValue,
} from '../../components/reports/reportValues';
import type {
  CatalogReport,
  Dashboard,
  FilterOption,
  ReportCatalog,
  ReportResult,
  ResolvedPeriod,
} from '../../services/reports';
import { screenAuthorized, sectionByID } from '../../navigation/sections';
import { dashboardRefreshMilliseconds } from './DashboardView';
import { ReportChart } from './ReportChart';
import { ReportsPage } from './ReportsPage';

const native = vi.hoisted(() => ({ invoke: vi.fn() }));
vi.mock('@tauri-apps/api/core', () => ({ invoke: native.invoke, isTauri: () => true }));

const timezone = 'America/Monterrey';

const periods: FilterOption[] = [
  { value: 'today', label: 'Hoy' },
  { value: 'yesterday', label: 'Ayer' },
  { value: 'current_week', label: 'Semana actual' },
  { value: 'current_month', label: 'Mes actual' },
  { value: 'current_year', label: 'Año actual' },
  { value: 'day', label: 'Día' },
  { value: 'iso_week', label: 'Semana del año (ISO 8601)' },
  { value: 'month', label: 'Mes' },
  { value: 'year', label: 'Año' },
  { value: 'range', label: 'Periodo personalizado' },
];

const today: ResolvedPeriod = {
  kind: 'today',
  label: 'Hoy, martes 10 de noviembre de 2026',
  first_day: '2026-11-10',
  last_day: '2026-11-10',
  from: '2026-11-10T11:00:00Z',
  until: '2026-11-11T11:00:00Z',
  days: 1,
  in_progress: true,
};

const yesterday: ResolvedPeriod = {
  ...today,
  kind: 'day',
  label: 'Lunes 9 de noviembre de 2026',
  first_day: '2026-11-09',
  last_day: '2026-11-09',
  in_progress: false,
};

const salesByDay: CatalogReport = {
  id: 'sales.by_day',
  title: 'Ventas por día',
  description: 'Venta de cada día comercial del periodo.',
  date_basis: 'Fecha de cierre de la cuenta, con la hora de inicio del día.',
  period: 'required',
  comparison: false,
  filters: [
    { key: 'category_id', label: 'Categoría', type: 'id', source: 'categories' },
    {
      key: 'mode',
      label: 'Modalidad',
      type: 'choice',
      options: [
        { value: 'dine_in', label: 'Salón' },
        { value: 'takeaway', label: 'Para llevar y ventanilla' },
      ],
    },
  ],
  columns: [
    {
      key: 'day',
      label: 'Día',
      type: 'date',
      visibility: 'default',
      total: 'none',
      sortable: true,
    },
    {
      key: 'accounts',
      label: 'Cuentas',
      type: 'integer',
      visibility: 'default',
      total: 'sum',
      sortable: true,
    },
    {
      key: 'net',
      label: 'Venta neta',
      type: 'money_mxn',
      visibility: 'default',
      total: 'sum',
      sortable: true,
    },
    {
      key: 'base',
      label: 'Subtotal sin IVA',
      type: 'money_mxn',
      visibility: 'optional',
      total: 'sum',
      sortable: true,
    },
  ],
  default_sort: { key: 'day', direction: 'asc' },
  export: { pdf: true, excel: true },
};

const salesByProduct: CatalogReport = {
  ...salesByDay,
  id: 'sales.by_product',
  title: 'Ventas por producto',
  description: 'Cantidades e ingresos por producto.',
  filters: [],
  columns: [
    {
      key: 'product',
      label: 'Producto',
      type: 'text',
      visibility: 'default',
      total: 'none',
      sortable: true,
    },
    salesByDay.columns[2]!,
  ],
  default_sort: { key: 'net', direction: 'desc' },
};

const paymentsList: CatalogReport = {
  ...salesByDay,
  id: 'payments.list',
  title: 'Detalle de cobros',
  description: 'Cada cobro con su método.',
  filters: [{ key: 'method_id', label: 'Método de pago', type: 'id', source: 'payment_methods' }],
  columns: [
    {
      key: 'created_at',
      label: 'Fecha',
      type: 'datetime',
      visibility: 'default',
      total: 'none',
      sortable: true,
    },
    {
      key: 'method',
      label: 'Método',
      type: 'text',
      visibility: 'default',
      total: 'none',
      sortable: true,
    },
    {
      key: 'amount',
      label: 'Importe',
      type: 'money_mxn',
      visibility: 'default',
      total: 'sum',
      sortable: true,
    },
  ],
  default_sort: { key: 'created_at', direction: 'asc' },
  export: { pdf: false, excel: false },
};

function catalogFor(options: { payments?: boolean; dashboard?: boolean } = {}): ReportCatalog {
  const groups: ReportCatalog['groups'] = [
    { id: 'sales', title: 'Ventas', reports: [salesByDay, salesByProduct] },
  ];
  if (options.payments !== false) {
    groups.push({ id: 'payments', title: 'Pagos', reports: [paymentsList] });
  }
  return {
    groups,
    periods,
    dashboard: options.dashboard !== false,
    timezone,
    day_start: '05:00',
  };
}

function resultFor(report: CatalogReport, query: Record<string, string>): ReportResult {
  const page = Number(query.page ?? '1');
  const base = {
    report: {
      id: report.id,
      group: 'sales',
      title: report.title,
      description: report.description,
    },
    meta: {
      title: report.title,
      period: query.period === 'yesterday' ? yesterday : today,
      comparison: null,
      filters: query.mode
        ? [{ key: 'mode', label: 'Modalidad', value: query.mode, display: 'Salón' }]
        : [],
      generated_at: '2026-11-10T18:00:00Z',
      timezone,
      day_start: '05:00',
      date_basis: report.date_basis,
      warnings: [] as ReportResult['meta']['warnings'],
      notes: [] as string[],
    },
    sort: {
      key: query.sort ?? report.default_sort.key,
      direction: (query.direction ?? 'asc') as 'asc' | 'desc',
    },
  };
  if (report.id === 'sales.by_day') {
    // Go protege «Subtotal sin IVA» en este ejemplo: no llega en las columnas ni en los renglones.
    const rows =
      page === 1
        ? [
            { day: '2026-11-09', accounts: 3, net: '1200.50' },
            { day: '2026-11-10', accounts: 2, net: '800.25' },
          ]
        : [{ day: '2026-11-11', accounts: 1, net: '99.25' }];
    return {
      ...base,
      columns: report.columns.filter((column) => column.key !== 'base'),
      rows,
      // Totales de todo el conjunto (no de la página): React no los recalcula.
      totals: { day: null, accounts: 6, net: '2100.00' },
      summary: [{ key: 'accounts', label: 'Cuentas cerradas', type: 'integer', value: 6 }],
      page: { page, page_size: 2, total_rows: 3, total_pages: 2 },
    };
  }
  if (report.id === 'payments.list') {
    return {
      ...base,
      meta: {
        ...base.meta,
        warnings: [{ code: 'SHIFT_CLOSING', message: 'Turnos en arqueo excluidos: 1.' }],
      },
      columns: report.columns,
      rows: [],
      totals: { created_at: null, method: null, amount: '0.00' },
      summary: [],
      page: { page: 1, page_size: 50, total_rows: 0, total_pages: 0 },
    };
  }
  return {
    ...base,
    columns: report.columns,
    rows: [
      { product: 'Filete', net: '900.00' },
      { product: 'Refresco', net: '100.00' },
    ],
    totals: { product: null, net: '1000.00' },
    summary: [],
    page: { page: 1, page_size: 50, total_rows: 2, total_pages: 1 },
  };
}

function dashboardFor(netValue: string): Dashboard {
  return {
    period: today,
    comparison: { ...yesterday, label: 'Ayer, lunes 9 de noviembre de 2026' },
    generated_at: '2026-11-10T18:05:00Z',
    timezone,
    day_start: '05:00',
    sections: [
      {
        id: 'sales',
        title: 'Ventas',
        indicators: [
          {
            key: 'net_sales',
            label: 'Ventas netas',
            type: 'money_mxn',
            value: netValue,
            previous: '3980.00',
            change: '540.00',
            change_percent: '13.57',
            report_id: 'sales.by_day',
          },
          {
            key: 'accounts',
            label: 'Cuentas cerradas',
            type: 'integer',
            value: 12,
            previous: 10,
            change: 2,
            change_percent: '20.00',
            report_id: 'sales.by_day',
          },
          {
            key: 'open_balance',
            label: 'Saldo en curso',
            type: 'money_mxn',
            value: '75.00',
            previous: null,
            change: null,
            change_percent: null,
          },
        ],
      },
      {
        id: 'discounts',
        title: 'Descuentos',
        indicators: [
          {
            key: 'discounts',
            label: 'Descuentos',
            type: 'money_mxn',
            value: '50.00',
            previous: '20.00',
            change: '30.00',
            change_percent: '150.00',
            report_id: 'payments.list',
          },
        ],
      },
    ],
    series: [],
    warnings: [],
  };
}

interface Calls {
  operation: string;
  id: unknown;
  query: Record<string, string>;
}

let calls: Calls[];
let exports: { report: string; format: string; query: Record<string, string> }[];
let dashboardNet: string;
let failSales: boolean;
let catalog: ReportCatalog;
let exportOutcome: unknown;

function requests(operation: string) {
  return calls.filter((call) => call.operation === operation);
}

beforeEach(() => {
  calls = [];
  exports = [];
  dashboardNet = '4520.00';
  failSales = false;
  catalog = catalogFor();
  exportOutcome = { status: 200, body: { result: 'saved', file_name: 'ventas.pdf', size: 24576 } };
  native.invoke.mockImplementation(async (command: string, args: Record<string, unknown>) => {
    if (command === 'export_report') {
      exports.push(args as (typeof exports)[number]);
      return exportOutcome;
    }
    const operation = String(args.operation);
    const query = (args.query ?? {}) as Record<string, string>;
    calls.push({ operation, id: args.resourceId, query });
    if (operation === 'reports_catalog') {
      return { status: 200, body: catalog };
    }
    if (operation === 'reports_dashboard') {
      return { status: 200, body: dashboardFor(dashboardNet) };
    }
    if (operation === 'reports_filters') {
      return {
        status: 200,
        body: {
          source: args.resourceId,
          options: [{ value: '11111111-1111-4111-8111-111111111111', label: 'Bebidas' }],
        },
      };
    }
    if (operation === 'reports_run') {
      if (failSales) {
        return {
          status: 503,
          body: { error: { code: 'REPORTS_BUSY', message: 'Hay otros reportes en curso.' } },
        };
      }
      const report = catalog.groups
        .flatMap((group) => group.reports)
        .find((entry) => entry.id === args.resourceId)!;
      return { status: 200, body: resultFor(report, query) };
    }
    return { status: 404, body: { error: { code: 'NOT_FOUND', message: 'No existe.' } } };
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

function renderPage(
  permissions: string[] = ['reports.read', 'reports.export.pdf', 'reports.export.excel'],
) {
  return render(
    <ReportsPage
      can={(permission) => permissions.includes(permission)}
      timezone={timezone}
      permissions={permissions}
    />,
  );
}

function groupTabs() {
  return within(screen.getByRole('tablist', { name: 'Grupos de reportes' })).getAllByRole('tab');
}

async function openSalesByDay() {
  renderPage();
  await screen.findByRole('heading', { name: 'Resumen ejecutivo' });
  fireEvent.click(screen.getByRole('tab', { name: 'Ventas' }));
  await screen.findByRole('heading', { name: 'Ventas por día' });
  await screen.findByRole('cell', { name: '09/11/2026' });
}

describe('pantalla Reportes y navegación por catálogo', () => {
  it('es un mosaico de Administración que se ve con reports.read', () => {
    const entry = sectionByID('reports');
    expect(entry.label).toBe('Reportes');
    expect(entry.parent).toBe('administration');
    expect(screenAuthorized(entry, ['reports.read'])).toBe(true);
    expect(screenAuthorized(entry, ['reports.sales.read'])).toBe(false);
  });

  it('muestra solo los grupos y reportes que entrega Go, con el resumen ejecutivo primero', async () => {
    renderPage();
    await screen.findByRole('heading', { name: 'Resumen ejecutivo' });
    expect(groupTabs().map((tab) => tab.textContent)).toEqual([
      'Resumen ejecutivo',
      'Ventas',
      'Pagos',
    ]);
    expect(screen.getByRole('tab', { name: 'Resumen ejecutivo' })).toHaveProperty(
      'ariaSelected',
      'true',
    );
    fireEvent.click(screen.getByRole('tab', { name: 'Ventas' }));
    const list = await screen.findByRole('navigation', { name: 'Reportes de Ventas' });
    expect(
      within(list)
        .getAllByRole('button')
        .map((button) => button.textContent),
    ).toEqual(['Ventas por día', 'Ventas por producto']);
    // El grupo de un solo reporte no lleva lista: muestra el reporte directamente.
    fireEvent.click(screen.getByRole('tab', { name: 'Pagos' }));
    await screen.findByRole('heading', { name: 'Detalle de cobros' });
    expect(screen.queryByRole('navigation', { name: 'Reportes de Pagos' })).toBeNull();
    // Cada grupo recuerda su último reporte.
    fireEvent.click(screen.getByRole('tab', { name: 'Ventas' }));
    fireEvent.click(screen.getByRole('button', { name: 'Ventas por producto' }));
    await screen.findByRole('heading', { name: 'Ventas por producto' });
    fireEvent.click(screen.getByRole('tab', { name: 'Pagos' }));
    fireEvent.click(screen.getByRole('tab', { name: 'Ventas' }));
    expect(await screen.findByRole('heading', { name: 'Ventas por producto' })).toBeTruthy();
  });

  it('un grupo sin permiso no aparece y sin dashboard empieza en el primer grupo', async () => {
    catalog = catalogFor({ payments: false, dashboard: false });
    renderPage(['reports.read']);
    await screen.findByRole('heading', { name: 'Ventas por día' });
    expect(groupTabs().map((tab) => tab.textContent)).toEqual(['Ventas']);
    expect(screen.queryByRole('tab', { name: 'Pagos' })).toBeNull();
    expect(requests('reports_dashboard')).toHaveLength(0);
  });

  it('sin reportes visibles lo explica', async () => {
    catalog = { ...catalogFor({ dashboard: false }), groups: [] };
    renderPage(['reports.read']);
    expect(await screen.findByText(/Tu usuario no tiene reportes disponibles/)).toBeTruthy();
  });
});

describe('filtro de periodo', () => {
  function renderFilter(value: PeriodSelection, onChange = vi.fn(), resolved = today) {
    render(
      <PeriodFilter
        periods={periods}
        value={value}
        onChange={onChange}
        resolved={resolved}
        allowCompare
        dayStart="05:00"
        timezone={timezone}
      />,
    );
    return onChange;
  }

  it('cada periodo rápido se aplica con un toque y Go dice el periodo aplicado', () => {
    const onChange = renderFilter({ ...defaultPeriod });
    expect(screen.getByText('Hoy, martes 10 de noviembre de 2026')).toBeTruthy();
    expect(screen.getByText('05:00 · America/Monterrey')).toBeTruthy();
    for (const [label, kind] of [
      ['Hoy', 'today'],
      ['Ayer', 'yesterday'],
      ['Semana actual', 'current_week'],
      ['Mes actual', 'current_month'],
      ['Año actual', 'current_year'],
    ] as const) {
      fireEvent.click(screen.getByRole('button', { name: label }));
      expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ period: kind }));
    }
    expect(periodQuery({ ...defaultPeriod, period: 'yesterday' }, false)).toEqual({
      period: 'yesterday',
    });
  });

  it('día, mes y año se completan con el día comercial de Go, no con la fecha de la terminal', () => {
    const onChange = renderFilter({ ...defaultPeriod }, vi.fn(), yesterday);
    fireEvent.click(screen.getByRole('button', { name: 'Día' }));
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ period: 'day', date: '2026-11-09' }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Mes' }));
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ period: 'month', month: '11', year: '2026' }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Año' }));
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ period: 'year', year: '2026' }),
    );
    expect(
      periodQuery({ ...defaultPeriod, period: 'month', month: '3', year: '2026' }, false),
    ).toEqual({ period: 'month', month: '2026-03' });
    expect(periodQuery({ ...defaultPeriod, period: 'day', date: '2026-11-09' }, false)).toEqual({
      period: 'day',
      date: '2026-11-09',
    });
  });

  it('semana del año ISO con su rango, la semana 53 y el cambio de año', () => {
    expect(isoWeeksInYear(2026)).toBe(53);
    expect(isoWeeksInYear(2025)).toBe(52);
    expect(isoWeekRange('2026', '46')).toEqual({ first: '2026-11-09', last: '2026-11-15' });
    expect(isoWeekRange('2026', '53')).toEqual({ first: '2026-12-28', last: '2027-01-03' });
    expect(isoWeekRange('2025', '53')).toBeNull();
    expect(isoWeekRange('2027', '1')).toEqual({ first: '2027-01-04', last: '2027-01-10' });
    expect(shiftIsoWeek('2026', '53', 1)).toEqual({ year: 2027, week: 1 });
    expect(shiftIsoWeek('2027', '1', -1)).toEqual({ year: 2026, week: 53 });

    const onChange = renderFilter({ ...defaultPeriod }, vi.fn(), {
      ...today,
      kind: 'current_week',
      iso_year: 2026,
      iso_week: 46,
    });
    fireEvent.click(screen.getByRole('button', { name: 'Semana del año' }));
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ period: 'iso_week', isoYear: '2026', isoWeek: '46' }),
    );
    expect(
      screen.getByText(
        'Semana 46 de 2026: del 9 al 15 de noviembre de 2026, de lunes a domingo (ISO 8601).',
      ),
    ).toBeTruthy();
    // La etiqueta «Semana» nombra el campo del número, no los botones de anterior y siguiente.
    const week = screen.getByRole('textbox', { name: 'Semana' });
    fireEvent.change(week, { target: { value: '53' } });
    expect(
      screen.getByText(
        'Semana 53 de 2026: del 28 de diciembre de 2026 al 3 de enero de 2027, de lunes a domingo (ISO 8601).',
      ),
    ).toBeTruthy();
    // Teclear no consulta: se aplica con «Aplicar periodo».
    expect(onChange).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('button', { name: 'Aplicar periodo' }));
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ period: 'iso_week', isoYear: '2026', isoWeek: '53' }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Semana siguiente' }));
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ isoYear: '2027', isoWeek: '1' }),
    );
    expect(
      periodQuery({ ...defaultPeriod, period: 'iso_week', isoYear: '2027', isoWeek: '1' }, false),
    ).toEqual({ period: 'iso_week', week: '2027-W01' });
  });

  it('rango personalizado con fechas dd/mm/aaaa, aplicado solo al completarlo', () => {
    const onChange = renderFilter({ ...defaultPeriod });
    fireEvent.click(screen.getByRole('button', { name: 'Personalizado' }));
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ period: 'range', from: '2026-11-10', until: '2026-11-10' }),
    );
    const from = screen.getByLabelText(/^Desde/, { selector: 'input' });
    const until = screen.getByLabelText(/^Hasta/, { selector: 'input' });
    fireEvent.change(from, { target: { value: '01112026' } });
    fireEvent.change(until, { target: { value: '1511' } });
    expect((from as HTMLInputElement).value).toBe('01/11/2026');
    expect(screen.getByRole('button', { name: 'Aplicar periodo' })).toHaveProperty(
      'disabled',
      true,
    );
    fireEvent.change(until, { target: { value: '15112026' } });
    fireEvent.click(screen.getByRole('button', { name: 'Aplicar periodo' }));
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ period: 'range', from: '2026-11-01', until: '2026-11-15' }),
    );
    expect(
      periodQuery(
        {
          ...defaultPeriod,
          period: 'range',
          from: '2026-11-01',
          until: '2026-11-15',
          compare: true,
        },
        true,
      ),
    ).toEqual({ period: 'range', from: '2026-11-01', until: '2026-11-15', compare: 'previous' });
  });

  it('la comparación con el periodo anterior es un interruptor', () => {
    const onChange = renderFilter({ ...defaultPeriod, compare: true });
    const toggle = screen.getByRole('button', { name: 'Comparar con el periodo anterior' });
    expect(toggle.getAttribute('aria-pressed')).toBe('true');
    fireEvent.click(toggle);
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ compare: false }));
  });
});

describe('reporte: tabla, filtros, paginación y estados', () => {
  it('muestra los totales de Go en el pie, pagina en Go sin cambiar los totales y omite columnas que Go no entrega', async () => {
    await openSalesByDay();
    const table = screen.getByRole('table');
    // La columna opcional protegida no llegó: no hay encabezado ni botón para mostrarla.
    expect(within(table).queryByText('Subtotal sin IVA')).toBeNull();
    expect(screen.queryByRole('button', { name: /columnas adicionales/ })).toBeNull();
    const footer = table.querySelector('tfoot')!;
    expect(footer.textContent).toContain('Total');
    // 2100.00 es el total de Go del conjunto completo (no la suma de esta página).
    expect(within(footer as HTMLElement).getByText('$2,100.00')).toBeTruthy();
    expect(within(footer as HTMLElement).getByText('6')).toBeTruthy();
    expect(screen.getByText('3 renglones · Página 1 de 2')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Siguiente' }));
    await screen.findByRole('cell', { name: '11/11/2026' });
    expect(requests('reports_run').at(-1)?.query.page).toBe('2');
    expect(within(table.querySelector('tfoot') as HTMLElement).getByText('$2,100.00')).toBeTruthy();
    expect(screen.getByText('3 renglones · Página 2 de 2')).toBeTruthy();
    // Fecha usada y cifras resumidas de Go.
    expect(
      screen.getByText('Fecha de cierre de la cuenta, con la hora de inicio del día.'),
    ).toBeTruthy();
    expect(screen.getByRole('list', { name: 'Cifras del reporte' }).textContent).toContain(
      'Cuentas cerradas6',
    );
  });

  it('ordena en Go con aria-sort y vuelve a la primera página', async () => {
    await openSalesByDay();
    const header = screen.getByRole('columnheader', { name: /Venta neta/ });
    expect(header.getAttribute('aria-sort')).toBe('none');
    expect(screen.getByRole('columnheader', { name: /Día/ }).getAttribute('aria-sort')).toBe(
      'ascending',
    );
    fireEvent.click(within(header).getByRole('button'));
    await waitFor(() => expect(requests('reports_run').at(-1)?.query.sort).toBe('net'));
    expect(requests('reports_run').at(-1)?.query).toMatchObject({ direction: 'desc', page: '1' });
    await waitFor(() =>
      expect(
        screen.getByRole('columnheader', { name: /Venta neta/ }).getAttribute('aria-sort'),
      ).toBe('descending'),
    );
  });

  it('el periodo y los filtros se eligen en un solo panel y se conservan al cambiar de reporte', async () => {
    await openSalesByDay();
    const before = requests('reports_run').length;
    fireEvent.click(screen.getByRole('button', { name: 'Ayer' }));
    await waitFor(() => expect(requests('reports_run').at(-1)?.query.period).toBe('yesterday'));
    await screen.findByText('Lunes 9 de noviembre de 2026');
    fireEvent.change(screen.getByRole('combobox', { name: 'Modalidad' }), {
      target: { value: 'dine_in' },
    });
    await waitFor(() => expect(requests('reports_run').at(-1)?.query.mode).toBe('dine_in'));
    expect(await screen.findByText('Modalidad: Salón')).toBeTruthy();
    // Las opciones de la categoría vienen de reports/filters/categories.
    const category = screen.getByRole('combobox', { name: 'Categoría' });
    await within(category).findByRole('option', { name: 'Bebidas' });
    expect(requests('reports_filters')[0]?.id).toBe('categories');
    // Una consulta por elección: Ayer y Modalidad.
    expect(requests('reports_run').length - before).toBe(2);
    // Otro reporte del grupo conserva el periodo; no envía filtros que no declara.
    fireEvent.click(screen.getByRole('button', { name: 'Ventas por producto' }));
    await screen.findByRole('heading', { name: 'Ventas por producto' });
    await waitFor(() => expect(requests('reports_run').at(-1)?.id).toBe('sales.by_product'));
    expect(requests('reports_run').at(-1)?.query).toMatchObject({ period: 'yesterday' });
    expect(requests('reports_run').at(-1)?.query.mode).toBeUndefined();
  });

  it('un filtro con el mismo parámetro pero otra fuente no se hereda entre reportes', async () => {
    const stock: CatalogReport = {
      ...salesByProduct,
      id: 'inventory.stock',
      title: 'Existencias por almacén',
      filters: [
        {
          key: 'category_id',
          label: 'Categoría de insumo',
          type: 'id',
          source: 'inventory_categories',
        },
      ],
    };
    catalog = {
      ...catalogFor({ payments: false }),
      groups: [
        { id: 'sales', title: 'Ventas', reports: [salesByDay] },
        { id: 'inventory', title: 'Inventarios', reports: [stock] },
      ],
    };
    renderPage();
    await screen.findByRole('heading', { name: 'Resumen ejecutivo' });
    fireEvent.click(screen.getByRole('tab', { name: 'Ventas' }));
    const category = await screen.findByRole('combobox', { name: 'Categoría' });
    await within(category).findByRole('option', { name: 'Bebidas' });
    fireEvent.change(category, { target: { value: '11111111-1111-4111-8111-111111111111' } });
    await waitFor(() =>
      expect(requests('reports_run').at(-1)?.query.category_id).toBe(
        '11111111-1111-4111-8111-111111111111',
      ),
    );
    fireEvent.click(screen.getByRole('tab', { name: 'Inventarios' }));
    await screen.findByRole('heading', { name: 'Existencias por almacén' });
    await waitFor(() => expect(requests('reports_run').at(-1)?.id).toBe('inventory.stock'));
    expect(requests('reports_run').at(-1)?.query.category_id).toBeUndefined();
    // De vuelta en Ventas, su categoría sigue aplicada.
    fireEvent.click(screen.getByRole('tab', { name: 'Ventas' }));
    await waitFor(() => expect(requests('reports_run').at(-1)?.id).toBe('sales.by_day'));
    expect(requests('reports_run').at(-1)?.query.category_id).toBe(
      '11111111-1111-4111-8111-111111111111',
    );
  });

  it('estado vacío, avisos de Go y error con reintento', async () => {
    renderPage();
    await screen.findByRole('heading', { name: 'Resumen ejecutivo' });
    fireEvent.click(screen.getByRole('tab', { name: 'Pagos' }));
    expect(
      await screen.findByText(/No hay datos en el periodo seleccionado \(Hoy, martes 10/),
    ).toBeTruthy();
    expect(screen.getByText('Turnos en arqueo excluidos: 1.')).toBeTruthy();
    expect(screen.getByRole('table').querySelector('tfoot')).toBeNull();

    failSales = true;
    fireEvent.click(screen.getByRole('tab', { name: 'Ventas' }));
    expect(await screen.findByText('Hay otros reportes en curso.')).toBeTruthy();
    failSales = false;
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }));
    expect(await screen.findByRole('cell', { name: '09/11/2026' })).toBeTruthy();
  });

  it('mientras carga lo indica', async () => {
    let release: (value: unknown) => void = () => undefined;
    native.invoke.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          release = resolve;
        }),
    );
    renderPage();
    expect(screen.getByText('Cargando reportes…')).toBeTruthy();
    await act(async () => {
      release({ status: 200, body: catalog });
    });
    expect(await screen.findByRole('heading', { name: 'Resumen ejecutivo' })).toBeTruthy();
  });

  it('la tabla sin resultado muestra la carga y con resultado vacío su mensaje', () => {
    const { rerender } = render(
      <ReportTable
        title="Ventas por día"
        columns={salesByDay.columns}
        result={null}
        loading
        error=""
        onRetry={() => undefined}
        onPage={() => undefined}
        timezone={timezone}
        emptyMessage="No hay datos."
      />,
    );
    expect(screen.getByText('Cargando reporte…')).toBeTruthy();
    const empty = resultFor(paymentsList, {});
    rerender(
      <ReportTable
        title="Detalle de cobros"
        columns={empty.columns}
        result={empty}
        loading={false}
        error=""
        onRetry={() => undefined}
        onPage={() => undefined}
        timezone={timezone}
        emptyMessage="No hay datos."
      />,
    );
    expect(screen.getByText('No hay datos.')).toBeTruthy();
  });
});

describe('resumen ejecutivo', () => {
  it('muestra los indicadores con su comparación y abre el reporte detallado desde una tarjeta', async () => {
    renderPage();
    const sales = await screen.findByRole('region', { name: 'Ventas' });
    const card = await within(sales).findByRole('button', { name: /Ventas netas/ });
    expect(card.textContent).toContain('$4,520.00');
    expect(card.textContent).toContain('+13.57 % frente al periodo anterior');
    expect(card.textContent).toContain('Anterior: $3,980.00');
    // Situación actual sin valor anterior ni reporte: tarjeta sin comparación y sin enlace.
    expect(within(sales).queryByRole('button', { name: /Saldo en curso/ })).toBeNull();
    expect(within(sales).getByText('Saldo en curso')).toBeTruthy();
    // En descuentos subir no es favorable: el tono se invierte (el texto dice la variación).
    const discounts = screen.getByRole('region', { name: 'Descuentos' });
    const discountCard = within(discounts).getByRole('button', { name: /Descuentos/ });
    expect(discountCard.querySelector('.metric-delta-bad')).toBeTruthy();
    expect(card.querySelector('.metric-delta-good')).toBeTruthy();
    expect(screen.getByText('Ayer, lunes 9 de noviembre de 2026')).toBeTruthy();
    expect(requests('reports_dashboard')[0]?.query).toEqual({
      period: 'today',
      compare: 'previous',
    });
    fireEvent.click(card);
    await screen.findByRole('heading', { name: 'Ventas por día' });
    expect(screen.getByRole('tab', { name: 'Ventas' }).getAttribute('aria-selected')).toBe('true');
  });

  it('no se recalcula con los eventos SSE; sí con «Actualizar» y con el temporizador solo si está visible', async () => {
    const intervals: { callback: () => void; delay: number }[] = [];
    const original = window.setInterval.bind(window);
    vi.spyOn(window, 'setInterval').mockImplementation(((callback: () => void, delay?: number) => {
      intervals.push({ callback, delay: delay ?? 0 });
      return original(() => undefined, 1_000_000);
    }) as typeof window.setInterval);
    renderPage();
    await screen.findByText('$4,520.00');
    expect(requests('reports_dashboard')).toHaveLength(1);
    dashboardNet = '9999.00';
    await act(async () => {
      window.dispatchEvent(new CustomEvent('adel-server-change'));
      await new Promise((resolve) => setTimeout(resolve, 150));
    });
    expect(requests('reports_dashboard')).toHaveLength(1);
    expect(screen.queryByText('$9,999.00')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Actualizar' }));
    expect(await screen.findByText('$9,999.00')).toBeTruthy();
    expect(requests('reports_dashboard')).toHaveLength(2);

    const timer = intervals.find((entry) => entry.delay === dashboardRefreshMilliseconds);
    expect(timer, 'temporizador de 5 minutos').toBeTruthy();
    await act(async () => {
      timer!.callback();
    });
    await waitFor(() => expect(requests('reports_dashboard')).toHaveLength(3));
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    await act(async () => {
      timer!.callback();
    });
    expect(requests('reports_dashboard')).toHaveLength(3);
    visibility.mockRestore();
    expect(screen.getByText(/Actualizado a las/)).toBeTruthy();
  });
});

describe('exportación', () => {
  it('los botones aparecen solo con su permiso y el resultado se avisa', async () => {
    renderPage(['reports.read', 'reports.export.pdf']);
    await screen.findByRole('heading', { name: 'Resumen ejecutivo' });
    fireEvent.click(screen.getByRole('tab', { name: 'Ventas' }));
    await screen.findByRole('cell', { name: '09/11/2026' });
    const group = screen.getByRole('group', { name: 'Exportar' });
    expect(within(group).getByRole('button', { name: 'Exportar PDF' })).toBeTruthy();
    expect(within(group).queryByRole('button', { name: 'Exportar Excel' })).toBeNull();
    fireEvent.click(within(group).getByRole('button', { name: 'Exportar PDF' }));
    expect(await screen.findByText('Ventas por día: PDF guardado.')).toBeTruthy();
    expect(screen.getByText('ventas.pdf · 24 KB')).toBeTruthy();
    expect(exports[0]?.report).toBe('sales.by_day');
    expect(exports[0]?.format).toBe('pdf');
    // La exportación lleva el periodo y el orden, sin la paginación de la pantalla.
    expect(exports[0]?.query).toMatchObject({ period: 'today' });
    expect(exports[0]?.query.page).toBeUndefined();

    exportOutcome = { status: 200, body: { result: 'cancelled' } };
    fireEvent.click(within(group).getByRole('button', { name: 'Exportar PDF' }));
    expect(
      await screen.findByText('Exportación a PDF cancelada: no se guardó ningún archivo.'),
    ).toBeTruthy();

    exportOutcome = {
      status: 403,
      body: { error: { code: 'PERMISSION_DENIED', message: 'No tienes permiso para exportar.' } },
    };
    fireEvent.click(within(group).getByRole('button', { name: 'Exportar PDF' }));
    expect(await screen.findByText('No tienes permiso para exportar.')).toBeTruthy();
  });

  it('sin permisos de exportación no hay botones, aunque el reporte se consulte', async () => {
    renderPage(['reports.read']);
    await screen.findByRole('heading', { name: 'Resumen ejecutivo' });
    fireEvent.click(screen.getByRole('tab', { name: 'Ventas' }));
    await screen.findByRole('cell', { name: '09/11/2026' });
    expect(screen.queryByRole('group', { name: 'Exportar' })).toBeNull();
    expect(screen.queryByRole('button', { name: /Exportar/ })).toBeNull();
  });
});

describe('formato sin aritmética y campos de texto', () => {
  it('da formato a cada tipo de columna sin calcular', () => {
    expect(formatReportValue('money_mxn', '1234.5', timezone)).toBe('$1,234.50');
    expect(formatReportValue('money_usd', '-20.00', timezone)).toBe('−US$20.00');
    expect(formatReportValue('integer', 12345, timezone)).toBe('12,345');
    expect(formatReportValue('quantity', '1250.125', timezone)).toBe('1,250.125');
    expect(formatReportValue('percent', '13.57', timezone)).toBe('13.57 %');
    expect(formatReportValue('date', '2026-11-09', timezone)).toBe('09/11/2026');
    expect(formatReportValue('money_mxn', null, timezone)).toBe('—');
    expect(formatSignedPercent('-4.20')).toBe('−4.20 %');
    expect(formatSignedPercent('0.00')).toBe('0.00 %');
    expect(formatSignedValue('money_mxn', '540.00')).toBe('+$540.00');
    expect(formatSignedValue('integer', -2)).toBe('−2');
  });

  it('ningún campo de Reportes usa type="number"', async () => {
    await openSalesByDay();
    fireEvent.click(screen.getByRole('button', { name: 'Semana del año' }));
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    fireEvent.click(screen.getByRole('button', { name: 'Personalizado' }));
    expect(document.querySelectorAll('input[type="number"]')).toHaveLength(0);
    expect(document.querySelectorAll('input').length).toBeGreaterThan(0);
  });
});

describe('gráficos SVG propios elegidos por la forma del resultado', () => {
  function chartResult(
    report: CatalogReport,
    rows: ReportResult['rows'],
    overrides: Partial<ReportResult> = {},
  ): ReportResult {
    return {
      ...resultFor(report, {}),
      columns: report.columns,
      rows,
      page: { page: 1, page_size: 50, total_rows: rows.length, total_pages: 1 },
      sort: { key: report.default_sort.key, direction: report.default_sort.direction },
      ...overrides,
    };
  }

  it('una secuencia de días dibuja la tendencia con su tabla alternativa', () => {
    const days = ['2026-11-09', '2026-11-10', '2026-11-11'].map((day, index) => ({
      day,
      accounts: index,
      net: ['0.00', '105.00', '20.50'][index]!,
    }));
    const closed = chartResult(salesByDay, days);
    closed.meta = { ...closed.meta, period: { ...yesterday, kind: 'range', in_progress: false } };
    const { unmount } = render(
      <ReportChart result={chartResult(salesByDay, days)} timezone={timezone} busy={false} />,
    );
    // En un periodo en curso se rotula el valor más alto (los días siguientes aún no ocurren).
    expect(
      screen
        .getByRole('figure', { name: 'Venta neta por día' })
        .querySelector('svg')
        ?.getAttribute('aria-label'),
    ).toBe('Venta neta por día: 3 puntos, de 09/11 a 11/11; valor más alto $105.00 (10/11).');
    unmount();
    render(<ReportChart result={closed} timezone={timezone} busy={false} />);
    const figure = screen.getByRole('figure', { name: 'Venta neta por día' });
    expect(within(figure).getByRole('img').getAttribute('aria-label')).toBe(
      'Venta neta por día: 3 puntos, de 09/11 a 11/11; último valor $20.50.',
    );
    // Rótulo directo solo en el último valor; los demás en el eje, la cruceta y la tabla.
    expect(within(figure).getByText('$20.50')).toBeTruthy();
    expect(within(figure).queryByText('$105.00')).toBeNull();
    fireEvent.click(within(figure).getByRole('button', { name: 'Ver como tabla' }));
    const table = within(figure).getByRole('region', { name: 'Datos de Venta neta por día' });
    expect(within(table).getByRole('rowheader', { name: '10/11' })).toBeTruthy();
    expect(within(table).getByText('$105.00')).toBeTruthy();
  });

  it('la secuencia en varias páginas o en otro orden no dibuja una tendencia engañosa', () => {
    const rows = [
      { day: '2026-11-09', accounts: 1, net: '10.00' },
      { day: '2026-11-10', accounts: 1, net: '20.00' },
    ];
    const { rerender } = render(
      <ReportChart
        result={chartResult(salesByDay, rows, {
          page: { page: 1, page_size: 2, total_rows: 4, total_pages: 2 },
        })}
        timezone={timezone}
        busy={false}
      />,
    );
    expect(screen.queryByRole('figure')).toBeNull();
    expect(screen.getByText(/cuando todo el periodo cabe en una página/)).toBeTruthy();
    rerender(
      <ReportChart
        result={chartResult(salesByDay, rows, { sort: { key: 'net', direction: 'desc' } })}
        timezone={timezone}
        busy={false}
      />,
    );
    expect(screen.queryByRole('figure')).toBeNull();
  });

  it('pocas categorías: barra segmentada; el color sigue al nombre y no al orden', () => {
    const rows = [
      { product: 'Refresco', net: '100.00' },
      { product: 'Filete', net: '900.00' },
    ];
    const { container, rerender } = render(
      <ReportChart result={chartResult(salesByProduct, rows)} timezone={timezone} busy={false} />,
    );
    expect(screen.getByRole('figure', { name: 'Distribución de venta neta' })).toBeTruthy();
    const toneOf = (name: string) =>
      [...container.querySelectorAll('.chart-legend li')]
        .find((item) => item.textContent === name)
        ?.querySelector('.chart-key')
        ?.className.match(/chart-tone-\d/)?.[0];
    expect(toneOf('Filete')).toBe('chart-tone-1');
    expect(toneOf('Refresco')).toBe('chart-tone-2');
    rerender(
      <ReportChart
        result={chartResult(salesByProduct, [...rows].reverse())}
        timezone={timezone}
        busy={false}
      />,
    );
    expect(toneOf('Filete')).toBe('chart-tone-1');
    expect(toneOf('Refresco')).toBe('chart-tone-2');
  });

  it('más categorías: barras horizontales con el valor de Go en la punta', () => {
    const rows = ['A', 'B', 'C', 'D', 'E', 'F'].map((name, index) => ({
      product: `Producto ${name}`,
      net: `${(6 - index) * 10}.00`,
    }));
    render(
      <ReportChart result={chartResult(salesByProduct, rows)} timezone={timezone} busy={false} />,
    );
    const figure = screen.getByRole('figure', { name: 'Venta neta por producto' });
    expect(within(figure).getByText('Producto A')).toBeTruthy();
    expect(within(figure).getByText('$60.00')).toBeTruthy();
    expect(within(figure).getByText('$10.00')).toBeTruthy();
  });

  it('un listado de detalle con fecha y hora no lleva gráfico', () => {
    const rows = [
      { created_at: '2026-11-10T18:00:00Z', method: 'Efectivo', amount: '10.00' },
      { created_at: '2026-11-10T19:00:00Z', method: 'Tarjeta', amount: '20.00' },
    ];
    render(
      <ReportChart result={chartResult(paymentsList, rows)} timezone={timezone} busy={false} />,
    );
    expect(screen.queryByRole('figure')).toBeNull();
  });
});
