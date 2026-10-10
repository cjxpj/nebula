package dic_server

import "net/http"

// opuiApiHandler 处理其被注册的那批 OPUI API 类型（请求体的 type 字段）。
// 约定：handler 只处理「自己注册过的类型」，因此总是会写响应，不需要返回值。
type opuiApiHandler func(w http.ResponseWriter, r *http.Request, h *HttpOpUiData)

// opuiApiHandlers 是 OPUI API 分派表：type -> handler。
// 各业务模块在自己的文件里用 init() + registerOpuiApi 把 handler 插入进来，
// opui.go 的 opuiHandleApi 只负责鉴权与查表分派，不再维护巨型 switch。
var opuiApiHandlers = map[string]opuiApiHandler{}

// registerOpuiApi 把同一个 handler 注册到多个 API 类型上（模块化插入入口）。
// 重复注册同一 type 时后注册者覆盖先注册者。
func registerOpuiApi(fn opuiApiHandler, types ...string) {
	for _, t := range types {
		opuiApiHandlers[t] = fn
	}
}
