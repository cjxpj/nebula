//go:build !windows && !darwin

package dic_server

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// sysCpuTemp 读取 CPU 温度（℃）：优先 CPU 相关的 thermal zone，否则取所有 zone 的最高温；无数据返回 0
func sysCpuTemp() float64 {
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	var fallback float64
	for _, z := range zones {
		typB, err := os.ReadFile(filepath.Join(z, "type"))
		if err != nil {
			continue
		}
		typ := strings.ToLower(strings.TrimSpace(string(typB)))

		raw, err := os.ReadFile(filepath.Join(z, "temp"))
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
		if err != nil || v <= 0 {
			continue
		}
		v /= 1000 // 毫摄氏度 -> 摄氏度

		if v > fallback {
			fallback = v
		}
		if strings.Contains(typ, "x86_pkg_temp") || strings.Contains(typ, "coretemp") || strings.Contains(typ, "cpu") {
			return v
		}
	}
	return fallback
}
