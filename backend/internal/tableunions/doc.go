// Package tableunions es lo común de las uniones de mesas y las mesas divisibles (etapa M1, Fase A; decisiones M1-D1 a
// M1-D10 y M1-D14): el nombre de una unión, los fragmentos SQL con los que cada lector dice el lugar de una cuenta
// («Mesa 5 + 6» o la mesa), la instantánea de la venta y SettleEmpty, el único lugar donde una unión se deshace sola.
//
// Es un paquete neutral: solo depende de pgx, de events y de documents, así que lo usan operations (las rutas de
// uniones y grupos, que viven en operations.Handle), payments (el cobro y el reconocimiento de la venta), kitchen (la
// comanda, el KDS y Preparación), reports y administration sin ciclos. No toma ningún lock: quien lo llama ya tiene
// 7820148305 (exclusivo para escribir, compartido para leer) y los reportes solo usan sus fragmentos SQL de lectura.
//
// El modelo (0031):
//   - app.table_unions: la unión con su nombre vigente, su área, su grupo (si une todas las secciones de uno) y su
//     estado open|dissolved;
//   - app.table_union_members: sus mesas; un índice único parcial impide que una mesa esté en dos uniones abiertas;
//   - app.table_union_events: su historial (unir, agregar, quitar y deshacer) con el nombre después de cada cambio;
//   - app.table_groups y app.table_group_sections: los grupos predefinidos y sus secciones en orden.
//
// Una cuenta es de la unión porque su table_id es una mesa miembro (restaurant_accounts no cambia): cobrar, cocina,
// dividir, la cola sin conexión y lo que se mueve a una mesa de la unión quedan en ella sin código propio.
package tableunions
