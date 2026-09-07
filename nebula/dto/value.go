package dto

import (
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/iancoleman/orderedmap"
)

// 词库变量
type DicVal struct {
	// 全局变量
	G *Val
	// 局部变量
	P *Val
}

// NewDicVal 初始化 DicVal 对象
func NewDicVal() *DicVal {
	return &DicVal{
		G: NewVal(),
		P: NewVal(),
	}
}

// value变量
type Val struct {
	mu  sync.RWMutex
	obj map[string]any
	// num 整数变量独立存储：int/int64 等整数类型直存 int64，避免存入 any 时的装箱分配。
	// 同一键在同一时刻只会出现在 obj 或 num 其中一个。
	num     map[string]int64
	objlock map[string]bool
	// once 一次性变量表：键 -> 是否一次性（//@一次性资源，读取后销毁）。
	// 由 SetOnce 登记，get 命中后消费并删除该键。
	once map[string]bool
	// hasOnce 标记本 Val 是否存在一次性变量，供 get 快速路径分流（无一次性变量走原快路径）。
	hasOnce atomic.Bool
	// slots 无锁变量槽：普通变量名经全局驻留（internVar）映射到槽号，按槽号直取，
	// 免去 map 哈希与每键加锁。仅局部变量（P）热路径使用，与 obj/num 并行维护（槽优先）。
	slots []slotCell
	// Class 变量表：类名 -> 类变量，供 %类名.变量% 解析
	Class map[string]*Val `json:"-"`
}

// slotCell 变量槽单元：isInt 为 true 时存 int64（免装箱），否则存 any；present 表示是否已赋值。
type slotCell struct {
	present bool
	isInt   bool
	i       int64
	v       any
}

// newSlotCell 依据值类型构造槽单元（整数直存 int64，其余存 any）。
func newSlotCell(val any) slotCell {
	if n, ok := toInt64Val(val); ok {
		return slotCell{present: true, isInt: true, i: n}
	}
	return slotCell{present: true, v: val}
}

// maxInt64Uint 以 uint64 表示的最大 int64，用于 uint/uint64 转 int64 时的溢出判断。
const maxInt64Uint = uint64(1<<63 - 1)

// toInt64Val 若值为整数类型且可无损表示为 int64，则返回其 int64 表示。
// 超出 int64 范围的 uint/uint64 返回 false（保持原值存于 obj，避免溢出取负）。
func toInt64Val(val any) (int64, bool) {
	switch n := val.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		if uint64(n) > maxInt64Uint {
			return 0, false
		}
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		if n > maxInt64Uint {
			return 0, false
		}
		return int64(n), true
	}
	return 0, false
}

// 回收词库变量
func (v *DicVal) Close() {
	v.G.Close()
	v.P.Close()
}

// 回收变量
func (v *Val) Close() {
	v.mu.Lock()
	for k := range v.objlock {
		delete(v.obj, k)
		delete(v.num, k)
		v.slotClearKey(k)
	}
	v.mu.Unlock()
}

// 线程变量
var GV *Val = NewVal()

// threadVarName 将词库变量名规范化为全局线程变量 GV 的键：
// 变量名前后都有下划线（_变量名_ / __变量名__ ...）时，去掉一层下划线得到线程变量键。
func threadVarName(key string) (string, bool) {
	if len(key) >= 3 && key[0] == '_' && key[len(key)-1] == '_' {
		if inner := key[1 : len(key)-1]; inner != "" {
			return inner, true
		}
	}
	return "", false
}

// ClearThreadVars 清空全部线程变量（不再保留 _ 开头的系统内部变量）。
// 同时清空对应无锁槽，避免 get 槽优先读返回残留旧值。
func ClearThreadVars() {
	GV.mu.Lock()
	for k := range GV.obj {
		delete(GV.obj, k)
		GV.slotClearKey(k)
	}
	for k := range GV.num {
		delete(GV.num, k)
		GV.slotClearKey(k)
	}
	GV.mu.Unlock()
}

// SetThreadVar 设置线程变量
func SetThreadVar(key, val string) {
	GV.Set(key, val)
}

// SetThreadVarRaw 以原始键（不做下划线规范化）写入线程变量。
func SetThreadVarRaw(key string, val any) {
	GV.set(key, val)
}

// GetThreadVarRaw 以原始键（不做下划线规范化）读取线程变量，返回是否存在。
func GetThreadVarRaw(key string) (any, bool) {
	return GV.get(key)
}

