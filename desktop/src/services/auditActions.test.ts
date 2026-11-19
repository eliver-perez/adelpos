import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import { describe, expect, it } from 'vitest';
import { auditActionLabel, auditActionLabels, unknownAuditAction } from './auditActions';

// Etapa A3 (§1, decisión A3-D22 del coordinador): el Registro de movimientos nombra cada operación en
// palabras del restaurante. Esta comprobación lee las fuentes de Go como texto (backend/internal, sin
// las pruebas) y exige un nombre en auditActions.ts para cada acción de auditoría que Go escribe:
//
//   1. las cadenas con forma de acción («módulo.algo») dentro de las llamadas a funciones de
//      auditoría (audit, auditAs, auditPairing…, también las de los módulos que reciben el callback) y
//      a las funciones que solo pasan la acción a la auditoría (passThrough, abajo);
//   2. las que se asignan a una variable o un campo cuyo nombre lleva «action»;
//   3. las que escriben directamente las sentencias INSERT INTO app.audit_events;
//   4. las de los tipos de evento de una cuenta (operations: Go audita «restaurant.» + tipo) y las de
//      la historia de cocina (kitchen: «kitchen.» + tipo);
//   5. las que Go arma concatenando o pasa en una variable, declaradas en dynamicActions con sus
//      valores y su porqué. Una concatenación o una variable nueva en esos lugares hace fallar la prueba
//      hasta que se declare.
//
// Una acción que se escape de estas reglas no deja un código a la vista: la pantalla dice
// unknownAuditAction y el código queda en «Ver cambios».

const actionShape = /"([a-z][a-z_]*(?:\.[a-z0-9_-]+)+)"/g;

// Funciones que reciben la acción y la pasan a la auditoría sin llamarse audit*: el archivo y el inicio
// de la llamada.
const passThrough: { file: string; call: string; reason: string }[] = [
  {
    file: 'payments/adjustments.go',
    call: 'handler.finishAdjustment(',
    reason: 'audita la acción del ajuste (descuento, cargo o anulación)',
  },
  {
    file: 'identity/actions.go',
    call: 'handler.changed(',
    reason: 'audita y publica un cambio de la identidad documental',
  },
  {
    file: 'designer/templates.go',
    call: 'handler.activate(',
    reason: 'audita la activación de una plantilla',
  },
  {
    file: 'operations/areas.go',
    call: 'handler.place(',
    reason: 'audita el nuevo orden de áreas y mesas',
  },
  {
    file: 'inventory/counts.go',
    call: 'handler.countTransition(',
    reason: 'audita el paso de un conteo (enviar, aprobar)',
  },
];

// Llamadas cuyas acciones lleva Go con un prefijo: operations audita «restaurant.» + el tipo de evento de
// la cuenta (queries.go) y kitchen «kitchen.» + el tipo de su historia (context.go). argument es la
// posición del tipo entre los argumentos: si no es una cadena, la llamada se declara en dynamicActions.
const prefixed: { directory: string; call: string; prefix: string; argument: number }[] = [
  { directory: 'operations/', call: 'handler.event(', prefix: 'restaurant.', argument: 2 },
  { directory: 'kitchen/', call: 'handler.history(', prefix: 'kitchen.', argument: 0 },
];

// Recursos del catálogo que se dan de alta y se cambian por la ruta genérica (catalog/handler.go).
const catalogResources = [
  'categories',
  'products',
  'ingredients',
  'complements',
  'portions',
  'groups',
  'price-lists',
  'promotions',
];

