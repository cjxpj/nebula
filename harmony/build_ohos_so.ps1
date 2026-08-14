# 编译 Go -> 鸿蒙(OpenHarmony)可加载的 native 库 libnebula.so
#
# 用法：
#   .\build_ohos_so.ps1                     # 默认 arm64（鸿蒙真机）
#   .\build_ohos_so.ps1 -Arch amd64         # x86_64 模拟器
#   .\build_ohos_so.ps1 -SdkPath "D:\DevEco Studio\sdk\default\openharmony"
#
# 说明：
#   - 鸿蒙内核为 Linux 系、libc 为 musl，因此用 GOOS=linux + OHOS clang/sysroot 做 CGO 交叉编译。
#   - x86_64 模拟器上 Go 的 TLS 模型与鸿蒙 musl 存在兼容问题（可能启动闪退），arm64 真机正常。
param(
    [ValidateSet("arm64", "amd64")]
    [string]$Arch = "arm64",
    [string]$SdkPath = $env:DEVECO_SDK_HOME
)

$ErrorActionPreference = "Stop"

if (-not $SdkPath) {
    $SdkPath = "D:\AppData\Huawei\DevEco Studio\sdk\default\openharmony"
}

# 路径含空格（如 "DevEco Studio"）时转成 8.3 短路径，避免 cgo 的 --sysroot 参数被空格拆分
if ($SdkPath -match '\s') {
    $SdkPath = (New-Object -ComObject Scripting.FileSystemObject).GetFolder($SdkPath).ShortPath
}

# harmony 目录的上一级 = 仓库根目录
$repoRoot = Split-Path -Parent $PSScriptRoot

if ($Arch -eq "arm64") {
    $goarch = "arm64"
    $triple = "aarch64-linux-ohos"
    $abi    = "arm64-v8a"
} else {
    $goarch = "amd64"
    $triple = "x86_64-linux-ohos"
    $abi    = "x86_64"
}

$clang   = Join-Path $SdkPath "native\llvm\bin\clang.exe"
$sysroot = (Join-Path $SdkPath "native\sysroot").Replace('\', '/')

if (-not (Test-Path $clang)) {
    Write-Error "未找到 OHOS clang：$clang`n请通过 -SdkPath 指定鸿蒙 SDK 根目录（如 ...\sdk\default\openharmony）"
}

$outDir     = Join-Path $repoRoot "harmony\entry\src\main\libs\$abi"
$outSo      = Join-Path $outDir "libnebula.so"
$outHeader  = Join-Path $repoRoot "harmony\entry\src\main\cpp\include\libnebula.h"

New-Item -ItemType Directory -Force -Path $outDir | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path $outHeader) | Out-Null

$env:CGO_ENABLED  = "1"
$env:GOOS         = "linux"
$env:GOARCH       = $goarch
$env:CC           = "`"$clang`""
$env:CGO_CFLAGS   = "-target $triple --sysroot=$sysroot -D__MUSL__"
$env:CGO_LDFLAGS  = "-target $triple --sysroot=$sysroot"

Write-Host "编译 Go -> libnebula.so（GOARCH=$goarch, target=$triple）..."

Push-Location $repoRoot
try {
    go build -tags ohos -buildmode=c-shared -ldflags "-s -w" -trimpath -o $outSo ./nebula/app
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}

# -buildmode=c-shared 会同时生成 libnebula.h，与 .so 同名同目录
$generatedHeader = Join-Path $outDir "libnebula.h"
if (Test-Path $generatedHeader) {
    Copy-Item $generatedHeader $outHeader -Force
} else {
    Write-Warning "未找到生成的头文件（预期 $generatedHeader），请检查 go build 输出"
}

Write-Host "完成："
Write-Host "  .so -> $outSo"
Write-Host "  .h  -> $outHeader"
Write-Host "后续：用 DevEco Studio 打开 harmony 工程，签名后 assembleHap 即可生成 .hap"
