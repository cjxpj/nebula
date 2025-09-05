package funcs

import (
	"net"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
)

func (f *DicFunc) Host_information() string {
	if f.Len == 1 {
		switch f.Inputs.String(1) {
		case "CPU":
			Sinfo, err := cpu.Percent(time.Second, false)
			if err != nil {
				return "error"
			}
			str := strconv.FormatFloat(Sinfo[0], 'f', 2, 64)
			return str
		case "CPU信息":
			Sinfo, err := cpu.Info()
			if err != nil {
				return "error"
			}
			if resJson, err := json.Marshal(Sinfo); err == nil {
				return string(resJson)
			}
			return "error"
		case "内存":
			Sinfo, err := mem.VirtualMemory()
			if err != nil {
				return "error"
			}
			if resJson, err := json.Marshal(Sinfo); err == nil {
				return string(resJson)
			}
			return "error"
		case "磁盘":
			Sinfo, err := disk.Partitions(true)
			if err != nil {
				return "error"
			}
			if resJson, err := json.Marshal(Sinfo); err == nil {
				return string(resJson)
			}
			return "error"
		case "网络":
			Sinfo, err := net.Interfaces()
			if err != nil {
				return "error"
			}
			if resJson, err := json.Marshal(Sinfo); err == nil {
				return string(resJson)
			}
			return "error"
		}
	}
	return ""
}
