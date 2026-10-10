package dic_server

import "net/http"

// HTTPS 证书 API。
func init() {
	registerOpuiApi(opuiHandleHTTPSAPI,
		"gen_https_cert", "import_https_cert", "list_system_certs", "extract_system_cert",
	)
}

// opuiHandleHTTPSAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleHTTPSAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	switch h.Type {
	case "gen_https_cert":
		opuiGenHttpsCert(w, r, h)
		return

	case "import_https_cert":
		opuiImportHttpsCert(w, r, h)
		return

	case "list_system_certs":
		opuiListSystemCerts(w, r, h)
		return

	case "extract_system_cert":
		opuiExtractSystemCert(w, r, h)
		return

	}
}
