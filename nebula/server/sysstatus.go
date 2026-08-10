package dic_server

import (
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
)

// sysDiskUsage 单块磁盘占用信息
type sysDiskUsage struct {
	Mount   string  `json:"mount"`
	FsType  string  `json:"fs_type"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

// getSysStatus 采集系统实时状态：CPU / 内存 / 磁盘 / 负载 / 主机信息
func getSysStatus() (map[string]any, error) {
	result := make(map[string]any)

	// ===== CPU 与磁盘 IO（共用一个采样周期） =====
	// cpu.Percent 内部会休眠 300ms，正好作为磁盘 IO 的采样间隔
	start := time.Now()
	ioBefore, ioErr := disk.IOCounters()
	diskIOPrime() // Windows: 在 cpu.Percent 之前 WMI 预查询，取 300ms 后的速率

	// 采样 300ms 计算使用率，保证读数稳定同时响应够快
	percent := 0.0
	if p, err := cpu.Percent(300*time.Millisecond, false); err == nil && len(p) > 0 {
		percent = p[0]
	}

	// 核心数用 runtime.NumCPU()（逻辑核心，与任务管理器一致）
	// cpu.Info() 在 Windows 上按物理处理器包返回，条数不等于核心数
	cpuInfo := map[string]any{"percent": percent, "cores": runtime.NumCPU()}
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		cpuInfo["model"] = strings.TrimSpace(infos[0].ModelName)
	}

	// ===== 负载 =====
	loadInfo := map[string]any{"load1": 0.0, "load5": 0.0, "load15": 0.0}
	if l, err := sysloadAvg(); err == nil {
		loadInfo = l
	}
	cpuInfo["load"] = loadInfo
	result["cpu"] = cpuInfo

	// ===== 磁盘 IO（读写速率 + 使用率） =====
	diskIO := map[string]any{"read_rate": 0.0, "write_rate": 0.0, "percent": 0.0}
	ioAfter, ioAfterErr := disk.IOCounters()
	if ioErr == nil && ioAfterErr == nil {
		diskIO = calcDiskIO(ioBefore, ioAfter, time.Since(start).Seconds())
	}
	diskIO["percent"] = diskIOPercent(ioBefore, ioAfter, time.Since(start).Seconds())
	result["disk_io"] = diskIO

	// ===== 内存 =====
	memInfo := map[string]any{"total": uint64(0), "used": uint64(0), "free": uint64(0), "percent": 0.0}
	if v, err := mem.VirtualMemory(); err == nil {
		memInfo = map[string]any{
			"total":   v.Total,
			"used":    v.Used,
			"free":    v.Free,
			"percent": v.UsedPercent,
		}
	}
	result["mem"] = memInfo

	// ===== 磁盘 =====
	disks := []sysDiskUsage{}
	if parts, err := disk.Partitions(false); err == nil {
		for _, p := range parts {
			if skipVirtualFs(p.Fstype) {
				continue
			}
			usage, err := disk.Usage(p.Mountpoint)
			if err != nil || usage == nil || usage.Total == 0 {
				continue
			}
			disks = append(disks, sysDiskUsage{
				Mount:   p.Mountpoint,
				FsType:  p.Fstype,
				Total:   usage.Total,
				Used:    usage.Used,
				Free:    usage.Free,
				Percent: usage.UsedPercent,
			})
		}
	}
	result["disk"] = disks

	// ===== 主机信息 =====
	hostInfo := map[string]any{
		"hostname": "", "os": runtime.GOOS, "platform": "", "uptime": uint64(0),
		"arch": runtime.GOARCH,
	}
	if h, err := host.Info(); err == nil {
		arch := h.KernelArch
		if arch == "" {
			arch = runtime.GOARCH
		}
		hostInfo = map[string]any{
			"hostname": h.Hostname,
			"os":       runtime.GOOS,
			"platform": h.Platform + " " + h.PlatformVersion,
			"uptime":   h.Uptime,
			"arch":     arch,
		}
	}
	result["host"] = hostInfo
	result["time"] = time.Now().Unix()

	return result, nil
}

// calcDiskIO 由前后两次磁盘 IO 计数器计算读写速率（KB/s）
func calcDiskIO(before, after map[string]disk.IOCountersStat, dt float64) map[string]any {
	result := map[string]any{"read_rate": 0.0, "write_rate": 0.0}
	if dt <= 0 {
		return result
	}

	var readBytes, writeBytes uint64
	for name, v2 := range after {
		v1, ok := before[name]
		if !ok {
			continue
		}
		if v2.ReadBytes > v1.ReadBytes {
			readBytes += v2.ReadBytes - v1.ReadBytes
		}
		if v2.WriteBytes > v1.WriteBytes {
			writeBytes += v2.WriteBytes - v1.WriteBytes
		}
	}

	// 字节数 / 采样秒数 / 1024 = KB/s
	result["read_rate"] = float64(readBytes) / dt / 1024
	result["write_rate"] = float64(writeBytes) / dt / 1024

	return result
}

// incDiff 计算增量，防止计数器回绕
func incDiff(after, before uint64) uint64 {
	if after > before {
		return after - before
	}
	return 0
}

// skipVirtualFs 跳过虚拟/伪文件系统，只统计真实磁盘
func skipVirtualFs(fsType string) bool {
	switch strings.ToLower(fsType) {
	case "", "proc", "sysfs", "devpts", "devtmpfs", "cgroup", "cgroup2",
		"overlay", "tmpfs", "ramfs", "squashfs", "securityfs", "pstore",
		"autofs", "debugfs", "mqueue", "hugetlbfs", "configfs",
		"binfmt_misc", "fusectl", "tracefs", "rpc_pipefs", "nsfs":
		return true
	}
	return false
}
