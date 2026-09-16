package dic_server

import (
	"strings"
	"testing"
)

// 常驻技能（语法结构）正文必须每轮注入系统提示，否则多轮压缩后模型会凭印象写错语法。
func TestAIPinnedSkillsTextIncludesSyntax(t *testing.T) {
	text := aiPinnedSkillsText([]string{aiBuiltinSkillSyntax})
	if text == "" {
		t.Fatalf("声明了常驻技能「%s」，正文不应为空", aiBuiltinSkillSyntax)
	}
	if !strings.Contains(text, "【常驻技能："+aiBuiltinSkillSyntax) {
		t.Fatalf("缺少常驻技能标题，实际内容：\n%s", text)
	}
	// 技能正文（8 篇语法文档索引）
	if !strings.Contains(text, "0-词库语法/08-高频易错点.md") {
		t.Fatalf("缺少语法技能正文，实际内容：\n%s", text)
	}
	// 随常驻一起给出的高频易错点全文
	if !strings.Contains(text, "高频易错点") {
		t.Fatalf("缺少高频易错点文档正文，实际内容：\n%s", text)
	}
	if !strings.Contains(text, "未声明参数规则") {
		t.Fatalf("高频易错点文档内容不完整，实际内容：\n%s", text)
	}
}

// 未声明常驻技能的智能体不应被注入任何常驻正文（不白白占用上下文）。
func TestAIPinnedSkillsTextEmptyWhenNotDeclared(t *testing.T) {
	if text := aiPinnedSkillsText([]string{aiBuiltinSkillWeb, aiBuiltinSkillTools}); text != "" {
		t.Fatalf("未声明常驻技能时应返回空串，实际内容：\n%s", text)
	}
	if text := aiPinnedSkillsText(nil); text != "" {
		t.Fatalf("无技能声明时应返回空串，实际内容：\n%s", text)
	}
}

// 常驻技能正文已直接给出，不应再作为「按需 read_skill」的清单项列出。
func TestAISkillsPromptTextExcludesPinned(t *testing.T) {
	text := aiSkillsPromptText([]string{aiBuiltinSkillWeb, aiBuiltinSkillSyntax})
	if strings.Contains(text, "- "+aiBuiltinSkillSyntax) {
		t.Fatalf("常驻技能不应出现在「可用技能」清单中，实际内容：\n%s", text)
	}
	if !strings.Contains(text, aiBuiltinSkillSyntax+" 为常驻技能") {
		t.Fatalf("清单中缺少常驻技能说明，实际内容：\n%s", text)
	}
	// 非常驻技能仍按原样列出
	if !strings.Contains(text, "- "+aiBuiltinSkillWeb) {
		t.Fatalf("非常驻技能应保留在清单中，实际内容：\n%s", text)
	}
}
