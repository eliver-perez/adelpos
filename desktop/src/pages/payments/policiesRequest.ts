// «Configurar impresora» (etapa 9F, §4.1) abre Administración › Cobros con «Políticas de cobro» a la
// vista, donde se elige la impresora de tickets. La petición se deja aquí antes de navegar con el único
// requestScreen y la página de Cobros la toma una sola vez al abrirse.
let requested = false;

export function requestPaymentPolicies() {
  requested = true;
}

export function takePaymentPoliciesRequest() {
  const value = requested;
  requested = false;
  return value;
}
