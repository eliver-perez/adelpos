// Comandos de desarrollo en Windows (etapa 9E). Equivalente de scripts/dev.sh, scripts/database.py,
// scripts/server-process.py, scripts/browser-tests.py y scripts/restore-check.py, que siguen siendo los de
// macOS. Se ejecuta con scripts\dev.ps1, que antes carga scripts\env.ps1 (herramientas de .tools en el PATH).
//
// Reglas que se conservan de los scripts de macOS:
// - El clúster de desarrollo es exclusivo del proyecto (.local\postgres, puerto 55432, solo 127.0.0.1) y lleva
//   un marcador de propiedad; nunca se opera sobre un clúster ajeno.
// - Los secretos se escriben en archivos de .local sin mostrarse; nunca se pasan como argumentos.
// - Las pruebas de integración y de navegador usan bases desechables adel_pos_test_*.
// - El servidor se detiene en orden (comando «stop» de Go con el evento de parada), nunca a la fuerza.
import { spawn, spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
const LOCAL = path.join(ROOT, '.local');
const DATA = path.join(LOCAL, 'postgres');
const PORT = '55432';
const PRODUCT = JSON.parse(fs.readFileSync(path.join(ROOT, 'config', 'product.json'), 'utf8'));
const DB_NAME = PRODUCT.database_name;
const PG_BIN = process.env.ADEL_PG_BIN || path.join(ROOT, '.tools', 'pgsql', 'bin');
const SERVER_BINARY = path.join(ROOT, 'bin', `${PRODUCT.server_binary}.exe`);
const SERVER_CONFIG = process.env.ADEL_SERVER_CONFIG || path.join(LOCAL, 'server.json');
const PID_FILE = path.join(LOCAL, 'server.pid');
const NPM_CLI = path.join(
  path.dirname(process.execPath),
  'node_modules',
  'npm',
  'bin',
  'npm-cli.js',
);
const NPX_CLI = path.join(
  path.dirname(process.execPath),
  'node_modules',
  'npm',
  'bin',
  'npx-cli.js',
);

// Paquetes con pruebas de integración: la misma tupla de scripts/database.py.
const INTEGRATION_PACKAGES = [
  './internal/database',
  './internal/administration',
  './internal/catalog',
  './internal/operations',
  './internal/payments',
  './internal/reports',
  './internal/reports/operational',
  './internal/designer',
  './internal/identity',
];

class CommandError extends Error {}

function fail(message) {
  throw new CommandError(message);
}

// run ejecuta un programa con argumentos separados (nunca un shell construido con datos) y falla si
// termina con error.
function run(program, args, options = {}) {
  let stdio = options.capture ? 'pipe' : 'inherit';
  if (options.input !== undefined) {
    stdio = ['pipe', 'inherit', 'inherit'];
  }
  if (options.detachOutput) {
    stdio = 'ignore';
  }
  const result = spawnSync(program, args, {
    cwd: options.cwd ?? ROOT,
    env: options.env ?? process.env,
    input: options.input,
    stdio,
    encoding: options.capture ? 'utf8' : undefined,
    windowsHide: true,
  });
  if (result.error) {
    fail(`No se pudo ejecutar ${path.basename(program)}: ${result.error.message}`);
  }
  if (result.status !== 0 && !options.allowFailure) {
    fail(`${path.basename(program)} terminó con código ${result.status}.`);
  }
  return result;
}

function npm(args, cwd) {
  return run(process.execPath, [NPM_CLI, ...args], { cwd });
}

function pg(tool) {
  return path.join(PG_BIN, `${tool}.exe`);
}

function writeJSON(filename, value, exclusive) {
  // Sin BOM: Go rechaza un JSON que empiece con él.
  fs.writeFileSync(filename, JSON.stringify(value, null, 2) + '\n', {
    flag: exclusive ? 'wx' : 'w',
  });
}

function secretFile(filename) {
  if (!fs.existsSync(filename)) {
    fs.writeFileSync(filename, randomBytes(24).toString('hex'), { flag: 'wx' });
  }
  return fs.readFileSync(filename, 'utf8').trim();
}

function adminEnv(database = 'postgres') {
  return {
    ...process.env,
    PGHOST: '127.0.0.1',
    PGPORT: PORT,
    PGUSER: 'adel_dev_admin',
    PGDATABASE: database,
    PGPASSWORD: fs.readFileSync(path.join(LOCAL, 'admin-password'), 'utf8').trim(),
    PGCONNECT_TIMEOUT: '3',
    PGCLIENTENCODING: 'UTF8',
  };
}

// ensureOwned exige el marcador del clúster exclusivo antes de tocarlo.
function ensureOwned() {
  const marker = path.join(LOCAL, 'cluster-owner.json');
  if (!fs.existsSync(marker)) {
    fail(
      'No existe el marcador del clúster exclusivo. No se operará sobre otros datos; ejecuta db-init.',
    );
  }
  const owner = JSON.parse(fs.readFileSync(marker, 'utf8'));
  if (owner.data_directory !== DATA || owner.port !== PORT) {
    fail('El marcador no coincide con este clúster. Revisión manual necesaria.');
  }
}

function clusterRunning() {
  return (
    run(pg('pg_ctl'), ['-D', DATA, 'status'], { capture: true, allowFailure: true }).status === 0
  );
}

function dbStart() {
  ensureOwned();
  if (clusterRunning()) {
    console.log('PostgreSQL del proyecto ya está activo.');
    return;
  }
  // En Windows, PostgreSQL hereda las salidas de pg_ctl y pg_ctl las de quien lo lanza: con un hijo de
  // Node heredaría también la tubería de esta consola y quien la lea (por ejemplo «| Out-Null») esperaría a
  // que PostgreSQL termine. pg_ctl se lanza sin heredar nada y su salida va a .local\postgres.log.
  const status = runWithoutInheritance(pg('pg_ctl'), [
    '-D',
    DATA,
    '-l',
    path.join(LOCAL, 'postgres.log'),
    '-w',
    'start',
  ]);
  if (status !== 0) {
    fail('PostgreSQL no pudo iniciar. Revisa .local\\postgres.log.');
  }
  console.log('PostgreSQL del proyecto iniciado.');
}

// runWithoutInheritance ejecuta un programa oculto con Start-Process (ShellExecute: sin heredar
// identificadores de este proceso), espera a que termine solo él (no sus descendientes) y devuelve su código.
function runWithoutInheritance(program, args) {
  const single = (value) => `'${value.replaceAll("'", "''")}'`;
  const script =
    `$p = Start-Process -FilePath ${single(program)} ` +
    `-ArgumentList ${single(args.map(quoteArgument).join(' '))} ` +
    // $p.Handle se lee antes de esperar: sin eso, Windows PowerShell 5.1 no conserva el código de salida.
    `-WorkingDirectory ${single(ROOT)} -WindowStyle Hidden -PassThru; $null = $p.Handle; ` +
    `$p.WaitForExit(); exit $p.ExitCode`;
  return run('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], {
    capture: true,
    allowFailure: true,
  }).status;
}

