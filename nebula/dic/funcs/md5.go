package funcs

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

func enMd5(d *dto.DicInputs) (any, error) {
	hash := md5.Sum([]byte(d.Inputs.String(1)))
	hashString := hex.EncodeToString(hash[:])
	return hashString, nil
}

// 读文件MD5：读取文件内容并计算 MD5，返回十六进制字符串
func readFileMd5(d *dto.DicInputs) (any, error) {
	data, err := utils.NewFileQueue(d.Inputs.String(1)).ReadFileByte()
	if err != nil {
		return "", err
	}
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}
