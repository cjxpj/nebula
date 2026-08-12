package dic

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
)

// chdirToAppWin 切换到 nebula/app/win（NebulaData 所在），基于源码文件位置计算绝对路径，多次调用幂等
func chdirToAppWin() {
	_, file, _, _ := runtime.Caller(0)
	winDir := filepath.Join(filepath.Dir(file), "..", "app", "win")
	if err := os.Chdir(winDir); err != nil {
		panic(err)
	}
}

func runHead(t *testing.T, head []string) *dic_dto.Dic {
	t.Helper()
	chdirToAppWin()
	content := strings.Join(head, "\n") + "\n"
	D := dic_dto.NewDic("t.n", content)
	dic_api.Api.DicRun(D, "Main")
	return D
}

func getStr(t *testing.T, D *dic_dto.Dic, key string) string {
	t.Helper()
	v, ok := D.Val.P.Get(key).(string)
	if !ok {
		t.Fatalf("%s 不是字符串: %T (%v)", key, D.Val.P.Get(key), D.Val.P.Get(key))
	}
	return v
}

func TestValChainBlock(t *testing.T) {
	// 块式：f:>>> ... <<< 逐行执行写回
	D := runHead(t, []string{"Main", "f:>>>", "o", "$复读 %f%$", "$复读 %f%$", "<<<"})
	if got := getStr(t, D, "f"); got != "oooo" {
		t.Errorf("块式 期望 oooo 实际 %q", got)
	}
}

func TestValChainSingleLine(t *testing.T) {
	// 单行链式：每段结果写回 f，下一段复用 %f%
	D := runHead(t, []string{"Main", "f:$sha256 123456$>>>$Byte转String %f%$>>>$编码 十六进制 %f%$"})
	want := "8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92"
	if got := getStr(t, D, "f"); got != want {
		t.Errorf("单行链式 期望 %s 实际 %q", want, got)
	}
}

func TestProbe_ChainWithQuestion(t *testing.T) {
	// 链段内 ?: 回退：f = ?:o 先回退得到 o，再复读得到 oo
	D := runHead(t, []string{"Main", "f:?:o>>>$复读 %f%$"})
	if got := getStr(t, D, "f"); got != "oo" {
		t.Errorf("?:链段 期望 oo 实际 %q", got)
	}
}

func TestProbe_ChainWithQuestion2(t *testing.T) {
	// 链段内 ?: 多个：f = o?:x 得到 o，再复读得到 oo
	D := runHead(t, []string{"Main", "f:o?:x>>>$复读 %f%$"})
	if got := getStr(t, D, "f"); got != "oo" {
		t.Errorf("?:链段2 期望 oo 实际 %q", got)
	}
}

func TestProbe_JsonWithChainSep(t *testing.T) {
	// 单行 JSON 值内含 >>> 字符串，不应被链式分割
	D := runHead(t, []string{`Main`, `f:{"a":">>>"}`})
	if got := getStr(t, D, "f"); got != `{"a":">>>"}` {
		t.Errorf("JSON 期望 %q 实际 %q", `{"a":">>>"}`, got)
	}
}

func TestProbe_BlockQuestion(t *testing.T) {
	// 块内行使用 ?: 回退
	D := runHead(t, []string{"Main", "f:>>>", "?:o", "$复读 %f%$", "<<<"})
	if got := getStr(t, D, "f"); got != "oo" {
		t.Errorf("块内?: 期望 oo 实际 %q", got)
	}
}

func TestProbe_JsonChainStillWorks(t *testing.T) {
	// 链式 JSON 段仍可执行
	D := runHead(t, []string{`Main`, `f:{"a":1}>>>{"b":2}`})
	if got := getStr(t, D, "f"); got != `{"b":2}` {
		t.Errorf("JSON链 期望 {\"b\":2} 实际 %q", got)
	}
}

func TestProbe_SplitValChain(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a>>>b", []string{"a", "b"}},
		{">>>", []string{""}},
		{">>>>", []string{"", ">"}},
		{"a>>>>b", []string{"a", ">b"}},
		{"a>>>", []string{"a"}},
		{"$a$>>>$b%f%$", []string{"$a$", "$b%f%$"}},
		{"$时间 yyyy-MM-dd>>>HH:mm:ss$", []string{"$时间 yyyy-MM-dd>>>HH:mm:ss$"}},
		{"a", []string{"a"}},
		{"", nil},
	}
	for _, c := range cases {
		got := SplitValChain(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitValChain(%q) = %#v, 期望 %#v", c.in, got, c.want)
		}
	}
}
