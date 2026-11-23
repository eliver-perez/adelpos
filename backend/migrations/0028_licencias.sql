-- Etapa L1: la única migración de la etapa (integración con el servidor de licencias; esquema 28). Cada fase agrega
-- aquí su propia sección con el encabezado «Fase X», en el orden «Fase A (revisión R1)», «Fase B», «Fase D». Es
-- transaccional (el migrador aplica todas las pendientes en una sola transacción). No toca ventas, pagos, cortes,
-- cuentas, pedidos, sesiones, usuarios ni roles. La licencia, su llave y la última hora confiable no van en la base
-- sino en identity/ (decisión L1-D9 del coordinador en docs/ETAPA_L1_AVANCE.md): identity/ nunca viaja en un
-- respaldo. La base solo guarda una segunda copia de las revisiones aceptadas (L1-D29) y el permiso.

-- =====================================================================================================================
-- Fase A (revisión R1) · Segunda copia del historial de revisiones aceptadas (hallazgo R1-3; decisión L1-D29)
-- =====================================================================================================================

-- 1. La revisión más alta aceptada de cada licencia (contrato §10.4) vive en identity/licencia/estado.json (L1-D9).
--    Esta tabla es una segunda copia: la revisión mínima aceptable es el máximo de las dos. Si se restaura un respaldo
--    viejo de la base, manda la de identity/; si se borra o se edita estado.json, manda la de la base. Las dos se
--    escriben al aceptar una licencia, primero esta (si falla, la licencia no se acepta). Nunca baja: el servidor solo
--    la sube (internal/licensing, RecordAcceptedRevision). No guarda la licencia ni el código comercial.
CREATE TABLE app.license_revisions (
    license_id uuid CONSTRAINT license_revisions_pkey PRIMARY KEY,
    license_revision bigint NOT NULL
        CONSTRAINT license_revisions_revision_check CHECK (license_revision >= 1),
    sha256 text NOT NULL
        CONSTRAINT license_revisions_sha256_check CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    accepted_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
COMMENT ON TABLE app.license_revisions IS
    'Segunda copia de la revisión más alta aceptada de cada licencia (etapa L1, revisión R1; la primera está en identity/licencia)';

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