// DeleteThreadVar 删除指定线程变量
func DeleteThreadVar(key string) {
	GV.mu.Lock()
	delete(GV.obj, key)
	delete(GV.num, key)
	GV.slotClearKey(key)
	GV.mu.Unlock()
}

// NewVal 初始化 Val 对象
func NewVal() *Val {
	return &Val{
		obj:     make(map[string]any),
		num:     make(map[string]int64),
		objlock: make(map[string]bool),
	}
}

// deepCopyAny 深拷贝变量值：递归拷贝 map/slice/类实例，避免快照与主流程共享可变引用；
// 基本类型与只读对象（函数框等）保持引用。
func deepCopyAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[k] = deepCopyAny(val)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, val := range t {
			s[i] = deepCopyAny(val)
		}
		return s
	case []map[string]any:
		s := make([]map[string]any, len(t))
		for i, val := range t {
			m := make(map[string]any, len(val))
			for k, vv := range val {
				m[k] = deepCopyAny(vv)
			}
			s[i] = m
		}
		return s
	case map[string]string:
		m := make(map[string]string, len(t))
		for k, val := range t {
			m[k] = val
		}
		return m
	case []string:
		s := make([]string, len(t))
		copy(s, t)
		return s
	case *orderedmap.OrderedMap:
		if t == nil {
			return nil
		}
		return deepCopyOrderedMap(t)
	case orderedmap.OrderedMap:
		return *deepCopyOrderedMap(&t)
	case *DicClass:
		if t == nil {
			return nil
		}
		// 深拷贝实例成员变量（LocalValue），函数/类定义保持共享只读。
		var lv *Val
		if t.LocalValue != nil {
			lv = t.LocalValue.Clone()
		}
		return &DicClass{
			LocalValue: lv,
			DicFuncs:   t.DicFuncs,
			Fn:         t.Fn,
		}
	default:
		return v
	}
}

// deepCopyOrderedMap 深拷贝有序字典：保留键序、嵌套值与 escapeHTML 配置，
// 避免快照与主流程共享底层 keys/values。
func deepCopyOrderedMap(m *orderedmap.OrderedMap) *orderedmap.OrderedMap {
	nm := orderedmap.New()
	// escapeHTML 无 getter，用反射读取未导出字段以保留原配置。
	if f := reflect.ValueOf(m).Elem().FieldByName("escapeHTML"); f.IsValid() && f.Kind() == reflect.Bool {
		nm.SetEscapeHTML(f.Bool())
	}
	for _, k := range m.Keys() {
		val, _ := m.Get(k)
		nm.Set(k, deepCopyAny(val))
	}
	return nm
}

// Clone 深拷贝 Val（值快照）：异步块（#:）借用局部变量时使用，与主流程隔离。
// Class 类变量表保持共享引用（类定义为全局只读）。
func (v *Val) Clone() *Val {
	v.mu.RLock()
	obj := make(map[string]any, len(v.obj))
	for k, val := range v.obj {
		obj[k] = deepCopyAny(val)
	}
	num := make(map[string]int64, len(v.num))
	maps.Copy(num, v.num)
	objlock := make(map[string]bool, len(v.objlock))
	maps.Copy(objlock, v.objlock)
	once := make(map[string]bool, len(v.once))
	maps.Copy(once, v.once)
	hasOnce := v.hasOnce.Load()
	slots := make([]slotCell, len(v.slots))
	copy(slots, v.slots)
	for i := range slots {
		if slots[i].present && !slots[i].isInt {
			slots[i].v = deepCopyAny(slots[i].v)
		}
	}
	class := v.Class
	v.mu.RUnlock()

	nv := &Val{
		obj:     obj,
		num:     num,
		objlock: objlock,
		once:    once,
		slots:   slots,
		Class:   class,
	}
	nv.hasOnce.Store(hasOnce)
	return nv
}

// get 读取变量值。普通变量名优先查无锁槽（槽为最新写入源，字节码热路径 SlotSetInt64
// 直写槽后 num/obj 尚未同步），未命中再查 num/obj 映射。槽始终不旧于 map，因此槽优先读语义正确。
// 存在一次性变量（hasOnce）时走 getOnce：命中一次性键后销毁该键。
func (v *Val) get(key string) (any, bool) {
	if v.hasOnce.Load() {
		return v.getOnce(key)
	}
	return v.getFast(key)
}

