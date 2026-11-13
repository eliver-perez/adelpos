# Prueba de humo de AD-EL POS INSTALADO (etapa 9E, §5.2). Abre la aplicación instalada con el puerto de
# depuración de WebView2 solo en esta ejecución (127.0.0.1:9222; la aplicación normal no lo abre) y corre
# desktop/e2e-installed con Playwright conectado a ese WebView2 real.
#
#   powershell -ExecutionPolicy Bypass -File scripts\windows\installed-smoke.ps1 [-Grep "texto del paso"]
param(
    [string]$Grep = '',
    [string]$GrepInvert = '',
    [int]$Port = 9222
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\env.ps1')
$AppExe = Join-Path $env:LOCALAPPDATA 'Programs\AD-EL POS\ad-el-pos-desktop.exe'
if (-not (Test-Path $AppExe)) {
    throw 'AD-EL POS no está instalado para este usuario.'
}
$running = Get-Process ad-el-pos-desktop -ErrorAction SilentlyContinue
if (-not $running) {
    # Variable solo para el proceso que se abre aquí: WebView2 la lee al iniciar.
    $env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = "--remote-debugging-port=$Port"
    Start-Process -FilePath $AppExe | Out-Null
    Remove-Item Env:\WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS
}
$deadline = (Get-Date).AddSeconds(60)
$ready = $false
while (-not $ready -and (Get-Date) -lt $deadline) {
    try {
        $ready = (Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$Port/json/version" -TimeoutSec 2).StatusCode -eq 200
    } catch {
        Start-Sleep -Milliseconds 500
    }
}
if (-not $ready) {
    throw 'El WebView2 de la aplicación no abrió el puerto de depuración (¿ya estaba abierta sin él?).'
}
$env:ADEL_INSTALLED_CDP = "http://127.0.0.1:$Port"
$arguments = @((Join-Path $env:PROJECT_ROOT 'desktop\node_modules\@playwright\test\cli.js'), 'test', '-c', 'playwright.installed.config.ts', '--reporter=list')
if ($Grep) {
    $arguments += @('--grep', $Grep)
}
if ($GrepInvert) {
    $arguments += @('--grep-invert', $GrepInvert)
}
Push-Location (Join-Path $env:PROJECT_ROOT 'desktop')
try {
    & node @arguments
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
