//go:build windows

package dic_server

import "github.com/yusufpapurcu/wmi"

type winThermalZone struct {
	CurrentTemperature uint32
}

// sysCpuTemp Windows 通过 ACPI 热区读取 CPU 温度（℃），桌面机通常无数据返回 0
func sysCpuTemp() float64 {
	var dst []winThermalZone
	if err := wmi.Query("SELECT CurrentTemperature FROM MSAcpi_ThermalZoneTemperature", &dst); err != nil || len(dst) == 0 {
		return 0
	}

	// CurrentTemperature 单位是 0.1 K，取最高温（CPU 通常是热区里最热的）
	var max uint32
	for _, d := range dst {
		if d.CurrentTemperature > max {
			max = d.CurrentTemperature
		}
	}
	if max == 0 {
		return 0
	}

	c := float64(max)/10 - 273.15
	if c < 0 || c > 150 {
		return 0
	}
	return c
}
