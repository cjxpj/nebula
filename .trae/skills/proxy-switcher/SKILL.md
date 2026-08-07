---
name: "proxy-switcher"
description: "Switch terminal proxy to 127.0.0.1:7890 (Clash) on Windows. Invoke when network access fails (GitHub, go mod, npm, pip) or user asks to enable/disable/switch proxy."
---

# Proxy Switcher

在 Windows 上切换/管理终端代理。默认代理端口为 7890（Clash 默认端口）。

## 何时使用

- 用户反馈网络访问失败（GitHub、go mod download、npm install、pip install 等）
- 用户要求开启/关闭/切换代理
- 需要为当前终端会话设置代理环境变量

## 代理默认值

- 代理地址：`127.0.0.1`
- 默认端口：`7890`（可让用户通过变量 `PROXY_PORT` 指定其他端口，如 7891）

## 操作步骤

### 1. 检查代理是否可用

先确认 7890 端口是否有进程监听：

```powershell
Get-NetTCPConnection -LocalPort 7890 -State Listen
```

没有输出则说明本地没有代理服务在监听，提示用户先启动 Clash/V2Ray 等工具。

### 2. 测试当前网络连通性

```powershell
# 测试外网连通性
curl.exe -s -m 5 https://www.google.com
# 或测试 GitHub
curl.exe -s -m 5 https://github.com
```

### 3. 开启代理（当前终端会话）

```powershell
$env:HTTP_PROXY = "http://127.0.0.1:7890"
$env:HTTPS_PROXY = "http://127.0.0.1:7890"
$env:ALL_PROXY = "http://127.0.0.1:7890"
$env:http_proxy = "http://127.0.0.1:7890"
$env:https_proxy = "http://127.0.0.1:7890"
$env:all_proxy = "http://127.0.0.1:7890"
```

注意：环境变量只对当前终端会话生效。若要持久化到用户级别，使用：

```powershell
[Environment]::SetEnvironmentVariable("HTTP_PROXY", "http://127.0.0.1:7890", "User")
[Environment]::SetEnvironmentVariable("HTTPS_PROXY", "http://127.0.0.1:7890", "User")
[Environment]::SetEnvironmentVariable("ALL_PROXY", "http://127.0.0.1:7890", "User")
```

### 4. 关闭代理

```powershell
Remove-Item Env:HTTP_PROXY, Env:HTTPS_PROXY, Env:ALL_PROXY, Env:http_proxy, Env:https_proxy, Env:all_proxy -ErrorAction SilentlyContinue
```

持久化的用户级代理清理：

```powershell
[Environment]::SetEnvironmentVariable("HTTP_PROXY", $null, "User")
[Environment]::SetEnvironmentVariable("HTTPS_PROXY", $null, "User")
[Environment]::SetEnvironmentVariable("ALL_PROXY", $null, "User")
```

### 5. 验证

开启代理后重新执行连通性测试，确认问题是否解决。若仍然失败，提示用户检查代理工具本身是否正常运行。

## 相关配置

### Go 模块代理

如果 `go mod download` 失败，可以同时配置 GOPROXY：

```powershell
go env -w GOPROXY=https://goproxy.cn,direct
```

### npm 代理

```powershell
npm config set proxy http://127.0.0.1:7890
npm config set https-proxy http://127.0.0.1:7890
```

## 注意事项

- 代理环境变量只对设置它的终端会话生效；如果要在所有新终端生效，请使用用户级（User）持久化方式。
- 开启代理前先确认本地 7890 端口有代理服务在监听，否则会导致所有请求失败。
- 不使用代理时及时清理环境变量，避免影响本地服务调试。
