// Plataforma y atajos de teclado (etapa 9C, fase C). Los atajos usan ⌘ en macOS y Ctrl en Windows;
// el menú nativo de Rust (src-tauri/src/menu.rs) declara los mismos con «CmdOrCtrl» y una prueba de
// Vitest comprueba que coinciden.

export type Platform = 'mac' | 'other';

export function currentPlatform(): Platform {
  if (typeof navigator === 'undefined') {
    return 'other';
  }
  const source = `${navigator.platform ?? ''} ${navigator.userAgent ?? ''}`;
  return /Mac|iPhone|iPad/.test(source) ? 'mac' : 'other';
}

// Combinación de teclas. `mod` es ⌘ en macOS y Ctrl en Windows; `ctrl` es Control además de `mod`
// (solo el atajo de pantalla completa de macOS, ⌃⌘F). `key` es una letra en minúscula, un dígito o
// el nombre de una tecla (F11).
export interface KeyCombo {
  key: string;
  mod?: boolean;
  ctrl?: boolean;
}

export interface Shortcut {
  mac: KeyCombo;
  other: KeyCombo;
}

// Mismo atajo en ambas plataformas con el modificador principal.
export function modShortcut(key: string): Shortcut {
  return { mac: { key, mod: true }, other: { key, mod: true } };
}

// Pantalla completa con el atajo estándar de cada sistema: ⌃⌘F en macOS y F11 en Windows.
export const fullscreenShortcut: Shortcut = {
  mac: { key: 'f', mod: true, ctrl: true },
  other: { key: 'F11' },
};

function comboOf(shortcut: Shortcut, platform: Platform) {
  return platform === 'mac' ? shortcut.mac : shortcut.other;
}

function keyName(key: string) {
  return key.length === 1 ? key.toUpperCase() : key;
}

// Texto visible del atajo: «⌘1» y «⌃⌘F» en macOS; «Ctrl+1» y «F11» en Windows.
export function shortcutLabel(shortcut: Shortcut, platform: Platform = currentPlatform()) {
  const combo = comboOf(shortcut, platform);
  if (platform === 'mac') {
    return `${combo.ctrl ? '⌃' : ''}${combo.mod ? '⌘' : ''}${keyName(combo.key)}`;
  }
  const parts: string[] = [];
  if (combo.mod || combo.ctrl) {
    parts.push('Ctrl');
  }
  parts.push(keyName(combo.key));
  return parts.join('+');
}

// Valor de aria-keyshortcuts («Meta+1», «Control+Meta+F», «Control+1», «F11»).
export function ariaShortcut(shortcut: Shortcut, platform: Platform = currentPlatform()) {
  const combo = comboOf(shortcut, platform);
  const parts: string[] = [];
  if (platform === 'mac') {
    if (combo.ctrl) {
      parts.push('Control');
    }
    if (combo.mod) {
      parts.push('Meta');
    }
  } else if (combo.mod || combo.ctrl) {
    parts.push('Control');
  }
  parts.push(keyName(combo.key));
  return parts.join('+');
}

// Una pulsación coincide con el atajo si lleva exactamente sus modificadores (sin Alt ni Mayús) y
// su tecla. Los dígitos aceptan también la tecla física (event.code), para que ⌘1 funcione con la
// distribución que sea; las letras siguen el carácter, como los atajos del menú de macOS.
export function matchesShortcut(
  event: Pick<KeyboardEvent, 'key' | 'code' | 'metaKey' | 'ctrlKey' | 'altKey' | 'shiftKey'>,
  shortcut: Shortcut,
  platform: Platform = currentPlatform(),
) {
  const combo = comboOf(shortcut, platform);
  const wantMeta = platform === 'mac' ? !!combo.mod : false;
  const wantControl = platform === 'mac' ? !!combo.ctrl : !!combo.mod || !!combo.ctrl;
  if (event.metaKey !== wantMeta || event.ctrlKey !== wantControl) {
    return false;
  }
  if (event.altKey || event.shiftKey) {
    return false;
  }
  const key = combo.key;
  if (/^[0-9]$/.test(key)) {
    return event.key === key || event.code === `Digit${key}`;
  }
  if (/^[a-z]$/.test(key)) {
    return event.key.toLowerCase() === key;
  }
  return event.key === key;
}
