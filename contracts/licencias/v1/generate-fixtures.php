<?php

declare(strict_types=1);

/*
 * Genera los archivos de prueba compartidos del contrato de licencias V1.0 (sección 13).
 *
 *   php contracts/v1/generate-fixtures.php           escribe contracts/v1/fixtures/
 *   php contracts/v1/generate-fixtures.php --check   comprueba que lo escrito es idéntico, sin tocarlo
 *
 * No usa el código del servidor: arma cada contenido a mano, con sus valores esperados, para que
 * sirva de referencia independiente. Las pruebas comparan después el servidor contra estos archivos.
 *
 * Todas las llaves salen de semillas públicas y deterministas. Son solo para pruebas: el servidor
 * las rechaza en producción y ninguna instalación de producción debe confiar en ellas.
 */

if (!extension_loaded('sodium')) {
	throw new RuntimeException('Hace falta la extensión Sodium.');
}

const SEED_PREFIX = 'ADELPOS LICENSE PUBLIC TEST ONLY V1 ';
const JWS_TYPE = 'adelpos-lic+jws';
const PROOF_PREFIX = 'ADELPOS-LIC-V1';
const REQUEST_PREFIX = "ADELPOS-LICREQ-V1\n";

function base64Url(string $bytes): string
{
	return sodium_bin2base64($bytes, SODIUM_BASE64_VARIANT_URLSAFE_NO_PADDING);
}

function jsonText(mixed $value): string
{
	return json_encode($value, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_THROW_ON_ERROR);
}

/** Llave Ed25519 determinista a partir de un nombre público. */
function testKey(string $name): array
{
	$seed = hash('sha256', SEED_PREFIX . $name, true);
	$pair = sodium_crypto_sign_seed_keypair($seed);
	return [
		'seed_hex' => bin2hex($seed),
		'public_key' => base64Url(sodium_crypto_sign_publickey($pair)),
		'secret' => sodium_crypto_sign_secretkey($pair),
	];
}

function sign(array $key, string $message): string
{
	return base64Url(sodium_crypto_sign_detached($message, $key['secret']));
}

/** JWS Compact con el encabezado exacto del contrato (sección 3). */
function jws(string $kid, array $signingKey, array $content): array
{
	$headerJson = jsonText(['alg' => 'EdDSA', 'kid' => $kid, 'typ' => JWS_TYPE]);
	$contentJson = jsonText($content);
	$signingInput = base64Url($headerJson) . '.' . base64Url($contentJson);
	$jws = $signingInput . '.' . sign($signingKey, $signingInput);
	return [
		'header_json' => $headerJson,
		'content_json' => $contentJson,
		'jws' => $jws,
		'sha256' => hash('sha256', $jws),
	];
}

function writeFixture(string $path, string $bytes): void
{
	if (in_array('--check', $GLOBALS['argv'] ?? [], true)) {
		if (!is_file($path) || file_get_contents($path) !== $bytes) {
			throw new RuntimeException('El archivo de prueba no coincide: ' . basename($path));
		}
		return;
	}
	if (!is_dir(dirname($path)) && !mkdir(dirname($path), 0755, true)) {
		throw new RuntimeException('No se pudo crear la carpeta de ' . basename($path));
	}
	if (file_put_contents($path, $bytes) !== strlen($bytes)) {
		throw new RuntimeException('No se pudo escribir ' . $path);
	}
}

// ---------------------------------------------------------------------------------------------
// Llaves de firma (del servidor) e instalaciones (del cliente)
// ---------------------------------------------------------------------------------------------

$signingKeys = [
	'adelpos-test-fixture-a' => ['environment' => 'test'] + testKey('adelpos-test-fixture-a'),
	'adelpos-test-fixture-b' => ['environment' => 'test'] + testKey('adelpos-test-fixture-b'),
	// Hace de llave de producción solo en los casos negativos. Sigue siendo una llave pública de prueba.
	'adelpos-prod-fixture-a' => ['environment' => 'production'] + testKey('adelpos-prod-fixture-a'),
];
$installations = [
	'a' => ['server_id' => '22222222-2222-4222-8222-222222222222'] + testKey('installation-fixture-a'),
	'b' => ['server_id' => '66666666-6666-4666-8666-666666666666'] + testKey('installation-fixture-b'),
];
$kid = 'adelpos-test-fixture-a';

// ---------------------------------------------------------------------------------------------
// Licencias válidas
// ---------------------------------------------------------------------------------------------

// El ejemplo de la sección 4 del contrato: suscripción mensual del plan elemental.
$subscription = [
	'schema_version' => '1.0',
	'product_id' => 'adel_pos',
	'environment' => 'test',
	'license_id' => '11111111-1111-4111-8111-111111111111',
	'activation_id' => '33333333-3333-4333-8333-333333333333',
	'server_id' => $installations['a']['server_id'],
	'installation_public_key' => $installations['a']['public_key'],
	'licensee_name' => 'Restaurante de Pruebas, S.A. de C.V.',
	'license_type' => 'subscription',
	'billing_period' => 'monthly',
	'license_status' => 'active',
	'status_reason' => null,
	'license_revision' => 1,
	'issued_at' => '2026-11-16T06:30:00.000000Z',
	'period_starts_at' => '2026-11-16T06:30:00.000000Z',
	'expires_at' => '2026-12-16T06:30:00.000000Z',
	'grace_days' => 5,
	'updates_until' => '2026-12-16T06:30:00.000000Z',
	'plan' => ['plan_id' => 'elemental', 'name' => 'Plan elemental', 'custom' => false],
	'limits' => ['cash_registers' => 3, 'order_takers' => 3, 'kitchen_displays' => 3],
	'features' => new stdClass(),
];
$perpetual = array_replace($subscription, [
	'license_type' => 'perpetual',
	'billing_period' => null,
	'expires_at' => null,
	'grace_days' => 0,
]);
$closing = ['license_status' => 'revoked', 'license_revision' => 2, 'issued_at' => '2026-11-20T18:00:00.000000Z'];

