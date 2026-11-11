import { dateTime } from '../../../components/Forms';
import { IconLabel } from '../../../components/Icon';
import type { IdentityLimits, IdentityLogo } from '../../../services/documentIdentity';

// Tamaño del archivo para mostrarlo (kilobytes con un decimal); el límite lo valida Go.
function fileSize(bytes: number) {
  if (bytes < 1024) {
    return `${bytes} bytes`;
  }
  return `${(bytes / 1024).toLocaleString('es-MX', { maximumFractionDigits: 1 })} KB`;
}

function formatName(mediaType: string) {
  return mediaType === 'image/jpeg' ? 'JPEG' : 'PNG';
}

// Logotipo del restaurante: subir, reemplazar, retirar (con confirmación) y ver el original.
//
// El selector de archivos nativo muestra textos del navegador en inglés: queda oculto dentro de
// una etiqueta con aspecto de botón, como en las imágenes del catálogo.
export function LogoSection({
  logo,
  limits,
  readOnly,
  busy,
  onUpload,
  onRemove,
}: {
  logo: IdentityLogo | null;
  limits: IdentityLimits;
  readOnly: boolean;
  busy: boolean;
  onUpload: (file: File) => void;
  onRemove: () => void;
}) {
  return (
    <section className="panel form-stack" aria-labelledby="identity-logo-title">
      <div>
        <h2 id="identity-logo-title">Logotipo del restaurante</h2>
        <p className="muted text-sm">
          Se imprime en los documentos con el bloque «Logotipo» y aparece en el encabezado de los
          reportes PDF y Excel. La marca AD-EL POS de la aplicación no cambia.
        </p>
      </div>
      <div className="grid gap-5 md:grid-cols-[minmax(0,220px)_minmax(0,1fr)]">
        <div className="grid min-h-[140px] place-items-center rounded-xl border border-dashed border-[var(--border)] bg-white p-3">
          {logo?.data_uri ? (
            <img
              src={logo.data_uri}
              alt="Logotipo original del restaurante"
              className="max-h-[160px] max-w-full object-contain"
            />
          ) : (
            <span className="text-center text-sm text-[#5b6675]">
              {logo ? 'Original no disponible' : 'Sin logotipo'}
            </span>
          )}
        </div>
        <div className="form-stack content-start">
          {logo ? (
            <dl className="grid gap-1 text-sm">
              <div>
                <dt className="inline font-semibold">Archivo: </dt>
                <dd className="inline">
                  {formatName(logo.media_type)} · {logo.width} × {logo.height} píxeles ·{' '}
                  {fileSize(logo.size_bytes)}
                </dd>
              </div>
              <div>
                <dt className="inline font-semibold">Cargado: </dt>
                <dd className="inline">
                  {dateTime(logo.uploaded_at)}
                  {logo.uploaded_by ? ` por ${logo.uploaded_by.name}` : ''}
                </dd>
              </div>
            </dl>
          ) : (
            <p className="text-sm">
              <strong>Sin logotipo.</strong> El bloque «Logotipo» de las plantillas no imprime nada
              y los reportes usan el logotipo de AD-EL POS.
            </p>
          )}
          <p className="muted text-xs">
            PNG o JPEG de hasta {Math.round(limits.logo_max_bytes / (1024 * 1024))} MiB y{' '}
            {limits.logo_max_dimension} × {limits.logo_max_dimension} píxeles. Funciona mejor un
            logotipo oscuro sobre fondo blanco o transparente; la transparencia se imprime en
            blanco.
          </p>
          {!readOnly && (
            <div className="button-row">
              <label className={`secondary-button file-button ${busy ? 'is-disabled' : ''}`}>
                <input
                  type="file"
                  className="visually-hidden"
                  accept="image/png,image/jpeg"
                  aria-label={logo ? 'Reemplazar logotipo' : 'Subir logotipo'}
                  disabled={busy}
                  onChange={(event) => {
                    const file = event.target.files?.[0];
                    // Se limpia para permitir elegir otra vez el mismo archivo.
                    event.target.value = '';
                    if (file) {
                      onUpload(file);
                    }
                  }}
                />
                <IconLabel icon="plus">{logo ? 'Reemplazar logotipo' : 'Subir logotipo'}</IconLabel>
              </label>
              {logo && (
                <button type="button" className="danger-button" disabled={busy} onClick={onRemove}>
                  <IconLabel icon="trash">Retirar logotipo</IconLabel>
                </button>
              )}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
