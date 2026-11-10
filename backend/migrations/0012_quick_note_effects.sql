-- Etapa 8, fase B (Bq): indicaciones rápidas con efecto. Una indicación rápida de categoría
-- (0009) puede vincularse a un ingrediente del catálogo con un efecto explícito: omitirlo, dejar
-- su porción habitual o pedir la porción extra. El efecto nunca se infiere del texto: al tocarla,
-- la captura aplica la misma selección estructurada del personalizador
-- (customization.ingredients) y Go comprueba que coincida, de modo que precio e inventario salen
-- del mismo dato y no se acumulan multiplicadores.
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0011 ni
-- siembra datos. Las indicaciones existentes quedan con efecto 'none' y sin ingrediente: siguen
-- siendo solo instrucción de cocina, como en la etapa 7.1. Agregar columnas con un valor
-- predeterminado constante no reescribe la tabla.

ALTER TABLE app.catalog_category_quick_notes
    ADD COLUMN effect text NOT NULL DEFAULT 'none'
        CONSTRAINT catalog_category_quick_notes_effect_check CHECK (effect IN ('none', 'omit', 'normal', 'extra')),
    ADD COLUMN ingredient_id uuid
        CONSTRAINT catalog_category_quick_notes_ingredient_id_fkey REFERENCES app.catalog_ingredients(id);

-- Coherencia del vínculo: sin efecto no hay ingrediente y todo efecto exige su ingrediente. Ambas
-- ramas comparan con IS NULL / IS NOT NULL explícitos, así que el CHECK nunca evalúa a NULL.
ALTER TABLE app.catalog_category_quick_notes
    ADD CONSTRAINT catalog_category_quick_notes_effect_ingredient_check CHECK (
        (effect = 'none' AND ingredient_id IS NULL)
        OR (effect IN ('omit', 'normal', 'extra') AND ingredient_id IS NOT NULL)
    );

-- Las indicaciones vinculadas a un ingrediente se consultan por él (llave foránea).
CREATE INDEX catalog_category_quick_notes_ingredient_idx
    ON app.catalog_category_quick_notes (ingredient_id)
    WHERE ingredient_id IS NOT NULL;