$contents = [
	// Perpetua con 0, 6 y 12 meses de actualizaciones, contados desde la activación.
	'perpetual-0-months' => array_replace($perpetual, [
		'license_id' => '10000000-0000-4000-8000-000000000001',
		'activation_id' => '30000000-0000-4000-8000-000000000001',
		'updates_until' => '2026-11-16T06:30:00.000000Z',
	]),
	'perpetual-6-months' => array_replace($perpetual, [
		'license_id' => '10000000-0000-4000-8000-000000000002',
		'activation_id' => '30000000-0000-4000-8000-000000000002',
		'updates_until' => '2027-05-16T06:30:00.000000Z',
	]),
	'perpetual-12-months' => array_replace($perpetual, [
		'license_id' => '10000000-0000-4000-8000-000000000003',
		'activation_id' => '30000000-0000-4000-8000-000000000003',
		'updates_until' => '2027-11-16T06:30:00.000000Z',
		'plan' => ['plan_id' => 'premium', 'name' => 'Plan premium', 'custom' => false],
		'limits' => ['cash_registers' => 5, 'order_takers' => 10, 'kitchen_displays' => 5],
	]),
	'subscription-monthly' => $subscription,
	'subscription-annual' => array_replace($subscription, [
		'license_id' => '10000000-0000-4000-8000-000000000005',
		'activation_id' => '30000000-0000-4000-8000-000000000005',
		'billing_period' => 'annual',
		'expires_at' => '2027-11-16T06:30:00.000000Z',
		'updates_until' => '2027-11-16T06:30:00.000000Z',
	]),
	// Prueba de 15 días: sin tolerancia y con el plan básico.
	'trial-15-days' => array_replace($subscription, [
		'license_id' => '10000000-0000-4000-8000-000000000006',
		'activation_id' => '30000000-0000-4000-8000-000000000006',
		'license_type' => 'trial',
		'billing_period' => null,
		'expires_at' => '2026-12-01T06:30:00.000000Z',
		'grace_days' => 0,
		'updates_until' => '2026-12-01T06:30:00.000000Z',
		'plan' => ['plan_id' => 'basico', 'name' => 'Plan básico', 'custom' => false],
		'limits' => ['cash_registers' => 1, 'order_takers' => 0, 'kitchen_displays' => 0],
	]),
	// Personalizada: los cupos ya no son los del plan. Lleva un módulo que el cliente no conoce.
	'custom' => array_replace($subscription, [
		'license_id' => '10000000-0000-4000-8000-000000000007',
		'activation_id' => '30000000-0000-4000-8000-000000000007',
		'licensee_name' => 'Café Ñandú',
		'billing_period' => 'quarterly',
		'expires_at' => '2027-02-16T06:30:00.000000Z',
		'updates_until' => '2027-02-16T06:30:00.000000Z',
		'plan' => ['plan_id' => 'basico', 'name' => 'Plan básico', 'custom' => true],
		'limits' => ['cash_registers' => 2, 'order_takers' => 1, 'kitchen_displays' => 0],
		'features' => ['modulo_futuro' => true],
	]),
	// Revisiones de cierre: la siguiente revisión de tres de las licencias anteriores.
	'closing-deactivated' => array_replace($subscription, $closing, ['status_reason' => 'deactivated']),
	'closing-transferred' => array_replace(
		$perpetual,
		[
			'license_id' => '10000000-0000-4000-8000-000000000002',
			'activation_id' => '30000000-0000-4000-8000-000000000002',
			'updates_until' => '2027-05-16T06:30:00.000000Z',
		],
		$closing,
		['status_reason' => 'transferred'],
	),
	'closing-revoked' => array_replace(
		$subscription,
		[
			'license_id' => '10000000-0000-4000-8000-000000000006',
			'activation_id' => '30000000-0000-4000-8000-000000000006',
			'license_type' => 'trial',
			'billing_period' => null,
			'expires_at' => '2026-12-01T06:30:00.000000Z',
			'grace_days' => 0,
			'updates_until' => '2026-12-01T06:30:00.000000Z',
			'plan' => ['plan_id' => 'basico', 'name' => 'Plan básico', 'custom' => false],
			'limits' => ['cash_registers' => 1, 'order_takers' => 0, 'kitchen_displays' => 0],
		],
		$closing,
		['status_reason' => 'revoked'],
	),
];
// Ciclo comercial (etapa 3). Cada caso es la revisión nueva de una licencia, con la misma activación.
// Las fechas están calculadas a mano: Monterrey va en UTC-6 todo el año.
$contents += [
	// La suscripción anual (revisión 1) se renueva el 10 de noviembre de 2027, antes de vencer: el
	// periodo nuevo empieza en el vencimiento anterior y conserva su día y su hora (contrato §5.3).
	'renewed-subscription' => array_replace($contents['subscription-annual'], [
		'license_revision' => 2,
		'issued_at' => '2027-11-10T18:00:00.000000Z',
		'period_starts_at' => '2027-11-16T06:30:00.000000Z',
		'expires_at' => '2028-11-16T06:30:00.000000Z',
		'updates_until' => '2028-11-16T06:30:00.000000Z',
	]),
	// Una prueba activada se paga el 25 de noviembre a las 12:00 y pasa a suscripción mensual del plan
	// elemental: el periodo nuevo empieza en la fecha de pago y los días de prueba no se acumulan (§5.4).
	'trial-converted-to-subscription' => array_replace($subscription, [
		'license_id' => '10000000-0000-4000-8000-000000000008',
		'activation_id' => '30000000-0000-4000-8000-000000000008',
		'license_revision' => 2,
		'issued_at' => '2026-11-25T18:00:00.000000Z',
		'period_starts_at' => '2026-11-25T18:00:00.000000Z',
		'expires_at' => '2026-12-25T18:00:00.000000Z',
		'updates_until' => '2026-12-25T18:00:00.000000Z',
	]),
	// La misma conversión, a perpetua con 6 meses de actualizaciones contados desde la fecha de pago.
	'trial-converted-to-perpetual' => array_replace($perpetual, [
		'license_id' => '10000000-0000-4000-8000-000000000009',
		'activation_id' => '30000000-0000-4000-8000-000000000009',
		'license_revision' => 2,
		'issued_at' => '2026-11-25T18:00:00.000000Z',
		'period_starts_at' => '2026-11-25T18:00:00.000000Z',
		'updates_until' => '2027-05-25T18:00:00.000000Z',
	]),
	// La perpetua sin actualizaciones compra 12 meses de mantenimiento el 20 de enero de 2027 a las
	// 12:00: como ya habían terminado, se cuentan desde la fecha de pago (§5.5).
	'perpetual-with-maintenance' => array_replace($contents['perpetual-0-months'], [
		'license_revision' => 2,
		'issued_at' => '2027-01-20T18:00:00.000000Z',
		'updates_until' => '2028-01-20T18:00:00.000000Z',
	]),
	// Suscripción mensual del plan elemental que sube a premium el 1 de diciembre a las 12:00: periodo
	// nuevo desde la fecha de pago, con los cupos del plan nuevo.
	'plan-upgrade' => array_replace($subscription, [
		'license_id' => '10000000-0000-4000-8000-000000000010',
		'activation_id' => '30000000-0000-4000-8000-000000000010',
		'license_revision' => 2,
		'issued_at' => '2026-12-01T18:00:00.000000Z',
		'period_starts_at' => '2026-12-01T18:00:00.000000Z',
		'expires_at' => '2027-01-01T18:00:00.000000Z',
		'updates_until' => '2027-01-01T18:00:00.000000Z',
		'plan' => ['plan_id' => 'premium', 'name' => 'Plan premium', 'custom' => false],
		'limits' => ['cash_registers' => 5, 'order_takers' => 10, 'kitchen_displays' => 5],
	]),
	// Suscripción premium que programó bajar a básico al vencer lo pagado (16 de diciembre) y se renovó
	// antes. La tarea diaria del 17 de diciembre firma la revisión 3 con los cupos del básico; el periodo
	// renovado no cambia.
	'scheduled-reduction-applied' => array_replace($subscription, [
		'license_id' => '10000000-0000-4000-8000-000000000011',
		'activation_id' => '30000000-0000-4000-8000-000000000011',
		'license_revision' => 3,
		'issued_at' => '2026-12-17T03:15:00.000000Z',
		'period_starts_at' => '2026-12-16T06:30:00.000000Z',
		'expires_at' => '2027-01-16T06:30:00.000000Z',
		'updates_until' => '2027-01-16T06:30:00.000000Z',
		'plan' => ['plan_id' => 'basico', 'name' => 'Plan básico', 'custom' => false],
		'limits' => ['cash_registers' => 1, 'order_takers' => 0, 'kitchen_displays' => 0],
	]),
];
// Transferencia por autoservicio (etapa 4; contrato §8.4). Una suscripción mensual activa en la
// instalación a (revisión 1) se transfiere el 20 de noviembre a las 12:00 a la instalación b. En la misma
// transacción, la anterior recibe su cierre revoked/transferred (revisión 2) y la nueva su revisión
// active (revisión 3), con una activación nueva y las mismas fechas, cupos, plan y módulos.
$transferBefore = array_replace($subscription, [
	'license_id' => '10000000-0000-4000-8000-000000000012',
	'activation_id' => '30000000-0000-4000-8000-000000000012',
]);
$contents += [
	'self-service-transfer-before' => $transferBefore,
	'self-service-transfer-closing' => array_replace($transferBefore, [
		'license_status' => 'revoked',
		'status_reason' => 'transferred',
		'license_revision' => 2,
		'issued_at' => '2026-11-20T18:00:00.000000Z',
	]),
	'self-service-transfer-new' => array_replace($transferBefore, [
		'activation_id' => '30000000-0000-4000-8000-000000000013',
		'server_id' => $installations['b']['server_id'],
		'installation_public_key' => $installations['b']['public_key'],
		'license_revision' => 3,
		'issued_at' => '2026-11-20T18:00:00.000000Z',
	]),
];
// La misma licencia perpetua, firmada después con otra llave de confianza (rotación de llaves).
$rotated = array_replace($contents['perpetual-12-months'], [
	'license_revision' => 2,
	'issued_at' => '2026-12-01T00:00:00.000000Z',
]);

