<h1 align="center">nebula</h1>

<div align="center">

![版本](https://img.shields.io/badge/%E7%89%88%E6%9C%AC-17.3.0-blue)
![平台](https://img.shields.io/badge/%E5%B9%B3%E5%8F%B0-Windows%20%7C%20Linux%20%7C%20macOS-green)
![语言](https://img.shields.io/badge/%E8%AF%AD%E8%A8%80-Go%20%7C%20Nebula%20Script-orange)
![许可证](https://img.shields.io/badge/%E8%AE%B8%E5%8F%AF%E8%AF%81-MIT-yellow)

</div>

## 🌟 项目介绍

**Nebula** 是一个面向 Web 服务的脚本语言与运行时系统，用于将词库逻辑直接映射为 HTTP 服务，显著降低后端业务编写与维护成本。它内置完整的运行环境，无需额外编译、复杂配置或容器部署，实现真正的开箱即用与即写即跑。

> 每一个 Nebula 词库都是可执行单元，可被直接映射为 Web API、WebSocket 或自动任务，非常适合快速构建业务中台与自动化服务。

### 🎯 设计理念
- **原生面向 Web 服务**：以内置 HTTP 服务为核心设计目标
- **词库即服务**：通过词库与脚本即可定义接口逻辑、返回结构与执行流程
- **零部署·直接启动**：启动程序即可自动加载词库并开放 Web 服务

## 🚀 核心特性

### 🔧 技术特性
- **按需扩展运行时**：支持 PHP、Python、FFmpeg 等扩展运行时，通过管理后台一键安装
- **多机器人支持**：QQ、NapCat、飞书、云湖机器人一体化集成
- **跨平台部署**：Windows 独立客户端 / Linux & macOS Docker 容器化部署
- **自动配置管理**：首次启动自动生成配置目录和默认文件
- **健康检查监控**：内置服务健康检查和资源监控

### 🤖 机器人功能
- **腾讯 QQ 机器人**：支持官方开放平台和 NapCat 非官方机器人
- **飞书机器人**：企业级飞书机器人集成
- **云湖机器人**：第三方机器人平台支持
- **消息类型**：文本、图片、Markdown、视频、语音等
- **管理功能**：禁言、踢人、撤回、点赞、戳一戳等

### 🌐 Web 服务
- **HTTP API 服务**：RESTful 风格 API 自动映射
- **WebSocket 支持**：实时双向通信
- **管理面板**：内置 Web 管理界面
- **Ngrok 穿透**：内网穿透支持
- **跨域支持**：CORS 配置和管理

## 📦 快速开始

### 跨平台部署策略

| 平台 | 部署方式 | 说明 |
|------|----------|------|
| **Windows** | 单个客户端 (`nebulaApp.exe`) | 独立可执行文件，无需 Docker |
| **Linux/macOS** | Docker 容器化部署 | 使用 Docker Compose 一键部署 |
| **Android** | 原生 App（Gradle + WebView） | Go 核心 `.so` 通过 JNI 集成 |
| **HarmonyOS** | 原生 App（ArkTS + C++ NAPI） | Go 核心 `.so` 通过 NAPI 集成 |

### Windows 平台（独立客户端）

1. **下载客户端**
   ```bash
   # 从 Releases 页面下载 nebulaApp.exe
   # 或使用项目中的预构建版本: nebula\app\win\nebulaApp.exe
   ```

2. **启动服务**
   ```bash
   # 直接双击运行，或使用命令行：
   .\nebula\app\win\nebulaApp.exe
   ```

3. **访问服务**
   - Web 管理面板：http://localhost:8080/nebula
   - HTTP API：http://localhost:8080
   - 首次启动后查看日志获取随机访问路径

4. **客户端命令**
   ```bash
   # 查看帮助
   .\nebula\app\win\nebulaApp.exe -help
   
   # 显示版本
   .\nebula\app\win\nebulaApp.exe -v
   
   # 设置开机自启
   .\nebula\app\win\nebulaApp.exe -autostart
   
   # 取消开机自启
   .\nebula\app\win\nebulaApp.exe -noautostart
   ```

### Linux/macOS 平台（Docker 部署）

1. **环境准备**
   ```bash
   # 确保已安装 Docker 和 Docker Compose
   docker --version
   docker-compose --version
   ```

2. **一键部署脚本**
   ```bash
   # 给脚本添加执行权限
   chmod +x deploy.sh
   
   # 查看帮助
   ./deploy.sh help
   
   # 构建镜像
   ./deploy.sh build
   
   # 启动服务（后台运行）
   ./deploy.sh up
   
   # 查看服务状态
   ./deploy.sh status
   
   # 查看日志
   ./deploy.sh logs
   
   # 停止服务
   ./deploy.sh down
   ```

## 📁 项目结构

```
nebula/
├── nebula/                    # 核心引擎模块
│   ├── app/                   # 多平台客户端（win / wasm / .so）
│   │   ├── win/               # Windows 客户端
│   │   └── wasm/              # WebAssembly 支持
│   ├── appfiles/             # 嵌入式资源文件
│   ├── bot/                  # 机器人模块（QQ、NapCat、飞书、云湖）
│   ├── dic/                  # 词库解析和运行引擎
│   ├── dto/                  # 数据传输对象
│   ├── server/               # HTTP 服务器实现
│   └── utils/                # 工具函数
├── deploy.sh                 # Linux/macOS 部署脚本
├── deploy.ps1                # Windows PowerShell 部署脚本
├── deploy.bat                # Windows 批处理部署脚本
├── Dockerfile                # Docker 构建配置
├── docker-compose.yml        # Docker Compose 配置
└── validate-deployment.sh    # 部署验证脚本
```

## ⚙️ 配置说明

### 配置文件位置
- **系统配置**：`NebulaData/private/system/system.ini`
- **机器人配置**：`NebulaData/private/system/config.ini`
- **启动词库**：`NebulaData/private/system/start.n`
- **路由词库**：`NebulaData/private/system/router.n`


## 🤖 机器人功能详解

### QQ 机器人
支持腾讯官方开放平台和 NapCat 非官方机器人：

```nebula
# 基础消息发送
$发送文本 <文本> <图片数据>$

# Markdown 消息（支持自定义按钮）
$发送MD <Markdown内容> <键盘JSON(可选)>$
$发送MD <模板ID> <键1> <值1>...$

# 消息按钮示例（指令按钮，点击自动填入 /确认）
$发送MD # 菜单 {"rows":[{"buttons":[{"id":"1","render_data":{"label":"确认","visited_label":"已确认","style":1},"action":{"type":2,"permission":{"type":2},"data":"/确认","enter":true,"unsupport_tips":"请升级"}}]}]}

# 群管理功能（通过官方 API）
$禁言 <群号> <QQ号> <秒数>$
$全体禁言 <群号>$
$撤回 <消息ID>$

# 群事件（词库监听）
[内部]入群         // 成员加入
[内部]退群         // 成员退出
[内部]入群申请     // 用户申请入群（%成员%=申请人OpenID）

# 互动功能
$点赞 <QQ号> <次数>$
$戳一戳 <QQ号> <群号>$
```

### NapCat 机器人扩展功能
```nebula
# 获取信息
$群列表$
$获取好友列表$
$获取群信息 <群号>$

# 高级管理
$设置群头衔 <群号> <QQ号> <头衔>$
$设置群管理 <群号> <QQ号>$
$发送音乐卡片 <群号> <类型> <标题> <描述> <链接>$
```

### 飞书机器人
```nebula
# 发送图片消息
$图片 <类型> <图片数据>$
```

## 📚 Nebula 语言基础

### 文件分类
| 名称 | 目录 | 用途 |
|------|------|------|
| 储存目录 | `database/` | 数据读写存储 |
| 资源目录 | `private/` | 重要资源和词库 |
| 词库目录 | `public/` | 公开访问的词库 |

### 基础语法

#### JSON 数据处理
```nebula
JSON>赋予值={}
a=b
a->a=b
<JSON
```

#### 文本块处理
```nebula
文本>赋予值=\n
第
二
行
<文本
```

#### 变量和函数
```nebula
# 定义变量
变量:值

# 调用函数
$函数名 <参数1> <参数2>$

# 条件判断
如果:1==1
yes
否则
no
```

### 数据库操作
```nebula
连接:$打开sqlite <文件|:内存:> <默认值>$
$读sqlite <文件> <键> <默认值>$
$写sqlite <文件> <键> <值>$
```


### HTTP 请求处理
```nebula
# 获取请求参数
%GET.参数名%
%POST.参数名%

# 文件上传
%FILES.文件名%
```

## 🐳 Docker 部署详情

### 镜像构建
```bash
# 多阶段构建，生成轻量级镜像
docker-compose build --no-cache

# 或者直接构建
docker build -t nebula:latest .
```

### 容器运行
```bash
# 使用 Docker Compose
docker-compose up -d

# 直接运行
docker run -d \
  --name nebula \
  -p 8080:8080 \
  -v ./data:/app/NebulaData \
  -e TZ=Asia/Shanghai \
  nebula:latest
```

### 服务管理
```bash
# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f

# 进入容器
docker-compose exec nebula sh

# 停止服务
docker-compose down

# 清理所有资源
docker-compose down -v --rmi all
```

## 🔧 开发指南

### 环境要求
- **Go 1.26+**：核心开发语言
- **Docker**：容器化部署和开发
- **Git**：版本控制

### 构建项目
```bash
# 克隆项目
git clone https://github.com/cjxpj/nebula.git
cd nebula

# 使用 Go Workspace
go work use ./nebula

# 下载依赖
go mod download

# 构建 Windows 版本
cd nebula/app/win
go build -o nebulaApp.exe

# 构建 Linux 版本（Docker 用）
CGO_ENABLED=0 GOOS=linux go build -tags linux -ldflags="-s -w" -o nebula-app
```

### 扩展开发

#### 添加新的机器人平台
1. 在 `nebula/bot/` 下创建新的机器人目录
2. 实现消息路由和处理逻辑
3. 在 `dto/config.go` 中添加配置加载
4. 在 `dic/server.go` 中注册路由

#### 添加新的内置函数
1. 在 `nebula/dic/funcs/` 下添加功能模块
2. 实现函数逻辑并注册
3. 在 `dic/funcs.go` 中导入和注册

## ❓ 常见问题

### Q1: Windows 客户端启动失败？
**A**: 
1. 检查防火墙设置，允许程序访问网络
2. 以管理员身份运行
3. 查看程序日志排查具体错误

### Q2: Docker 部署无法访问？
**A**:
1. 检查 Docker 服务是否运行：`docker ps`
2. 验证端口是否被占用：`netstat -an | grep 8080`
3. 查看容器日志：`docker-compose logs`

### Q3: 机器人消息不回复？
**A**:
1. 检查配置文件中的 `启用` 选项是否为 `true`
2. 验证机器人密钥配置是否正确
3. 查看机器人词库文件是否存在

### Q4: 如何自定义词库？
**A**:
1. 在 `NebulaData/private/` 下创建词库文件（`.n` 后缀）
2. 在 `router.n` 中配置路由规则
3. 重启服务生效

## 👥 社区与支持

### 交流群组
- **QQ 群**：927467925
- **GitHub Discussions**：[项目讨论区](https://github.com/cjxpj/nebula/discussions)

### 代码仓库
- **GitHub**: https://github.com/cjxpj/nebula
- **Gitee**: https://gitee.com/cjxpj/nebula

### 问题反馈
1. GitHub Issues: https://github.com/cjxpj/nebula/issues
2. 提交详细的错误日志和复现步骤

## 📄 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

## 🙏 致谢

感谢所有贡献者和用户的支持！特别感谢：

- Go 语言社区提供的强大基础
- Docker 社区提供的容器化解决方案
- 各机器人平台提供的开放接口

---

**Nebula - 让 Web 服务开发更简单，更高效！** 🚀