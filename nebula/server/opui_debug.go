package dic_server

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	dic_dto "github.com/cjxpj/nebula/dic/dto"
	dic_funcs "github.com/cjxpj/nebula/dic/funcs"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

func toDataURI(data []byte) string {
	return "data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// resolveImgSrc 把 ±img= 的值解析为浏览器可直接显示的图片地址：
// http(s)/data: 原样返回；本地文件路径读取后转 data URI；纯 base64 图片数据也转 data URI
func resolveImgSrc(src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return src
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "data:") {
		return src
	}
	// 尝试作为本地文件（相对路径基于应用目录）
	if data, err := utils.NewFileQueue(src).ReadFile(); err == nil {
		return toDataURI([]byte(data))
	}
	// 尝试作为 base64 图片数据（绘图等函数输出的纯 base64 字符串）
	if dec, err := base64.StdEncoding.DecodeString(src); err == nil && len(dec) > 4 {
		if ct := http.DetectContentType(dec); strings.HasPrefix(ct, "image/") {
			return toDataURI(dec)
		}
	}
	return src
}

// isImageData 判断字节数据是否为常见图片格式（按文件头魔数识别）
func isImageData(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	ct := http.DetectContentType(data)
	return strings.HasPrefix(ct, "image/")
}

// imageMagics 常见图片格式的文件头魔数（用于在文本中扫描嵌入的图片字节）
var imageMagics = [][]byte{
	{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, // PNG
	{0xFF, 0xD8, 0xFF},                            // JPEG
	{'G', 'I', 'F', '8', '7', 'a'},                // GIF
	{'G', 'I', 'F', '8', '9', 'a'},                // GIF
	{0x00, 0x00, 0x01, 0x00},                      // ICO
	{'B', 'M'},                                    // BMP
}

// findImageStart 在字节流中查找第一个图片数据（文件头魔数）的起始位置，找不到返回 -1
func findImageStart(data []byte) int {
	for i := range len(data) {
		// WebP：RIFF + 4 字节长度 + WEBP
		if i+12 <= len(data) && data[i] == 'R' &&
			bytes.Equal(data[i:i+4], []byte("RIFF")) && bytes.Equal(data[i+8:i+12], []byte("WEBP")) {
			return i
		}
		for _, magic := range imageMagics {
			if i+len(magic) <= len(data) && bytes.Equal(data[i:i+len(magic)], magic) {
				// 短魔数（BMP 仅 2 字节）用内容嗅探二次确认，避免文本误报
				if len(magic) >= 4 || isImageData(data[i:]) {
					return i
				}
			}
		}
	}
	return -1
}

// findImageEnd 返回从 start 开始图片数据的结束位置（不含）。无法确定结束位置时返回 len(data)
func findImageEnd(data []byte, start int) int {
	rest := data[start:]
	switch {
	case bytes.HasPrefix(rest, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		// PNG 以 IEND chunk（00 00 00 00 49 45 4E 44 AE 42 60 82）结束
		if idx := bytes.Index(rest, []byte{0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}); idx >= 0 {
			return start + idx + 12
		}
	case bytes.HasPrefix(rest, []byte{0xFF, 0xD8, 0xFF}):
		// JPEG 以 FFD9 结束
		if idx := bytes.Index(rest, []byte{0xFF, 0xD9}); idx >= 0 {
			return start + idx + 2
		}
	case bytes.HasPrefix(rest, []byte("GIF87a")) || bytes.HasPrefix(rest, []byte("GIF89a")):
		// GIF 以 0x3B 结束
		if idx := bytes.IndexByte(rest, 0x3B); idx >= 0 {
			return start + idx + 1
		}
	case bytes.HasPrefix(rest, []byte("RIFF")) && len(rest) >= 12 && bytes.Equal(rest[8:12], []byte("WEBP")):
		// WebP：RIFF 头部 4~7 字节为整个文件长度（含 8 字节头）
		size := int(rest[4]) | int(rest[5])<<8 | int(rest[6])<<16 | int(rest[7])<<24
		if size >= 8 && len(rest) >= 8+size {
			return start + 8 + size
		}
	}
	return len(data)
}

// anyTypeName 返回变量值的类型名（中文，用于词库调试面板展示）
func anyTypeName(v any) string {
	switch v.(type) {
	case string:
		return "字符串"
	case bool:
		return "布尔"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return "数值"
	case time.Time:
		return "时间"
	case []byte:
		return "字节"
	case *dic_funcs.NDrawImg:
		return "画布"
	case nil:
		return "空"
	}
	// 兜底：按反射归类，避免常见类型（结构体、自定义切片/字典、指针等）显示为「未知」
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return "空"
		}
		return anyTypeName(rv.Elem().Interface())
	case reflect.Slice, reflect.Array:
		return "数组"
	case reflect.Map:
		return "字典"
	case reflect.Struct:
		return "对象"
	default:
		return "未知"
	}
}

// varDebugItem 将变量值转换为调试展示结构；类实例额外携带成员变量，供前端折叠展示
func varDebugItem(v any) map[string]any {
	return varDebugItemDepth(v, 0)
}

