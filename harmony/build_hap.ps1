# Build HarmonyOS HAP/app and copy to dist.
#
# Steps:
#   1. Sync version from nebula/appfiles/embed.go to harmony/AppScope/app.json5
#   2. Cross-compile Go -> libnebula.so (arm64 by default)
#   3. hvigor assembleHap -> entry-default-unsigned.hap
#   4. hvigor assembleApp -> harmony-default-unsigned.app
#   5. Copy both artifacts to dist/

param(
    [ValidateSet("arm64", "amd64")]
    [string]$Arch = "arm64"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$harmonyDir = Join-Path $repoRoot "harmony"

$nodeExe = "D:\AppData\Huawei\DevEco Studio\tools\node\node.exe"
$hvigorJs = "D:\AppData\Huawei\DevEco Studio\tools\hvigor\bin\hvigorw.js"

if (-not (Test-Path $nodeExe)) { Write-Error "node.exe not found: $nodeExe" }
if (-not (Test-Path $hvigorJs)) { Write-Error "hvigorw.js not found: $hvigorJs" }

# --- 1. Sync version ---
$embedGo = Join-Path $repoRoot "nebula\appfiles\embed.go"
$appJson5 = Join-Path $harmonyDir "AppScope\app.json5"

$content = [IO.File]::ReadAllText($embedGo)
$m = [regex]::Match($content, 'var Version string = "(.+)"')
if (-not $m.Success) { Write-Error "Version string not found in embed.go" }
$version = $m.Groups[1].Value
$parts = $version.Split('.')
if ($parts.Count -lt 3) { Write-Error "Invalid version format: $version (expected x.y.z)" }
$major = [int]$parts[0]
$minor = [int]$parts[1]
$patch = [int]$parts[2]
$versionCode = $major * 10000 + $minor * 100 + $patch

$json5 = [IO.File]::ReadAllText($appJson5)
$json5 = [regex]::Replace($json5, '"versionCode"\s*:\s*\d+', '"versionCode": ' + $versionCode)
$json5 = [regex]::Replace($json5, '"versionName"\s*:\s*"[^"]*"', '"versionName": "' + $version + '"')
[IO.File]::WriteAllText($appJson5, $json5, (New-Object System.Text.UTF8Encoding $false))
Write-Host "[1/5] Version synced: $version (versionCode=$versionCode)"

# --- 2. Compile Go -> libnebula.so ---
Write-Host "[2/5] Compiling Go -> libnebula.so ($Arch)..."
& (Join-Path $PSScriptRoot "build_ohos_so.ps1") -Arch $Arch
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# --- 3 & 4. hvigor assembleHap + assembleApp ---
Push-Location $harmonyDir
try {
    Write-Host "[3/5] hvigor assembleHap..."
    & $nodeExe $hvigorJs '--mode' 'module' '-p' 'product=default' '-p' 'module=entry@default' '-p' 'buildMode=release' '--no-daemon' 'assembleHap'
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    Write-Host "[4/5] hvigor assembleApp..."
    & $nodeExe $hvigorJs '--mode' 'project' '-p' 'product=default' '-p' 'buildMode=release' '--no-daemon' 'assembleApp'
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}

# --- 5. Copy to dist ---
$dist = Join-Path $repoRoot "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null

$hapSrc = Join-Path $harmonyDir "entry\build\default\outputs\default\entry-default-unsigned.hap"
$appSrc = Join-Path $harmonyDir "build\outputs\default\harmony-default-unsigned.app"

if (Test-Path $hapSrc) {
    Copy-Item $hapSrc (Join-Path $dist "nebula-unsigned.hap") -Force
    Write-Host "[5/5] HAP -> dist\nebula-unsigned.hap"
} else {
    Write-Warning "HAP artifact not found: $hapSrc"
}

if (Test-Path $appSrc) {
    Copy-Item $appSrc (Join-Path $dist "nebula-unsigned.app") -Force
    Write-Host "[5/5] APP -> dist\nebula-unsigned.app"
} else {
    Write-Warning "APP artifact not found: $appSrc"
}

Write-Host "HarmonyOS build completed."
