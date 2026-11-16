import { useEffect, useState } from 'react';
import { IconLabel } from '../../components/Icon';
import { readLegacyStatus } from '../../services/legacyMigration';
import type { LegacyBatch } from '../../services/legacyMigration';
import { BatchFacts } from './LegacySummary';

// Sistema › Mantenimiento › «Migrar del sistema anterior» (etapa 9H, fase D): dice si ya hay un lote
// importado y abre el asistente. Solo en la computadora principal; en una terminal lo explica.
export function LegacyMigrationSection({
  mainComputer,
  onOpen,
}: {
  mainComputer: boolean;
  onOpen: () => void;
}) {
  const [batch, setBatch] = useState<LegacyBatch | null>(null);
  useEffect(() => {
    if (!mainComputer) {
      return;
    }
    let active = true;
    readLegacyStatus()
      .then((status) => {
        if (active) {
          setBatch(status.current_batch);
        }
      })
      .catch(() => {
        // Sin el estado (por ejemplo, sin conexión) la sección sigue ofreciendo abrir el asistente.
      });
    return () => {
      active = false;
    };
  }, [mainComputer]);
  return (
    <section className="surface panel maintenance-panel" aria-labelledby="maintenance-legacy-title">
      <div className="section-heading">
        <h2 id="maintenance-legacy-title">Migrar del sistema anterior</h2>
      </div>
      {mainComputer ? (
        <>
          <p className="muted maintenance-help">
            Prepara un paquete con el respaldo del sistema anterior e impórtalo con todo su
            historial. Se puede repetir: cada importación reemplaza completa la anterior.
          </p>
          {batch && (
            <div className="legacy-current">
              <strong>Ya hay datos importados del sistema anterior.</strong>
              <BatchFacts batch={batch} />
            </div>
          )}
          <div className="button-row">
            <button type="button" className="primary-button" onClick={onOpen}>
              <IconLabel icon="database">Abrir el asistente de migración</IconLabel>
            </button>
          </div>
        </>
      ) : (
        <p className="muted maintenance-help">
          La migración del sistema anterior se hace en la computadora principal del restaurante.
        </p>
      )}
    </section>
  );
}
