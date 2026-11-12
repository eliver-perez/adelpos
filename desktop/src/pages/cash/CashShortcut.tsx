import { useEffect, useId, useRef, useState } from 'react';
import type { KeyboardEvent, ReactNode } from 'react';
import { trapFocus } from '../../components/ConfirmDialog';
import { Notice } from '../../components/Forms';
import { Icon, IconLabel } from '../../components/Icon';
import { shortcutHint } from '../../commands/hints';
import type { IconName } from '../../components/Icon';
import { useCashEvents } from '../../hooks/useCashEvents';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { CashCatalog, RegisterShift, ShiftDetail } from '../../services/cash';
import type { CurrentShift, Shift } from '../../services/payments';
import { ShiftOpening } from '../restaurant/checkout/ShiftOpening';
import { CashOperationForm } from './CashOperationForm';
import { DrawerControl } from './DrawerControl';
import { PartialCutPanel } from './PartialCutPanel';
import { PendingCashOperation } from './PendingCashOperation';
import { useCashRecording } from './recordingFlow';

// Atajo «Caja» (etapa 7.1, decisión 10): un botón con el estado del turno de la caja de la terminal.
// Su menú rápido abre, en una hoja superpuesta, los mismos componentes de la etapa 7
// (CashOperationForm, PartialCutPanel, DrawerControl y ShiftOpening): no hay un segundo flujo de
// caja. Cada opción aparece solo con su permiso y Go vuelve a validar todo al registrar. Desde la
// etapa 9C el gancho y la hoja viven en App.tsx: el botón está en la barra superior de todas las
// pantallas y en la barra reducida de la captura, con una sola consulta del turno.

export type CashSheet = 'income' | 'withdrawal' | 'expense' | 'partial' | 'drawer' | 'opening';
export type ShortcutState = 'open' | 'closing' | 'none' | 'unknown';

export const shortcutStateLabels: Record<ShortcutState, string> = {
  open: 'Turno abierto',
  closing: 'En arqueo',
  none: 'Sin turno',
  unknown: 'Consultando el turno',
};

const sheetTitles: Record<CashSheet, string> = {
  income: 'Ingreso de efectivo',
  withdrawal: 'Retiro de efectivo',
  expense: 'Gasto pagado desde caja',
  partial: 'Corte parcial',
  drawer: 'Abrir cajón',
  opening: 'Abrir turno',
};

// Estado del turno de la caja de la terminal, del menú y de la hoja. Lo crea App.tsx una sola vez
// (etapa 9C): la barra superior, la captura y la hoja comparten la misma consulta. Una pantalla
// operativa montada sola (pruebas) crea el suyo.
export function useCashShortcut({
  active,
  can,
  onOpenCash,
}: {
  active: boolean;
  can: (permission: string) => boolean;
  onOpenCash?: () => void;
}) {
  const visible = can('cash.shifts.read') && can('payments.read');
  const current = useLiveResource<CurrentShift>('payments_current_shift', {
    enabled: active && can('payments.read'),
    interval: 15000,
  });
  // La apertura manual del cajón solo se ofrece si la caja de la terminal tiene el cajón activado.
  const registers = useLiveResource<RegisterShift[]>('payments_cash_registers', {
    enabled: active && visible && can('cash.drawer.open'),
    interval: 30000,
  });
  const [menuOpen, setMenuOpen] = useState(false);
  const [sheet, setSheet] = useState<CashSheet | null>(null);
  const shift = current.data?.shift ?? null;

  function reload() {
    void current.reload();
    void registers.reload();
  }

  function closeSheet() {
    setSheet(null);
  }

  // Flujo tras registrar: la hoja se cierra, el aviso confirma y se recarga el turno. La escritura
  // vive aquí (no en la hoja) para que el aviso y la impresión posterior sobrevivan al cierre.
  const recording = useCashRecording(shift?.id ?? '', can, () => {
    closeSheet();
    reload();
  });
  useCashEvents(reload, active);

  let state: ShortcutState = 'unknown';
  if (current.data) {
    state = 'none';
    if (shift) {
      state = shift.state === 'closing' ? 'closing' : 'open';
    }
  }
  const own = Array.isArray(registers.data)
    ? (registers.data.find((entry) => entry.own) ?? null)
    : null;
  return {
    visible,
    current: current.data,
    shift,
    state,
    drawerEnabled: !!own?.drawer_enabled,
    menuOpen,
    setMenuOpen,
    sheet,
    openSheet: (next: CashSheet) => {
      setMenuOpen(false);
      setSheet(next);
    },
    closeSheet,
    reload,
    recording,
    onOpenCash,
  };
}

