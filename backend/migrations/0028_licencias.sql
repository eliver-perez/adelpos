-- Etapa L1: la única migración de la etapa (integración con el servidor de licencias; esquema 28). Cada fase agrega
-- aquí su propia sección con el encabezado «Fase X», en el orden «Fase B», «Fase D». Es transaccional (el migrador
-- aplica todas las pendientes en una sola transacción). No toca ventas, pagos, cortes, cuentas, pedidos, sesiones,
-- usuarios ni roles. La licencia, su llave y la última hora confiable no van en la base sino en identity/ (decisión
-- L1-D9 del coordinador en docs/ETAPA_L1_AVANCE.md): identity/ nunca viaja en un respaldo.

-- =====================================================================================================================
-- Fase B · Cliente del servidor de licencias (§3 y §4 de la especificación; decisiones L1-D21 y L1-D23)
-- =====================================================================================================================

-- 1. «Administrar la licencia» (L1-D21): activar, transferir, buscar actualización, liberar, la solicitud sin internet,
--    importar un archivo de licencia y crear una identidad nueva para una instalación copiada. Ver el estado de la
--    licencia no pide permiso. Solo lo recibe el rol Administrador de fábrica (administrator): ningún otro rol de
--    fábrica lo lleva, tampoco el alcance de mesero. En una instalación nueva los roles nacen en el asistente, después
--    de las migraciones, y el asistente le da al administrador todos los permisos (setup.go). Se registra también en Go
--    (administration.Permissions) con la misma descripción.
INSERT INTO app.permissions(key, description) VALUES
    ('license.manage', 'Administrar la licencia');
INSERT INTO app.role_permissions(role_id, permission_key)
SELECT id, 'license.manage' FROM app.roles WHERE code = 'administrator';
