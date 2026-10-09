package dic

import (
	"bytes"
	"html/template"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/run"
	"golang.org/x/net/html"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.Config{
	EscapeHTML: false, // 禁用 HTML 转义
}.Froze()

// initSnapshot [f]_初始化 执行一次的结果快照：输出文本 + 执行后产生的局部变量表。
// 命中缓存时必须连同变量一起恢复：词库每次执行都是新的局部变量表（P），
// 只跳过初始化不恢复变量会让其定义的变量在后续执行里全部消失（静默输出空值）。
type initSnapshot struct {
	out  string
	vars map[string]any
	halt bool // 初始化里执行了 >终止（全局终止），命中缓存时需同样终止本次执行
}

// initOutputCache 缓存各词库 [f]_初始化 的执行结果，实现「首次加载时执行一次」（类似 Go init / Lua require）：
// 首次执行并缓存「输出 + 局部变量快照」，后续执行跳过初始化（副作用不再重复），
// 仅复用输出并把变量快照恢复到本次执行的变量表中。
// key 为词库依赖指纹（Deps 的依赖路径 + 内容 hash 排序拼接），内容变化自动失效。
var initOutputCache sync.Map // map[string]initSnapshot

// triggerIdxKey 记录当前触发词命中的正文词条下标（0-based），供 $继续执行$ 从下一个词条继续级联匹配。
const triggerIdxKey = "_触发词下标_"

type scriptNebula struct {
	Id   string `json:"id"`
	Text string `json:"text"`
}

func isNebulaScript(n *html.Node) bool {
	if n.Type != html.ElementNode || n.Data != "script" {
		return false
	}

	for _, a := range n.Attr {
		if a.Key == "type" && a.Val == "nebula" {
			return true
		}
	}
	return false
}

func removeNebulaScripts(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling

		if isNebulaScript(c) {
			n.RemoveChild(c)
		} else {
			removeNebulaScripts(c)
		}

		c = next
	}
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func extractText(n *html.Node) string {
	var s strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			s.WriteString(c.Data)
		}
	}
	return s.String()
}

// 在 html 或 body 直接子树中查找 <script type="nebula">
func findNebulaScripts(doc *html.Node) []scriptNebula {
	var result []scriptNebula

	// 找 html
	var htmlNode *html.Node
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "html" {
			htmlNode = c
			break
		}
	}
	if htmlNode == nil {
		return result
	}

	// 扫描 head 和 body
	for c := htmlNode.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}

		if c.Data == "head" || c.Data == "body" {
			for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
				if cc.Type == html.ElementNode &&
					cc.Data == "script" &&
					isNebulaScript(cc) {

					result = append(result, scriptNebula{
						Id:   getAttr(cc, "id"),
						Text: strings.TrimSpace(extractText(cc)),
					})
				}
			}
		}
	}

	return result
}

