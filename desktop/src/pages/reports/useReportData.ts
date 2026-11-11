import { useCallback, useEffect, useRef, useState } from 'react';
import { errorMessage } from '../../services/notify';

// Carga de un reporte o del resumen ejecutivo. A diferencia de useResource/useLiveResource, **no**
// escucha los eventos SSE (§0-B, «Rendimiento»): se consulta al abrir, al cambiar periodo, filtros,
// orden o página, y cuando la persona pulsa «Actualizar» (el resumen agrega un temporizador
// moderado). Mientras se vuelve a consultar se conserva el resultado anterior (sin parpadeo); al
// cambiar de reporte («scope») se descarta, para no mostrar columnas de otro reporte.
//
// «request» identifica la consulta (null = no consultar): cuando cambia, se consulta de nuevo.

export function useReportData<T>(request: string | null, scope: string, load: () => Promise<T>) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadedScope, setLoadedScope] = useState(scope);
  const generation = useRef(0);
  const loader = useRef(load);
  loader.current = load;

  // Estado derivado: otro reporte descarta el resultado anterior antes de pintar.
  if (loadedScope !== scope) {
    setLoadedScope(scope);
    setData(null);
    setError('');
  }

  const reload = useCallback(async () => {
    if (request === null) {
      setLoading(false);
      return;
    }
    const attempt = ++generation.current;
    setLoading(true);
    setError('');
    try {
      const result = await loader.current();
      if (attempt === generation.current) {
        setData(result);
      }
    } catch (failure) {
      if (attempt === generation.current) {
        setError(errorMessage(failure, 'No se pudo consultar el reporte.'));
      }
    } finally {
      if (attempt === generation.current) {
        setLoading(false);
      }
    }
  }, [request]);

  useEffect(() => {
    void reload();
    return () => {
      // Una respuesta que llegue después de cambiar la consulta se descarta.
      generation.current++;
    };
  }, [reload]);

  return { data, error, loading, reload };
}
