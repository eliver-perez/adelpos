-- Etapa K1: la única migración de la etapa (aplicación de cocina para Android; esquema 29). Cada fase agrega aquí su
-- propia sección con el encabezado «Fase X» (decisión K1-D18 del coordinador): la Fase A pone todo lo del servidor y las
-- fases C a E solo agregan una sección propia si les falta algo. Es transaccional (el migrador aplica todas las
-- pendientes en una sola transacción). No cambia ninguna fila existente: solo agrega columnas con su valor por omisión,
-- relaja dos restricciones para la pantalla de cocina (con un disparador que exige que sea una pantalla de cocina) y
-- agrega una tabla y un índice.

-- =====================================================================================================================
-- Fase A · Servidor: dispositivo de cocina, estaciones, API y eventos (§2 y §3 de la especificación)
-- =====================================================================================================================

-- 1. Límites de color generales de las pantallas de cocina Android (§4 y §6; decisión K1-D17): «atención» desde
--    display_warning_minutes y «retraso» desde display_late_minutes de espera, comunes a todas las pantallas. Son solo de
--    las pantallas Android: el modo Cocina del escritorio sigue con los tiempos de cada estación. Se cambian con
--    PUT kitchen/timing (kitchen.timing.manage).
ALTER TABLE app.kitchen_settings
    ADD COLUMN display_warning_minutes integer NOT NULL DEFAULT 10
        CONSTRAINT kitchen_settings_display_warning_check CHECK (display_warning_minutes BETWEEN 1 AND 1440),
    ADD COLUMN display_late_minutes integer NOT NULL DEFAULT 20,
    ADD CONSTRAINT kitchen_settings_display_late_check
        CHECK (display_late_minutes > display_warning_minutes AND display_late_minutes <= 2880);

-- 2. Ajustes de cada pantalla de cocina (§4, §5.2 y §6; decisión K1-D15), elegidos en Terminales con PATCH devices/{id}
--    (devices.manage): tamaño de letra, tema, sonido (activado por omisión en las pantallas nuevas) y su volumen. Son
--    aparte de kitchen_sound_alert, que es el aviso del modo Cocina del escritorio. Las demás terminales los tienen con
--    su valor por omisión y no se usan.
--
--    kitchen_display_seat_released_at es el estado «cupo liberado» de K1-D6: «Liberar cupo» en Terminales libera el
--    cupo de cocina de la pantalla y lo anota aquí; desde entonces la pantalla no lo vuelve a ocupar sola al conectarse,
--    solo cuando alguien toca «Usar esta pantalla» en ella (POST kitchen-display/connect con take_seat), que lo borra.
--    Solo una pantalla de cocina puede tenerlo.
ALTER TABLE app.devices
    ADD COLUMN kitchen_display_text_size text NOT NULL DEFAULT 'normal'
        CONSTRAINT devices_kitchen_display_text_size_check CHECK (kitchen_display_text_size IN ('normal', 'large')),
    ADD COLUMN kitchen_display_theme text NOT NULL DEFAULT 'dark'
        CONSTRAINT devices_kitchen_display_theme_check CHECK (kitchen_display_theme IN ('dark', 'light')),
    ADD COLUMN kitchen_display_sound boolean NOT NULL DEFAULT true,
    ADD COLUMN kitchen_display_volume text NOT NULL DEFAULT 'medium'
        CONSTRAINT devices_kitchen_display_volume_check CHECK (kitchen_display_volume IN ('low', 'medium', 'high')),
    ADD COLUMN kitchen_display_seat_released_at timestamptz,
    ADD CONSTRAINT devices_kitchen_display_seat_released_check
        CHECK (kitchen_display_seat_released_at IS NULL OR kind = 'kitchen');

-- 3. El cupo de cocina de una pantalla Android va ligado al dispositivo, no a una sesión (decisión K1-D6): la pantalla
--    no tiene sesión y un corte de red no debe liberarlo. La fila kitchen_display puede quedar sin session_id; entonces
--    cuenta mientras el dispositivo no esté revocado (internal/licenseseats, inUseCondition) y se borra al revocarlo o
--    con «Liberar cupo». La de una terminal de escritorio en Preparación sigue apuntando a su sesión, como en la L1.
--    Como una restricción CHECK no puede mirar otra tabla, el disparador exige que una fila de cocina sin sesión sea de
--    un dispositivo de tipo kitchen (con el mismo código de error que una restricción CHECK, 23514).
ALTER TABLE app.license_seats DROP CONSTRAINT license_seats_link_check;
ALTER TABLE app.license_seats
    ADD CONSTRAINT license_seats_link_check CHECK (
        (kind = 'cash_register' AND shift_id IS NOT NULL AND session_id IS NULL)
        OR (kind = 'order_taker' AND shift_id IS NULL AND session_id IS NULL)
        OR (kind = 'kitchen_display' AND shift_id IS NULL)
    );

