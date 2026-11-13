import { APIError } from './api';
import { dismissNotice, notifyWarning } from './notify';
import type { NotifyAction } from './notify';

// Problemas de impresión en palabras del restaurante (etapa 9F, §4.1–§4.3). Go entrega una clave de una
// lista cerrada (kitchen/print_problems.go: el problema del último intento o del trabajo encolado) y esta
// capa la convierte en una frase con el nombre de la impresora, sin términos técnicos. El mensaje técnico
// del transporte queda solo para soporte.

// «not_found» (revisión 9F, B4): la impresora o su cola no existe en el equipo servidor (cola CUPS
// desconocida, impresora no instalada o renombrada en Windows); a diferencia de «no responde», reintentar no
// lo resuelve y el aviso ofrece «Configurar impresora».
export type PrintProblem =
  'unreachable' | 'not_found' | 'inactive' | 'unsupported' | 'setup' | 'uncertain';

// Qué impresora: la de caja (precuentas, tickets y documentos de caja, la «Impresora de tickets» de
// Cobros) o cualquier otra (comandas, pruebas y muestras, por su nombre).
export type PrinterRole = 'cash' | 'other';

function named(printerName?: string | null) {
  const name = printerName?.trim();
  return name ? `La impresora ${name}` : 'La impresora';
}

// Frase de lo que pasó. Vacía si el problema no se conoce o no hay problema.
export function printProblemMessage(
  problem: string | null | undefined,
  printerName?: string | null,
  role: PrinterRole = 'other',
): string {
  switch (problem) {
    case 'unreachable':
      return role === 'cash'
        ? 'La impresora de caja no responde.'
        : `${named(printerName)} está apagada o sin conexión.`;
    case 'not_found':
      return role === 'cash'
        ? 'No se encontró la impresora de caja en el equipo.'
        : `No se encontró ${printerName?.trim() ? `la impresora ${printerName.trim()}` : 'la impresora'} en el equipo.`;
    case 'inactive':
      return `${named(printerName)} está desactivada.`;
    case 'unsupported':
      return 'Esta impresora todavía no puede usarse para imprimir.';
    case 'setup':
      return `${named(printerName)} necesita que se revise su configuración.`;
    case 'uncertain':
      return 'No se sabe si el documento salió en papel.';
  }
  return '';
}

// Qué hacer, como segunda línea del aviso.
export function printProblemHint(problem: string | null | undefined, printerName?: string | null) {
  const name = printerName?.trim();
  switch (problem) {
    case 'unreachable':
      return `Revisa que ${name ? `la impresora ${name}` : 'la impresora'} esté encendida, con papel y conectada, y toca «Reintentar».`;
    case 'uncertain':
      return 'Revisa el papel en la impresora antes de pedir otra copia; se resuelve en Cocina › Cola de impresión.';
    case 'not_found':
      return 'Revisa en Cocina › Impresoras que esté elegida la impresora del equipo o elige otra en Cobros › Políticas de cobro.';
    case 'inactive':
    case 'unsupported':
    case 'setup':
      return 'Revisa la impresora en Cocina › Impresoras o elige otra en Cobros › Políticas de cobro.';
  }
  return '';
}

// Problemas que se resuelven configurando la impresora (no esperando a que vuelva a responder): con ellos se
// ofrece «Configurar impresora» además de «Reintentar» (revisión 9F, B4).
export function printerSetupProblem(problem: string | null | undefined) {
  return (
    problem === 'not_found' ||
    problem === 'inactive' ||
    problem === 'unsupported' ||
    problem === 'setup'
  );
}

// Un trabajo que nunca salió en papel se reintenta con un toque, sin motivo.
export function retryableProblem(state: string | null | undefined) {
  return state === 'error' || state === 'intervention';
}

// «Configurar impresora» lleva a Administración › Cobros › Políticas de cobro con el único requestScreen.
// App lo registra solo si el usuario puede elegir la impresora de tickets; si no, el aviso dice a quién
// pedirlo.
let openPrinterSetup: (() => void) | null = null;

export function registerPrinterSetup(open: (() => void) | null) {
  openPrinterSetup = open;
}

export function printerSetupAvailable() {
  return openPrinterSetup !== null;
}

// Para un botón «Configurar impresora» fuera de un aviso.
export function openPrinterSetupScreen() {
  openPrinterSetup?.();
}

export const printerSetupLabel = 'Configurar impresora';

const askAdministrator =
  'Pide a quien administra Cobros que asigne la impresora de tickets en Cobros › Políticas de cobro.';

// Rechazos de Go por falta de una impresora válida para precuentas, tickets y documentos de caja.
export const printerUnavailableCodes = ['PRINTER_NOT_CONFIGURED', 'PRINTER_UNAVAILABLE'];

export function isPrinterUnavailable(problem: unknown): problem is APIError {
  return problem instanceof APIError && printerUnavailableCodes.includes(problem.code);
}

// Acciones del aviso: «Configurar impresora» (si el usuario puede) y «Reintentar».
export function printerActions(retry?: () => void): NotifyAction[] {
  const actions: NotifyAction[] = [];
  if (openPrinterSetup) {
    const open = openPrinterSetup;
    actions.push({ label: printerSetupLabel, onClick: () => open() });
  }
  if (retry) {
    actions.push({ label: 'Reintentar', onClick: retry });
  }
  return actions;
}

// Aviso vigente de cada documento (clave → id del aviso de Sonner).
const shownNotices = new Map<string, string>();
let noticeSequence = 0;

// Aviso inmediato de que el documento no tiene una impresora válida (decisión D4): el mensaje de Go
// («No hay impresora configurada para precuentas», «La impresora … está desactivada»), «Configurar
// impresora» y «Reintentar» en el mismo aviso, que permanece hasta cerrarse.
//
// Cada aviso lleva un id nuevo y retira el anterior del mismo documento: tocar una acción cierra el
// aviso y, si el reintento vuelve a fallar antes de que termine su animación de salida, un aviso con el
// mismo id quedaba retirado junto con el anterior (se observó en WebKit, el motor de la ventana de macOS).
export function notifyPrinterUnavailable(
  message: string,
  retry?: () => void,
  key = 'printer-missing',
) {
  const previous = shownNotices.get(key);
  if (previous) {
    dismissNotice(previous);
  }
  noticeSequence += 1;
  const id = `${key}:${noticeSequence}`;
  shownNotices.set(key, id);
  notifyWarning(message, {
    id,
    persistent: true,
    description: openPrinterSetup ? undefined : askAdministrator,
    actions: printerActions(retry),
  });
}

// Avisa el rechazo si es por falta de impresora; devuelve si lo era.
export function notifyPrinterRejection(problem: unknown, retry?: () => void, key?: string) {
  if (!isPrinterUnavailable(problem)) {
    return false;
  }
  notifyPrinterUnavailable(problem.message, retry, key);
  return true;
}
