//go:build darwin

package dic_server

func getSysStatus() (map[string]any, error) {
	return map[string]any{
		"cpu":     map[string]any{"percent": 0, "cores": 0},
		"mem":     map[string]any{"total": uint64(0), "used": uint64(0), "free": uint64(0), "percent": 0.0},
		"disk_io": map[string]any{"read_rate": 0.0, "write_rate": 0.0, "percent": 0.0},
		"disk":    []any{},
		"host":    map[string]any{"hostname": "", "os": "darwin", "platform": "macOS", "uptime": uint64(0), "arch": ""},
		"time":    int64(0),
		"run_time": int64(0),
	}, nil
}
