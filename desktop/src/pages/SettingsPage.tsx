import { useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { TextareaHTMLAttributes } from 'react';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { ToggleButton } from '../components/ToggleButton';
import { useResource } from '../hooks/useResource';
import { api, APIError } from '../services/api';
import { saveFeature } from '../services/features';
import { notifyError, notifySuccess } from '../services/notify';
import type { EstablishmentFeature } from '../services/features';
import type { Settings } from '../services/models';
import { filterTimezones, timezoneLabel, timezoneOptions } from '../services/timezones';
import { formatDayStartDigits, saveBusinessDay } from '../services/reports';
import type { BusinessDaySettings } from '../services/reports';

export const initialSettings: Settings = {
  name: '',
  timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
  currency: 'MXN',
  phone: '',
  email: '',
  address: '',
  theme: 'light',
};

// Zona horaria sin escritura libre: un buscador filtra la lista IANA (México primero) y la
// elección se hace en el selector. La zona elegida permanece visible aunque el filtro la oculte.
export function TimezoneField({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const [query, setQuery] = useState('');
  const zones = useMemo(() => timezoneOptions(value), [value]);
  const matches = filterTimezones(zones, query);
  const options = matches.includes(value) || !value ? matches : [value, ...matches];
  return (
    <div className="timezone-field">
      <Field label="Buscar zona horaria">
        <input
          type="search"
          maxLength={60}
          value={query}
          placeholder="Ej. Monterrey, Tijuana o Madrid"
          onChange={(event) => setQuery(event.target.value)}
        />
      </Field>
      <Field
        label="Zona horaria"
        hint={
          matches.length === 0
            ? 'Ninguna zona coincide con la búsqueda.'
            : 'Las zonas de México aparecen primero. El servidor confirma la zona al guardar.'
        }
      >
        <select required value={value} onChange={(event) => onChange(event.target.value)}>
          {!value && <option value="">Selecciona…</option>}
          {options.map((zone) => (
            <option key={zone} value={zone}>
              {timezoneLabel(zone)}
            </option>
          ))}
        </select>
      </Field>
    </div>
  );
}

// Área de texto que crece con su contenido hasta el alto máximo de .auto-grow (220 px); a partir
// de ahí muestra desplazamiento interno en lugar de crecer sin límite.
export function AutoGrowTextarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  const element = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const textarea = element.current;
    if (!textarea) {
      return;
    }
    textarea.style.height = 'auto';
    const borders = textarea.offsetHeight - textarea.clientHeight;
    textarea.style.height = `${textarea.scrollHeight + borders}px`;
  }, [props.value]);
  return (
    <textarea
      {...props}
      ref={element}
      rows={props.rows ?? 3}
      className={`auto-grow ${props.className ?? ''}`}
    />
  );
}

export function SettingsFields({
  value,
  onChange,
}: {
  value: Settings;
  onChange: (value: Settings) => void;
}) {
  function change(key: keyof Settings, next: string) {
    onChange({ ...value, [key]: next });
  }
  return (
    <div className="form-grid">
      <Field label="Nombre comercial">
        <input
          required
          maxLength={100}
          value={value.name}
          onChange={(event) => change('name', event.target.value)}
        />
      </Field>
      <TimezoneField value={value.timezone} onChange={(timezone) => change('timezone', timezone)} />
      {/* El sistema solo admite pesos mexicanos (CHECK en PostgreSQL y validación en Go). */}
      <div className="field">
        <span>Moneda</span>
        <p className="read-only-value">MXN · Peso mexicano</p>
      </div>
      <Field label="Apariencia inicial">
        <select value={value.theme} onChange={(event) => change('theme', event.target.value)}>
          <option value="light">Claro</option>
          <option value="dark">Oscuro</option>
        </select>
      </Field>
      <Field label="Teléfono (opcional)">
        <input
          type="tel"
          maxLength={100}
          value={value.phone}
          onChange={(event) => change('phone', event.target.value)}
        />
      </Field>
      <Field label="Correo (opcional)">
        <input
          type="email"
          maxLength={200}
          value={value.email}
          onChange={(event) => change('email', event.target.value)}
        />
      </Field>
      <Field label="Dirección (opcional)">
        <AutoGrowTextarea
          maxLength={500}
          value={value.address}
          onChange={(event) => change('address', event.target.value)}
        />
      </Field>
    </div>
  );
}

function SettingsEditor({
  settings,
  editable,
  onSaved,
}: {
  settings: Settings;
  editable: boolean;
  onSaved: () => Promise<void>;
}) {
  const [value, setValue] = useState<Settings>(() => ({
    name: settings.name,
    timezone: settings.timezone,
    currency: settings.currency,
    phone: settings.phone,
    email: settings.email,
    address: settings.address,
    theme: settings.theme,
  }));
  return (
    <div className="surface panel">
      <Form
        disabled={!editable}
        successMessage="Configuración del establecimiento guardada."
        onSave={async () => {
          await api('save_settings', { body: value });
          await onSaved();
        }}
      >
        <SettingsFields value={value} onChange={setValue} />
      </Form>
    </div>
  );
}

