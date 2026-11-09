import { useState } from 'react';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { CashCatalog, RegisterShift, ShiftDetail } from '../../services/cash';
import type { CurrentShift } from '../../services/payments';

// Datos compartidos por las vistas de Caja: cajas visibles con su turno activo (la de la terminal
// y, con cash.shifts.read_all, las demás), el turno de la terminal, el catálogo de caja y el
// detalle del turno de la caja seleccionada. Todo se consulta con sondeo y se invalida con los
// eventos de la aplicación (useLiveResource).
export function useCashWorkspace(can: (permission: string) => boolean) {
  const registers = useLiveResource<RegisterShift[]>('payments_cash_registers', {
    interval: 15000,
  });
  const current = useLiveResource<CurrentShift>('payments_current_shift', {
    enabled: can('payments.read'),
    interval: 15000,
  });
  const catalog = useLiveResource<CashCatalog>('payments_cash_catalog', {
    enabled: can('payments.read'),
    interval: 60000,
  });
  const [registerID, setRegisterID] = useState('');
  const list = registers.data ?? [];
  const own = list.find((entry) => entry.own) ?? null;
  const selected = list.find((entry) => entry.register.id === registerID) ?? own ?? list[0] ?? null;
  const shift = selected?.shift ?? null;
  const detail = useLiveResource<ShiftDetail>('payments_shift', {
    id: shift?.id,
    enabled: !!shift,
    interval: 10000,
  });

  return {
    registers,
    current,
    catalog,
    list,
    own,
    selected,
    shift,
    // El detalle solo vale para el turno seleccionado (no se muestra uno anterior).
    detail: detail.data && detail.data.shift.id === shift?.id ? detail.data : null,
    detailError: detail.error,
    selectRegister: setRegisterID,
    // Función de dólares activa según el turno de la terminal.
    usdCash: !!current.data?.usd_cash,
    reload: () => {
      void registers.reload();
      void current.reload();
      void detail.reload();
    },
  };
}

export type CashWorkspace = ReturnType<typeof useCashWorkspace>;
