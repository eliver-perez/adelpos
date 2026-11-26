// Etapa K1.1 (§4.2): los botones del diagnóstico de una pantalla de cocina (Sistema › Terminales) abren Cocina en la
// pestaña donde se corrige el aviso (Estaciones, Enrutamiento o Impresoras). La petición se deja aquí antes de navegar
// con el único requestScreen y la página de Cocina la toma una sola vez al abrirse, como «Configurar impresora» con
// Cobros (payments/policiesRequest.ts).
export type KitchenView = 'stations' | 'routing' | 'printers';

let requested: KitchenView | null = null;

export function requestKitchenView(view: KitchenView) {
  requested = view;
}

export function takeKitchenViewRequest(): KitchenView | null {
  const value = requested;
  requested = null;
  return value;
}
