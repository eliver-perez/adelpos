#!/usr/bin/env bash
# Comprobación de datos reales del sistema anterior (etapa 9H, «Datos reales»): falla si el índice de git tiene un
# respaldo de SQL Server o un paquete de migración, por su nombre (*.bak y sus versiones comprimidas .bak.gz, .bak.zip y
# .bak.7z, los archivos de datos y de registro *.mdf, *.ndf y *.ldf, los respaldos del registro *.trn, *.adelmig y la
# carpeta legacy-data/) o por su contenido (un archivo binario que empiece con la firma de un paquete, «ADELMIG» y un
# NUL, o con la de un respaldo de SQL Server, el bloque «TAPE» del formato MTF, también dentro de un gzip), aunque se le
# haya cambiado el nombre. La ejecutan
# `scripts/dev.sh format-check` y `scripts/dev.sh test`; en Windows, `scripts/windows/dev.mjs` (checkLegacyData).
#
#   scripts/check-legacy-data.sh [raíz del repositorio]
#
# Sin .git (una copia hecha con git archive) no hay índice que revisar: lo dice y termina bien.
set -euo pipefail
ROOT="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"

if [ ! -e "$ROOT/.git" ]; then
    echo "Datos del sistema anterior: sin .git en $ROOT (copia sin índice); no hay nada que comprobar."
    exit 0
fi

found=()

# Por nombre: lo que .gitignore excluye y alguien agregó a la fuerza (sin distinguir mayúsculas).
shopt -s nocasematch
while IFS= read -r -d '' path; do
    case "$path" in
        *.bak | *.bak.gz | *.bak.zip | *.bak.7z | *.mdf | *.ndf | *.ldf | *.trn | *.adelmig | legacy-data/* | */legacy-data/*)
            found+=("$path (nombre)")
            ;;
    esac
done < <(git -C "$ROOT" ls-files -z --cached)
shopt -u nocasematch

# Por contenido: archivos del índice cuya primera línea empieza con una de las firmas (git grep -o -n -z entrega
# «ruta NUL línea NUL firma») y que git considera binarios (no aparecen con -I, que omite los binarios).
text_matches="$(git -C "$ROOT" grep --cached -I -l -z -E -e '^TAPE' -e '^ADELMIG' | tr '\0' '\n' || true)"
while IFS= read -r -d '' path && IFS= read -r -d '' line && IFS= read -r signature; do
    if [ "$line" != "1" ]; then
        continue
    fi
    if printf '%s\n' "$text_matches" | grep -Fxq -- "$path"; then
        continue
    fi
    if [ "$signature" = "ADELMIG" ]; then
        found+=("$path (paquete de migración)")
    else
        found+=("$path (respaldo de SQL Server)")
    fi
done < <(git -C "$ROOT" -c core.quotePath=false grep --cached -a -n -o -z -E -e '^TAPE' -e '^ADELMIG' || true)

# Respaldos comprimidos con gzip con otro nombre: archivos cuya primera línea empieza con la firma de gzip (1f 8b) y
# cuyo contenido, al descomprimir sus primeros bytes, empieza con «TAPE». Solo se descomprime el principio.
while IFS= read -r -d '' path && IFS= read -r -d '' line && IFS= read -r _signature; do
    if [ "$line" != "1" ]; then
        continue
    fi
    start="$( (git -C "$ROOT" cat-file blob ":$path" | gzip -dc 2>/dev/null | head -c 4) 2>/dev/null || true)"
    if [ "$start" = "TAPE" ]; then
        found+=("$path (respaldo de SQL Server comprimido)")
    fi
done < <(LC_ALL=C git -C "$ROOT" -c core.quotePath=false grep --cached -a -n -o -z -E -e $'^\x1f\x8b' || true)

if [ "${#found[@]}" -gt 0 ]; then
    echo "El índice de git tiene datos del sistema anterior (nunca deben versionarse; quítalos con git rm --cached):" >&2
    for item in "${found[@]}"; do
        echo "  $item" >&2
    done
    exit 1
fi
echo "Datos del sistema anterior: el índice de git no tiene respaldos ni paquetes de migración."