// Funciones activables: un interruptor por función con su explicación. Go guarda el estado y lo
// hace cumplir aunque se llame a la API directamente; desactivar nunca borra registros.
export function FeaturesSection({ can }: { can: (key: string) => boolean }) {
  const resource = useResource<EstablishmentFeature[]>('settings_features');
  const editable = can('settings.features.manage');
  const [saving, setSaving] = useState('');

  async function change(feature: EstablishmentFeature, enabled: boolean) {
    if (saving !== '') {
      return;
    }
    setSaving(feature.code);
    try {
      const saved = await saveFeature({
        code: feature.code,
        enabled,
        revision: feature.revision,
      });
      notifySuccess(`${saved.name}: ${saved.enabled ? 'activada' : 'desactivada'}.`);
      // Las pantallas abiertas (cobro, turno, cobros) vuelven a consultar el estado.
      window.dispatchEvent(new CustomEvent('adel-server-change'));
    } catch (problem) {
      if (problem instanceof APIError && problem.code === 'REVISION_CONFLICT') {
        notifyError(
          'La función cambió en otra terminal. Se cargó su estado actual; revísalo antes de cambiarla.',
        );
      } else {
        notifyError(problem, 'No se pudo guardar la función.');
      }
    } finally {
      setSaving('');
      await resource.reload();
    }
  }

  return (
    <section className="surface panel form-stack" aria-labelledby="features-title">
      <div>
        <h2 id="features-title">Funciones del establecimiento</h2>
        <p className="muted text-sm">
          Activa solo lo que usa este restaurante. Una función desactivada no aparece en la
          operación y el servidor rechaza sus datos; desactivarla nunca borra ni altera los
          registros que ya generó.
        </p>
      </div>
      <Notice>{resource.error}</Notice>
      <ul className="feature-list">
        {(resource.data ?? []).map((feature) => (
          <li key={feature.code} className="feature-row">
            {/* El interruptor lleva el nombre de la función; su estado se anuncia con aria-pressed. */}
            <ToggleButton
              label={feature.name}
              pressed={feature.enabled}
              onChange={(enabled) => void change(feature, enabled)}
              disabled={!editable || saving !== '' || resource.loading}
              describedBy={`feature-${feature.code}-description`}
            />
            <div className="feature-text">
              <p id={`feature-${feature.code}-description`} className="muted text-sm">
                {feature.description}
              </p>
              <small className={feature.enabled ? 'feature-state-on' : 'muted'}>
                {feature.enabled ? 'Activada' : 'Desactivada'}
                {feature.updated_by_name ? ` · último cambio: ${feature.updated_by_name}` : ''}
              </small>
            </div>
          </li>
        ))}
      </ul>
      {!editable && (
        <p className="muted text-sm">
          Cambiar las funciones requiere el permiso «Activar o desactivar funciones del
          establecimiento».
        </p>
      )}
    </section>
  );
}

function BusinessDayForm({
  settings,
  editable,
  onSaved,
}: {
  settings: BusinessDaySettings;
  editable: boolean;
  onSaved: () => Promise<void>;
}) {
  const [dayStart, setDayStart] = useState(settings.operating_day_cutoff);
  return (
    <Form
      label="Guardar hora de inicio"
      disabled={!editable}
      successMessage="Hora de inicio del día guardada."
      onSave={async () => {
        try {
          await saveBusinessDay({ operating_day_cutoff: dayStart, revision: settings.revision });
        } finally {
          // Con conflicto de revisión se carga la versión vigente para revisarla.
          await onSaved();
        }
      }}
    >
      <div className="form-grid">
        <Field
          label="Hora de inicio del día"
          hint="HH:MM en la zona horaria del establecimiento (de 00:00 a 23:59)."
        >
          <input
            type="text"
            inputMode="numeric"
            autoComplete="off"
            required
            maxLength={5}
            pattern="([01][0-9]|2[0-3]):[0-5][0-9]"
            placeholder="05:00"
            value={dayStart}
            onChange={(event) => {
              const formatted = formatDayStartDigits(event.target.value);
              if (formatted !== null) {
                setDayStart(formatted);
              }
            }}
          />
        </Field>
      </div>
    </Form>
  );
}

// «Hora de inicio del día» (etapa 9A): una sola configuración para reportes e inventario. Se
// edita aquí con settings.manage; Inventarios → Parámetros la muestra en solo lectura.
export function BusinessDaySection({ can }: { can: (key: string) => boolean }) {
  const resource = useResource<BusinessDaySettings>('settings_business_day');
  return (
    <section className="surface panel form-stack" aria-labelledby="business-day-title">
      <div>
        <h2 id="business-day-title">Hora de inicio del día</h2>
        <p className="muted text-sm">
          Define cuándo empieza el día comercial para los reportes, el día operativo del inventario
          y las cancelaciones por clasificar. Con 00:00 el día es la fecha del calendario. Por
          ejemplo, con 05:00 una venta de la 01:30 pertenece al día anterior. Cambiarla no modifica
          los registros: solo cambia cómo se agrupan los días en las consultas.
        </p>
      </div>
      <Notice>{resource.error}</Notice>
      {resource.data && (
        <BusinessDayForm
          key={resource.data.revision}
          settings={resource.data}
          editable={can('settings.manage')}
          onSaved={resource.reload}
        />
      )}
    </section>
  );
}

export function SettingsPage({
  can,
  onSaved,
}: {
  can: (key: string) => boolean;
  onSaved: () => Promise<void>;
}) {
  const resource = useResource<Settings>('settings');
  return (
    <>
      <PageTitle
        title="Configuración general"
        description="Datos del establecimiento guardados en PostgreSQL."
      />
      <Notice>{resource.error}</Notice>
      {resource.data && (
        <SettingsEditor
          settings={resource.data}
          editable={can('settings.manage')}
          onSaved={onSaved}
        />
      )}
      <BusinessDaySection can={can} />
      <FeaturesSection can={can} />
    </>
  );
}