export type CashShortcutState = ReturnType<typeof useCashShortcut>;

interface ShortcutOption {
  id: CashSheet | 'cash';
  label: string;
  icon: IconName;
  disabled: boolean;
}

// Opciones del menú rápido según permisos y estado del turno. Ingreso, retiro, gasto y corte
// parcial exigen un turno abierto (Go los rechaza durante el arqueo); el cajón depende de la caja;
// «Abrir turno» aparece sin turno; «Ir a Caja» siempre.
export function shortcutOptions(
  shortcut: Pick<CashShortcutState, 'state' | 'current' | 'drawerEnabled'>,
  can: (permission: string) => boolean,
): ShortcutOption[] {
  const withoutOpenShift = shortcut.state !== 'open';
  const candidates: (ShortcutOption & { allowed: boolean })[] = [
    {
      id: 'income',
      label: 'Ingreso',
      icon: 'income',
      disabled: withoutOpenShift,
      allowed: can('cash.income.record'),
    },
    {
      id: 'withdrawal',
      label: 'Retiro',
      icon: 'withdrawal',
      disabled: withoutOpenShift,
      allowed: can('cash.withdrawals.record'),
    },
    {
      id: 'expense',
      label: 'Gasto',
      icon: 'expense',
      disabled: withoutOpenShift,
      allowed: can('cash.expenses.record'),
    },
    {
      id: 'drawer',
      label: 'Abrir cajón',
      icon: 'drawer',
      disabled: false,
      allowed: can('cash.drawer.open') && shortcut.drawerEnabled,
    },
    {
      id: 'partial',
      label: 'Corte parcial',
      icon: 'report',
      disabled: withoutOpenShift,
      allowed: can('cash.partial_cuts.create'),
    },
    {
      id: 'opening',
      label: 'Abrir turno',
      icon: 'register',
      disabled: false,
      allowed: can('cash.shifts.open') && shortcut.state === 'none' && !!shortcut.current?.register,
    },
    {
      id: 'cash',
      label: 'Ir a Caja',
      icon: 'arrow',
      disabled: false,
      allowed: true,
    },
  ];
  return candidates
    .filter((candidate) => candidate.allowed)
    .map(({ id, label, icon, disabled }) => ({ id, label, icon, disabled }));
}

// Explicación breve del estado en el menú, sin importes.
function shortcutContext(shortcut: CashShortcutState) {
  const register = shortcut.current?.register;
  if (!shortcut.current) {
    return 'Consultando el turno de la caja…';
  }
  if (!register) {
    return 'Esta terminal no tiene una caja física asignada.';
  }
  if (shortcut.state === 'closing') {
    return `${register.name} está en arqueo de cierre: las operaciones de efectivo esperan a que termine.`;
  }
  if (shortcut.state === 'none') {
    return `No hay un turno abierto en ${register.name}.`;
  }
  return `Turno abierto en ${register.name}.`;
}

