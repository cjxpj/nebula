package dic

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dic/ast"
	"github.com/cjxpj/nebula/dic/bc"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/run"
)

// 词库信息约定：在 [函数]词库信息 内用局部变量声明元数据。
const (
	dicInfoFunc  = "词库信息"
	dicInfoName  = "名称"
	dicInfoDesc  = "描述"
	dicInfoPrice = "价格"
)

// DicReadInfo 读取词库信息：以引擎同一套执行逻辑运行 [函数]词库信息 内的逻辑后读取局部变量（名称/价格/描述）。
// 与普通执行等价，仅额外禁用：函数（$函数$ 调用与 函数>/执行函数> 框）与循环框（循环>/判断循环>）。
// 其余语法（判断框、遍历框、匹配框、[计算]、%变量% 插值、快捷赋值等）保持引擎默认语义。
// 未定义 [函数]词库信息 时返回错误。
func (m *dicImpl) DicReadInfo(D *dic_dto.Dic) (*dic_dto.DicInfo, error) {
	text, _, _, errRule, ok := run.RunFuncIndexed(D.Data.GetFuncIndex(), dicInfoFunc, 0)
	if !ok {
		if errRule != "" {
			return nil, fmt.Errorf("[函数]%s 参数数量错误(需要%s，实际0)", dicInfoFunc, errRule)
		}
		return nil, fmt.Errorf("词库未定义 [函数]%s", dicInfoFunc)
	}

	entry := dic_dto.NewRunDicEntry().CloseTrigger().SetGlobal_v(D.Val.G).SetDic_v(D.Data)
	// 禁用函数：所有 $函数$ 调用（含判断条件/插值中的）一律不执行。
	entry.Sys_v.NoFunc.Store(true)

	m.dicRunLineBytecodeInstrs(entry, bc.Compile(dropInfoBlocks(ast.ParseBody(text, nil))))

	info := &dic_dto.DicInfo{
		Name: entry.Val.P.GetStr(dicInfoName),
		Desc: entry.Val.P.GetStr(dicInfoDesc),
	}
	if p := strings.TrimSpace(entry.Val.P.GetStr(dicInfoPrice)); p != "" {
		pv, perr := strconv.ParseInt(p, 10, 64)
		if perr != nil || pv < 0 {
			return nil, errors.New("价格必须是不小于 0 的整数")
		}
		info.Price = pv
	}
	return info, nil
}

// dropInfoBlocks 剔除词库信息函数体内的函数框与循环框，使这两类框整块不执行；
// 其余框块交给引擎按默认语义编译执行。
func dropInfoBlocks(nodes []ast.Node) []ast.Node {
	out := make([]ast.Node, 0, len(nodes))
	for _, n := range nodes {
		if b, ok := n.(*ast.Block); ok {
			if b.OpenKind == ast.BlockFunc || b.OpenKind == ast.BlockFor {
				continue
			}
			b.Children = dropInfoBlocks(b.Children)
		}
		out = append(out, n)
	}
	return out
}
