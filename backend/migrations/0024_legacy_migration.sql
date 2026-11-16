-- Etapa 9H: la única migración de la etapa (migración del sistema anterior y ajustes pendientes; esquema 24). Cada fase
-- agrega aquí su propia sección con el encabezado «Fase X». Es transaccional (el migrador aplica todas las pendientes
-- en una sola transacción) y no modifica filas operativas: ventas, pagos, cortes, cuentas, pedidos, sesiones, trabajos
-- de impresión ni sus fotografías.

-- =====================================================================================================================
-- Fase A · Pendientes y ajustes (§2 de la especificación)
-- =====================================================================================================================

-- 1. Sesiones en varios dispositivos (§2.4, decisión del propietario): un usuario puede tener una sesión activa en cada
--    terminal a la vez (por ejemplo, una caja y un teléfono). Desde la 0002, sessions_one_active_user_idx permitía una
--    sola sesión vigente por usuario en todo el sistema e iniciar sesión en una terminal cerraba las de las demás. Ahora
--    el índice es por usuario y terminal: iniciar sesión solo cierra la sesión anterior del usuario en esa misma
--    terminal (administration/auth.go, issueSession). El índice nuevo es más permisivo que el anterior, así que las
--    sesiones existentes lo cumplen; ninguna se cierra ni se modifica.
DROP INDEX app.sessions_one_active_user_idx;
CREATE UNIQUE INDEX sessions_one_active_user_device_idx ON app.sessions (user_id, device_id) WHERE revoked_at IS NULL;