function dbStop() {
  ensureOwned();
  if (!clusterRunning()) {
    console.log('PostgreSQL del proyecto ya está detenido.');
    return;
  }
  run(pg('pg_ctl'), ['-D', DATA, '-m', 'fast', '-w', 'stop']);
}

function dbInit() {
  if (!fs.existsSync(pg('initdb'))) {
    fail('Falta PostgreSQL 18 en .tools\\pgsql o en ADEL_PG_BIN; consulta DEVELOPMENT_WINDOWS.md.');
  }
  fs.mkdirSync(LOCAL, { recursive: true });
  if (fs.existsSync(DATA)) {
    ensureOwned();
    if (!fs.existsSync(path.join(DATA, 'PG_VERSION'))) {
      fail(
        'Inicialización anterior incompleta. No se borrará el directorio; revisa .local\\postgres.',
      );
    }
  } else {
    secretFile(path.join(LOCAL, 'admin-password'));
    writeJSON(path.join(LOCAL, 'cluster-owner.json'), { data_directory: DATA, port: PORT }, true);
    run(pg('initdb'), [
      '-D',
      DATA,
      '--username=adel_dev_admin',
      '--auth=scram-sha-256',
      '--encoding=UTF8',
      '--locale=C',
      '--pwfile',
      path.join(LOCAL, 'admin-password'),
    ]);
    // Igual que en macOS: solo TCP de loopback, UTC y SCRAM. En Windows no se usa socket de Unix.
    fs.appendFileSync(
      path.join(DATA, 'postgresql.conf'),
      `\nlisten_addresses = '127.0.0.1'\nport = ${PORT}\nunix_socket_directories = ''\n` +
        `timezone = 'UTC'\npassword_encryption = 'scram-sha-256'\n`,
    );
  }
  dbStart();
  const password = secretFile(path.join(LOCAL, 'app-password'));
  // Variables de psql citadas: los secretos no se interpolan en argumentos.
  const sql = [
    `\\set app_password '${password}'`,
    `\\set database_name '${DB_NAME}'`,
    "SELECT format('CREATE ROLE adel_app LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE', :'app_password') WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname='adel_app')\\gexec",
    "SELECT format('CREATE DATABASE %I OWNER adel_app', :'database_name') WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname=:'database_name')\\gexec",
    '',
  ].join('\n');
  run(pg('psql'), ['-X', '-q', '-v', 'ON_ERROR_STOP=1'], { input: sql, env: adminEnv() });
  const configFile = path.join(LOCAL, 'server.json');
  if (!fs.existsSync(configFile)) {
    const config = JSON.parse(
      fs.readFileSync(path.join(ROOT, 'config', 'server.development.example.json'), 'utf8'),
    );
    config.database.password = password;
    config.database.name = DB_NAME;
    writeJSON(configFile, config, true);
  }
  console.log(
    'Base de desarrollo preparada. Configuración privada en .local\\server.json; secretos no mostrados.',
  );
}

