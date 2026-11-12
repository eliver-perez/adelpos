import { useCallback, useEffect, useRef, useState } from 'react';
import { APIError } from '../../../services/api';
import { previewDocument } from '../../../services/documents';
import type {
  DocumentKind,
  DocumentPreview,
  PaperWidth,
  PreviewSource,
  TemplateLayout,
} from '../../../services/documents';
import { errorMessage } from '../../../services/notify';

// Vista previa «al instante» con las líneas exactas de Go. Cada cambio del borrador programa una
// petición tras un antirrebote corto; nunca hay más de una petición en vuelo: si llegan cambios
// mientras otra está en curso, al terminar se envía solo la más reciente y la respuesta anterior se
// descarta (la última gana). Así escribir en un texto no genera una petición por tecla.

export const previewDelay = 250;

export interface PreviewRequest {
  kind: DocumentKind;
  layout: TemplateLayout;
  paperWidth: PaperWidth;
  // Impresora elegida (etapa 9C): vacía para los anchos predeterminados de 58 u 80 mm.
  printerID?: string;
  source: PreviewSource;
}

export interface PreviewProblem {
  message: string;
  // Bloque señalado por Go (422 TEMPLATE_INVALID); vacío si el problema es del documento.
  blockID: string;
}

function problemOf(error: unknown): PreviewProblem {
  const message = errorMessage(error, 'No se pudo generar la vista previa.');
  if (error instanceof APIError && error.code === 'TEMPLATE_INVALID') {
    const details = error.details as { block_id?: unknown } | undefined;
    const blockID = typeof details?.block_id === 'string' ? details.block_id : '';
    return { message, blockID };
  }
  return { message, blockID: '' };
}

export function usePreview(request: PreviewRequest | null) {
  const [preview, setPreview] = useState<DocumentPreview | null>(null);
  const [problem, setProblem] = useState<PreviewProblem | null>(null);
  const [loading, setLoading] = useState(false);
  const queued = useRef<{ key: string; request: PreviewRequest } | null>(null);
  const inFlight = useRef(false);
  // Clave de la última vista previa pedida: solo su respuesta se muestra.
  const wantedKey = useRef('');
  const mounted = useRef(true);

  const pump = useCallback(async () => {
    if (inFlight.current || !queued.current) {
      return;
    }
    const next = queued.current;
    queued.current = null;
    inFlight.current = true;
    setLoading(true);
    try {
      const result = await previewDocument(
        next.request.kind,
        next.request.layout,
        next.request.paperWidth,
        next.request.source,
        next.request.printerID ?? '',
      );
      // Si mientras tanto cambió el borrador, esta respuesta ya no corresponde: se espera la nueva.
      if (mounted.current && next.key === wantedKey.current) {
        setPreview(result);
        setProblem(null);
      }
    } catch (error) {
      if (mounted.current && next.key === wantedKey.current) {
        setProblem(problemOf(error));
      }
    } finally {
      inFlight.current = false;
      if (mounted.current) {
        if (queued.current) {
          void pump();
        } else {
          setLoading(false);
        }
      }
    }
  }, []);

  // request cambia de identidad en cada render; la clave resume su contenido y el efecto solo
  // reacciona a ella.
  const key = request ? JSON.stringify(request) : '';
  const latest = useRef(request);
  latest.current = request;

  useEffect(() => {
    const current = latest.current;
    if (!current || key === wantedKey.current) {
      return;
    }
    const timer = window.setTimeout(() => {
      wantedKey.current = key;
      queued.current = { key, request: current };
      void pump();
    }, previewDelay);
    return () => window.clearTimeout(timer);
  }, [key, pump]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  // Vuelve a pedir la misma vista previa (por ejemplo, tras cambiar la identidad).
  const refresh = useCallback(() => {
    const current = latest.current;
    if (!current) {
      return;
    }
    const key = JSON.stringify(current);
    wantedKey.current = key;
    queued.current = { key, request: current };
    void pump();
  }, [pump]);

  return { preview, problem, loading, refresh };
}
