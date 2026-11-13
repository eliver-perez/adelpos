import type { StationScope, Task } from '../../services/kitchen';

// Reglas puras del modo cocina de Preparación (etapa 9F, §5.4). Go entrega los tiempos (segundos
// transcurridos y minutos de la estación) y la intersección de estaciones; aquí solo se decide cómo se
// presentan, para poder probarlo sin la pantalla.

export type TaskUrgency = 'normal' | 'warning' | 'late';

// Urgencia de una tarjeta contra el objetivo de su estación (decisión D7 del coordinador):
// - normal antes del aviso;
// - ámbar desde la advertencia de la estación o, si la advertencia no es menor que el objetivo, desde
//   el 75 % del objetivo;
// - rojo al pasar el objetivo.
export function taskUrgency(
  elapsedSeconds: number,
  warningMinutes: number,
  targetMinutes: number,
): TaskUrgency {
  const targetSeconds = targetMinutes * 60;
  if (elapsedSeconds > targetSeconds) {
    return 'late';
  }
  const warningSeconds =
    warningMinutes < targetMinutes ? warningMinutes * 60 : Math.floor(targetSeconds * 0.75);
  if (elapsedSeconds >= warningSeconds) {
    return 'warning';
  }
  return 'normal';
}

// Texto que acompaña al color (el color nunca es la única señal).
export const urgencyLabels: Record<TaskUrgency, string> = {
  normal: '',
  warning: 'Por vencer',
  late: 'Atrasada',
};

// Compara nombres sin distinguir mayúsculas, acentos ni espacios repetidos.
export function sameName(first: string, second: string) {
  return normalizedName(first) === normalizedName(second);
}

function normalizedName(value: string) {
  return value
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLocaleLowerCase('es')
    .replace(/\s+/g, ' ')
    .trim();
}

// Contexto de la tarjeta: cuenta y mesa sin repetir el nombre de la mesa cuando la cuenta se llama
// igual (la referencia neutra de la 6.2 es el nombre de la mesa: antes salía «Mesa 2 · Mesa 2»).
export function accountContext(accountName: string, tableName: string | null) {
  if (!tableName) {
    return `${accountName} · Sin mesa`;
  }
  if (sameName(accountName, tableName)) {
    return accountName;
  }
  return `${accountName} · ${tableName}`;
}

// Une nombres como en español: «A», «A y B», «A, B y C».
export function joinNames(names: readonly string[]) {
  if (names.length <= 1) {
    return names[0] ?? '';
  }
  return `${names.slice(0, -1).join(', ')} y ${names[names.length - 1]}`;
}

// Aviso cuando la terminal y el usuario no tienen estaciones en común (en lugar de una pantalla
// vacía). Go informa qué estaciones muestra la terminal y cuáles atiende el usuario.
export function noCommonStationsMessage(scope: StationScope) {
  const terminal = scope.terminal_all
    ? 'Esta terminal muestra todas las estaciones'
    : scope.terminal_stations.length === 0
      ? 'Esta terminal no tiene estaciones asignadas'
      : `Esta terminal muestra ${joinNames(scope.terminal_stations)}`;
  const user = scope.user_all
    ? 'tu usuario atiende todas'
    : scope.user_stations.length === 0
      ? 'tu usuario no tiene estaciones asignadas'
      : `tu usuario atiende ${joinNames(scope.user_stations)}`;
  return `${terminal}, y ${user}: no hay estaciones en común. Pide que ajusten la terminal o tu usuario.`;
}

// Partes que la tarjeta muestra a la vista: la parte de esta estación cuando no es solo el producto
// del título (un componente de combo que llegó a esta estación, una variante o una indicación).
export function visibleParts(task: Task) {
  return task.instructions.parts.filter(
    (part) =>
      part.path !== 'root' ||
      !!part.variant ||
      !!part.notes ||
      !sameName(part.name, task.instructions.name),
  );
}

// Tarea nueva para el aviso sonoro: un pedido recién liberado (released, de hace menos de dos
// minutos) cuya liberación no se había visto en esta pantalla. Las actualizaciones de tareas ya vistas,
// los cambios de estado y las tareas antiguas que reaparecen (otra página o filtro) no suenan.
const freshArrivalSeconds = 120;

export function newArrivals(seen: ReadonlySet<string>, tasks: readonly Task[]) {
  const arrivals = new Set<string>();
  for (const task of tasks) {
    if (
      task.state === 'released' &&
      task.elapsed_seconds < freshArrivalSeconds &&
      !seen.has(task.release_id)
    ) {
      arrivals.add(task.release_id);
    }
  }
  return arrivals;
}
