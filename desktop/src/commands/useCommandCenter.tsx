import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode, RefObject } from 'react';
import { isTauri } from '@tauri-apps/api/core';
import { toggleWindowFullscreen } from '../components/WindowControls';
import { sectionByID } from '../navigation/sections';
import type { ScreenID } from '../navigation/sections';
import type { CashSheet } from '../pages/cash/CashShortcut';
import { notifyInfo } from '../services/notify';
import { actionByID, actionEnabled, actions, commandEntries, enabledMenuActions } from './actions';
import type { CashOptionState, CommandContext, CommandEntry, CommandRuntime } from './actions';
import { CommandPalette } from './CommandPalette';
import type { PaletteScope } from './CommandPalette';
import { createDispatcher } from './dispatcher';
import type { CommandSource } from './dispatcher';
import { AboutDialog, ShortcutsDialog } from './HelpDialogs';
import {
  formShown,
  isEditable,
  modalOpen,
  overlayOpen,
  ownsEscape,
  screenSearch,
} from './keyboard';
import { listenNativeMenu, reportNativeMenuState } from './nativeMenu';
import type { OperationCommandHandlers, OperationCommandState } from './operationBridge';
import { currentPlatform, matchesShortcut } from './platform';

// Centro de comandos (etapa 9C, fase C): une el registro de acciones con sus tres vías —el menú
// nativo (eventos por el Channel de Rust y estado habilitado por menu_set_state), los atajos de
// teclado y la paleta de comandos— y con «Atajos de teclado» y «Acerca de AD-EL POS». Todas las
// vías pasan por el mismo despachador (sin doble ejecución) y ejecutan las funciones de los botones
// que App le entrega.

export interface CommandCenterOptions {
  authenticated: boolean;
  permissions: string[];
  screen: ScreenID | null;
  operation: OperationCommandState | null;
  operationHandlers: RefObject<OperationCommandHandlers | null>;
  cashOptions: CashOptionState[];
  requestScreen: (next: ScreenID) => Promise<boolean>;
  openCashSheet: (sheet: CashSheet) => void;
  logout: () => Promise<void>;
  toggleTheme: () => Promise<void>;
}

interface PaletteState {
  query: string;
  scope: PaletteScope;
}

interface EscapeSnapshot {
  event: KeyboardEvent;
  overlay: boolean;
  editing: boolean;
  owned: boolean;
}

