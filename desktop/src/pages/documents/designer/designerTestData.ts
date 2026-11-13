import type {
  CatalogBlock,
  CatalogKind,
  DocumentCatalog,
  DocumentPreview,
  PrinterCompatibility,
  TemplateBlock,
  TemplateDetail,
  TemplateLayout,
} from '../../../services/documents';

// Datos de prueba del editor (Vitest): un catálogo reducido con la forma del de Go, plantillas y
// vistas previas. Las líneas de la vista previa imitan la IR: el editor las muestra tal cual.

const common = ['visible', 'align', 'size', 'bold', 'uppercase', 'spacing_before', 'spacing_after'];

function definition(
  type: string,
  name: string,
  overrides: Partial<CatalogBlock> = {},
): CatalogBlock {
  return {
    type,
    name,
    group: 'common',
    description: `Descripción de ${name.toLowerCase()}.`,
    properties: common as CatalogBlock['properties'],
    options: [],
    required: false,
    protected: false,
    ...overrides,
  };
}

const sharedBlocks: CatalogBlock[] = [
  definition('logo', 'Logotipo', {
    properties: ['visible', 'spacing_before', 'spacing_after'],
    max: 3,
  }),
  definition('establishment_name', 'Nombre del establecimiento'),
  definition('fixed_text', 'Texto fijo', {
    options: [
      {
        key: 'text',
        label: 'Texto',
        type: 'text',
        default: '',
        max_length: 300,
        max_lines: 8,
        required: true,
      },
    ],
  }),
  definition('divider', 'Separador', {
    options: [
      {
        key: 'character',
        label: 'Carácter',
        type: 'enum',
        default: '-',
        values: [
          { value: '-', label: 'Guiones' },
          { value: '=', label: 'Doble línea' },
          { value: '*', label: 'Asteriscos' },
          { value: '.', label: 'Puntos' },
        ],
      },
    ],
  }),
  definition('spacer', 'Espacio', {
    properties: ['visible'],
    options: [
      { key: 'lines', label: 'Líneas en blanco', type: 'integer', default: 1, min: 1, max: 5 },
    ],
  }),
  definition('field', 'Campo de datos', {
    options: [
      {
        key: 'items',
        label: 'Datos',
        type: 'field_items',
        default: [],
        max_length: 40,
        min: 1,
        max: 4,
        required: true,
      },
      {
        key: 'layout',
        label: 'Disposición',
        type: 'enum',
        default: 'inline',
        values: [
          { value: 'inline', label: 'En línea, unidos con «·»' },
          { value: 'pair', label: 'Etiqueta a la izquierda y valor a la derecha (un solo dato)' },
        ],
      },
      {
        key: 'omit_empty',
        label: 'Omitir el renglón si no hay datos',
        type: 'boolean',
        default: true,
      },
    ],
  }),
];

export const receiptKind: CatalogKind = {
  kind: 'receipt',
  name: 'Ticket de pago',
  family: 'commercial',
  text_sizes: ['normal'],
  blocks: [
    ...sharedBlocks,
    definition('products', 'Lista de productos', { group: 'commercial', max: 1 }),
    definition('total', 'Total', { group: 'commercial', max: 1 }),
    definition('this_payment', 'Este pago', { group: 'commercial', max: 1 }),
    definition('fiscal_notice', 'Aviso fiscal', { max: 1, required: true }),
  ],
  field_keys: [
    { key: 'date', name: 'Fecha', default_label: 'Fecha:' },
    { key: 'folio', name: 'Folio', default_label: 'Folio:' },
    { key: 'cashier', name: 'Cajero', default_label: 'Atendió:' },
  ],
  shortcuts: [
    {
      name: 'Fecha',
      options: { items: [{ key: 'date', label: 'Fecha:' }], layout: 'inline', omit_empty: true },
    },
    {
      name: 'Cajero',
      options: {
        items: [{ key: 'cashier', label: 'Atendió:' }],
        layout: 'inline',
        omit_empty: true,
      },
    },
  ],
};

