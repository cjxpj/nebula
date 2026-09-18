---
name: API词库开发
description: API 词库（.n 被 HTTP 访问）的开发规范：以 Main 为触发词、读取 GET/POST 与访问数据、设置输出头部/响应状态/COOKIE。写或修改对外提供接口的 .n 词库时读取本技能。
---
【API词库开发（.n 被 HTTP 访问）】
目标：编写放在网站根目录（public/ 等）下、对外提供接口的 .n 词库——被 HTTP 访问时以 Main 为触发词执行，读取 GET / POST 数据并输出响应体。

一、执行模型
1. 请求由 system/router.n 路由：路径命中 .n 文件时执行 $执行词库文件 <网站根目录><访问路径> Main$，即**以 Main 为触发词**运行该词库的 Main 词条；
2. 也就是说执行入口只有 Main 这一个：只能用 Main 触发词执行，写别的触发词在被 HTTP 访问时永远不会被唤起（自己验证时也用 run_dic 传 trigger=Main）；所以 API 词库必须写 Main（与机器人功能词库刚好相反），Main 正文直接输出的内容就是 HTTP 响应体；
3. 最简范式（参考 public/api.n，注意空行分界）：
输出:%版本%

Main
%输出%
    —— 空行之前是头部（可放 $引入 ...$、赋值），空行之后是词条；Main 是触发词，其下是正文；
4. .n 与 .wn 的区别：路径为 .wn 时走网页词库（HTML + 执行块：<?n ... ?> 内联块 / <script type="nebula"> 脚本块，无触发词），路径为 .n 时走本条 Main 机制。

二、读取请求数据
1. $GET <键> <默认值>$：读查询参数（如 /api.n?name=abc 中的 name）；
2. $POST <键> <默认值>$：读表单参数或 JSON 体字段（JSON 请求体按对象解析，键不存在时返回默认值）；
3. $全局变量 访问数据$：整个请求信息的 JSON，字段为 路径 / 来源（GET、POST 等）/ GET / POST / 请求头 / IP / Host / POSTFile（上传文件）；嵌套取值写 %@访问数据.POST.键%，注意变量里不要用 %变量[键]% 这种写法；
4. POSTFile 为上传文件：字段名 → [{名称, 大小, 数据(base64)}]，需要落盘时自行解码写出。

三、设置响应
1. 响应头：$设置头部 <名称> <值>$（如 $设置头部 Content-Type application/json;charset=UTF-8$）；
2. 响应状态码：$全局变量 响应状态 404$（默认 200）；
3. 额外响应头：$全局变量 输出头部 {"Location":"https://cjxpj.com"}$（JSON 对象，输出时统一写入响应头）；
4. COOKIE：$全局变量 COOKIE %cookies%$，值为 JSON 数组，形如 [{"命名":"id","数据":"123456","路径":"/","禁止JS":false,"存活":3600}]（存活为秒，默认 0）；
5. 不要用 $全局变量 输出类型$ 设置内容类型（这条路径没有读取它），要设 Content-Type 就用 $设置头部$。

四、示例（JSON 接口 public/api.n）
$设置头部 Content-Type application/json;charset=UTF-8$
名字:$GET 名字 访客$

Main
{"状态":"ok","名字":"%名字%"}

五、跨域与另一套机制
1. 服务器开启跨域后会统一处理 CORS（含 OPTIONS 预检，允许 GET / POST / PUT / DELETE / OPTIONS），接口词库通常无需自己设置跨域头；
2. $创建服务器$ 创建的是「独立 HTTP 词库服务器」，那套机制以 URL 路径作为触发词（不是 Main），每个请求路径直接对应词条名，与 router.n 的 Main 机制不是一回事；
3. 与机器人词库的区别：$GET$ / $POST$ 只在被 HTTP 访问时有效，机器人词库读用户输入要用 %参数N% / %括号N%。

六、注意事项
1. 不要往 API 词库写菜单、娱乐触发词等面向用户消息的内容——它只会被 HTTP 访问，用户发消息不会触发它；
2. 函数名、参数个数与用法一律以内置文档为准：先用 search_docs 检索（直接返回命中小节的原文），不够再 read_dic_doc 读整篇；不确定就查证或向用户确认，不要写试探词条。
