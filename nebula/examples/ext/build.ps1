# Build the Nebula C extension example (run inside nebula/examples/ext).
#
#   .\build.ps1                  # Windows (MinGW-w64) -> example.dll
#   .\build.ps1 -Platform linux  # Linux -> example.so
#
# Cross-compiling a Linux .so from Windows needs zig or x86_64-linux-gnu-gcc.

param(
    [string]$Platform = "windows"
)

$ErrorActionPreference = "Stop"

switch ($Platform) {
    "windows" {
        gcc -shared -Os -s -ffunction-sections -fdata-sections "-Wl,--gc-sections" -fno-asynchronous-unwind-tables -fno-stack-protector -o example.dll main.c
        Write-Host "Built example.dll (place it under private/plugins/ to use)"
    }
    "linux" {
        gcc -shared -fPIC -Os -s -ffunction-sections -fdata-sections "-Wl,--gc-sections" -fno-asynchronous-unwind-tables -fno-stack-protector -o example.so main.c
        Write-Host "Built example.so (place it under private/plugins/ to use)"
    }
    default {
        Write-Error "Unsupported platform: $Platform (choose windows / linux)"
    }
}
