package dic_server

import (
	"testing"

	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// TestSetRespHeader 同名头覆盖且保持首次出现的位置，头名规范化、空头名忽略。
func TestSetRespHeader(t *testing.T) {
	headers := []respHeader{}
	setRespHeader(&headers, "content-type", "text/html")
	setRespHeader(&headers, "X-One", "abc")
	setRespHeader(&headers, "Content-Type", "application/json")
	setRespHeader(&headers, "", "ignored")
	want := []respHeader{
		{Key: "Content-Type", Value: "application/json"},
		{Key: "X-One", Value: "abc"},
	}
	if len(headers) != len(want) {
		t.Fatalf("头部数量 = %d，期望 %d: %+v", len(headers), len(want), headers)
	}
	for i := range want {
		if headers[i] != want[i] {
			t.Fatalf("第 %d 个头 = %+v，期望 %+v", i, headers[i], want[i])
		}
	}
}

// TestCollectLocalResp 输出头部（JSON）合并进收集结果并覆盖同名头；响应状态取自全局变量。
func TestCollectLocalResp(t *testing.T) {
	val := dto.NewVal().
		Set("响应状态", "404").
		Set("输出头部", `{"Content-Type":"application/json","X-Two":"bcd"}`)
	headers := []respHeader{}
	setRespHeader(&headers, "content-type", "text/html")
	setRespHeader(&headers, "X-One", "abc")

	status, got := collectLocalResp(val, headers)
	if status != "404" {
		t.Fatalf("响应状态 = %q，期望 404", status)
	}
	m := map[string]string{}
	for _, h := range got {
		m[h.Key] = h.Value
	}
	if m["Content-Type"] != "application/json" {
		t.Fatalf("Content-Type = %q，期望 application/json（输出头部 应覆盖 设置头部）", m["Content-Type"])
	}
	if m["X-One"] != "abc" || m["X-Two"] != "bcd" {
		t.Fatalf("头部收集不完整: %+v", got)
	}
	if len(got) != 3 {
		t.Fatalf("头部数量 = %d，期望 3: %+v", len(got), got)
	}
}

// TestCollectLocalRespDefaults 未设置时响应状态默认 200、无额外头。
func TestCollectLocalRespDefaults(t *testing.T) {
	val := dto.NewVal().Set("输出头部", "{}")
	status, got := collectLocalResp(val, []respHeader{})
	if status != "200" || len(got) != 0 {
		t.Fatalf("默认响应信息 = %q / %+v，期望 200 / 空", status, got)
	}
}

// callSetHeader 以 $设置头部 key value$ 的形式调用注入的实现。
func callSetHeader(t *testing.T, fn func(*dto.DicInputs) (any, error), key, value string) {
	t.Helper()
	in := utils.NewDicInputs()
	in.List = []any{"设置头部", key, value}
	if _, err := fn(dto.NewDicInputs(nil, nil, &in)); err != nil {
		t.Fatalf("设置头部(%s, %s) 报错: %v", key, value, err)
	}
}

// TestLocalHTTPFuncsCollect 注入的 设置头部 写入的是调用方持有的同一个收集切片
// （append 后仍能被读到），且同名头覆盖而非追加。
func TestLocalHTTPFuncsCollect(t *testing.T) {
	headers := &[]respHeader{}
	setHeader := localHTTPFuncs(headers)["设置头部"].Fn
	callSetHeader(t, setHeader, "content-type", "application/json")
	callSetHeader(t, setHeader, "Content-Type", "text/plain")
	if len(*headers) != 1 {
		t.Fatalf("头部数量 = %d，期望 1（同名覆盖）: %+v", len(*headers), *headers)
	}
	if (*headers)[0].Key != "Content-Type" || (*headers)[0].Value != "text/plain" {
		t.Fatalf("收集结果 = %+v，期望 Content-Type: text/plain", *headers)
	}
}

// TestAttachLocalHTTPFuncs 普通词库（.n）注入：不污染原函数表、补齐响应默认值（词库已设置的不被覆盖），
// 且注入后的 设置头部 能被 collectLocalResp 收集到。
func TestAttachLocalHTTPFuncs(t *testing.T) {
	origin := map[string]dto.DicFunc{
		"自定义": {L: "0", Fn: func(*dto.DicInputs) (any, error) { return "", nil }},
	}
	dic := &dic_dto.Dic{Val: dto.NewDicVal(), MyFunc: origin}
	headers := attachLocalHTTPFuncs(dic)

	if len(origin) != 1 {
		t.Fatalf("原函数表被污染: %+v", origin)
	}
	for _, name := range []string{"自定义", "设置头部", "GET", "POST"} {
		if _, ok := dic.MyFunc[name]; !ok {
			t.Fatalf("注入后缺少函数 %s: %+v", name, dic.MyFunc)
		}
	}
	if dic.Val.G.Get("响应状态") != "200" || dic.Val.G.Get("输出头部") != "{}" {
		t.Fatalf("响应默认值未补齐: %v / %v", dic.Val.G.Get("响应状态"), dic.Val.G.Get("输出头部"))
	}

	callSetHeader(t, dic.MyFunc["设置头部"].Fn, "X-One", "abc")
	status, got := collectLocalResp(dic.Val.G, *headers)
	if status != "200" || len(got) != 1 || got[0].Key != "X-One" || got[0].Value != "abc" {
		t.Fatalf("收集结果 = %q / %+v", status, got)
	}

	// 词库已自行设置响应状态时，默认值不得覆盖
	dic2 := &dic_dto.Dic{Val: dto.NewDicVal(), MyFunc: map[string]dto.DicFunc{}}
	dic2.Val.G.Set("响应状态", "404")
	attachLocalHTTPFuncs(dic2)
	if dic2.Val.G.Get("响应状态") != "404" {
		t.Fatalf("词库设置的响应状态被默认值覆盖: %v", dic2.Val.G.Get("响应状态"))
	}
}