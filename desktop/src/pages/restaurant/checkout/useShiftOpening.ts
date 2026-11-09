import { useState } from 'react';
import { useCountPreview } from '../../../hooks/usePaymentPreview';
import { useResource } from '../../../hooks/useResource';
import { useRestaurantMutation } from '../../../hooks/useRestaurantMutation';
import { APIError } from '../../../services/api';
import { toWireAmount } from '../../../services/amountInput';
import type { CashCatalog, CountPreviewInput } from '../../../services/cash';
import { formatDollars, formatMoney, isZeroMoney } from '../../../services/payments';
import type { CurrentShift } from '../../../services/payments';
import { acceptQuantity, capturableDenominations, countLines } from '../../cash/countCapture';
import type { CountQuantities } from '../../cash/countCapture';
import { confirmationCode, shiftPendingKey } from './ShiftRateCapture';

export type OpeningMode = 'total' | 'denominations';

// Campo que recibe el teclado numérico: fondo en pesos, fondo en dólares, tipo de cambio o la
// cantidad de una denominación («count:<id>»).
type OpeningTarget = 'opening' | 'opening_usd' | 'rate' | `count:${string}`;

// Estado de la apertura ampliada (etapa 7 §5), compartido por el cobro y la pantalla Caja. El
// fondo se captura por total o por denominaciones; en el segundo caso Go calcula el total con la
// vista previa del conteo y la apertura envía exactamente esos importes con su desglose.
export function useShiftOpening({
  current,
  can,
  onChanged,
}: {
  current: CurrentShift;
  can: (permission: string) => boolean;
  onChanged: () => void;
}) {
  const [mode, setMode] = useState<OpeningMode>('total');
  const [opening, setOpening] = useState('');
  const [openingUSD, setOpeningUSD] = useState('');
  const [rate, setRate] = useState('');
  const [notes, setNotes] = useState('');
  const [quantities, setQuantities] = useState<CountQuantities>({});
  const [target, setTarget] = useState<OpeningTarget>('opening');
  const [confirmation, setConfirmation] = useState('');
  const mutation = useRestaurantMutation(() => {
    setOpening('');
    setOpeningUSD('');
    setRate('');
    setNotes('');
    setQuantities({});
    setTarget('opening');
    onChanged();
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  }, shiftPendingKey);
  const register = current.register;
  const openable = !!register && !current.shift && can('cash.shifts.open');
  // Sin la función activa, la apertura no muestra dólares (queda como en la etapa 6).
  const dollars = !!current.usd_cash;
  const canSetRate = dollars && can('payments.usd_rate.manage');
  const counting = mode === 'denominations';
  const catalog = useResource<CashCatalog>('payments_cash_catalog', {
    enabled: openable && counting,
  });
  const denominations = capturableDenominations(catalog.data?.denominations ?? [], dollars);
  const lines = counting ? countLines(quantities, denominations) : [];
  const previewInput: CountPreviewInput | null = lines.length > 0 ? { lines } : null;
  const preview = useCountPreview('opening', previewInput);
  const plan = preview.plan;

  // Importes que se envían a Go: los capturados (por total) o los que Go totalizó (desglose).
  let floatWire = toWireAmount(opening);
  let usdWire = dollars ? toWireAmount(openingUSD) : '';
  if (counting) {
    floatWire = plan?.counted_mxn ?? '';
    usdWire = plan && !isZeroMoney(plan.counted_usd) ? plan.counted_usd : '';
  }
  const rateWire = canSetRate ? toWireAmount(rate) : '';
  const ready = floatWire !== '' && (!counting || (!!plan && !preview.validating));

  function openLabel() {
    if (mutation.busy) {
      return 'Abriendo turno…';
    }
    if (counting && preview.validating) {
      return 'Calculando el fondo…';
    }
    if (floatWire === '') {
      return counting ? 'Captura las cantidades contadas' : 'Captura el fondo inicial';
    }
    let label = `Abrir turno con fondo de ${formatMoney(floatWire)}`;
    if (usdWire !== '') {
      label += ` + ${formatDollars(usdWire)}`;
    }
    if (rateWire !== '') {
      label += ` y dólar a ${formatMoney(rateWire)}`;
    }
    return label;
  }

  function openingBody(confirmed: boolean) {
    const body: Record<string, unknown> = { opening_float: floatWire };
    if (usdWire !== '') {
      body.opening_float_usd = usdWire;
    }
    if (notes.trim() !== '') {
      body.opening_notes = notes.trim();
    }
    if (counting && previewInput) {
      body.count = { lines: previewInput.lines };
    }
    if (rateWire !== '') {
      body.usd_rate = rateWire;
      if (confirmed) {
        body.confirm_rate_difference = true;
      }
    }
    return body;
  }

  async function open(confirmed = false) {
    if (!ready || mutation.isPending()) {
      return;
    }
    try {
      await mutation.execute('payments_open_shift', { body: openingBody(confirmed) });
    } catch (problem) {
      if (problem instanceof APIError && problem.code === confirmationCode) {
        setConfirmation(problem.message);
        return;
      }
      // Un turno abierto por otra terminal mientras se capturaba el fondo se refleja al consultar.
      if (problem instanceof APIError && problem.status === 409) {
        onChanged();
      }
    }
  }

  function changeMode(next: OpeningMode) {
    setMode(next);
    setTarget(next === 'total' ? 'opening' : `count:${denominations[0]?.id ?? ''}`);
  }

  const countID = target.startsWith('count:') ? target.slice('count:'.length) : '';
  const activeCountID = counting && target !== 'rate' ? countID || denominations[0]?.id || '' : '';

  // El teclado fijo escribe en el campo activo; las cantidades solo admiten dígitos.
  function keypad() {
    if (target === 'rate') {
      return { value: rate, onChange: setRate, label: 'Teclado numérico del tipo de cambio' };
    }
    if (counting) {
      const value = quantities[activeCountID] ?? '';
      return {
        value,
        onChange: (next: string) => changeQuantity(activeCountID, next),
        label: 'Teclado numérico del conteo',
        integer: true,
      };
    }
    if (target === 'opening_usd') {
      return {
        value: openingUSD,
        onChange: setOpeningUSD,
        label: 'Teclado numérico del fondo en dólares',
      };
    }
    return { value: opening, onChange: setOpening, label: 'Teclado numérico del fondo inicial' };
  }

  function changeQuantity(id: string, next: string) {
    if (!id) {
      return;
    }
    setQuantities((previous) => ({
      ...previous,
      [id]: acceptQuantity(next, previous[id] ?? ''),
    }));
  }

  return {
    mutation,
    register,
    openable,
    dollars,
    canSetRate,
    mode,
    changeMode,
    opening,
    setOpening,
    openingUSD,
    setOpeningUSD,
    rate,
    setRate,
    notes,
    setNotes,
    quantities,
    changeQuantity,
    target,
    setTarget,
    activeCountID,
    catalog,
    denominations,
    preview,
    ready,
    openLabel,
    open,
    confirmation,
    setConfirmation,
    keypad,
  };
}

export type ShiftOpeningState = ReturnType<typeof useShiftOpening>;
