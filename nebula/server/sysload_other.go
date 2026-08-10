//go:build !windows

package dic_server

import (
	"runtime"

	"github.com/shirou/gopsutil/load"
)

// sysloadAvg 获取系统负载，返回 0-100 百分比值（负载 / 核心数 * 100）
func sysloadAvg() (map[string]any, error) {
	avg, err := load.Avg()
	if err != nil {
		return nil, err
	}
	cores := float64(runtime.NumCPU())
	if cores < 1 {
		cores = 1
	}
	return map[string]any{
		"load1":  avg.Load1 / cores * 100,
		"load5":  avg.Load5 / cores * 100,
		"load15": avg.Load15 / cores * 100,
	}, nil
}
