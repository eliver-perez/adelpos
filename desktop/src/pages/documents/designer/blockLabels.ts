import type {
  CatalogBlock,
  CatalogKind,
  DocumentFamily,
  DocumentKind,
  TemplateBlock,
} from '../../../services/documents';
import { fieldItems } from './draft';

// Presentación del catálogo en el editor: agrupación del panel de elementos y descripción breve de
// cada bloque en la estructura. Solo organiza lo que entrega Go; ningún dato impreso sale de aquí.

export type PaletteGroupID = 'header' | 'data' | 'content' | 'totals' | 'format';

export const paletteGroups: { id: PaletteGroupID; label: string }[] = [
  { id: 'header', label: 'Encabezado' },
  { id: 'data', label: 'Datos del documento' },
  { id: 'content', label: 'Contenido del documento' },
  { id: 'totals', label: 'Totales y pagos' },
  { id: 'format', label: 'Texto y formato' },
];

const headerBlocks = [
  'logo',
  'establishment_name',
  'establishment_address',
  'establishment_phone',
  'establishment_email',
  'establishment_extra',
  'header_text',
  'document_title',
];

const formatBlocks = ['fixed_text', 'divider', 'spacer', 'footer_text', 'fiscal_notice'];

const totalBlocks = [
  'taxes',
  'total',
  'usd_total',
  'this_payment',
  'payments',
  'balance',
  'shift_totals',
  'expected_differences',
];

// Grupo del panel de elementos para un tipo de bloque. Los que no se reconocen van a «Contenido».
export function paletteGroupOf(block: CatalogBlock): PaletteGroupID {
  if (block.type === 'field') {
    return 'data';
  }
  if (headerBlocks.includes(block.type)) {
    return 'header';
  }
  if (formatBlocks.includes(block.type)) {
    return 'format';
  }
  if (totalBlocks.includes(block.type)) {
    return 'totals';
  }
  return 'content';
}

// Los diez tipos agrupados como se trabajan en el restaurante.
export const familyGroups: { family: DocumentFamily; label: string; kinds: DocumentKind[] }[] = [
  { family: 'commercial', label: 'Venta', kinds: ['receipt', 'precheck'] },
  {
    family: 'kitchen',
    label: 'Cocina',
    kinds: ['kitchen_order', 'kitchen_modification', 'kitchen_cancellation'],
  },
  {
    family: 'cash',
    label: 'Caja',
    kinds: ['cash_income', 'cash_withdrawal', 'cash_expense', 'cash_partial_cut', 'cash_final_cut'],
  },
];

export function catalogBlock(kind: CatalogKind | undefined, type: string) {
  return kind?.blocks.find((block) => block.type === type);
}

function firstLine(text: string) {
  const line = text.split('\n')[0] ?? '';
  return line.length > 40 ? `${line.slice(0, 40)}…` : line;
}

const dividerNames: Record<string, string> = {
  '-': 'guiones',
  '=': 'doble línea',
  '*': 'asteriscos',
  '.': 'puntos',
};

// Nombre visible de un bloque en la estructura: el del catálogo y, si ayuda a distinguirlo, un
// resumen de sus opciones («Campo de datos · Fecha · Hora», «Texto fijo · Gracias…»).
export function blockTitle(kind: CatalogKind | undefined, block: TemplateBlock) {
  return catalogBlock(kind, block.type)?.name ?? 'Elemento';
}

export function blockSummary(kind: CatalogKind | undefined, block: TemplateBlock) {
  const options = block.options;
  if (block.type === 'field') {
    const names = fieldItems(block).map((item) => {
      const info = kind?.field_keys.find((key) => key.key === item.key);
      return info?.name ?? 'Dato';
    });
    return names.join(' · ');
  }
  if (block.type === 'fixed_text' && typeof options.text === 'string') {
    return `«${firstLine(options.text)}»`;
  }
  if (block.type === 'document_title' && typeof options.text === 'string' && options.text) {
    return `«${firstLine(options.text)}»`;
  }
  if (block.type === 'divider' && typeof options.character === 'string') {
    return `Con ${dividerNames[options.character] ?? 'guiones'}`;
  }
  if (block.type === 'spacer' && typeof options.lines === 'number') {
    return options.lines === 1 ? '1 línea en blanco' : `${options.lines} líneas en blanco`;
  }
  return '';
}

// Descripción completa para lectores de pantalla y para el título del panel de propiedades.
export function blockLabel(kind: CatalogKind | undefined, block: TemplateBlock) {
  const summary = blockSummary(kind, block);
  const title = blockTitle(kind, block);
  return summary ? `${title} · ${summary}` : title;
}
