import { act, fireEvent, screen, within } from '@testing-library/react';
import { inventoryGroups, inventorySections } from './inventorySections';

// Ayudantes de prueba para la navegación agrupada de Inventarios (etapa 9A): las pruebas abren una
// sección por su etiqueta y el ayudante elige primero su grupo y después la sección en la segunda
// fila (si el grupo tiene más de una sección permitida).

function groupsTablist() {
  return screen.getByRole('tablist', { name: 'Secciones de inventario' });
}

function sectionsTablist(groupLabel: string) {
  return screen.queryByRole('tablist', { name: `Apartados de ${groupLabel}` });
}

function sectionGroupLabel(label: string) {
  const section = inventorySections.find((entry) => entry.label === label);
  const group = section ? inventoryGroups.find((entry) => entry.id === section.group) : undefined;
  if (!group) {
    throw new Error(`Sección de inventario desconocida: ${label}`);
  }
  return group.label;
}

export async function openInventorySection(label: string) {
  const groupLabel = sectionGroupLabel(label);
  const groups = await screen.findByRole('tablist', { name: 'Secciones de inventario' });
  await act(async () => {
    fireEvent.click(within(groups).getByRole('tab', { name: groupLabel }));
  });
  const sections = sectionsTablist(groupLabel);
  if (sections) {
    await act(async () => {
      fireEvent.click(within(sections).getByRole('tab', { name: label }));
    });
  }
}

// Pestaña de la segunda fila (sección) con la etiqueta indicada, en el grupo abierto.
export function sectionTab(label: string) {
  const sections = sectionsTablist(sectionGroupLabel(label));
  if (!sections) {
    throw new Error(`El grupo de «${label}» no muestra la segunda fila`);
  }
  return within(sections).getByRole('tab', { name: label });
}

export function groupTabNames() {
  return within(groupsTablist())
    .getAllByRole('tab')
    .map((tab) => tab.textContent);
}

// Mapa grupo → secciones visibles (vacío si el grupo no muestra la segunda fila), recorriendo cada
// grupo como lo haría el usuario.
export function navigationMap() {
  const result: Record<string, string[]> = {};
  for (const tab of within(groupsTablist()).getAllByRole('tab')) {
    const label = tab.textContent ?? '';
    act(() => {
      fireEvent.click(tab);
    });
    const sections = sectionsTablist(label);
    result[label] = sections
      ? within(sections)
          .getAllByRole('tab')
          .map((entry) => entry.textContent ?? '')
      : [];
  }
  return result;
}
