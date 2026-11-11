import { useConfirmDialog } from '../../components/ConfirmDialog';
import { dateTime, Notice } from '../../components/Forms';
import { IconLabel } from '../../components/Icon';
import { identityPermissions } from '../../services/documentIdentity';
import { EstablishmentSummary } from './identity/EstablishmentSummary';
import { IdentityTexts } from './identity/IdentityTexts';
import { LogoSection } from './identity/LogoSection';
import { PaperPreview } from './identity/PaperPreview';
import { PresentationControls } from './identity/PresentationControls';
import { useIdentityEditor } from './identity/useIdentityEditor';

type IdentityEditor = ReturnType<typeof useIdentityEditor>;

const actionNames = {
  save: 'guardar la identidad',
  upload: 'cargar el logotipo',
  remove: 'retirar el logotipo',
};

// Pestaña «Identidad del establecimiento» de la pantalla Documentos (etapa 9B, fase B2). La fase C
// la monta con los permisos efectivos de la sesión y la acción para abrir Configuración general.
//
// - Los datos del establecimiento (nombre, dirección, teléfono y correo) se reutilizan de
//   Configuración general y aquí solo se consultan.
// - Información adicional, encabezado, pie y la presentación del logotipo se guardan juntos.
// - El logotipo se carga, reemplaza o retira al momento, con su vista previa en 58 y 80 mm.
// - Sin documents.identity.manage todo es de solo lectura (Go lo vuelve a comprobar).
export function IdentityPanel(props: { permissions: string[]; onOpenSettings: () => void }) {
  const canManage = props.permissions.includes(identityPermissions.manage);
  const editor = useIdentityEditor(canManage);
  const { confirm, dialog } = useConfirmDialog();

  async function confirmRemove() {
    const confirmed = await confirm({
      title: 'Retirar el logotipo',
      message: (
        <>
          <p>
            Los documentos nuevos dejarán de imprimir el logotipo y los reportes usarán el de AD-EL
            POS.
          </p>
          <p>Los documentos ya emitidos conservan su logotipo al reimprimirse.</p>
        </>
      ),
      confirmLabel: 'Retirar logotipo',
      tone: 'danger',
    });
    if (confirmed) {
      await editor.remove();
    }
  }

  if (editor.loading && !editor.identity) {
    return (
      <p className="muted" role="status">
        Cargando la identidad del establecimiento…
      </p>
    );
  }
  if (!editor.identity || !editor.fields) {
    return (
      <div className="form-stack">
        <Notice>{editor.loadError || 'No se pudo cargar la identidad del establecimiento.'}</Notice>
        <div className="button-row">
          <button type="button" className="secondary-button" onClick={() => void editor.reload()}>
            <IconLabel icon="refresh">Reintentar</IconLabel>
          </button>
        </div>
      </div>
    );
  }

  const identity = editor.identity;
  const fields = editor.fields;
  const readOnly = !canManage;
  // Con una escritura en curso o sin confirmar, los textos, la presentación y el logotipo quedan
  // fijos hasta conocer su resultado (el reintento envía exactamente lo que se ve).
  const locked = editor.locked;
  return (
    <div className="form-stack min-w-0" aria-label="Identidad del establecimiento">
      <IdentityIntro readOnly={readOnly} />
      <IdentityStatus editor={editor} />
      <ReloadStatus editor={editor} />
      {identity.warnings.length > 0 && <Notice>{identity.warnings.join(' ')}</Notice>}
      <EstablishmentSummary
        establishment={identity.establishment}
        onOpenSettings={props.onOpenSettings}
      />
      <div className="grid min-w-0 gap-6 xl:grid-cols-2">
        <div className="min-w-0">
          <IdentityTexts
            fields={fields}
            limits={identity.limits}
            readOnly={readOnly || locked}
            onChange={editor.change}
          />
        </div>
        <div className="min-w-0">
          <LogoSection
            logo={identity.logo}
            limits={identity.limits}
            readOnly={readOnly}
            busy={locked}
            onUpload={(file) => void editor.upload(file)}
            onRemove={() => void confirmRemove()}
          />
          <section className="panel form-stack" aria-labelledby="identity-paper-title">
            <div>
              <h2 id="identity-paper-title">Cómo saldrá en papel</h2>
              <p className="muted text-sm">
                Vista previa de los puntos que se envían a la impresora. El resultado en papel es
                una prueba física pendiente: compruébalo con tus impresoras. El logotipo solo se
                imprime por ESC/POS en red o en impresoras CUPS marcadas «Esta impresora entiende
                ESC/POS (formato y logotipo)» cuya cola admita datos raw; en las demás se omite con
                un aviso y el documento se imprime igual.
              </p>
            </div>
            <PresentationControls
              presentation={fields.presentation}
              limits={identity.limits}
              readOnly={readOnly || locked}
              onChange={editor.changePresentation}
            />
            <PaperPreview
              rasters={paperRasters(editor)}
              limits={identity.limits}
              hasLogo={identity.logo !== null}
              status={previewStatus(editor)}
            />
          </section>
        </div>
      </div>
      {canManage && <IdentityActions editor={editor} />}
      {identity.updated_by && (
        <p className="muted text-xs">
          Última modificación: {dateTime(identity.updated_at)} por {identity.updated_by.name}.
        </p>
      )}
      {dialog}
    </div>
  );
}

