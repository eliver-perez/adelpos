import { useState } from 'react';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import type { Identity, Preferences } from '../services/models';

export const screens = [
  { id: 'kitchen', label: 'Cocina y preparación', permission: 'kitchen.states.read' },
  { id: 'restaurant', label: 'Restaurante', permission: 'accounts.read' },
  { id: 'takeaway', label: 'Para llevar', permission: 'takeaway.read' },
  { id: 'catalog', label: 'Catálogo', permission: 'catalog.read' },
  { id: 'preview', label: 'Probar personalización', permission: 'catalog.prices.read' },
  { id: 'status', label: 'Estado del sistema', permission: '' },
  { id: 'users', label: 'Usuarios', permission: 'users.read' },
  { id: 'roles', label: 'Roles', permission: 'roles.read' },
  { id: 'devices', label: 'Terminales', permission: 'devices.read' },
  { id: 'registers', label: 'Cajas físicas', permission: 'registers.read' },
  { id: 'sessions', label: 'Sesiones', permission: 'sessions.read' },
  { id: 'settings', label: 'Configuración', permission: 'settings.read' },
  { id: 'audit', label: 'Auditoría', permission: 'audit.read' },
  { id: 'preferences', label: 'Mis preferencias', permission: '' },
];

export function UserPreferencesForm({
  value,
  userID,
  permissions,
  onSaved,
}: {
  value: Preferences;
  userID?: string;
  permissions?: string[];
  onSaved: () => Promise<void>;
}) {
  const [theme, setTheme] = useState(value.theme);
  const [home, setHome] = useState(value.home_screen);
  const available = screens.filter(
    (screen) => !permissions || !screen.permission || permissions.includes(screen.permission),
  );
  return (
    <Form
      onSave={async () => {
        await api(userID ? 'save_user_preferences' : 'save_preferences', {
          id: userID,
          body: { theme, home_screen: home },
        });
        await onSaved();
      }}
    >
      <div className="form-grid">
        <Field label="Tema">
          <select
            value={theme}
            onChange={(event) => setTheme(event.target.value as Preferences['theme'])}
          >
            <option value="light">Claro</option>
            <option value="dark">Oscuro</option>
          </select>
        </Field>
        <Field
          label="Pantalla inicial"
          hint="Si deja de estar permitida, se utilizará Estado del sistema."
        >
          <select value={home} onChange={(event) => setHome(event.target.value)}>
            {available.map((screen) => (
              <option value={screen.id} key={screen.id}>
                {screen.label}
              </option>
            ))}
          </select>
        </Field>
      </div>
    </Form>
  );
}

export function OtherUserPreferences({
  userID,
  onSaved,
}: {
  userID: string;
  onSaved: () => Promise<void>;
}) {
  const preferences = useResource<Preferences>('user_preferences', { id: userID });
  return (
    <>
      <Notice>{preferences.error}</Notice>
      {preferences.data && (
        <UserPreferencesForm
          key={userID}
          userID={userID}
          value={preferences.data}
          onSaved={onSaved}
        />
      )}
    </>
  );
}

export function PreferencesPage({
  identity,
  refresh,
}: {
  identity: Identity;
  refresh: () => Promise<void>;
}) {
  const [keyboard, setKeyboard] = useState(identity.terminal.keyboard_mode);
  return (
    <>
      <PageTitle
        title="Mis preferencias"
        description="Tu apariencia se conserva por usuario. El teclado se configura por terminal."
      />
      <section className="surface panel">
        <h2 className="mb-5">Apariencia personal</h2>
        <UserPreferencesForm
          value={identity.preferences}
          permissions={identity.user.permissions}
          onSaved={refresh}
        />
      </section>
      <section className="surface panel">
        <h2 className="mb-5">Teclado de {identity.terminal.name}</h2>
        <Form
          onSave={async () => {
            await api('save_terminal_preferences', { body: { keyboard_mode: keyboard } });
            await refresh();
          }}
        >
          <Field label="Teclado virtual para formularios futuros">
            <select
              value={keyboard}
              onChange={(event) => setKeyboard(event.target.value as typeof keyboard)}
            >
              <option value="auto">Automático</option>
              <option value="always">Siempre habilitado</option>
              <option value="disabled">Desactivado</option>
            </select>
          </Field>
          <p className="muted text-sm">
            Esta etapa guarda la preferencia. El teclado flotante se incorporará con los formularios
            que lo necesiten.
          </p>
        </Form>
      </section>
    </>
  );
}

export function PasswordPage({ refresh }: { refresh: () => Promise<void> }) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirmation, setConfirmation] = useState('');
  return (
    <>
      <PageTitle
        title="Cambiar contraseña"
        description="La nueva credencial se guarda como hash seguro y reemplaza tu sesión actual."
      />
      <section className="surface panel">
        <Form
          label="Cambiar contraseña"
          onSave={async () => {
            if (next !== confirmation) {
              throw new Error('La confirmación no coincide.');
            }
            try {
              await api('change_password', {
                body: { current_password: current, new_password: next },
              });
            } finally {
              setCurrent('');
              setNext('');
              setConfirmation('');
            }
            await refresh();
          }}
        >
          <Field label="Contraseña actual">
            <input
              required
              type="password"
              autoComplete="current-password"
              value={current}
              onChange={(event) => setCurrent(event.target.value)}
            />
          </Field>
          <div className="form-grid">
            <Field label="Nueva contraseña">
              <input
                required
                type="password"
                autoComplete="new-password"
                minLength={8}
                maxLength={256}
                value={next}
                onChange={(event) => setNext(event.target.value)}
              />
            </Field>
            <Field label="Confirmación">
              <input
                required
                type="password"
                autoComplete="new-password"
                value={confirmation}
                onChange={(event) => setConfirmation(event.target.value)}
              />
            </Field>
          </div>
        </Form>
      </section>
    </>
  );
}
