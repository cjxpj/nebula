package ast

import (
	"strings"

	"github.com/cjxpj/nebula/build"
)

// blockOpen 识别「框开启」行，语义与 run 包编译检查的 blockOpen 保持一致。
func blockOpen(line string) (BlockKind, bool) {
	switch {
	case len(line) > 7 && strings.HasPrefix(line, "如果>"):
		return BlockIf, true
	case len(line) > 7 && strings.HasPrefix(line, "匹配>"):
		return BlockMatch, true
	case strings.HasPrefix(line, "判断循环>") && len(line) > len("判断循环>"):
		return BlockFor, true
	case strings.HasPrefix(line, "循环>"):
		return BlockFor, true
	case strings.HasPrefix(line, "遍历>"):
		return BlockForEach, true
	case strings.HasPrefix(line, "纯文本>"):
		return BlockText, true
	case strings.HasPrefix(line, "文本>"):
		return BlockText, true
	case line == "JSON>[":
		return BlockNewJson, true
	case line == "JSON>{":
		return BlockNewJson, true
	case strings.HasPrefix(line, "JSON>"):
		return BlockJson, true
	case line == "--js":
		return BlockNodeJs, true
	case line == "#:>>>":
		return BlockValChain, true
	case strings.HasPrefix(line, "#:执行函数>"):
		return BlockFunc, true
	}
	// 变量: 开头的赋值框（vType 6，后缀决定框类型）：
	//   { / [   → 多行 JSON 赋值（变量名不含 -> 路径，避免与 a->b:{ 单行 JSON 路径赋值混淆）
	//   >>>     → 连续执行框（与单行链式 a>>>b 区分）
	//   """     → 赋值文本框（内容 %变量% 插值）
	//   '''     → 赋值文本框（内容原样）
	//   函数> / 执行函数> → 函数框（存储 / 立即执行）
	if vt, vp, vs := build.ValTextTest(line); vt == 6 {
		// 变量:函数> / 变量:执行函数> 开头的函数框（赋予值形式）。
		if vp != "" {
			switch {
			case strings.HasPrefix(vs, "执行函数>"):
				return BlockFunc, true
			case strings.HasPrefix(vs, "函数>"):
				return BlockFunc, true
			}
		}
		switch vs {
		case "{", "[":
			if !strings.Contains(vp, "->") {
				return BlockVarNewJson, true
			}
		case ">>>":
			return BlockValChain, true
		case `"""`:
			return BlockValText, true
		case `'''`:
			return BlockValTextr, true
		}
	}
	return 0, false
}

// blockClose 识别「框关闭」标记行。
func blockClose(line string) (BlockKind, bool) {
	switch line {
	case "<函数":
		return BlockFunc, true
	case "<如果":
		return BlockIf, true
	case "<匹配":
		return BlockMatch, true
	case "<循环":
		return BlockFor, true
	case "<遍历":
		return BlockForEach, true
	case "<文本":
		return BlockText, true
	case "<JSON":
		return BlockJson, true
	}
	return 0, false
}

// ParseEntry 将一条词条的正文解析为 Entry。
// trigger/triggerLine 为触发词及其行号；body 为正文行；lineNums 为每行对应原始文件行号
// （1-based，可与 body 不等长，缺失处按 0 处理）。
func ParseEntry(trigger string, triggerLine int, body []string, lineNums []int) *Entry {
	return &Entry{
		Trigger:     trigger,
		TriggerLine: triggerLine,
		Body:        ParseBody(body, lineNums),
	}
}