// normalizeWebDicNewlines 把网页词库文本的换行符统一为 \n。
// Windows 下 .wn 文件为 CRLF，而 <?n ... ?> 内联块在 html.Parse 之前就按 \n 切分执行，
// 行尾残留的 \r 会破坏「循环>i=10」这类次数解析（strconv.Atoi("10\r") 失败、循环退化为 1 次），
// 也会让「<循环」等关闭标记匹配不上。统一换行符后与 HTTP 链路 ReadFromFile 的行为一致。
func normalizeWebDicNewlines(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func (m *dicImpl) WebPHPDicRun(WD *dic_dto.WebDic) string {

	dicRun := dic_dto.NewRunDicEntry().
		SetV(WD.Val)

	// 按账号会员状态限时（会员 1 分钟 / 非会员 10 秒），超时置停止标志中断并返回已产出内容。
	result, _ := m.withDicTimeout(WD.Path, WD.Val.G, DefaultDicRunTimeout, func() {
		dicRun.Sys_v.Stop.Store(true)
	}, func() string {
		return run.ReplaceProcessedContent(normalizeWebDicNewlines(WD.Text), "<?n", "?>", func(text string) string {
			// fmt.Println("词库文本:", text)
			// 词条总数据
			lines := strings.Split(text, "\n")
			SplitText := run.Web(WD.Path, lines)
			dicRun.SetDic(SplitText)
			dicRun.Dic.MyFunc = WD.MyFunc
			maps.Copy(dicRun.Dic.MyFunc, SplitText.MyFunc)
			// fmt.Println("词库:", SplitText)
			if head := SplitText.HeaderEntry(); head != nil {
				return m.DicRunLine(dicRun, head.Text)
			}
			return ""
		})
	})

	return result
}

// 运行网页词库：按账号会员状态限时（会员 1 分钟 / 非会员 10 秒），超时置各执行块停止标志中断并返回已产出内容。
func (m *dicImpl) WebDicRun(WD *dic_dto.WebDic) string {

	// 每个执行块独立作用域：私有变量区（P）各块隔离、块间不接力；
	// 全局变量区（G）跨块共享，承载 响应状态 / 输出头部 / GET / POST 等页面级上下文。
	// <script type="nebula" id="x"> 脚本块把整块输出存进 data[x]，页面用 {{.x}} 取；
	// 不带 id 的脚本块里赋值的变量并入模板数据，页面用 {{.变量名}} 取；
	// 内联块就地输出，其赋值不进模板数据。
	var (
		blockMu   sync.Mutex
		blockRuns []*dic_dto.DicEntry
	)
	newBlockRun := func() *dic_dto.DicEntry {
		// 挂载调用方注入的内置函数（HTTP 链路的 设置头部 / GET / POST，本地调试注入的空实现）：
		// 不挂载时脚本里的 $GET$ / $设置头部$ 会解析不到函数、被当成普通文本原样输出。
		run := dic_dto.NewRunDicEntry().SetGlobal_v(WD.Val.G)
		run.Dic.MyFunc = WD.MyFunc
		// 登记执行块，供超时回调统一置停止标志：各块 Sys_v 独立，需逐个中断。
		blockMu.Lock()
		blockRuns = append(blockRuns, run)
		blockMu.Unlock()
		return run
	}

	result, _ := m.withDicTimeout(WD.Path, WD.Val.G, DefaultDicRunTimeout, func() {
		blockMu.Lock()
		defer blockMu.Unlock()
		for _, r := range blockRuns {
			r.Sys_v.Stop.Store(true)
		}
	}, func() string {
		// 返回数据
		var result string

		data := make(map[string]any)

		// 1. 先处理内联执行块 <?n ... ?>，就地执行并把结果插回块所在位置。
		// 必须在 html.Parse 之前按原文处理：Go 的 html 解析器会把 <?...> 当成注释节点吞掉，
		// 解析后再找就找不到这个块了。结果按原文插回，因此块内可以直接输出 HTML 片段。
		// 先统一换行符，避免 CRLF 的 \r 残留破坏块内循环次数/关闭标记解析。
		src := run.ReplaceProcessedContent(normalizeWebDicNewlines(WD.Text), "<?n", "?>", func(block string) string {
			// 与脚本块一致：先裁掉首尾空白（块内首行是空行时会被当成「头部结束」，语句不会执行）
			lines := run.TrimWebScriptIndent(strings.Split(strings.TrimSpace(block), "\n"))
			return m.DicRunLine(newBlockRun(), lines)
		})

		// 解析成节点树
		doc, err := html.Parse(strings.NewReader(src))
		if err != nil {
			debugLog.Error(err)
		}

		// 2. 执行 nebula script，收集数据
		for _, s := range findNebulaScripts(doc) {
			// 脚本块正文可能带缩进（格式化后的网页词库），先去掉行首空白再解析
			lines := run.TrimWebScriptIndent(strings.Split(s.Text, "\n"))
			run := newBlockRun()
			res := m.DicRunLine(run, lines)
			if s.Id != "" {
				data[s.Id] = res
			} else {
				maps.Copy(data, run.Val.P.GetAll())
			}
		}

		// 3. 只移除 type="nebula" 的 script
		removeNebulaScripts(doc)

		var htmlBuf bytes.Buffer
		html.Render(&htmlBuf, doc)

		// 4. 使用 Go 模板引擎渲染
		tpl, err := template.New("page").Parse(htmlBuf.String())
		if err != nil {
			debugLog.Infof("模板解析失败: %v", err)
		}
		if err == nil {
			var buf bytes.Buffer
			if err := tpl.Execute(&buf, data); err != nil {
				debugLog.Infof("模板渲染失败: %v", err)
			}
			// 5. 模板渲染结果
			result = buf.String()
		}

		return result
	})

	return result
}

// 运行内部
func (m *dicImpl) DicRunPrivate(D *dic_dto.Dic, trigger string) string {
	newV := dto.NewDicVal()
	newV.G = D.Val.G
	return m.DicRunPrivateVal(D, trigger, newV)
}

// 运行内部-自义定局部变量：按账号会员状态限时（会员 1 分钟 / 非会员 10 秒），超时中断并返回已产出内容。
func (m *dicImpl) DicRunPrivateVal(D *dic_dto.Dic, trigger string, v *dto.DicVal) string {

	D.Val.G.SetRaw("_词库路径_", D.Path)

	D.Data.MergeFuncs(D.FuncText)

	if D.ClassText != nil {
		maps.Copy(D.Data.Class, D.ClassText)
	}

	GetDic, GetDicTrigger, _, _ := run.RunFor(D.Data.DicFuncs["内部"], trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)

	dicRun := dic_dto.NewRunDicEntry().
		SetV(D.Val).
		SetDic(D.Data)
	dicRun.Dic.MyFunc = D.MyFunc

	// 注入编译期资源变量（//@资源），供内部函数引用
	D.Data.ApplyResources(D.Val)

	result, _ := m.withDicTimeout(D.Path, D.Val.G, DefaultDicRunTimeout, func() {
		dicRun.Sys_v.Stop.Store(true)
	}, func() string {
		return m.DicRunLine(dicRun, GetDic)
	})

	return result
}

// 运行特殊触发
func (m *dicImpl) DicRunEvent(D *dic_dto.Dic, event string, trigger string) string {
	newV := dto.NewDicVal()
	newV.G = D.Val.G
	return m.DicRunEventVal(D, event, trigger, newV)
}

// 运行特殊触发-自义定局部变量：按账号会员状态限时（会员 1 分钟 / 非会员 10 秒），超时中断并返回已产出内容。
func (m *dicImpl) DicRunEventVal(D *dic_dto.Dic, event string, trigger string, v *dto.DicVal) string {

	D.Val.G.SetRaw("_词库路径_", D.Path)

	D.Data.MergeFuncs(D.FuncText)

	if D.ClassText != nil {
		maps.Copy(D.Data.Class, D.ClassText)
	}

	var (
		GetDic        []string
		GetDicTrigger string
	)
	GetDic, GetDicTrigger, _, _ = run.RunFor(D.Data.DicFuncs[event], trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)

	dicRun := dic_dto.NewRunDicEntry().
		SetV(D.Val).
		SetDic(D.Data)
	dicRun.Dic.MyFunc = D.MyFunc

	// 注入编译期资源变量（//@资源），供特殊事件引用
	D.Data.ApplyResources(D.Val)

	result, _ := m.withDicTimeout(D.Path, D.Val.G, DefaultDicRunTimeout, func() {
		dicRun.Sys_v.Stop.Store(true)
	}, func() string {
		return m.DicRunLine(dicRun, GetDic)
	})

	return result
}

// 新建运行
func (m *dicImpl) NewDicRunLine(D *dic_dto.DicEntry, txt []string) string {
	D.Set_v(dto.NewVal())
	return m.DicRunLine(D, txt)
}

// DefaultDicRunTimeout 词库执行的默认超时上限（会员账号，及未注入解析器时）：1 分钟。
// 超时后中断执行并返回已产出内容。
const DefaultDicRunTimeout = time.Minute

// MemberDicRunTimeout 会员账号词库执行的超时上限：1 分钟。
const MemberDicRunTimeout = DefaultDicRunTimeout

// GuestDicRunTimeout 非会员账号词库执行的超时上限：10 秒。
const GuestDicRunTimeout = 10 * time.Second

// DicRunTimeoutResolver 按词库路径解析该账号可用的词库执行超时上限。
// 由引擎 app 层注入（依据账号会员到期时间返回 MemberDicRunTimeout / GuestDicRunTimeout）；
// 未注入或返回 0 时按 DefaultDicRunTimeout 处理（保持既有行为）。
var DicRunTimeoutResolver func(path string) time.Duration

// SetDicRunTimeoutResolver 注入按词库路径解析账号超时上限的解析器。
func SetDicRunTimeoutResolver(fn func(path string) time.Duration) {
	DicRunTimeoutResolver = fn
}

// dicRunTimeoutLimit 返回该词库路径对应的超时上限；无解析器或不适用时回退默认值。
func dicRunTimeoutLimit(path string) time.Duration {
	if DicRunTimeoutResolver != nil {
		if d := DicRunTimeoutResolver(path); d > 0 {
			return d
		}
	}
	return DefaultDicRunTimeout
}

// clampDicRunTimeout 把请求的超时夹到该账号的上限内：timeout<=0 或超过上限时取上限。
func clampDicRunTimeout(path string, timeout time.Duration) time.Duration {
	limit := dicRunTimeoutLimit(path)
	if timeout <= 0 || timeout > limit {
		return limit
	}
	return timeout
}

// dicRunTimeoutMarker 记录「当前词库实例正在执行且已由最外层计时」的标记，存放于全局变量区 G。
// $执行词库$ / $网页词库$ 等嵌套执行共享外层的 G，据此识别为嵌套执行、直接同步执行不重复计时，
// 从而让整条执行链（含嵌套）共用同一个 1 分钟上限。
const dicRunTimeoutMarker = "_词库执行超时中_"

// isNestedDicRun 判断当前执行是否为嵌套执行（G 上已存在最外层设置的执行标记）。
func isNestedDicRun(G *dto.Val) bool {
	if G == nil {
		return false
	}
	marked, _ := G.GetRaw(dicRunTimeoutMarker)
	return marked == true
}

// withDicTimeout 以统一超时运行一次词库执行：
//   - 嵌套执行（G 上已有标记）：直接同步运行，不重复计时；
//   - 顶层执行：在 G 上打标记并计时，超时调用 stop 置停止标志尽快打断执行，返回已产出结果。
//
// timeout <=0 或超过该账号上限（会员 1 分钟 / 非会员 10 秒）时按上限计；
// stop 为 nil 表示无法主动打断（超时后仅返回已产出结果）。
func (m *dicImpl) withDicTimeout(path string, G *dto.Val, timeout time.Duration, stop func(), run func() string) (result string, timedOut bool) {
	if isNestedDicRun(G) {
		return run(), false
	}
	timeout = clampDicRunTimeout(path, timeout)
	G.SetRaw(dicRunTimeoutMarker, true)
	defer G.SetRaw(dicRunTimeoutMarker, false)

	type outcome struct{ text string }
	done := make(chan outcome, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				debugLog.Errorf("词库执行 panic: %v", r)
				done <- outcome{}
			}
		}()
		done <- outcome{run()}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case o := <-done:
		return o.text, false
	case <-timer.C:
		// 超时：置停止标志，引擎会在行间检查时尽快退出
		if stop != nil {
			stop()
		}
		// 给引擎短暂宽限，尽量在返回前安全退出，避免并发访问已释放的变量
		select {
		case o := <-done:
			return o.text, true
		case <-time.After(3 * time.Second):
			return "", true
		}
	}
}

