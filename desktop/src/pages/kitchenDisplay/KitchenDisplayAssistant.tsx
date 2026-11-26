import { useState } from 'react';
import { Field, Notice } from '../../components/Forms';
import { MultiSelect, Select } from '../../components/Select';
import { Check } from '../../components/catalog/EditorFields';
import { useLiveResource } from '../../hooks/useLiveResource';
import { useResource } from '../../hooks/useResource';
import { useRestaurantMutation } from '../../hooks/useRestaurantMutation';
import { APIError } from '../../services/api';
import type { Operation } from '../../services/api';
import type { Station } from '../../services/kitchen';
import type { CoverageStation, KitchenCoverage } from '../../services/kitchenCoverage';
import {
  categoryArrives,
  categoryChangeWarning,
  categoryRouteText,
  displayStationIDs,
  displaySummary,
  displayWarnings,
  receivesOnScreen,
  receptionLabels,
  stationsByID,
} from '../../services/kitchenCoverage';
import type { DeviceConfiguration } from '../../services/models';
import { notifyError, notifySuccess } from '../../services/notify';

// Etapa K1.1 (§4.1, K11-D6): asistente corto para que una pantalla de cocina reciba pedidos. Se abre al aprobar el
// código de una pantalla de cocina en Sistema › Terminales y desde «Configurar qué recibe» de una ya emparejada.
//   1. Estaciones de la pantalla: las que reciben comandas en pantalla; si no hay, crear «Cocina» (o usar la que ya
//      existe) y, para las que solo imprimen, pasar a «Impresora y pantalla de cocina». Con una pantalla ya emparejada
//      también se eligen sus estaciones (PUT kitchen/access).
//   2. Qué categorías llegan: marcar una crea su ruta o la cambia a la estación de la pantalla (deja de ir a la otra).
//   3. Resumen: «Esta pantalla recibirá: … No recibirá: … (no necesitan preparación).».
// No hay rutas nuevas: usa las de siempre en orden, cada una con su operation_id, y la cobertura de Go para leer y
// para el resumen. Cada paso exige su permiso (sin él se ve en solo lectura) y se puede saltar.

export interface AssistantTarget {
  // null: la pantalla recién aprobada todavía no tiene dispositivo (se crea cuando la tableta reclama su credencial).
  deviceId: string | null;
  name: string;
  step?: AssistantStep;
}

export type AssistantStep = 1 | 2 | 3;

const stepTitles: Record<AssistantStep, string> = {
  1: 'Estaciones de esta pantalla',
  2: 'Qué categorías llegan',
  3: 'Resumen',
};

// Nombre de la estación que se ofrece crear cuando ninguna recibe comandas en pantalla.
export const suggestedStationName = 'Cocina';

const askAdministrator = 'Pide a quien administra Cocina que lo configure.';

// Lo que hace cada escritura del asistente, con el controlador de operaciones idempotentes de siempre: un rechazo de
// Go se avisa; un resultado incierto queda para recuperarlo con el mismo operation_id.
type Run = <T>(
  operation: Operation,
  options: { id?: string; body?: Record<string, unknown> },
  success?: string,
) => Promise<T | null>;

