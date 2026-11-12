import type { CSSProperties, KeyboardEvent as ReactKeyboardEvent, RefObject } from 'react';
import { Icon } from './Icon';
import { countLabel } from '../services/plural';
import { flattenOptions, isGroup } from './selectModel';
import type { SelectAction, SelectItem, SelectOption } from './selectModel';

// Partes visibles del selector único (components/Select.tsx, etapa 9C): la lista emergente con su
// búsqueda, grupos, opciones de acción y anuncio de resultados, y las etiquetas de lo elegido en la
// selección múltiple. Solo dibujan: el estado y el teclado los lleva Select.tsx.

export interface SelectPopupProps<T extends string> {
  popupRef: RefObject<HTMLDivElement | null>;
  searchRef: RefObject<HTMLInputElement | null>;
  style: CSSProperties | null;
  baseID: string;
  listID: string;
  labelID: string | undefined;
  ariaLabel: string | undefined;
  multiple: boolean;
  withSearch: boolean;
  searchName: string;
  searchPlaceholder: string;
  query: string;
  onQuery: (query: string) => void;
  items: readonly SelectItem<T>[];
  actions: readonly SelectAction[];
  activeIndex: number;
  activeID: string | undefined;
  selected: ReadonlySet<string>;
  loading: boolean;
  emptyMessage: string;
  selectedCount: number;
  onPick: (index: number) => void;
  onKeyDown: (event: ReactKeyboardEvent<HTMLInputElement>) => void;
  onDone: () => void;
}

// Identificador de cada elemento recorrible (aria-activedescendant apunta a él).
export function optionID(baseID: string, index: number) {
  return `${baseID}-option-${index}`;
}

function resultsAnnouncement(loading: boolean, query: string, count: number, empty: string) {
  if (loading) {
    return 'Buscando…';
  }
  if (query.trim() === '') {
    return '';
  }
  return count === 0 ? empty : countLabel(count, 'resultado', 'resultados');
}