// prepareDicRun 组装一次词库执行实例：注入词库路径、合并函数/类、按触发词匹配正文词条。
func (m *dicImpl) prepareDicRun(D *dic_dto.Dic, trigger string) *dic_dto.DicEntry {
	D.Val.G.SetRaw("_词库路径_", D.Path)

	D.Data.MergeFuncs(D.FuncText)

	if D.ClassText != nil {
		maps.Copy(D.Data.Class, D.ClassText)
	}

	// 正文词块按触发词匹配。
	_, GetDicTrigger, triggerIdx := run.RunForIndexed(D.Data.GetTriggerIndex(), D.Data.Dic, trigger, 0)
	D.Val.P.Set("触发词", trigger)
	D.Val.P.Set("触发", GetDicTrigger)
	D.Val.P.SetRaw(triggerIdxKey, triggerIdx)

	dicRun := dic_dto.NewRunDicEntry().
		SetV(D.Val).
		SetDic(D.Data)
	dicRun.Dic.MyFunc = D.MyFunc

	// 注入编译期资源变量（//@资源），供初始化/中间件与正文引用
	D.Data.ApplyResources(D.Val)

	return dicRun
}

// dicRunBody 在已准备的执行实例上按生命周期执行正文：
// [f]_初始化（首次加载只一次）→ 中间件（每次）→ 正文词条。
func (m *dicImpl) dicRunBody(dicRun *dic_dto.DicEntry, D *dic_dto.Dic) string {
	DicText := D.Data.Dic

	RunInit := m.runInitOnce(dicRun, D.Data, D.Path)

	RunMiddleware := m.runMiddleware(dicRun, D.Data)

	RunDic := ""
	if !dicRun.Sys_v.Stop.Load() {
		// 中间件可能通过 $重定向触发词$ 或变量改写修改了触发词，重新匹配一次再执行正文
		GetDic, GetDicTrigger, triggerIdx := run.RunForIndexed(D.Data.GetTriggerIndex(), DicText, D.Val.P.GetStr("触发词"), 0)
		D.Val.P.Set("触发", GetDicTrigger)
		D.Val.P.SetRaw(triggerIdxKey, triggerIdx)
		// 设置 body 行号映射（仅当触发器匹配时）
		if GetDic != nil && triggerIdx < len(DicText) {
			dicRun.LineNums = DicText[triggerIdx].LineNums
		}
		RunDic = m.DicRunLine(dicRun, GetDic)
	}

	return RunInit + RunMiddleware + RunDic
}

