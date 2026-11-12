import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { KeyboardEvent as ReactKeyboardEvent, MouseEvent as ReactMouseEvent } from 'react';
import { createPortal } from 'react-dom';
import { Icon } from './Icon';
import { countLabel } from '../services/plural';
import { SelectChips, SelectPopup, optionID } from './SelectPopup';
import {
  openedByTouch,
  trackPointerType,
  useDismissOutside,
  useFieldLabel,
  usePopupPosition,
} from './selectHooks';
import {
  edgeIndex,
  entryDisabled,
  entryKey,
  filterItems,
  findOption,
  listKeys,
  navigableEntries,
  navigationIndex,
  openingFor,
  optionCount,
  searchThreshold,
  typeaheadIndex,
} from './selectModel';
import type {
  NavigableEntry,
  OpeningKey,
  SelectAction,
  SelectItem,
  SelectOption,
} from './selectModel';

export type { SelectAction, SelectGroup, SelectItem, SelectOption } from './selectModel';

// Selector único de la aplicación (etapa 9C, fase B; sustituye a todos los <select>). Patrón ARIA
// «combobox» de solo selección con una lista emergente (listbox):
// - el disparador es un <button role="combobox"> que toma su nombre del <label> que lo rodea (o
//   de aria-label) y conserva el valor elegido en su atributo value;
// - la lista se dibuja en un portal con posición fija, para que ningún contenedor con
//   desplazamiento o recorte la corte, y se coloca arriba o abajo según el espacio libre (también
//   el que deja el teclado virtual);
// - con más de 8 opciones aparece una búsqueda sin distinguir mayúsculas ni acentos; con
//   onSearchChange la búsqueda la resuelve Go y el componente solo muestra lo que llega;
// - teclado completo: flechas, Inicio/Fin, Re Pág/Av Pág, Intro, Espacio, Esc, Tab y escribir
//   para buscar; las opciones de acción van al final y siempre se ven;
// - con required (o name) un campo oculto participa en la validación nativa del formulario: el
//   acordeón que lo contiene se abre y el foco vuelve al disparador.

interface SelectBaseProps<T extends string> {
  options: readonly SelectItem<T>[];
  // Texto del disparador cuando no hay nada elegido (el valor no coincide con ninguna opción).
  placeholder?: string;
  disabled?: boolean;
  required?: boolean;
  name?: string;
  id?: string;
  className?: string;
  'aria-label'?: string;
  'aria-labelledby'?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
  // Por omisión la búsqueda aparece con más de 8 opciones o si se indica onSearchChange.
  searchable?: boolean;
  searchPlaceholder?: string;
  // Nombre accesible del campo de búsqueda; por omisión «Buscar <etiqueta>».
  searchLabel?: string;
  // Búsqueda en el servidor: el componente informa el texto y no filtra las opciones.
  onSearchChange?: (query: string) => void;
  // Las opciones de una búsqueda en el servidor todavía no llegan (Intro no elige nada).
  loading?: boolean;
  actions?: readonly SelectAction[];
  // Texto cuando una búsqueda no encuentra nada.
  emptyMessage?: string;
}

export interface SelectProps<T extends string> extends SelectBaseProps<T> {
  value: string | null | undefined;
  onChange: (value: T) => void;
}

export interface MultiSelectProps<T extends string> extends SelectBaseProps<T> {
  value: readonly string[];
  onChange: (value: T[]) => void;
  // Etiquetas con «Quitar» debajo del disparador para ver y retirar lo elegido (por omisión, sí).
  chips?: boolean;
}

export function Select<T extends string>(props: SelectProps<T>) {
  const { value, onChange, ...rest } = props;
  // El valor vacío también se busca entre las opciones: «Todos» o «Sin asignar» suelen ser la
  // opción con valor ''. Si ninguna lo tiene, el disparador muestra el texto de «sin elegir».
  const selected = value === null || value === undefined ? [] : [value];
  return (
    <SelectControl<T>
      {...rest}
      multiple={false}
      selectedValues={selected}
      singleValue={value ?? ''}
      onPickValues={(values) => {
        const next = values[0];
        if (next !== undefined) {
          onChange(next);
        }
      }}
    />
  );
}

export function MultiSelect<T extends string>(props: MultiSelectProps<T>) {
  const { value, onChange, chips = true, ...rest } = props;
  return (
    <SelectControl<T>
      {...rest}
      multiple
      chips={chips}
      selectedValues={value}
      singleValue=""
      onPickValues={onChange}
    />
  );
}

