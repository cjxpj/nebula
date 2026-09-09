# 发布步骤: 更新版本号 (embed.go + versioninfo.json)
# 说明: 不使用 ConvertFrom-Json/ConvertTo-Json —— PowerShell 5.1 对共享引用的 PSObject
#       序列化时会进入死循环挂起(ConvertTo-Json 偶发卡死)。这里改为纯文本正则替换,
#       同时保留 goversioninfo 生成的原始 JSON 排版。
$ErrorActionPreference = 'Stop';
$verFile = Join-Path $env:TEMP 'nebula_release_ver.txt';
if (-not (Test-Path $verFile)) { throw "未找到版本号临时文件: $verFile" }
$v = (Get-Content -Raw $verFile).Trim();
if ($v -notmatch '^\d+(\.\d+){1,3}$') { throw "版本号格式错误: $v" }
$root = Split-Path -Parent $PSScriptRoot;
$enc = New-Object System.Text.UTF8Encoding($false);

# 1) embed.go: var Version string = "x.y.z"
$e = Join-Path $root 'nebula\appfiles\embed.go';
$t = [IO.File]::ReadAllText($e);
if ($t -notmatch 'var Version string = "[^"]*"') { throw 'embed.go 中未找到版本号声明' }
$t = [regex]::Replace($t, 'var Version string = "[^"]*"', 'var Version string = "' + $v + '"');
[IO.File]::WriteAllText($e, $t, $enc);

# 2) versioninfo.json: 更新 FileVersion/ProductVersion 两个字符串字段与 Major/Minor/Patch 数值块
$p = Join-Path $root 'nebula\app\win\versioninfo.json';
if (-not (Test-Path $p)) { throw "未找到 versioninfo.json: $p" }
$j = [IO.File]::ReadAllText($p);
$j = [regex]::Replace($j, '("(?:FileVersion|ProductVersion)"\s*:\s*)"[^"]*"', ('${1}"' + $v + '"'));
$a = $v.Split('.');
$j = [regex]::Replace($j, '("Major"\s*:\s*)\d+(\s*,\s*"Minor"\s*:\s*)\d+(\s*,\s*"Patch"\s*:\s*)\d+', ('${1}' + [int]$a[0] + '${2}' + [int]$a[1] + '${3}' + [int]$a[2]));
[IO.File]::WriteAllText($p, $j, $enc);
Write-Host ('版本号已更新为 ' + $v);