export function SelectPopup<T extends string>({
  popupRef,
  searchRef,
  style,
  baseID,
  listID,
  labelID,
  ariaLabel,
  multiple,
  withSearch,
  searchName,
  searchPlaceholder,
  query,
  onQuery,
  items,
  actions,
  activeIndex,
  activeID,
  selected,
  loading,
  emptyMessage,
  selectedCount,
  onPick,
  onKeyDown,
  onDone,
}: SelectPopupProps<T>) {
  const visibleCount = flattenOptions(items).length;
  // Las opciones y las acciones se numeran en el orden en que se ven (el mismo de la lista plana
  // que recorre el teclado).
  let position = 0;

  function optionElement(option: SelectOption<T>) {
    const index = position;
    position += 1;
    const isSelected = selected.has(option.value);
    const classes = ['select-option'];
    if (index === activeIndex) {
      classes.push('select-option-active');
    }
    if (isSelected) {
      classes.push('select-option-selected');
    }
    return (
      <div
        key={`option-${option.value}`}
        id={optionID(baseID, index)}
        role="option"
        className={classes.join(' ')}
        data-value={option.value}
        aria-selected={isSelected}
        aria-disabled={option.disabled || undefined}
        onClick={() => onPick(index)}
      >
        {multiple && (
          <span className="select-check-box" aria-hidden="true">
            {isSelected && <Icon name="check" size={16} />}
          </span>
        )}
        <span className="select-option-text">
          <span className="select-option-label">{option.label}</span>
          {option.description && (
            <span className="select-option-description">{option.description}</span>
          )}
        </span>
        {!multiple && isSelected && <Icon name="check" size={18} className="select-option-check" />}
      </div>
    );
  }

  const optionElements = items.map((item, itemIndex) => {
    if (!isGroup(item)) {
      return optionElement(item);
    }
    const groupLabelID = `${baseID}-group-${itemIndex}`;
    return (
      <div
        key={`group-${item.label}`}
        role="group"
        className="select-group"
        aria-labelledby={groupLabelID}
      >
        <div id={groupLabelID} role="presentation" className="select-group-label">
          {item.label}
        </div>
        {item.options.map((option) => optionElement(option))}
      </div>
    );
  });

  const actionElements = actions.map((action) => {
    const index = position;
    position += 1;
    const classes = ['select-option', 'select-action'];
    if (index === activeIndex) {
      classes.push('select-option-active');
    }
    return (
      <div
        key={`action-${action.label}`}
        id={optionID(baseID, index)}
        role="option"
        aria-selected={false}
        aria-disabled={action.disabled || undefined}
        className={classes.join(' ')}
        onClick={() => onPick(index)}
      >
        {action.icon && <Icon name={action.icon} size={18} />}
        <span className="select-option-text">
          <span className="select-option-label">{action.label}</span>
        </span>
      </div>
    );
  });

  return (
    <div
      ref={popupRef}
      className="select-popup"
      style={style ?? { visibility: 'hidden', left: 0, top: 0 }}
      // El foco se queda en el disparador o en la búsqueda al tocar una opción.
      onMouseDown={(event) => {
        if (event.target !== searchRef.current) {
          event.preventDefault();
        }
      }}
      // Los clics de la lista no llegan a los componentes que contienen al selector (un renglón con
      // clic propio, un diálogo…): el portal sigue dentro del árbol de React.
      onClick={(event) => event.stopPropagation()}
    >
      {withSearch && (
        <div className="select-search">
          <Icon name="search" size={18} />
          <input
            ref={searchRef}
            type="text"
            role="searchbox"
            enterKeyHint="search"
            autoComplete="off"
            spellCheck={false}
            maxLength={100}
            aria-label={searchName}
            aria-controls={listID}
            aria-activedescendant={activeID}
            aria-autocomplete="list"
            placeholder={searchPlaceholder}
            value={query}
            onChange={(event) => onQuery(event.target.value)}
            onKeyDown={onKeyDown}
          />
        </div>
      )}
      <div
        id={listID}
        role="listbox"
        className="select-list"
        aria-labelledby={labelID}
        aria-label={labelID ? undefined : ariaLabel}
        aria-multiselectable={multiple || undefined}
      >
        {optionElements}
        {actionElements}
      </div>
      {loading && <p className="select-status">Buscando…</p>}
      {!loading && visibleCount === 0 && (
        <p className="select-status">{query.trim() !== '' ? emptyMessage : 'No hay opciones.'}</p>
      )}
      {multiple && (
        <div className="select-footer">
          <span className="muted">
            {countLabel(selectedCount, 'seleccionado', 'seleccionados')}
          </span>
          <button type="button" className="secondary-button" onClick={onDone}>
            Listo
          </button>
        </div>
      )}
      <span className="visually-hidden" role="status" aria-live="polite">
        {resultsAnnouncement(loading, query, visibleCount, emptyMessage)}
      </span>
    </div>
  );
}

// Lo elegido en la selección múltiple, a la vista y con un botón de 44 px para quitar cada uno.
export function SelectChips<T extends string>({
  options,
  disabled,
  onRemove,
}: {
  options: SelectOption<T>[];
  disabled: boolean;
  onRemove: (value: T) => void;
}) {
  return (
    <span
      className="select-chips"
      role="list"
      aria-label="Elegidos"
      // Un toque sobre las etiquetas no debe activar el <label> que las contiene (abriría la lista).
      onClick={(event) => event.preventDefault()}
    >
      {options.map((option) => (
        <span key={option.value} className="select-chip" role="listitem">
          <span className="select-chip-label">{option.label}</span>
          <button
            type="button"
            className="select-chip-remove"
            aria-label={`Quitar «${option.label}»`}
            disabled={disabled}
            onClick={() => onRemove(option.value)}
          >
            <Icon name="close" size={16} />
          </button>
        </span>
      ))}
    </span>
  );
}