interface ControlProps<T extends string> extends SelectBaseProps<T> {
  multiple: boolean;
  chips?: boolean;
  selectedValues: readonly string[];
  singleValue: string;
  onPickValues: (values: T[]) => void;
}

type OpenSource = 'keyboard' | 'mouse' | 'touch';

const noActions: readonly SelectAction[] = [];

function lowerFirst(text: string) {
  return text.charAt(0).toLocaleLowerCase('es') + text.slice(1);
}

function isPrintableKey(event: ReactKeyboardEvent) {
  return event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey;
}

// Texto del disparador: la opción elegida o, en la múltiple, cuántas («3 seleccionados»).
function triggerText<T extends string>(
  selected: SelectOption<T>[],
  multiple: boolean,
  placeholder: string | undefined,
) {
  if (!multiple) {
    return selected[0]?.label ?? placeholder ?? 'Selecciona…';
  }
  if (selected.length === 0) {
    return placeholder ?? 'Ninguno';
  }
  if (selected.length === 1) {
    return selected[0]!.label;
  }
  return countLabel(selected.length, 'seleccionado', 'seleccionados');
}

function SelectControl<T extends string>({
  options,
  placeholder,
  disabled = false,
  required = false,
  name,
  id,
  className = '',
  'aria-label': ariaLabel,
  'aria-labelledby': ariaLabelledBy,
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
  searchable,
  searchPlaceholder = 'Escribe para buscar',
  searchLabel,
  onSearchChange,
  loading = false,
  actions = noActions,
  emptyMessage = 'Sin resultados',
  multiple,
  chips = false,
  selectedValues,
  singleValue,
  onPickValues,
}: ControlProps<T>) {
  const baseID = useId();
  const listID = `${baseID}-listbox`;
  const root = useRef<HTMLSpanElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const focusSearchOnOpen = useRef(false);
  const typeahead = useRef({ text: '', time: 0 });
  const remote = onSearchChange !== undefined;
  const withSearch = searchable ?? (remote || optionCount(options) > searchThreshold);
  const label = useFieldLabel(trigger, root, baseID, ariaLabel, ariaLabelledBy);
  const position = usePopupPosition(open, trigger, popup);

  // Lo que se ve y se recorre: sin abrir no se calcula nada (hay pantallas con ≈150 selectores).
  const visibleItems = useMemo(() => {
    if (!open || remote) {
      return options;
    }
    return filterItems(options, query);
  }, [open, remote, options, query]);
  const entries = useMemo(
    () => (open ? navigableEntries(visibleItems, actions) : []),
    [open, visibleItems, actions],
  );
  const activeIndex = useMemo(() => {
    const found =
      activeKey === null ? -1 : entries.findIndex((entry) => entryKey(entry) === activeKey);
    if (found >= 0 && !entryDisabled(entries[found]!)) {
      return found;
    }
    return edgeIndex(entries, 'first');
  }, [entries, activeKey]);
  const activeID = open && activeIndex >= 0 ? optionID(baseID, activeIndex) : undefined;
  const selectedSet = useMemo(() => new Set(selectedValues), [selectedValues]);
  const selectedOptions = useMemo(
    () =>
      selectedValues
        .map((value) => findOption(options, value))
        .filter((option): option is SelectOption<T> => option !== undefined),
    [options, selectedValues],
  );

  // Elemento con el que abre la lista: el elegido (el primero en la múltiple), el primero o el
  // último habilitado.
  function openingKey(opening: OpeningKey) {
    const all = navigableEntries(options, actions);
    if (opening === 'selected') {
      const chosen = all.find(
        (entry) =>
          entry.kind === 'option' && selectedSet.has(entry.option.value) && !entry.option.disabled,
      );
      return chosen ? entryKey(chosen) : null;
    }
    const index = edgeIndex(all, opening);
    return index >= 0 ? entryKey(all[index]!) : null;
  }

  function openList(source: OpenSource, opening: OpeningKey = 'selected') {
    if (disabled || trigger.current?.matches(':disabled')) {
      return;
    }
    trackPointerType();
    focusSearchOnOpen.current = withSearch && source !== 'touch';
    if (!focusSearchOnOpen.current) {
      // WebKit (Safari, WKWebView) no enfoca un botón al tocarlo: sin el foco en el disparador,
      // las flechas, Intro y Esc no llegarían a la lista.
      trigger.current?.focus({ preventScroll: true });
    }
    setQuery('');
    setActiveKey(openingKey(opening));
    setOpen(true);
  }

  function closeList(returnFocus: boolean) {
    setOpen(false);
    if (query !== '') {
      setQuery('');
      onSearchChange?.('');
    }
    if (returnFocus) {
      trigger.current?.focus({ preventScroll: true });
    }
  }

  function changeQuery(next: string) {
    setQuery(next);
    setActiveKey(null);
    onSearchChange?.(next);
  }

  function pick(entry: NavigableEntry<T> | undefined) {
    if (!entry || entryDisabled(entry)) {
      return;
    }
    if (entry.kind === 'action') {
      // El foco vuelve al disparador antes de la acción: un diálogo que se abra lo devuelve ahí.
      closeList(true);
      entry.action.onSelect();
      return;
    }
    const value = entry.option.value;
    if (multiple) {
      setActiveKey(entryKey(entry));
      const next = selectedSet.has(value)
        ? selectedValues.filter((current) => current !== value)
        : [...selectedValues, value];
      onPickValues(next as T[]);
      return;
    }
    closeList(true);
    // Como una lista nativa: elegir otra vez la misma opción no avisa un cambio.
    if (value !== singleValue) {
      onPickValues([value]);
    }
  }

  function pageSize() {
    const list = popup.current?.querySelector<HTMLElement>('[role="listbox"]');
    if (!list || list.clientHeight === 0) {
      return 8;
    }
    return Math.max(1, Math.floor(list.clientHeight / 48));
  }

  // Sin búsqueda, escribir lleva a la opción que empieza con lo escrito (como una lista nativa).
  function typeaheadKey(letter: string, all: readonly NavigableEntry<T>[], current: number) {
    const now = Date.now();
    const previous = typeahead.current;
    const text = now - previous.time < 700 ? previous.text + letter : letter;
    typeahead.current = { text, time: now };
    const found = typeaheadIndex(all, current, text);
    return found >= 0 ? entryKey(all[found]!) : null;
  }

  function handleClosedKey(event: ReactKeyboardEvent<HTMLButtonElement>) {
    const opening = event.altKey && event.key === 'ArrowDown' ? 'selected' : openingFor(event.key);
    if (opening) {
      event.preventDefault();
      openList('keyboard', opening);
      return;
    }
    if (!isPrintableKey(event)) {
      return;
    }
    // Escribir sobre el disparador abre la lista; con búsqueda, lo escrito va a la búsqueda.
    event.preventDefault();
    if (withSearch) {
      openList('keyboard');
      changeQuery(event.key);
      return;
    }
    const all = navigableEntries(options, actions);
    const current = all.findIndex(
      (entry) => entry.kind === 'option' && selectedSet.has(entry.option.value),
    );
    const found = typeaheadKey(event.key, all, current);
    openList('keyboard');
    if (found) {
      setActiveKey(found);
    }
  }

  function handleOpenKey(event: ReactKeyboardEvent<HTMLElement>, source: 'trigger' | 'search') {
    if (listKeys.includes(event.key)) {
      event.stopPropagation();
    }
    if (event.key === 'ArrowUp' && event.altKey) {
      event.preventDefault();
      closeList(true);
      return;
    }
    const target = navigationIndex(event.key, entries, activeIndex, pageSize());
    if (target !== null) {
      event.preventDefault();
      const entry = entries[target];
      if (entry) {
        setActiveKey(entryKey(entry));
      }
      return;
    }
    switch (event.key) {
      case 'Enter':
        event.preventDefault();
        if (!(remote && loading)) {
          pick(entries[activeIndex]);
        }
        return;
      case 'Escape':
        event.preventDefault();
        closeList(true);
        return;
      case 'Tab':
        // Tab cierra la lista sin cambiar el valor y deja que el navegador avance desde el
        // disparador (por eso el foco vuelve a él antes de la acción predeterminada).
        closeList(source === 'search');
        return;
      default:
        break;
    }
    if (source === 'search' || !isPrintableKey(event)) {
      return;
    }
    event.preventDefault();
    if (event.key === ' ') {
      pick(entries[activeIndex]);
      return;
    }
    if (withSearch) {
      // Abierta con el dedo, una tecla física lleva a la búsqueda con lo escrito.
      changeQuery(query + event.key);
      search.current?.focus({ preventScroll: true });
      return;
    }
    const found = typeaheadKey(event.key, entries, activeIndex);
    if (found) {
      setActiveKey(found);
    }
  }

  function handleTriggerClick(event: ReactMouseEvent<HTMLButtonElement>) {
    if (open) {
      closeList(true);
      return;
    }
    let source: OpenSource = 'mouse';
    if (event.detail === 0) {
      source = 'keyboard';
    } else if (openedByTouch()) {
      source = 'touch';
    }
    openList(source);
  }

  useDismissOutside(open, () => closeList(false), [root, popup]);

  useEffect(() => {
    if (open && disabled) {
      closeList(false);
    }
  });

  // La búsqueda se enfoca cuando la lista ya es visible (un elemento oculto no recibe el foco),
  // con el cursor al final de lo que se haya escrito en el disparador.
  useLayoutEffect(() => {
    const input = search.current;
    if (!open || position === null || !focusSearchOnOpen.current || !input) {
      return;
    }
    focusSearchOnOpen.current = false;
    input.focus({ preventScroll: true });
    input.setSelectionRange(input.value.length, input.value.length);
  }, [open, position]);

  useEffect(() => {
    if (activeID) {
      document.getElementById(activeID)?.scrollIntoView?.({ block: 'nearest' });
    }
  }, [activeID]);

  const describedBy = [ariaDescribedBy, label.hintID].filter(Boolean).join(' ') || undefined;
  const fieldValue = multiple ? selectedValues.join(',') : singleValue;
  const labelledBy = ariaLabelledBy ?? label.labelID;
  const searchName =
    searchLabel ?? (label.labelText ? `Buscar ${lowerFirst(label.labelText.trim())}` : 'Buscar');

  return (
    <span
      ref={root}
      className={`select ${multiple ? 'select-multiple' : ''} ${className}`.trim()}
      data-open={open || undefined}
    >
      <button
        ref={trigger}
        type="button"
        id={id}
        role="combobox"
        className="select-trigger"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listID : undefined}
        aria-activedescendant={activeID}
        aria-label={ariaLabel}
        aria-labelledby={labelledBy}
        aria-describedby={describedBy}
        aria-required={required || undefined}
        aria-invalid={ariaInvalid || undefined}
        disabled={disabled}
        value={fieldValue}
        onClick={handleTriggerClick}
        onKeyDown={(event) => (open ? handleOpenKey(event, 'trigger') : handleClosedKey(event))}
        onKeyUp={(event) => {
          // En algunos motores Espacio activa el botón al soltarse; ya se atendió al pulsarse.
          if (event.key === ' ') {
            event.preventDefault();
          }
        }}
      >
        <span
          className={`select-value ${selectedOptions.length === 0 ? 'select-placeholder' : ''}`}
        >
          {triggerText(selectedOptions, multiple, placeholder)}
        </span>
        <Icon name="chevron-down" size={18} className="select-chevron" />
      </button>
      {(required || name) && (
        <input
          className="select-validation"
          tabIndex={-1}
          aria-hidden="true"
          autoComplete="off"
          data-virtual-keyboard="off"
          name={name}
          required={required}
          disabled={disabled}
          value={fieldValue}
          onChange={() => undefined}
          onFocus={() => trigger.current?.focus({ preventScroll: true })}
        />
      )}
      {multiple && chips && selectedOptions.length > 0 && (
        <SelectChips<T>
          options={selectedOptions}
          disabled={disabled}
          onRemove={(value) =>
            onPickValues(selectedValues.filter((current) => current !== value) as T[])
          }
        />
      )}
      {open &&
        createPortal(
          <SelectPopup<T>
            popupRef={popup}
            searchRef={search}
            style={position}
            baseID={baseID}
            listID={listID}
            labelID={labelledBy}
            ariaLabel={ariaLabel}
            multiple={multiple}
            withSearch={withSearch}
            searchName={searchName}
            searchPlaceholder={searchPlaceholder}
            query={query}
            onQuery={changeQuery}
            items={visibleItems}
            actions={actions}
            activeIndex={activeIndex}
            activeID={activeID}
            selected={selectedSet}
            loading={remote && loading}
            emptyMessage={emptyMessage}
            selectedCount={selectedOptions.length}
            onPick={(index) => pick(entries[index])}
            onKeyDown={(event) => handleOpenKey(event, 'search')}
            onDone={() => closeList(true)}
          />,
          document.body,
        )}
    </span>
  );
}
