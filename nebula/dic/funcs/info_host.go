//go:build !darwin && !js

package funcs

import (
	"errors"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/process"
)

func host_information(d *dto.DicInputs) (any, error) {
	switch d.Inputs.String(1) {
	case "程序CPU":
		p, err := process.NewProcess(int32(os.Getpid()))
		if err != nil {
			return "error", nil
		}
		percent, err := p.CPUPercent()
		if err != nil {
			return "error", nil
		}
		// 转成千分位后取整（保留 0 位小数）
		return strconv.Itoa(int(percent)), nil

	case "程序内存":
		p, err := process.NewProcess(int32(os.Getpid()))
		if err != nil {
			return "error", nil
		}
		memInfo, err := p.MemoryInfo()
		if err != nil {
			return "error", nil
		}
		mb := memInfo.RSS / 1024 / 1024
		return strconv.FormatInt(int64(mb), 10), nil
	case "CPU":
		Sinfo, err := cpu.Percent(time.Second, false)
		if err != nil || len(Sinfo) == 0 {
			return "error", nil
		}
		str := strconv.FormatFloat(Sinfo[0], 'f', 2, 64)
		return str, nil
	case "CPU百分比":
		Sinfo, err := cpu.Percent(time.Second, false)
		if err != nil || len(Sinfo) == 0 {
			return "error", nil
		}
		str := strconv.Itoa(int(Sinfo[0]))
		return str, nil
	case "CPU信息":
		Sinfo, err := cpu.Info()
		if err != nil {
			return "error", nil
		}
		if resJson, err := json.Marshal(Sinfo); err == nil {
			return string(resJson), nil
		}
		return "error", nil
	case "内存":
		Sinfo, err := mem.VirtualMemory()
		if err != nil {
			return "error", nil
		}
		if resJson, err := json.Marshal(Sinfo); err == nil {
			return string(resJson), nil
		}
		return "error", nil
	case "内存百分比":
		v, err := mem.VirtualMemory()
		if err != nil {
			return "error", nil
		}
		return strconv.Itoa(int(v.UsedPercent)), nil
	case "磁盘":
		Sinfo, err := disk.Partitions(true)
		if err != nil {
			return "error", nil
		}
		if resJson, err := json.Marshal(Sinfo); err == nil {
			return string(resJson), nil
		}
		return "error", nil
	case "网络":
		Sinfo, err := net.Interfaces()
		if err != nil {
			return "error", nil
		}
		if resJson, err := json.Marshal(Sinfo); err == nil {
			return string(resJson), nil
		}
		return "error", nil
	}
	return "", errors.New("未知参数")
}
