# Comandos de desarrollo en Windows (etapa 9E): equivalente de scripts/dev.sh, que sigue siendo el de macOS.
# Uso: powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 <comando> [argumentos]
# La lógica vive en scripts/windows/dev.mjs (Node fijado del proyecto), que maneja rutas con espacios y
# procesos sin depender de comillas de PowerShell 5.1. Consulta DEVELOPMENT_WINDOWS.md.
param(
    [Parameter(Position = 0)]
    [string]$Command = 'help',
    [Parameter(Position = 1, ValueFromRemainingArguments = $true)]
    [string[]]$Arguments = @()
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'env.ps1')

$Node = Join-Path $env:PROJECT_ROOT '.tools\node\node.exe'
if (-not (Test-Path $Node)) {
    Write-Error 'Falta Node en .tools\node. Consulta DEVELOPMENT_WINDOWS.md («Herramientas»).'
    exit 1
}
if ($Command -eq 'build-windows') {
    & (Join-Path $PSScriptRoot 'build-windows.ps1') @Arguments
    exit $LASTEXITCODE
}
& $Node (Join-Path $PSScriptRoot 'windows\dev.mjs') $Command @Arguments
exit $LASTEXITCODE
