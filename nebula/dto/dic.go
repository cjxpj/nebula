package dto

import (
	"database/sql"
	"strings"
	"sync/atomic"

	"github.com/cjxpj/nebula/utils"
)

// RunState 表示执行引擎当前正在收集的「框」状态。
// 任意时刻至多一个非 Normal 状态，进入框时置为对应枚举，框结束后回到 StateNormal。
type RunState uint8

const (
	StateNormal     RunState = iota // 普通行处理
	StateFunc                       // 函数框     变量:函数> ... <函数
	StateForEach                    // 遍历框     遍历> ... <遍历
	StateFor                        // 循环框     循环> ... <循环
	StateIf                         // 判断框     如果> ... <如果
	StateText                       // 纯文本框   <文本/文本>/纯文本> ... <文本
	StateValText                    // 赋予值文本框  """
	StateValTextr                   // 赋予值纯文本框 '''
	StateValChain                   // 连续执行框   >>>
	StateSetJson                    // JSON 框      JSON> ... <JSON
	StateSetNewJson                 // 新建 JSON 框 JSON>{ / JSON>[ / { / [
	StateNodeJs                     // JS 执行框    --js ... --end
)

// 系统变量
type LocalDicValue struct {
	For        LocalDicValueFor        `json:"循环框"`
	ForEach    LocalDicValueForEach    `json:"遍历框"`
	Func       LocalDicValueFunc       `json:"函数框"`
	Text       LocalDicValueText       `json:"文本框"`
	ValText    LocalDicValueValText    `json:"赋予值文本框"`
	ValTextr   LocalDicValueValTextr   `json:"赋予值纯文本框"`
	ValChain   LocalDicValueValChain   `json:"赋予值连续执行框"`
	IfFunc     LocalDicValueIfFunc     `json:"判断框"`
	SetJson    LocalDicValueSetJson    `json:"Json框"`
	SetNewJson LocalDicValueSetNewJson `json:"新建Json框"`
	NodeJs     LocalDicValueNodeJs
	Database   *sql.DB     `json:"database"`
	Stop       atomic.Bool `json:"stop"`
	State      RunState    `json:"state"`
}

// 循环框
type LocalDicValueFor struct {
	Run       any      `json:"for"`
	Num       int      `json:"num"`
	ValueName string   `json:"valueName"`
	Content   []string `json:"content"`
	IsFor     bool     `json:"IsFor"`
	Jump      bool     `json:"jump"`
	LineNums  []int    `json:"-"` // 每行内容对应的原始文件行号（1-based），用于调试报错定位
}

// 遍历框
type LocalDicValueForEach struct {
	Run       any      `json:"for"`
	Num       int      `json:"num"`
	ValueName string   `json:"valueName"`
	Content   []string `json:"content"`
	IsFor     bool     `json:"IsFor"`
	Jump      bool     `json:"jump"`
	LineNums  []int    `json:"-"` // 每行内容对应的原始文件行号（1-based），用于调试报错定位
}

// 函数框
type LocalDicValueFunc struct {
	Num       int      `json:"num"`
	ValueName string   `json:"valueName"`
	Trigger   string   `json:"trigger"`
	Content   []string `json:"content"`
	LineNums  []int    `json:"-"` // 每行内容对应的原始文件行号（1-based），用于调试报错定位
}

// 文本框
type LocalDicValueText struct {
	ReadValue bool            `json:"readValue"`
	LineFeed  string          `json:"lineFeed"`
	ValueName string          `json:"valueName"`
	Content   strings.Builder `json:"content"`
}

// 赋予值文本框
type LocalDicValueValText struct {
	ValueName string   `json:"valueName"`
	Content   []string `json:"content"`
}

// 赋予值纯文本框
type LocalDicValueValTextr struct {
	ValueName string   `json:"valueName"`
	Content   []string `json:"content"`
}

