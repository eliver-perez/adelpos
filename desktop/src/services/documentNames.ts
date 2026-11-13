import { useSyncExternalStore } from 'react';

// Nombre visible del documento previo al cobro (etapa 9F, §4.5). El sistema anterior llamaba «ticket
// de venta» a lo que se entregaba antes de cobrar; el restaurante elige en Administración › Documentos
// (revisión 9F, M3; antes en Cobros › Políticas de cobro) cómo se llama en botones, avisos, la paleta, el
// menú y el encabezado impreso. Internamente es la
// misma precuenta, con las mismas reglas. La clave llega con la sesión (establishment.precheck_name),
// que leen todos los usuarios, y Go la valida contra la misma lista cerrada.

export type PrecheckNameKey = 'precheck' | 'sales_ticket' | 'bill';

export interface PrecheckName {
  key: PrecheckNameKey;
  // «Precuenta» en botones y títulos.
  label: string;
  // «precuenta» dentro de una frase («Imprimir precuenta»).
  lower: string;
  // Con su artículo: «la precuenta», «el ticket de venta».
  withArticle: string;
  // Con «de» y su artículo: «de la precuenta», «del ticket de venta».
  ofWithArticle: string;
  // «precuentas» en «No hay impresora configurada para precuentas».
  plural: string;
  // Participio que concuerda en género: «enviada» o «enviado».
  sent: string;
  // «impresa» o «impreso» (revisión 9F, M3: «precuenta impresa» en el mapa de mesas y el Registro).
  printedAdjective: string;
  // Encabezado impreso.
  printed: string;
}

export const precheckNames: Record<PrecheckNameKey, PrecheckName> = {
  precheck: {
    key: 'precheck',
    label: 'Precuenta',
    lower: 'precuenta',
    withArticle: 'la precuenta',
    ofWithArticle: 'de la precuenta',
    plural: 'precuentas',
    sent: 'enviada',
    printedAdjective: 'impresa',
    printed: 'PRECUENTA',
  },
  sales_ticket: {
    key: 'sales_ticket',
    label: 'Ticket de venta',
    lower: 'ticket de venta',
    withArticle: 'el ticket de venta',
    ofWithArticle: 'del ticket de venta',
    plural: 'tickets de venta',
    sent: 'enviado',
    printedAdjective: 'impreso',
    printed: 'TICKET DE VENTA',
  },
  bill: {
    key: 'bill',
    label: 'Cuenta',
    lower: 'cuenta',
    withArticle: 'la cuenta',
    ofWithArticle: 'de la cuenta',
    plural: 'cuentas',
    sent: 'enviada',
    printedAdjective: 'impresa',
    printed: 'CUENTA',
  },
};

// Opciones de la lista cerrada en su orden (Administración › Documentos › Identidad del establecimiento).
export const precheckNameOptions = (Object.keys(precheckNames) as PrecheckNameKey[]).map((key) => ({
  value: key,
  label: precheckNames[key].label,
}));

let current: PrecheckNameKey = 'precheck';
const listeners = new Set<() => void>();

function isKey(value: unknown): value is PrecheckNameKey {
  return typeof value === 'string' && value in precheckNames;
}

// App la actualiza con cada identidad de la sesión; una clave desconocida o ausente es «Precuenta».
export function setPrecheckName(key: unknown) {
  const next = isKey(key) ? key : 'precheck';
  if (next === current) {
    return;
  }
  current = next;
  for (const listener of listeners) {
    listener();
  }
}

export function precheckName(): PrecheckName {
  return precheckNames[current];
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function snapshot() {
  return current;
}

// Nombre vigente para los componentes: se vuelven a dibujar cuando cambia.
export function usePrecheckName(): PrecheckName {
  return precheckNames[useSyncExternalStore(subscribe, snapshot, snapshot)];
}
