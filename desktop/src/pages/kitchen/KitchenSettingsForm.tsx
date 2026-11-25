import { useState } from 'react';
import { Accordion } from '../../components/Accordion';
import { Field, Form, Notice } from '../../components/Forms';
import { Check, IntegerField } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import type { ExecuteKitchen, KitchenSettings } from '../../services/kitchen';
import { Select } from '../../components/Select';
import { kitchenDisplayCapability, serverHas } from '../../services/serverCapabilities';

// Límites de color de las pantallas de cocina (etapa K1, K1-D17): «atención» de 1 a 1440 minutos y «retraso» después
// de «atención», hasta 2880 (Go los valida igual).
export function displayLimitsProblem(warning: number, late: number): string {
  if (warning < 1 || warning > 1440 || late <= warning || late > 2880) {
    return 'El aviso de atención va de 1 a 1440 minutos y el de retraso debe ser mayor, hasta 2880.';
  }
  return '';
}

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
  const displayLimits =
    serverHas(kitchenDisplayCapability) && initial.display_warning_minutes !== undefined;
  const limitsProblem = displayLimits
    ? displayLimitsProblem(value.display_warning_minutes ?? 10, value.display_late_minutes ?? 20)
    : '';
  return (
    <section className="panel form-stack">
      <h2>Comandas, tiempos y programación</h2>
      <Notice>
        {initial.revision !== value.revision &&
          'La configuración cambió. Tus campos se conservaron; vuelve a abrir el editor para comparar antes de guardar.'}
      </Notice>
      <Form
        disabled={busy || initial.revision !== value.revision || limitsProblem !== ''}
        label="Guardar configuración de cocina"
        successMessage="Configuración de cocina guardada."
        onSave={async () => {
          const formats = can('kitchen.formats.manage');
          // Etapa K1: los límites de color de las pantallas viajan con los tiempos si el servidor los conoce.
          const limits =
            displayLimits && value.display_warning_minutes !== undefined
              ? {
                  display_warning_minutes: value.display_warning_minutes,
                  display_late_minutes: value.display_late_minutes,
                }
              : {};
          const body = formats
            ? { ...value }
            : {
                revision: value.revision,
                automatic_release: value.automatic_release,
                lead_minutes: value.lead_minutes,
                tracking: value.tracking,
                ...limits,
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
          summary="La programación se cumple aunque las terminales estén cerradas"
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
        {displayLimits && (
          <Accordion
            title="Colores de las pantallas de cocina"
            summary="Tiempo de espera para el color de atención y el de retraso"
            defaultOpen
          >
            <fieldset disabled={!can('kitchen.timing.manage')} className="form-stack">
              <IntegerField
                label="Color de atención desde (minutos de espera)"
                min={1}
                max={1440}
                value={value.display_warning_minutes ?? 10}
                onChange={(display_warning_minutes) =>
                  setValue({ ...value, display_warning_minutes })
                }
              />
              <IntegerField
                label="Color de retraso desde (minutos de espera)"
                min={2}
                max={2880}
                value={value.display_late_minutes ?? 20}
                onChange={(display_late_minutes) => setValue({ ...value, display_late_minutes })}
              />
              <Notice>{limitsProblem}</Notice>
              <p className="muted">
                Se aplican a todas las pantallas de cocina de las tabletas. El modo Cocina de esta
                aplicación sigue con los tiempos de cada estación.
              </p>
            </fieldset>
          </Accordion>
        )}
      </Form>
    </section>
  );
}
