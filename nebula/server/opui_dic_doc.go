package dic_server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/cjxpj/nebula/run"
)

// 词库文档 / 缓存 API。
func init() {
	registerOpuiApi(opuiHandleDicDocAPI,
		"list_dic_cache", "remove_dic_cache", "clear_dic_cache", "get_dic_doc_list",
		"get_dic_doc", "search_dic_doc", "download_dic_docs",
	)
}

// opuiHandleDicDocAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleDicDocAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "list_dic_cache":
		// 词库编译缓存列表（private/.dic_cache）：供「清理」页展示并逐条清理
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "items": run.ListDicCache()})
		w.Write(jsonResp)
		return

	case "remove_dic_cache":
		// 清理单条词库编译缓存（缓存下次加载词库时会自动重建）
		var j struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(h.Data, &j)
		if err := run.RemoveDicCache(j.Name); err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "clear_dic_cache":
		// 清空全部词库编译缓存
		run.ClearDicCache()
		w.Write([]byte(`{"status":"ok"}`))
		return

	case "get_dic_doc_list":
		// 内置说明文档清单（分篇 md，见 server/dic_doc.go）：供文档页左侧目录树使用
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "list": dicDocList()})
		w.Write(jsonResp)
		return

	case "get_dic_doc":
		var j struct {
			Doc string `json:"doc"`
		}
		_ = json.Unmarshal(h.Data, &j)
		doc, err := dicDocResolveOrFirst(j.Doc)
		if err != nil {
			http.Error(w, `{"status":"error","error":"embedded file not found"}`, http.StatusInternalServerError)
			return
		}
		html := dicDocRenderMarkdown(doc.content)
		resp := map[string]any{"content": string(html), "path": doc.Path, "title": doc.Title, "group": doc.Group}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "search_dic_doc":
		// 跨篇搜索内置文档（见 server/dic_doc.go）：说明文档已拆成多篇，
		// 搜索必须覆盖全部篇章，否则只能搜到当前篇，其它篇的内容就搜不到。
		var j struct {
			Keyword string `json:"keyword"`
		}
		_ = json.Unmarshal(h.Data, &j)
		jsonResp, _ := json.Marshal(map[string]any{"status": "ok", "hits": dicDocSearch(j.Keyword)})
		w.Write(jsonResp)
		return

	case "download_dic_docs":
		// 打包全部内置文档为 zip 供下载（见 server/dic_doc.go）。
		// WS 通道只传文本帧，二进制用 base64 承载（文档总共百来 KB，开销可忽略）。
		data, err := dicDocsZip()
		if err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		jsonResp, _ := json.Marshal(map[string]any{
			"status":  "ok",
			"name":    "nebula-docs.zip",
			"content": base64.StdEncoding.EncodeToString(data),
		})
		w.Write(jsonResp)
		return

	}
}
