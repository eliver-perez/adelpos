// Package operational reúne las pruebas de aceptación de los reportes operativos de la etapa 9A
// (fase B2: caja, inventarios, costos y márgenes, compras, proveedores y empleados) contra
// PostgreSQL con la API real. Vive en su propio paquete para tener una base temporal exclusiva: sus
// escenarios (turnos permanentes de varios días, arqueos, inventario activado, compras) no deben
// alterar los totales exactos de la aceptación de internal/reports ni depender de ellos. No contiene
// código de producción; las pruebas llevan la etiqueta de compilación integration.
package operational