// 赋予值连续执行框
type LocalDicValueValChain struct {
	ValueName string `json:"valueName"`
}

// 判断框
type LocalDicValueIfFunc struct {
	IsElse bool       `json:"IsElse"`
	Num    int        `json:"num"`
	IfNum  int        `json:"ifnum"`
	If     []string   `json:"if"`
	Else   []string   `json:"Else"`
	Run    [][]string `json:"Run"`
	IsIf   bool       `json:"IsIf"`
	Jump   bool       `json:"jump"`
	// 每个分支（Run）与 Else 分支每行内容对应的原始文件行号，用于调试报错定位
	LineNums     [][]int `json:"-"`
	ElseLineNums []int   `json:"-"`
}

// Json框
type LocalDicValueSetJson struct {
	ValueName string `json:"valueName"`
	Json      any    `json:"json"`
	OkLen     bool   `json:"OkLen"`
	Len       int    `json:"Len"`
}

// 新建Json框
type LocalDicValueSetNewJson struct {
	ValueName string `json:"valueName"`
	Json      string `json:"json"`
	// true时候是{},false时候是[]
	JsonType bool `json:"JsonType"`
	Len      int  `json:"Len"`
	// buf 用于累积 JSON 文本，避免逐行字符串拼接带来的 O(n²) 开销
	buf strings.Builder
}

// Start 初始化新建 JSON 框：prefix 为 "{" 或 "["，valueName 为赋值目标变量名。
func (s *LocalDicValueSetNewJson) Start(prefix, valueName string) {
	s.buf.Reset()
	s.buf.WriteString(prefix)
	s.Json = prefix
	s.JsonType = true
	s.Len = 1
	s.ValueName = valueName
}

// Append 累积一行 JSON 文本。
func (s *LocalDicValueSetNewJson) Append(text string) {
	s.buf.WriteString(text)
}

// Complete 返回累积的 JSON 文本并重置，同时同步到 Json 字段供序列化查看。
func (s *LocalDicValueSetNewJson) Complete() string {
	s.Json = s.buf.String()
	s.buf.Reset()
	return s.Json
}

// JS执行框
type LocalDicValueNodeJs struct {
	Content []string `json:"content"`
}

// 词库结构
type BuildDic struct {
	Trigger  string   `json:"trigger"`
	Text     []string `json:"text"`
	LineNums []int    `json:"-"` // 每行文本对应的原始文件行号（1-based），用于调试定位
	// ParamRule 参数数量规则，如 "1|2"、"1.."；仅在 [函数|规则] 声明时非空，空串表示不校验（沿用正则匹配）。
	ParamRule string `json:"paramRule,omitempty"`
	// Desc 函数说明：来自 [函数] 上方连续的 // 注释（按行拼接）。
	Desc string `json:"desc,omitempty"`
	// TriggerLine 触发词所在原始文件行号（1-based），用于触发词正则语法等编译警告定位。
	TriggerLine int `json:"-"`
}

type DicClass struct {
	LocalValue *Val                   `json:"变量"`
	DicFuncs   map[string][]*BuildDic `json:"函数"`
	Fn         map[string]DicFunc     `json:"-"` // 自定义函数
	// funcIndex 类内函数名 -> 词条索引，惰性构建；非序列化。
	funcIndex atomic.Pointer[map[string][]*BuildDic]
}

// GetFuncIndex 返回类内函数查找索引，惰性构建并缓存。
func (c *DicClass) GetFuncIndex() map[string][]*BuildDic {
	if p := c.funcIndex.Load(); p != nil {
		return *p
	}
	idx := BuildFuncIndex(c.DicFuncs)
	if idx == nil {
		idx = map[string][]*BuildDic{}
	}
	c.funcIndex.Store(&idx)
	return idx
}

// NewDicClass 初始化 Class，避免字段为 nil 导致 JSON 序列化输出 null。
func NewDicClass() *DicClass {
	return &DicClass{
		LocalValue: NewVal(),
		DicFuncs:   make(map[string][]*BuildDic),
		Fn:         make(map[string]DicFunc),
	}
}

