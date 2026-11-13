# Verificación automática de AD-EL POS instalado en Windows (etapa 9E, §5). No usa el modo desarrollo: trabaja
# con el instalador NSIS y con la aplicación que queda instalada para el usuario actual.
#
#   powershell -ExecutionPolicy Bypass -File scripts\windows\verify-installed.ps1 -Installer <setup.exe> [-Uninstall]
#
# Pasos: instala en silencio (/S); abre la aplicación y mide cuánto tarda la ventana y cuánto el servidor
# local en responder; comprueba que no apareció ninguna ventana de consola; suma la memoria de la
# aplicación, WebView2, el servidor y PostgreSQL; cierra la aplicación en orden (como el botón cerrar) y
# comprueba que no queden procesos del programa. Con -Uninstall también desinstala en silencio y comprueba
# que la carpeta de datos sigue ahí. Escribe un informe JSON en -Report.
#
# Nunca borra la carpeta de datos (%LOCALAPPDATA%\AD-EL POS) ni toca otros programas.
param(
    [string]$Installer = '',
    [switch]$SkipInstall,
    [switch]$Uninstall,
    [switch]$KeepOpen,
    [int]$ReadyTimeoutSeconds = 180,
    [string]$Report = ''
)
$ErrorActionPreference = 'Stop'
$Product = 'AD-EL POS'
$InstallDir = Join-Path $env:LOCALAPPDATA "Programs\$Product"
$DataDir = Join-Path $env:LOCALAPPDATA $Product
$AppExe = Join-Path $InstallDir 'ad-el-pos-desktop.exe'
$OwnNames = @('ad-el-pos-desktop', 'ad-el-pos-server', 'postgres', 'pg_ctl')
$Result = [ordered]@{ started = (Get-Date).ToString('s') }

Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;
public static class AdelWindows {
    delegate bool EnumProc(IntPtr hwnd, IntPtr lParam);
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc callback, IntPtr lParam);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr hwnd);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetClassName(IntPtr hwnd, StringBuilder name, int size);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr hwnd, StringBuilder text, int size);
    public static List<string> Visible() {
        var result = new List<string>();
        EnumWindows((hwnd, l) => {
            if (!IsWindowVisible(hwnd)) { return true; }
            uint pid; GetWindowThreadProcessId(hwnd, out pid);
            var cls = new StringBuilder(256); GetClassName(hwnd, cls, 256);
            var title = new StringBuilder(256); GetWindowText(hwnd, title, 256);
            result.Add(hwnd.ToInt64() + "|" + pid + "|" + cls + "|" + title);
            return true;
        }, IntPtr.Zero);
        return result;
    }
}
'@

function Step($text) {
    Write-Host ''
    Write-Host "== $text" -ForegroundColor Cyan
}

function OwnProcesses {
    Get-Process -ErrorAction SilentlyContinue | Where-Object {
        $OwnNames -contains $_.ProcessName -and $_.Path -and $_.Path.StartsWith($InstallDir, [StringComparison]::OrdinalIgnoreCase)
    }
}

function WebViewProcesses($appPid) {
    # WebView2 cuelga de la aplicación: su proceso principal es hijo directo y los demás son hijos de ese.
    $all = Get-CimInstance Win32_Process -Filter "Name = 'msedgewebview2.exe'"
    $roots = $all | Where-Object { $_.ParentProcessId -eq $appPid }
    $ids = @($roots | ForEach-Object { $_.ProcessId })
    $children = $all | Where-Object { $ids -contains $_.ParentProcessId }
    @($roots) + @($children) | Where-Object { $_ } | ForEach-Object { Get-Process -Id $_.ProcessId -ErrorAction SilentlyContinue }
}

function Ready {
    try {
        $response = Invoke-WebRequest -UseBasicParsing 'http://127.0.0.1:8181/api/v1/health/ready' -TimeoutSec 2
        return $response.StatusCode -eq 200
    } catch {
        return $false
    }
}