function databaseEnv(config) {
  const database = config.database;
  const env = {
    ...process.env,
    PGHOST: database.host,
    PGPORT: String(database.port),
    PGUSER: database.user,
    PGDATABASE: database.name,
    PGPASSWORD: process.env.ADEL_DB_PASSWORD ?? database.password,
    PGSSLMODE: database.ssl_mode,
    PGCONNECT_TIMEOUT: '3',
  };
  if (database.ssl_root_cert) {
    env.PGSSLROOTCERT = database.ssl_root_cert;
  }
  return env;
}

function timestamp() {
  return new Date()
    .toISOString()
    .replace(/[-:]/g, '')
    .replace(/\.\d{3}Z$/, 'Z');
}

function backup() {
  const config = JSON.parse(fs.readFileSync(SERVER_CONFIG, 'utf8'));
  const destination = path.join(ROOT, 'backups');
  fs.mkdirSync(destination, { recursive: true });
  const filename = path.join(
    destination,
    `${config.database.name}-${timestamp()}-${randomBytes(3).toString('hex')}.dump`,
  );
  try {
    // --file en lugar de redirigir la salida: el respaldo nunca pasa por la consola.
    run(pg('pg_dump'), ['--format=custom', '--no-owner', '--no-acl', '--file', filename], {
      env: databaseEnv(config),
    });
    run(pg('pg_restore'), ['--list', filename], { capture: true });
  } catch (error) {
    fs.rmSync(filename, { force: true });
    throw error;
  }
  console.log(
    `Respaldo generado y catálogo legible: ${path.basename(filename)}. Aún debe probarse su restauración.`,
  );
}

