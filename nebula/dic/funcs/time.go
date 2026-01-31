package funcs

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dto"
)

func timeSince(d *dto.DicInputs) (any, error) {
	sec, ok := d.Inputs.Get(1).(time.Time)
	if !ok {
		return nil, errors.New("传入参数要为时间")
	}
	// 计算距今多久
	duration := time.Since(sec)

	return duration.String(), nil
}

func appSleep(d *dto.DicInputs) (any, error) {
	ms, ok := d.Inputs.IntOk(1)
	if !ok {
		switch d.Inputs.String(1) {
		case "s", "一秒":
			time.Sleep(1 * time.Second)
		case "m", "一分":
			time.Sleep(1 * time.Minute)
		case "h", "一小时":
			time.Sleep(1 * time.Hour)
		case "d", "一天":
			time.Sleep(24 * time.Hour)
		case "整点":
			// 下一个整点
			nextHour := time.Now().Truncate(time.Hour).Add(time.Hour)
			time.Sleep(time.Until(nextHour))
		case "整分":
			// 下一分钟
			nextMinute := time.Now().Truncate(time.Minute).Add(time.Minute)
			time.Sleep(time.Until(nextMinute))
		default:
			return "", errors.New("传入参数要为毫秒")
		}
		return "", nil
	}

	// 毫秒数
	duration := time.Duration(ms) * time.Millisecond
	time.Sleep(duration)
	return "", nil
}

func (f *DicFunc) TimestampFormattingTime() string {
	if f.Len == 2 {
		layout := f.Inputs.String(2)
		replacements := map[string]string{
			"yyyy": "2006", // 年份

			"MM": "01", // 月份

			"dd": "02", // 日期

			"hh": "03", // 12小时制的小时

			"HH": "15", // 24小时制的小时

			"mm": "04", // 分钟

			"ss": "05", // 秒钟

			"Mon":    "Mon",    // 星期几的缩写
			"Monday": "Monday", // 星期几的全名
		}
		for key, value := range replacements {
			layout = strings.ReplaceAll(layout, key, value)
		}

		timestampInt, _ := strconv.ParseInt(f.Inputs.String(1), 10, 64)
		timeObj := time.Unix(timestampInt, 0)

		timeStr := timeObj.Format(layout)
		return timeStr
	}
	return ""
}