// varDebugItemDepth 递归构建变量调试结构，限制深度避免循环引用
func varDebugItemDepth(v any, depth int) map[string]any {
	// 类实例：展示为「类」类型，值用变量数量概括，成员变量放入 children 供前端折叠
	if cls, ok := v.(*dto.DicClass); ok {
		item := map[string]any{"t": "类", "v": "类实例"}
		if cls != nil && cls.LocalValue != nil {
			members := cls.LocalValue.GetAll()
			item["v"] = fmt.Sprintf("类实例（%d 个变量）", len(members))
			if depth < 3 && len(members) > 0 {
				children := make(map[string]any, len(members))
				for k, cv := range members {
					children[k] = varDebugItemDepth(cv, depth+1)
				}
				item["children"] = children
			}
		}
		return item
	}
	// 字符串：词库变量多为字符串存储，尝试智能识别类型（布尔/数值/JSON），便于调试展示
	if s, ok := v.(string); ok {
		return stringDebugItem(s, depth)
	}
	return map[string]any{
		"v": utils.AnyToString(v),
		"t": anyTypeName(v),
	}
}

// stringDebugItem 对字符串变量做类型识别：布尔/数值/JSON 对象/数组，其余按字符串展示
func stringDebugItem(s string, depth int) map[string]any {
	if s == "true" || s == "false" {
		return map[string]any{"v": s, "t": "布尔"}
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return map[string]any{"v": s, "t": "数值"}
	}
	if depth < 3 {
		if j := utils.IsJSONResult(s); j != nil {
			switch jv := j.(type) {
			case map[string]any:
				item := map[string]any{"v": s, "t": "对象"}
				if len(jv) > 0 {
					children := make(map[string]any, len(jv))
					for k, cv := range jv {
						children[k] = varDebugItemDepth(cv, depth+1)
					}
					item["children"] = children
				}
				return item
			case []any:
				item := map[string]any{"v": s, "t": "数组"}
				if len(jv) > 0 {
					children := make(map[string]any, len(jv))
					for i, cv := range jv {
						children[strconv.Itoa(i)] = varDebugItemDepth(cv, depth+1)
					}
					item["children"] = children
				}
				return item
			}
		}
	}
	return map[string]any{"v": s, "t": "字符串"}
}

// outputImgRe 匹配输出中的图片标记：±img=xxx± / <img src=...> / ![alt](url)
var outputImgRe = regexp.MustCompile(`±img=([^±]+)±|<img[^>]*\bsrc=["']([^"']+)["'][^>]*>|!\[[^\]]*\]\(([^)\s]+)\)`)

// parseOutputSegments 把词库输出中的图片标记解析为分段（文本/图片），
// 图片源解析为浏览器可直接显示的地址；同时自动识别直接输出的图片二进制数据
// （如 $画布.获取$ 返回的 PNG/JPEG 字节）
func parseOutputSegments(output string) []map[string]string {
	var segments []map[string]string

	// appendText 追加文本段，若该段本身是图片二进制或嵌入了图片字节则识别为图片
	appendText := func(text string) {
		if text == "" {
			return
		}
		// 整段本身就是图片二进制数据
		if isImageData([]byte(text)) {
			segments = append(segments, map[string]string{"type": "img", "src": toDataURI([]byte(text))})
			return
		}
		// 文本中嵌入图片字节（如 $画布.获取$ 输出带前缀文本）：按魔数拆分
		data := []byte(text)
		for {
			imgStart := findImageStart(data)
			if imgStart < 0 {
				if len(data) > 0 {
					segments = append(segments, map[string]string{"type": "text", "text": string(data)})
				}
				return
			}
			if imgStart > 0 {
				segments = append(segments, map[string]string{"type": "text", "text": string(data[:imgStart])})
			}
			imgEnd := findImageEnd(data, imgStart)
			segments = append(segments, map[string]string{"type": "img", "src": toDataURI(data[imgStart:imgEnd])})
			if imgEnd >= len(data) {
				return
			}
			data = data[imgEnd:]
		}
	}

	last := 0
	for _, m := range outputImgRe.FindAllStringSubmatchIndex(output, -1) {
		start, end := m[0], m[1]
		if start > last {
			appendText(output[last:start])
		}
		var src string
		switch {
		case m[2] >= 0: // ±img=xxx±
			src = output[m[2]:m[3]]
		case m[4] >= 0: // <img src="...">
			src = output[m[4]:m[5]]
		case m[6] >= 0: // ![alt](url)
			src = output[m[6]:m[7]]
		}
		segments = append(segments, map[string]string{"type": "img", "src": resolveImgSrc(src)})
		last = end
	}
	if last < len(output) {
		appendText(output[last:])
	}
	if len(segments) == 0 {
		appendText(output)
	}
	return segments
}

