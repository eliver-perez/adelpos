# Prueba de humo de AD-EL POS INSTALADO (etapa 9E, §5.2; desde la 9F, fuera de los datos reales). Abre la
# aplicación instalada con el puerto de depuración de WebView2 solo en esta ejecución (127.0.0.1:9222; la
# aplicación normal no lo abre) y corre desktop/e2e-installed con Playwright conectado a ese WebView2 real.
#
# Etapa 9F (decisión D1): las pruebas nunca escriben en la instalación real. La aplicación se abre con:
#   - LOCALAPPDATA en una carpeta temporal: el servidor crea ahí su carpeta de datos (clúster, configuración,
#     secretos y respaldos), nunca en %LOCALAPPDATA%\AD-EL POS;
#   - ADEL_TERMINAL_CONFIG con otro perfil de credenciales (credential_namespace): con el perfil «main», el
#     servidor nuevo rechazaría la terminal y la aplicación borraría la credencial real;
#   - ADEL_TEST_MODE=1: el servidor local (host) se niega a usar la carpeta de datos real del usuario.
# La aplicación real debe estar cerrada (comparten los puertos 8181 y 55432). Al terminar, pase lo que pase,
# se cierra la aplicación de prueba, se borra la carpeta temporal y la credencial de prueba (cmdkey /delete).
# El perfil de WebView2 y la geometría de la ventana siguen en las carpetas reales del usuario (Windows las
# resuelve por su API de carpetas conocidas, no por la variable).
#
#   powershell -ExecutionPolicy Bypass -File scripts\windows\installed-smoke.ps1 [-Grep "texto del paso"]
param(
    [string]$Grep = '',
    [string]$GrepInvert = '',
    [int]$Port = 9222
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\env.ps1')
$Product = 'AD-EL POS'
$InstallDir = Join-Path $env:LOCALAPPDATA "Programs\$Product"
$AppExe = Join-Path $InstallDir 'ad-el-pos-desktop.exe'
$RealDataDir = Join-Path $env:LOCALAPPDATA $Product
if (-not (Test-Path $AppExe)) {
    throw 'AD-EL POS no está instalado para este usuario.'
}

# La aplicación real y la de prueba usan los mismos puertos: la real debe estar cerrada.
$running = @(Get-Process ad-el-pos-desktop, ad-el-pos-server -ErrorAction SilentlyContinue)
if ($running.Count -gt 0) {
    throw 'Cierra AD-EL POS antes de la prueba de humo: la aplicación de prueba usa los mismos puertos (8181 y 55432).'
}
foreach ($busy in @(8181, 55432, $Port)) {
    if (Get-NetTCPConnection -LocalPort $busy -State Listen -ErrorAction SilentlyContinue) {
        throw "El puerto $busy está en uso: cierra AD-EL POS y el clúster de desarrollo antes de la prueba de humo."
    }
}
$realClusterRunning = Test-Path (Join-Path $RealDataDir 'postgres\postmaster.pid')

# Carpeta temporal propia de esta ejecución y perfil de credenciales de prueba.
$TestRoot = Join-Path ([IO.Path]::GetTempPath()) ('adel-pos-humo-' + [guid]::NewGuid().ToString('N'))
$TestLocalAppData = Join-Path $TestRoot 'LocalAppData'
$TestDataDir = Join-Path $TestLocalAppData $Product
$Namespace = 'humo-' + [guid]::NewGuid().ToString('N').Substring(0, 12)
$ProductMetadata = Get-Content -Raw -Encoding UTF8 (Join-Path $env:PROJECT_ROOT 'config\product.json') | ConvertFrom-Json
$CredentialTarget = "http://127.0.0.1:8181|$Namespace.$($ProductMetadata.bundle_id)"
New-Item -ItemType Directory -Force -Path $TestLocalAppData | Out-Null
$TerminalConfig = Join-Path $TestRoot 'connection.json'
$terminal = [ordered]@{
    installation_mode    = 'server-terminal'
    server_url           = 'http://127.0.0.1:8181'
    credential_namespace = $Namespace
}
# Sin BOM: la aplicación lee el archivo como JSON estricto.
[IO.File]::WriteAllText($TerminalConfig, ($terminal | ConvertTo-Json), (New-Object Text.UTF8Encoding $false))

function TestProcesses {
    # Procesos de la aplicación de prueba: los del programa instalado y el PostgreSQL de la carpeta temporal.
    Get-CimInstance Win32_Process | Where-Object {
        ($_.ExecutablePath -and $_.ExecutablePath.StartsWith($InstallDir, [StringComparison]::OrdinalIgnoreCase)) -or
        ($_.CommandLine -and $_.CommandLine.IndexOf($TestRoot, [StringComparison]::OrdinalIgnoreCase) -ge 0)
    }
}

# Limpieza final (revisión independiente de la 9F, B5): cada paso en su propio try/catch. Con
# $ErrorActionPreference = 'Stop', un fallo (por ejemplo, de Get-CimInstance) interrumpiría los pasos
# siguientes y quedarían la carpeta temporal (clúster y secretos de la prueba) y la credencial de prueba.
# Lo que no se pudo limpiar se anota aquí, se informa al final y la prueba termina con error.
$CleanupFailures = New-Object System.Collections.Generic.List[string]
function CleanupStep([string]$Description, [scriptblock]$Step) {
    try {
        & $Step | Out-Null
    } catch {
        $CleanupFailures.Add("$Description ($($_.Exception.Message))")
    }
}

$app = $null
$exitCode = 1
$saved = @{}
foreach ($name in @('LOCALAPPDATA', 'ADEL_TERMINAL_CONFIG', 'ADEL_TEST_MODE', 'WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS')) {
    $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
    # Variables solo para el proceso que se abre aquí (la aplicación y el servidor que lanza las heredan).
    $env:LOCALAPPDATA = $TestLocalAppData
    $env:ADEL_TERMINAL_CONFIG = $TerminalConfig
    $env:ADEL_TEST_MODE = '1'
    $env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = "--remote-debugging-port=$Port"
    $app = Start-Process -FilePath $AppExe -PassThru
    foreach ($name in $saved.Keys) {
        [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process')
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
        throw 'El WebView2 de la aplicación no abrió el puerto de depuración.'
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
        $exitCode = $LASTEXITCODE
    } finally {
        Pop-Location
    }
} finally {
    CleanupStep 'restaurar las variables de entorno de esta consola' {
        foreach ($name in $saved.Keys) {
            [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process')
        }
        Remove-Item Env:\ADEL_INSTALLED_CDP -ErrorAction SilentlyContinue
    }
    # Cierre ordenado (como el botón cerrar): la aplicación cierra la entrada de Go y Go detiene PostgreSQL.
    CleanupStep 'pedir el cierre de la aplicación de prueba' {
        if ($app -and -not $app.HasExited) {
            $null = $app.CloseMainWindow()
        }
    }
    # Espera a la aplicación abierta aquí aunque después no se pueda consultar la lista de procesos.
    CleanupStep 'esperar el cierre de la aplicación de prueba' {
        if ($app -and -not $app.HasExited) {
            $null = $app.WaitForExit(60000)
        }
    }
    CleanupStep 'cerrar los procesos de la prueba (servidor y PostgreSQL de la carpeta temporal)' {
        $watch = [Diagnostics.Stopwatch]::StartNew()
        while ((TestProcesses) -and $watch.Elapsed.TotalSeconds -lt 60) {
            Start-Sleep -Milliseconds 500
        }
        $left = @(TestProcesses)
        if ($left.Count -gt 0) {
            Write-Warning "Quedaron $($left.Count) procesos de la prueba; se detienen."
            $left | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
            Start-Sleep -Seconds 2
        }
    }
    # Carpeta temporal (clúster, configuración, respaldos y bitácoras de la prueba).
    CleanupStep "borrar la carpeta temporal $TestRoot (bórrala a mano)" {
        for ($attempt = 0; $attempt -lt 5 -and (Test-Path $TestRoot); $attempt++) {
            Remove-Item -Recurse -Force $TestRoot -ErrorAction SilentlyContinue
            if (Test-Path $TestRoot) {
                Start-Sleep -Seconds 2
            }
        }
        if (Test-Path $TestRoot) {
            throw 'la carpeta sigue ahí'
        }
    }
    # Credencial de la terminal de prueba (la real, con el perfil «main», no se toca). cmdkey termina con
    # error si la credencial no existe (la prueba falló antes de configurar la terminal): solo se avisa.
    CleanupStep "borrar la credencial de prueba $CredentialTarget (cmdkey /delete)" {
        & cmdkey "/delete:$CredentialTarget" | Out-Null
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "cmdkey no borró la credencial de prueba $CredentialTarget (código $LASTEXITCODE); si existe, bórrala a mano."
        }
    }
    try {
        if (-not $realClusterRunning -and (Test-Path (Join-Path $RealDataDir 'postgres\postmaster.pid'))) {
            Write-Warning 'El clúster de la instalación real quedó en marcha: revisa que nada haya usado los datos reales.'
            $exitCode = 1
        }
    } catch {
        $CleanupFailures.Add("comprobar el clúster de la instalación real ($($_.Exception.Message))")
    }
    foreach ($failure in $CleanupFailures) {
        Write-Warning "No se pudo ${failure}."
    }
    if ($CleanupFailures.Count -gt 0) {
        $exitCode = 1
    }
}
exit $exitCode
