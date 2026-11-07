import { useState } from 'react';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import type { Settings } from '../services/models';

export const initialSettings: Settings = {
  name: '',
  timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
  currency: 'MXN',
  phone: '',
  email: '',
  address: '',
  theme: 'light',
};

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
      <Field label="Zona horaria" hint="Identificador IANA del establecimiento.">
        <input
          required
          list="timezones"
          value={value.timezone}
          onChange={(event) => change('timezone', event.target.value)}
        />
        <datalist id="timezones">
          <option value="America/Monterrey" />
          <option value="America/Mexico_City" />
          <option value="America/Cancun" />
          <option value="America/Tijuana" />
          <option value="UTC" />
        </datalist>
      </Field>
      <Field label="Moneda">
        <input readOnly value="MXN" />
      </Field>
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
        <textarea
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
