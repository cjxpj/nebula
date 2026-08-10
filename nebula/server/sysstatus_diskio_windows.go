//go:build windows

package dic_server

import (
	"github.com/shirou/gopsutil/disk"
	"github.com/yusufpapurcu/wmi"
)

type winPerfDisk struct {
	Name            string
	PercentDiskTime uint64
}

// diskIOPrime 在 cpu.Percent 300ms 延时前执行 WMI 预查询，
// 使 diskIOPercent 拿到的 PercentDiskTime 覆盖精确的 300ms 间隔，
// 与任务管理器采样口径一致
func diskIOPrime() {
	var probe []winPerfDisk
	wmi.Query("SELECT Name, PercentDiskTime FROM Win32_PerfFormattedData_PerfDisk_PhysicalDisk", &probe)
}

// diskIOPercent 获取磁盘总使用率（%）
// Windows 上 gopsutil 的 ReadTime/WriteTime 不更新，改用 WMI PercentDiskTime，
// 该值反映自 diskIOPrime 以来约 300ms 的磁盘繁忙占比，与任务管理器同源且无需管理员权限
func diskIOPercent(_, _ map[string]disk.IOCountersStat, _ float64) float64 {
	var dst []winPerfDisk
	if err := wmi.Query("SELECT Name, PercentDiskTime FROM Win32_PerfFormattedData_PerfDisk_PhysicalDisk", &dst); err != nil {
		return 0
	}

	// _Total 在该类下恒为 0，取各物理盘平均值
	var total, sum uint64
	for _, d := range dst {
		if d.Name == "_Total" {
			continue
		}
		sum += d.PercentDiskTime
		total++
	}
	if total == 0 {
		return 0
	}
	return normDiskPercent(sum / total)
}

func normDiskPercent(v uint64) float64 {
	if v > 100 {
		return float64(v) / 10
	}
	return float64(v)
}
