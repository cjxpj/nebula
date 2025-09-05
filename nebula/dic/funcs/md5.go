package funcs

import (
	"crypto/md5"
	"encoding/hex"
)

func (f *DicFunc) Md5() string {
	if f.Len == 1 {
		hash := md5.Sum([]byte(f.Inputs.String(1)))
		hashString := hex.EncodeToString(hash[:])
		return hashString
	}
	return ""
}
