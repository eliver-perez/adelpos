import { useState } from 'react';
import { useConfirmDialog } from '../../components/ConfirmDialog';
import { Field, Form } from '../../components/Forms';
import { notifyError, notifySuccess } from '../../services/notify';
import { uncertainError } from './capture/draftSync';
import { Accordion } from '../../components/Accordion';
import {
  Check,
  IntegerField,
  ReferenceField,
  TextField,
} from '../../components/catalog/EditorFields';
import { TableMap } from '../../components/restaurant/TableMap';
import { PositionField } from '../../components/PositionField';
import { positionPayload } from '../../services/positions';
import type { Operation } from '../../services/api';
import type { Position } from '../../services/catalog';
import type {
  Area,
  Overview,
  RestaurantTable,
  RestaurantSettings,
} from '../../services/restaurant';
import { Select } from '../../components/Select';

// Borrador local del área: la posición elegida solo viaja al guardar.
type AreaDraft = Area & { position?: Position };

// Alta continua de mesas: si el nombre termina en número propone el siguiente («Mesa 4» →
// «Mesa 5», «M09» → «M10») sin repetir un nombre ya usado en el área; si no, deja el nombre
// vacío.
export function nextTableName(name: string, taken: string[]): string {
  const match = /^(.*?)([0-9]+)$/.exec(name.trim());
  if (!match) {
    return '';
  }
  const prefix = match[1] ?? '';
  const digits = match[2] ?? '';
  const used = new Set(taken.map((entry) => entry.trim().toLowerCase()));
  let number = Number(digits) + 1;
  for (let attempt = 0; attempt < 1000; attempt++) {
    const candidate = `${prefix}${String(number).padStart(digits.length, '0')}`;
    if (!used.has(candidate.toLowerCase())) {
      return candidate;
    }
    number++;
  }
  return '';
}

export type ExecuteMutation = <T>(
  operation: Operation,
  options: { id?: string; body?: Record<string, unknown> },
) => Promise<T>;

const newArea = (): Area => ({
  id: '',
  name: '',
  description: '',
  sort_order: 0,
  active: true,
  revision: 0,
  table_count: 0,
});
const newTable = (areaID: string): RestaurantTable => ({
  id: '',
  area_id: areaID,
  name: '',
  shape: 'rectangle',
  x: 20,
  y: 20,
  width: 100,
  height: 72,
  seats: 4,
  active: true,
  out_of_service: false,
  revision: 0,
  account_count: 0,
  state: 'free',
  occupied_since: null,
  occupied_seconds: 0,
  balance: '0.00',
});

