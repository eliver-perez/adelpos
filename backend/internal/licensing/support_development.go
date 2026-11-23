//go:build !adelpos_production

package licensing

import _ "embed"

// compiledSupportKeys es la copia sincronizada de config/licencias/soporte-development.pub.json
// (scripts/sync-metadata.mjs): la llave pública de soporte de desarrollo (§6.4 de la especificación). Su privada vive
// solo en ~/.adel-pos-dev/soporte/ de la computadora del propietario.
//
//go:embed soporte-development.pub.json
var compiledSupportKeys []byte