function IdentityIntro({ readOnly }: { readOnly: boolean }) {
  return (
    <div className="form-stack">
      <p className="muted text-sm">
        La identidad del establecimiento se usa en tickets, precuentas, comandas, comprobantes y
        cortes según los bloques de cada plantilla, y en el encabezado de los reportes PDF y Excel.
        La marca AD-EL POS de la aplicación no cambia.
      </p>
      {readOnly && (
        <Notice>
          Solo lectura: para cambiar la identidad o el logotipo se necesita el permiso «Modificar la
          identidad documental y el logotipo del establecimiento».
        </Notice>
      )}
    </div>
  );
}

// Estado vigente del panel: conflicto de revisión u operación sin respuesta confirmada.
function IdentityStatus({ editor }: { editor: IdentityEditor }) {
  if (editor.conflict) {
    return (
      <div className="notice" role="alert">
        <p>
          <strong>Otra persona guardó la identidad del establecimiento mientras editabas.</strong>{' '}
          Tus cambios siguen en pantalla y no se guardaron.
        </p>
        <div className="button-row mt-3">
          <button
            type="button"
            className="primary-button"
            disabled={editor.busy !== null}
            onClick={() => void editor.keepMine()}
          >
            Guardar mis cambios sobre la versión nueva
          </button>
          <button
            type="button"
            className="secondary-button"
            disabled={editor.busy !== null}
            onClick={editor.discardMine}
          >
            Descartar mis cambios y ver la versión guardada
          </button>
        </div>
      </div>
    );
  }
  if (editor.pending) {
    return (
      <div className="notice" role="alert">
        <p>
          <strong>No se confirmó si se pudo {actionNames[editor.pending.action]}.</strong> Al
          reintentar se usa el mismo identificador de operación: si ya se aplicó, no se repite. Los
          textos, la presentación y el logotipo no se pueden cambiar hasta confirmar el resultado.
        </p>
        <div className="button-row mt-3">
          <button
            type="button"
            className="primary-button"
            disabled={editor.busy !== null}
            onClick={() => void editor.retryPending()}
          >
            <IconLabel icon="refresh">Reintentar</IconLabel>
          </button>
          <button
            type="button"
            className="secondary-button"
            disabled={editor.busy !== null}
            onClick={editor.dismissPending}
          >
            Consultar el estado actual
          </button>
        </div>
      </div>
    );
  }
  return null;
}

// Error al volver a consultar la identidad cuando ya hay una cargada: se sigue mostrando lo último
// que respondió el servidor (también la respuesta de una escritura propia) y se puede reintentar.
function ReloadStatus({ editor }: { editor: IdentityEditor }) {
  if (!editor.loadError) {
    return null;
  }
  return (
    <div className="notice" role="status">
      <p>
        <strong>No se pudo volver a consultar la identidad del establecimiento.</strong>{' '}
        {editor.loadError}
      </p>
      <p className="muted text-sm">
        Se muestra lo último que respondió el servidor; las vistas previas pueden faltar hasta
        volver a cargar.
      </p>
      <div className="button-row mt-3">
        <button type="button" className="secondary-button" onClick={() => void editor.reload()}>
          <IconLabel icon="refresh">Volver a cargar la identidad</IconLabel>
        </button>
      </div>
    </div>
  );
}

function IdentityActions({ editor }: { editor: IdentityEditor }) {
  const saving = editor.busy === 'save';
  return (
    <div className="form-actions">
      <button
        type="button"
        className="primary-button"
        disabled={!editor.dirty || editor.locked || editor.conflict !== null}
        onClick={() => void editor.save()}
      >
        <IconLabel icon="check">{saving ? 'Guardando…' : 'Guardar cambios'}</IconLabel>
      </button>
      <button
        type="button"
        className="secondary-button"
        disabled={!editor.dirty || editor.locked}
        onClick={editor.discardDraft}
      >
        Descartar cambios
      </button>
      <span className="muted text-sm" aria-live="polite">
        {editor.dirty ? 'Hay cambios sin guardar.' : 'Sin cambios pendientes.'}
      </span>
    </div>
  );
}

// Conversiones que se muestran: las de la presentación en edición (vista previa sin guardar) o
// las guardadas.
function paperRasters(editor: IdentityEditor) {
  if (editor.needsPreview) {
    return editor.draftPreview?.result?.rasters ?? [];
  }
  return editor.identity?.logo?.rasters ?? [];
}

function previewStatus(editor: IdentityEditor) {
  if (!editor.identity?.logo) {
    return 'Sin logotipo: el bloque «Logotipo» no imprime nada en ningún documento.';
  }
  if (!editor.needsPreview) {
    return 'Presentación guardada.';
  }
  if (!editor.draftPreview) {
    return 'Preparando la vista previa…';
  }
  if (editor.draftPreview.error) {
    return editor.draftPreview.error;
  }
  return 'Vista previa sin guardar: guarda los cambios para usar esta presentación.';
}