export function OperationsAdmin({
  overview,
  can,
  execute,
  busy,
}: {
  overview: Overview;
  can: (permission: string) => boolean;
  execute: ExecuteMutation;
  busy: boolean;
}) {
  const [area, setArea] = useState<AreaDraft | null>(null);
  const [table, setTable] = useState<RestaurantTable | null>(null);
  const [areaID, setAreaID] = useState(overview.areas[0]?.id ?? '');
  const [positions, setPositions] = useState<Record<string, RestaurantTable>>({});
  const [settings, setSettings] = useState<RestaurantSettings>(overview.settings);
  const { confirm, dialog } = useConfirmDialog();
  const pendingPositions = Object.keys(positions).length;
  const tables = overview.tables.map((entry) => positions[entry.id] ?? entry);
  const selectedArea = overview.areas.some((entry) => entry.id === areaID)
    ? areaID
    : (overview.areas[0]?.id ?? '');

  async function saveTable(current: RestaurantTable) {
    const saved = await execute<RestaurantTable>(
      current.id ? 'restaurant_save_table' : 'restaurant_create_table',
      { id: current.id || undefined, body: { ...current } },
    );
    setPositions((existing) => {
      const remaining = { ...existing };
      delete remaining[saved.id];
      return remaining;
    });
    setAreaID(saved.area_id);
    return saved;
  }

  async function saveLayout() {
    try {
      await execute('restaurant_save_layout', {
        body: {
          tables: Object.values(positions).map(({ id, revision, x, y, width, height, shape }) => ({
            id,
            revision,
            x,
            y,
            width,
            height,
            shape,
          })),
        },
      });
      setPositions({});
      notifySuccess('Distribución de mesas guardada');
    } catch (problem) {
      // Un resultado incierto queda en «Recuperar resultado» de la página.
      if (!uncertainError(problem)) {
        notifyError(problem, 'No se pudo guardar la distribución.');
      }
    }
  }

  // Descartar las posiciones movidas en esta terminal pierde el trabajo sin guardar: se confirma.
  async function discardPositions() {
    if (pendingPositions === 0) {
      return;
    }
    const discard = await confirm({
      title: '¿Descartar las posiciones sin guardar?',
      message: 'Las mesas vuelven a la última distribución guardada.',
      confirmLabel: 'Descartar posiciones',
      cancelLabel: 'Conservarlas',
      tone: 'danger',
    });
    if (discard) {
      setPositions({});
    }
  }

  return (
    <div className="operations-admin form-stack">
      {dialog}
      <h2>Áreas, mesas y distribución</h2>
      <div className="button-row">
        {can('restaurant.areas.manage') && (
          <button className="secondary-button" onClick={() => setArea(newArea())}>
            Crear área
          </button>
        )}
        {can('restaurant.tables.manage') && (
          <button
            className="secondary-button"
            disabled={!selectedArea}
            onClick={() => setTable(newTable(selectedArea))}
          >
            Crear mesa
          </button>
        )}
        {can('restaurant.areas.manage') && (
          <ReferenceField
            optional
            label="Editar área"
            value={area?.id ?? ''}
            options={overview.areas}
            onChange={(id) => setArea(overview.areas.find((entry) => entry.id === id) ?? null)}
          />
        )}
        {can('restaurant.tables.manage') && (
          <ReferenceField
            optional
            label="Editar mesa"
            value={table?.id ?? ''}
            options={overview.tables}
            onChange={(id) => setTable(overview.tables.find((entry) => entry.id === id) ?? null)}
          />
        )}
      </div>
      {area && (
        <Accordion title={area.id ? `Área: ${area.name}` : 'Nueva área'} defaultOpen>
          <Form
            disabled={busy}
            label="Guardar área"
            successMessage="Área guardada"
            onSave={async () => {
              const body: Record<string, unknown> = { ...area };
              delete body.position;
              const position = positionPayload(overview.areas, area.id, area.position?.after);
              if (position) {
                body.position = position;
              }
              const saved = await execute<Area>(
                area.id ? 'restaurant_save_area' : 'restaurant_create_area',
                { id: area.id || undefined, body },
              );
              setArea(saved);
              setAreaID(saved.id);
            }}
          >
            <div className="form-grid">
              <TextField
                label="Nombre del área"
                required
                value={area.name}
                onChange={(name) => setArea({ ...area, name })}
              />
              <PositionField
                label="Posición del área"
                siblings={overview.areas}
                recordID={area.id}
                value={area.position?.after}
                onChange={(after) => setArea({ ...area, position: { after } })}
              />
            </div>
            <TextField
              label="Descripción del área"
              value={area.description}
              onChange={(description) => setArea({ ...area, description })}
            />
            <Check
              label="Área activa"
              value={area.active}
              onChange={(active) => setArea({ ...area, active })}
            />
          </Form>
        </Accordion>
      )}
      {table && (
        <Accordion title={table.id ? `Mesa: ${table.name}` : 'Nueva mesa'} defaultOpen>
          <Form
            disabled={busy}
            label="Guardar mesa"
            successMessage="Mesa guardada"
            onSave={async () => {
              setTable(await saveTable(table));
            }}
            onSaveAndNew={async () => {
              const saved = await saveTable(table);
              // El formulario queda limpio para la siguiente mesa, en la misma área.
              const taken = overview.tables
                .filter((entry) => entry.area_id === saved.area_id && entry.id !== saved.id)
                .map((entry) => entry.name);
              setTable({
                ...newTable(saved.area_id),
                name: nextTableName(saved.name, [...taken, saved.name]),
              });
            }}
          >
            <div className="form-grid">
              <TextField
                label="Nombre o número de mesa"
                required
                value={table.name}
                onChange={(name) => setTable({ ...table, name })}
              />
              <ReferenceField
                label="Área de la mesa"
                value={table.area_id}
                options={overview.areas}
                onChange={(area_id) => setTable({ ...table, area_id })}
              />
              <IntegerField
                label="Capacidad"
                value={table.seats}
                min={1}
                max={40}
                onChange={(seats) => setTable({ ...table, seats })}
              />
              <Field label="Forma">
                <Select<RestaurantTable['shape']>
                  value={table.shape}
                  options={[
                    { value: 'rectangle', label: 'Rectangular' },
                    { value: 'round', label: 'Redonda' },
                  ]}
                  onChange={(shape) => setTable({ ...table, shape })}
                />
              </Field>
              <IntegerField
                label="Posición horizontal"
                value={table.x}
                max={1000 - table.width}
                onChange={(x) => setTable({ ...table, x })}
              />
              <IntegerField
                label="Posición vertical"
                value={table.y}
                max={560 - table.height}
                onChange={(y) => setTable({ ...table, y })}
              />
              <IntegerField
                label="Ancho"
                value={table.width}
                min={64}
                max={240}
                onChange={(width) => setTable({ ...table, width })}
              />
              <IntegerField
                label="Alto"
                value={table.height}
                min={56}
                max={180}
                onChange={(height) => setTable({ ...table, height })}
              />
            </div>
            <div className="button-row">
              <Check
                label="Mesa activa"
                value={table.active}
                onChange={(active) => setTable({ ...table, active })}
              />
              <Check
                label="Fuera de servicio"
                value={table.out_of_service}
                onChange={(out_of_service) => setTable({ ...table, out_of_service })}
              />
            </div>
          </Form>
        </Accordion>
      )}
      <TableMap
        areas={overview.areas}
        tables={tables}
        areaID={selectedArea}
        onAreaChange={setAreaID}
        onSelect={(entry) => setTable(entry)}
        designer={can('restaurant.tables.manage')}
        onPosition={(id, x, y) => {
          const entry = tables.find((candidate) => candidate.id === id);
          if (entry) setPositions((current) => ({ ...current, [id]: { ...entry, x, y } }));
        }}
      />
      {can('restaurant.tables.manage') && (
        <div className="button-row">
          <button
            className="primary-button"
            disabled={busy || pendingPositions === 0}
            onClick={() => void saveLayout()}
          >
            Guardar distribución
          </button>
          <button
            className="danger-button"
            disabled={busy || pendingPositions === 0}
            onClick={() => void discardPositions()}
          >
            Descartar posiciones locales
          </button>
        </div>
      )}
      {can('restaurant.settings.manage') && (
        <Accordion
          title="Advertencias del tablero para llevar"
          summary="Umbrales de antigüedad de los pedidos"
        >
          <Form
            disabled={busy}
            successMessage="Advertencias del tablero guardadas"
            onSave={async () => {
              const saved = await execute<RestaurantSettings>('restaurant_save_settings', {
                body: { ...settings },
              });
              setSettings(saved);
            }}
          >
            <div className="form-grid">
              <IntegerField
                label="Advertir desde (minutos)"
                min={1}
                max={240}
                value={settings.warning_minutes}
                onChange={(warning_minutes) => setSettings({ ...settings, warning_minutes })}
              />
              <IntegerField
                label="Atrasado desde (minutos)"
                min={settings.warning_minutes}
                max={1440}
                value={settings.overdue_minutes}
                onChange={(overdue_minutes) => setSettings({ ...settings, overdue_minutes })}
              />
            </div>
          </Form>
        </Accordion>
      )}
    </div>
  );
}