// «installation» es la instalación a la que va la licencia: la que la verifica.
$installationOf = [
	$installations['a']['server_id'] => 'installation-fixture-a',
	$installations['b']['server_id'] => 'installation-fixture-b',
];
$licenses = [];
foreach ($contents as $name => $content) {
	$licenses[$name] =
		[
			'name' => $name,
			'kid' => $kid,
			'installation' => $installationOf[$content['server_id']],
			'content' => $content,
		] + jws($kid, $signingKeys[$kid], $content);
}
$licenses['rotated-key'] =
	[
		'name' => 'rotated-key',
		'kid' => 'adelpos-test-fixture-b',
		'installation' => $installationOf[$rotated['server_id']],
		'content' => $rotated,
	] + jws('adelpos-test-fixture-b', $signingKeys['adelpos-test-fixture-b'], $rotated);

// ---------------------------------------------------------------------------------------------
// Casos negativos (contrato, sección 13): lo que el cliente debe rechazar, y por qué
// ---------------------------------------------------------------------------------------------

$valid = $licenses['subscription-monthly'];
[$headerSegment, $contentSegment, $signatureSegment] = explode('.', $valid['jws']);
$badSignature = sodium_base642bin($signatureSegment, SODIUM_BASE64_VARIANT_URLSAFE_NO_PADDING);
$badSignature[0] = chr(ord($badSignature[0]) ^ 1);
$unknownHeader = base64Url(jsonText(['alg' => 'EdDSA', 'kid' => 'adelpos-test-desconocida', 'typ' => JWS_TYPE]));
$previousTypeHeader = base64Url(jsonText(['alg' => 'EdDSA', 'kid' => $kid, 'typ' => 'lic+jws']));
$reorderedHeader = base64Url(jsonText(['kid' => $kid, 'alg' => 'EdDSA', 'typ' => JWS_TYPE]));
$resign = fn(string $header): string => $header .
	'.' .
	$contentSegment .
	'.' .
	sign($signingKeys[$kid], $header . '.' . $contentSegment);
$signed = fn(array $content, string $withKid = 'adelpos-test-fixture-a', ?string $withKey = null): string => jws(
	$withKid,
	$signingKeys[$withKey ?? $withKid],
	$content,
)['jws'];
$withoutLimits = $subscription;
unset($withoutLimits['limits']);
$production = array_replace($subscription, ['environment' => 'production']);

// «verifier» describe a quien verifica: su entorno (qué llaves son de confianza) y su instalación.
// «schema_valid» dice si el contenido, por sí solo, cumple license.schema.json.
$testA = ['environment' => 'test', 'installation' => 'installation-fixture-a'];
$negative = [
	[
		'name' => 'altered-signature',
		'description' => 'Un bit de la firma cambiado.',
		'jws' => $headerSegment . '.' . $contentSegment . '.' . base64Url($badSignature),
		'verifier' => $testA,
		'expected_error' => 'invalid_signature',
		'schema_valid' => true,
	],
	[
		'name' => 'altered-content',
		'description' => 'El contenido cambia los cupos y conserva la firma original.',
		'jws' =>
			$headerSegment .
			'.' .
			base64Url(
				jsonText(
					array_replace($subscription, [
						'limits' => ['cash_registers' => 99, 'order_takers' => 99, 'kitchen_displays' => 99],
					]),
				),
			) .
			'.' .
			$signatureSegment,
		'verifier' => $testA,
		'expected_error' => 'invalid_signature',
		'schema_valid' => true,
	],
	[
		'name' => 'unknown-kid',
		'description' => 'El encabezado nombra una llave que no es de confianza.',
		'jws' => $resign($unknownHeader),
		'verifier' => $testA,
		'expected_error' => 'unknown_kid',
		'schema_valid' => true,
	],
	[
		'name' => 'previous-header-type',
		'description' => 'Encabezado con el tipo «lic+jws» del protocolo anterior.',
		'jws' => $resign($previousTypeHeader),
		'verifier' => $testA,
		'expected_error' => 'invalid_header',
		'schema_valid' => true,
	],
	[
		'name' => 'reordered-header',
		'description' => 'Encabezado con los mismos campos en otro orden: no es el encabezado exacto.',
		'jws' => $resign($reorderedHeader),
		'verifier' => $testA,
		'expected_error' => 'invalid_header',
		'schema_valid' => true,
	],
	[
		'name' => 'test-license-on-production-installation',
		'description' => 'Una licencia de prueba válida presentada a una instalación de producción.',
		'jws' => $valid['jws'],
		'verifier' => ['environment' => 'production', 'installation' => 'installation-fixture-a'],
		'expected_error' => 'unknown_kid',
		'schema_valid' => true,
	],
	[
		'name' => 'test-key-under-production-kid',
		'description' => 'Una llave de prueba firma con el nombre de una llave de producción.',
		'jws' => $signed($production, 'adelpos-prod-fixture-a', 'adelpos-test-fixture-a'),
		'verifier' => ['environment' => 'production', 'installation' => 'installation-fixture-a'],
		'expected_error' => 'invalid_signature',
		'schema_valid' => true,
	],
	[
		'name' => 'environment-does-not-match-kid',
		'description' => 'El contenido dice «production» y lo firma una llave de pruebas.',
		'jws' => $signed($production),
		'verifier' => $testA,
		'expected_error' => 'environment_mismatch',
		'schema_valid' => true,
	],
	[
		'name' => 'other-installation',
		'description' => 'Licencia válida de otra computadora: otra identidad y otra llave.',
		'jws' => $valid['jws'],
		'verifier' => ['environment' => 'test', 'installation' => 'installation-fixture-b'],
		'expected_error' => 'other_installation',
		'schema_valid' => true,
	],
	[
		'name' => 'other-installation-key',
		'description' => 'La misma identidad con la llave de otra computadora.',
		'jws' => $signed(
			array_replace($subscription, ['installation_public_key' => $installations['b']['public_key']]),
		),
		'verifier' => $testA,
		'expected_error' => 'other_installation',
		'schema_valid' => true,
	],
	[
		'name' => 'other-product',
		'description' => 'Licencia bien firmada de otro producto.',
		'jws' => $signed(array_replace($subscription, ['product_id' => 'otro_producto'])),
		'verifier' => $testA,
		'expected_error' => 'other_product',
		'schema_valid' => false,
	],
	[
		'name' => 'missing-field',
		'description' => 'Falta «limits».',
		'jws' => $signed($withoutLimits),
		'verifier' => $testA,
		'expected_error' => 'invalid_content',
		'schema_valid' => false,
	],
	[
		'name' => 'schema-version-2',
		'description' => '«schema_version» 2.0: el cliente la trata como incompatible.',
		'jws' => $signed(array_replace($subscription, ['schema_version' => '2.0'])),
		'verifier' => $testA,
		'expected_error' => 'incompatible_schema',
		'schema_valid' => false,
	],
	[
		'name' => 'wrong-types',
		'description' => 'La revisión y los cupos llegan como texto.',
		'jws' => $signed(
			array_replace($subscription, [
				'license_revision' => '1',
				'limits' => ['cash_registers' => '3', 'order_takers' => '3', 'kitchen_displays' => '3'],
			]),
		),
		'verifier' => $testA,
		'expected_error' => 'invalid_content',
		'schema_valid' => false,
	],
	[
		'name' => 'unlimited-does-not-exist',
		'description' => 'Un cupo vacío: en V1 no existe «sin límite».',
		'jws' => $signed(
			array_replace($subscription, [
				'limits' => ['cash_registers' => 3, 'order_takers' => null, 'kitchen_displays' => 3],
			]),
		),
		'verifier' => $testA,
		'expected_error' => 'invalid_content',
		'schema_valid' => false,
	],
	[
		'name' => 'local-time-with-offset',
		'description' => 'Un instante con desfase horario en lugar de UTC terminado en «Z».',
		'jws' => $signed(array_replace($subscription, ['expires_at' => '2026-12-16T00:30:00.000000-06:00'])),
		'verifier' => $testA,
		'expected_error' => 'invalid_content',
		'schema_valid' => false,
	],
	[
		'name' => 'revoked-without-reason',
		'description' => 'Revocada sin motivo.',
		'jws' => $signed(array_replace($subscription, ['license_status' => 'revoked'])),
		'verifier' => $testA,
		'expected_error' => 'invalid_content',
		'schema_valid' => false,
	],
];

