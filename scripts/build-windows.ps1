# Instalador de prueba de AD-EL POS para Windows (etapa 9E, §4), de principio a fin y con un solo comando:
#
#   powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1
#
# 1. Comprueba las herramientas del proyecto (.tools: Go, Node, Rust y PostgreSQL 18) y Visual Studio Build
#    Tools con C++.
# 2. Sincroniza los metadatos de config/product.json e instala las dependencias de npm si faltan.
# 3. Compila el servidor Go (.exe) como binario externo de Tauri.
# 4. Prepara solo los binarios de PostgreSQL que usa la instalación, con el runtime de Visual C++, y
#    comprueba que con ellos se puede crear un clúster.
# 5. Genera el instalador NSIS con tauri.windows-installer.conf.json (destino nsis, plantilla propia,
#    hooks, WebView2 incluido) y muestra su ruta, tamaño y SHA-256.
#
# No instala nada en el equipo ni toca datos: el resultado queda en desktop\src-tauri\target\release\bundle\nsis.
param(
    # Vuelve a ejecutar npm ci aunque ya exista node_modules.
    [switch]$CleanInstall,
    # Omite la comprobación de PostgreSQL preparado (initdb en una carpeta temporal, unos 10 s).
    [switch]$SkipPostgresCheck
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'env.ps1')
$Root = $env:PROJECT_ROOT
$Desktop = Join-Path $Root 'desktop'
$Tauri = Join-Path $Desktop 'src-tauri'
$Node = Join-Path $Root '.tools\node\node.exe'
$NpmCli = Join-Path $Root '.tools\node\node_modules\npm\bin\npm-cli.js'
$PgSource = Join-Path $Root '.tools\pgsql'
$Stage = Join-Path $Tauri 'target\windows-bundle\postgresql'
$Target = 'x86_64-pc-windows-msvc'

function Step($text) {
    Write-Host ''
    Write-Host "== $text" -ForegroundColor Cyan
}

function Require($condition, $message) {
    if (-not $condition) {
        throw $message
    }
}

function Invoke-Checked {
    param([string]$Program, [string[]]$Arguments, [string]$WorkingDirectory = $Root)
    Push-Location $WorkingDirectory
    try {
        & $Program @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "$([IO.Path]::GetFileName($Program)) terminó con código $LASTEXITCODE."
        }
    } finally {
        Pop-Location
    }
}

Step 'Herramientas'
Require (Test-Path $Node) 'Falta Node en .tools\node (DEVELOPMENT_WINDOWS.md, «Herramientas»).'
Require (Test-Path (Join-Path $Root '.tools\go\bin\go.exe')) 'Falta Go en .tools\go.'
Require (Test-Path (Join-Path $Root '.tools\cargo\bin\cargo.exe')) 'Falta Rust en .tools\cargo.'
Require (Test-Path (Join-Path $PgSource 'bin\postgres.exe')) 'Faltan los binarios de PostgreSQL 18 en .tools\pgsql.'
$VsWhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
Require (Test-Path $VsWhere) 'Falta Visual Studio Build Tools con «Desarrollo para el escritorio con C++».'
$VsPath = & $VsWhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
Require ([bool]$VsPath) 'Visual Studio Build Tools no tiene las herramientas de C++ x64.'
Write-Host "Go: $(& go version)"
Write-Host "Node: $(& $Node --version)"
Write-Host "Rust: $(& rustc --version)"
Write-Host "PostgreSQL: $(& (Join-Path $PgSource 'bin\postgres.exe') --version)"
Write-Host "Visual Studio: $VsPath"

Step 'Metadatos y dependencias'
Invoke-Checked $Node @((Join-Path $Root 'scripts\sync-metadata.mjs'))
$Product = Get-Content -Raw -Encoding UTF8 (Join-Path $Root 'config\product.json') | ConvertFrom-Json
if ($CleanInstall -or -not (Test-Path (Join-Path $Desktop 'node_modules'))) {
    Invoke-Checked $Node @($NpmCli, 'ci') $Desktop
}

Step "Servidor Go $($Product.version)"
$Binaries = Join-Path $Tauri 'binaries'
New-Item -ItemType Directory -Force $Binaries | Out-Null
$ServerExe = Join-Path $Binaries "$($Product.server_binary)-$Target.exe"
$env:CGO_ENABLED = '0'
$env:GOFLAGS = '-mod=readonly'
Invoke-Checked 'go' @('build', '-trimpath', '-o', $ServerExe, './cmd/ad-el-pos-server') (Join-Path $Root 'backend')
Remove-Item Env:\GOFLAGS
Write-Host "Servidor: $ServerExe ($([math]::Round((Get-Item $ServerExe).Length / 1MB, 1)) MB)"

