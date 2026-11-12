import { useState } from 'react';
import { api } from '../../services/api';
import { notifyError, notifySuccess } from '../../services/notify';
import { CatalogImage } from './EditorFields';

// Imagen opcional de categorías y productos.
//
// El selector de archivos nativo muestra textos del navegador en inglés («Choose File / no file
// selected»). El control nativo queda oculto visualmente dentro de una etiqueta con aspecto de
// botón: tocar «Elegir imagen» abre el selector del sistema y el estado se describe en español.
export function ImageEditor({
  id,
  onChange,
}: {
  id: string | null;
  onChange: (id: string | null) => void;
}) {
  const [busy, setBusy] = useState(false);

  // Cargar una imagen es una acción: su resultado se confirma con un aviso. La imagen queda
  // asignada al registro en edición y se conserva al guardarlo.
  function upload(file: File) {
    if (file.size > 1024 * 1024) {
      notifyError('La imagen excede 1 MiB.');
      return;
    }
    setBusy(true);
    const reader = new FileReader();
    reader.onload = () => {
      const content = String(reader.result).split(',')[1];
      void api<{ id: string }>('catalog_upload_image', {
        body: { content_base64: content },
      })
        .then((result) => {
          onChange(result.id);
          notifySuccess('Imagen cargada. Guarda el registro para conservarla.');
        })
        .catch((problem: unknown) => notifyError(problem, 'No se pudo cargar la imagen.'))
        .finally(() => setBusy(false));
    };
    reader.onerror = () => {
      notifyError('No se pudo leer la imagen.');
      setBusy(false);
    };
    reader.readAsDataURL(file);
  }

  let status = 'Sin imagen';
  if (busy) {
    status = 'Cargando imagen…';
  } else if (id) {
    status = 'Imagen asignada';
  }

  return (
    <section className="catalog-section">
      <h3>Imagen opcional</h3>
      <div className="image-editor">
        <CatalogImage id={id} />
        <div className="image-editor-body">
          <p className="image-editor-status">
            <strong>{status}</strong>
          </p>
          <p className="muted text-sm">PNG o JPEG · hasta 1 MiB y 2048 × 2048 píxeles.</p>
          <div className="button-row">
            <label className={`secondary-button file-button ${busy ? 'is-disabled' : ''}`}>
              <input
                type="file"
                className="visually-hidden"
                accept="image/png,image/jpeg"
                disabled={busy}
                onChange={(event) => {
                  const file = event.target.files?.[0];
                  // Se limpia para permitir elegir otra vez el mismo archivo.
                  event.target.value = '';
                  if (file) {
                    upload(file);
                  }
                }}
              />
              {id ? 'Cambiar imagen' : 'Elegir imagen'}
            </label>
            {id && (
              <button
                type="button"
                className="secondary-button"
                disabled={busy}
                onClick={() => onChange(null)}
              >
                Quitar imagen
              </button>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
