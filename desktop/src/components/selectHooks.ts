import { useEffect, useLayoutEffect, useState } from 'react';
import type { CSSProperties, RefObject } from 'react';

// Ganchos del selector único (components/Select.tsx, etapa 9C): nombre accesible tomado de la
// etiqueta, posición de la lista emergente y cierre al tocar o enfocar fuera de ella.

// Último tipo de puntero usado en la página: al abrir con el dedo la búsqueda no se enfoca sola
// (el teclado virtual taparía media pantalla para elegir entre las opciones a la vista).
let lastPointerType = 'mouse';
let pointerTrackerInstalled = false;

export function trackPointerType() {
  if (pointerTrackerInstalled || typeof window === 'undefined') {
    return;
  }
  pointerTrackerInstalled = true;
  window.addEventListener(
    'pointerdown',
    (event) => {
      lastPointerType = event.pointerType || 'mouse';
    },
    { capture: true, passive: true },
  );
}

export function openedByTouch() {
  return lastPointerType === 'touch' || lastPointerType === 'pen';
}

// Elementos que no cierran la lista al tocarlos: el propio teclado virtual y su botón «Teclado».
function belongsToVirtualKeyboard(target: EventTarget | null) {
  return (
    target instanceof Element && target.closest('.virtual-keyboard, .keyboard-reopen') !== null
  );
}

// Alto que ocupa el teclado virtual (components/VirtualKeyboard.tsx lo publica en esta variable).
function virtualKeyboardHeight() {
  const raw = getComputedStyle(document.documentElement).getPropertyValue(
    '--virtual-keyboard-height',
  );
  const parsed = Number.parseFloat(raw);
  return Number.isFinite(parsed) ? parsed : 0;
}

const popupGutter = 12;
const popupGap = 4;
const popupMinWidth = 240;
const popupMaxHeight = 440;
// Espacio mínimo para abrir hacia abajo antes de probar hacia arriba.
const popupComfortHeight = 260;
// Alto mínimo de la lista aunque falte espacio por ambos lados.
const popupMinHeight = 120;

// Posición fija de la lista junto al disparador: al menos tan ancha como él (y 240 px), sin
// salirse de la ventana por ningún lado y sin quedar debajo del teclado virtual. Abre hacia abajo
// si caben 260 px o si hay más espacio abajo que arriba.
export function popupPosition(trigger: HTMLElement): CSSProperties {
  const rect = trigger.getBoundingClientRect();
  const viewportWidth = document.documentElement.clientWidth || window.innerWidth;
  const viewportHeight = window.innerHeight;
  const bottomLimit = viewportHeight - virtualKeyboardHeight() - popupGutter;
  const available = Math.max(0, viewportWidth - popupGutter * 2);
  const width = Math.min(available, Math.max(rect.width, popupMinWidth));
  const left = Math.min(Math.max(rect.left, popupGutter), viewportWidth - popupGutter - width);
  const below = bottomLimit - rect.bottom - popupGap;
  const above = rect.top - popupGutter - popupGap;
  const placeBelow = below >= popupComfortHeight || below >= above;
  const space = Math.max(placeBelow ? below : above, popupMinHeight);
  const style: CSSProperties = {
    left: Math.max(popupGutter, left),
    width,
    maxHeight: Math.min(popupMaxHeight, space),
  };
  if (placeBelow) {
    style.top = rect.bottom + popupGap;
  } else {
    style.bottom = viewportHeight - rect.top + popupGap;
  }
  return style;
}

