import { formatMoney } from '../../services/money';
import { serverDateTime } from '../../services/restaurant';
import type { ColumnType, ReportValue } from '../../services/reports';

// Presentación de los valores tipados que entrega Go en los reportes (etapa 9A). Todo opera sobre
// el texto o el entero recibido: React no suma, resta, redondea ni convierte importes. Los importes
// usan services/money.ts; las cantidades y porcentajes conservan los dígitos de Go.

const emptyValue = '—';
const screenMinus = '−';
const decimalPattern = /^(-?)(\d+)(?:\.(\d+))?$/;

export const numericTypes: ColumnType[] = [
  'integer',
  'quantity',
  'money_mxn',
  'money_usd',
  'percent',
];

export function isNumericType(type: ColumnType) {
  return numericTypes.includes(type);
}

export function isMoneyType(type: ColumnType) {
  return type === 'money_mxn' || type === 'money_usd';
}

function groupThousands(integer: string) {
  return integer.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

// Decimal exacto con separador de miles y el menos tipográfico, sin tocar los decimales.
export function formatDecimalText(value: string | number) {
  const match = decimalPattern.exec(String(value));
  if (!match) {
    return emptyValue;
  }
  const integer = groupThousands((match[2] ?? '').replace(/^0+(?=\d)/, ''));
  const fraction = match[3] ? `.${match[3]}` : '';
  const zero = /^0+$/.test(match[2] ?? '') && /^0*$/.test(match[3] ?? '');
  const sign = match[1] === '-' && !zero ? screenMinus : '';
  return `${sign}${integer}${fraction}`;
}

// Porcentaje de Go («13.57») como «13.57 %».
export function formatPercent(value: ReportValue) {
  if (value === null || value === undefined || value === '') {
    return emptyValue;
  }
  const text = formatDecimalText(value);
  return text === emptyValue ? emptyValue : `${text} %`;
}

// Variación porcentual con signo explícito: «+13.57 %», «−4.20 %», «0.00 %».
export function formatSignedPercent(value: string | null) {
  if (value === null || value === '') {
    return emptyValue;
  }
  const text = formatDecimalText(value);
  if (text === emptyValue) {
    return emptyValue;
  }
  if (text.startsWith(screenMinus) || /^[0.,]+$/.test(text)) {
    return `${text} %`;
  }
  return `+${text} %`;
}

// Fecha comercial AAAA-MM-DD como dd/mm/aaaa (la fecha ya es del día comercial; no hay zona).
export function formatBusinessDate(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) {
    return value;
  }
  return `${match[3]}/${match[2]}/${match[1]}`;
}

// Valor de una celda, una cifra o un indicador según el tipo de su columna.
export function formatReportValue(type: ColumnType, value: ReportValue, timezone: string) {
  if (value === null || value === undefined || value === '') {
    return emptyValue;
  }
  switch (type) {
    case 'money_mxn':
      return formatMoney(String(value));
    case 'money_usd':
      return formatMoney(String(value), 'USD');
    case 'percent':
      return formatPercent(value);
    case 'integer':
    case 'quantity':
      return formatDecimalText(value);
    case 'date':
      return formatBusinessDate(String(value));
    case 'datetime':
      return serverDateTime(String(value), timezone);
    default:
      return String(value);
  }
}

// Variación con signo de un indicador (importe, entero o porcentaje).
export function formatSignedValue(type: ColumnType, value: ReportValue) {
  if (value === null || value === undefined || value === '') {
    return emptyValue;
  }
  if (type === 'percent') {
    return formatSignedPercent(String(value));
  }
  const text = isMoneyType(type)
    ? formatMoney(String(value), type === 'money_usd' ? 'USD' : 'MXN')
    : formatDecimalText(value);
  if (text === emptyValue || text.startsWith(screenMinus) || /^[$US0.,]+$/.test(text)) {
    return text;
  }
  return `+${text}`;
}

// Signo de un valor decimal de Go sin convertirlo: 1, −1 o 0.
export function decimalSign(value: ReportValue): -1 | 0 | 1 {
  if (value === null || value === undefined) {
    return 0;
  }
  const match = decimalPattern.exec(String(value));
  if (!match || (/^0+$/.test(match[2] ?? '') && /^0*$/.test(match[3] ?? ''))) {
    return 0;
  }
  return match[1] === '-' ? -1 : 1;
}

// Número solo para la geometría de un gráfico (posición y longitud de las marcas). Ningún texto
// visible sale de este número: los rótulos usan el valor de Go con formatReportValue.
export function geometryNumber(value: ReportValue): number | null {
  if (value === null || value === undefined || value === '') {
    return null;
  }
  const number = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(number) ? number : null;
}

// Estados que Go entrega como texto en algunas columnas (costos, turnos, cuentas): se muestran como
// etiquetas con su tono, siempre con su texto.
const statusTones: Record<string, 'good' | 'warning' | 'info' | 'muted'> = {
  'Sin costo': 'warning',
  Estimado: 'info',
  Firme: 'good',
  'En arqueo': 'warning',
  Reabierta: 'info',
  'Pagada sin entregar': 'info',
  'Datos incompletos': 'warning',
};

export function statusTone(value: ReportValue) {
  if (typeof value !== 'string') {
    return undefined;
  }
  return statusTones[value];
}