if (-not $SkipInstall) {
    Step 'Instalación en silencio'
    if (-not (Test-Path $Installer)) {
        throw 'Indica el instalador con -Installer.'
    }
    $Result.installer = (Resolve-Path $Installer).Path
    $Result.installer_bytes = (Get-Item $Installer).Length
    $watch = [Diagnostics.Stopwatch]::StartNew()
    $setup = Start-Process -FilePath $Installer -ArgumentList '/S' -PassThru -Wait
    $Result.install_seconds = [math]::Round($watch.Elapsed.TotalSeconds, 1)
    $Result.install_exit = $setup.ExitCode
    Write-Host "Instalador terminó con código $($setup.ExitCode) en $($Result.install_seconds) s"
}
if (-not (Test-Path $AppExe)) {
    throw "No se encontró la aplicación instalada en $InstallDir"
}
$Result.install_dir_files = (Get-ChildItem $InstallDir -Recurse -File).Count
$Result.install_dir_mb = [math]::Round(((Get-ChildItem $InstallDir -Recurse -File | Measure-Object Length -Sum).Sum / 1MB), 1)
$Result.version = (Get-Item $AppExe).VersionInfo.ProductVersion
$Result.data_dir_existed = Test-Path (Join-Path $DataDir 'postgres\PG_VERSION')
$uninstallKey = Get-ChildItem 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall' |
    Where-Object { (Get-ItemProperty $_.PSPath).DisplayName -eq $Product } |
    Select-Object -First 1
