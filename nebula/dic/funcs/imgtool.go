package funcs

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/corona10/goimagehash"
)

// 图片相似度
func (f *DicFunc) ImageSimilarity() (string, error) {
	if f.Len != 2 {
		return "", errors.New("参数数量错误，应为2个图片二进制字符串")
	}

	// 获取参数并断言为 string
	s1, ok1 := f.Inputs.Get(1).(string)
	s2, ok2 := f.Inputs.Get(2).(string)
	if !ok1 || !ok2 {
		return "", errors.New("参数类型错误，必须是字符串（二进制图片数据）")
	}

	similarity, err := compareImageStringSimilarity(s1, s2)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%.4f", similarity), nil
}

// 将 string（二进制图片）解码为 image.Image，并比较 hash 相似度
func compareImageStringSimilarity(s1, s2 string) (float64, error) {
	img1, err := decodeImageFromString(s1)
	if err != nil {
		return 0, fmt.Errorf("img1 解码失败: %w", err)
	}
	img2, err := decodeImageFromString(s2)
	if err != nil {
		return 0, fmt.Errorf("img2 解码失败: %w", err)
	}

	hash1, err := goimagehash.PerceptionHash(img1)
	if err != nil {
		return 0, err
	}
	hash2, err := goimagehash.PerceptionHash(img2)
	if err != nil {
		return 0, err
	}

	distance, _ := hash1.Distance(hash2)
	similarity := 1.0 - float64(distance)/64.0
	return similarity, nil
}

// 将 string（二进制）转 image.Image
func decodeImageFromString(data string) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader([]byte(data)))
	return img, err
}
