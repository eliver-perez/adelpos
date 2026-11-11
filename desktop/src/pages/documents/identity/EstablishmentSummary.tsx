import { IconLabel } from '../../../components/Icon';
import type { IdentityEstablishment } from '../../../services/documentIdentity';

// Datos del establecimiento que la identidad reutiliza de Configuración general. Aquí solo se
// consultan: se editan en su pantalla, para no duplicar campos.
export function EstablishmentSummary({
  establishment,
  onOpenSettings,
}: {
  establishment: IdentityEstablishment;
  onOpenSettings: () => void;
}) {
  const rows = [
    { label: 'Nombre comercial', value: establishment.name },
    { label: 'Dirección', value: establishment.address },
    { label: 'Teléfono', value: establishment.phone },
    { label: 'Correo', value: establishment.email },
  ];
  return (
    <section className="panel form-stack" aria-labelledby="identity-establishment-title">
      <div className="section-heading">
        <div>
          <h2 id="identity-establishment-title">Datos del establecimiento</h2>
          <p className="muted text-sm">
            Se toman de Configuración general y se imprimen con los bloques de nombre, dirección,
            teléfono y correo.
          </p>
        </div>
        <button type="button" className="secondary-button" onClick={onOpenSettings}>
          <IconLabel icon="edit">Editar en Configuración general</IconLabel>
        </button>
      </div>
      <dl className="grid gap-3 sm:grid-cols-2">
        {rows.map((row) => (
          <div key={row.label} className="field">
            <dt>{row.label}</dt>
            <dd className="read-only-value break-words">
              {row.value.trim() === '' ? (
                <span className="muted">Sin capturar</span>
              ) : (
                <span className="whitespace-pre-line">{row.value}</span>
              )}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