// Regla de la revisión (contrato, sección 10, punto 4). «accepted» es lo que la instalación ya
// tenía aceptado para esa licencia antes de recibir «jws».
$sameRevisionOtherBytes = $signed(array_replace($subscription, ['issued_at' => '2026-11-16T06:31:00.000000Z']));
$accepted = fn(array $license): array => [
	'license_id' => $license['content']['license_id'],
	'license_revision' => $license['content']['license_revision'],
	'sha256' => $license['sha256'],
];
$revisionCases = [
	[
		'name' => 'first-license',
		'description' => 'No había ninguna licencia aceptada.',
		'accepted' => null,
		'jws' => $valid['jws'],
		'expected' => 'accepted',
	],
	[
		'name' => 'higher-revision',
		'description' => 'Llega la revisión 2 después de aceptar la 1.',
		'accepted' => $accepted($valid),
		'jws' => $licenses['closing-deactivated']['jws'],
		'expected' => 'accepted',
	],
	[
		'name' => 'same-revision-identical-bytes',
		'description' => 'La misma revisión, idéntica byte por byte, se vuelve a aceptar.',
		'accepted' => $accepted($valid),
		'jws' => $valid['jws'],
		'expected' => 'accepted',
	],
	[
		'name' => 'same-revision-other-bytes',
		'description' => 'La misma revisión con otro contenido se rechaza.',
		'accepted' => $accepted($valid),
		'jws' => $sameRevisionOtherBytes,
		'expected' => 'stale_revision',
	],
	[
		'name' => 'lower-revision',
		'description' => 'Llega la revisión 1 después de aceptar la 2: no se retrocede.',
		'accepted' => $accepted($licenses['closing-deactivated']),
		'jws' => $valid['jws'],
		'expected' => 'stale_revision',
	],
	[
		'name' => 'renewal-same-activation',
		'description' => 'Llega la renovación (revisión 2, misma activación) después de aceptar la revisión 1.',
		'accepted' => $accepted($licenses['subscription-annual']),
		'jws' => $licenses['renewed-subscription']['jws'],
		'expected' => 'accepted',
	],
	[
		'name' => 'other-license-is-independent',
		'description' => 'La revisión aceptada de otra licencia no cuenta.',
		'accepted' => $accepted($licenses['closing-transferred']),
		'jws' => $valid['jws'],
		'expected' => 'accepted',
	],
	// Transferencia por autoservicio (etapa 4): cada computadora acepta lo suyo.
	[
		'name' => 'transfer-new-installation-first-revision',
		'description' => 'La computadora nueva acepta su primera revisión (3) de una licencia que ya tenía revisiones.',
		'installation' => 'installation-fixture-b',
		'accepted' => null,
		'jws' => $licenses['self-service-transfer-new']['jws'],
		'expected' => 'accepted',
	],
	[
		'name' => 'transfer-old-installation-closing',
		'description' =>
			'La computadora anterior acepta su cierre «transferred» (revisión 2), mayor que su revisión 1.',
		'installation' => 'installation-fixture-a',
		'accepted' => $accepted($licenses['self-service-transfer-before']),
		'jws' => $licenses['self-service-transfer-closing']['jws'],
		'expected' => 'accepted',
	],
	[
		'name' => 'transfer-new-revision-is-not-for-the-old-installation',
		'description' => 'La revisión de la computadora nueva no sirve en la anterior: es de otra instalación.',
		'installation' => 'installation-fixture-a',
		'accepted' => $accepted($licenses['self-service-transfer-before']),
		'jws' => $licenses['self-service-transfer-new']['jws'],
		'expected' => 'other_installation',
	],
];
// Las demás las verifica la instalación a.
foreach ($revisionCases as $index => $case) {
	$revisionCases[$index] = array_merge(
		['name' => $case['name'], 'description' => $case['description'], 'installation' => 'installation-fixture-a'],
		$case,
	);
}

// ---------------------------------------------------------------------------------------------
// Fechas (contrato, secciones 5.1, 5.2, 5.4 y 5.5). Las calcula solo el servidor.
// Cada resultado está calculado a mano. America/Monterrey va en UTC-6 todo el año.
// America/Tijuana sigue el horario de Estados Unidos: en 2026 cambia el 1 de noviembre a las
// 09:00Z; en 2027, el 14 de marzo a las 10:00Z y el 7 de noviembre a las 09:00Z.
// ---------------------------------------------------------------------------------------------