// 运行词库(全局变量,词库文本,触发)：按账号会员状态限时（会员 1 分钟 / 非会员 10 秒），超时中断并返回已产出内容。
func (m *dicImpl) DicRun(D *dic_dto.Dic, trigger string) string {
	dicRun := m.prepareDicRun(D, trigger)

	result, _ := m.withDicTimeout(D.Path, D.Val.G, DefaultDicRunTimeout, func() {
		dicRun.Sys_v.Stop.Store(true)
	}, func() string {
		return m.dicRunBody(dicRun, D)
	})

	dicRun.Close()

	return result
}

// runInitOnce 执行 [f]_初始化 生命周期钩子：首次加载该词库内容时执行一次并缓存「输出 + 变量快照」，
// 后续执行跳过初始化（副作用不再重复）、复用输出并把变量快照恢复到本次的局部变量表，
// 保证正文仍能读到初始化定义的变量。未定义 _初始化 时返回空串。
func (m *dicImpl) runInitOnce(dicRun *dic_dto.DicEntry, data *dto.BuildValue, path string) string {
	entry := data.LifecycleFunc(dto.InitTrigger)
	if entry == nil {
		return ""
	}
	key := dicFingerprint(data, path)
	if v, ok := initOutputCache.Load(key); ok {
		snap := v.(initSnapshot)
		// 快照值按次深拷贝：正文对变量的改动不回流到快照，避免跨次执行互相污染。
		for k, val := range cloneVars(snap.vars) {
			// 本次执行已有该变量（框架写入的 触发词/触发，或调用方按次注入的 类型/账号 等）时保留现值，
			// 否则会把首次执行时的旧值盖回去。
			if dicRun.Val.P.Has(k) {
				continue
			}
			dicRun.Val.P.Set(k, val)
		}
		if snap.halt {
			dicRun.Sys_v.Stop.Store(true)
		}
		return snap.out
	}

	dicRun.LineNums = entry.LineNums
	out := m.DicRunLine(dicRun, entry.Text)

	// 用 Clone 取快照：初始化变量后续会被中间件/正文改动，直接存引用会让快照跟着漂移。
	initOutputCache.Store(key, initSnapshot{out: out, vars: dicRun.Val.P.Clone().GetAll(), halt: dicRun.Sys_v.Stop.Load()})
	return out
}

