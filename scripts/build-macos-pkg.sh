#!/usr/bin/env bash
# Instalador .pkg de AD-EL POS para macOS (etapa 9G, fase A; decisión D3), de principio a fin:
#
#   scripts/dev.sh build-macos-pkg [--reuse-app]
#
# 1. Sincroniza los metadatos de config/product.json.
# 2. Compila el servidor Go como binario externo de Tauri (Contents/MacOS/ad-el-pos-server dentro de la
#    aplicación) y la aplicación con desktop/src-tauri/tauri.macos-pkg.conf.json. Con --reuse-app usa la
#    aplicación ya compilada si trae el servidor (para repetir solo el empaquetado).
# 3. Arma tres paquetes con pkgbuild: la aplicación (no reubicable, a /Applications, con un preinstall
#    que detiene el servicio anterior), «Servidor del restaurante» (sin contenido; crea la carpeta de datos
#    y el archivo de habilitación) y el servicio (sin contenido; escribe el LaunchDaemon y lo registra).
# 4. Los une con productbuild y la distribución de scripts/macos-pkg/Distribution.xml (opción «Servidor
#    del restaurante» marcada por omisión) y muestra la ruta, el tamaño y el SHA-256.
#
# No instala nada ni toca datos: el resultado queda en <destino de Cargo>/release/bundle/macos-pkg. Sin
# firma de desarrollador (el .pkg y la aplicación quedan con firma ad hoc): si el .pkg se descarga de
# Internet, macOS pide abrirlo con clic secundario › Abrir.
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PROJECT_ROOT
# shellcheck source=env.sh
source "$PROJECT_ROOT/scripts/env.sh"
cd "$PROJECT_ROOT"

reuse_app=0
for argument in "$@"; do
    case "$argument" in
        --reuse-app) reuse_app=1 ;;
        *)
            echo "Opción desconocida: $argument" >&2
            exit 2
            ;;
    esac
done

step() {
    echo
    echo "== $*"
}

for tool in pkgbuild productbuild plutil codesign shasum; do
    command -v "$tool" >/dev/null || {
        echo "Falta $tool (Command Line Tools de Xcode)." >&2
        exit 1
    }
done

step "Metadatos"
node scripts/sync-metadata.mjs
version="$(plutil -extract version raw -o - config/product.json)"
name="$(plutil -extract name raw -o - config/product.json)"
server_binary="$(plutil -extract server_binary raw -o - config/product.json)"
target_dir="${CARGO_TARGET_DIR:-$PROJECT_ROOT/desktop/src-tauri/target}"
app="$target_dir/release/bundle/macos/$name.app"
output_dir="$target_dir/release/bundle/macos-pkg"
stage="$target_dir/macos-pkg-stage"

if [ "$reuse_app" -eq 1 ] && [ -x "$app/Contents/MacOS/$server_binary" ]; then
    step "Aplicación existente $app"
else
    step "Servidor Go $version"
    triple="$(rustc -vV | sed -n 's/^host: //p')"
    mkdir -p desktop/src-tauri/binaries
    (cd backend && go build -trimpath -o "../desktop/src-tauri/binaries/$server_binary-$triple" ./cmd/ad-el-pos-server)
    step "Aplicación $version con el servidor dentro"
    (cd desktop && npm run tauri -- build --bundles app --config src-tauri/tauri.macos-pkg.conf.json)
fi
if [ ! -x "$app/Contents/MacOS/$server_binary" ]; then
    echo "La aplicación no trae el servidor en Contents/MacOS/$server_binary." >&2
    exit 1
fi
codesign --verify --deep --strict "$app"
"$app/Contents/MacOS/$server_binary" version

step "Paquetes"
rm -rf "$stage"
mkdir -p "$stage/root/Applications" "$stage/packages" "$stage/resources"
for package in app server service; do
    mkdir -p "$stage/scripts-$package"
    cp scripts/macos-pkg/common.sh "$stage/scripts-$package/common.sh"
done
cp scripts/macos-pkg/app/preinstall "$stage/scripts-app/preinstall"
cp scripts/macos-pkg/server/preinstall scripts/macos-pkg/server/postinstall "$stage/scripts-server/"
cp scripts/macos-pkg/service/postinstall "$stage/scripts-service/postinstall"
cp ops/launchd/local.adel.pos.server.plist "$stage/scripts-service/local.adel.pos.server.plist"
chmod 0755 "$stage"/scripts-*/*install
for script in "$stage"/scripts-*/*install "$stage"/scripts-*/common.sh; do
    bash -n "$script"
done
plutil -lint "$stage/scripts-service/local.adel.pos.server.plist" >/dev/null
ditto "$app" "$stage/root/Applications/$name.app"

# La aplicación no se reubica: si existe otra copia con el mismo identificador en otra carpeta, el
# Instalador la actualizaría allí y el servicio buscaría el servidor en /Applications.
pkgbuild --analyze --root "$stage/root" "$stage/app-component.plist" >/dev/null
count="$(plutil -extract . raw -o - "$stage/app-component.plist" 2>/dev/null || echo 1)"
index=0
while [ "$index" -lt "$count" ]; do
    plutil -replace "$index.BundleIsRelocatable" -bool NO "$stage/app-component.plist"
    plutil -replace "$index.BundleOverwriteAction" -string upgrade "$stage/app-component.plist"
    index=$((index + 1))
done
pkgbuild --root "$stage/root" --component-plist "$stage/app-component.plist" --scripts "$stage/scripts-app" \
    --identifier local.adel.pos.app --version "$version" --install-location / "$stage/packages/app.pkg"
pkgbuild --nopayload --scripts "$stage/scripts-server" --identifier local.adel.pos.server \
    --version "$version" "$stage/packages/server.pkg"
pkgbuild --nopayload --scripts "$stage/scripts-service" --identifier local.adel.pos.service \
    --version "$version" "$stage/packages/service.pkg"

step "Instalador"
sed "s/__VERSION__/$version/g" scripts/macos-pkg/Distribution.xml >"$stage/Distribution.xml"
cp scripts/macos-pkg/welcome.html "$stage/resources/welcome.html"
mkdir -p "$output_dir"
result="$output_dir/${name}_${version}.pkg"
rm -f "$result"
productbuild --distribution "$stage/Distribution.xml" --package-path "$stage/packages" \
    --resources "$stage/resources" "$result"
size="$(du -h "$result" | cut -f1)"
digest="$(shasum -a 256 "$result" | cut -d' ' -f1)"
echo
echo "Instalador: $result"
echo "Tamaño: $size"
echo "SHA-256: $digest"
