package funcs

import (
	"errors"
	"fmt"

	"github.com/cjxpj/nebula/dto"

	gimghash "github.com/corona10/goimagehash"
)

func imageSimilarity(d *dto.DicInputs) (any, error) {
	if d.Inputs.Len() != 2 {
		return "", errors.New("参数数量错误，应为2个图片二进制字符串")
	}
	s1, ok1 := d.Inputs.Get(1).(string)
	s2, ok2 := d.Inputs.Get(2).(string)
	if !ok1 || !ok2 {
		return "", errors.New("参数类型错误，必须是字符串（二进制图片数据）")
	}
	img1, err := decodeImageFromString(s1)
	if err != nil {
		return "", fmt.Errorf("img1 解码失败: %w", err)
	}
	img2, err := decodeImageFromString(s2)
	if err != nil {
		return "", fmt.Errorf("img2 解码失败: %w", err)
	}
	hash1, err := gimghash.PerceptionHash(img1)
	if err != nil {
		return "", err
	}
	hash2, err := gimghash.PerceptionHash(img2)
	if err != nil {
		return "", err
	}
	distance, _ := hash1.Distance(hash2)
	similarity := 1.0 - float64(distance)/64.0
	return fmt.Sprintf("%.4f", similarity), nil
}