// runMiddleware 执行 [f]_中间件 生命周期钩子：每次执行都会经过。
// 头部 与 [f]_中间件 二选一，已定义 _中间件 时头部被覆盖（编译期已忽略头部运行时语句），
// 这里只需执行 _中间件 函数即可；未定义时返回空串。
func (m *dicImpl) runMiddleware(dicRun *dic_dto.DicEntry, data *dto.BuildValue) string {
	entry := data.LifecycleFunc(dto.MiddlewareTrigger)
	if entry == nil {
		return ""
	}
	dicRun.LineNums = entry.LineNums
	data.InHeader = true
	out := m.DicRunLine(dicRun, entry.Text)
	data.InHeader = false
	return out
}

// cloneVars 深拷贝变量快照（借助 Val.Clone 的 deepCopyAny），避免复用缓存时多个执行实例共享同一批可变值。
func cloneVars(vars map[string]any) map[string]any {
	if len(vars) == 0 {
		return nil
	}
	tmp := dto.NewVal().Reset(vars)
	return tmp.Clone().GetAll()
}

// dicFingerprint 由词库依赖指纹生成缓存 key：用 Deps（依赖路径 + 内容 hash）排序拼接，
// 保证词库内容变化后缓存自动失效；无依赖信息时回退到词库路径。
func dicFingerprint(data *dto.BuildValue, path string) string {
	if data == nil || len(data.Deps) == 0 {
		return path
	}
	keys := make([]string, 0, len(data.Deps))
	for k := range data.Deps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(data.Deps[k])
		b.WriteByte(';')
	}
	return b.String()
}

