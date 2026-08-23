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

// convertTimeLayout 将 yyyy/MM/dd 等日期格式转换为 Go 的 layout 格式
func convertTimeLayout(layout string) string {
	replacements := map[string]string{
		"yyyy":   "2006",   // 年份
		"MM":     "01",     // 月份
		"dd":     "02",     // 日期
		"hh":     "03",     // 12小时制的小时
		"HH":     "15",     // 24小时制的小时
		"mm":     "04",     // 分钟
		"ss":     "05",     // 秒钟
		"Mon":    "Mon",    // 星期几的缩写
		"Monday": "Monday", // 星期几的全名
	}
	for key, value := range replacements {
		layout = strings.ReplaceAll(layout, key, value)
	}
	return layout
}

func timestampToTime(d *dto.DicInputs) (any, error) {
	timestampInt, _ := strconv.ParseInt(d.Inputs.String(1), 10, 64)
	timeObj := time.Unix(timestampInt, 0)

	// 默认返回完整时间格式，传入第二个参数时按自定义格式
	layout := "yyyy-MM-dd HH:mm:ss"
	if d.Inputs.Len() == 2 {
		layout = d.Inputs.String(2)
	}
	return timeObj.Format(convertTimeLayout(layout)), nil
}

// autoTimeLayouts 常见时间格式，用于自动识别时间字符串
var autoTimeLayouts = []string{
	"2006-01-02 15:04:05", // yyyy-MM-dd HH:mm:ss
	"2006/01/02 15:04:05", // yyyy/MM/dd HH:mm:ss
	"2006-01-02 15:04",    // yyyy-MM-dd HH:mm
	"2006/01/02 15:04",    // yyyy/MM/dd HH:mm
	"2006-01-02",          // yyyy-MM-dd
	"2006/01/02",          // yyyy/MM/dd
	"20060102",            // yyyyMMdd
	time.RFC3339,          // 2006-01-02T15:04:05Z07:00 (ISO8601)
	"2006-01-02T15:04:05", // 无时区的 ISO8601
}

// normalizeTimeStr 将各种 Unicode 连字符/破折号统一替换为 ASCII 连字符，
// 解决从网页/文档复制时间时带特殊连字符导致解析失败的问题。
func normalizeTimeStr(s string) string {
	replacer := strings.NewReplacer(
		"\u2010", "-", // HYPHEN
		"\u2011", "-", // NON-BREAKING HYPHEN
		"\u2012", "-", // FIGURE DASH
		"\u2013", "-", // EN DASH
		"\u2014", "-", // EM DASH
		"\u2015", "-", // HORIZONTAL BAR
		"\u2212", "-", // MINUS SIGN
		"\uFF0D", "-", // FULLWIDTH HYPHEN-MINUS
		"\uFE58", "-", // SMALL EM DASH
		"\uFE63", "-", // SMALL HYPHEN-MINUS
	)
	return replacer.Replace(s)
}

// parseTimeAuto 依次尝试常见时间格式解析，返回命中的时间
func parseTimeAuto(s string) (time.Time, error) {
	for _, layout := range autoTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("无法自动识别时间格式")
}

// timeToTimestamp 将时间字符串转换为秒级时间戳，未传格式时自动识别
func timeToTimestamp(d *dto.DicInputs) (any, error) {
	timeStr := normalizeTimeStr(d.Inputs.String(1))

	var (
		timeObj time.Time
		err     error
	)
	if d.Inputs.Len() == 2 {
		timeObj, err = time.Parse(convertTimeLayout(d.Inputs.String(2)), timeStr)
	} else {
		timeObj, err = parseTimeAuto(timeStr)
	}
	if err != nil {
		return nil, errors.New("时间格式解析失败：" + err.Error())
	}
	return timeObj.Unix(), nil
}
