package build

import (
	"os"
	"strings"
	"testing"
)

// TestFormatWebDicIndentsHTMLAndScripts HTML 结构与两类脚本块都应对齐缩进：
// <script type="nebula"> 的正文按 .n 块结构缩进，普通 <script> 的 JS 整体平移（相对缩进不变）。
func TestFormatWebDicIndentsHTMLAndScripts(t *testing.T) {
	text := `<html>
<body>
<script type="nebula">
甲:1
如果>$甲$>0
文本>正数
<文本
<如果
</script>
<script>
var a = 1;
if (a) {
  console.log(a);
}
</script>
</body>
</html>
`
	got := FormatWebDic(text)
	want := `<html>
    <body>
        <script type="nebula">
            甲:1
            如果>$甲$>0
                文本>正数
                <文本
            <如果
        </script>
        <script>
            var a = 1;
            if (a) {
              console.log(a);
            }
        </script>
    </body>
</html>
`
	if got != want {
		t.Fatalf("格式化结果不符:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatWebDicIdempotent 二次格式化必须稳定；pre 与 HTML 注释逐字节保留。
func TestFormatWebDicIdempotent(t *testing.T) {
	text := "<html>\n<body>\n<pre>\n  保留\n    缩进\n</pre>\n<!--\n <b>注释不缩进</b>\n-->\n<script type=\"nebula\">\n  文本>内容\n$内容$\n</script>\n</body>\n</html>\n"
	once := FormatWebDic(text)
	if twice := FormatWebDic(once); twice != once {
		t.Fatalf("二次格式化结果不一致:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
	if !strings.Contains(once, "<pre>\n  保留\n    缩进\n</pre>") {
		t.Fatalf("pre 内容被改动:\n%s", once)
	}
	if !strings.Contains(once, "<!--\n <b>注释不缩进</b>\n-->") {
		t.Fatalf("注释被改动:\n%s", once)
	}
	if !strings.Contains(once, "<script type=\"nebula\">\n            文本>内容\n                $内容$\n        </script>") {
		t.Fatalf("脚本块正文未按块结构缩进:\n%s", once)
	}
}

// TestFormatWebDicRealFiles 真实网页词库：格式化幂等、块数量不变、脚本/样式块正文缩进到标签内。
func TestFormatWebDicRealFiles(t *testing.T) {
	for _, path := range []string{
		"../appfiles/static/dic/public/404.wn",
		"../appfiles/static/dic/public/index.wn",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", path, err)
		}
		got := FormatWebDic(string(data))
		if twice := FormatWebDic(got); twice != got {
			t.Fatalf("%s 二次格式化结果不一致:\n--- once ---\n%s\n--- twice ---\n%s", path, got, twice)
		}
		if n := strings.Count(strings.ToLower(got), "<script"); n != strings.Count(strings.ToLower(string(data)), "<script") {
			t.Fatalf("%s 脚本块数量变化", path)
		}
		if n := strings.Count(strings.ToLower(got), "<style"); n != strings.Count(strings.ToLower(string(data)), "<style") {
			t.Fatalf("%s 样式块数量变化", path)
		}
		checkWebBlockIndent(t, path, got)
	}
}

// checkWebBlockIndent 校验 <script>/<style> 块的正文行都缩进在开始标签之内。
func checkWebBlockIndent(t *testing.T, path, text string) {
	t.Helper()
	openTag := ""
	openIndent := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if openTag != "" {
			if strings.HasPrefix(strings.ToLower(trimmed), "</"+openTag) {
				openTag = ""
				continue
			}
			if trimmed != "" && indent <= openIndent {
				t.Fatalf("%s 的 %s 块正文未缩进：%q", path, openTag, line)
			}
			continue
		}
		for _, tag := range []string{"script", "style"} {
			lower := strings.ToLower(trimmed)
			if !strings.HasPrefix(lower, "<"+tag) {
				continue
			}
			if !strings.Contains(lower, "</"+tag) {
				openTag, openIndent = tag, indent
			}
			break
		}
	}
	if openTag != "" {
		t.Fatalf("%s 的 %s 块未找到关闭标签", path, openTag)
	}
}