$monterrey = 'America/Monterrey';
$tijuana = 'America/Tijuana';
$addMonths = fn(string $name, string $start, int $months, string $zone, string $expected): array => [
	'name' => $name,
	'time_zone' => $zone,
	'start' => $start,
	'months' => $months,
	'expected' => $expected,
];
$dates = [
	// Sumar meses conserva el día ancla y la hora local del inicio.
	'add_months' => [
		$addMonths(
			'31 de enero más 1 mes',
			'2026-01-31T16:00:00.000000Z',
			1,
			$monterrey,
			'2026-02-28T16:00:00.000000Z',
		),
		$addMonths(
			'31 de enero más 2 meses',
			'2026-01-31T16:00:00.000000Z',
			2,
			$monterrey,
			'2026-03-31T16:00:00.000000Z',
		),
		$addMonths(
			'31 de enero más 3 meses',
			'2026-01-31T16:00:00.000000Z',
			3,
			$monterrey,
			'2026-04-30T16:00:00.000000Z',
		),
		$addMonths(
			'31 de enero más 12 meses',
			'2026-01-31T16:00:00.000000Z',
			12,
			$monterrey,
			'2027-01-31T16:00:00.000000Z',
		),
		$addMonths(
			'31 de enero más 1 mes en año bisiesto',
			'2028-01-31T16:00:00.000000Z',
			1,
			$monterrey,
			'2028-02-29T16:00:00.000000Z',
		),
		$addMonths(
			'29 de febrero más 12 meses',
			'2028-02-29T16:00:00.000000Z',
			12,
			$monterrey,
			'2029-02-28T16:00:00.000000Z',
		),
		$addMonths(
			'29 de febrero más 48 meses',
			'2028-02-29T16:00:00.000000Z',
			48,
			$monterrey,
			'2032-02-29T16:00:00.000000Z',
		),
		$addMonths(
			'30 de noviembre más 3 meses, con cambio de año',
			'2026-11-30T16:00:00.000000Z',
			3,
			$monterrey,
			'2027-02-28T16:00:00.000000Z',
		),
		$addMonths(
			'el día local no es el día en UTC',
			'2026-03-01T03:30:00.000000Z',
			1,
			$monterrey,
			'2026-03-29T03:30:00.000000Z',
		),
		$addMonths(
			'termina el horario de verano y el mes no tiene día 31',
			'2026-10-31T12:00:00.000000Z',
			1,
			$tijuana,
			'2026-11-30T13:00:00.000000Z',
		),
		$addMonths(
			'la hora local no existe: primer instante después del salto',
			'2027-02-14T10:30:00.000000Z',
			1,
			$tijuana,
			'2027-03-14T10:00:00.000000Z',
		),
		$addMonths(
			'la hora local se repite: se usa la primera',
			'2027-10-07T08:30:00.000000Z',
			1,
			$tijuana,
			'2027-11-07T08:30:00.000000Z',
		),
	],
	// «15 días» son 15 días de calendario a la misma hora local.
	'add_days' => [
		[
			'name' => '15 días',
			'time_zone' => $monterrey,
			'start' => '2026-09-25T12:00:00.000000Z',
			'days' => 15,
			'expected' => '2026-10-10T12:00:00.000000Z',
		],
		[
			'name' => '15 días a través del fin del horario de verano',
			'time_zone' => $tijuana,
			'start' => '2026-10-25T12:00:00.000000Z',
			'days' => 15,
			'expected' => '2026-11-09T13:00:00.000000Z',
		],
	],
	// Primer periodo de cada tipo de licencia, activada el 2026-11-16 a las 00:30 de Monterrey.
	'first_period' => [],
];
$firstPeriods = [
	[
		'perpetua sin actualizaciones',
		['license_type' => 'perpetual', 'updates_months' => 0],
		null,
		'2026-11-16T06:30:00.000000Z',
	],
	[
		'perpetua con 6 meses',
		['license_type' => 'perpetual', 'updates_months' => 6],
		null,
		'2027-05-16T06:30:00.000000Z',
	],
	[
		'perpetua con 12 meses',
		['license_type' => 'perpetual', 'updates_months' => 12],
		null,
		'2027-11-16T06:30:00.000000Z',
	],
	[
		'suscripción mensual',
		['license_type' => 'subscription', 'billing_period' => 'monthly'],
		'2026-12-16T06:30:00.000000Z',
		null,
	],
	[
		'suscripción trimestral',
		['license_type' => 'subscription', 'billing_period' => 'quarterly'],
		'2027-02-16T06:30:00.000000Z',
		null,
	],
	[
		'suscripción semestral',
		['license_type' => 'subscription', 'billing_period' => 'semiannual'],
		'2027-05-16T06:30:00.000000Z',
		null,
	],
	[
		'suscripción anual',
		['license_type' => 'subscription', 'billing_period' => 'annual'],
		'2027-11-16T06:30:00.000000Z',
		null,
	],
	['prueba de 15 días', ['license_type' => 'trial', 'trial_duration' => '15d'], '2026-12-01T06:30:00.000000Z', null],
	['prueba de 1 mes', ['license_type' => 'trial', 'trial_duration' => '1m'], '2026-12-16T06:30:00.000000Z', null],
	['prueba de 3 meses', ['license_type' => 'trial', 'trial_duration' => '3m'], '2027-02-16T06:30:00.000000Z', null],
];
foreach ($firstPeriods as [$name, $terms, $expires, $updates]) {
	$dates['first_period'][] = [
		'name' => $name,
		'time_zone' => $monterrey,
		'terms' => $terms,
		'start' => '2026-11-16T06:30:00.000000Z',
		'expected' => [
			'period_starts_at' => '2026-11-16T06:30:00.000000Z',
			'expires_at' => $expires,
			// En suscripción y prueba, las actualizaciones terminan con la vigencia.
			'updates_until' => $updates ?? $expires,
			'anchor_day' => 16,
			'anchor_local_time' => '00:30:00.000000',
		],
	];
}

