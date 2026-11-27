package tableunions

// Fragmentos SQL del lugar de una cuenta (M1-D14): todos los lectores que muestran dónde está una cuenta (documentos,
// comanda, KDS, Preparación, avisos de listos, reportes y la instantánea de la venta) los usan, así que dicen lo mismo.
// Reciben la expresión SQL de la mesa (por ejemplo "a.table_id") y devuelven una expresión escalar: el nombre de la
// unión o NULL si la mesa no estaba unida; quien llama hace coalesce con el nombre de la mesa.

// UnionNameSQL es el nombre de la unión abierta de la mesa (NULL si no está unida).
func UnionNameSQL(tableExpression string) string {
	return `(SELECT tu.name FROM app.table_union_members tum JOIN app.table_unions tu ON tu.id=tum.union_id
        WHERE tum.table_id=` + tableExpression + ` AND tum.left_at IS NULL AND tu.state='open')`
}

// UnionIDSQL es el identificador de la unión abierta de la mesa (NULL si no está unida).
func UnionIDSQL(tableExpression string) string {
	return `(SELECT tum.union_id FROM app.table_union_members tum
        WHERE tum.table_id=` + tableExpression + ` AND tum.left_at IS NULL)`
}

// UnionNameAtSQL es el nombre que tenía la unión de la mesa en un instante (atExpression, por ejemplo la hora de una
// cancelación o del cierre): la unión de la que la mesa era miembro en ese instante, con el nombre de su último cambio
// hasta entonces. Una unión que se deshizo en la misma transacción que el cambio (now() igual) sigue contando, porque
// left_at y la hora del cambio son el mismo now(). NULL si la mesa no estaba unida.
func UnionNameAtSQL(tableExpression, atExpression string) string {
	return `(SELECT tue.union_name FROM app.table_union_members tum
        JOIN app.table_union_events tue ON tue.union_id=tum.union_id AND tue.created_at<=` + atExpression + `
        WHERE tum.table_id=` + tableExpression + ` AND tum.joined_at<=` + atExpression + `
            AND (tum.left_at IS NULL OR tum.left_at>=` + atExpression + `)
        ORDER BY tue.sequence DESC LIMIT 1)`
}

// GroupIDSQL es el grupo activo del que la mesa es sección (NULL si no es sección de ninguno).
func GroupIDSQL(tableExpression string) string {
	return `(SELECT tgs.group_id FROM app.table_group_sections tgs JOIN app.table_groups tg ON tg.id=tgs.group_id
        WHERE tgs.table_id=` + tableExpression + ` AND tg.active)`
}

// PlaceTablesSQL son las mesas cuyas cuentas abiertas cuentan como de la mesa (M1-D10): todas las de su unión abierta
// o, si no está unida, solo ella. Es una subconsulta de identificadores para «a.table_id IN (…)».
func PlaceTablesSQL(tableExpression string) string {
	return `(SELECT tum2.table_id FROM app.table_union_members tum1
            JOIN app.table_union_members tum2 ON tum2.union_id=tum1.union_id AND tum2.left_at IS NULL
            WHERE tum1.table_id=` + tableExpression + ` AND tum1.left_at IS NULL
        UNION ALL
        SELECT ` + tableExpression + ` WHERE NOT EXISTS(SELECT FROM app.table_union_members tum3
            WHERE tum3.table_id=` + tableExpression + ` AND tum3.left_at IS NULL))`
}
