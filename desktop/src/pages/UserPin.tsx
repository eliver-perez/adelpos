import { useState } from 'react';
import { ColorField } from '../components/catalog/ColorField';
import { useConfirmDialog } from '../components/ConfirmDialog';
import { Field, Form, Notice, PageTitle } from '../components/Forms';
import { useResource } from '../hooks/useResource';
import { api } from '../services/api';
import type { PaletteColor } from '../services/catalog';
import type { User } from '../services/models';
import { notifyError, notifySuccess } from '../services/notify';

// PIN de los meseros (etapa A1, fase B). Sirve solo para entrar en los teléfonos y tabletas de meseros; las reglas
// (4 a 6 dígitos, nada de PIN triviales, bloqueo por intentos) son de Go y aquí solo se muestra lo que responde. La
// pantalla nunca recibe ni muestra un PIN guardado. El color del distintivo sale de la paleta que entrega Go
// (GET users/badge-colors): nunca un color escrito a mano.

// Hora corta («14:05») de un bloqueo.
export function clockTime(value: string) {
  return new Date(value).toLocaleTimeString('es-MX', { hour: '2-digit', minute: '2-digit' });
}

// Solo dígitos, como máximo 6 (el PIN se escribe en un campo oculto como una contraseña).
function digitsOnly(value: string) {
  return value.replace(/\D/g, '').slice(0, 6);
}

function PinField({
  label,
  value,
  required,
  onChange,
}: {
  label: string;
  value: string;
  required: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <Field label={label}>
      <input
        required={required}
        type="password"
        inputMode="numeric"
        autoComplete="new-password"
        pattern="[0-9]{4,6}"
        minLength={4}
        maxLength={6}
        title="De 4 a 6 dígitos"
        value={value}
        onChange={(event) => onChange(digitsOnly(event.target.value))}
      />
    </Field>
  );
}

// Estado del PIN en la lista de usuarios: sin PIN, con PIN (y su distintivo), bloqueado unos minutos por intentos
// fallidos o bloqueado hasta que un administrador lo desbloquee.
export function PinStatus({ user }: { user: User }) {
  // A1-D16: tras tres bloqueos seguidos solo lo desbloquea un administrador.
  if (user.pin_requires_unlock) {
    return <span className="tag tag-warning">PIN bloqueado: pide desbloqueo</span>;
  }
  if (user.pin_locked_until) {
    return (
      <span className="tag tag-warning">
        PIN bloqueado hasta las {clockTime(user.pin_locked_until)}
      </span>
    );
  }
  if (!user.has_pin) {
    return <span className="muted">Sin PIN</span>;
  }
  return (
    <span className="users-pin">
      {user.badge_color && (
        <span
          className="color-swatch users-pin-swatch"
          style={{ background: user.badge_color }}
          aria-hidden="true"
        />
      )}
      Con PIN{user.short_name ? ` · ${user.short_name}` : ''}
    </span>
  );
}

// «Desbloquear» un PIN bloqueado por intentos fallidos (users.manage).
export async function unlockUserPIN(user: User, onChanged: () => Promise<void>) {
  try {
    await api('unlock_user_pin', { id: user.id, body: {} });
    notifySuccess(`PIN de ${user.display_name} desbloqueado.`);
  } catch (error) {
    notifyError(error, 'No se pudo desbloquear el PIN.');
  } finally {
    await onChanged();
  }
}