// Botón «Caja» con el estado del turno y su menú rápido. El menú se cierra con Escape o al tocar
// fuera de él. Desde la etapa 9C vive en la barra superior (placement «bar»: sección de 52–56 px con
// el indicador verde, ámbar o gris del turno) y en la barra reducida de la captura.
export function CashShortcut({
  shortcut,
  can,
  placement = 'inline',
  active = false,
}: {
  shortcut: CashShortcutState;
  can: (permission: string) => boolean;
  placement?: 'inline' | 'bar';
  // Resalta la sección cuando la pantalla visible es Caja.
  active?: boolean;
}) {
  const menuID = useId();
  const root = useRef<HTMLDivElement>(null);
  const { menuOpen, setMenuOpen } = shortcut;

  useEffect(() => {
    if (!menuOpen) {
      return;
    }
    const outside = (event: PointerEvent) => {
      if (root.current && !root.current.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    };
    const escape = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') {
        setMenuOpen(false);
      }
    };
    document.addEventListener('pointerdown', outside);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('keydown', escape);
    };
  }, [menuOpen, setMenuOpen]);

  if (!shortcut.visible) {
    return null;
  }
  const status = shortcutStateLabels[shortcut.state];
  const options = shortcutOptions(shortcut, can);
  // Etapa 9C: el atajo de «Ir a Caja» en la descripción del botón (el botón abre el menú rápido).
  const goHint = shortcutHint('view.cash');

  function choose(option: ShortcutOption) {
    if (option.id === 'cash') {
      setMenuOpen(false);
      shortcut.onOpenCash?.();
      return;
    }
    shortcut.openSheet(option.id);
  }

  const buttonClass =
    placement === 'bar'
      ? `topbar-button topbar-section cash-shortcut-button cash-shortcut-${shortcut.state} ${
          active ? 'topbar-section-active' : ''
        }`
      : `secondary-button cash-shortcut-button cash-shortcut-${shortcut.state}`;
  return (
    <div className={`cash-shortcut cash-shortcut-${placement}`} ref={root}>
      <button
        type="button"
        className={buttonClass}
        aria-label={`Caja · ${status}`}
        title={goHint ? `Caja · ${status} · Ir a Caja: ${goHint.label}` : undefined}
        aria-expanded={menuOpen}
        aria-controls={menuID}
        aria-current={active ? 'page' : undefined}
        data-shift-state={shortcut.state}
        onClick={() => setMenuOpen(!menuOpen)}
      >
        <Icon name="cash" size={20} />
        <span className="cash-shortcut-name">Caja</span>
        {/* Indicador del turno (§1.1): verde abierto, ámbar en arqueo y gris sin turno. */}
        <span className="cash-shortcut-indicator" aria-hidden="true" />
        {shortcut.state !== 'unknown' && <span className="cash-shortcut-state">{status}</span>}
      </button>
      {menuOpen && (
        <div
          id={menuID}
          className="cash-shortcut-menu"
          role="group"
          aria-label="Menú rápido de caja"
        >
          <p className="muted text-xs">{shortcutContext(shortcut)}</p>
          {options.map((option) => (
            <button
              type="button"
              key={option.id}
              className="secondary-button"
              disabled={option.disabled}
              onClick={() => choose(option)}
            >
              <IconLabel icon={option.icon}>{option.label}</IconLabel>
            </button>
          ))}
          <PendingCashOperation mutation={shortcut.recording.printer} />
        </div>
      )}
    </div>
  );
}

// Aviso discreto de la captura sin turno (etapa 7.1): las cuentas son del restaurante, no de la
// caja, así que se puede capturar; cobrar exige un turno abierto. Con permiso, ofrece abrirlo.
export function NoShiftNotice({
  shortcut,
  can,
}: {
  shortcut: CashShortcutState;
  can: (permission: string) => boolean;
}) {
  if (shortcut.state !== 'none') {
    return null;
  }
  const canOpen = can('cash.shifts.open') && !!shortcut.current?.register;
  return (
    <div className="capture-shift-notice" role="status" aria-label="Turno de caja">
      <Icon name="info" size={18} />
      <span>No hay turno abierto: puedes capturar, pero no cobrar.</span>
      {canOpen && (
        <button
          type="button"
          className="secondary-button"
          onClick={() => shortcut.openSheet('opening')}
        >
          <IconLabel icon="register">Abrir turno</IconLabel>
        </button>
      )}
    </div>
  );
}

// Marco de la hoja superpuesta: diálogo modal con el foco atrapado, Escape para cerrar y el foco
// devuelto al control anterior al cerrarse.
function SheetFrame({
  title,
  header,
  onClose,
  children,
}: {
  title: string;
  // Algunos componentes (formulario de operación, corte parcial) traen su propio encabezado con
  // el botón de cierre; los demás usan el de la hoja.
  header: boolean;
  onClose: () => void;
  children: ReactNode;
}) {
  const panel = useRef<HTMLElement>(null);

  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    panel.current?.focus();
    return () => {
      if (previous && previous.isConnected) {
        previous.focus();
      }
    };
  }, []);

  function handleKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      onClose();
      return;
    }
    if (event.key === 'Tab' && panel.current) {
      trapFocus(event, panel.current);
    }
  }

  return (
    <div className="cash-sheet-backdrop">
      <section
        ref={panel}
        className="cash-sheet"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
      >
        {header && (
          <header className="cash-panel-header cash-sheet-header">
            <h3>{title}</h3>
            <button type="button" className="secondary-button" onClick={onClose}>
              <IconLabel icon="close">Cerrar</IconLabel>
            </button>
          </header>
        )}
        <div className="cash-sheet-body">{children}</div>
      </section>
    </div>
  );
}

