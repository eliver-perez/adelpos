import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { KeyboardEvent } from 'react';
import { trapFocus } from '../components/ConfirmDialog';
import { Icon } from '../components/Icon';
import { countLabel } from '../services/plural';
import type { CommandEntry } from './actions';
import { ariaShortcut, matchesShortcut, modShortcut, shortcutLabel } from './platform';
import type { Platform } from './platform';
import { searchEntries } from './search';

export type PaletteScope = 'all' | 'tables';

const paletteShortcut = modShortcut('k');

// Paleta de comandos (etapa 9C, §3.3): una búsqueda para ir a cualquier pantalla, ejecutar una
// acción o elegir una mesa escribiendo, con las mismas entradas que el menú nativo y los atajos
// (filtradas por permisos y contexto). Patrón ARIA combobox con lista: el foco se queda en el campo
// y las flechas mueven la opción activa (aria-activedescendant); Intro la ejecuta, Escape cierra.
// Opciones de 48 px para usarla también con el dedo.
export function CommandPalette({
  entries,
  initialQuery = '',
  scope = 'all',
  platform,
  onRun,
  onClose,
}: {
  entries: CommandEntry[];
  initialQuery?: string;
  scope?: PaletteScope;
  platform: Platform;
  onRun: (entry: CommandEntry) => void;
  onClose: () => void;
}) {
  const [query, setQuery] = useState(initialQuery);
  const [active, setActive] = useState(0);
  const [hint, setHint] = useState('');
  const panel = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const titleID = useId();
  const listID = useId();
  const optionPrefix = useId();
  const tables = scope === 'tables';
  const results = useMemo(() => {
    const scoped = tables ? entries.filter((entry) => entry.kind === 'Mesa') : entries;
    if (tables && !query.trim()) {
      return scoped.slice(0, 60);
    }
    return searchEntries(scoped, query);
  }, [entries, query, tables]);
  const current = results[Math.min(active, Math.max(results.length - 1, 0))];

  // El foco entra antes de pintar (etapa 9E, como «Acerca de» en la 9D): con un efecto normal,
  // un Escape pulsado justo al aparecer el diálogo llegaba fuera de él y no lo cerraba.
  useLayoutEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    input.current?.focus();
    input.current?.select();
    return () => {
      if (previous && previous.isConnected) {
        previous.focus();
      }
    };
  }, []);

  // La opción activa siempre queda a la vista dentro de la lista.
  useEffect(() => {
    if (!current) {
      return;
    }
    const element = list.current?.querySelector<HTMLElement>(`[data-entry="${current.id}"]`);
    element?.scrollIntoView?.({ block: 'nearest' });
  }, [current]);

  function choose(entry: CommandEntry | undefined) {
    if (!entry) {
      return;
    }
    if (!entry.enabled) {
      setHint(entry.hint ?? 'Esta acción no está disponible ahora.');
      return;
    }
    onRun(entry);
  }

  function move(delta: number) {
    if (results.length === 0) {
      return;
    }
    setHint('');
    setActive((index) => {
      const base = Math.min(index, results.length - 1);
      return (base + delta + results.length) % results.length;
    });
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      onClose();
      return;
    }
    // ⌘K / Ctrl+K vuelve a cerrar la paleta.
    if (matchesShortcut(event.nativeEvent, paletteShortcut, platform)) {
      event.preventDefault();
      event.stopPropagation();
      onClose();
      return;
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      move(event.key === 'ArrowDown' ? 1 : -1);
      return;
    }
    if (event.key === 'PageDown' || event.key === 'PageUp') {
      event.preventDefault();
      move(event.key === 'PageDown' ? 5 : -5);
      return;
    }
    if ((event.key === 'Home' || event.key === 'End') && event.ctrlKey) {
      event.preventDefault();
      setActive(event.key === 'Home' ? 0 : Math.max(results.length - 1, 0));
      return;
    }
    if (event.key === 'Enter') {
      event.preventDefault();
      choose(current);
      return;
    }
    if (event.key === 'Tab' && panel.current) {
      trapFocus(event, panel.current);
    }
  }

  const label = tables ? 'Buscar mesa' : 'Paleta de comandos';
  const activeID = current ? `${optionPrefix}-${results.indexOf(current)}` : undefined;
  return (
    <div
      className="dialog-backdrop command-palette-backdrop"
      onPointerDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <div
        ref={panel}
        className="command-palette"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        onKeyDown={handleKeyDown}
      >
        <h2 id={titleID} className="visually-hidden">
          {label}
        </h2>
        <div className="command-palette-search">
          <Icon name="search" size={20} />
          <input
            ref={input}
            type="text"
            role="combobox"
            aria-label={tables ? 'Buscar mesa por nombre o área' : 'Buscar pantallas y acciones'}
            aria-expanded="true"
            aria-controls={listID}
            aria-autocomplete="list"
            aria-activedescendant={activeID}
            autoComplete="off"
            spellCheck={false}
            placeholder={tables ? 'Mesa o área…' : 'Escribe una pantalla, una acción o una mesa…'}
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setActive(0);
              setHint('');
            }}
          />
          <kbd
            className="command-palette-key"
            aria-label={`Cerrar con Esc o ${shortcutLabel(paletteShortcut, platform)}`}
          >
            Esc
          </kbd>
        </div>
        <div
          ref={list}
          id={listID}
          className="command-palette-list"
          role="listbox"
          aria-label="Resultados"
        >
          {results.map((entry, index) => {
            const selected = entry === current;
            return (
              <div
                key={entry.id}
                id={`${optionPrefix}-${index}`}
                data-entry={entry.id}
                role="option"
                aria-selected={selected}
                aria-disabled={!entry.enabled || undefined}
                aria-keyshortcuts={
                  entry.shortcut ? ariaShortcut(entry.shortcut, platform) : undefined
                }
                className={`command-palette-option ${selected ? 'command-palette-active' : ''} ${
                  entry.enabled ? '' : 'command-palette-disabled'
                }`}
                onPointerMove={() => {
                  if (!selected) {
                    setActive(index);
                  }
                }}
                onClick={() => {
                  setActive(index);
                  choose(entry);
                }}
              >
                <span className="command-palette-text">
                  <strong>{entry.label}</strong>
                  {(entry.description || (!entry.enabled && entry.hint)) && (
                    <span className="muted">{entry.enabled ? entry.description : entry.hint}</span>
                  )}
                </span>
                <span className="command-palette-kind">{entry.kind}</span>
                {entry.shortcut && (
                  <kbd className="command-palette-key">
                    {shortcutLabel(entry.shortcut, platform)}
                  </kbd>
                )}
              </div>
            );
          })}
        </div>
        <p className="command-palette-status" role="status">
          {hint ||
            (results.length === 0
              ? `Sin resultados para «${query.trim()}».`
              : `${countLabel(results.length, 'resultado', 'resultados')} · ↑ ↓ para elegir · Intro para abrir`)}
        </p>
      </div>
    </div>
  );
}
