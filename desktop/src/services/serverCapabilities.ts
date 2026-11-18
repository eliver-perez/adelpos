// Capacidades que anuncia el servidor del restaurante en GET /api/v1/version (etapa A1, fase B). No es una regla de
// negocio: solo decide si la terminal muestra una pantalla que usa rutas que un servidor anterior no tiene. El
// escritorio 0.12.0 sigue funcionando con un servidor 0.11.0 (min_server_version no subió): lo del PIN de los meseros y
// el bloqueo por inactividad de los teléfonos y tabletas aparecen solo si el servidor anuncia «waiter-pin-v1».

// Entrada con PIN, credencial de envío y su administración (Personal › Usuarios, «Mi PIN» y Terminales).
export const waiterPINCapability = 'waiter-pin-v1';

let announced: ReadonlySet<string> = new Set();

// rememberServerCapabilities guarda lo que informó el servidor al conectar (useSession lo llama con la respuesta de
// «version»). Un valor que no es una lista de textos se trata como «sin capacidades».
export function rememberServerCapabilities(capabilities: unknown) {
  if (!Array.isArray(capabilities)) {
    announced = new Set();
    return;
  }
  announced = new Set(
    capabilities.filter((capability): capability is string => typeof capability === 'string'),
  );
}

// serverHas dice si el servidor anunció la capacidad.
export function serverHas(capability: string): boolean {
  return announced.has(capability);
}
