//go:build !windows && !darwin

package dic_server

import "github.com/shirou/gopsutil/disk"

// diskIOPrime 非 Windows 无需 WMI 预查询
func diskIOPrime() {}

// incDiff 安全计算差值，a > b 返回 a-b，否则返回 0
func incDiff(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return 0
}

// diskIOPercent 获取磁盘总使用率（%）
// 用 IO 忙碌时间占比计算：采样周期内读+写耗时 / 采样时长，
// 与任务管理器/psutil 口径一致
func diskIOPercent(before, after map[string]disk.IOCountersStat, dt float64) float64 {
	if dt <= 0 || len(before) == 0 || len(after) == 0 {
		return 0
	}

	var busyMs uint64
	for name, v2 := range after {
		v1, ok := before[name]
		if !ok {
			continue
		}
		busyMs += incDiff(v2.ReadTime, v1.ReadTime) + incDiff(v2.WriteTime, v1.WriteTime)
	}

	percent := float64(busyMs) / (dt * 1000) * 100
	if percent > 100 {
		percent = 100
	}
	return percent
}
