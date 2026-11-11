-- Etapa 9A, fase A (A1): precio de complementos configurable por producto (§3.1) y vigencia de la
-- precuenta por revisión de la cuenta (§33).
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0014 ni
-- filas existentes y no siembra datos: las columnas nuevas dejan a los productos con la regla de
-- siempre (opción desactivada) y a las precuentas históricas con NULL, que conservan la regla
-- anterior por marcas de tiempo. Las restricciones llevan nombre explícito y el CHECK de las
-- columnas opcionales exige IS NOT NULL en la rama que valida, porque un CHECK que evalúa a NULL se
-- acepta.

-- 1. «El precio puede bajar al cambiar complementos» (§3.1). Desactivada por omisión: todo producto
-- existente conserva la equivalencia por valor con piso en el precio base (sustituir por un
-- complemento más barato o quitarlo sin reemplazo nunca baja el precio). Activada, Go permite que
-- las diferencias negativas de los complementos bajen el precio dentro de sus límites. Se edita con
-- catalog.prices.manage, como el precio, y queda congelada en el snapshot del pedido
-- (restaurant_orders.pricing_snapshot) y en la cotización de cada partida al confirmar: cambiarla
-- después no altera partidas confirmadas.
ALTER TABLE app.catalog_products
    ADD COLUMN price_can_decrease boolean NOT NULL DEFAULT false;

-- 2. Vigencia de la precuenta (§33). Al imprimir una precuenta, Go registra la revisión de la
-- cuenta (restaurant_accounts.revision) y el número de ajustes de la cuenta (descuentos y cargos
-- registrados más sus cancelaciones, ambos de solo inserción) leídos bajo el lock de cobros. La
-- precuenta es vigente mientras la cuenta conserve esa revisión y ese número de ajustes: ya no se
-- comparan instantes de inicio de transacción, que fallaban cuando una modificación esperaba el lock
-- mientras se imprimía. No es una segunda fuente de verdad: ambos valores se derivan de los
-- registros de la cuenta. Solo las precuentas los llevan; los tickets de pago y las filas
-- anteriores a esta migración quedan en NULL (las precuentas históricas siguen la regla anterior).
ALTER TABLE app.payment_documents
    ADD COLUMN account_revision integer,
    ADD COLUMN adjustment_count integer,
    ADD CONSTRAINT payment_documents_precheck_version_check CHECK (
        (account_revision IS NULL AND adjustment_count IS NULL)
        OR (
            kind = 'precheck'
            AND account_revision IS NOT NULL
            AND account_revision > 0
            AND adjustment_count IS NOT NULL
            AND adjustment_count >= 0
        )
    );
