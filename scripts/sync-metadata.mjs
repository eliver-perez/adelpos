import { readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = name => readFile(path.join(root, name), 'utf8');
const product = JSON.parse(await read('config/product.json'));
const check = process.argv.includes('--check');
const json = value => `${JSON.stringify(value, null, 2)}\n`;
async function output(name, value) {
  if (check) {
    if (await read(name) !== value) throw new Error(`Metadatos desactualizados: ${name}. Ejecuta ./scripts/dev.sh sync`);
  } else await writeFile(path.join(root, name), value);
}
for (const name of ['backend/internal/product/metadata.json', 'desktop/src/generated/product.json']) await output(name, json(product));
await output('desktop/src/generated/connection.json', json(JSON.parse(await read('config/terminal.development.json'))));
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
await output('desktop/index.html', (await read('desktop/index.html')).replace(/<title>.*<\/title>/, `<title>${product.name}</title>`));
console.log(check ? 'Metadatos coherentes.' : 'Metadatos sincronizados desde config/product.json.');
