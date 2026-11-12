import { useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { TextareaHTMLAttributes } from 'react';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { IntegerField } from '../components/catalog/EditorFields';
import { IconLabel } from '../components/Icon';
import { ToggleButton } from '../components/ToggleButton';
import { useResource } from '../hooks/useResource';
import { api, APIError } from '../services/api';
import { saveFeature } from '../services/features';
import { countLabel } from '../services/plural';
import { saveLongOpenHours } from '../services/restaurant';
import type { LongOpenHoursSettings } from '../services/restaurant';
import { notifyError, notifySuccess } from '../services/notify';
import type { EstablishmentFeature } from '../services/features';
import type { Settings } from '../services/models';
import { timezoneGroups } from '../services/timezones';
import { saveBusinessDay } from '../services/reports';
import type { BusinessDaySettings } from '../services/reports';
import { Select } from '../components/Select';
import { TimeField } from '../components/TimeField';

export const initialSettings: Settings = {
  name: '',
  timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
  currency: 'MXN',
  phone: '',
  email: '',
  address: '',
  theme: 'light',
};

// Zona horaria sin escritura libre: la búsqueda integrada del selector filtra la lista IANA (sin
// acentos ni guiones bajos) y las zonas de México van primero, en su propio grupo.
export function TimezoneField({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const groups = useMemo(() => timezoneGroups(value), [value]);
  return (
    <Field label="Zona horaria del restaurante" hint="Las zonas de México aparecen primero.">
      <Select
        required
        value={value}
        options={groups}
        searchPlaceholder="Ej. Monterrey, Tijuana o Madrid"
        emptyMessage="Ninguna zona coincide con la búsqueda."
        onChange={onChange}
      />
    </Field>
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
  setup = false,
}: {
  value: Settings;
  onChange: (value: Settings) => void;
  // En el asistente inicial la apariencia también es la del administrador que se crea.
  setup?: boolean;
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
      {/* Etapa 9C (§4.4): Go copia esta apariencia a cada usuario nuevo; la de cada usuario se
          cambia en Mis preferencias y es la que se aplica. */}
      <Field
        label="Apariencia predeterminada para usuarios nuevos"
        hint={
          setup
            ? 'También es la de tu usuario administrador. Cada usuario la cambia en Mis preferencias.'
            : 'Cada usuario la cambia en Mis preferencias; los usuarios que ya existen conservan la suya.'
        }
      >
        <Select
          value={value.theme}
          options={[
            { value: 'light', label: 'Claro' },
            { value: 'dark', label: 'Oscuro' },
          ]}
          onChange={(theme) => change('theme', theme)}
        />
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

function LongOpenHoursForm({
  settings,
  editable,
  onSaved,
}: {
  settings: LongOpenHoursSettings;
  editable: boolean;
  onSaved: () => Promise<void>;
}) {
  const [hours, setHours] = useState(settings.long_open_hours);
  return (
    <Form
      label="Guardar umbral"
      disabled={!editable}
      successMessage="Umbral de cuenta abierta guardado."
      onSave={async () => {
        try {
          await saveLongOpenHours({ long_open_hours: hours, revision: settings.revision });
          // Mesas, Cuentas y Para llevar vuelven a consultar el mapa y las cuentas con el umbral
          // nuevo.
          window.dispatchEvent(new CustomEvent('adel-server-change'));
        } finally {
          // Con conflicto de revisión se carga la versión vigente para revisarla.
          await onSaved();
        }
      }}
    >
      <div className="form-grid">
        {/* Entrada numérica de la 7.1 (texto con teclado numérico, sin type="number"); Go valida el
            intervalo y la revisión. */}
        <IntegerField
          label="Marcar una cuenta abierta después de (horas)"
          min={settings.minimum_hours}
          max={settings.maximum_hours}
          hint={`De ${settings.minimum_hours} a ${settings.maximum_hours} horas.`}
          value={hours}
          onChange={setHours}
        />
      </div>
      <p className="muted text-sm" aria-live="polite">
        Las cuentas abiertas más de {countLabel(hours, 'hora', 'horas')} se marcan en ámbar en Mesas
        y en las tarjetas de las cuentas y de Para llevar. Solo es un aviso: no cierra ni bloquea
        ninguna cuenta.
      </p>
    </Form>
  );
}

// Umbral de cuenta abierta por mucho tiempo (etapa 9D, §3.4): lo lee cualquier sesión y lo cambia
// quien activa las funciones del establecimiento. Go calcula la marca de cada cuenta y mesa.
function LongOpenHoursSection({ editable }: { editable: boolean }) {
  const resource = useResource<LongOpenHoursSettings>('settings_long_open_hours');
  return (
    <div className="feature-threshold form-stack">
      <div>
        <h3>Cuenta abierta por mucho tiempo</h3>
        <p className="muted text-sm">
          Señala las cuentas que siguen abiertas después de muchas horas, por ejemplo una que se
          quedó sin cobrar o sin cancelar.
        </p>
      </div>
      <Notice>{resource.error}</Notice>
      {resource.data && (
        <LongOpenHoursForm
          key={resource.data.revision}
          settings={resource.data}
          editable={editable}
          onSaved={resource.reload}
        />
      )}
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
          operación ni admite registros nuevos; desactivarla nunca borra ni altera los registros que
          ya generó.
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
      <LongOpenHoursSection editable={editable} />
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
        {/* Etapa 9C (§2.1): hora y minutos con el selector, de 5 en 5; una hora guardada fuera del
            paso (por ejemplo 05:07) se conserva. Go valida de 00:00 a 23:59. */}
        <TimeField
          label="Hora de inicio del día"
          hint="En la zona horaria del restaurante (de 00:00 a 23:59)."
          required
          value={dayStart}
          onChange={setDayStart}
        />
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
  onOpenStatus,
}: {
  can: (key: string) => boolean;
  onSaved: () => Promise<void>;
  // Etapa 9C (§4.1): «Estado del sistema» salió de la navegación y se abre desde aquí.
  onOpenStatus?: () => void;
}) {
  const resource = useResource<Settings>('settings');
  return (
    <>
      <PageTitle title="Configuración general" description="Datos del establecimiento.">
        {onOpenStatus && (
          <button type="button" className="secondary-button" onClick={onOpenStatus}>
            <IconLabel icon="activity">Ver estado del sistema</IconLabel>
          </button>
        )}
      </PageTitle>
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