CREATE FUNCTION app.license_seats_kitchen_device_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.kind = 'kitchen_display' AND NEW.session_id IS NULL
        AND NOT EXISTS (SELECT FROM app.devices device WHERE device.id = NEW.device_id AND device.kind = 'kitchen') THEN
        RAISE EXCEPTION 'Solo una pantalla de cocina ocupa el cupo de cocina sin sesión'
            USING ERRCODE = '23514', CONSTRAINT = 'license_seats_kitchen_device_check';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER license_seats_kitchen_device_check
    BEFORE INSERT OR UPDATE ON app.license_seats
    FOR EACH ROW EXECUTE FUNCTION app.license_seats_kitchen_device_guard();

-- 4. Acuse de recepción de la pantalla de cocina (decisión K1-D12): la pantalla confirma las comandas de sus estaciones
--    cuando las muestra, con su dispositivo y sin usuario, para que Comandas no siga diciendo «Recepción en pantalla de
--    cocina pendiente». user_id solo puede quedar vacío en un acuse de un dispositivo kitchen; los de una terminal con
--    sesión (POST kitchen/tickets/{id}/ack) siguen con su usuario.
ALTER TABLE app.kitchen_kds_receipts ALTER COLUMN user_id DROP NOT NULL;

CREATE FUNCTION app.kitchen_kds_receipts_device_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.user_id IS NULL
        AND NOT EXISTS (SELECT FROM app.devices device WHERE device.id = NEW.device_id AND device.kind = 'kitchen') THEN
        RAISE EXCEPTION 'Solo una pantalla de cocina confirma una comanda sin usuario'
            USING ERRCODE = '23514', CONSTRAINT = 'kitchen_kds_receipts_device_check';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER kitchen_kds_receipts_device_check
    BEFORE INSERT OR UPDATE ON app.kitchen_kds_receipts
    FOR EACH ROW EXECUTE FUNCTION app.kitchen_kds_receipts_device_guard();

-- 5. Tarjeta cerrada por una cancelación (§4: el platillo cancelado no desaparece «hasta que se marque el pedido»).
--    Cuando lo último que pasa en una tarjeta es que se cancela lo que faltaba, no queda nada por marcar y la tarjeta
--    desaparecería sin que la cocina lo viera. La tarjeta se queda con sus platillos tachados hasta que alguien toca el
--    botón del pedido en una pantalla (o pasan 30 minutos), y aquí queda que esa cancelación ya se vio. Una fila por
--    tarea cancelada; si la tarea se volviera a cancelar después (otra modificación), cuenta solo si dismissed_at es
--    posterior a la cancelación. No es un dato de operación: no cambia ventas, cocina ni impresión.
CREATE TABLE app.kitchen_display_dismissals (
    task_id uuid CONSTRAINT kitchen_display_dismissals_pkey PRIMARY KEY REFERENCES app.kitchen_tasks(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    dismissed_at timestamptz NOT NULL DEFAULT now()
);
COMMENT ON TABLE app.kitchen_display_dismissals IS
    'Cancelaciones que una pantalla de cocina ya dio por vistas al marcar el pedido (etapa K1)';

-- 6. Índices de las lecturas de la pantalla, que se repiten con cada cambio de cocina: «Recientes» (§4) lee lo marcado
--    listo en los últimos 30 minutos de sus estaciones (sin el índice recorrería todas las tareas listas de la estación
--    desde que existe) y los acuses pendientes buscan las comandas de las liberaciones que muestra.
CREATE INDEX kitchen_tasks_ready_idx ON app.kitchen_tasks(station_id, ready_at) WHERE state = 'ready';
CREATE INDEX kitchen_tickets_release_idx ON app.kitchen_tickets(release_id) WHERE release_id IS NOT NULL;
