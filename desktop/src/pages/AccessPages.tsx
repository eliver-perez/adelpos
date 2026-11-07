import { useState } from 'react';
import { Field, Form, Notice } from '../components/Forms';
import { api } from '../services/api';
import { BrandLogo } from '../components/BrandLogo';
import product from '../generated/product.json';
import { initialSettings, SettingsFields } from './SettingsPage';

export function AccessFrame({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <main className="access-screen">
      <div className="access-card surface">
        <div className="mb-7 flex items-center gap-3">
          <BrandLogo />
          <span className="muted text-xs">v{product.version}</span>
        </div>
        <h1>{title}</h1>
        <p className="muted mb-7 mt-2 text-sm">{description}</p>
        {children}
      </div>
    </main>
  );
}

export function SetupPage({
  available,
  refresh,
}: {
  available: boolean;
  refresh: () => Promise<void>;
}) {
  const [settings, setSettings] = useState({ ...initialSettings, name: 'ADEL POS' });
  const [terminalName, setTerminalName] = useState('Terminal principal');
  return (
    <AccessFrame
      title="Prepara tu establecimiento"
      description="Esta configuración se realiza una sola vez desde el equipo servidor."
    >
      {available ? (
        <Form
          label="Configurar establecimiento"
          onSave={async () => {
            await api('setup', { body: { ...settings, terminal_name: terminalName } });
            await refresh();
          }}
        >
          <SettingsFields value={settings} onChange={setSettings} />
          <Field label="Nombre de esta terminal">
            <input
              required
              maxLength={80}
              value={terminalName}
              onChange={(event) => setTerminalName(event.target.value)}
            />
          </Field>
          <p className="muted text-sm">
            Se creará Caja 1 como caja física. No se abrirá un turno ni se registrarán fondos.
            Después podrás acceder con admin / admin.
          </p>
        </Form>
      ) : (
        <Notice>
          Prepara el acceso local con <code>./scripts/dev.sh prepare-setup</code> y abre la
          aplicación con <code>./scripts/dev.sh desktop</code> o{' '}
          <code>./scripts/dev.sh open-app</code>. La configuración anónima remota está
          deshabilitada.
        </Notice>
      )}
    </AccessFrame>
  );
}

export function LoginPage({ message, refresh }: { message: string; refresh: () => Promise<void> }) {
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  return (
    <AccessFrame
      title="Inicia sesión"
      description="Accede a las funciones autorizadas para tu usuario."
    >
      <Notice>{message}</Notice>
      <Form
        label="Entrar"
        onSave={async () => {
          try {
            await api('login', { body: { username, password } });
          } finally {
            setPassword('');
          }
          await refresh();
        }}
      >
        <Field label="Usuario">
          <input
            autoFocus
            required
            autoComplete="username"
            maxLength={40}
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
        </Field>
        <Field label="Contraseña">
          <input
            required
            type="password"
            autoComplete="current-password"
            maxLength={256}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </Field>
      </Form>
    </AccessFrame>
  );
}

export function EnrollmentPage({
  message,
  refresh,
  localAvailable = false,
}: {
  message: string;
  refresh: () => Promise<void>;
  localAvailable?: boolean;
}) {
  const [name, setName] = useState('');
  const [code, setCode] = useState('');
  return (
    <AccessFrame
      title="Autoriza esta terminal"
      description="Un administrador debe generar un código de un solo uso desde Terminales."
    >
      <Notice>{message}</Notice>
      <Form
        label="Autorizar terminal"
        onSave={async () => {
          try {
            await api('enroll', { body: { name, code: code.trim() } });
          } finally {
            setCode('');
          }
          await refresh();
        }}
      >
        <Field label="Nombre de terminal">
          <input
            required
            maxLength={80}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <Field
          label="Código de autorización"
          hint="Expira a los 10 minutos y solo puede utilizarse una vez."
        >
          <input
            required
            type="password"
            autoComplete="off"
            value={code}
            onChange={(event) => setCode(event.target.value)}
          />
        </Field>
      </Form>
      {localAvailable && (
        <section className="mt-6">
          <Form
            label="Usar autorización local"
            onSave={async () => {
              await api('enroll_local', { body: { name } });
              await refresh();
            }}
          >
            <p className="muted text-sm">
              Hay una autorización temporal emitida por el propietario en este equipo. Indica arriba
              el nombre de la terminal antes de utilizarla.
            </p>
          </Form>
        </section>
      )}
    </AccessFrame>
  );
}
