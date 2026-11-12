-- Etapa 9C: pantalla de inicio por terminal (§1.5 y §4.1) y columnas por impresora (§5.3).
-- No modifica filas existentes salvo el valor por omisión de las columnas nuevas; no agrega permisos.

-- 1. Pantalla de inicio de cada terminal (lista cerrada de secciones de la barra superior). Mesas por omisión. La
--    preferencia por usuario (user_preferences.home_screen) se conserva pero la aplicación deja de usarla: abre en la
--    pantalla de la terminal, o en la primera sección que el usuario puede ver si no tiene la de la terminal.
ALTER TABLE app.devices
    ADD COLUMN home_screen text NOT NULL DEFAULT 'tables'
        CONSTRAINT devices_home_screen_check CHECK (home_screen IN ('tables', 'takeaway', 'kitchen', 'cash'));

-- 2. Columnas de texto por impresora: NULL es el valor predeterminado de su papel (42 en 80 mm y 30 en 58 mm, como desde
--    la etapa 5), de modo que las impresoras existentes y las fotografías guardadas en trabajos anteriores imprimen
--    exactamente igual. Solo se admite 48 en 80 mm y 32 en 58 mm como alternativa (lista cerrada). La rama que valida
--    exige el valor explícito y el papel correspondiente; un CHECK que evalúa a NULL se aceptaría.
ALTER TABLE app.kitchen_printers
    ADD COLUMN columns smallint
        CONSTRAINT kitchen_printers_columns_check CHECK (
            columns IS NULL
            OR (columns IS NOT NULL AND paper_width = 80 AND columns IN (42, 48))
            OR (columns IS NOT NULL AND paper_width = 58 AND columns IN (30, 32))
        );