function tableCounts(database) {
  const env = adminEnv(database);
  const query = (sql) =>
    run(pg('psql'), ['-XAt', '-v', 'ON_ERROR_STOP=1', '-c', sql], {
      env,
      capture: true,
    }).stdout.trim();
  const tables = query("SELECT tablename FROM pg_tables WHERE schemaname='app' ORDER BY tablename")
    .split(/\r?\n/)
    .filter(Boolean);
  const counts = {};
  for (const table of tables) {
    counts[table] = Number(query(`SELECT count(*) FROM app."${table.replaceAll('"', '""')}"`));
  }
  return counts;
}

function restoreCheck(dumpArgument) {
  ensureOwned();
  if (!dumpArgument) {
    fail('Indica el respaldo: dev.ps1 restore-check backups\\NOMBRE.dump');
  }
  const dump = path.resolve(dumpArgument);
  if (
    path.dirname(dump) !== path.join(ROOT, 'backups') ||
    !fs.statSync(dump, { throwIfNoEntry: false })?.isFile()
  ) {
    fail('Selecciona un respaldo existente dentro de backups del proyecto');
  }
  const config = JSON.parse(fs.readFileSync(path.join(LOCAL, 'server.json'), 'utf8')).database;
  if (config.host !== '127.0.0.1' || config.port !== 55432) {
    fail('El ensayo está limitado al clúster exclusivo de desarrollo');
  }
  const name = 'adel_pos_test_restore_' + randomBytes(6).toString('hex');
  const before = tableCounts(config.name);
  run(pg('createdb'), ['--owner=adel_app', name], { env: adminEnv() });
  try {
    run(
      pg('pg_restore'),
      ['--exit-on-error', '--no-owner', '--no-acl', '--role=adel_app', '--dbname', name, dump],
      { env: adminEnv() },
    );
    const restored = tableCounts(name);
    if (JSON.stringify(before) !== JSON.stringify(restored)) {
      fail('Los conteos restaurados difieren; no continuar con la migración');
    }
    const report = {
      backup: path.basename(dump),
      restored_in_temporary_database: true,
      counts_equal: true,
      tables: restored,
    };
    writeJSON(
      path.join(LOCAL, `stage${PRODUCT.schema_version}-restore-verification.json`),
      report,
      false,
    );
    console.log(
      `Restauración completa verificada: ${Object.keys(restored).length} tablas con conteos idénticos.`,
    );
  } finally {
    run(pg('dropdb'), [name], { env: adminEnv() });
  }
}

function sync(extra = []) {
  run(process.execPath, [path.join(ROOT, 'scripts', 'sync-metadata.mjs'), ...extra]);
}

function buildServer() {
  sync();
  fs.mkdirSync(path.join(ROOT, 'bin'), { recursive: true });
  run('go', ['build', '-trimpath', '-o', SERVER_BINARY, './cmd/ad-el-pos-server'], {
    cwd: path.join(ROOT, 'backend'),
  });
}

function serverCommand(command, args) {
  buildServer();
  run(SERVER_BINARY, [command, '--config', SERVER_CONFIG, ...args]);
}

function httpStatus(url, timeout = 500) {
  return new Promise((resolve) => {
    const request = http.get(url, { timeout }, (response) => {
      response.resume();
      resolve(response.statusCode);
    });
    request.on('timeout', () => request.destroy());
    request.on('error', () => resolve(0));
  });
}

const sleep = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));

function processAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function registeredPid() {
  if (!fs.existsSync(PID_FILE)) {
    return null;
  }
  const pid = Number(fs.readFileSync(PID_FILE, 'utf8').trim());
  if (!Number.isInteger(pid) || pid <= 0 || !processAlive(pid)) {
    return null;
  }
  return pid;
}

// quoteArgument cita un argumento para la línea de comandos de Windows. Las rutas de Windows no pueden
// contener comillas dobles; cualquier otro argumento con ellas se rechaza.
function quoteArgument(value) {
  if (value.includes('"')) {
    fail('Argumento con comillas no admitido.');
  }
  return /[\s]/.test(value) ? `"${value}"` : value;
}