// Renovación de una suscripción (§5.3): límite = vencimiento + grace_days × 24 h, exclusivo. Antes del
// límite, la base es el vencimiento y se conserva el ancla; desde el límite, la base es la fecha de pago.
// Vence el 2 de marzo de 2027 a las 10:00 de Monterrey (16:00 UTC), con día ancla 2.
$renewal = fn(
	string $name,
	string $zone,
	string $expires,
	string $paid,
	string $period,
	int $day,
	string $time,
	array $expected,
): array => [
	'name' => $name,
	'time_zone' => $zone,
	'expires_at' => $expires,
	'grace_days' => 5,
	'paid_at' => $paid,
	'billing_period' => $period,
	'anchor_day' => $day,
	'anchor_local_time' => $time,
	'expected' => [
		'period_starts_at' => $expected[0],
		'expires_at' => $expected[1],
		'anchor_day' => $expected[2],
		'anchor_local_time' => $expected[3],
	],
];
$march2 = '2027-03-02T16:00:00.000000Z';
$ten = '10:00:00.000000';
$dates['renewal'] = [
	$renewal(
		'paga el 1 de marzo, antes de vencer',
		$monterrey,
		$march2,
		'2027-03-01T16:00:00.000000Z',
		'monthly',
		2,
		$ten,
		[$march2, '2027-04-02T16:00:00.000000Z', 2, $ten],
	),
	$renewal(
		'paga el 5 de marzo, dentro de la tolerancia',
		$monterrey,
		$march2,
		'2027-03-05T16:00:00.000000Z',
		'monthly',
		2,
		$ten,
		[$march2, '2027-04-02T16:00:00.000000Z', 2, $ten],
	),
	$renewal(
		'un microsegundo antes del límite de la tolerancia',
		$monterrey,
		$march2,
		'2027-03-07T15:59:59.999999Z',
		'monthly',
		2,
		$ten,
		[$march2, '2027-04-02T16:00:00.000000Z', 2, $ten],
	),
	$renewal(
		'paga el 7 de marzo a las 10:00, justo en el límite',
		$monterrey,
		$march2,
		'2027-03-07T16:00:00.000000Z',
		'monthly',
		2,
		$ten,
		['2027-03-07T16:00:00.000000Z', '2027-04-07T16:00:00.000000Z', 7, $ten],
	),
	$renewal('paga el 20 de marzo', $monterrey, $march2, '2027-03-20T16:00:00.000000Z', 'monthly', 2, $ten, [
		'2027-03-20T16:00:00.000000Z',
		'2027-04-20T16:00:00.000000Z',
		20,
		$ten,
	]),
	// Día ancla 31: el 28 de febrero vuelve al 31 de marzo, y el 31 de marzo pasa al 30 de abril.
	$renewal(
		'encadenada con día ancla 31, desde febrero',
		$monterrey,
		'2027-02-28T16:00:00.000000Z',
		'2027-02-20T16:00:00.000000Z',
		'monthly',
		31,
		$ten,
		['2027-02-28T16:00:00.000000Z', '2027-03-31T16:00:00.000000Z', 31, $ten],
	),
	$renewal(
		'encadenada con día ancla 31, desde marzo',
		$monterrey,
		'2027-03-31T16:00:00.000000Z',
		'2027-03-20T16:00:00.000000Z',
		'monthly',
		31,
		$ten,
		['2027-03-31T16:00:00.000000Z', '2027-04-30T16:00:00.000000Z', 31, $ten],
	),
	$renewal(
		'cambio de mensual a anual al renovar',
		$monterrey,
		$march2,
		'2027-03-01T16:00:00.000000Z',
		'annual',
		2,
		$ten,
		[$march2, '2028-03-02T16:00:00.000000Z', 2, $ten],
	),
	// En Tijuana vence a las 10:00 del horario estándar (18:00 UTC); el periodo nuevo vence a las 10:00
	// del horario de verano (17:00 UTC).
	$renewal(
		'zona del cliente con cambio de horario',
		$tijuana,
		'2027-03-02T18:00:00.000000Z',
		'2027-03-01T18:00:00.000000Z',
		'monthly',
		2,
		$ten,
		['2027-03-02T18:00:00.000000Z', '2027-04-02T17:00:00.000000Z', 2, $ten],
	),
];
// Conversión de una prueba activada en licencia de pago (§5.4): el periodo nuevo empieza en la fecha de
// pago, el 25 de noviembre de 2026 a las 12:00 de Monterrey, con ese día y esa hora como ancla.
$conversion = fn(string $name, array $terms, ?string $expires, string $updates): array => [
	'name' => $name,
	'time_zone' => $monterrey,
	'terms' => $terms,
	'paid_at' => '2026-11-25T18:00:00.000000Z',
	'expected' => [
		'period_starts_at' => '2026-11-25T18:00:00.000000Z',
		'expires_at' => $expires,
		'updates_until' => $updates,
		'anchor_day' => 25,
		'anchor_local_time' => '12:00:00.000000',
	],
];
$dates['conversion'] = [
	$conversion(
		'prueba a suscripción mensual',
		['license_type' => 'subscription', 'billing_period' => 'monthly'],
		'2026-12-25T18:00:00.000000Z',
		'2026-12-25T18:00:00.000000Z',
	),
	$conversion(
		'prueba a suscripción anual',
		['license_type' => 'subscription', 'billing_period' => 'annual'],
		'2027-11-25T18:00:00.000000Z',
		'2027-11-25T18:00:00.000000Z',
	),
	$conversion(
		'prueba a perpetua con 6 meses',
		['license_type' => 'perpetual', 'updates_months' => 6],
		null,
		'2027-05-25T18:00:00.000000Z',
	),
	$conversion(
		'prueba a perpetua sin actualizaciones',
		['license_type' => 'perpetual', 'updates_months' => 0],
		null,
		'2026-11-25T18:00:00.000000Z',
	),
];
// Mantenimiento de una perpetua (§5.5 y decisión 4 del propietario): 12 meses desde la fecha posterior
// entre el pago y el fin vigente de las actualizaciones.
$maintenance = fn(string $name, string $until, int $day, string $time, string $paid, array $expected): array => [
	'name' => $name,
	'time_zone' => $monterrey,
	'updates_until' => $until,
	'anchor_day' => $day,
	'anchor_local_time' => $time,
	'paid_at' => $paid,
	'expected' => [
		'updates_from' => $expected[0],
		'updates_until' => $expected[1],
		'anchor_day' => $expected[2],
		'anchor_local_time' => $expected[3],
	],
];
$dates['maintenance'] = [
	$maintenance(
		'antes de que terminen las actualizaciones',
		'2027-05-16T06:30:00.000000Z',
		16,
		'00:30:00.000000',
		'2027-04-01T18:00:00.000000Z',
		['2027-05-16T06:30:00.000000Z', '2028-05-16T06:30:00.000000Z', 16, '00:30:00.000000'],
	),
	$maintenance(
		'encadenado: la segunda compra se suma al fin nuevo',
		'2028-05-16T06:30:00.000000Z',
		16,
		'00:30:00.000000',
		'2027-06-01T18:00:00.000000Z',
		['2028-05-16T06:30:00.000000Z', '2029-05-16T06:30:00.000000Z', 16, '00:30:00.000000'],
	),
	$maintenance(
		'después de que terminaron: desde la fecha de pago',
		'2026-11-16T06:30:00.000000Z',
		16,
		'00:30:00.000000',
		'2027-01-20T18:00:00.000000Z',
		['2027-01-20T18:00:00.000000Z', '2028-01-20T18:00:00.000000Z', 20, '12:00:00.000000'],
	),
];

// ---------------------------------------------------------------------------------------------
// Bonificación de un cambio de plan (decisión 6 del propietario para la etapa 3, corregida por la
// decisión 3 para la etapa 4; propuestas V1.1 n.º 27 y 28). Cuentan todos los periodos pagados que
// todavía no terminan en el instante del pago:
//
//     valor del periodo = monto cobrado + bonificación aplicada en ese pago
//     parte             = valor del periodo × segundos no consumidos ÷ segundos del periodo
//
// Los segundos son completos, un periodo que todavía no empieza cuenta completo, cada parte se redondea
// al centavo (la mitad hacia arriba) y después se suman. Los montos van en centavos y en texto MXN.
// Útiles si algún día AD-EL POS muestra montos.
// ---------------------------------------------------------------------------------------------