// getFast 无一次性变量的快速读取路径（原 get 逻辑，读锁）。
func (v *Val) getFast(key string) (any, bool) {
	if isPlainVarName(key) {
		if val, ok := v.slotGet(internVar(key)); ok {
			return val, true
		}
	}
	v.mu.RLock()
	if n, ok := v.num[key]; ok {
		v.mu.RUnlock()
		return n, true
	}
	value, ok := v.obj[key]
	v.mu.RUnlock()
	return value, ok
}

// getOnce 存在一次性变量时的读取路径：普通键走快速读，一次性键命中后销毁。
func (v *Val) getOnce(key string) (any, bool) {
	if isPlainVarName(key) {
		if val, ok := v.slotGet(internVar(key)); ok {
			return val, true
		}
	}
	v.mu.Lock()
	if n, ok := v.num[key]; ok {
		if v.once[key] {
			delete(v.num, key)
			delete(v.once, key)
			v.mu.Unlock()
			v.slotClearKey(key)
			return n, true
		}
		v.mu.Unlock()
		return n, true
	}
	value, ok := v.obj[key]
	if ok && v.once[key] {
		delete(v.obj, key)
		delete(v.once, key)
		v.mu.Unlock()
		v.slotClearKey(key)
		return value, true
	}
	v.mu.Unlock()
	return value, ok
}

// getInt64 读取整数变量值，未命中返回 false。用于免装箱的数值快速路径。
// 普通变量名优先查无锁整数槽：槽存在且为整数直读；槽存在但非整数则按 GetInt64 语义
// 返回未命中（不能落到 num 映射，否则可能读到直写槽前的旧整数）；槽不存在再查 num。
func (v *Val) getInt64(key string) (int64, bool) {
	if isPlainVarName(key) {
		if val, ok := v.slotGet(internVar(key)); ok {
			if n, isInt := toInt64Val(val); isInt {
				return n, true
			}
			return 0, false
		}
	}
	v.mu.RLock()
	n, ok := v.num[key]
	v.mu.RUnlock()
	return n, ok
}

// growSlots 将槽数组扩容到至少 slot+1 长度。槽数组扩容极低频（仅首次出现新变量名时），
// 用写锁保护扩容，热路径的 slotGet/slotSet 不再加锁。
func (v *Val) growSlots(slot int32) {
	if slot < int32(len(v.slots)) {
		return
	}
	v.mu.Lock()
	if slot >= int32(len(v.slots)) {
		ns := make([]slotCell, slot+1)
		copy(ns, v.slots)
		v.slots = ns
	}
	v.mu.Unlock()
}

// slotGet 无锁读取变量槽（普通变量热路径）。slot 越界表示该变量从未在本 Val 赋值。
func (v *Val) slotGet(slot int32) (any, bool) {
	if slot < 0 || slot >= int32(len(v.slots)) {
		return nil, false
	}
	c := v.slots[slot]
	if !c.present {
		return nil, false
	}
	if c.isInt {
		return c.i, true
	}
	return c.v, true
}

// slotGetInt64 无锁读取整数变量槽，未命中返回 false。
func (v *Val) slotGetInt64(slot int32) (int64, bool) {
	if slot < 0 || slot >= int32(len(v.slots)) {
		return 0, false
	}
	c := v.slots[slot]
	if !c.present || !c.isInt {
		return 0, false
	}
	return c.i, true
}

// SlotGetInt64 导出包装：供 dic 包编译期槽快速路径跨包无锁读取整数变量槽。
func (v *Val) SlotGetInt64(slot int32) (int64, bool) {
	return v.slotGetInt64(slot)
}

// SlotGet 导出包装：供 dic 包按槽号无锁读取任意类型变量值（热路径，未命中回退 map）。
func (v *Val) SlotGet(slot int32) (any, bool) {
	return v.slotGet(slot)
}

// SlotSetInt64 无锁写入整数变量槽（免装箱，不同步 num 映射）。
// 供字节码热路径直写：槽为快速读源，num/obj 映射由调用方在块结束时调用 FlushSlotsToMap 统一同步，
// 从而保证 num/obj 始终为权威存储（GetAll/Clone/Get 等冷路径不受影响）。
func (v *Val) SlotSetInt64(slot int32, n int64) {
	v.slotSetInt64(slot, n)
}

