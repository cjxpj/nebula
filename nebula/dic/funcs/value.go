package funcs

import (
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
	"github.com/patrickmn/go-cache"
)

var (
	tempStore *cache.Cache
	tempOnce  sync.Once
)

func getTempStore() *cache.Cache {
	tempOnce.Do(func() {
		interval := 60
		if dto.ServerConfig.Router != nil && dto.ServerConfig.Router.TempCleanupInterval > 0 {
			interval = dto.ServerConfig.Router.TempCleanupInterval
		}
		tempStore = cache.New(cache.NoExpiration, time.Duration(interval)*time.Second)
	})
	return tempStore
}

// 线程变量
func threadVar(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk("2") {
		dto.GV.Set(d.Inputs.String(1), d.Inputs.Get(2))
		return "", nil
	}
	if res := dto.GV.Get(d.Inputs.String(1)); res != nil {
		return res, nil
	}
	return "", nil
}

// 局部变量
func localVar(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk("2") {
		d.V.P.Set(d.Inputs.String(1), d.Inputs.Get(2))
		return "", nil
	}
	if res := d.V.Get(d.Inputs.String(1)); res != nil {
		return res, nil
	}
	return "", nil
}

// 存在变量
func localVarExist(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk("2") {
		d.V.P.Set(d.Inputs.String(1), d.Inputs.Get(2))
		return "false", nil
	}
	if res := d.V.P.Get(d.Inputs.String(1)); res != nil {
		return "true", nil
	}
	if res := d.V.G.Get(d.Inputs.String(1)); res != nil {
		return "true", nil
	}
	return "false", nil
}

// 全局变量
func globalVar(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk("2") {
		d.V.G.Set(d.Inputs.String(1), d.Inputs.Get(2))
		return "", nil
	}
	if res := d.V.G.Get(d.Inputs.String(1)); res != nil {
		return res, nil
	}
	return "", nil
}

// 局部变量锁
func localVarLock(d *dto.DicInputs) (any, error) {
	str := strings.SplitSeq(d.Inputs.String(1), ",")
	for s := range str {
		d.V.P.SetLock(s, true)
	}
	return "", nil
}

// 局部变量文本
func localVarText(d *dto.DicInputs) (any, error) {
	return utils.AnyIsString(d.V.P.Get(d.Inputs.String(1))), nil
}

// 临时写
func tempWrite(d *dto.DicInputs) (any, error) {
	key := d.Inputs.String(1)
	s := getTempStore()
	// 2参数: 键 值 → 永久存储
	if d.Inputs.LenOk("2") {
		s.Set(key, d.Inputs.Get(2), cache.NoExpiration)
		return "OK", nil
	}
	// 3参数: 键 值 时间 → 时间=0清除, 时间>0设置过期
	t := d.Inputs.Int(3)
	if t == 0 {
		s.Delete(key)
		return "OK", nil
	}
	s.Set(key, d.Inputs.Get(2), time.Duration(t)*time.Second)
	return "OK", nil
}

// 临时读
func tempRead(d *dto.DicInputs) (any, error) {
	key := d.Inputs.String(1)
	defVal := d.Inputs.Get(2)
	v, ok := getTempStore().Get(key)
	if !ok {
		return defVal, nil
	}
	return v, nil
}