// Recalcula la posición al abrir, al desplazar cualquier contenedor, al cambiar el tamaño de la
// ventana y al aparecer o desaparecer el teclado virtual (cambia el estilo de <html>). Cerrada
// devuelve null; mientras es null la lista se dibuja oculta.
export function usePopupPosition(
  open: boolean,
  trigger: RefObject<HTMLElement | null>,
  popup: RefObject<HTMLElement | null>,
) {
  const [position, setPosition] = useState<CSSProperties | null>(null);
  useLayoutEffect(() => {
    const button = trigger.current;
    if (!open || !button) {
      setPosition(null);
      return;
    }
    setPosition(popupPosition(button));
    let frame = 0;
    const schedule = (event?: Event) => {
      const fromList =
        event !== undefined &&
        event.target instanceof Node &&
        (popup.current?.contains(event.target) ?? false);
      if (fromList) {
        return;
      }
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => setPosition(popupPosition(button)));
    };
    window.addEventListener('resize', schedule);
    document.addEventListener('scroll', schedule, { capture: true, passive: true });
    const observer = new MutationObserver(() => schedule());
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['style'] });
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener('resize', schedule);
      document.removeEventListener('scroll', schedule, { capture: true });
      observer.disconnect();
    };
  }, [open, trigger, popup]);
  return position;
}

// Cierra la lista al tocar o enfocar fuera del selector y de su lista; tocar el teclado virtual
// no la cierra (se escribe en la búsqueda con él).
export function useDismissOutside(
  open: boolean,
  close: () => void,
  areas: RefObject<HTMLElement | null>[],
) {
  useEffect(() => {
    if (!open) {
      return;
    }
    const inside = (target: EventTarget | null) =>
      target instanceof Node &&
      (areas.some((area) => area.current?.contains(target) ?? false) ||
        belongsToVirtualKeyboard(target));
    const pointer = (event: PointerEvent) => {
      if (!inside(event.target)) {
        close();
      }
    };
    const focus = (event: FocusEvent) => {
      if (!inside(event.target)) {
        close();
      }
    };
    document.addEventListener('pointerdown', pointer, true);
    document.addEventListener('focusin', focus);
    return () => {
      document.removeEventListener('pointerdown', pointer, true);
      document.removeEventListener('focusin', focus);
    };
  });
}

export interface FieldLabel {
  labelID: string | undefined;
  hintID: string | undefined;
  labelText: string;
}

// El nombre accesible del disparador es el texto de su etiqueta (el <span> del <label> de
// Forms.Field o el <label for> que lo nombra), sin el valor elegido ni la indicación; la indicación
// (<small>) queda como descripción. Se revisa en cada render porque la indicación puede aparecer o
// cambiar (por ejemplo, un error al cargar). Si el elemento no tiene id se le asigna uno propio.
export function useFieldLabel(
  trigger: RefObject<HTMLElement | null>,
  root: RefObject<HTMLElement | null>,
  baseID: string,
  ariaLabel: string | undefined,
  ariaLabelledBy: string | undefined,
): FieldLabel {
  const [label, setLabel] = useState<FieldLabel>({
    labelID: undefined,
    hintID: undefined,
    labelText: ariaLabel ?? '',
  });
  useLayoutEffect(() => {
    const button = trigger.current;
    if (!button) {
      return;
    }
    const next: FieldLabel = { labelID: undefined, hintID: undefined, labelText: ariaLabel ?? '' };
    const element =
      (button instanceof HTMLButtonElement ? button.labels?.[0] : undefined) ??
      button.closest('label');
    if (element && !ariaLabel && !ariaLabelledBy) {
      const children = Array.from(element.children).filter(
        (child) => !child.contains(button) && child !== root.current,
      );
      const text = children.find((child) => child.tagName === 'SPAN');
      const hint = children.find((child) => child.tagName === 'SMALL');
      const named = text ?? (element.contains(button) ? null : element);
      if (named) {
        if (!named.id) {
          named.id = `${baseID}-label`;
        }
        next.labelID = named.id;
        next.labelText = named.textContent ?? '';
      }
      if (hint) {
        if (!hint.id) {
          hint.id = `${baseID}-hint`;
        }
        next.hintID = hint.id;
      }
    }
    const changed =
      next.labelID !== label.labelID ||
      next.hintID !== label.hintID ||
      next.labelText !== label.labelText;
    if (changed) {
      setLabel(next);
    }
  });
  return label;
}
