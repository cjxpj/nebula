//go:build windows

package funcs

import (
	"strings"

	"github.com/cjxpj/nebula/dto"

	"golang.org/x/sys/windows"
)

// fileAttrSettable 为可通过 SetFileAttributes 修改的属性位。
// 目录/压缩/加密/离线等由系统维护，不在可设置范围内，避免设置时报错。
const fileAttrSettable = windows.FILE_ATTRIBUTE_READONLY |
	windows.FILE_ATTRIBUTE_HIDDEN |
	windows.FILE_ATTRIBUTE_SYSTEM |
	windows.FILE_ATTRIBUTE_ARCHIVE |
	windows.FILE_ATTRIBUTE_TEMPORARY |
	windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED

// fileAttrNames 属性名到可设置属性位的映射。
var fileAttrNames = map[string]uint32{
	"只读":       windows.FILE_ATTRIBUTE_READONLY,
	"隐藏":       windows.FILE_ATTRIBUTE_HIDDEN,
	"系统":       windows.FILE_ATTRIBUTE_SYSTEM,
	"存档":       windows.FILE_ATTRIBUTE_ARCHIVE,
	"临时":       windows.FILE_ATTRIBUTE_TEMPORARY,
	"非内容索引": windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED,
}

// 获取文件属性：返回 JSON 对象，键为属性名，值为是否启用。
func fileAttributeGet(d *dto.DicInputs) (any, error) {
	p, err := windows.UTF16PtrFromString(d.Inputs.String(1))
	if err != nil {
		return "{}", nil
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return "{}", nil
	}

	res := map[string]bool{
		"只读":     attrs&windows.FILE_ATTRIBUTE_READONLY != 0,
		"隐藏":     attrs&windows.FILE_ATTRIBUTE_HIDDEN != 0,
		"系统":     attrs&windows.FILE_ATTRIBUTE_SYSTEM != 0,
		"存档":     attrs&windows.FILE_ATTRIBUTE_ARCHIVE != 0,
		"目录":     attrs&windows.FILE_ATTRIBUTE_DIRECTORY != 0,
		"临时":     attrs&windows.FILE_ATTRIBUTE_TEMPORARY != 0,
		"压缩":     attrs&windows.FILE_ATTRIBUTE_COMPRESSED != 0,
		"离线":     attrs&windows.FILE_ATTRIBUTE_OFFLINE != 0,
		"非内容索引": attrs&windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED != 0,
		"加密":     attrs&windows.FILE_ATTRIBUTE_ENCRYPTED != 0,
	}

	b, err := json.Marshal(res)
	if err != nil {
		return "{}", nil
	}
	return string(b), nil
}

// 设置文件属性：第二参数为逗号分隔的属性名列表，置位指定属性；传「正常」清除全部属性。
func fileAttributeSet(d *dto.DicInputs) (any, error) {
	p, err := windows.UTF16PtrFromString(d.Inputs.String(1))
	if err != nil {
		return "false", nil
	}

	// 「正常」表示无其他属性，单独设置会清除全部属性
	if strings.TrimSpace(d.Inputs.String(2)) == "正常" {
		if err := windows.SetFileAttributes(p, windows.FILE_ATTRIBUTE_NORMAL); err != nil {
			return "false", nil
		}
		return "true", nil
	}

	// 兼容中英文逗号、顿号分隔
	replacer := strings.NewReplacer("，", ",", "、", ",")
	var mask uint32
	for _, name := range strings.Split(replacer.Replace(d.Inputs.String(2)), ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		bit, ok := fileAttrNames[name]
		if !ok {
			return "false", nil
		}
		mask |= bit
	}
	if mask == 0 {
		return "false", nil
	}

	// 在现有可设置属性基础上置位，避免清掉其他已有属性
	cur, err := windows.GetFileAttributes(p)
	if err != nil {
		return "false", nil
	}
	if err := windows.SetFileAttributes(p, (cur&fileAttrSettable)|mask); err != nil {
		return "false", nil
	}
	return "true", nil
}