// BuildWarning 编译警告/错误（如循环引入、框配对错误），携带行号与级别供前端定位。
type BuildWarning struct {
	Line  int    `json:"line"`  // 触发警告的行号（1-based）
	Text  string `json:"text"`  // 警告文本
	Level string `json:"level"` // 级别：error（红色错误）/ warning（黄色警告）
}

type BuildValue struct {
	Head         []string               `json:"头部"`
	HeadLineNums []int                  `json:"-"` // 头部每行对应的原始文件行号（1-based）
	Dic          []*BuildDic            `json:"词库"`
	DicFuncs     map[string][]*BuildDic `json:"函数"`
	Class        map[string]*DicClass   `json:"class"`
	MyFunc       map[string]DicFunc     `json:"自定义函数"`
	BotImports   []string               `json:"bot引入"`
	Warnings     []BuildWarning         `json:"警告,omitempty"` // 编译警告（如循环引入），供前端调试面板展示
	// Resources 编译期资源变量（//@资源 变量名:路径 读入的文件内容），运行时注入到局部变量表。
	// 值为 string（文本）或 []byte（.n 词库自动编译的 gob 字节）。
	Resources map[string]any `json:"资源,omitempty"`
	// OnceResources 一次性资源变量名集合（//@一次性资源 变量名:路径）：内容存于 Resources，
	// 标记在此集合中的变量读取一次后即销毁。
	OnceResources map[string]bool `json:"一次性资源,omitempty"`
	// Deps 所有依赖文件（含主文件、#引入、//@资源）路径 -> 内容 sha256，用于编译缓存与打包指纹的确定性失效校验。非序列化。
	Deps map[string]string `json:"-"`
	// InHeader 运行时标记：当前是否正在执行词库头部（供 $重定向触发词$ 等仅在头部生效的功能判断）。
	InHeader bool `json:"-"`
	// funcIndex 函数名 -> 词条索引（触发词去掉 -> 后缀后作为键），惰性构建；MergeFuncs 追加后失效重建。非序列化。
	funcIndex atomic.Pointer[map[string][]*BuildDic]
	// triggerIndex 词库正文触发词匹配索引，惰性构建；Dic 编译后不变，非序列化。
	triggerIndex atomic.Pointer[TriggerIndex]
	// classValues 类名 -> 类变量表的缓存，避免每次执行（含循环体内每轮迭代）重建 map。非序列化。
	classValues atomic.Pointer[map[string]*Val]
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
	// Raw 未展开的原始参数（%变量%/[算术] 保持原样），供需要自行按 operand 边界求值的函数使用；为 nil 时退回 Inputs。
	Raw *utils.DicInputs
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

// MergeFuncs 合并运行时注入函数（按类别追加），用于 FuncText 继承。
func (v *BuildValue) MergeFuncs(fn map[string][]*BuildDic) {
	if len(fn) == 0 {
		return
	}
	if v.DicFuncs == nil {
		v.DicFuncs = make(map[string][]*BuildDic, len(fn))
	}
	for k, val := range fn {
		v.DicFuncs[k] = append(v.DicFuncs[k], val...)
	}
	// 函数表已变化，失效函数索引，下次 GetFuncIndex 时重建。
	v.funcIndex.Store(nil)
}

// BuildFuncIndex 从 DicFuncs 的「函数」类别构建函数名 -> 词条索引（触发词去掉 -> 后缀后作为键）。
func BuildFuncIndex(dicFuncs map[string][]*BuildDic) map[string][]*BuildDic {
	list := dicFuncs["函数"]
	if len(list) == 0 {
		return nil
	}
	idx := make(map[string][]*BuildDic, len(list))
	for _, item := range list {
		name := item.Trigger
		if i := strings.LastIndex(name, "->"); i != -1 {
			name = name[:i]
		}
		idx[name] = append(idx[name], item)
	}
	return idx
}

// GetFuncIndex 返回函数查找索引，惰性构建并缓存；MergeFuncs 追加后自动失效重建。
func (v *BuildValue) GetFuncIndex() map[string][]*BuildDic {
	if p := v.funcIndex.Load(); p != nil {
		return *p
	}
	idx := BuildFuncIndex(v.DicFuncs)
	if idx == nil {
		idx = map[string][]*BuildDic{}
	}
	v.funcIndex.Store(&idx)
	return idx
}

// TriggerIndex 触发词匹配索引：纯文本触发词 -> 原始下标列表（保序），正则触发词 -> 原始下标列表（保序）。
type TriggerIndex struct {
	Plain map[string][]int
	Regex []int
}

// isPlainTriggerText 判断触发词是否不含任何正则元字符，纯文本触发词可 O(1) 定位。
func isPlainTriggerText(t string) bool {
	for i := 0; i < len(t); i++ {
		switch t[i] {
		case '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '|', '^', '$', '\\':
			return false
		}
	}
	return true
}

// BuildTriggerIndex 从词条切片构建触发词匹配索引。
func BuildTriggerIndex(list []*BuildDic) *TriggerIndex {
	idx := &TriggerIndex{Plain: make(map[string][]int)}
	for i, item := range list {
		if isPlainTriggerText(item.Trigger) {
			idx.Plain[item.Trigger] = append(idx.Plain[item.Trigger], i)
		} else {
			idx.Regex = append(idx.Regex, i)
		}
	}
	return idx
}

// GetTriggerIndex 返回词库正文触发词匹配索引，惰性构建并缓存。
func (v *BuildValue) GetTriggerIndex() *TriggerIndex {
	if p := v.triggerIndex.Load(); p != nil {
		return p
	}
	idx := BuildTriggerIndex(v.Dic)
	v.triggerIndex.Store(idx)
	return idx
}

// ClassValues 返回 Class 变量表（类名 -> 类变量），供 %类名.变量% 解析使用。
func (v *BuildValue) ClassValues() map[string]*Val {
	if v.Class == nil {
		return nil
	}
	// 类表编译后不变，惰性构建一次并缓存，避免循环/遍历体内每轮迭代都重建 map。
	if cv := v.classValues.Load(); cv != nil {
		return *cv
	}
	m := make(map[string]*Val, len(v.Class))
	for name, c := range v.Class {
		if c != nil {
			m[name] = c.LocalValue
		}
	}
	v.classValues.Store(&m)
	return m
}

// ResolveClassData 解析类标识（string 类名 或 *DicClass 实例）为类数据。
func (v *BuildValue) ResolveClassData(class any) *DicClass {
	switch c := class.(type) {
	case string:
		if c != "" {
			return v.Class[c]
		}
	case *DicClass:
		return c
	}
	return nil
}

// ApplyResources 将编译期资源变量写入局部变量表（P），供头部与正文引用。
// 资源由 //@资源 指令在编译期读入并随编译结果缓存，运行时在头部执行前注入。
// 一次性资源（//@一次性资源）以 SetOnce 注入，读取一次后即销毁。
func (v *BuildValue) ApplyResources(val *DicVal) {
	if val == nil || val.P == nil {
		return
	}
	for name, content := range v.Resources {
		if v.OnceResources[name] {
			val.P.SetOnce(name, content)
		} else {
			val.P.Set(name, content)
		}
	}
}

// 关闭回收
func (v *BuildValue) Close() {
	v.DicFuncs = nil
	v.Class = nil
	v.Dic = nil
	v.Head = nil
	v.Resources = nil
	v.OnceResources = nil
}

// BotFuncsRegistry bot函数注册表，由各bot包在init()中自行注册，避免循环依赖
var BotFuncsRegistry = map[string]map[string]DicFunc{}
