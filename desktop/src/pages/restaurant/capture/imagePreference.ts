import { useState } from 'react';

// Preferencia «Mostrar imágenes» de la captura, propia de esta terminal. Las preferencias de
// terminal del servidor solo admiten el modo de teclado (cambiarlas exigiría una migración), por
// lo que se guarda en el almacenamiento local del WebView. Si no está disponible (modo privado,
// datos bloqueados), la captura funciona igual con el valor predeterminado.
const storageKey = 'adel-pos.capture.show-images';

export function readShowImages(): boolean {
  try {
    return window.localStorage.getItem(storageKey) !== 'false';
  } catch {
    return true;
  }
}

export function writeShowImages(value: boolean) {
  try {
    window.localStorage.setItem(storageKey, value ? 'true' : 'false');
  } catch {
    // Sin almacenamiento local la preferencia dura hasta cerrar la aplicación.
  }
}

export function useShowImages(): [boolean, (value: boolean) => void] {
  const [value, setValue] = useState(readShowImages);
  return [
    value,
    (next: boolean) => {
      setValue(next);
      writeShowImages(next);
    },
  ];
}