// startDetached lanza un programa oculto sin heredar identificadores de este proceso y devuelve su PID.
function startDetached(program, args) {
  const single = (value) => `'${value.replaceAll("'", "''")}'`;
  const script =
    `$p = Start-Process -FilePath ${single(program)} ` +
    `-ArgumentList ${single(args.map(quoteArgument).join(' '))} ` +
    `-WorkingDirectory ${single(ROOT)} -WindowStyle Hidden -PassThru; $p.Id`;
  const result = run('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], {
    capture: true,
  });
  const pid = Number(result.stdout.trim());
  if (!Number.isInteger(pid) || pid <= 0) {
    fail('No se pudo iniciar el proceso en segundo plano.');
  }
  return pid;
}

async function serverStart() {
  if (registeredPid()) {
    console.log('El servidor del proyecto ya está activo.');
    return;
  }
  buildServer();
  const config = JSON.parse(fs.readFileSync(SERVER_CONFIG, 'utf8'));
  if (config.tls_certificate) {
    fail('server-start de Windows solo verifica HTTP local; usa «server» para probar TLS.');
  }
  // Proceso independiente y oculto que sigue vivo al terminar este script, como en macOS. Se lanza con
  // Start-Process (ShellExecute) porque un hijo de Node hereda en Windows las tuberías de salida de Node
  // y la consola que lo ejecutó esperaría a que el servidor termine. La bitácora va con --log-file.
  const pid = startDetached(SERVER_BINARY, [
    'serve',
    '--config',
    SERVER_CONFIG,
    '--log-file',
    path.join(LOCAL, 'server.log'),
  ]);
  fs.writeFileSync(PID_FILE, String(pid));
  for (let attempt = 0; attempt < 60; attempt += 1) {
    if (!processAlive(pid)) {
      fs.rmSync(PID_FILE, { force: true });
      fail('Go no pudo iniciar. Revisa .local\\server.log.');
    }
    if ((await httpStatus(`http://${config.listen_address}/api/v1/health/live`)) === 200) {
      console.log(`Servidor independiente iniciado (PID ${pid}). Log: .local\\server.log`);
      return;
    }
    await sleep(150);
  }
  fail('El proceso sigue activo, pero no se pudo verificar HTTP. Revisa .local\\server.log.');
}

function serverStop() {
  const pid = registeredPid();
  if (pid === null) {
    fs.rmSync(PID_FILE, { force: true });
    console.log('No hay un servidor activo registrado por este script.');
    return;
  }
  // Go comprueba que el proceso sea este mismo binario antes de pedirle que se detenga.
  run(SERVER_BINARY, ['stop', '--pid', String(pid)]);
  fs.rmSync(PID_FILE, { force: true });
  console.log('Servidor detenido de forma ordenada.');
}

function hasC() {
  return run('where.exe', ['gcc'], { capture: true, allowFailure: true }).status === 0;
}

function goTestArguments() {
  // El detector de carreras de Go exige cgo y un compilador de C en Windows.
  if (hasC()) {
    return { args: ['-race'], env: { ...process.env, CGO_ENABLED: '1' } };
  }
  console.log('Aviso: sin compilador de C; las pruebas de Go corren sin -race en este equipo.');
  return { args: [], env: process.env };
}

function formatCheck() {
  run(process.execPath, [
    path.join(ROOT, 'desktop', 'node_modules', 'prettier', 'bin', 'prettier.cjs'),
    '.',
    '--check',
  ]);
  const unformatted = run('gofmt', ['-l', 'backend'], { capture: true }).stdout.trim();
  if (unformatted) {
    console.log(unformatted);
    fail('Hay archivos Go sin formato.');
  }
  run('cargo', ['fmt', '--check'], { cwd: path.join(ROOT, 'desktop', 'src-tauri') });
}

