import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import { Check, IntegerField } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { ExecuteKitchen, KitchenSettings } from '../../services/kitchen';

export function KitchenSettingsForm({
  can,
  execute,
  busy,
}: {
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
}) {
  const resource = useLiveResource<KitchenSettings>('kitchen_settings');
  if (!resource.data) return <Notice>{resource.error || 'Consultando configuración…'}</Notice>;
  return <SettingsEditor initial={resource.data} execute={execute} busy={busy} can={can} />;
}

function SettingsEditor({
  initial,
  can,
  execute,
  busy,
}: {
  initial: KitchenSettings;
  can: (key: string) => boolean;
  execute: ExecuteKitchen;
  busy: boolean;
}) {
  const [value, setValue] = useState(initial);
  return (
    <section className="panel form-stack">
      <h2>Comandas, tiempos y programación</h2>
      <Notice>
        {initial.revision !== value.revision &&
          'La configuración cambió. Tus campos se conservaron; vuelve a abrir el editor para comparar antes de guardar.'}
      </Notice>
      <Form
        disabled={busy || initial.revision !== value.revision}
        label="Guardar configuración de cocina"
        successMessage="Configuración de cocina guardada."
        onSave={async () => {
          const formats = can('kitchen.formats.manage');
          const body = formats
            ? { ...value }
            : {
                revision: value.revision,
                automatic_release: value.automatic_release,
                lead_minutes: value.lead_minutes,
                tracking: value.tracking,
              };
          setValue(
            await execute<KitchenSettings>(
              formats ? 'kitchen_save_settings' : 'kitchen_save_timing',
              { body },
            ),
          );
        }}
      >
        <Accordion
          title="Presentación y agrupación"
          summary="Tickets independientes o secciones por impresora"
          defaultOpen
        >
          <fieldset disabled={!can('kitchen.formats.manage')} className="form-stack">
            <Field label="Agrupación del establecimiento">
              <select
                value={value.grouping}
                onChange={(event) =>
                  setValue({
                    ...value,
                    grouping: event.target.value as KitchenSettings['grouping'],
                  })
                }
              >
                <option value="independent">Tickets independientes por estación</option>
                <option value="unified">Unificar estaciones de una misma impresora</option>
              </select>
            </Field>
            <Field label="Tamaño del texto">
              <select
                value={value.text_size}
                onChange={(event) =>
                  setValue({
                    ...value,
                    text_size: event.target.value as KitchenSettings['text_size'],
                  })
                }
              >
                <option value="normal">Normal</option>
                <option value="large">Grande, según controlador</option>
              </select>
            </Field>
            <Field label="Presentación de cantidades">
              <select
                value={value.quantity_style}
                onChange={(event) =>
                  setValue({
                    ...value,
                    quantity_style: event.target.value as KitchenSettings['quantity_style'],
                  })
                }
              >
                <option value="prefix">Cantidad antes del producto</option>
                <option value="multiplier">Producto seguido de cantidad</option>
              </select>
            </Field>
            <IntegerField
              label="Separación entre productos"
              min={1}
              max={3}
              value={value.line_spacing}
              onChange={(line_spacing) => setValue({ ...value, line_spacing })}
            />
            <Check
              label="Mostrar nombre del establecimiento"
              value={value.show_establishment}
              onChange={(show_establishment) => setValue({ ...value, show_establishment })}
            />
            <Check
              label="Mostrar observaciones de preparación"
              value={value.show_notes}
              onChange={(show_notes) => setValue({ ...value, show_notes })}
            />
            <Check
              label="Incluir importes comerciales en comandas"
              value={value.show_prices}
              onChange={(show_prices) => setValue({ ...value, show_prices })}
            />
          </fieldset>
        </Accordion>
        <Accordion
          title="Seguimiento y pedidos programados"
          summary="Go ejecuta la programación aunque Tauri esté cerrado"
          defaultOpen
        >
          <fieldset disabled={!can('kitchen.timing.manage')} className="form-stack">
            <Field label="Seguimiento predeterminado">
              <select
                value={value.tracking}
                onChange={(event) =>
                  setValue({
                    ...value,
                    tracking: event.target.value as KitchenSettings['tracking'],
                  })
                }
              >
                <option value="item">Por producto o partida</option>
                <option value="order">Por pedido completo</option>
              </select>
            </Field>
            <Check
              label="Liberar automáticamente los pedidos programados"
              value={value.automatic_release}
              onChange={(automatic_release) => setValue({ ...value, automatic_release })}
            />
            <IntegerField
              label="Anticipación de liberación en minutos"
              min={0}
              max={1440}
              value={value.lead_minutes}
              onChange={(lead_minutes) => setValue({ ...value, lead_minutes })}
            />
            <p className="muted">
              Esta política se captura al confirmar. Los pedidos existentes conservan su
              programación y pueden ajustarse expresamente desde Preparación.
            </p>
          </fieldset>
        </Accordion>
      </Form>
    </section>
  );
}