// Acciones que Go arma concatenando, por archivo y expresión (sin espacios), con todos sus valores.
const dynamicActions: { file: string; expression: string; actions: string[]; reason: string }[] = [
  {
    file: 'administration/auth.go',
    expression: '"session."+reason',
    actions: ['session.replaced', 'session.password_changed'],
    reason:
      'issueSession audita con el motivo del cierre de las sesiones anteriores (auth.go y pin_login.go)',
  },
  {
    file: 'catalog/handler.go',
    expression: '"catalog."+resource+".created"',
    actions: catalogResources.map((resource) => `catalog.${resource}.created`),
    reason:
      'alta de un registro del catálogo, por recurso (writePermissions de catalog/handler.go)',
  },
  {
    file: 'catalog/handler.go',
    expression: '"catalog."+resource+".updated"',
    actions: catalogResources.map((resource) => `catalog.${resource}.updated`),
    reason: 'cambio de un registro del catálogo, por recurso',
  },
  {
    file: 'catalog/positions.go',
    expression: '"catalog."+request.resource+".reordered"',
    actions: ['catalog.categories.reordered', 'catalog.products.reordered'],
    reason: 'orden de categorías y productos (los recursos de positions.go)',
  },
  {
    file: 'operations/queries.go',
    expression: '"restaurant."+kind',
    actions: [],
    reason: 'los tipos de evento de la cuenta; los toma la regla 4 de handler.event(',
  },
  {
    file: 'operations/transfers.go',
    expression: 'kind',
    actions: ['restaurant.items.transferred', 'restaurant.account.split'],
    reason: 'pasar productos a otra cuenta o dividirla (kind de transferItems)',
  },
  {
    file: 'operations/accounts_write.go',
    expression: '"account."+action',
    actions: ['restaurant.account.move', 'restaurant.account.assign'],
    reason: 'mover y transferir una cuenta (accountAction, rutas move y assign)',
  },
  {
    file: 'kitchen/context.go',
    expression: '"kitchen."+kind',
    actions: [],
    reason: 'los tipos de la historia de cocina; los toma la regla 4 de handler.history(',
  },
  {
    file: 'kitchen/commercial.go',
    expression: 'input.History',
    actions: [],
    reason:
      'EnqueueRendered arma su service sin audit: solo escribe app.kitchen_history; la auditoría la hacen quienes lo llaman (payments, kitchen/rerender.go, designer/samples.go)',
  },
  {
    file: 'kitchen/queue_actions.go',
    expression: '"print."+action',
    actions: ['kitchen.print.retry', 'kitchen.print.resolve'],
    reason: 'reintentar y resolver un trabajo de impresión (rutas jobs/{id}/retry y resolve)',
  },
  {
    file: 'kitchen/releases.go',
    expression: '"release."+action',
    actions: ['kitchen.release.release', 'kitchen.release.schedule'],
    reason: 'liberar y programar un pedido (rutas releases/{id}/release y schedule)',
  },
  {
    file: 'kitchen/commercial.go',
    expression: '"print."+input.Kind+"_requested"',
    actions: [
      'kitchen.print.receipt_requested',
      'kitchen.print.precheck_requested',
      'kitchen.print.cash_report_requested',
      'kitchen.print.cash_voucher_requested',
    ],
    reason: 'impresión de un documento comercial (isCommercialKind de kitchen/commercial.go)',
  },
];

function goSources() {
  const root = join(__dirname, '../../../backend/internal');
  const files: { file: string; text: string }[] = [];
  const walk = (directory: string) => {
    for (const entry of readdirSync(directory)) {
      const path = join(directory, entry);
      if (statSync(path).isDirectory()) {
        if (entry !== 'testdata') {
          walk(path);
        }
      } else if (entry.endsWith('.go') && !entry.endsWith('_test.go')) {
        files.push({
          file: relative(root, path).split(sep).join('/'),
          text: readFileSync(path, 'utf8'),
        });
      }
    }
  };
  walk(root);
  return files;
}

// La llamada que empieza en start (justo después de su «(»), hasta su «)», sin contar los paréntesis
// dentro de cadenas.
function callSpan(text: string, start: number) {
  let depth = 1;
  let index = start;
  while (depth > 0 && index < text.length) {
    const char = text[index]!;
    if (char === '"' || char === '`') {
      const closing = text.indexOf(char, index + 1);
      index = closing < 0 ? text.length : closing + 1;
      continue;
    }
    if (char === '(') {
      depth += 1;
    } else if (char === ')') {
      depth -= 1;
    }
    index += 1;
  }
  return text.slice(start, index);
}