// Contenido de una hoja que necesita el turno abierto: el catálogo de caja y, para el corte
// parcial, los cortes del turno.
function ShiftSheet({
  sheet,
  shift,
  shortcut,
  can,
  timezone,
}: {
  sheet: 'income' | 'withdrawal' | 'expense' | 'partial';
  shift: Shift;
  shortcut: CashShortcutState;
  can: (permission: string) => boolean;
  timezone: string;
}) {
  const catalog = useLiveResource<CashCatalog>('payments_cash_catalog', { interval: 60000 });
  const detail = useLiveResource<ShiftDetail>('payments_shift', {
    id: shift.id,
    enabled: sheet === 'partial',
    interval: 15000,
  });
  const { mutation } = shortcut.recording;
  if (!catalog.data) {
    return (
      <SheetFrame title={sheetTitles[sheet]} header onClose={shortcut.closeSheet}>
        <Notice>{catalog.error || 'Consultando el catálogo de caja…'}</Notice>
      </SheetFrame>
    );
  }
  if (sheet === 'partial') {
    return (
      <SheetFrame title={sheetTitles[sheet]} header={false} onClose={shortcut.closeSheet}>
        <PendingCashOperation mutation={mutation} />
        <PartialCutPanel
          shift={shift}
          reports={detail.data?.reports ?? []}
          can={can}
          mutation={mutation}
          timezone={timezone}
          onWithdraw={() => shortcut.openSheet('withdrawal')}
          onClose={shortcut.closeSheet}
          backLabel="Cerrar"
        />
      </SheetFrame>
    );
  }
  return (
    <SheetFrame title={sheetTitles[sheet]} header={false} onClose={shortcut.closeSheet}>
      <PendingCashOperation mutation={mutation} />
      <CashOperationForm
        key={sheet}
        kind={sheet}
        shift={shift}
        catalog={catalog.data}
        can={can}
        dollars={!!shortcut.current?.usd_cash && !!shift.usd_rate}
        active
        mutation={mutation}
        onClose={shortcut.closeSheet}
        backLabel="Cerrar"
      />
    </SheetFrame>
  );
}

// Hoja superpuesta del atajo con el componente de la etapa 7 que corresponde a la opción elegida.
export function CashShortcutSheet({
  shortcut,
  can,
  timezone,
}: {
  shortcut: CashShortcutState;
  can: (permission: string) => boolean;
  timezone: string;
}) {
  const { sheet, shift, current } = shortcut;
  if (!sheet) {
    return null;
  }
  const register = current?.register ?? null;
  if (sheet === 'opening') {
    return (
      <SheetFrame title={sheetTitles[sheet]} header onClose={shortcut.closeSheet}>
        {current && !shift ? (
          <ShiftOpening
            current={current}
            can={can}
            active
            onChanged={shortcut.reload}
            onOpened={shortcut.closeSheet}
          />
        ) : (
          <Notice>La caja de esta terminal ya tiene un turno.</Notice>
        )}
      </SheetFrame>
    );
  }
  if (sheet === 'drawer') {
    return (
      <SheetFrame title={sheetTitles[sheet]} header onClose={shortcut.closeSheet}>
        {register ? (
          <DrawerControl
            registerID={register.id}
            registerName={register.name}
            initiallyOpen
            onSent={shortcut.closeSheet}
          />
        ) : (
          <Notice>Esta terminal no tiene una caja física asignada.</Notice>
        )}
      </SheetFrame>
    );
  }
  if (!shift || shift.state !== 'open') {
    return (
      <SheetFrame title={sheetTitles[sheet]} header onClose={shortcut.closeSheet}>
        <Notice>
          La caja de esta terminal no tiene un turno abierto: las operaciones de efectivo se
          registran con el turno abierto.
        </Notice>
      </SheetFrame>
    );
  }
  return (
    <ShiftSheet
      key={shift.id}
      sheet={sheet}
      shift={shift}
      shortcut={shortcut}
      can={can}
      timezone={timezone}
    />
  );
}
