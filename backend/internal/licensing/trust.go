package licensing

import "crypto/ed25519"

// TrustedKey es una llave pública de firma en la que confía esta versión de AD-EL POS.
type TrustedKey struct {
	Environment string
	PublicKey   ed25519.PublicKey
}

// TrustSet son las llaves de confianza de esta versión, todas de un mismo entorno de firma (contrato §3):
// una versión de desarrollo solo confía en las de prueba y una de producción solo en las de producción.
type TrustSet struct {
	Environment string
	Keys        map[string]TrustedKey
}
