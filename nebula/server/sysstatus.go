//go:build !darwin

package dic_server

import (
	"os"
	"path/filepath"
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

	result["cpu"] = cpuInfo

	// ===== 磁盘 IO（读写速率 + 使用率） =====
	diskIO := map[string]any{"read_rate": 0.0, "write_rate": 0.0, "percent": 0.0}
	ioAfter, ioAfterErr := disk.IOCounters()
	if ioErr == nil && ioAfterErr == nil {
		diskIO = calcDiskIO(ioBefore, ioAfter, time.Since(start).Seconds())
	}
	diskIOPct := diskIOPercent(ioBefore, ioAfter, time.Since(start).Seconds())
	diskIO["percent"] = diskIOPct
	result["disk_io"] = diskIO

	// ===== 内存 =====
	memInfo := map[string]any{"total": uint64(0), "used": uint64(0), "free": uint64(0), "percent": 0.0}
	memPercent := 0.0
	if v, err := mem.VirtualMemory(); err == nil {
		memPercent = v.UsedPercent
		memInfo = map[string]any{
			"total":   v.Total,
			"used":    v.Used,
			"free":    v.Free,
			"percent": v.UsedPercent,
		}
	}
	result["mem"] = memInfo

	// ===== 磁盘 =====
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}

	parts, _ := disk.Partitions(false)
	appMountKey := detectAppMount(exeDir, parts)
	appMountRaw := ""

	disks := []sysDiskUsage{}
	seen := map[string]bool{}
	for _, p := range parts {
		if skipVirtualFs(p.Fstype) {
			continue
		}
		key := normalizeMount(p.Mountpoint)
		if key == "" || seen[key] {
			continue
		}
		usage, err := disk.Usage(p.Mountpoint)
		if err != nil || usage == nil || usage.Total == 0 {
			continue
		}
		seen[key] = true
		if appMountKey != "" && key == appMountKey {
			appMountRaw = p.Mountpoint
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
	// 星云程序所在磁盘不在分区列表时，单独追加
	if appMountKey != "" && !seen[appMountKey] && exeDir != "" {
		if usage, err := disk.Usage(exeDir); err == nil && usage != nil && usage.Total > 0 {
			appMountRaw = appMountName(exeDir)
			disks = append(disks, sysDiskUsage{
				Mount:   appMountRaw,
				Total:   usage.Total,
				Used:    usage.Used,
				Free:    usage.Free,
				Percent: usage.UsedPercent,
			})
		}
	}
	result["disk"] = disks
	result["app_mount"] = appMountRaw

	// ===== CPU 温度 =====
	cpuTemp := sysCpuTemp()

	// ===== 综合负载（各资源使用率加权平均） =====
	result["overall_load"] = calcOverallLoad(percent, memPercent, diskIOPct)
	result["cpu_temp"] = cpuTemp

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

// calcOverallLoad 计算综合负载：对 CPU / 内存 / 磁盘 IO 使用率加权平均。
func calcOverallLoad(cpuP, memP, ioP float64) float64 {
	// CPU 50% · 内存 30% · IO 20%
	total := 0.50*cpuP + 0.30*memP + 0.20*ioP
	if total > 100 {
		total = 100
	}
	return total
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

// pathContains 判断 dir 是否位于 mount 挂载点之下（Windows 忽略大小写）
func pathContains(dir, mount string) bool {
	if dir == "" || mount == "" {
		return false
	}
	dir = filepath.Clean(dir)
	mount = filepath.Clean(mount)
	if runtime.GOOS == "windows" {
		dir = strings.ToLower(dir)
		mount = strings.ToLower(mount)
	}
	if len(dir) < len(mount) || dir[:len(mount)] != mount {
		return false
	}
	if len(dir) == len(mount) {
		return true
	}
	// 挂载点本身以分隔符结尾，或下一字符是分隔符，才视为其子路径
	return os.IsPathSeparator(mount[len(mount)-1]) || os.IsPathSeparator(dir[len(mount)])
}

// appMountName 生成星云程序所在磁盘的挂载点名称（用于分区列表未覆盖时兜底）
func appMountName(dir string) string {
	if runtime.GOOS == "windows" {
		if vol := filepath.VolumeName(dir); vol != "" {
			return vol
		}
	}
	return dir
}

// normalizeMount 规范化挂载点，作为去重与匹配的 key。
// Windows 忽略大小写并去掉尾部 / 或 \（把 C: 和 C:\ 归一），其它平台保持原样。
func normalizeMount(m string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(strings.TrimRight(strings.TrimSpace(m), `/\`))
	}
	return m
}

// detectAppMount 计算星云程序所在磁盘的规范化挂载点。
// Windows 直接取盘符；其它平台取包含 exeDir 的最长挂载点前缀。
func detectAppMount(exeDir string, parts []disk.PartitionStat) string {
	if exeDir == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		return normalizeMount(filepath.VolumeName(exeDir))
	}
	best := ""
	for _, p := range parts {
		if p.Mountpoint != "" && pathContains(exeDir, p.Mountpoint) && len(p.Mountpoint) > len(best) {
			best = p.Mountpoint
		}
	}
	return normalizeMount(best)
}
