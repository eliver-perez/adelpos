import type { InventoryUnit, UnitReference } from '../../services/inventory';
import type {
  Cancellation,
  ConsumptionDiagnostic,
  ConsumptionDiagnosticCode,
  ConsumptionRequest,
  PortionSource,
  Recipe,
  RecipeTargetKind,
} from '../../services/inventoryRecipes';

// Textos de la fase B de Inventarios (recetas, cancelaciones y diagnóstico del consumo). Solo
// traducen claves cerradas de Go a español y arman referencias legibles; ninguna función opera con
// cantidades ni dinero.

export const targetKindLabels: Record<RecipeTargetKind, string> = {
  product: 'Producto',
  variant: 'Variante',
  complement: 'Complemento',
};

export const portionSourceLabels: Record<PortionSource, string> = {
  recipe: 'Renglón de esta receta',
  habitual: 'Porción habitual',
  none: 'Sin porción: no consume',
};

export const serviceModeLabels: Record<'dine_in' | 'takeaway', string> = {
  dine_in: 'Salón',
  takeaway: 'Para llevar',
};

// Diagnósticos de la resolución del consumo explicados sin datos técnicos. Go nunca inventa una
// cantidad: cuando falta configuración, la parte afectada no consume y queda este aviso.
const diagnosticLabels: Record<ConsumptionDiagnosticCode, string> = {
  no_configuration: 'Sin receta ni porciones configuradas: no consumió inventario.',
  ingredient_without_portion: 'Un ingrediente del catálogo no tiene porción habitual: no consumió.',
  complement_without_configuration:
    'Un complemento no tiene receta ni producto o ingrediente con porción: no consumió.',
  preparation_unavailable: 'Una preparación de la receta está desactivada: no consumió.',
  depth_exceeded: 'La receta tiene más de 8 niveles de preparaciones: se detuvo el cálculo.',
  preparation_cycle: 'Una preparación se usa a sí misma: se detuvo el cálculo.',
  unit_not_consumed: 'La partida no había consumido inventario: el cambio no tiene efecto.',
  nothing_consumed: 'La partida no tenía consumo neto: no queda nada por clasificar.',
  unit_reversed: 'El consumo de la partida ya estaba revertido: el cambio no tiene efecto.',
  unit_already_registered: 'El consumo de la partida ya estaba registrado: no se duplicó.',
};

// Parte de la partida a la que se refiere un diagnóstico: «root» es el producto; «root/0» el
// primer componente de un combo, «root/0/1» el segundo componente de ese componente.
function partLabel(path: string) {
  if (path === '' || path === 'root') {
    return '';
  }
  const positions = path
    .split('/')
    .slice(1)
    .map((segment) => (/^\d+$/.test(segment) ? String(Number(segment) + 1) : segment));
  return `Componente ${positions.join('.')} del combo: `;
}

export function diagnosticText(diagnostic: ConsumptionDiagnostic) {
  const text = diagnosticLabels[diagnostic.code] ?? 'Aviso del cálculo del consumo.';
  return `${partLabel(diagnostic.path)}${text}`;
}

// Diagnósticos distintos de una solicitud (el mismo aviso en varias partes se muestra una vez por
// parte, nunca con identificadores).
export function diagnosticTexts(diagnostics: ConsumptionDiagnostic[]) {
  return Array.from(new Set(diagnostics.map(diagnosticText)));
}

// Nombres neutros que Go asigna a una cuenta sin nombre capturado (ver services/restaurant.ts).
const neutralAccountNames = ['Cuenta', 'Pedido para llevar', 'Para llevar'];
const takeawayAccountNames = ['Pedido para llevar', 'Para llevar'];

// Cuenta o mesa legible de una cancelación: «Cuenta #12 · Mesa 4», «Pedido #7» o «Cuenta #3».
export function cancellationAccount(
  cancellation: Pick<Cancellation, 'account_folio' | 'account_name'>,
) {
  const name = cancellation.account_name?.trim() ?? '';
  const folio = cancellation.account_folio;
  const takeaway = takeawayAccountNames.includes(name);
  const prefix = takeaway ? 'Pedido' : 'Cuenta';
  const reference = folio === null ? prefix : `${prefix} #${folio}`;
  if (name === '' || neutralAccountNames.includes(name)) {
    return reference;
  }
  return `${reference} · ${name}`;
}

// Cuenta de una solicitud de consumo: el folio con la modalidad congelada al liberar.
export function requestAccount(
  request: Pick<ConsumptionRequest, 'account_folio' | 'service_mode'>,
) {
  const prefix = request.service_mode === 'takeaway' ? 'Pedido' : 'Cuenta';
  return request.account_folio === null ? prefix : `${prefix} #${request.account_folio}`;
}

// Día operativo AAAA-MM-DD mostrado como dd/mm/aaaa (solo reordena el texto).
export function operatingDateText(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) {
    return value;
  }
  return `${match[3]}/${match[2]}/${match[1]}`;
}

// Unidad base en que Go expresa el rendimiento y los renglones de una receta: la del insumo
// preparado (almacenable) o la unidad del sistema de equivalencia 1 de la dimensión del
// rendimiento (inmediata, producto y complemento). Es una búsqueda en la lista de unidades, sin
// conversiones.
export function recipeBaseUnit(recipe: Recipe, units: InventoryUnit[]): UnitReference | null {
  if (recipe.preparation_item) {
    return recipe.preparation_item.unit;
  }
  const dimension = recipe.version.yield_unit.dimension;
  const base = units.find(
    (unit) => unit.system && unit.dimension === dimension && unit.factor === '1',
  );
  if (!base) {
    return null;
  }
  return { id: base.id, code: base.code, name: base.name, dimension: base.dimension };
}