// FlushSlotsToMap 将本 Val 中所有已赋值槽同步回 num/obj 映射（单锁冷路径）。
// 无锁热路径（SlotSetInt64 直写槽）在块结束时调用一次，保证 num/obj 权威且与槽一致。
func (v *Val) FlushSlotsToMap() {
	v.mu.Lock()
	varSlotMu.RLock()
	for slot := int32(0); slot < int32(len(v.slots)); slot++ {
		c := v.slots[slot]
		if !c.present || slot >= int32(len(varSlotNames)) {
			continue
		}
		key := varSlotNames[slot]
		if c.isInt {
			v.num[key] = c.i
			delete(v.obj, key)
		} else {
			v.writeUnsafe(key, c.v)
		}
	}
	varSlotMu.RUnlock()
	v.mu.Unlock()
}

// slotSet 无锁写入变量槽（按值类型直存 int64 或 any）。
func (v *Val) slotSet(slot int32, val any) {
	if slot < 0 {
		return
	}
	v.growSlots(slot)
	v.slots[slot] = newSlotCell(val)
}

// slotSetInt64 无锁写入整数变量槽（免装箱）。
func (v *Val) slotSetInt64(slot int32, n int64) {
	if slot < 0 {
		return
	}
	v.growSlots(slot)
	v.slots[slot] = slotCell{present: true, isInt: true, i: n}
}

// slotClearKey 清除指定普通变量名对应的槽（供仅写 map 的冷路径避免槽内残留旧值）。
func (v *Val) slotClearKey(key string) {
	if !isPlainVarName(key) {
		return
	}
	s := internVar(key)
	if s < int32(len(v.slots)) {
		v.slots[s] = slotCell{}
	}
}

// writeUnsafe 在已持有写锁时按值类型写入 num 或 obj（并清理另一侧），保证同一键只存于一处。
func (v *Val) writeUnsafe(key string, val any) {
	if n, ok := toInt64Val(val); ok {
		v.num[key] = n
		delete(v.obj, key)
	} else {
		v.obj[key] = val
		delete(v.num, key)
	}
}

// set 写入变量值（带写锁，不检查锁定状态，供 __ 前缀线程变量等内部路径使用）。
// 写穿透到无锁槽，维持「槽始终不旧于 map」的不变量，保证 get 槽优先读正确。
func (v *Val) set(key string, val any) {
	v.mu.Lock()
	v.writeUnsafe(key, val)
	v.mu.Unlock()
	if isPlainVarName(key) {
		v.slotSet(internVar(key), val)
	}
}

// setInt64 写入整数变量值（带写锁，不检查锁定状态）。
func (v *Val) setInt64(key string, val int64) {
	v.mu.Lock()
	v.num[key] = val
	delete(v.obj, key)
	v.mu.Unlock()
	if isPlainVarName(key) {
		v.slotSetInt64(internVar(key), val)
	}
}

// 生成参数跟括号
func RunTrigger(msg, trigger string, v *Val) {
	v.Set("括号0", msg)
	regex, err := regexp.Compile("^" + trigger + "$")
	if err == nil {
		matches := regex.FindStringSubmatch(msg)
		for i, val := range matches {
			if i == 0 {
				continue
			}
			key := fmt.Sprintf("括号%d", i)
			v.Set(key, val)
		}
	} else {
		debugLog.Infof("正则语法错误:%v", err)
	}
	triggerSplit := strings.Split(msg, " ")
	for i, val := range triggerSplit {
		key := fmt.Sprintf("参数%d", i)
		v.Set(key, val)
	}
}

// 变量生成参数跟括号
func ValRunTrigger(msg, trigger string, setV, v *DicVal) {
	setV.P.Set("括号0", v.Text(msg))
	regex, err := regexp.Compile("^" + trigger + "$")
	if err == nil {
		matches := regex.FindStringSubmatch(msg)
		for i, val := range matches {
			if i == 0 {
				continue
			}
			key := fmt.Sprintf("括号%d", i)
			setV.P.Set(key, v.Text(val))
		}
	} else {
		debugLog.Infof("正则语法错误:%v", err)
	}
	triggerSplit := strings.Split(msg, " ")
	for i, val := range triggerSplit {
		key := fmt.Sprintf("参数%d", i)
		setV.P.Set(key, v.Text(val))
	}
}

// 词库获取变量
func (v *DicVal) Get(key string) any {
	res := v.P.Get(key)
	if res == nil {
		return v.G.Get(key)
	}
	return res
}

// 获取全部变量
func (v *DicVal) GetAll() map[string]any {
	res := v.P.GetAll()
	gv := v.G.GetAll()
	maps.Copy(res, gv)
	return res
}

