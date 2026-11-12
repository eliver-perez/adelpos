import { BrandLogo } from '../components/BrandLogo';
import product from '../generated/product.json';
import { actionByID } from './actions';
import type { ActionID } from './actions';
import { CommandDialog } from './CommandDialog';
import { shortcutLabel } from './platform';
import type { Platform } from './platform';

// «Atajos de teclado» y «Acerca de AD-EL POS» (etapa 9C, fase C). Se abren desde Ayuda en el menú
// nativo y desde la paleta de comandos.

interface ShortcutRow {
  keys: string;
  label: string;
  detail?: string;
}

const groups: { title: string; actions: ActionID[] }[] = [
  {
    title: 'Ir a una sección',
    actions: ['view.tables', 'view.takeaway', 'view.kitchen', 'view.cash', 'view.administration'],
  },
  { title: 'Buscar', actions: ['view.palette', 'edit.find'] },
  { title: 'Cuentas', actions: ['file.table_account', 'file.print_precheck'] },
  { title: 'Ventana', actions: ['view.fullscreen'] },
];

function rowsOf(actions: ActionID[], platform: Platform): ShortcutRow[] {
  const rows: ShortcutRow[] = [];
  for (const id of actions) {
    const action = actionByID(id);
    if (!action?.shortcut) {
      continue;
    }
    rows.push({
      keys: shortcutLabel(action.shortcut, platform),
      label: action.label,
      detail: action.description,
    });
  }
  return rows;
}

function editingRows(platform: Platform): ShortcutRow[] {
  const mod = platform === 'mac' ? '⌘' : 'Ctrl+';
  return [
    { keys: `${mod}C · ${mod}V · ${mod}X`, label: 'Copiar, pegar y cortar en los campos' },
    {
      keys: platform === 'mac' ? '⌘Z · ⇧⌘Z' : 'Ctrl+Z · Ctrl+Y',
      label: 'Deshacer y rehacer en los campos',
    },
    { keys: `${mod}A`, label: 'Seleccionar todo el texto del campo' },
  ];
}

function ShortcutTable({ title, rows }: { title: string; rows: ShortcutRow[] }) {
  return (
    <table className="shortcuts-table">
      <caption>{title}</caption>
      <thead>
        <tr>
          <th scope="col">Atajo</th>
          <th scope="col">Acción</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={`${row.keys}-${row.label}`}>
            <td>
              <kbd>{row.keys}</kbd>
            </td>
            <td>
              <strong>{row.label}</strong>
              {row.detail && <span className="muted">{row.detail}</span>}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function ShortcutsDialog({
  platform,
  onClose,
}: {
  platform: Platform;
  onClose: () => void;
}) {
  return (
    <CommandDialog title="Atajos de teclado" className="shortcuts-dialog" onClose={onClose}>
      <p className="muted">
        {platform === 'mac'
          ? 'En esta terminal los atajos usan ⌘ (Comando).'
          : 'En esta terminal los atajos usan Ctrl.'}{' '}
        No actúan mientras escribes en un campo, salvo Esc. Cobrar, cancelar, cerrar el turno o
        devolver siempre piden su confirmación: los atajos solo abren la pantalla o la hoja.
      </p>
      <div className="shortcuts-grid">
        {groups.map((group) => (
          <ShortcutTable
            key={group.title}
            title={group.title}
            rows={rowsOf(group.actions, platform)}
          />
        ))}
        <ShortcutTable
          title="General"
          rows={[
            {
              keys: 'Esc',
              label: 'Cerrar o volver',
              detail:
                'Cierra el diálogo o panel abierto, o vuelve un nivel (de la captura al mapa, de una pantalla de Administración a la anterior). En el cobro conserva su función: limpia el importe.',
            },
            ...editingRows(platform),
          ]}
        />
      </div>
    </CommandDialog>
  );
}

export function AboutDialog({ onClose }: { onClose: () => void }) {
  return (
    <CommandDialog title="Acerca de AD-EL POS" className="about-dialog" onClose={onClose}>
      <div className="about-brand">
        <BrandLogo />
      </div>
      <p className="about-version">Versión {product.version}</p>
      <p className="muted">Punto de venta para restaurantes, optimizado para pantallas táctiles.</p>
      <p className="muted">
        Salir de esta aplicación no detiene el servidor del restaurante: las demás terminales siguen
        funcionando.
      </p>
    </CommandDialog>
  );
}
