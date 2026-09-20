package funcs

import (
	"testing"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// callHtmlFn 直接调用 HTML 对象的某个方法（输入第 1 位起为方法参数）。
func callHtmlFn(f dto.DicFunc, args ...any) (any, error) {
	inputs := utils.NewDicInputs()
	list := []any{""}
	list = append(list, args...)
	inputs.Set(list)
	return f.Fn(dto.NewDicInputsWithOutput(nil, nil, &inputs, &dto.SingleValue{}))
}

// newTestHtml 创建一个没有标题的 HTML 组件对象。
func newTestHtml(t *testing.T) *dto.DicClass {
	t.Helper()
	res, err := callHtmlFn(dto.DicFunc{Fn: htmlNew}, "")
	if err != nil {
		t.Fatalf("创建HTML 失败: %v", err)
	}
	cls, ok := res.(*dto.DicClass)
	if !ok {
		t.Fatalf("创建HTML 返回类型错误: %T", res)
	}
	return cls
}

// createLayout 调用「创建布局」并断言其返回布局分支句柄。
func createLayout(t *testing.T, obj *dto.DicClass, kind string) *dto.DicClass {
	t.Helper()
	res, err := callHtmlFn(obj.Fn["创建布局"], kind)
	if err != nil {
		t.Fatalf("创建布局 %s 失败: %v", kind, err)
	}
	handle, ok := res.(*dto.DicClass)
	if !ok {
		t.Fatalf("创建布局 %s 应返回句柄，实际: %T", kind, res)
	}
	return handle
}

// TestHtmlCreateLayoutHandle 验证「创建布局」返回分支句柄，句柄能继续开子布局，
// 且句柄上的「设置*」会切回自己所在的分支。
func TestHtmlCreateLayoutHandle(t *testing.T) {
	h := newTestHtml(t)

	createLayout(t, h, "页面")
	createLayout(t, h, "容器")
	head := createLayout(t, h, "页头")
	if _, ok := head.Fn["创建布局"]; !ok {
		t.Fatal("布局句柄应能继续创建布局分支")
	}
	createLayout(t, head, "横向")

	// 「设置*」在句柄上调用时回到「页头」分支内部
	if _, err := callHtmlFn(head.Fn["设置文本"], "页头文字"); err != nil {
		t.Fatalf("句柄设置文本失败: %v", err)
	}

	got, err := callHtmlFn(h.Fn["获取"], "片段")
	if err != nil {
		t.Fatalf("获取失败: %v", err)
	}
	want := `<div class="nebula-page">
  <div class="nebula-container">
    <header class="nebula-header">
      <div class="nebula-row"></div>
      <p>页头文字</p>
    </header>
  </div>
</div>`
	if got != want {
		t.Fatalf("嵌套结果错误:\n得到:\n%s\n期望:\n%s", got, want)
	}
}

// TestHtmlCreateLayoutHandleAutoClose 验证句柄下创建「同层或更浅」的布局时，
// 相对该分支自动闭合，落到分支的上一层（与原有整页写法口径一致）。
func TestHtmlCreateLayoutHandleAutoClose(t *testing.T) {
	h := newTestHtml(t)

	createLayout(t, h, "页面")
	container := createLayout(t, h, "容器")
	head := createLayout(t, h, "页头")

	// 「内容」与「页头」同层：自动闭合掉「页头」，成为容器下的兄弟节点
	createLayout(t, head, "内容")

	// 句柄可重复使用：回到「容器」分支后继续追加，与「页头」「内容」同级
	createLayout(t, container, "页脚")

	got, err := callHtmlFn(h.Fn["获取"], "片段")
	if err != nil {
		t.Fatalf("获取失败: %v", err)
	}
	want := `<div class="nebula-page">
  <div class="nebula-container">
    <header class="nebula-header"></header>
    <main class="nebula-main"></main>
    <footer class="nebula-footer"></footer>
  </div>
</div>`
	if got != want {
		t.Fatalf("自动闭合结果错误:\n得到:\n%s\n期望:\n%s", got, want)
	}
}