Step 'PostgreSQL incluido (solo lo que se usa)'
if (Test-Path $Stage) {
    Remove-Item -Recurse -Force $Stage
}
New-Item -ItemType Directory -Force (Join-Path $Stage 'bin'), (Join-Path $Stage 'lib') | Out-Null
# Programas que usa la instalación (crear, arrancar y detener el clúster, respaldos y soporte) y las
# bibliotecas de las que dependen, obtenidas con dumpbin /dependents.
$BinFiles = @(
    'initdb.exe', 'pg_ctl.exe', 'postgres.exe', 'pg_dump.exe', 'pg_restore.exe', 'psql.exe',
    'libpq.dll', 'libintl-9.dll', 'libiconv-2.dll', 'libcrypto-3-x64.dll', 'libssl-3-x64.dll',
    'zlib1.dll', 'liblz4.dll', 'libzstd.dll', 'libwinpthread-1.dll', 'libxml2.dll',
    'icudt77.dll', 'icuin77.dll', 'icuuc77.dll'
)
foreach ($file in $BinFiles) {
    Copy-Item (Join-Path $PgSource "bin\$file") (Join-Path $Stage 'bin')
}
# PL/pgSQL (las migraciones lo usan), los diccionarios Snowball que crea initdb y las conversiones de
# codificación (psql en la consola de Windows usa la página de códigos del sistema).
$LibFiles = @('plpgsql.dll', 'dict_snowball.dll')
$LibFiles += Get-ChildItem (Join-Path $PgSource 'lib') -Filter '*.dll' |
    Where-Object { $_.Name -match '_and_|^utf8_' } |
    ForEach-Object { $_.Name }
foreach ($file in $LibFiles) {
    Copy-Item (Join-Path $PgSource "lib\$file") (Join-Path $Stage 'lib')
}
# share sin traducciones (locale, 22 MB): la instalación usa mensajes en inglés de PostgreSQL en su bitácora.
Copy-Item -Recurse (Join-Path $PgSource 'share') (Join-Path $Stage 'share')
Remove-Item -Recurse -Force (Join-Path $Stage 'share\locale'), (Join-Path $Stage 'share\doc') -ErrorAction SilentlyContinue
Get-ChildItem (Join-Path $Stage 'share\extension') | Where-Object { $_.Name -notlike 'plpgsql*' } | Remove-Item -Force
foreach ($license in 'server_license.txt', 'commandlinetools_3rd_party_licenses.txt') {
    Copy-Item (Join-Path $PgSource $license) $Stage
}
# Runtime de Visual C++ junto a los programas de PostgreSQL (los binarios de EDB no lo incluyen y un
# Windows recién instalado puede no tenerlo). Implementación local permitida por la licencia de Visual Studio.
$Crt = Get-ChildItem (Join-Path $VsPath 'VC\Redist\MSVC') -Directory |
    Sort-Object Name -Descending |
    ForEach-Object { Join-Path $_.FullName 'x64\Microsoft.VC143.CRT' } |
    Where-Object { Test-Path (Join-Path $_ 'vcruntime140.dll') } |
    Select-Object -First 1
Require ([bool]$Crt) 'No se encontró el runtime de Visual C++ (Microsoft.VC143.CRT) en Visual Studio Build Tools.'
foreach ($file in 'vcruntime140.dll', 'vcruntime140_1.dll', 'msvcp140.dll') {
    Copy-Item (Join-Path $Crt $file) (Join-Path $Stage 'bin')
}
$StageSize = (Get-ChildItem $Stage -Recurse -File | Measure-Object Length -Sum).Sum
Write-Host "PostgreSQL preparado: $([math]::Round($StageSize / 1MB, 1)) MB en $Stage"

if (-not $SkipPostgresCheck) {
    Step 'Comprobación de PostgreSQL preparado (initdb en una carpeta temporal)'
    $Check = Join-Path ([IO.Path]::GetTempPath()) ("adel-pg-check-" + [guid]::NewGuid().ToString('N'))
    $PwFile = "$Check.pw"
    try {
        [IO.File]::WriteAllText($PwFile, [guid]::NewGuid().ToString('N'))
        $env:LC_MESSAGES = 'C'
        & (Join-Path $Stage 'bin\initdb.exe') -D $Check --username=adel_check --auth=scram-sha-256 --encoding=UTF8 --locale=C "--pwfile=$PwFile" | Out-Null
        Require ($LASTEXITCODE -eq 0) 'initdb falló con los binarios preparados; revisa la lista de archivos.'
        Write-Host 'initdb funcionó con los binarios preparados.'
    } finally {
        Remove-Item -Recurse -Force $Check, $PwFile -ErrorAction SilentlyContinue
    }
}

Step 'Instalador NSIS'
Invoke-Checked $Node @($NpmCli, 'run', 'tauri', '--', 'build', '--bundles', 'nsis', '--config', 'src-tauri/tauri.windows-installer.conf.json') $Desktop
$Installer = Get-ChildItem (Join-Path $Tauri 'target\release\bundle\nsis') -Filter "*_$($Product.version)_x64-setup.exe" |
    Sort-Object LastWriteTime -Descending |
    Select-Object -First 1
Require ([bool]$Installer) 'No se encontró el instalador generado.'
$Hash = (Get-FileHash -Algorithm SHA256 $Installer.FullName).Hash.ToLower()

Step 'Listo'
Write-Host "Instalador: $($Installer.FullName)"
Write-Host "Tamaño:     $([math]::Round($Installer.Length / 1MB, 1)) MB ($($Installer.Length) bytes)"
Write-Host "SHA-256:    $Hash"
