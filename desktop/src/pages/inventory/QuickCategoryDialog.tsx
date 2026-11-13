import { useId, useLayoutEffect, useRef, useState } from 'react';
import type { FormEvent, KeyboardEvent } from 'react';
import { trapFocus } from '../../components/ConfirmDialog';
import { Field } from '../../components/Forms';
import { createInventoryCategory } from '../../services/inventory';
import type { InventoryCategory, InventoryExecute } from '../../services/inventory';
import { notifyError, notifySuccess } from '../../services/notify';

// Nombre comparable de una categoría: Go la rechaza repetida sin distinguir mayúsculas.
function comparable(name: string) {
  return name.trim().toLocaleLowerCase('es-MX');
}

// Alta rápida de una categoría desde el editor de un insumo (etapa 9A, §3.5). Es un diálogo propio
// que se monta fuera del formulario del insumo (no hay formularios anidados): al guardarse, el
// editor agrega la categoría a su lista y la selecciona sin perder nada de lo capturado. Go valida
// el nombre y la unicidad (DUPLICATE_NAME); la pantalla solo avisa antes si ya está en la lista.
export function QuickCategoryDialog({
  categories,
  execute,
  busy,
  onCreated,
  onCancel,
}: {
  categories: InventoryCategory[];
  execute: InventoryExecute;
  busy: boolean;
  onCreated: (category: InventoryCategory) => void;
  onCancel: () => void;
}) {
  const titleID = useId();
  const panel = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const [name, setName] = useState('');
  const [saving, setSaving] = useState(false);
  const existing = categories.find((category) => comparable(category.name) === comparable(name));

  // El foco entra antes de pintar (etapa 9E, como «Acerca de» en la 9D): con un efecto normal,
  // un Escape pulsado justo al aparecer el diálogo llegaba fuera de él y no lo cerraba.
  useLayoutEffect(() => {
    // El foco va al nombre; al cerrar vuelve al selector de categoría.
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    input.current?.focus();
    return () => {
      if (previous && previous.isConnected) {
        previous.focus();
      }
    };
  }, []);

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      onCancel();
      return;
    }
    if (event.key === 'Tab' && panel.current) {
      trapFocus(event, panel.current);
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (existing || name.trim() === '') {
      return;
    }
    setSaving(true);
    try {
      const created = await createInventoryCategory(execute, { name: name.trim(), active: true });
      notifySuccess(`Categoría «${created.name}» creada y seleccionada.`);
      onCreated(created);
    } catch (error) {
      notifyError(error, 'No se pudo crear la categoría.');
      setSaving(false);
    }
  }

  let hint = 'Se agrega al final de la lista y queda activa.';
  if (existing) {
    hint = existing.active
      ? `Ya existe la categoría «${existing.name}»: elígela en la lista.`
      : `Ya existe la categoría «${existing.name}», desactivada: actívala en Configuración → Categorías.`;
  }
  const locked = busy || saving;
  return (
    <div className="dialog-backdrop">
      <div
        ref={panel}
        className="dialog-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        onKeyDown={handleKeyDown}
      >
        <div className="dialog-heading">
          <h2 id={titleID}>Nueva categoría de insumos</h2>
        </div>
        <form className="form-stack" aria-label="Nueva categoría de insumos" onSubmit={submit}>
          <p className="dialog-message">
            Lo que llevas capturado del insumo se conserva; al crearla queda seleccionada.
          </p>
          <Field label="Nombre de la categoría" hint={hint}>
            <input
              ref={input}
              required
              maxLength={60}
              value={name}
              aria-invalid={existing ? true : undefined}
              onChange={(event) => setName(event.target.value)}
            />
          </Field>
          <div className="dialog-actions">
            <button type="button" className="secondary-button dialog-button" onClick={onCancel}>
              Cancelar
            </button>
            <button
              type="submit"
              className="primary-button dialog-button"
              disabled={locked || existing !== undefined || name.trim() === ''}
            >
              {saving ? 'Creando…' : 'Crear y seleccionar'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