// Asignar o cambiar el PIN de un usuario, con su nombre corto y el color del distintivo, y quitarlo (users.manage).
// Quitar el PIN se puede deshacer asignando otro: el botón es secundario y su confirmación lo explica.
export function UserPinPanel({
  user,
  onChanged,
  onClose,
}: {
  user: User;
  onChanged: () => Promise<void>;
  onClose: () => void;
}) {
  const palette = useResource<PaletteColor[]>('user_badge_colors');
  const [pin, setPin] = useState('');
  const [repeated, setRepeated] = useState('');
  const [shortName, setShortName] = useState(user.short_name ?? '');
  const [color, setColor] = useState(user.badge_color ?? '');
  const [busy, setBusy] = useState(false);
  const { confirm, dialog } = useConfirmDialog();
  const hasPIN = user.has_pin === true;

  async function remove() {
    const confirmed = await confirm({
      title: `Quitar el PIN de ${user.display_name}`,
      message: `${user.display_name} ya no podrá entrar con PIN en los teléfonos y tabletas de meseros y se cerrarán sus sesiones en ellos. Puedes asignarle otro PIN cuando quieras.`,
      confirmLabel: 'Sí, quitar el PIN',
      cancelLabel: 'Conservar el PIN',
    });
    if (!confirmed) {
      return;
    }
    setBusy(true);
    try {
      await api('remove_user_pin', { id: user.id, body: {} });
      notifySuccess(`Se quitó el PIN de ${user.display_name}.`);
      onClose();
    } catch (error) {
      notifyError(error, 'No se pudo quitar el PIN.');
    } finally {
      setBusy(false);
      await onChanged();
    }
  }

  return (
    <section className="surface panel" aria-labelledby="user-pin-title" aria-busy={busy}>
      <div className="section-heading">
        <h2 id="user-pin-title">
          {hasPIN ? 'Cambiar el PIN de' : 'Asignar PIN a'} {user.display_name}
        </h2>
        <button className="secondary-button" onClick={onClose}>
          Cerrar
        </button>
      </div>
      <Notice>{palette.error}</Notice>
      <Form
        label={hasPIN ? 'Guardar' : 'Asignar PIN'}
        successMessage={hasPIN && !pin ? 'Nombre corto y color guardados.' : 'PIN guardado.'}
        onSave={async () => {
          if (pin !== repeated) {
            throw new Error('Los dos PIN no coinciden.');
          }
          try {
            await api('set_user_pin', {
              id: user.id,
              body: {
                ...(pin ? { pin } : {}),
                short_name: shortName,
                badge_color: color,
              },
            });
          } finally {
            setPin('');
            setRepeated('');
          }
          await onChanged();
          onClose();
        }}
      >
        <p className="muted text-sm">
          El PIN sirve para entrar en los teléfonos y tabletas de meseros. Debe tener de 4 a 6
          dígitos y no puede ser fácil de adivinar (como 1234 o 0000).
          {hasPIN ? ' Déjalo vacío para conservar el PIN actual.' : ''}
        </p>
        <div className="form-grid">
          <PinField
            label={hasPIN ? 'PIN nuevo' : 'PIN'}
            value={pin}
            required={!hasPIN}
            onChange={setPin}
          />
          <PinField
            label="Escríbelo otra vez"
            value={repeated}
            required={!hasPIN || pin !== ''}
            onChange={setRepeated}
          />
          <Field
            label="Nombre corto"
            hint="Así aparece en el botón de la pantalla de entrada. Si lo dejas vacío, se usa el nombre visible."
          >
            <input
              maxLength={20}
              value={shortName}
              onChange={(event) => setShortName(event.target.value)}
            />
          </Field>
        </div>
        <ColorField
          label="Color del distintivo"
          value={color}
          palette={palette.data ?? []}
          onChange={setColor}
        />
      </Form>
      {hasPIN && (
        <div className="button-row mt-4">
          <button className="secondary-button" disabled={busy} onClick={() => void remove()}>
            Quitar PIN
          </button>
        </div>
      )}
      {dialog}
    </section>
  );
}

// «Mi PIN» (menú del usuario): el propio usuario define o cambia su PIN con su contraseña actual.
export function MyPinPage({ hasPIN, refresh }: { hasPIN: boolean; refresh: () => Promise<void> }) {
  const [current, setCurrent] = useState('');
  const [pin, setPin] = useState('');
  const [repeated, setRepeated] = useState('');
  return (
    <>
      <PageTitle
        title="Mi PIN"
        description="Tu PIN sirve para entrar en los teléfonos y tabletas de meseros del restaurante."
      />
      <section className="surface panel">
        <Form
          label={hasPIN ? 'Cambiar mi PIN' : 'Guardar mi PIN'}
          successMessage={hasPIN ? 'PIN cambiado.' : 'PIN guardado.'}
          onSave={async () => {
            if (pin !== repeated) {
              throw new Error('Los dos PIN no coinciden.');
            }
            try {
              await api('set_own_pin', { body: { current_password: current, pin } });
            } finally {
              setCurrent('');
              setPin('');
              setRepeated('');
            }
            await refresh();
          }}
        >
          <p className="muted text-sm">
            {hasPIN
              ? 'Ya tienes PIN. Al cambiarlo se cierran tus sesiones en los teléfonos y tabletas; entra otra vez con el PIN nuevo.'
              : 'No tienes PIN.'}{' '}
            Debe tener de 4 a 6 dígitos y no puede ser fácil de adivinar (como 1234 o 0000).
          </p>
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
            <PinField label={hasPIN ? 'PIN nuevo' : 'PIN'} value={pin} required onChange={setPin} />
            <PinField label="Escríbelo otra vez" value={repeated} required onChange={setRepeated} />
          </div>
        </Form>
      </section>
    </>
  );
}
