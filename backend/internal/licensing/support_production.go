//go:build adelpos_production

package licensing

import _ "embed"

// compiledSupportKeys es la copia sincronizada de config/licencias/soporte-production.pub.json: la llave pública de
// soporte de producción. Se genera con el paso a producción (docs/licencias/PASO_A_PRODUCCION.md); mientras no exista,
// esta compilación falla a propósito, igual que sin llaves-production.json (§1.2 y §6.4 de la especificación).
//
//go:embed soporte-production.pub.json
var compiledSupportKeys []byte