if ($uninstallKey) {
    $entry = Get-ItemProperty $uninstallKey.PSPath
    $Result.apps_entry = [ordered]@{ name = $entry.DisplayName; version = $entry.DisplayVersion; publisher = $entry.Publisher; uninstall = $entry.UninstallString }
}
$Result.start_menu = @(Get-ChildItem (Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\$Product") -ErrorAction SilentlyContinue | ForEach-Object { $_.Name })
$Result.desktop_shortcut = Test-Path (Join-Path ([Environment]::GetFolderPath('Desktop')) "$Product.lnk")

Step 'Arranque'
if (OwnProcesses) {
    throw 'La aplicación ya está abierta; ciérrala antes de medir el arranque.'
}
$before = [AdelWindows]::Visible()
$watch = [Diagnostics.Stopwatch]::StartNew()
$app = Start-Process -FilePath $AppExe -PassThru
$windowSeconds = $null
$readySeconds = $null
while ($watch.Elapsed.TotalSeconds -lt $ReadyTimeoutSeconds) {
    if ($null -eq $windowSeconds) {
        $app.Refresh()
        if ($app.MainWindowHandle -ne 0) {
            $windowSeconds = [math]::Round($watch.Elapsed.TotalSeconds, 2)
        }
    }
    if ($null -eq $readySeconds -and (Ready)) {
        $readySeconds = [math]::Round($watch.Elapsed.TotalSeconds, 2)
    }
    if ($null -ne $windowSeconds -and $null -ne $readySeconds) {
        break
    }
    Start-Sleep -Milliseconds 200
}
$Result.window_seconds = $windowSeconds
$Result.server_ready_seconds = $readySeconds
Write-Host "Ventana en $windowSeconds s; servidor listo en $readySeconds s"
if ($null -eq $readySeconds) {
    throw 'El servidor local no respondió a tiempo; revisa %LOCALAPPDATA%\AD-EL POS\logs.'
}
$version = (Invoke-WebRequest -UseBasicParsing 'http://127.0.0.1:8181/api/v1/version' -TimeoutSec 5).Content | ConvertFrom-Json
$Result.server_version = $version.server_version
$Result.schema_version = $version.schema_version
Start-Sleep -Seconds 8

Step 'Ventanas nuevas (no debe haber consolas)'
$after = [AdelWindows]::Visible()
$new = $after | Where-Object { $before -notcontains $_ }
$windows = @()
foreach ($line in $new) {
    $parts = $line.Split('|', 4)
    $process = Get-Process -Id ([int]$parts[1]) -ErrorAction SilentlyContinue
    $windows += [ordered]@{ process = $process.ProcessName; class = $parts[2]; title = $parts[3] }
    Write-Host "  $($process.ProcessName) · $($parts[2]) · $($parts[3])"
}
$Result.new_windows = $windows
$consoleClasses = @('ConsoleWindowClass', 'CASCADIA_HOSTING_WINDOW_CLASS', 'PseudoConsoleWindow')
$Result.console_windows = @($windows | Where-Object { $consoleClasses -contains $_.class }).Count
Write-Host "Ventanas de consola nuevas: $($Result.console_windows)"

Step 'Memoria con la aplicación abierta'
$own = @(OwnProcesses)
$webview = @(WebViewProcesses $app.Id)
$groups = [ordered]@{
    aplicacion = @($own | Where-Object { $_.ProcessName -eq 'ad-el-pos-desktop' })
    webview2 = $webview
    servidor = @($own | Where-Object { $_.ProcessName -eq 'ad-el-pos-server' })
    postgresql = @($own | Where-Object { $_.ProcessName -eq 'postgres' })
}
$memory = [ordered]@{}
$total = 0
foreach ($name in $groups.Keys) {
    $set = ($groups[$name] | Measure-Object WorkingSet64 -Sum).Sum
    $private = ($groups[$name] | Measure-Object PrivateMemorySize64 -Sum).Sum
    $memory[$name] = [ordered]@{ processes = $groups[$name].Count; working_set_mb = [math]::Round($set / 1MB, 1); private_mb = [math]::Round($private / 1MB, 1) }
    $total += $set
    Write-Host ("  {0,-11} {1,2} procesos · {2,7:N1} MB en memoria · {3,7:N1} MB privados" -f $name, $groups[$name].Count, ($set / 1MB), ($private / 1MB))
}
$Result.memory = $memory
$Result.memory_total_working_set_mb = [math]::Round($total / 1MB, 1)
Write-Host "Total: $($Result.memory_total_working_set_mb) MB"

if ($KeepOpen) {
    Write-Host 'La aplicación queda abierta (-KeepOpen).'
} else {
    Step 'Cierre ordenado'
    $watch = [Diagnostics.Stopwatch]::StartNew()
    $null = $app.CloseMainWindow()
    while ((OwnProcesses) -and $watch.Elapsed.TotalSeconds -lt 60) {
        Start-Sleep -Milliseconds 250
    }
    $Result.close_seconds = [math]::Round($watch.Elapsed.TotalSeconds, 2)
    $left = @(OwnProcesses | ForEach-Object { $_.ProcessName })
    $Result.processes_left = $left
    $Result.postmaster_pid_left = Test-Path (Join-Path $DataDir 'postgres\postmaster.pid')
    Write-Host "Cerrado en $($Result.close_seconds) s; procesos que quedan: $($left.Count); postmaster.pid: $($Result.postmaster_pid_left)"
}

if ($Uninstall) {
    Step 'Desinstalación en silencio'
    $uninstaller = Join-Path $InstallDir 'uninstall.exe'
    Start-Process -FilePath $uninstaller -ArgumentList '/S' -Wait | Out-Null
    $watch = [Diagnostics.Stopwatch]::StartNew()
    while ((Test-Path $AppExe) -and $watch.Elapsed.TotalSeconds -lt 60) {
        Start-Sleep -Milliseconds 250
    }
    $Result.uninstalled = -not (Test-Path $AppExe)
    $Result.data_kept = Test-Path (Join-Path $DataDir 'postgres\PG_VERSION')
    $Result.backups_kept = @(Get-ChildItem (Join-Path $DataDir 'backups') -Filter *.dump -ErrorAction SilentlyContinue).Count
    Write-Host "Programa quitado: $($Result.uninstalled); datos conservados: $($Result.data_kept); respaldos: $($Result.backups_kept)"
}

$Result.finished = (Get-Date).ToString('s')
if ($Report) {
    $Result | ConvertTo-Json -Depth 6 | Out-File -Encoding utf8 $Report
    Write-Host "Informe: $Report"
}