function test() {
  formatCheck();
  sync(['--check']);
  const go = goTestArguments();
  run('go', ['test', ...go.args, './...'], { cwd: path.join(ROOT, 'backend'), env: go.env });
  run('go', ['vet', './...'], { cwd: path.join(ROOT, 'backend') });
  npm(['test'], path.join(ROOT, 'desktop'));
  npm(['run', 'build'], path.join(ROOT, 'desktop'));
  run('cargo', ['test', '--locked'], { cwd: path.join(ROOT, 'desktop', 'src-tauri') });
}

function testIntegration() {
  ensureOwned();
  const password = fs.readFileSync(path.join(LOCAL, 'app-password'), 'utf8').trim();
  const selected = process.env.ADEL_TEST_PACKAGE;
  if (selected && !INTEGRATION_PACKAGES.includes(selected)) {
    fail('El paquete de integración solicitado no está permitido.');
  }
  const go = goTestArguments();
  for (const pkg of selected ? [selected] : INTEGRATION_PACKAGES) {
    const name = 'adel_pos_test_' + randomBytes(6).toString('hex');
    run(pg('createdb'), ['--owner=adel_app', name], { env: adminEnv() });
    try {
      const url = `postgresql://adel_app:${encodeURIComponent(password)}@127.0.0.1:${PORT}/${name}?sslmode=disable`;
      const runFilter = process.env.ADEL_TEST_RUN ? ['-run', process.env.ADEL_TEST_RUN] : [];
      run('go', ['test', ...go.args, '-tags=integration', pkg, '-count=1', '-v', ...runFilter], {
        cwd: path.join(ROOT, 'backend'),
        env: { ...go.env, ADEL_TEST_DATABASE_URL: url },
      });
    } finally {
      run(pg('dropdb'), [name], { env: adminEnv() });
    }
  }
}

async function runBrowser(engine) {
  ensureOwned();
  const name = 'adel_pos_test_browser_' + randomBytes(6).toString('hex');
  const directory = path.join(LOCAL, name);
  fs.mkdirSync(directory);
  const config = JSON.parse(fs.readFileSync(path.join(LOCAL, 'server.json'), 'utf8'));
  config.database.name = name;
  config.listen_address = '127.0.0.1:8183';
  config.allow_lan = false;
  // Etapa 9F: modo de pruebas (el servidor se niega a arrancar si la base no es adel_pos_test_* y lo
  // informa a native-driver.ts) y respaldos de «Quitar datos de prueba» dentro de la carpeta temporal.
  config.test_mode = true;
  config.backup_directory = path.join(directory, 'backups');
  const filename = path.join(directory, 'server.json');
  writeJSON(filename, config, true);
  let server = null;
  let created = false;
  try {
    run(pg('createdb'), ['--owner=adel_app', name], { env: adminEnv() });
    created = true;
    for (const command of ['migrate', 'prepare-setup']) {
      run(SERVER_BINARY, [command, '--config', filename]);
    }
    const log = fs.openSync(path.join(directory, 'server.log'), 'w');
    server = spawn(SERVER_BINARY, ['serve', '--config', filename], {
      cwd: ROOT,
      stdio: ['ignore', log, log],
      windowsHide: true,
    });
    let ready = false;
    for (let attempt = 0; attempt < 100 && !ready; attempt += 1) {
      ready = (await httpStatus('http://127.0.0.1:8183/api/v1/health/ready', 1000)) === 200;
      if (!ready) {
        await sleep(100);
      }
    }
    if (!ready) {
      fail('El servidor temporal no quedó listo');
    }
    // Los argumentos extra (por ejemplo, el nombre de una especificación) van antes de --project:
    // Playwright acepta varios valores después de --project y los tomaría como nombres de proyecto.
    const extra = process.argv.slice(3);
    run(process.execPath, [NPM_CLI, 'run', 'test:browser', '--', ...extra, '--project', engine], {
      cwd: path.join(ROOT, 'desktop'),
      env: {
        ...process.env,
        ADEL_BROWSER_ENGINE: engine,
        ADEL_BROWSER_PROVISIONING: path.join(directory, 'provisioning-token'),
        PLAYWRIGHT_BROWSERS_PATH: path.join(ROOT, '.cache', 'playwright'),
      },
    });
  } finally {
    if (server !== null && server.exitCode === null) {
      run(SERVER_BINARY, ['stop', '--pid', String(server.pid)], { allowFailure: true });
    }
    if (created) {
      run(pg('dropdb'), ['--if-exists', name], { env: adminEnv() });
    }
    fs.rmSync(directory, { recursive: true, force: true });
  }
}