$mxn = fn(int $cents): string => sprintf('%d.%02d', intdiv($cents, 100), $cents % 100);
// Un periodo pagado: lo cobrado, la bonificación que se aplicó en ese pago, su inicio y su fin.
$paidPeriod = fn(int $amount, int $applied, string $start, string $end): array => [
	'amount_cents' => $amount,
	'amount_mxn' => $mxn($amount),
	'applied_credit_cents' => $applied,
	'applied_credit_mxn' => $mxn($applied),
	'value_cents' => $amount + $applied,
	'value_mxn' => $mxn($amount + $applied),
	'period_starts_at' => $start,
	'expires_at' => $end,
];
// Resultado esperado de un periodo: segundos no consumidos, segundos del periodo y su parte.
$part = fn(int $remaining, int $period, int $credit): array => [
	'remaining_seconds' => $remaining,
	'period_seconds' => $period,
	'credit_cents' => $credit,
	'credit_mxn' => $mxn($credit),
];
$credit = fn(string $name, string $paidAt, array $periods, array $parts): array => [
	'name' => $name,
	'paid_at' => $paidAt,
	'periods' => $periods,
	'expected' => [
		'parts' => $parts,
		'credit_cents' => array_sum(array_column($parts, 'credit_cents')),
		'credit_mxn' => $mxn(array_sum(array_column($parts, 'credit_cents'))),
	],
];
$march = ['2027-03-01T16:00:00.000000Z', '2027-04-01T16:00:00.000000Z'];
$april = ['2027-04-01T16:00:00.000000Z', '2027-05-01T16:00:00.000000Z'];
// Del 16 de marzo al 16 de abril (31 días) y del 16 de abril al 16 de mayo (30 días), a las 10:00 de Monterrey.
$sixteenth = ['2027-03-16T16:00:00.000000Z', '2027-04-16T16:00:00.000000Z'];
$nextSixteenth = ['2027-04-16T16:00:00.000000Z', '2027-05-16T16:00:00.000000Z'];
$credits = [
	// 900.00 × 1 382 400 ÷ 2 678 400 = 464.516… → 464.52
	$credit(
		'mensual de 900.00, faltan 16 de 31 días',
		'2027-03-16T16:00:00.000000Z',
		[$paidPeriod(90000, 0, ...$march)],
		[$part(1382400, 2678400, 46452)],
	),
	// 850.00 × 16 ÷ 31 = 438.709… → 438.71
	$credit(
		'mensual de 850.00, faltan 16 de 31 días',
		'2027-03-16T16:00:00.000000Z',
		[$paidPeriod(85000, 0, ...$march)],
		[$part(1382400, 2678400, 43871)],
	),
	// Anual de 9,000.00 del 1 de marzo de 2027 al 1 de marzo de 2028 (366 días); faltan 351 días.
	$credit(
		'anual de 9,000.00, mayor que el precio nuevo de un plan mensual',
		'2027-03-16T16:00:00.000000Z',
		[$paidPeriod(900000, 0, '2027-03-01T16:00:00.000000Z', '2028-03-01T16:00:00.000000Z')],
		[$part(30326400, 31622400, 863115)],
	),
	$credit(
		'al empezar el periodo, todo su valor',
		'2027-03-01T16:00:00.000000Z',
		[$paidPeriod(90000, 0, ...$march)],
		[$part(2678400, 2678400, 90000)],
	),
	$credit(
		'al vencer, nada',
		'2027-04-01T16:00:00.000000Z',
		[$paidPeriod(90000, 0, ...$march)],
		[$part(0, 2678400, 0)],
	),
	$credit(
		'un periodo que todavía no empieza cuenta completo',
		'2027-02-20T16:00:00.000000Z',
		[$paidPeriod(90000, 0, ...$march)],
		[$part(2678400, 2678400, 90000)],
	),
	// 1 centavo × 1 s ÷ 2 s = 0.5 → 1 centavo.
	$credit(
		'la mitad de un centavo sube',
		'2027-01-01T00:00:01.000000Z',
		[$paidPeriod(1, 0, '2027-01-01T00:00:00.000000Z', '2027-01-01T00:00:02.000000Z')],
		[$part(1, 2, 1)],
	),
	// Faltan 1.999999 s: cuentan como 1. 3.00 × 1 ÷ 3 = 1.00.
	$credit(
		'segundos completos',
		'2027-01-01T00:00:01.000001Z',
		[$paidPeriod(300, 0, '2027-01-01T00:00:00.000000Z', '2027-01-01T00:00:03.000000Z')],
		[$part(1, 3, 100)],
	),
	// Renovó el 20 de marzo un mes por adelantado (900.00) y el 28 de marzo sube de plan. Del periodo en curso
	// faltan 4 de 31 días: 900.00 × 4 ÷ 31 = 116.129… → 116.13. El mes de abril no ha empezado: cuenta
	// completo, 900.00. Total: 1,016.13.
	$credit(
		'renovación anticipada con subida de plan: 4 días del periodo en curso más el mes pagado por adelantado',
		'2027-03-28T16:00:00.000000Z',
		[$paidPeriod(90000, 0, ...$march), $paidPeriod(90000, 0, ...$april)],
		[$part(345600, 2678400, 11613), $part(2592000, 2592000, 90000)],
	),
	// El 16 de marzo subió del plan básico (400.00, faltaban 16 de 31 días: 206.45) al elemental (900.00):
	// cobró 693.55. Ese periodo vale 693.55 + 206.45 = 900.00. El 1 de abril vuelve a subir y faltan 15 de
	// 31 días: 900.00 × 15 ÷ 31 = 435.483… → 435.48. Con solo lo cobrado habría sido 335.59.
	$credit(
		'segunda subida sobre un periodo pagado con bonificación: vale lo cobrado más la bonificación',
		'2027-04-01T16:00:00.000000Z',
		[$paidPeriod(69355, 20645, ...$sixteenth)],
		[$part(1296000, 2678400, 43548)],
	),
	// El mismo periodo pagado con bonificación (900.00), más abril-mayo renovado por adelantado el 8 de abril
	// (900.00). El 10 de abril sube de plan: faltan 6 de 31 días del periodo en curso, 900.00 × 6 ÷ 31 =
	// 174.193… → 174.19, y el mes renovado cuenta completo. Total: 1,074.19.
	$credit(
		'combinación: periodo pagado con bonificación más el mes renovado por adelantado',
		'2027-04-10T16:00:00.000000Z',
		[$paidPeriod(69355, 20645, ...$sixteenth), $paidPeriod(90000, 0, ...$nextSixteenth)],
		[$part(518400, 2678400, 17419), $part(2592000, 2592000, 90000)],
	),
];

// ---------------------------------------------------------------------------------------------
// Estado temporal, calculado por el cliente (contrato, sección 5.6)
//   ahora_efectivo = máximo(reloj local, última hora confiable)
// ---------------------------------------------------------------------------------------------

