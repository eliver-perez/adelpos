-- Etapa A3: la única migración de la etapa (ajustes de Android, permisos agrupados y roles de fábrica; esquema 27).
-- Cada fase agrega aquí su propia sección con el encabezado «Fase X». Es transaccional (el migrador aplica todas las
-- pendientes en una sola transacción). No toca ventas, pagos, cortes, cuentas, pedidos, sesiones, usuarios, roles ni
-- permisos: los roles de fábrica solo se conceden en el asistente de una instalación nueva y con «Restablecer permisos
-- predeterminados» (decisión A3-D2 del coordinador). Solo cambia lo que dice cada sección: la nota de la versión 1 de
-- las plantillas de documentos del sistema (Fase A) y la pantalla inicial de los teléfonos y tabletas de meseros que
-- tenían «Cocina» o «Caja» (Fase D).

-- =====================================================================================================================
-- Fase A · Regla de textos (§1 de la especificación; decisión A3-D12 del coordinador en docs/ETAPA_A3_AVANCE.md)
-- =====================================================================================================================

-- 1. La 0018 guardó como nota de la versión 1 de las diez plantillas del sistema «Formato predeterminado, igual al
--    anterior a la etapa 9B», y Administración › Documentos y el historial del diseñador la muestran: una nota de
--    etapas en la interfaz. Se reescribe solo donde el texto es exactamente ese; el formato, el autor y la fecha de
--    cada versión no cambian (no es un dato de operación). Las versiones son inmutables: el disparador
--    document_template_versions_insert_only de la 0018 rechaza todo UPDATE. Se desactiva solo para esta sentencia y se
--    vuelve a activar enseguida, dentro de la misma transacción: ALTER TABLE toma un bloqueo exclusivo de la tabla
--    hasta el final de la transacción, así que nadie más puede escribir en ella mientras tanto, y si algo falla la
--    transacción se deshace con el disparador activo.
ALTER TABLE app.document_template_versions DISABLE TRIGGER document_template_versions_insert_only;
UPDATE app.document_template_versions
    SET note = 'Formato predeterminado'
    WHERE note = 'Formato predeterminado, igual al anterior a la etapa 9B';
ALTER TABLE app.document_template_versions ENABLE TRIGGER document_template_versions_insert_only;

-- =====================================================================================================================
-- Fase D · Terminales en el escritorio (§5 de la especificación; decisión A3-D7 del coordinador)
-- =====================================================================================================================

-- 2. Un teléfono o tableta de meseros (kind android) solo abre en Mesas o Para llevar: la aplicación no tiene modo
--    cocina ni cobra (§5.1: «Cocina» no se ofrece y cada tipo solo ve lo que le aplica). Los que tenían «Cocina» o
--    «Caja» guardada pasan a «Mesas», que es lo que la aplicación ya abría con cualquiera de las dos. Las terminales de
--    escritorio y de cocina conservan la suya. La restricción repite lo que Go valida en PATCH devices/{id}
--    (administration/devices.go) y se suma a devices_home_screen_check de la 0020, que sigue siendo la lista cerrada de
--    todas las terminales.
UPDATE app.devices
    SET home_screen = 'tables'
    WHERE kind = 'android' AND home_screen IN ('kitchen', 'cash');
ALTER TABLE app.devices
    ADD CONSTRAINT devices_android_home_check CHECK (kind <> 'android' OR home_screen IN ('tables', 'takeaway'));
