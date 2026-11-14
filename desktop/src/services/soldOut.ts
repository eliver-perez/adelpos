import { api } from './api';

// «Agotado» desde la captura (etapa 9G, §5.4, decisión D8). Go decide todo: hasta cuándo dura (el inicio del día
// comercial siguiente o hasta nuevo aviso), que un producto agotado no se capture y el aviso a las demás terminales.
// La interfaz solo envía el cambio con una clave de operación nueva y muestra el estado que devuelve Go.

export type SoldOutDuration = 'today' | 'indefinite';

export interface SoldOutState {
  product_id: string;
  name: string;
  sold_out: boolean;
  indefinite: boolean;
  sold_out_at: string | null;
  sold_out_until: string | null;
  catalog_version: number;
}

export const soldOutDurations: { value: SoldOutDuration; label: string }[] = [
  { value: 'today', label: 'Solo por hoy' },
  { value: 'indefinite', label: 'Hasta nuevo aviso' },
];

export function setSoldOut(
  productID: string,
  soldOut: boolean,
  duration: SoldOutDuration = 'today',
): Promise<SoldOutState> {
  return api<SoldOutState>('catalog_product_sold_out', {
    id: productID,
    body: {
      operation_id: crypto.randomUUID(),
      sold_out: soldOut,
      indefinite: soldOut && duration === 'indefinite',
    },
  });
}

// soldOutMessage es el aviso del resultado, con el estado que devolvió Go.
export function soldOutMessage(state: SoldOutState): string {
  if (!state.sold_out) {
    return `«${state.name}» está disponible otra vez.`;
  }
  if (state.indefinite) {
    return `«${state.name}» quedó agotado hasta nuevo aviso.`;
  }
  return `«${state.name}» quedó agotado por hoy.`;
}
