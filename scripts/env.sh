#!/usr/bin/env bash
# Cargar desde dev.sh, que define PROJECT_ROOT antes de incluir este archivo.
export PATH="$PROJECT_ROOT/.tools/node/bin:$PROJECT_ROOT/.tools/go/bin:$PROJECT_ROOT/.tools/cargo/bin:$PATH"
if [ -d "$PROJECT_ROOT/.tools/rustup" ]; then
  export RUSTUP_HOME="$PROJECT_ROOT/.tools/rustup"
  export CARGO_HOME="$PROJECT_ROOT/.tools/cargo"
fi
export GOCACHE="$PROJECT_ROOT/.cache/go-build"
export GOMODCACHE="$PROJECT_ROOT/.cache/go-mod"
export npm_config_cache="$PROJECT_ROOT/.cache/npm"
export GOTOOLCHAIN=local
