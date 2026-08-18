package funcs

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/cjxpj/nebula/dto"
)

// 将 string（二进制）转 image.Image
func decodeImageFromString(data string) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader([]byte(data)))
	return img, err
}

// 图片最多颜色：统计图片中出现次数最多的颜色，返回 #RRGGBB
func imageMostColor(d *dto.DicInputs) (any, error) {
	img, err := decodeImageFromString(d.Inputs.String(1))
	if err != nil {
		return "", fmt.Errorf("解码图片失败: %w", err)
	}

	bounds := img.Bounds()
	count := make(map[[3]uint8]int)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			key := [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
			count[key]++
		}
	}

	if len(count) == 0 {
		return "", errors.New("图片为空")
	}

	var maxKey [3]uint8
	var maxCount int
	for k, c := range count {
		if c > maxCount {
			maxCount = c
			maxKey = k
		}
	}
	return fmt.Sprintf("#%02X%02X%02X", maxKey[0], maxKey[1], maxKey[2]), nil
}

// 图片平均颜色：计算图片所有像素的平均颜色，返回 #RRGGBB
func imageAvgColor(d *dto.DicInputs) (any, error) {
	img, err := decodeImageFromString(d.Inputs.String(1))
	if err != nil {
		return "", fmt.Errorf("解码图片失败: %w", err)
	}

	bounds := img.Bounds()
	var rSum, gSum, bSum, n uint64
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			rSum += uint64(r >> 8)
			gSum += uint64(g >> 8)
			bSum += uint64(b >> 8)
			n++
		}
	}

	if n == 0 {
		return "", errors.New("图片为空")
	}
	r := uint8(rSum / n)
	g := uint8(gSum / n)
	b := uint8(bSum / n)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b), nil
}
