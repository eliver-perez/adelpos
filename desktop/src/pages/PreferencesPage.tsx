import { useState } from 'react';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import type { Identity, Preferences } from '../services/models';
import { savePreferences } from '../services/preferences';
import { Select } from '../components/Select';

// Apariencia de un usuario. Desde la etapa 9C la pantalla inicial es de la terminal (Sistema →
// Terminales): el formulario ya no la ofrece y reenvía sin cambios el valor que guarda Go, que
// conserva el campo por compatibilidad y ya no lo usa para abrir la aplicación.
export function UserPreferencesForm({
  value,
  userID,
  onSaved,
}: {
  value: Preferences;
  userID?: string;
  onSaved: () => Promise<void>;
}) {
  const [theme, setTheme] = useState(value.theme);
  return (
    <Form
      successMessage="Preferencias guardadas."
      onSave={async () => {
        // La misma operación que la acción «Tema claro u oscuro» (etapa 9C, fase C).
        await savePreferences({ theme, home_screen: value.home_screen }, userID);
        await onSaved();
      }}
    >
      <div className="form-grid">
        {/* Etapa 9C (§4.4): la apariencia propia del usuario es la que se aplica; la de
            Configuración general solo es la inicial de los usuarios nuevos. */}
        <Field
          label="Apariencia"
          hint="Claro u oscuro. Se aplica en cuanto la guardas y en cualquier terminal donde entres con este usuario."
        >
          <Select<Preferences['theme']>
            value={theme}
            options={[
              { value: 'light', label: 'Claro' },
              { value: 'dark', label: 'Oscuro' },
            ]}
            onChange={setTheme}
          />
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
        description="Tu apariencia se conserva por usuario. El teclado en pantalla se configura en cada terminal."
      />
      <section className="surface panel">
        <h2 className="mb-5">Apariencia personal</h2>
        <UserPreferencesForm value={identity.preferences} onSaved={refresh} />
      </section>
      <section className="surface panel">
        <h2 className="mb-5">Teclado de {identity.terminal.name}</h2>
        <Form
          successMessage="Preferencia de teclado de la terminal guardada."
          onSave={async () => {
            await api('save_terminal_preferences', { body: { keyboard_mode: keyboard } });
            await refresh();
          }}
        >
          <Field
            label="Teclado en pantalla"
            hint="Automático: aparece al escribir si el equipo tiene pantalla táctil (si no, con el botón «Teclado»). Siempre: aparece en cada campo. Desactivado: no aparece."
          >
            <Select<typeof keyboard>
              value={keyboard}
              options={[
                { value: 'auto', label: 'Automático' },
                { value: 'always', label: 'Siempre habilitado' },
                { value: 'disabled', label: 'Desactivado' },
              ]}
              onChange={setKeyboard}
            />
          </Field>
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
        description="La nueva contraseña se guarda de forma segura y sigues trabajando en esta terminal."
      />
      <section className="surface panel">
        <Form
          label="Cambiar contraseña"
          successMessage="Contraseña cambiada."
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
