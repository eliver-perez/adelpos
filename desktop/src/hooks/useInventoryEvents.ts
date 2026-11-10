// Evento de la aplicación que publica useRealtime cuando llegan eventos del ámbito «inventory» por
// SSE (etapa 8), y que la sección Inventarios publica también después de una escritura propia.
// Solo lleva los tipos de evento, nunca cantidades ni costos: quien lo recibe vuelve a consultar
// el estado autoritativo de Go. Las consultas de la sección se recargan con este evento (ver
// useResource, opción reloadEvent) y no con cada cambio de otras áreas (cocina, caja…).
export const inventoryChangeEvent = 'adel-inventory-change';

export interface InventoryChange {
  kinds: string[];
}

// Publica un cambio de inventario; la pestaña visible vuelve a consultar sus datos.
export function notifyInventoryChanged(kinds: string[] = []) {
  window.dispatchEvent(
    new CustomEvent<InventoryChange>(inventoryChangeEvent, {
      detail: { kinds },
    }),
  );
}
