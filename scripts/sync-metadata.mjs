import { readdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(new URL('../desktop/package.json', import.meta.url));
const prettier = require('prettier');

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = (name) => readFile(path.join(root, name), 'utf8');
const product = JSON.parse(await read('config/product.json'));
const check = process.argv.includes('--check');
const json = (value) => `${JSON.stringify(value, null, 2)}\n`;
async function output(name, value) {
  // Los archivos generados respetan el mismo formato que el código mantenido a mano.
  if (name.endsWith('.json') || name.endsWith('.html')) {
    const filepath = path.join(root, name);
    const options = await prettier.resolveConfig(filepath);
    value = await prettier.format(value, { ...options, filepath });
  }
  if (check) {
    if ((await read(name)) !== value)
      throw new Error(`Metadatos desactualizados: ${name}. Ejecuta ./scripts/dev.sh sync`);
  } else await writeFile(path.join(root, name), value);
}
for (const name of ['backend/internal/product/metadata.json', 'desktop/src/generated/product.json'])
  await output(name, json(product));
// Etapa L1 (licencias): las llaves públicas de confianza de cada entorno (config/licencias/llaves-<entorno>.json)
// se embeben en el servidor Go con go:embed, que no puede leer fuera de backend/. config/licencias/ es la
// fuente; backend/internal/licensing/ guarda la copia. Procedimiento: docs/licencias/ROTAR_LLAVES.md.
const keyFiles = (await readdir(path.join(root, 'config/licencias')))
  .filter((name) => /^llaves-[a-z]+\.json$/.test(name))
  .sort();
for (const name of keyFiles) {
  const keys = JSON.parse(await read(`config/licencias/${name}`));
  await output(`backend/internal/licensing/${name}`, json(keys));
}
await output(
  'desktop/src/generated/connection.json',
  json(JSON.parse(await read('config/terminal.development.json'))),
);
const tauri = JSON.parse(await read('desktop/src-tauri/tauri.conf.json'));
tauri.productName = product.name;
tauri.version = product.version;
tauri.identifier = product.bundle_id;
tauri.app.windows[0].title = product.name;
await output('desktop/src-tauri/tauri.conf.json', json(tauri));
const pkg = JSON.parse(await read('desktop/package.json'));
pkg.name = `${product.technical_id}-desktop`;
pkg.version = product.version;
await output('desktop/package.json', json(pkg));
const cargo = (await read('desktop/src-tauri/Cargo.toml'))
  .replace(/^name = ".*"/m, `name = "${product.technical_id}-desktop"`)
  .replace(/^version = ".*"/m, `version = "${product.version}"`);
await output('desktop/src-tauri/Cargo.toml', cargo);
await output(
  'desktop/index.html',
  (await read('desktop/index.html')).replace(
    /<title>.*<\/title>/,
    `<title>${product.name}</title>`,
  ),
);
console.log(check ? 'Metadatos coherentes.' : 'Metadatos sincronizados desde config/product.json.');
