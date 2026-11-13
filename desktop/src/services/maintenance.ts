import { api } from './api';
import { countLabel } from './plural';

// Mantenimiento del administrador (etapa 9F): «Quitar datos de prueba». Go identifica lo que crearon las
// pruebas automáticas por la firma exacta de cada archivo de prueba, decide qué se elimina, qué se
// desactiva (porque ya tiene operaciones registradas) y qué configuración se desvincula, y antes de quitar
// crea un respaldo. La interfaz solo muestra la lista y pide la confirmación.

export type TestDataKind = 'printer' | 'station' | 'route' | 'product' | 'category' | 'setting';

export interface TestDataEntry {
  kind: TestDataKind;
  id: string;
  name: string;
  reason: string;
}

export interface TestDataPreview {
  // Resumen de la lista revisada: quitar exige el mismo, de modo que nunca se aplica algo distinto.
  plan_id: string;
  remove: TestDataEntry[];
  deactivate: TestDataEntry[];
  unlink: TestDataEntry[];
  keep: TestDataEntry[];
}

export interface TestDataResult {
  removed: TestDataEntry[];
  deactivated: TestDataEntry[];
  unlinked: TestDataEntry[];
  kept: TestDataEntry[];
  // Nombre del archivo de respaldo, sin carpeta; vacío si no había nada que quitar.
  backup: string;
}

export const testDataKindLabels: Record<TestDataKind, string> = {
  printer: 'Impresora',
  station: 'Estación',
  route: 'Ruta de preparación',
  product: 'Producto',
  category: 'Categoría',
  setting: 'Configuración',
};

export function readTestData() {
  return api<TestDataPreview>('maintenance_test_data');
}

export function removeTestData(operationID: string, planID: string) {
  return api<TestDataResult>('maintenance_remove_test_data', {
    body: {
      operation_id: operationID,
      plan_id: planID,
    },
  });
}

export function hasTestData(preview: TestDataPreview) {
  return preview.remove.length + preview.deactivate.length + preview.unlink.length > 0;
}

// Resumen del resultado para el aviso: lo eliminado, lo desactivado, lo desvinculado y el respaldo.
export function testDataSummary(result: TestDataResult) {
  const parts = [
    countLabel(result.removed.length, 'dato eliminado', 'datos eliminados'),
    countLabel(result.deactivated.length, 'dato desactivado', 'datos desactivados'),
    countLabel(result.unlinked.length, 'ajuste desvinculado', 'ajustes desvinculados'),
  ];
  const summary = `${parts.join(', ')}.`;
  if (!result.backup) {
    return summary;
  }
  return `${summary} Respaldo previo: ${result.backup}`;
}