// dicRunResultPayload 汇总一次词库运行的输出、分段、错误行与变量快照。
// 手动运行（dic_debug_run）与 AI 工具运行（run_dic）共用，保证前端「运行结果」面板展示一致。
// fellBack=true（触发词未命中、走了线性脚本兜底）时，额外附一条黄色警告诊断。
func dicRunResultPayload(dic *dic_dto.Dic, output string, timedOut bool, fellBack bool, trigger string) map[string]any {
	pVars := make(map[string]any)
	for k, v := range dic.Val.P.GetAll() {
		pVars[k] = varDebugItem(v)
	}
	gVars := make(map[string]any)
	for k, v := range dic.Val.G.GetAll() {
		gVars[k] = varDebugItem(v)
	}
	gvVars := make(map[string]any)
	for k, v := range dto.GV.GetAll() {
		gvVars[k] = varDebugItem(v)
	}
	resp := map[string]any{
		"output":   output,
		"timedOut": timedOut,
		"segments": parseOutputSegments(output),
		"vars": map[string]any{
			"P":  pVars,
			"G":  gVars,
			"GV": gvVars,
		},
	}
	// 从输出中提取错误行号（格式：funcName(line:N)：error 或 JS错误(line:N)：error）
	if re := regexp.MustCompile(`\(line:(\d+)\)`); re != nil {
		if m := re.FindStringSubmatch(output); len(m) >= 2 {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				resp["errorLine"] = n
			}
		}
	}
	warnings := dic.Data.Warnings
	if fellBack {
		// 用新切片追加，避免就地扩容污染 dic.Data.Warnings
		warnings = append(append([]dto.BuildWarning{}, dic.Data.Warnings...), dicTriggerMissWarning(dic, trigger))
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	return resp
}

// dicTriggerMissWarning 触发词未命中时生成一条黄色警告。
// 改为警告诊断后，前端会在对应行整行标黄，悬浮气泡里给出原因。
func dicTriggerMissWarning(dic *dic_dto.Dic, trigger string) dto.BuildWarning {
	return dto.BuildWarning{
		Line:  dicTriggerMissLine(dic),
		Text:  dicTriggerMissHint(trigger),
		Level: "warning",
	}
}

// dicTriggerMissLine 把兜底警告定位到首个词条的触发词行；没有词条时退回第 1 行。
// 头部已作为特殊触发词词块并入 Dic 列表（TriggerLine 为 0），需跳过它取首个真实词条。
func dicTriggerMissLine(dic *dic_dto.Dic) int {
	if dic != nil && dic.Data != nil {
		for _, e := range dic.Data.Dic {
			if e == nil || e.Trigger == dto.HeaderTrigger {
				continue
			}
			if e.TriggerLine > 0 {
				return e.TriggerLine
			}
		}
	}
	return 1
}

// dicTriggerMissHint 触发词未命中任何词条时的提示文案。
// 未命中时只跑头部（初始化代码）、不跑正文，也不会把触发词行当正文输出。
// 手动运行与 AI 工具运行共用同一文案。
func dicTriggerMissHint(trigger string) string {
	if strings.TrimSpace(trigger) == "" {
		return "触发词为空，未命中任何词条；如需执行某个词条，请在运行配置里把「触发文本」填成该词条的触发词。"
	}
	return "触发词「" + trigger + "」未命中任何词条；如需执行某个词条，请在运行配置里把「触发文本」改成该词条的触发词。"
}

// outputImages 从词库输出中提取图片地址（±img= / <img> / ![]() 标记与直接输出的图片二进制），
// 返回可直接用于多模态上传的地址（data URI 或 http(s) URL）。无图片时返回 nil。
func outputImages(output string) []string {
	var imgs []string
	for _, seg := range parseOutputSegments(output) {
		if seg["type"] == "img" && seg["src"] != "" {
			imgs = append(imgs, seg["src"])
		}
	}
	return imgs
}

// aiVisionUserMessage 构造携带图片的多模态 user 消息：文本 + image_url 分段数组。
func aiVisionUserMessage(text string, images []string) aiChatMessage {
	parts := make([]map[string]any, 0, len(images)+1)
	if strings.TrimSpace(text) != "" {
		parts = append(parts, map[string]any{"type": "text", "text": text})
	}
	for _, src := range images {
		parts = append(parts, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": src},
		})
	}
	return aiChatMessage{Role: "user", Content: parts}
}

// aiVisionDisabledHint 视觉能力未开启、本次运行又输出过图片时的提示文案。
// 同时用于回灌给模型（让它知道自己看不到画面内容、不要凭空臆测）与前端面板黄色警告；
// 文案里点出「已附量化数据 + 可用 view_image 补看」，避免模型以为对图片一无所知。
func aiVisionDisabledHint(n int) string {
	return fmt.Sprintf("本次运行输出中包含 %d 张图片，但当前模型的「AI 视觉能力」未开启，看不到图片的画面内容（已附尺寸、格式与颜色等量化数据，也可用 view_image 工具补看）；如需让 AI 真正看到画面，请在「基础配置 → AI」中为当前模型开启视觉能力后重试。", n)
}

// loadBlockDefaults 读取合并配置中 [积木编程] 节的配置。
// 积木编程页直接以 .n 词库为编辑对象（无独立的积木工程文件），
// 「打开的词库标签」在本页独立记录，与其它开发工具页共用的 [词库调试] 标签互不影响。