export function KitchenDisplayAssistant({
  target,
  can,
  onClose,
}: {
  target: AssistantTarget;
  can: (key: string) => boolean;
  onClose: () => void;
}) {
  const [step, setStep] = useState<AssistantStep>(target.step ?? 1);
  const coverage = useLiveResource<KitchenCoverage>('kitchen_coverage');
  const mutation = useRestaurantMutation(() => {
    window.dispatchEvent(new CustomEvent('adel-server-change'));
  });
  const busy = mutation.busy || mutation.uncertain;

  const run: Run = async (operation, options, success) => {
    try {
      const result = await mutation.execute(operation, options);
      if (success) {
        notifySuccess(success);
      }
      await coverage.reload();
      return result as never;
    } catch (problem) {
      const uncertain =
        problem instanceof APIError &&
        (problem.code === 'CONNECTION_ERROR' || problem.status >= 500);
      if (!uncertain) {
        notifyError(problem, 'No se pudo guardar la configuración de cocina.');
      }
      await coverage.reload();
      return null;
    }
  };

  async function recover() {
    const problem = await mutation.retry();
    if (!problem) {
      notifySuccess('Configuración de cocina recuperada.');
    } else if (!(
      problem instanceof APIError &&
      (problem.code === 'CONNECTION_ERROR' || problem.status >= 500)
    )) {
      notifyError(problem, 'No se pudo recuperar la operación.');
    }
    await coverage.reload();
  }

  const data = coverage.data;
  const displayIDs = data ? displayStationIDs(data, target.deviceId) : [];

  return (
    <section className="surface panel kitchen-assistant" aria-labelledby="kitchen-assistant-title">
      <h2 id="kitchen-assistant-title">Qué recibe {target.name}</h2>
      <ol className="kitchen-assistant-steps" aria-label="Pasos">
        {([1, 2, 3] as AssistantStep[]).map((entry) => (
          <li key={entry} aria-current={entry === step ? 'step' : undefined}>
            {entry}. {stepTitles[entry]}
          </li>
        ))}
      </ol>
      <Notice>{coverage.error}</Notice>
      {mutation.uncertain && (
        <div className="operation-recovery">
          <p>El resultado no pudo confirmarse. Recupera la misma operación antes de continuar.</p>
          <button
            className="primary-button"
            disabled={mutation.busy}
            onClick={() => void recover()}
          >
            Recuperar la operación
          </button>
        </div>
      )}
      {!data && !coverage.error && <p className="muted">Consultando la configuración de cocina…</p>}
      {data && step === 1 && (
        <StationsStep
          coverage={data}
          target={target}
          displayIDs={displayIDs}
          can={can}
          run={run}
          busy={busy}
        />
      )}
      {data && step === 2 && (
        <CategoriesStep
          coverage={data}
          displayIDs={displayIDs}
          can={can}
          run={run}
          busy={busy}
          onSaved={() => setStep(3)}
        />
      )}
      {data && step === 3 && (
        <SummaryStep coverage={data} target={target} displayIDs={displayIDs} />
      )}
      <div className="button-row kitchen-assistant-footer">
        {step > 1 && (
          <button
            className="secondary-button"
            disabled={busy}
            onClick={() => setStep((step - 1) as AssistantStep)}
          >
            Anterior
          </button>
        )}
        {step < 3 && (
          <button
            className="secondary-button"
            disabled={busy}
            onClick={() => setStep((step + 1) as AssistantStep)}
          >
            {step === 1 ? 'Siguiente' : 'Saltar este paso'}
          </button>
        )}
        {step < 3 ? (
          <button className="secondary-button" disabled={busy} onClick={onClose}>
            Saltar el asistente
          </button>
        ) : (
          <button className="primary-button" disabled={busy} onClick={onClose}>
            Terminar
          </button>
        )}
      </div>
    </section>
  );
}

