-- Etapa K1.1: la única migración de la etapa (pedidos mixtos, productos sin ruta y configuración guiada de cocina;
-- esquema 30). Cada fase agrega aquí su propia sección con el encabezado «Fase X» (decisión K11-D14). Es transaccional
-- (el migrador aplica todas las pendientes en una sola transacción) y el servicio respalda la base antes de aplicarla.
-- Nunca cambia ventas, pagos, cuentas, partidas, unidades, cortes, inventario ni kitchen_unit_releases: solo agrega dos
-- columnas de cocina y da por terminados, con su historial, los pendientes de cocina de cuentas que ya no están
-- abiertas.

-- =====================================================================================================================
-- Fase A · Servidor: productos sin ruta y pendientes de liberación (§2 y §3 de la especificación)
-- =====================================================================================================================

-- 1. Ajuste «Productos sin ruta» (§2; decisión K11-D3): a dónde va lo que no tiene ruta de preparación (o lo que solo
--    iba a estaciones desactivadas). NULL es «No necesitan preparación», el valor de siempre al actualizar (no van a
--    ninguna estación ni detienen nada); una estación es «Enviar a una estación». Se guarda con PUT kitchen/unrouted
--    (kitchen.routing.manage), con su propio valor esperado, y no es parte de la configuración que viaja en cada
--    comanda. Si la estación se desactiva, lo sin ruta no necesita preparación mientras tanto; «Quitar datos de prueba»
--    la vuelve NULL antes de borrar una estación de prueba.
ALTER TABLE app.kitchen_settings
    ADD COLUMN unrouted_station_id uuid REFERENCES app.kitchen_stations(id);

-- 2. Un aviso pendiente de cocina ya terminado (§3; decisión K11-D4): el de una cuenta que se cerró o se canceló, o que
--    ya no estaba abierta al actualizar. Se conserva con la hora y el motivo (account_closed, account_cancelled o
--    account_not_open) en lugar de borrarse; los lectores de avisos pendientes solo ven los que no tienen dismissed_at.
ALTER TABLE app.kitchen_pending_changes
    ADD COLUMN dismissed_at timestamptz,
    ADD COLUMN dismissed_reason text,
    ADD CONSTRAINT kitchen_pending_changes_dismissed_check CHECK ((dismissed_at IS NULL) = (dismissed_reason IS NULL));

-- 3. Lo que ya existe (§3, «Lo que ya está en la Mac del propietario»; decisión K11-D4): las liberaciones pendientes,
--    programadas o bloqueadas cuyas unidades activas ya no están en ninguna cuenta abierta (por la pertenencia actual
--    de las unidades: lo combinado, dividido o transferido a una cuenta abierta sigue pendiente; un pedido para llevar
--    cobrado y sin entregar sigue abierto) pasan a «cancelled» con su revisión siguiente. Sus advertencias y sus
--    fotografías quedan como estaban. Cada una deja una fila release.dismissed en el historial de cocina, sin actor.
--    Lo que nunca se envió tampoco consumió existencias, y darlo por terminado no registra ese consumo (K11-D11).
WITH stale AS (
    SELECT r.id, r.state, r.warnings
    FROM app.kitchen_releases r
    WHERE r.state IN ('pending', 'scheduled', 'blocked')
        AND NOT EXISTS (
            SELECT FROM app.restaurant_item_units u
            JOIN app.restaurant_accounts a ON a.id = u.account_id
            WHERE u.id = ANY (r.unit_ids) AND u.state = 'active' AND a.state = 'open'
        )
),
dismissed AS (
    UPDATE app.kitchen_releases r
    SET state = 'cancelled', revision = r.revision + 1
    FROM stale
    WHERE r.id = stale.id
    RETURNING r.id, stale.state AS previous_state, stale.warnings AS previous_warnings
)
INSERT INTO app.kitchen_history (id, entity_id, kind, before_value, after_value)
SELECT gen_random_uuid(), id, 'release.dismissed',
    jsonb_build_object('state', previous_state, 'warnings', previous_warnings),
    jsonb_build_object('state', 'cancelled', 'reason', 'account_not_open', 'migration', '0030')
FROM dismissed;

-- 4. Los avisos pendientes (modificación o cancelación para una estación desactivada) que ya no son de una cuenta
--    abierta: ninguna de sus unidades activas está en una cuenta abierta y su cuenta (o aquella en la que terminó
--    combinada, también en cadena) tampoco lo está. Reciben dismissed_at con el motivo account_not_open y una fila
--    change.dismissed en el historial, sin actor.
WITH stale AS (
    SELECT c.event_id, c.kind, c.warnings
    FROM app.kitchen_pending_changes c
    WHERE NOT EXISTS (
            SELECT FROM app.restaurant_item_units u
            JOIN app.restaurant_accounts a ON a.id = u.account_id
            WHERE u.state = 'active' AND a.state = 'open'
                AND u.id::text IN (
                    SELECT jsonb_array_elements_text(
                        coalesce(c.payload -> 'modified_unit_ids', '[]') || coalesce(c.payload -> 'added_unit_ids', '[]')
                        || coalesce(c.payload -> 'cancelled_unit_ids', '[]') || coalesce(c.payload -> 'unit_ids', '[]')
                    )
                )
        )
        AND NOT EXISTS (
            WITH RECURSIVE chain (id, depth) AS (
                SELECT c.account_id, 0
                UNION ALL
                SELECT a.merged_into_id, chain.depth + 1
                FROM chain JOIN app.restaurant_accounts a ON a.id = chain.id
                WHERE a.state = 'merged' AND a.merged_into_id IS NOT NULL AND chain.depth < 32
            )
            SELECT FROM chain JOIN app.restaurant_accounts a ON a.id = chain.id WHERE a.state = 'open'
        )
),
dismissed AS (
    UPDATE app.kitchen_pending_changes c
    SET dismissed_at = now(), dismissed_reason = 'account_not_open'
    FROM stale
    WHERE c.event_id = stale.event_id
    RETURNING c.event_id, stale.kind, stale.warnings
)
INSERT INTO app.kitchen_history (id, entity_id, kind, before_value, after_value)
SELECT gen_random_uuid(), event_id, 'change.dismissed',
    jsonb_build_object('kind', kind, 'warnings', warnings),
    jsonb_build_object('dismissed', true, 'reason', 'account_not_open', 'migration', '0030')
FROM dismissed;