// Los argumentos de una llamada (su texto sin el «)» final), separados por las comas de primer nivel.
function callArguments(span: string) {
  const found: string[] = [];
  let depth = 0;
  let current = '';
  for (let index = 0; index < span.length - 1; index += 1) {
    const char = span[index]!;
    if (char === '"' || char === '`') {
      const closing = span.indexOf(char, index + 1);
      const end = closing < 0 ? span.length - 1 : closing + 1;
      current += span.slice(index, end);
      index = end - 1;
      continue;
    }
    if ('([{'.includes(char)) {
      depth += 1;
    } else if (')]}'.includes(char)) {
      depth -= 1;
    } else if (char === ',' && depth === 0) {
      found.push(current.trim());
      current = '';
      continue;
    }
    current += char;
  }
  found.push(current.trim());
  return found;
}

function lineOf(text: string, index: number) {
  return text.slice(0, index).split('\n').length;
}

// Las expresiones concatenadas con una cadena dentro de un texto (sin espacios), p. ej. "kitchen."+kind.
function concatenations(span: string) {
  const found: string[] = [];
  const pattern = /"[a-z_.]*"\s*\+\s*[\w.]+(?:\s*\+\s*"[a-z_.]*")?/g;
  for (const match of span.matchAll(pattern)) {
    found.push(match[0].replace(/\s+/g, ''));
  }
  return found;
}

interface Scan {
  actions: Map<string, string>;
  dynamic: { file: string; expression: string; line: number }[];
}

