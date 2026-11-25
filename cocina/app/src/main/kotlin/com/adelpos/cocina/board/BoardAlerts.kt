package com.adelpos.cocina.board

/** Los dos sonidos de la pantalla de cocina (§5.2, K1-D15). */
enum class AlertSound {
    /** Un pedido nuevo: una tarjeta nueva o platillos agregados («NUEVO»). */
    NEW_ORDER,

    /** Una cancelación o un cambio que cocina todavía no recibe (`pending_change`). */
    CHANGE,
}

/**
 * Lo que una lectura trae de nuevo frente a la anterior: qué suena y qué tarjetas se animan.
 *
 * @property sounds a lo más uno de cada tipo por lectura (nunca uno por pedido).
 * @property freshCards las tarjetas con algo nuevo o cambiado: llevan la animación breve.
 */
data class BoardAlerts(val sounds: Set<AlertSound>, val freshCards: Set<String>) {
    companion object {
        val NONE = BoardAlerts(emptySet(), emptySet())

        /**
         * Compara dos lecturas por las **tareas** de cocina (una por unidad), que no cambian aunque
         * Go agrupe los platillos de otra manera:
         * - **Primera lectura** ([previous] null: al abrir la aplicación, al emparejar o después de
         *   una pantalla sin pedidos): no suena ni se anima nada. Lo que ya estaba no es nuevo.
         * - **Al volver de «Sin conexión»** la lectura se compara con la última que se vio: lo que
         *   llegó mientras tanto suena **una sola vez** por tipo, no una por pedido.
         * - Tareas pendientes que la lectura anterior no tenía → pedido nuevo, salvo las que esta
         *   pantalla acaba de recuperar ([recovered]): regresarlas no es un pedido nuevo.
         * - Tareas canceladas que antes no lo estaban (o que llegaron ya canceladas) y cambios que
         *   cocina todavía no recibe → cambio. Una tarjeta con un cambio suena solo como cambio
         *   (una modificación llega como cancelación y agregado a la vez).
         * - Lo que otra pantalla marca listo no suena.
         */
        fun between(previous: Board?, next: Board, recovered: Set<String>): BoardAlerts {
            if (previous == null) {
                return NONE
            }
            val before = HashMap<String, Dish>()
            for (card in previous.cards) {
                for (dish in card.dishes) {
                    for (task in dish.tasks) {
                        before[task.id] = dish
                    }
                }
            }
            val sounds = mutableSetOf<AlertSound>()
            val fresh = mutableSetOf<String>()
            for (card in next.cards) {
                val changed = card.dishes.any { dish -> changed(dish, before) }
                val added =
                    card.dishes.any { dish ->
                        dish.state == DishState.PENDING &&
                            dish.tasks.any { it.id !in before && it.id !in recovered }
                    }
                when {
                    changed -> {
                        sounds += AlertSound.CHANGE
                        fresh += card.id
                    }
                    added -> {
                        sounds += AlertSound.NEW_ORDER
                        fresh += card.id
                    }
                }
            }
            return BoardAlerts(sounds, fresh)
        }

        // Cancelado ahora (antes no, o llegó ya cancelado) o con un cambio que antes no tenía.
        private fun changed(dish: Dish, before: Map<String, Dish>): Boolean {
            val known = dish.tasks.mapNotNull { before[it.id] }
            return when (dish.state) {
                DishState.CANCELLED ->
                    known.size < dish.tasks.size || known.any { it.state != DishState.CANCELLED }
                DishState.PENDING -> dish.pendingChange && known.none { it.pendingChange }
                DishState.READY -> false
            }
        }
    }
}