// Get 返回指定键的值
func (v *Val) Get(key string) any {
	if name, ok := threadVarName(key); ok {
		value, _ := GV.get(name)
		return value
	}
	value, _ := v.get(key)
	return value
}

// GetStr 返回指定键的值
func (v *Val) GetStr(key string) string {
	value, _ := v.get(key)
	if value, ok := value.(string); ok {
		return value
	}
	return ""
}

// GetINT 返回指定键的值
func (v *Val) GetINT(key string) int {
	value, _ := v.get(key)
	switch n := value.(type) {
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

// GetObj 返回指定键的值
func (v *Val) GetObj(key string) map[string]any {
	value, _ := v.get(key)
	if value, ok := value.(map[string]any); ok {
		return value
	}
	return make(map[string]any)
}

// GetAll 返回全部对象
func (v *Val) GetAll() map[string]any {
	v.mu.RLock()
	all := make(map[string]any, len(v.obj)+len(v.num))
	for k, value := range v.obj {
		all[k] = value
	}
	for k, n := range v.num {
		all[k] = n
	}
	v.mu.RUnlock()
	return all
}

// NewObj 添加新对象
func (v *Val) NewObj(val map[string]any) {
	v.mu.Lock()
	for k, newVal := range val {
		v.writeUnsafe(k, newVal)
	}
	// 批量覆盖后清空槽，避免残留旧值（冷路径）
	v.slots = nil
	v.mu.Unlock()
}

// 新建词库对象
func (dv *DicVal) NewDicVal(v *Val) *DicVal {
	return &DicVal{
		G: dv.G,
		P: v,
	}
}

// 覆盖obj
func (v *Val) AddObjs(key string, mapV []map[string]any) {
	v.mu.Lock()
	value := v.obj[key]
	var obj []map[string]any
	if m, ok := value.([]map[string]any); ok {
		obj = m
	}
	obj = append(obj, mapV...)
	v.obj[key] = obj
	delete(v.num, key)
	v.mu.Unlock()
	v.slotClearKey(key)
}

// Reset 重新设置对象
func (v *Val) Reset(val map[string]any) *Val {
	v.mu.Lock()
	v.obj = make(map[string]any, len(val))
	v.num = make(map[string]int64, len(val))
	for k, newVal := range val {
		v.writeUnsafe(k, newVal)
	}
	// 清空槽，避免残留旧值导致槽优先读返回过期数据
	v.slots = nil
	v.mu.Unlock()
	return v
}

// SetObj 设置指定键的值，如果操作成功返回 true，否则返回 false
func (v *Val) SetObj(key string, objkey string, val any) bool {
	v.mu.Lock()
	value := v.obj[key]
	if m, ok := value.(map[string]any); ok {
		m[objkey] = val
		v.obj[key] = m
		v.mu.Unlock()
		return true
	}
	v.mu.Unlock()
	return false
}

// SetLock 设置指定键的锁定状态
func (v *Val) SetLock(key string, val bool) *Val {
	v.mu.Lock()
	v.objlock[key] = val
	v.mu.Unlock()
	if val {
		v.slotClearKey(key)
	}
	return v
}

// Set 设置指定键的值，只有在键未被锁定时才设置
func (v *Val) Set(key string, val any) *Val {
	if name, ok := threadVarName(key); ok {
		GV.set(name, val)
		return v
	}
	locked := true
	v.mu.Lock()
	if len(v.objlock) == 0 || !v.objlock[key] {
		v.writeUnsafe(key, val)
		locked = false
	}
	v.mu.Unlock()
	// 写穿透：普通变量名同步写槽（无锁），保证槽优先读命中且 map 仍为权威（GetAll/Add 等依赖）
	if !locked && isPlainVarName(key) {
		v.slotSet(internVar(key), val)
	}
	return v
}

// SetRaw 以原始键（不做下划线规范化）写入变量，供内部对象变量（如 _请求数据_/_响应数据_/_WS连接_）绕过线程变量映射。
func (v *Val) SetRaw(key string, val any) *Val {
	v.set(key, val)
	return v
}

// GetRaw 以原始键（不做下划线规范化）读取变量。
func (v *Val) GetRaw(key string) (any, bool) {
	return v.get(key)
}

// SetOnce 设置一次性变量（//@一次性资源）：写入后仅在首次读取时返回，读取后即销毁。
// 不写无锁槽，使 %变量% 读取回退到 map 路径并触发 getOnce 消费语义。
func (v *Val) SetOnce(key string, val any) *Val {
	v.mu.Lock()
	v.writeUnsafe(key, val)
	if v.once == nil {
		v.once = make(map[string]bool)
	}
	v.once[key] = true
	v.mu.Unlock()
	v.hasOnce.Store(true)
	// 清槽，避免槽快路径直接命中返回而绕过一次性销毁语义
	v.slotClearKey(key)
	return v
}

// SetInt64 设置整数变量值（直存 int64 免装箱），只有在键未被锁定时才设置。
func (v *Val) SetInt64(key string, val int64) *Val {
	if name, ok := threadVarName(key); ok {
		GV.setInt64(name, val)
		return v
	}
	locked := true
	v.mu.Lock()
	if len(v.objlock) == 0 || !v.objlock[key] {
		v.num[key] = val
		delete(v.obj, key)
		locked = false
	}
	v.mu.Unlock()
	if !locked && isPlainVarName(key) {
		v.slotSetInt64(internVar(key), val)
	}
	return v
}

// GetInt64 读取整数变量值（直读 int64 免装箱），未命中返回 false。
func (v *Val) GetInt64(key string) (int64, bool) {
	if name, ok := threadVarName(key); ok {
		return GV.getInt64(name)
	}
	return v.getInt64(key)
}

// Add 将值添加到指定键的值后面
func (v *Val) Add(key string, val any) {
	v.mu.Lock()
	if existingVal, ok := v.obj[key].(string); ok {
		if newVal, ok := val.(string); ok {
			v.obj[key] = existingVal + newVal
		} else {
			v.writeUnsafe(key, val)
		}
	} else {
		v.writeUnsafe(key, val)
	}
	v.mu.Unlock()
	v.slotClearKey(key)
}

// HeaderAdd 将值添加到指定键的值前面
func (v *Val) HeaderAdd(key string, val any) {
	v.mu.Lock()
	if existingVal, ok := v.obj[key].(string); ok {
		if newVal, ok := val.(string); ok {
			v.obj[key] = newVal + existingVal
		} else {
			v.writeUnsafe(key, val)
		}
	} else {
		v.writeUnsafe(key, val)
	}
	v.mu.Unlock()
	v.slotClearKey(key)
}

// 获取变量值，优先从 P，再从 G
func (v *DicVal) GetVal(key string) (any, bool) {
	if name, ok := threadVarName(key); ok {
		return GV.get(name)
	}
	value, ok := v.P.get(key)
	if !ok && v.G != nil {
		value, ok = v.G.get(key)
	}
	return value, ok
}

// GetInt64 读取整数变量值（优先从 P，再从 G），未命中返回 false。用于免装箱的数值快速路径。
func (v *DicVal) GetInt64(key string) (int64, bool) {
	if name, ok := threadVarName(key); ok {
		return GV.getInt64(name)
	}
	n, ok := v.P.getInt64(key)
	if !ok && v.G != nil {
		n, ok = v.G.getInt64(key)
	}
	return n, ok
}

// GetSlotInt64 无锁按槽号读取整数变量（优先从 P，再从 G），未命中返回 false。编译期槽快速路径。
func (v *DicVal) GetSlotInt64(slot int32) (int64, bool) {
	if n, ok := v.P.slotGetInt64(slot); ok {
		return n, true
	}
	if v.G != nil {
		return v.G.slotGetInt64(slot)
	}
	return 0, false
}

// GetSlot 无锁按槽号读取变量（优先从 P，再从 G），未命中返回 false。编译期槽快速路径。
func (v *DicVal) GetSlot(slot int32) (any, bool) {
	if val, ok := v.P.slotGet(slot); ok {
		return val, true
	}
	if v.G != nil {
		return v.G.slotGet(slot)
	}
	return nil, false
}

// 获取变量值，优先从 P，再从 G
func (v *Val) GetVal(vv *Val, key string) (any, bool) {
	if name, ok := threadVarName(key); ok {
		return GV.get(name)
	}
	value, ok := v.get(key)
	if !ok && vv != nil {
		value, ok = vv.get(key)
	}
	return value, ok
}

// 词库变量
func (v *DicVal) Text(content any) any {
	return v.P.Text(v.G, content)
}

func (v *Val) Text(vv *Val, content any) any {
	str, ok := content.(string)
	if !ok {
		return content
	}
	// 无 % 的纯文本原样返回，跳过模板缓存查找与分段渲染（最常见输出路径）
	if !strings.Contains(str, "%") {
		return str
	}
	return v.renderVarSegments(vv, getVarSegments(str))
}
