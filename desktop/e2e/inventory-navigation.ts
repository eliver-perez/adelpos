import { expect } from '@playwright/test';
import type { Page } from '@playwright/test';

// Navegación agrupada de Inventarios (etapa 9A, §3.6): cinco grupos en la primera fila y las
// secciones del grupo en la segunda. Refleja desktop/src/pages/inventory/inventorySections.ts.

export const inventoryGroups = ['Resumen', 'Catálogo', 'Existencias', 'Compras', 'Configuración'];

// Secciones de cada grupo con todos los permisos (Resumen no tiene segunda fila).
export const inventorySectionsByGroup: Record<string, string[]> = {
  Resumen: [],
  Catálogo: ['Insumos', 'Recetas', 'Almacenes', 'Proveedores'],
  Existencias: ['Existencias', 'Movimientos', 'Transferencias', 'Ajustes y mermas', 'Conteos'],
  Compras: [
    'Órdenes de compra',
    'Sugerencias de compra',
    'Recepciones',
    'Devoluciones a proveedor',
  ],
  Configuración: ['Unidades', 'Categorías', 'Parámetros'],
};

// Todas las secciones en orden, para recorrerlas.
export const allInventorySections = [
  'Resumen',
  ...inventoryGroups.flatMap((group) => inventorySectionsByGroup[group] ?? []),
];

function groupOf(section: string) {
  if (section === 'Resumen') {
    return 'Resumen';
  }
  const group = inventoryGroups.find((entry) =>
    (inventorySectionsByGroup[entry] ?? []).includes(section),
  );
  if (!group) {
    throw new Error(`Sección de inventario desconocida: ${section}`);
  }
  return group;
}

export function groupTablist(page: Page) {
  return page.getByRole('tablist', { name: 'Secciones de inventario' });
}

export function sectionTablist(page: Page, group: string) {
  return page.getByRole('tablist', { name: `Apartados de ${group}` });
}

// Abre una sección: primero su grupo y después la sección en la segunda fila.
export async function openInventorySection(page: Page, section: string) {
  const group = groupOf(section);
  const groupTab = groupTablist(page).getByRole('tab', { name: group, exact: true });
  await groupTab.click();
  await expect(groupTab).toHaveAttribute('aria-selected', 'true');
  if (group === 'Resumen') {
    return;
  }
  const sectionTab = sectionTablist(page, group).getByRole('tab', { name: section, exact: true });
  await sectionTab.click();
  await expect(sectionTab).toHaveAttribute('aria-selected', 'true');
}

// La sección indicada está abierta (después de llegar por un destino interno).
export async function expectInventorySection(page: Page, section: string) {
  const group = groupOf(section);
  await expect(groupTablist(page).getByRole('tab', { name: group, exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  if (group !== 'Resumen') {
    await expect(
      sectionTablist(page, group).getByRole('tab', { name: section, exact: true }),
    ).toHaveAttribute('aria-selected', 'true');
  }
}

// La navegación cabe sin desplazarse a lo ancho, cada fila en una sola línea de columnas iguales
// y con pestañas de al menos 44 px: recorre cada grupo y comprueba sus dos filas.
export async function checkInventoryNavigation(
  page: Page,
  size: { width: number; height: number },
  capture: (name: string) => Promise<void> = async () => undefined,
) {
  await page.setViewportSize(size);
  await expect(groupTablist(page).getByRole('tab')).toHaveText(inventoryGroups);
  for (const group of inventoryGroups) {
    await groupTablist(page).getByRole('tab', { name: group, exact: true }).click();
    const rows = [groupTablist(page)];
    if (group !== 'Resumen') {
      await expect(sectionTablist(page, group).getByRole('tab')).toHaveText(
        inventorySectionsByGroup[group] ?? [],
      );
      rows.push(sectionTablist(page, group));
    }
    for (const row of rows) {
      const overflow = await row.evaluate((element) => element.scrollWidth - element.clientWidth);
      expect(overflow, `la navegación de ${group} se desborda a ${size.width}`).toBeLessThanOrEqual(
        0,
      );
      const tops = new Set<number>();
      for (const tab of await row.getByRole('tab').all()) {
        const box = await tab.boundingBox();
        expect(box?.height ?? 0, 'pestaña de al menos 44 px').toBeGreaterThanOrEqual(44);
        tops.add(Math.round(box?.y ?? 0));
      }
      expect(tops.size, `la fila de ${group} quedó en varias líneas a ${size.width}`).toBe(1);
    }
    const pageOverflow = await page.evaluate(
      () => document.documentElement.scrollWidth - window.innerWidth,
    );
    expect(pageOverflow, `${group} desborda la página a ${size.width}`).toBe(0);
    await capture(`navegacion-${group}`);
  }
}
