//go:build windows

package dic_server

import (
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/yusufpapurcu/wmi"
)

// winProcessorQueue WMI 处理器队列长度
type winProcessorQueue struct {
	ProcessorQueueLength uint64
}

// loadState EMA 状态（模仿 Linux /proc/loadavg 指数平滑算法）
type loadState struct {
	load1, load5, load15 float64
	lastTime             time.Time
}

var (
	loadMu  sync.Mutex
	loadEMA loadState
)

// sysloadAvg 获取系统负载，返回 0-100 百分比值
// 模仿 Linux /proc/loadavg 的指数移动平均算法：
//   - 每次调用从 WMI 获取 ProcessorQueueLength 作为瞬时负载
//   - 用与前次调用的时间间隔计算 EMA 衰减系数
//   - load(t) = load(t-1) * e^(-dt/period) + current * (1 - e^(-dt/period))
//   - 最终转为百分比：load / cores * 100
func sysloadAvg() (map[string]any, error) {
	cores := float64(runtime.NumCPU())
	if cores < 1 {
		cores = 1
	}

	// 查询瞬时处理器队列长度
	var dst []winProcessorQueue
	if err := wmi.Query("SELECT ProcessorQueueLength FROM Win32_PerfFormattedData_PerfOS_System", &dst); err != nil || len(dst) == 0 {
		return map[string]any{"load1": 0.0, "load5": 0.0, "load15": 0.0}, nil
	}
	curr := float64(dst[0].ProcessorQueueLength)

	loadMu.Lock()
	defer loadMu.Unlock()

	now := time.Now()
	if loadEMA.lastTime.IsZero() {
		// 首次采样，直接使用瞬时值
		loadEMA = loadState{load1: curr, load5: curr, load15: curr, lastTime: now}
	} else {
		dt := now.Sub(loadEMA.lastTime).Seconds()
		if dt <= 0 {
			dt = 1 // 防御极小间隔
		}
		// 三种周期的 EMA 衰减系数
		loadEMA.load1 = emaStep(loadEMA.load1, curr, dt, 60)
		loadEMA.load5 = emaStep(loadEMA.load5, curr, dt, 300)
		loadEMA.load15 = emaStep(loadEMA.load15, curr, dt, 900)
		loadEMA.lastTime = now
	}

	toPercent := func(v float64) float64 { return v / cores * 100 }
	return map[string]any{
		"load1":  toPercent(loadEMA.load1),
		"load5":  toPercent(loadEMA.load5),
		"load15": toPercent(loadEMA.load15),
	}, nil
}

// emaStep 单步 EMA 更新：new = old * e^(-dt/period) + sample * (1 - e^(-dt/period))
func emaStep(old, sample, dt, period float64) float64 {
	decay := math.Exp(-dt / period)
	return old*decay + sample*(1-decay)
}
