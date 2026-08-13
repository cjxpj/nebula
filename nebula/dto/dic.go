package dto

import (
	"database/sql"
	"strings"
	"sync/atomic"

	"github.com/cjxpj/nebula/utils"
)

// 系统变量
type LocalDicValue struct {
	For struct {
		Success   bool     `json:"success"`
		Run       any      `json:"for"`
		Num       int      `json:"num"`
		VlaueName string   `json:"vlaueName"`
		Content   []string `json:"content"`
		IsFor     bool     `json:"IsFor"`
		Jump      bool     `json:"jump"`
	} `json:"循环框"`
	ForEach LocalDicValueForEach `json:"遍历框"`
	Func    struct {
		Success   bool     `json:"success"`
		Num       int      `json:"num"`
		VlaueName string   `json:"vlaueName"`
		Trigger   string   `json:"trigger"`
		Content   []string `json:"content"`
	} `json:"函数框"`
	Text struct {
		Success   bool            `json:"success"`
		ReadValue bool            `json:"readValue"`
		LineFeed  string          `json:"lineFeed"`
		VlaueName string          `json:"vlaueName"`
		Content   strings.Builder `json:"content"`
	} `json:"文本框"`
	ValText struct {
		Success   bool     `json:"success"`
		VlaueName string   `json:"vlaueName"`
		Content   []string `json:"content"`
	} `json:"赋予值文本框"`
	ValTextr struct {
		Success   bool     `json:"success"`
		VlaueName string   `json:"vlaueName"`
		Content   []string `json:"content"`
	} `json:"赋予值纯文本框"`
	ValChain struct {
		Success   bool   `json:"success"`
		VlaueName string `json:"vlaueName"`
	} `json:"赋予值连续执行框"`
	IfFunc struct {
		Success bool       `json:"success"`
		IsElse  bool       `json:"IsElse"`
		Num     int        `json:"num"`
		IfNum   int        `json:"ifnum"`
		If      []string   `json:"if"`
		Else    []string   `json:"Else"`
		Run     [][]string `json:"Run"`
		IsIf    bool       `json:"IsIf"`
		Jump    bool       `json:"jump"`
	} `json:"判断框"`
	SetJson struct {
		Success   bool   `json:"success"`
		VlaueName string `json:"vlaueName"`
		Json      any    `json:"json"`
		OkLen     bool   `json:"OkLen"`
		Len       int    `json:"Len"`
	} `json:"Json框"`
	SetNewJson struct {
		Success   bool   `json:"success"`
		VlaueName string `json:"vlaueName"`
		Json      string `json:"json"`
		// true时候是{},false时候是[]
		JsonType bool `json:"JsonType"`
		Len      int  `json:"Len"`
	} `json:"新建Json框"`
	NodeJs struct {
		Success bool     `json:"success"`
		Content []string `json:"content"`
	}
	Database *sql.DB     `json:"database"`
	Stop     atomic.Bool `json:"stop"`
}

type LocalDicValueForEach struct {
	Success   bool     `json:"success"`
	Run       any      `json:"for"`
	Num       int      `json:"num"`
	VlaueName string   `json:"vlaueName"`
	Content   []string `json:"content"`
	IsFor     bool     `json:"IsFor"`
	Jump      bool     `json:"jump"`
}

// 词库结构
type BuildDic struct {
	Trigger  string   `json:"trigger"`
	Text     []string `json:"text"`
	LineNums []int    `json:"-"` // 每行文本对应的原始文件行号（1-based），用于调试定位
}

// =================================================

type DicLine []string

type DicInfoData struct {
	Data  *DicInfo `json:"data"`
	Value *Val     `json:"value"`
}

type DicInfo struct {
	Value       *Val                     `json:"变量"`
	Path        string                   `json:"路径"`
	Head        DicLine                  `json:"头部"`
	Dic         []*RegDicLine            `json:"词库"`
	LocalStatic []*RegDicLine            `json:"内部"`
	LocalFunc   []*BuildDicFunc          `json:"函数"`
	LocalClass  map[string]*DicClassInfo `json:"整合包"`
	Special     map[string][]*RegDicLine `json:"特殊"`
}

// 词库结构
type RegDicLine struct {
	Trigger    string  `json:"触发词"`
	CodeBloack DicLine `json:"代码块"`
}