export function useCommandCenter(options: CommandCenterOptions): { overlay: ReactNode } {
  const latest = useRef(options);
  latest.current = options;
  const [dispatcher] = useState(() => createDispatcher());
  const [platform] = useState(currentPlatform);
  const [palette, setPalette] = useState<PaletteState | null>(null);
  const [dialog, setDialog] = useState<'shortcuts' | 'about' | null>(null);
  const desktop = isTauri();

  const contextOf = useCallback(
    (current: CommandCenterOptions): CommandContext => ({
      authenticated: current.authenticated,
      permissions: current.permissions,
      screen: current.screen,
      desktop,
      operation: current.operation,
      cashOptions: current.cashOptions,
    }),
    [desktop],
  );

  const openPalette = useCallback((query = '', scope: PaletteScope = 'all') => {
    setDialog(null);
    setPalette({ query, scope });
  }, []);

  // ⌘F / Ctrl+F: el buscador de la pantalla visible; en el mapa de mesas (sin buscador propio), la
  // búsqueda de mesas de la paleta; en otra pantalla sin buscador, la paleta.
  const focusSearch = useCallback(() => {
    const root = document.querySelector('.admin-main') ?? document.body;
    const input = screenSearch(root);
    if (input) {
      input.focus();
      input.select();
      return;
    }
    const current = latest.current;
    const operation = current.operation;
    if (
      current.screen === 'restaurant' &&
      operation?.view === 'tables' &&
      !operation.captureMode &&
      current.permissions.includes('restaurant.tables.read')
    ) {
      openPalette('', 'tables');
      return;
    }
    openPalette();
  }, [openPalette]);

  const runtime: CommandRuntime = useMemo(
    () => ({
      requestScreen: (next) => latest.current.requestScreen(next),
      operation: () => latest.current.operationHandlers.current,
      openCashSheet: (sheet) => latest.current.openCashSheet(sheet),
      openPalette: (query) => openPalette(query),
      openShortcuts: () => {
        setPalette(null);
        setDialog('shortcuts');
      },
      openAbout: () => {
        setPalette(null);
        setDialog('about');
      },
      toggleFullscreen: async () => {
        try {
          await toggleWindowFullscreen();
        } catch {
          notifyInfo('No se pudo cambiar el modo de ventana.');
        }
      },
      toggleTheme: () => latest.current.toggleTheme(),
      logout: () => latest.current.logout(),
      focusSearch,
      notify: (message) => notifyInfo(message),
    }),
    [focusSearch, openPalette],
  );

  // Punto único de ejecución: el menú nativo, los atajos y la paleta llegan aquí.
  const execute = useCallback(
    (id: string, source: CommandSource) => {
      const action = actionByID(id);
      if (!action || dispatcher.isTwin(id, source)) {
        return false;
      }
      const context = contextOf(latest.current);
      if (!action.visible(context)) {
        dispatcher.record(id, source);
        return false;
      }
      if (!actionEnabled(action, context)) {
        dispatcher.record(id, source);
        if (source !== 'menu') {
          notifyInfo(action.hint ?? 'Esta acción no está disponible ahora.');
        }
        return false;
      }
      // Con un diálogo abierto (una confirmación, la hoja de Caja) el menú no actúa detrás de él;
      // la pantalla completa no cambia nada de la pantalla.
      if (source === 'menu' && action.id !== 'view.fullscreen' && modalOpen()) {
        dispatcher.record(id, source);
        notifyInfo('Cierra la ventana abierta para continuar.');
        return false;
      }
      return dispatcher.dispatch(id, source, () => action.run(runtime, context));
    },
    [contextOf, dispatcher, runtime],
  );
  const executeRef = useRef(execute);
  executeRef.current = execute;

  // Elegir en la paleta: la paleta se cierra (y devuelve el foco) antes de ejecutar, para que una
  // acción que mueve el foco (buscar, abrir una hoja) lo conserve.
  const runEntry = useCallback(
    (entry: CommandEntry) => {
      setPalette(null);
      window.setTimeout(() => {
        if (actionByID(entry.id)) {
          executeRef.current(entry.id, 'palette');
          return;
        }
        dispatcher.dispatch(entry.id, 'palette', entry.run);
      }, 0);
    },
    [dispatcher],
  );

  // Atajos de teclado (Cmd en macOS, Ctrl en Windows).
  useEffect(() => {
    let escape: EscapeSnapshot | null = null;

    // Fase de captura: se toma la foto antes de que los diálogos, menús y listas atiendan su propio
    // Escape (algunos se cierran en ese mismo evento).
    function capture(event: KeyboardEvent) {
      if (event.key !== 'Escape') {
        return;
      }
      escape = {
        event,
        overlay: overlayOpen(),
        editing: isEditable(event.target),
        owned: ownsEscape(event.target),
      };
    }

    function handleEscape(event: KeyboardEvent) {
      const snapshot = escape?.event === event ? escape : null;
      escape = null;
      if (!snapshot || snapshot.overlay || snapshot.owned || event.isComposing) {
        return;
      }
      // Después de todos los oyentes del evento: el teclado del cobro usa Escape para limpiar el
      // importe (y lo marca con preventDefault); entonces no se vuelve de nivel.
      window.setTimeout(() => {
        if (event.defaultPrevented || !latest.current.authenticated) {
          return;
        }
        if (snapshot.editing) {
          // Primer Escape en un campo: solo sale del campo; el siguiente vuelve de nivel.
          if (event.target instanceof HTMLElement) {
            event.target.blur();
          }
          return;
        }
        goBack();
      }, 0);
    }

    // Escape fuera de diálogos: cierra el panel abierto de la operación o vuelve un nivel. En
    // Administración vuelve a la pantalla anterior de la ruta (con la protección de cambios sin
    // guardar), salvo que la pantalla muestre un formulario, para no descartar lo capturado.
    function goBack() {
      const current = latest.current;
      const screen = current.screen;
      if (!screen) {
        return;
      }
      if (screen === 'restaurant' || screen === 'takeaway') {
        current.operationHandlers.current?.back();
        return;
      }
      const entry = sectionByID(screen);
      const root = document.querySelector('.admin-main');
      if (!entry.parent || (root && formShown(root))) {
        return;
      }
      void current.requestScreen(entry.parent);
    }

    function keydown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        handleEscape(event);
        return;
      }
      if (event.defaultPrevented || event.isComposing) {
        return;
      }
      const action = actions.find(
        (candidate) => candidate.shortcut && matchesShortcut(event, candidate.shortcut, platform),
      );
      if (!action) {
        return;
      }
      // El atajo es de la aplicación: el navegador no hace lo suyo (buscar, imprimir, ventana
      // nueva) y, en macOS, el menú nativo no repite la acción.
      event.preventDefault();
      if (event.repeat) {
        return;
      }
      // Mientras se escribe en un campo o con un diálogo, menú o lista abiertos, los atajos no
      // actúan (la pantalla completa sí, porque no cambia nada de la pantalla).
      const blocked =
        isEditable(event.target) || (action.id !== 'view.fullscreen' && overlayOpen());
      if (blocked) {
        dispatcher.suppress(action.id, 'keyboard');
        return;
      }
      executeRef.current(action.id, 'keyboard');
    }

    window.addEventListener('keydown', capture, true);
    window.addEventListener('keydown', keydown);
    return () => {
      window.removeEventListener('keydown', capture, true);
      window.removeEventListener('keydown', keydown);
    };
  }, [dispatcher, platform]);

  // Eventos del menú nativo.
  useEffect(
    () =>
      listenNativeMenu((action) => {
        executeRef.current(action, 'menu');
      }),
    [],
  );

  // Estado del menú nativo: se informa solo cuando cambia.
  const context = contextOf(options);
  const enabledKey = enabledMenuActions(context).join(',');
  useEffect(() => {
    reportNativeMenuState(enabledKey ? enabledKey.split(',') : []);
  }, [enabledKey]);

  let overlay: ReactNode = null;
  if (palette && options.authenticated) {
    const tables = options.operationHandlers.current?.tables() ?? [];
    overlay = (
      <CommandPalette
        entries={commandEntries(context, runtime, tables)}
        initialQuery={palette.query}
        scope={palette.scope}
        platform={platform}
        onRun={runEntry}
        onClose={() => setPalette(null)}
      />
    );
  } else if (dialog === 'shortcuts') {
    overlay = <ShortcutsDialog platform={platform} onClose={() => setDialog(null)} />;
  } else if (dialog === 'about') {
    overlay = <AboutDialog onClose={() => setDialog(null)} />;
  }
  return { overlay };
}