async function testBrowser() {
  buildServer();
  const engines = process.env.ADEL_BROWSER_ENGINES
    ? process.env.ADEL_BROWSER_ENGINES.split(',')
    : ['chromium', 'webkit'];
  for (const engine of engines) {
    await runBrowser(engine);
  }
}

function desktopEnv() {
  const env = { ...process.env };
  for (const [file, variable] of [
    ['provisioning-token', 'ADEL_PROVISIONING_FILE'],
    ['enrollment-code', 'ADEL_ENROLLMENT_FILE'],
  ]) {
    const filename = path.join(LOCAL, file);
    if (fs.existsSync(filename)) {
      env[variable] = filename;
    }
  }
  return env;
}

async function main() {
  const [command = 'help', ...args] = process.argv.slice(2);
  switch (command) {
    case 'sync':
      sync(args);
      break;
    case 'install':
      npm(['ci'], path.join(ROOT, 'desktop'));
      sync();
      run('go', ['mod', 'download'], { cwd: path.join(ROOT, 'backend') });
      break;
    case 'format':
      run(process.execPath, [
        path.join(ROOT, 'desktop', 'node_modules', 'prettier', 'bin', 'prettier.cjs'),
        '.',
        '--write',
      ]);
      run('gofmt', ['-w', 'backend']);
      run('cargo', ['fmt'], { cwd: path.join(ROOT, 'desktop', 'src-tauri') });
      break;
    case 'format-check':
      formatCheck();
      break;
    case 'db-init':
      dbInit();
      break;
    case 'db-start':
      dbStart();
      break;
    case 'db-stop':
      dbStop();
      break;
    case 'backup':
      backup();
      break;
    case 'restore-check':
      restoreCheck(args[0]);
      break;
    case 'build-server':
      buildServer();
      break;
    case 'migrate':
    case 'check-config':
    case 'prepare-setup':
    case 'provision-sa':
    case 'reset-sa':
    case 'tls-init':
    case 'create-enrollment':
      serverCommand(command, args);
      break;
    case 'server':
      serverCommand('serve', args);
      break;
    case 'server-start':
      await serverStart();
      break;
    case 'server-stop':
      serverStop();
      break;
    case 'status':
      console.log(`Servidor: ${registeredPid() ?? 'detenido'}`);
      break;
    case 'desktop':
      sync();
      run(process.execPath, [NPM_CLI, 'run', 'tauri', '--', 'dev'], {
        cwd: path.join(ROOT, 'desktop'),
        env: desktopEnv(),
      });
      break;
    case 'open-app': {
      const exe = path.join(
        ROOT,
        'desktop',
        'src-tauri',
        'target',
        'release',
        'ad-el-pos-desktop.exe',
      );
      spawn(exe, [], { cwd: ROOT, env: desktopEnv(), detached: true, stdio: 'ignore' }).unref();
      break;
    }
    case 'test':
      test();
      break;
    case 'test-integration':
      testIntegration();
      break;
    case 'test-browser':
      await testBrowser();
      break;
    case 'npx':
      run(process.execPath, [NPX_CLI, ...args], { cwd: path.join(ROOT, 'desktop') });
      break;
    default:
      console.log('Consulta DEVELOPMENT_WINDOWS.md para los comandos de desarrollo en Windows.');
  }
}

main().catch((error) => {
  console.error(error instanceof CommandError ? error.message : error);
  process.exit(1);
});