// 运行词库（带超时）：超过 timeout 后置停止标志强行打断执行，返回当前已产出结果。
// timeout <=0 或超过 DefaultDicRunTimeout 时统一按 DefaultDicRunTimeout（1 分钟）计。
func (m *dicImpl) DicRunTimeout(D *dic_dto.Dic, trigger string, timeout time.Duration) (result string, timedOut bool) {
	dicRun := m.prepareDicRun(D, trigger)

	result, timedOut = m.withDicTimeout(D.Path, D.Val.G, timeout, func() {
		dicRun.Sys_v.Stop.Store(true)
	}, func() string {
		return m.dicRunBody(dicRun, D)
	})

	dicRun.Close()

	return result, timedOut
}

// DicRunScript 执行词库，带「无触发词兜底」：始终走正常执行路径——
// 触发词命中时跑头部 + 命中正文；未命中时只跑头部（初始化代码）、不跑正文，也不会把触发词行当文本输出。
// 与 DicRun/DicRunTimeout 的唯一区别是多返回 fellBack：触发词未命中任何词条时为 true，
// 调用方据此提示用户 / AI「触发词未命中」。
func (m *dicImpl) DicRunScript(D *dic_dto.Dic, trigger string, timeout time.Duration) (result string, timedOut bool, fellBack bool) {
	_, matchedTrigger, _, _ := run.RunFor(D.Data.Dic, trigger, 0)
	fellBack = matchedTrigger == ""
	if timeout > 0 {
		r, t := m.DicRunTimeout(D, trigger, timeout)
		return r, t, fellBack
	}
	return m.DicRun(D, trigger), false, fellBack
}
