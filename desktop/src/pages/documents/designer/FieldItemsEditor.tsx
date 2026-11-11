import { Icon } from '../../../components/Icon';
import type { FieldItem, FieldKeyInfo } from '../../../services/documents';

// Datos de un «Campo de datos»: de uno a cuatro datos del tipo de documento, cada uno con su
// etiqueta (por ejemplo «Mesa:»). Las claves y las etiquetas propuestas vienen del catálogo de Go;
// la disposición «etiqueta y valor» admite un solo dato.
export function FieldItemsEditor({
  items,
  keys,
  max,
  labelLimit,
  pair,
  disabled,
  onChange,
}: {
  items: FieldItem[];
  keys: FieldKeyInfo[];
  max: number;
  labelLimit: number;
  pair: boolean;
  disabled: boolean;
  onChange: (items: FieldItem[]) => void;
}) {
  const limit = pair ? 1 : max;

  function update(index: number, change: Partial<FieldItem>) {
    onChange(items.map((item, position) => (position === index ? { ...item, ...change } : item)));
  }

  function changeKey(index: number, key: string) {
    // Al cambiar el dato se propone su etiqueta habitual, que se puede editar después.
    const info = keys.find((entry) => entry.key === key);
    update(index, { key, label: info?.default_label ?? '' });
  }

  function add() {
    const used = new Set(items.map((item) => item.key));
    const next = keys.find((entry) => !used.has(entry.key)) ?? keys[0];
    if (!next) {
      return;
    }
    onChange([...items, { key: next.key, label: next.default_label }]);
  }

  return (
    <div className="designer-field-items" role="group" aria-label="Datos del campo">
      <span className="designer-control-label">Datos</span>
      {items.map((item, index) => (
        <div className="designer-field-item" key={index}>
          <label className="field">
            <span>Dato {index + 1}</span>
            <select
              value={item.key}
              disabled={disabled}
              onChange={(event) => changeKey(index, event.target.value)}
            >
              {keys.map((entry) => (
                <option key={entry.key} value={entry.key}>
                  {entry.name}
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>Etiqueta del dato {index + 1}</span>
            <input
              type="text"
              value={item.label}
              maxLength={labelLimit}
              disabled={disabled}
              placeholder="Sin etiqueta"
              onChange={(event) => update(index, { label: event.target.value })}
            />
          </label>
          <button
            type="button"
            className="icon-button designer-remove"
            aria-label={`Quitar el dato ${index + 1}`}
            disabled={disabled || items.length <= 1}
            onClick={() => onChange(items.filter((_, position) => position !== index))}
          >
            <Icon name="trash" />
          </button>
        </div>
      ))}
      {items.length < limit && (
        <button type="button" className="secondary-button" disabled={disabled} onClick={add}>
          Agregar otro dato
        </button>
      )}
      <p className="muted text-xs">
        Cada dato se imprime con su etiqueta seguida de un espacio y el valor; varios datos se unen
        con «·».
      </p>
    </div>
  );
}