$state = fn(string $name, string $license, string $clock, ?string $trusted, string $expected): array => [
	'name' => $name,
	'license' => $license,
	'local_clock' => $clock,
	'last_trusted_time' => $trusted,
	'expected_state' => $expected,
];
$temporalStates = [
	// Suscripción mensual: vence el 2026-12-16T06:30Z y su tolerancia termina 5 × 86 400 s después.
	$state('un microsegundo antes de vencer', 'subscription-monthly', '2026-12-16T06:29:59.999999Z', null, 'active'),
	$state(
		'en el instante de vencer empieza la tolerancia',
		'subscription-monthly',
		'2026-12-16T06:30:00.000000Z',
		null,
		'grace',
	),
	$state(
		'un microsegundo antes de agotar la tolerancia',
		'subscription-monthly',
		'2026-12-21T06:29:59.999999Z',
		null,
		'grace',
	),
	$state(
		'en el límite de la tolerancia ya está vencida',
		'subscription-monthly',
		'2026-12-21T06:30:00.000000Z',
		null,
		'expired',
	),
	// Prueba: sin tolerancia.
	$state('prueba un microsegundo antes de vencer', 'trial-15-days', '2026-12-01T06:29:59.999999Z', null, 'active'),
	$state('prueba en el instante de vencer', 'trial-15-days', '2026-12-01T06:30:00.000000Z', null, 'expired'),
	// Perpetua: no vence, aunque hayan terminado sus actualizaciones.
	$state('perpetua diez años después', 'perpetual-0-months', '2036-11-16T06:30:00.000000Z', null, 'active'),
	// Atrasar el reloj nunca prolonga una licencia.
	$state(
		'reloj atrasado a antes del vencimiento',
		'subscription-monthly',
		'2026-11-20T00:00:00.000000Z',
		'2026-12-25T00:00:00.000000Z',
		'expired',
	),
	$state(
		'reloj atrasado dentro de la tolerancia',
		'subscription-monthly',
		'2026-12-01T00:00:00.000000Z',
		'2026-12-18T00:00:00.000000Z',
		'grace',
	),
	$state(
		'reloj correcto con una hora confiable anterior',
		'subscription-monthly',
		'2026-12-25T00:00:00.000000Z',
		'2026-11-20T00:00:00.000000Z',
		'expired',
	),
	$state(
		'prueba con el reloj atrasado',
		'trial-15-days',
		'2026-11-17T00:00:00.000000Z',
		'2026-12-01T06:30:00.000000Z',
		'expired',
	),
	// Una licencia revocada está revocada en cualquier momento.
	$state(
		'revisión de cierre antes del vencimiento',
		'closing-deactivated',
		'2026-11-21T00:00:00.000000Z',
		null,
		'revoked',
	),
	$state('revisión de cierre de una perpetua', 'closing-transferred', '2030-01-01T00:00:00.000000Z', null, 'revoked'),
];

// ---------------------------------------------------------------------------------------------
// Pruebas de posesión (contrato, sección 8.2), para las cuatro acciones
// ---------------------------------------------------------------------------------------------

$proofs = [];
foreach (['activate', 'transfer', 'refresh', 'deactivate'] as $action) {
	$activationId = in_array($action, ['activate', 'transfer'], true) ? null : $subscription['activation_id'];
	$challengeId = '55555555-5555-4555-8555-555555555555';
	$nonce = base64Url(hash('sha256', 'ADELPOS LICENSE PUBLIC TEST NONCE ' . $action, true));
	$message = implode("\n", [
		PROOF_PREFIX,
		$action,
		$challengeId,
		$nonce,
		'adel_pos',
		$installations['a']['server_id'],
		$activationId ?? '-',
	]);
	$proofs[] = [
		'action' => $action,
		'installation' => 'installation-fixture-a',
		'challenge_id' => $challengeId,
		'nonce' => $nonce,
		'product_id' => 'adel_pos',
		'server_id' => $installations['a']['server_id'],
		'activation_id' => $activationId,
		'message' => $message,
		'proof' => sign($installations['a'], $message),
	];
}

// ---------------------------------------------------------------------------------------------
// Solicitudes sin internet «.licreq» (contrato, sección 9.1), para las tres acciones
// ---------------------------------------------------------------------------------------------

$requests = [];
foreach (['activate', 'renew', 'deactivate'] as $index => $action) {
	$payload = [
		'action' => $action,
		'request_id' => '44444444-4444-4444-8444-44444444444' . ($index + 1),
		'created_at' => '2026-11-16T06:00:00Z',
		'product_id' => 'adel_pos',
		'server_id' => $installations['a']['server_id'],
		'installation_public_key' => $installations['a']['public_key'],
		'app_version' => '0.12.0',
	];
	if ($action === 'activate') {
		// Código comercial de ejemplo, el del contrato. No corresponde a ninguna licencia.
		$payload['license_key'] = 'K7M-9R3X-6TVP-8CHD';
	} else {
		$payload['license_id'] = $subscription['license_id'];
		$payload['activation_id'] = $subscription['activation_id'];
	}
	$payloadJson = jsonText($payload);
	$segment = base64Url($payloadJson);
	$requests[] = [
		'action' => $action,
		'installation' => 'installation-fixture-a',
		'payload' => $payload,
		'payload_json' => $payloadJson,
		'message' => REQUEST_PREFIX . $segment,
		'envelope_json' => jsonText([
			'schema_version' => '1.0',
			'payload_b64u' => $segment,
			'signature_b64u' => sign($installations['a'], REQUEST_PREFIX . $segment),
		]),
	];
}

// ---------------------------------------------------------------------------------------------
// Archivos
// ---------------------------------------------------------------------------------------------

$publicKeys = [];
foreach ($signingKeys as $name => $key) {
	$publicKeys[] = [
		// «name» es el nombre público del que sale la semilla: sha256(prefijo + name).
		'name' => $name,
		'kid' => $name,
		'environment' => $key['environment'],
		'seed_hex' => $key['seed_hex'],
		'public_key' => $key['public_key'],
	];
}
$publicInstallations = [];
foreach ($installations as $name => $installation) {
	$publicInstallations[] = [
		'name' => 'installation-fixture-' . $name,
		'server_id' => $installation['server_id'],
		'seed_hex' => $installation['seed_hex'],
		'public_key' => $installation['public_key'],
	];
}
$vectors = [
	'fixture_version' => '1.0',
	'contract' => 'Contrato de licencias de AD-EL POS V1.0',
	'warning' => 'SOLO PARA PRUEBAS. Las llaves son públicas: nunca se debe confiar en ellas en producción.',
	'signing_keys' => $publicKeys,
	'installations' => $publicInstallations,
	'licenses' => array_values($licenses),
	'negative_licenses' => $negative,
	'revision_cases' => $revisionCases,
	'dates' => $dates,
	'credits' => $credits,
	'temporal_states' => $temporalStates,
	'proofs' => $proofs,
	'offline_requests' => $requests,
];

$directory = __DIR__ . '/fixtures';
foreach ($licenses as $license) {
	writeFixture($directory . '/licenses/' . $license['name'] . '.lic', $license['jws']);
}
foreach ($requests as $request) {
	writeFixture($directory . '/requests/' . $request['action'] . '.licreq', $request['envelope_json']);
}
writeFixture(
	$directory . '/vectors.json',
	json_encode($vectors, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_THROW_ON_ERROR) .
		"\n",
);

// Ningún archivo de una versión anterior debe quedar en la carpeta.
$expected = ['vectors.json'];
foreach ($licenses as $license) {
	$expected[] = 'licenses/' . $license['name'] . '.lic';
}
foreach ($requests as $request) {
	$expected[] = 'requests/' . $request['action'] . '.licreq';
}
$found = [];
$files = new RecursiveIteratorIterator(new RecursiveDirectoryIterator($directory, FilesystemIterator::SKIP_DOTS));
foreach ($files as $file) {
	$found[] = substr($file->getPathname(), strlen($directory) + 1);
}
sort($expected);
sort($found);
if ($found !== $expected) {
	throw new RuntimeException(
		'La carpeta de archivos de prueba tiene archivos de más o de menos: ' .
			implode(', ', array_merge(array_diff($found, $expected), array_diff($expected, $found))),
	);
}

printf(
	'%s: %d licencias, %d casos negativos, %d casos de revisión, %d casos de fechas, %d casos de bonificación, ' .
		"%d estados temporales, %d pruebas de posesión y %d solicitudes .licreq.\n",
	in_array('--check', $argv, true) ? 'Comprobado' : 'Generado',
	count($licenses),
	count($negative),
	count($revisionCases),
	array_sum(array_map('count', $dates)),
	count($credits),
	count($temporalStates),
	count($proofs),
	count($requests),
);