// ParseBody 将正文行解析为节点列表（框块 + 语句叶子）。
// 采用栈式结构，语义与解释器逐行分类、run 包编译检查的框配对逻辑一致：
//   - 文本/JSON/新建JSON 为「叶子框」，内部行不再识别嵌套框；
//   - 新建JSON 通过括号平衡判断闭合；
//   - 函数/判断/循环/遍历 支持嵌套。
//
// 每个框块都会记录其原始行（Raw），供字节码编译器对未下沉的复杂框整段委托。
func ParseBody(body []string, lineNums []int) []Node {
	var roots []Node
	var stack []*Block

	// cur 返回当前应追加节点的列表（栈顶框的 Children，或顶层 roots）。
	cur := func() *[]Node {
		if len(stack) == 0 {
			return &roots
		}
		return &stack[len(stack)-1].Children
	}

	lineNumAt := func(i int) int {
		if i >= 0 && i < len(lineNums) && lineNums[i] > 0 {
			return lineNums[i]
		}
		// 与解释器 execProgram 的 fallback 一致：LineNums 缺失或为 0 时，
		// 以正文内 1-based 下标作为行号（CurLine = RunDicindex）。
		return i + 1
	}

	// closeFrame 记录框的原始行（及其行号）并弹出栈顶。
	closeFrame := func(endIdx int) {
		top := stack[len(stack)-1]
		top.Raw = body[top.rawStart : endIdx+1]
		top.RawLineNums = make([]int, 0, endIdx-top.rawStart+1)
		for j := top.rawStart; j <= endIdx; j++ {
			top.RawLineNums = append(top.RawLineNums, lineNumAt(j))
		}
		stack = stack[:len(stack)-1]
	}

	for i, line := range body {
		ln := lineNumAt(i)

		// 叶子框：只识别各自关闭，内部行一律作为语句叶子。
		if len(stack) > 0 {
			top := stack[len(stack)-1]
			switch top.OpenKind {
			case BlockText:
				if line == "<文本" {
					top.CloseLine = ln
					closeFrame(i)
				} else {
					*cur() = append(*cur(), &Stmt{Line: ln, Text: line})
				}
				continue
			case BlockJson:
				if line == "<JSON" {
					top.CloseLine = ln
					closeFrame(i)
				} else {
					*cur() = append(*cur(), &Stmt{Line: ln, Text: line})
				}
				continue
			case BlockNewJson:
				if strings.HasSuffix(line, "{") || strings.HasSuffix(line, "[") {
					top.depth++
				}
				if line == "}" || line == "]" || line == "}," || line == "]," {
					top.depth--
					if top.depth == 0 && (line == "}" || line == "]") {
						top.CloseLine = ln
						closeFrame(i)
						continue
					}
				}
				*cur() = append(*cur(), &Stmt{Line: ln, Text: line})
				continue
			case BlockVarNewJson:
				if strings.HasSuffix(line, "{") || strings.HasSuffix(line, "[") {
					top.depth++
				}
				if line == "}" || line == "]" || line == "}," || line == "]," {
					top.depth--
					if top.depth == 0 && (line == "}" || line == "]") {
						top.CloseLine = ln
						closeFrame(i)
						continue
					}
				}
				*cur() = append(*cur(), &Stmt{Line: ln, Text: line})
				continue
			case BlockValChain:
				if line == "<<<" {
					top.CloseLine = ln
					closeFrame(i)
				} else {
					*cur() = append(*cur(), &Stmt{Line: ln, Text: line})
				}
				continue
			case BlockValText:
				if line == `"""` {
					top.CloseLine = ln
					closeFrame(i)
				} else {
					*cur() = append(*cur(), &Stmt{Line: ln, Text: line})
				}
				continue
			case BlockValTextr:
				if line == `'''` {
					top.CloseLine = ln
					closeFrame(i)
				} else {
					*cur() = append(*cur(), &Stmt{Line: ln, Text: line})
				}
				continue
			case BlockNodeJs:
				if line == "--end" {
					top.CloseLine = ln
					closeFrame(i)
				} else {
					*cur() = append(*cur(), &Stmt{Line: ln, Text: line})
				}
				continue
			}
		}

		// 关闭标记：匹配则闭合栈顶框；不匹配或栈空时保留为语句叶子（配对错误由编译检查报告）。
		if k, ok := blockClose(line); ok {
			if len(stack) > 0 && stack[len(stack)-1].OpenKind == k {
				stack[len(stack)-1].CloseLine = ln
				closeFrame(i)
			} else {
				*cur() = append(*cur(), &Stmt{Line: ln, Text: line})
			}
			continue
		}

		// 开启标记：压栈新框。
		if k, ok := blockOpen(line); ok {
			b := &Block{Open: line, OpenKind: k, OpenLine: ln, depth: 1, rawStart: i}
			*cur() = append(*cur(), b)
			stack = append(stack, b)
			continue
		}

		*cur() = append(*cur(), &Stmt{Line: ln, Text: line})
	}

	// 未闭合的框：原始行到末尾。
	for _, b := range stack {
		b.Raw = body[b.rawStart:]
		b.RawLineNums = make([]int, 0, len(body)-b.rawStart)
		for j := b.rawStart; j < len(body); j++ {
			b.RawLineNums = append(b.RawLineNums, lineNumAt(j))
		}
	}

	return roots
}
