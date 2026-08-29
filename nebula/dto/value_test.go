package dto

import (
	"testing"

	"github.com/iancoleman/orderedmap"
)

// TestValCloneDeepCopy 验证 Clone 对 map/slice 做深拷贝，且基本值与主流程隔离。
func TestValCloneDeepCopy(t *testing.T) {
	v := NewVal()
	v.Set("str", "hello")
	v.SetInt64("num", 42)
	v.Set("obj", map[string]any{"a": 1})

	c := v.Clone()

	// 修改快照中的 map 内部，不应影响原值
	if m, ok := c.Get("obj").(map[string]any); ok {
		m["a"] = 999
	}
	if m, _ := v.Get("obj").(map[string]any); m["a"] != 1 {
		t.Fatalf("map 深拷贝失败：原值被修改为 %v", m["a"])
	}

	// 修改原值，不应影响快照（值隔离）
	v.Set("str", "world")
	if got := c.Get("str"); got != "hello" {
		t.Fatalf("字符串快照未隔离：c=%q", got)
	}
	if got, _ := c.GetInt64("num"); got != 42 {
		t.Fatalf("整数快照未隔离：c=%d", got)
	}
}

// TestValCloneDeepCopyDicClass 验证 Clone 对类实例深拷贝成员变量（LocalValue），避免共享可变状态。
func TestValCloneDeepCopyDicClass(t *testing.T) {
	cls := NewDicClass()
	cls.LocalValue.Set("成员", "原值")
	v := NewVal()
	v.Set("甲", cls)

	c := v.Clone()

	if cc, ok := c.Get("甲").(*DicClass); ok {
		cc.LocalValue.Set("成员", "改值")
	}
	if orig, ok := v.Get("甲").(*DicClass); ok {
		if got := orig.LocalValue.Get("成员"); got != "原值" {
			t.Fatalf("类实例深拷贝失败：原值被修改为 %v", got)
		}
	}
}

// TestValCloneDeepCopyOrderedMap 验证 Clone 对有序字典（指针与值类型）深拷贝，避免共享 keys/values。
func TestValCloneDeepCopyOrderedMap(t *testing.T) {
	om := orderedmap.New()
	om.Set("键1", "值1")
	om.Set("键2", []any{"a", map[string]any{"x": 1}})

	// 值类型嵌套在 map[string]any 中（JSON 反序列化后的形态）
	inner := orderedmap.New()
	inner.Set("内", "原")

	v := NewVal()
	v.Set("字典", om)
	v.Set("外层", map[string]any{"嵌套": *inner})

	c := v.Clone()

	// 修改快照中的指针字典，不应影响原值
	if cm, ok := c.Get("字典").(*orderedmap.OrderedMap); ok {
		cm.Set("键1", "改值")
		cm.Set("新键", "新值")
	}
	if ov, _ := v.Get("字典").(*orderedmap.OrderedMap); ov != nil {
		if got, _ := ov.Get("键1"); got != "值1" {
			t.Fatalf("有序字典深拷贝失败：键1 被修改为 %v", got)
		}
		if _, exists := ov.Get("新键"); exists {
			t.Fatalf("有序字典深拷贝失败：新增键泄漏到原值")
		}
	}

	// 修改快照中的值类型字典，不应影响原值
	if cm, ok := c.Get("外层").(map[string]any); ok {
		if nm, ok := cm["嵌套"].(orderedmap.OrderedMap); ok {
			nm.Set("内", "改")
		}
	}
	if ov, _ := v.Get("外层").(map[string]any); ov != nil {
		if nm, ok := ov["嵌套"].(orderedmap.OrderedMap); ok {
			if got, _ := nm.Get("内"); got != "原" {
				t.Fatalf("值类型有序字典深拷贝失败：内 被修改为 %v", got)
			}
		}
	}
}
