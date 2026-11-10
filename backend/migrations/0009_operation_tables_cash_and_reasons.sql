-- Etapa 7.1: ajustes de operación, mesas, caja y motivos. Es la única migración de la etapa: la
-- fase C (caja) aporta las dos primeras secciones y la fase D agregará las suyas al final.
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0008 ni
-- filas existentes y no siembra datos ficticios.

-- 1. Cajón de dinero (fase C): «Probar apertura» desde la configuración registra su comando con el
-- motivo test. El CHECK de motivos tiene nombre explícito desde la 0008; se reemplaza con el mismo
-- nombre y conserva los motivos anteriores, de modo que las filas existentes siguen siendo válidas.
ALTER TABLE app.cash_drawer_events DROP CONSTRAINT cash_drawer_events_reason_check;
ALTER TABLE app.cash_drawer_events
    ADD CONSTRAINT cash_drawer_events_reason_check
        CHECK (reason IN ('payment', 'refund', 'operation', 'manual', 'test'));

-- 2. Rechazos por efectivo insuficiente (fase C): cada retiro o gasto rechazado queda en
-- audit_events con la acción cash.outflow_rejected, la entidad cash_shift y success=false. La
-- consulta por turno (cash.expected.read) usa este índice parcial, que solo contiene esas filas.
CREATE INDEX audit_events_cash_outflow_rejected_idx
    ON app.audit_events (entity_id, occurred_at)
    WHERE action = 'cash.outflow_rejected';
