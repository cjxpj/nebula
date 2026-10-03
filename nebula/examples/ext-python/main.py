"""独立进程扩展示例（Python）。

主程序按同目录下的 plugin.json 启动本程序，并通过环境变量 NEBULA_PLUGIN_PORT
注入监听端口。本程序监听该端口，用「换行分隔的 JSON」协议与主程序通信（每行一条消息）：

    请求：{"id":1,"type":"list"}
    响应：{"id":1,"type":"list","funcs":[{"name":"Py示例加法","l":"2"}]}
    请求：{"id":2,"type":"call","fn":"Py示例加法","args":["1","2"]}
    响应：{"id":2,"type":"call","result":"3"}
    请求：{"id":3,"type":"call","fn":"Py示例报错","args":[]}
    响应：{"id":3,"type":"call","error":"这是一个测试错误"}
    请求：{"id":0,"type":"close"}

主程序对单个扩展串行发请求（收到上一条响应才发下一条），本程序无需处理并发。

用法：把本目录整体复制到 <数据目录>/private/plugins/ 下，重启主程序即可。
"""

import json
import os
import socket
import sys

FUNCS = [
    {"name": "Py示例测试", "l": "0"},
    {"name": "Py示例回显", "l": "1"},
    {"name": "Py示例加法", "l": "2"},
    {"name": "Py示例报错", "l": "0"},
]


def call(name, args):
    """分发并执行具体的扩展函数，返回结果字符串；出错时抛异常。"""
    if name == "Py示例测试":
        return "独立进程扩展加载成功（Python）"
    if name == "Py示例回显":
        return arg(args, 0)
    if name == "Py示例加法":
        return "%g" % (to_float(arg(args, 0)) + to_float(arg(args, 1)))
    if name == "Py示例报错":
        raise ValueError("这是一个测试错误")
    raise ValueError("未知函数 %s" % name)


def arg(args, i):
    """取第 i 个参数，越界返回空串。"""
    return args[i] if 0 <= i < len(args) else ""


def to_float(s):
    """宽松地把字符串解析为浮点数，失败按 0 处理。"""
    try:
        return float(s.strip())
    except (TypeError, ValueError):
        return 0.0


def handle(req):
    """处理一条请求，返回响应字典。"""
    resp = {"id": req.get("id", 0), "type": req.get("type", "")}
    if resp["type"] == "list":
        resp["funcs"] = FUNCS
    elif resp["type"] == "call":
        try:
            resp["result"] = call(req.get("fn", ""), req.get("args") or [])
        except Exception as exc:  # noqa: BLE001 扩展侧错误一律透传为主程序错误
            resp["error"] = str(exc)
    return resp


def main():
    port = os.environ.get("NEBULA_PLUGIN_PORT")
    if not port:
        print("缺少环境变量 NEBULA_PLUGIN_PORT", file=sys.stderr)
        sys.exit(1)

    srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind(("127.0.0.1", int(port)))
    srv.listen(1)

    conn, _ = srv.accept()
    reader = conn.makefile("r", encoding="utf-8", newline="\n")
    try:
        while True:
            line = reader.readline()
            if not line:
                break  # 主程序关闭了连接
            req = json.loads(line)
            if req.get("type") == "close":
                break
            data = json.dumps(handle(req), ensure_ascii=False)
            conn.sendall((data + "\n").encode("utf-8"))
    finally:
        reader.close()
        conn.close()
        srv.close()


if __name__ == "__main__":
    main()
