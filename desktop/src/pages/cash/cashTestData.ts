import type { CashCatalog, CashOperation, RegisterShift } from '../../services/cash';
import type { CurrentShift, Shift } from '../../services/payments';

// Datos simulados para las pruebas de Caja. Las cifras representan lo que calcularía Go: las
// pruebas solo sustituyen el transporte nativo y nunca calculan importes.

export const registerID = '00000000-0000-4000-8000-0000000000a1';
export const shiftID = '00000000-0000-4000-8000-0000000000b1';

export function shiftOf(overrides: Partial<Shift> = {}): Shift {
  return {
    id: shiftID,
    register_id: registerID,
    register_name: 'Caja 1',
    state: 'open',
    opening_float: '500.00',
    opening_float_usd: null,
    opening_notes: '',
    business_date: '2026-11-09',
    opened_by: 'user-admin',
    opened_by_name: 'Administrador',
    opened_device_name: 'Terminal 1',
    opened_at: '2026-11-09T15:00:00Z',
    closed_at: null,
    revision: 1,
    shared_allowed: false,
    ...overrides,
  };
}

export function closingShift(): Shift {
  return shiftOf({
    state: 'closing',
    closing_started_by: 'user-admin',
    closing_started_by_name: 'Supervisora Ana',
    closing_started_at: '2026-11-09T22:00:00Z',
    closing_device_name: 'Terminal de barra',
  });
}

export function registersWith(shift: Shift | null): RegisterShift[] {
  return [{ register: { id: registerID, name: 'Caja 1', active: true }, shift, own: true }];
}

export function currentWith(shift: Shift | null): CurrentShift {
  return {
    register: { id: registerID, name: 'Caja 1', active: true },
    shift,
    shared_shifts: false,
    usable: shift?.state === 'open',
    reason: shift
      ? ''
      : 'No hay un turno abierto en Caja 1. Ábrelo con su fondo inicial antes de cobrar.',
    usd_cash: false,
    usd_rate_required: false,
  };
}

function denomination(id: string, currency: 'MXN' | 'USD', value: string, kind: 'bill' | 'coin') {
  return {
    id,
    currency,
    value,
    kind,
    active: true,
    revision: 1,
    updated_at: '2026-11-08T12:00:00Z',
  };
}

export const catalog: CashCatalog = {
  denominations: [
    denomination('den-mxn-500', 'MXN', '500.00', 'bill'),
    denomination('den-mxn-200', 'MXN', '200.00', 'bill'),
    denomination('den-mxn-100', 'MXN', '100.00', 'bill'),
    denomination('den-mxn-10', 'MXN', '10.00', 'coin'),
    denomination('den-usd-20', 'USD', '20.00', 'bill'),
  ],
  expense_categories: [
    {
      id: 'cat-insumos',
      name: 'Insumos',
      active: true,
      sort_order: 10,
      revision: 1,
      updated_at: '2026-11-08T12:00:00Z',
    },
  ],
  income_types: [{ code: 'float_contribution', label: 'Aportación adicional al fondo' }],
  withdrawal_types: [{ code: 'safekeeping', label: 'Resguardo' }],
};

export function operationOf(overrides: Partial<CashOperation> = {}): CashOperation {
  return {
    id: '00000000-0000-4000-8000-0000000000c1',
    shift_id: shiftID,
    register_id: registerID,
    register_name: 'Caja 1',
    kind: 'withdrawal',
    currency: 'MXN',
    amount: '200.00',
    direction: 'out',
    concept: 'Resguardo',
    type: 'safekeeping',
    type_label: 'Resguardo',
    category_id: null,
    category_name: null,
    reference: '',
    notes: '',
    reason: '',
    corrects_operation_id: null,
    corrected_by_operation_id: null,
    regularization: false,
    exceptional: false,
    usd_rate: null,
    equivalent_mxn: '200.00',
    actor_name: 'Administrador',
    device_name: 'Terminal 1',
    authorized_by_name: 'Administrador',
    authorization_mode: 'self',
    status: 'recorded',
    created_at: '2026-11-09T16:00:00Z',
    ...overrides,
  };
}

// Respuesta de error de Go con su código y mensaje.
export function failure(status: number, code: string, message: string) {
  return { status, body: { error: { code, message } } };
}
