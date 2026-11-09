import { useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { TextareaHTMLAttributes } from 'react';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import type { Settings } from '../services/models';
import { filterTimezones, timezoneLabel, timezoneOptions } from '../services/timezones';

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
    </>
  );
}
