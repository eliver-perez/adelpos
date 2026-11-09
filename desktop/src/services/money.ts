// Formato único de importes en pantalla (etapa 7.1). Go entrega el dinero como cadenas decimales ya
// calculadas; aquí solo se agrega el símbolo, el separador de miles y los dos decimales operando
// sobre el texto. Nunca se convierte a number: una cadena con más dígitos de los que representa un
// number se muestra igual, sin redondeo.
//
// Reglas, idénticas a las de Go (backend/internal/moneytext), con la misma tabla de casos en las
// pruebas de ambos lenguajes:
// - separador de miles «,» y punto decimal;
// - al menos dos decimales; si Go entrega más, se conservan sin redondear (solo se quitan los ceros
//   finales que sobran después del segundo decimal);
// - «$» para pesos y «US$» para dólares;
// - el signo negativo va antes del símbolo («−$1,234.56»). En pantalla se usa el menos tipográfico
//   «−»; la impresión de Go usa el guion ASCII «-»;
// - un valor vacío o que no es un decimal válido se muestra como «—».

export type MoneyCurrency = 'MXN' | 'USD';

const emptyAmount = '—';
const screenMinus = '−';
const decimalPattern = /^(-?)(\d+)(?:\.(\d+))?$/;

interface ParsedAmount {
  negative: boolean;
  integer: string;
  fraction: string;
}

// Descompone el texto decimal sin interpretarlo como número. Rechaza signos «+», espacios,
// separadores de miles, notación científica y puntos sin dígitos a ambos lados.
function parseAmount(value: string | null | undefined): ParsedAmount | null {
  if (value === null || value === undefined) {
    return null;
  }
  const match = decimalPattern.exec(value);
  if (!match) {
    return null;
  }
  const integer = (match[2] ?? '').replace(/^0+(?=\d)/, '');
  const fraction = normalizeFraction(match[3] ?? '');
  return {
    negative: match[1] === '-' && !isZero(integer, fraction),
    integer,
    fraction,
  };
}

// Completa a dos decimales; si hay más, solo quita los ceros finales a partir del tercero.
function normalizeFraction(fraction: string) {
  if (fraction.length < 2) {
    return fraction.padEnd(2, '0');
  }
  let end = fraction.length;
  while (end > 2 && fraction[end - 1] === '0') {
    end -= 1;
  }
  return fraction.slice(0, end);
}

function isZero(integer: string, fraction: string) {
  return /^0+$/.test(integer) && /^0+$/.test(fraction);
}

function groupThousands(integer: string) {
  return integer.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

function currencySymbol(currency: MoneyCurrency) {
  if (currency === 'USD') {
    return 'US$';
  }
  return '$';
}

function compose(amount: ParsedAmount, currency: MoneyCurrency, fraction: string, sign: string) {
  const decimals = fraction === '' ? '' : `.${fraction}`;
  return `${sign}${currencySymbol(currency)}${groupThousands(amount.integer)}${decimals}`;
}

// Importe con símbolo, miles y dos decimales: «$1,234.56», «−US$20.00», «—».
export function formatMoney(value: string | null | undefined, currency: MoneyCurrency = 'MXN') {
  const amount = parseAmount(value);
  if (!amount) {
    return emptyAmount;
  }
  return compose(amount, currency, amount.fraction, amount.negative ? screenMinus : '');
}

// Importe en dólares; equivale a formatMoney(value, 'USD').
export function formatDollars(value: string | null | undefined) {
  return formatMoney(value, 'USD');
}

// Importe con signo explícito para ajustes y diferencias: «+$50.00», «−$50.00» y «$0.00».
export function formatSignedMoney(
  value: string | null | undefined,
  currency: MoneyCurrency = 'MXN',
) {
  const amount = parseAmount(value);
  if (!amount) {
    return emptyAmount;
  }
  let sign = '+';
  if (amount.negative) {
    sign = screenMinus;
  } else if (isZero(amount.integer, amount.fraction)) {
    sign = '';
  }
  return compose(amount, currency, amount.fraction, sign);
}

// Importe que se resta (descuentos, cortesías, promociones) cuando Go lo entrega en positivo:
// «−$50.00». Invertir el signo es solo presentación sobre el texto, sin aritmética; el cero se
// muestra sin signo.
export function formatDeduction(value: string | null | undefined, currency: MoneyCurrency = 'MXN') {
  const amount = parseAmount(value);
  if (!amount) {
    return emptyAmount;
  }
  let sign = screenMinus;
  if (isZero(amount.integer, amount.fraction)) {
    sign = '';
  } else if (amount.negative) {
    sign = '+';
  }
  return compose(amount, currency, amount.fraction, sign);
}

// Valor de un billete o una moneda: sin decimales cuando es entero («$1,000», «US$100») y con dos
// cuando no lo es («$0.50»).
export function formatDenomination(
  value: string | null | undefined,
  currency: MoneyCurrency = 'MXN',
) {
  const amount = parseAmount(value);
  if (!amount) {
    return emptyAmount;
  }
  const fraction = /^0+$/.test(amount.fraction) ? '' : amount.fraction;
  return compose(amount, currency, fraction, amount.negative ? screenMinus : '');
}
