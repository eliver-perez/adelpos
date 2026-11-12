import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import { Check, IntegerField } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { ExecuteKitchen, KitchenSettings } from '../../services/kitchen';
import { Select } from '../../components/Select';

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
              <Select<KitchenSettings['grouping']>
                value={value.grouping}
                options={[
                  { value: 'independent', label: 'Tickets independientes por estación' },
                  { value: 'unified', label: 'Unificar estaciones de una misma impresora' },
                ]}
                onChange={(grouping) => setValue({ ...value, grouping })}
              />
            </Field>
            <Field label="Tamaño del texto">
              <Select<KitchenSettings['text_size']>
                value={value.text_size}
                options={[
                  { value: 'normal', label: 'Normal' },
                  { value: 'large', label: 'Grande, según controlador' },
                ]}
                onChange={(text_size) => setValue({ ...value, text_size })}
              />
            </Field>
            <Field label="Presentación de cantidades">
              <Select<KitchenSettings['quantity_style']>
                value={value.quantity_style}
                options={[
                  { value: 'prefix', label: 'Cantidad antes del producto' },
                  { value: 'multiplier', label: 'Producto seguido de cantidad' },
                ]}
                onChange={(quantity_style) => setValue({ ...value, quantity_style })}
              />
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
              <Select<KitchenSettings['tracking']>
                value={value.tracking}
                options={[
                  { value: 'item', label: 'Por producto o partida' },
                  { value: 'order', label: 'Por pedido completo' },
                ]}
                onChange={(tracking) => setValue({ ...value, tracking })}
              />
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
