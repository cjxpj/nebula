package funcs

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/cjxpj/nebula/dto"
)

func enMd5(d *dto.DicInputs) (any, error) {
	hash := md5.Sum([]byte(d.Inputs.String(1)))
	hashString := hex.EncodeToString(hash[:])
	return hashString, nil
}
