# Entorno de desarrollo en Windows (etapa 9E). Equivale a scripts/env.sh de macOS: antepone al PATH las
# herramientas fijadas que viven dentro del proyecto (.tools, excluida de git) y dirige las cachés a .cache.
# Se carga con «. scripts\env.ps1» o lo carga scripts\dev.ps1; no cambia variables del usuario ni del equipo.
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Tools = Join-Path $ProjectRoot '.tools'

$env:PROJECT_ROOT = $ProjectRoot
$env:RUSTUP_HOME = Join-Path $Tools 'rustup'
$env:CARGO_HOME = Join-Path $Tools 'cargo'
$env:GOCACHE = Join-Path $ProjectRoot '.cache\go-build'
$env:GOMODCACHE = Join-Path $ProjectRoot '.cache\go-mod'
$env:npm_config_cache = Join-Path $ProjectRoot '.cache\npm'
$env:PLAYWRIGHT_BROWSERS_PATH = Join-Path $ProjectRoot '.cache\playwright'
$env:GOTOOLCHAIN = 'local'

# Binarios oficiales de PostgreSQL 18 para Windows (zip de EDB); ADEL_PG_BIN permite otra ruta.
if (-not $env:ADEL_PG_BIN) {
    $env:ADEL_PG_BIN = Join-Path $Tools 'pgsql\bin'
}

$ToolPaths = @(
    (Join-Path $Tools 'node'),
    (Join-Path $Tools 'go\bin'),
    (Join-Path $Tools 'cargo\bin'),
    $env:ADEL_PG_BIN
)
# Compilador de C opcional para «go test -race» (cgo); solo se usa si existe.
$MingwBin = Join-Path $Tools 'mingw64\bin'
if (Test-Path $MingwBin) {
    $ToolPaths += $MingwBin
}
$env:PATH = (($ToolPaths | Where-Object { Test-Path $_ }) -join ';') + ';' + $env:PATH