function scanGo(): Scan {
  const actions = new Map<string, string>();
  const dynamic: Scan['dynamic'] = [];
  const add = (action: string, where: string) => {
    if (!actions.has(action)) {
      actions.set(action, where);
    }
  };
  for (const { file, text } of goSources()) {
    // Regla 1: llamadas a funciones de auditoría y a las que solo pasan la acción.
    const calls = [/\b(?:[\w.]+\.)?audit\w*\(/g];
    for (const helper of passThrough.filter((entry) => entry.file === file)) {
      calls.push(
        new RegExp(
          helper.call.replace(/[.(]/g, (char) => `\\${char}`),
          'g',
        ),
      );
    }
    for (const call of calls) {
      for (const match of text.matchAll(call)) {
        const lineStart = text.lastIndexOf('\n', match.index) + 1;
        if (/^\s*(func|\/\/)/.test(text.slice(lineStart, match.index + 1))) {
          continue;
        }
        const span = callSpan(text, match.index + match[0].length);
        const where = `${file}:${lineOf(text, match.index)}`;
        for (const literal of span.matchAll(actionShape)) {
          add(literal[1]!, where);
        }
        for (const expression of concatenations(span)) {
          dynamic.push({ file, expression, line: lineOf(text, match.index) });
        }
      }
    }
    // Regla 2: asignaciones a una variable o un campo con «action» en el nombre.
    for (const match of text.matchAll(
      /\b\w*[aA]ction\w*\s*(?::=|=|:)\s*"([a-z][a-z_]*(?:\.[a-z0-9_-]+)+)"/g,
    )) {
      add(match[1]!, `${file}:${lineOf(text, match.index)}`);
    }
    for (const match of text.matchAll(/\b\w*[aA]ction\w*\s*(?::=|=)\s*("[a-z_.]*"\s*\+[^\n]*)/g)) {
      for (const expression of concatenations(match[1]!)) {
        dynamic.push({ file, expression, line: lineOf(text, match.index) });
      }
    }
    // Regla 3: las sentencias que escriben la auditoría directamente.
    for (const match of text.matchAll(/INSERT INTO app\.audit_events[^`]*/g)) {
      for (const literal of match[0].matchAll(/'([a-z][a-z_]*(?:\.[a-z0-9_-]+)+)'/g)) {
        add(literal[1]!, `${file}:${lineOf(text, match.index)}`);
      }
    }
    // Regla 4: tipos de evento de una cuenta y de la historia de cocina, con su prefijo.
    for (const family of prefixed.filter((entry) => file.startsWith(entry.directory))) {
      const call = new RegExp(
        family.call.replace(/[.(]/g, (char) => `\\${char}`),
        'g',
      );
      for (const match of text.matchAll(call)) {
        const lineStart = text.lastIndexOf('\n', match.index) + 1;
        if (/^\s*func/.test(text.slice(lineStart, match.index + 1))) {
          continue;
        }
        const span = callSpan(text, match.index + match[0].length);
        for (const literal of span.matchAll(actionShape)) {
          add(`${family.prefix}${literal[1]!}`, `${file}:${lineOf(text, match.index)}`);
        }
        for (const expression of concatenations(span)) {
          dynamic.push({ file, expression, line: lineOf(text, match.index) });
        }
        const kind = callArguments(span)[family.argument] ?? '';
        if (!kind.startsWith('"')) {
          dynamic.push({
            file,
            expression: kind.replace(/\s+/g, ''),
            line: lineOf(text, match.index),
          });
        }
      }
    }
  }
  for (const entry of dynamicActions) {
    for (const action of entry.actions) {
      add(action, `${entry.file} (${entry.expression})`);
    }
  }
  return { actions, dynamic };
}

describe('nombres de las operaciones del Registro de movimientos (etapa A3, §1)', () => {
  const scan = scanGo();

  it('encuentra las acciones de auditoría de Go', () => {
    // Si el recorrido dejara de encontrarlas (otra carpeta, otra forma de auditar), no probaría nada.
    expect(scan.actions.size).toBeGreaterThan(200);
    expect(scan.actions.has('session.login')).toBe(true);
    expect(scan.actions.has('restaurant.account.opened')).toBe(true);
    expect(scan.actions.has('kitchen.printer.saved')).toBe(true);
  });

  it('toda acción que escribe Go tiene un nombre en palabras del restaurante', () => {
    const missing = [...scan.actions.entries()]
      .filter(([action]) => !(action in auditActionLabels))
      .map(([action, where]) => `${action} (${where})`)
      .sort();
    expect(missing).toEqual([]);
  });

  it('las acciones que Go arma concatenando están declaradas con sus valores', () => {
    const declared = new Set(dynamicActions.map((entry) => `${entry.file} ${entry.expression}`));
    const undeclared = scan.dynamic
      .filter((entry) => !declared.has(`${entry.file} ${entry.expression}`))
      .map((entry) => `${entry.file}:${entry.line} ${entry.expression}`);
    expect(undeclared).toEqual([]);
    const found = new Set(scan.dynamic.map((entry) => `${entry.file} ${entry.expression}`));
    const stale = dynamicActions
      .filter((entry) => !found.has(`${entry.file} ${entry.expression}`))
      .map((entry) => `${entry.file} ${entry.expression} ya no está en Go`);
    expect(stale).toEqual([]);
  });

  it('un nombre nunca es el código y una acción desconocida se dice en general', () => {
    for (const [action, label] of Object.entries(auditActionLabels)) {
      expect(label, action).not.toMatch(/[a-z]+[._][a-z]+/);
      expect(label.length, action).toBeGreaterThan(3);
    }
    expect(auditActionLabel('otro.modulo.algo_nuevo')).toBe(unknownAuditAction);
    expect(auditActionLabel('session.login')).toBe(auditActionLabels['session.login']);
  });

  it('dos operaciones distintas no comparten nombre', () => {
    // En la columna «Operación» se distinguirían solo abriendo «Ver cambios».
    const byLabel = new Map<string, string[]>();
    for (const [action, label] of Object.entries(auditActionLabels)) {
      byLabel.set(label, [...(byLabel.get(label) ?? []), action]);
    }
    const shared = [...byLabel.entries()]
      .filter(([label, actions]) => actions.length > 1 || label === unknownAuditAction)
      .map(([label, actions]) => `${label}: ${actions.join(', ')}`);
    expect(shared).toEqual([]);
  });
});