// Paso 1: las estaciones de la pantalla.
function StationsStep({
  coverage,
  target,
  displayIDs,
  can,
  run,
  busy,
}: {
  coverage: KitchenCoverage;
  target: AssistantTarget;
  displayIDs: string[];
  can: (key: string) => boolean;
  run: Run;
  busy: boolean;
}) {
  const manage = can('kitchen.stations.manage');
  // Las estaciones completas (con su revisión) para cambiar su recepción con PATCH kitchen/stations/{id}.
  const stations = useLiveResource<Station[]>('kitchen_stations', { enabled: manage });
  const screenStations = coverage.stations.filter(
    (station) => station.active && receivesOnScreen(station),
  );
  const existing = coverage.stations.find(
    (station) => station.name.trim().toLowerCase() === suggestedStationName.toLowerCase(),
  );
  // «Cocina» ya existe pero no recibe en pantalla (desactivada o solo imprime): se ofrece usarla (K11-D6).
  const reuse =
    screenStations.length === 0 && existing && !(existing.active && receivesOnScreen(existing))
      ? existing
      : undefined;
  const printerOnly = coverage.stations.filter(
    (station) => station.active && station.reception === 'printer' && station.id !== reuse?.id,
  );

  // Una estación que solo imprime pasa a «Impresora y pantalla» con su misma impresora. «Cocina» desactivada se
  // activa: con su impresora activa, conserva la impresión («Impresora y pantalla», o «Solo pantalla» si ya lo era);
  // sin impresora o con ella desactivada, «Solo pantalla» (Go no deja activar una estación con su impresora
  // desactivada).
  async function receiveOnScreen(station: CoverageStation, activate: boolean) {
    const full = stations.data?.find((entry) => entry.id === station.id);
    if (!full) {
      notifyError('La estación cambió. Vuelve a intentarlo en unos segundos.');
      await stations.reload();
      return;
    }
    const printerUsable = full.printer_id !== null && station.printer?.active !== false;
    let reception: Station['reception'] = 'both';
    if (activate) {
      reception = printerUsable && full.reception !== 'kds' ? 'both' : 'kds';
    }
    await run(
      'kitchen_save_station',
      {
        id: full.id,
        body: {
          ...full,
          active: activate ? true : full.active,
          reception,
          printer_id: reception === 'kds' ? null : full.printer_id,
        },
      },
      reception === 'both'
        ? `${full.name} ahora imprime y se ve en la pantalla de cocina.`
        : `${full.name} ahora se ve en la pantalla de cocina.`,
    );
    await stations.reload();
  }

  return (
    <div className="form-stack">
      <p className="muted">
        Una pantalla de cocina muestra los pedidos de las estaciones que reciben comandas en
        pantalla («Solo pantalla de cocina» o «Impresora y pantalla de cocina»).
      </p>
      {target.deviceId ? (
        <DisplayStationsChoice
          deviceId={target.deviceId}
          coverage={coverage}
          screenStations={screenStations}
          manage={manage}
          run={run}
          busy={busy}
        />
      ) : (
        <div className="kitchen-assistant-block">
          <h3>Estaciones que mostrará</h3>
          {screenStations.length > 0 ? (
            <>
              <ul className="kitchen-assistant-list">
                {screenStations.map((station) => (
                  <li key={station.id}>
                    <strong>{station.name}</strong> · {receptionLabels[station.reception]}
                  </li>
                ))}
              </ul>
              <p className="muted text-sm">
                Una pantalla nueva muestra todas. Cuando se conecte puedes elegir solo algunas con
                «Configurar qué recibe».
              </p>
            </>
          ) : (
            <p>No hay estaciones que reciban comandas en pantalla.</p>
          )}
        </div>
      )}
      {screenStations.length === 0 && !reuse && (
        <CreateStation manage={manage} run={run} busy={busy} />
      )}
      {reuse && (
        <div className="kitchen-assistant-block">
          <h3>Usar la estación «{reuse.name}»</h3>
          <p>
            {!reuse.active
              ? `Ya existe y está desactivada: se activa para que reciba las comandas en esta pantalla.`
              : `Ya existe y solo imprime: pasa a «Impresora y pantalla de cocina», así se sigue imprimiendo y además se ve en esta pantalla.`}
          </p>
          {manage ? (
            <button
              className="primary-button"
              disabled={busy || !stations.data}
              onClick={() => void receiveOnScreen(reuse, true)}
            >
              Usar «{reuse.name}»
            </button>
          ) : (
            <p className="muted">{askAdministrator}</p>
          )}
        </div>
      )}
      {printerOnly.length > 0 && (
        <div className="kitchen-assistant-block">
          <h3>Estaciones que solo imprimen</h3>
          <p className="muted text-sm">
            Con «Impresora y pantalla de cocina» la comanda se sigue imprimiendo y además aparece en
            la pantalla.
          </p>
          <ul className="kitchen-assistant-list">
            {printerOnly.map((station) => (
              <li key={station.id}>
                <span>
                  <strong>{station.name}</strong> · {receptionLabels.printer}
                </span>
                {manage && (
                  <button
                    className="secondary-button"
                    disabled={busy || !stations.data}
                    onClick={() => void receiveOnScreen(station, false)}
                  >
                    Recibir también en pantalla
                  </button>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
      {!manage && (
        <p className="muted">
          Para cambiar las estaciones hace falta el permiso de administrar estaciones de cocina.{' '}
          {askAdministrator}
        </p>
      )}
      {displayIDs.length === 0 && screenStations.length > 0 && target.deviceId && (
        <p className="muted">Esta pantalla no muestra ninguna estación.</p>
      )}
    </div>
  );
}

// Crear «Cocina» ahí mismo (K11-D6): con la impresora de cocina activa si hay una, preguntando cuál si hay varias y
// «Solo pantalla» si no hay ninguna.
function CreateStation({ manage, run, busy }: { manage: boolean; run: Run; busy: boolean }) {
  const outputs = useLiveResource<
    { id: string; name: string; active: boolean; terminal_name?: string | null }[]
  >('kitchen_outputs', { enabled: manage });
  const active = (outputs.data ?? []).filter((printer) => printer.active);
  const [printer, setPrinter] = useState<string | null>(null);
  // Con una sola impresora activa se usa esa; con varias hay que elegir; sin ninguna, solo pantalla.
  const chosen = printer ?? (active.length === 1 ? (active[0]?.id ?? '') : '');
  const needsChoice = active.length > 1 && chosen === '';

  async function create() {
    const printerID = chosen === 'none' || chosen === '' ? null : chosen;
    await run(
      'kitchen_create_station',
      {
        body: {
          name: suggestedStationName,
          active: true,
          reception: printerID ? 'both' : 'kds',
          printer_id: printerID,
        },
      },
      `Estación ${suggestedStationName} creada.`,
    );
  }

  return (
    <div className="kitchen-assistant-block">
      <h3>Crear la estación «{suggestedStationName}»</h3>
      {!manage && <p className="muted">{askAdministrator}</p>}
      {manage && outputs.data && active.length === 0 && (
        <p>Recibirá las comandas solo en la pantalla: no hay impresoras de cocina activas.</p>
      )}
      {manage && active.length > 0 && (
        <Field
          label="Impresora de la estación"
          hint="La comanda se imprime ahí y además se ve en la pantalla. Sin impresora, solo se ve en la pantalla."
        >
          <Select
            value={chosen}
            placeholder="Elige la impresora"
            options={[
              ...active.map((entry) => ({
                value: entry.id,
                label: entry.terminal_name
                  ? `${entry.name} · en ${entry.terminal_name}`
                  : entry.name,
              })),
              { value: 'none', label: 'Sin impresora: solo pantalla' },
            ]}
            onChange={setPrinter}
          />
        </Field>
      )}
      <Notice>{outputs.error}</Notice>
      {manage && (
        <button
          className="primary-button"
          disabled={busy || !outputs.data || needsChoice}
          onClick={() => void create()}
        >
          Crear «{suggestedStationName}»
        </button>
      )}
    </div>
  );
}

// Una pantalla ya emparejada: elegir todas sus estaciones o algunas (PUT kitchen/access con lo que se leyó, para que Go
// rechace un cambio de otra pantalla). Solo se ofrecen las que reciben comandas en pantalla (K1-D11).
function DisplayStationsChoice({
  deviceId,
  coverage,
  screenStations,
  manage,
  run,
  busy,
}: {
  deviceId: string;
  coverage: KitchenCoverage;
  screenStations: CoverageStation[];
  manage: boolean;
  run: Run;
  busy: boolean;
}) {
  const configuration = useResource<DeviceConfiguration>('device_configuration', { id: deviceId });
  const loaded = configuration.data;
  const loadedAll = !loaded || loaded.kitchen_all_stations || loaded.kitchen_stations.length === 0;
  const loadedIDs = loaded?.kitchen_stations.map((station) => station.id) ?? [];
  const shownIDs = loadedIDs.filter((id) => screenStations.some((station) => station.id === id));
  const [all, setAll] = useState<boolean | null>(null);
  const [chosen, setChosen] = useState<string[] | null>(null);
  const chosenAll = all ?? loadedAll;
  const chosenIDs = chosen ?? shownIDs;
  const shown = stationsByID(coverage, displayStationIDs(coverage, deviceId));
  const changed =
    loaded !== null &&
    (chosenAll !== loadedAll ||
      (!chosenAll && [...chosenIDs].sort().join(',') !== [...shownIDs].sort().join(',')));

  async function save() {
    const saved = await run(
      'kitchen_save_access',
      {
        body: {
          device_id: deviceId,
          all_stations: chosenAll,
          station_ids: chosenAll ? [] : chosenIDs,
          expected_all_stations: loaded?.kitchen_all_stations ?? true,
          expected_station_ids: loadedIDs,
        },
      },
      'Estaciones de la pantalla guardadas.',
    );
    if (saved !== null) {
      setAll(null);
      setChosen(null);
    }
    await configuration.reload();
  }

  return (
    <div className="kitchen-assistant-block">
      <h3>Estaciones que muestra</h3>
      <Notice>{configuration.error}</Notice>
      <p>
        {shown.length > 0
          ? shown.map((station) => station.name).join(', ')
          : 'Ninguna: no hay estaciones que reciban comandas en pantalla.'}
      </p>
      {screenStations.length > 0 && (
        <fieldset disabled={!manage || busy || !loaded} className="form-stack">
          <Check
            label="Mostrar todas las estaciones que reciben comandas en pantalla"
            value={chosenAll}
            onChange={setAll}
          />
          {!chosenAll && (
            <Field label="Estaciones que muestra esta pantalla" hint="Elige al menos una.">
              <MultiSelect
                value={chosenIDs}
                placeholder="Elige las estaciones"
                options={screenStations.map((station) => ({
                  value: station.id,
                  label: station.name,
                }))}
                onChange={setChosen}
              />
            </Field>
          )}
          {manage && (
            <div className="button-row">
              <button
                className="primary-button"
                disabled={!changed || (!chosenAll && chosenIDs.length === 0)}
                onClick={() => void save()}
              >
                Guardar estaciones de la pantalla
              </button>
            </div>
          )}
        </fieldset>
      )}
    </div>
  );
}

// Paso 2: qué categorías llegan a las estaciones de la pantalla.
function CategoriesStep({
  coverage,
  displayIDs,
  can,
  run,
  busy,
  onSaved,
}: {
  coverage: KitchenCoverage;
  displayIDs: string[];
  can: (key: string) => boolean;
  run: Run;
  busy: boolean;
  onSaved: () => void;
}) {
  const manage = can('kitchen.routing.manage');
  const targets = stationsByID(coverage, displayIDs);
  // Categorías marcadas en este paso, cada una con la estación a la que irá (por omisión, la primera).
  const [chosen, setChosen] = useState<Record<string, string>>({});
  const firstTarget = targets[0]?.id ?? '';
  const pending = Object.entries(chosen).filter(([id]) =>
    coverage.categories.some((category) => category.id === id),
  );

  async function save() {
    let saved = 0;
    for (const [categoryID, stationID] of pending) {
      const category = coverage.categories.find((entry) => entry.id === categoryID);
      if (!category) {
        continue;
      }
      // Sin ruta de categoría se crea; con ella, se cambia a la estación de la pantalla con su revisión.
      const body: Record<string, unknown> = {
        product_id: null,
        category_id: category.id,
        mode: 'prepare',
        station_ids: [stationID || firstTarget],
      };
      const result = category.route_id
        ? await run('kitchen_save_route', {
            id: category.route_id,
            body: { ...body, id: category.route_id, revision: category.route_revision },
          })
        : await run('kitchen_create_route', { body });
      if (result === null) {
        // Go rechazó una: se detiene para que se vea el aviso y sigue marcada; las anteriores ya quedaron guardadas.
        return;
      }
      saved++;
      setChosen((current) => {
        const next = { ...current };
        delete next[categoryID];
        return next;
      });
    }
    if (saved > 0) {
      notifySuccess(saved === 1 ? 'Categoría guardada.' : `${saved} categorías guardadas.`);
    }
    onSaved();
  }

  if (targets.length === 0) {
    return (
      <p>Esta pantalla no tiene estaciones. Vuelve al paso anterior para crear o elegir una.</p>
    );
  }
  return (
    <div className="form-stack">
      <p className="muted">
        Marca las categorías que deben llegar a {targets.map((station) => station.name).join(' o ')}
        . Las que ya llegan salen marcadas; para quitarlas, cambia su ruta en Cocina › Enrutamiento.
      </p>
      {!manage && (
        <p className="muted">
          Para cambiar las rutas hace falta el permiso de administrar el enrutamiento de cocina.{' '}
          {askAdministrator}
        </p>
      )}
      <fieldset disabled={!manage || busy} className="kitchen-assistant-categories">
        <legend className="visually-hidden">Categorías del catálogo</legend>
        {coverage.categories.map((category) => {
          const arrives = categoryArrives(category, displayIDs);
          const checked = arrives || category.id in chosen;
          const warning =
            !arrives && category.id in chosen ? categoryChangeWarning(coverage, category) : '';
          return (
            <div className="kitchen-assistant-category" key={category.id}>
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={arrives || !manage || busy}
                  onChange={(event) =>
                    setChosen((current) => {
                      const next = { ...current };
                      if (event.target.checked) {
                        next[category.id] = firstTarget;
                      } else {
                        delete next[category.id];
                      }
                      return next;
                    })
                  }
                />
                <span>
                  {category.name}
                  <small className="muted">
                    {' '}
                    · {categoryRouteText(coverage, category, displayIDs)}
                  </small>
                </span>
              </label>
              {!arrives && category.id in chosen && targets.length > 1 && (
                <Field label={`Estación para ${category.name}`}>
                  <Select
                    value={chosen[category.id] ?? firstTarget}
                    options={targets.map((station) => ({ value: station.id, label: station.name }))}
                    onChange={(stationID) =>
                      setChosen((current) => ({ ...current, [category.id]: stationID }))
                    }
                  />
                </Field>
              )}
              {warning && <small className="kitchen-assistant-warning">{warning}</small>}
            </div>
          );
        })}
        {coverage.categories.length === 0 && (
          <p className="muted">El catálogo no tiene categorías.</p>
        )}
      </fieldset>
      {manage && (
        <div className="button-row">
          <button
            className="primary-button"
            disabled={busy || pending.length === 0}
            onClick={() => void save()}
          >
            Guardar y seguir
          </button>
        </div>
      )}
    </div>
  );
}

// Paso 3: el resumen con la cobertura que Go acaba de resolver, y los avisos de la pantalla.
function SummaryStep({
  coverage,
  target,
  displayIDs,
}: {
  coverage: KitchenCoverage;
  target: AssistantTarget;
  displayIDs: string[];
}) {
  const summary = displaySummary(coverage, displayIDs);
  const warnings = displayWarnings(coverage, target.deviceId, displayIDs);
  return (
    <div className="form-stack kitchen-assistant-summary" aria-label="Resumen de lo que recibe">
      <p>
        <strong>{summary.receives}</strong>
      </p>
      {summary.misses && <p>{summary.misses}</p>}
      {warnings.length > 0 && (
        <ul className="kitchen-coverage-warnings" aria-label="Avisos de la pantalla">
          {warnings.map((warning, index) => (
            <li key={`${warning.code}-${index}`}>
              <span>{warning.message}</span>
            </li>
          ))}
        </ul>
      )}
      <p className="muted text-sm">
        Los cambios le llegan a la pantalla al momento. Puedes volver aquí con «Configurar qué
        recibe» en Sistema › Terminales.
      </p>
    </div>
  );
}
