package funcs

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
)

// 将 string（二进制）转 image.Image
func decodeImageFromString(data string) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader([]byte(data)))
	return img, err
}
