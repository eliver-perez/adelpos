// Navegación entre las pestañas de compras: una orden lleva a sus recepciones, una recepción a su
// orden y el historial de un proveedor a sus documentos. InventoryPage cambia de pestaña y abre el
// documento indicado.
export interface PurchasingNavigation {
  openOrder: (id: string) => void;
  openReceipt: (id: string) => void;
  openSupplier: (id: string) => void;
}