export const kitchenKind: CatalogKind = {
  kind: 'kitchen_order',
  name: 'Comanda',
  family: 'kitchen',
  text_sizes: ['inherit', 'normal', 'large'],
  blocks: [
    ...sharedBlocks,
    definition('kitchen_location', 'Ubicación', { group: 'kitchen' }),
    definition('kitchen_items', 'Lista de cocina', { group: 'kitchen', max: 1 }),
  ],
  field_keys: [{ key: 'table', name: 'Mesa', default_label: 'Mesa:' }],
  shortcuts: [
    {
      name: 'Mesa',
      options: { items: [{ key: 'table', label: 'Mesa:' }], layout: 'inline', omit_empty: true },
    },
  ],
};

export const catalog: DocumentCatalog = {
  kinds: [receiptKind, kitchenKind],
  paper_widths: [
    { paper_width: 58, columns: 30 },
    { paper_width: 80, columns: 42 },
  ],
  properties: {
    align: [
      { value: 'left', label: 'Izquierda' },
      { value: 'center', label: 'Centro' },
      { value: 'right', label: 'Derecha' },
    ],
    size: [
      { value: 'normal', label: 'Normal' },
      { value: 'double_height', label: 'Doble alto' },
      { value: 'double_width', label: 'Doble ancho' },
      { value: 'double', label: 'Doble' },
    ],
    spacing: { min: 0, max: 3 },
  },
  limits: {
    blocks: 60,
    bytes: 32768,
    fixed_text_chars: 300,
    fixed_text_lines: 8,
    label_chars: 40,
    field_items: 4,
    spacing: 3,
    spacer_lines: 5,
  },
};

export function block(
  id: string,
  type: string,
  options: Record<string, unknown> = {},
): TemplateBlock {
  return {
    id,
    type,
    visible: true,
    align: 'left',
    size: 'normal',
    bold: false,
    uppercase: false,
    spacing_before: 0,
    spacing_after: 0,
    options,
  };
}

export function receiptLayout(): TemplateLayout {
  return {
    options: { text_size: 'normal' },
    blocks: [
      block('name', 'establishment_name'),
      block('greeting', 'fixed_text', { text: 'Gracias' }),
      block('products', 'products'),
      block('total', 'total'),
      block('fiscal', 'fiscal_notice'),
    ],
  };
}

export function template(overrides: Partial<TemplateDetail> = {}): TemplateDetail {
  return {
    id: 't-own',
    kind: 'receipt',
    kind_name: 'Ticket de pago',
    name: 'Ticket de la casa',
    system: false,
    active: false,
    revision: 3,
    current_version: {
      id: 'v-2',
      number: 2,
      note: '',
      created_at: '2026-11-11T15:00:00Z',
      created_by_name: 'Administrador',
      block_count: 5,
    },
    created_at: '2026-11-10T15:00:00Z',
    created_by_name: 'Administrador',
    updated_at: '2026-11-11T15:00:00Z',
    updated_by_name: 'Administrador',
    layout: receiptLayout(),
    ...overrides,
  };
}

export function previewFor(layout: TemplateLayout, paperWidth: 58 | 80 = 80): DocumentPreview {
  return {
    kind: 'receipt',
    paper_width: paperWidth,
    columns: paperWidth === 58 ? 30 : 42,
    text_size: 'normal',
    sample: true,
    includes_expected: false,
    lines: layout.blocks
      .filter((item) => item.visible)
      .map((item) => ({
        text: `${item.id.toUpperCase()} (${paperWidth})`,
        bold: item.bold,
        size: item.size,
        block_id: item.id,
        image: null,
      })),
    text: '',
    warnings: [],
    empty_blocks: [],
  };
}

export const printers: PrinterCompatibility[] = [
  {
    id: 'p-agent',
    name: 'Destino controlado',
    transport: 'agent',
    paper_width: 80,
    columns: 42,
    default_columns: 42,
    format: {
      status: 'unsupported',
      code: 'agent',
      reason: 'El agente de impresión aún no existe.',
    },
    logo: { status: 'unsupported', code: 'agent', reason: 'El agente de impresión aún no existe.' },
  },
];