type BuildDicFunc struct {
	Name   string                                                     `json:"name"`
	Params []Param                                                    `json:"params"`
	Text   DicLine                                                    `json:"text"`
	Func   func(dic *DicInfoData, input utils.DicInputs) (any, error) `json:"-"`
}

type Param struct {
	Name    string `json:"name"`
	Default string `json:"default"`
}

type DicClassInfo struct {
	LocalValue  *Val                     `json:"变量"`
	LocalStatic []*RegDicLine            `json:"内部"`
	LocalFunc   []*BuildDicFunc          `json:"函数"`
	Special     map[string][]*RegDicLine `json:"特殊"`
}

// =================================================

type DicClass struct {
	LocalValue  *Val                   `json:"变量"`
	LocalFunc   []*BuildDic            `json:"函数"`
	LocalStatic []*BuildDic            `json:"内部"`
	Special     map[string][]*BuildDic `json:"特殊"`
}

// NewDicClass 初始化整合包类，避免字段为 nil 导致 JSON 序列化输出 null。
func NewDicClass() *DicClass {
	return &DicClass{
		LocalValue:  NewVal(),
		LocalFunc:   make([]*BuildDic, 0),
		LocalStatic: make([]*BuildDic, 0),
		Special:     make(map[string][]*BuildDic),
	}
}

type BuildValue struct {
	Head         []string               `json:"头部"`
	HeadLineNums []int                  `json:"-"` // 头部每行对应的原始文件行号（1-based）
	Dic          []*BuildDic            `json:"词库"`
	LocalStatic  []*BuildDic            `json:"内部"`
	LocalFunc    []*BuildDic            `json:"函数"`
	LocalClass   map[string]*DicClass   `json:"整合包"`
	Special      map[string][]*BuildDic `json:"特殊"`
	MyFunc       map[string]DicFunc     `json:"自定义函数"`
	BotImports   []string               `json:"bot引入"`
}

// 词库参数数据
type DicInputs struct {
	// 执行词库数据
	Dic *BuildValue
	// 变量数据
	V *DicVal
	// 输入参数数据
	Inputs *utils.DicInputs
	// 输出数据
	Output *SingleValue
}

func NewDicInputs(dic *BuildValue, v *DicVal, i *utils.DicInputs) *DicInputs {
	return &DicInputs{
		Dic:    dic,
		V:      v,
		Inputs: i,
	}
}

func NewDicInputsWithOutput(dic *BuildValue, v *DicVal, i *utils.DicInputs, output *SingleValue) *DicInputs {
	return &DicInputs{
		Dic:    dic,
		V:      v,
		Inputs: i,
		Output: output,
	}
}

// 克隆
func (v *BuildValue) Clone() *BuildValue {
	return &BuildValue{
		Head:        v.Head,
		Dic:         v.Dic,
		LocalStatic: v.LocalStatic,
		LocalFunc:   v.LocalFunc,
		LocalClass:  v.LocalClass,
		Special:     v.Special,
		MyFunc:      v.MyFunc,
	}
}

// ClassValues 返回整合包变量表（类名 -> 类变量），供 %类名.变量% 解析使用。
func (v *BuildValue) ClassValues() map[string]*Val {
	if v.LocalClass == nil {
		return nil
	}
	m := make(map[string]*Val, len(v.LocalClass))
	for name, c := range v.LocalClass {
		if c != nil {
			m[name] = c.LocalValue
		}
	}
	return m
}

// ResolveClassData 解析类标识（string 类名 或 *DicClass 实例）为类数据。
func (v *BuildValue) ResolveClassData(class any) *DicClass {
	switch c := class.(type) {
	case string:
		if c != "" {
			return v.LocalClass[c]
		}
	case *DicClass:
		return c
	}
	return nil
}

// 关闭回收
func (v *BuildValue) Close() {
	v.LocalFunc = nil
	v.LocalStatic = nil
	v.LocalClass = nil
	v.Special = nil
	v.Dic = nil
	v.Head = nil
}

// BotFuncsRegistry bot函数注册表，由各bot包在init()中自行注册，避免循环依赖
var BotFuncsRegistry = map[string]map[string]DicFunc{}
