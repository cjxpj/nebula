package utils

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/png"
	"os"
)

// 固定头部标识
var nebulaHeader = []byte("Nebula")

// -------------------- 封装函数 --------------------

// SetImgData 把 data 写入 inPath 图片，返回新的 PNG 图片字节
func SetImgData(inPath string, data []byte) ([]byte, error) {
	img := mustOpen(inPath)

	// 带上头部
	payload := append(nebulaHeader, data...)

	stego := embed(img, payload)

	// 编码为 PNG 字节
	var buf bytes.Buffer
	if err := png.Encode(&buf, stego); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ReadImgData 从图片 path 中读取隐藏的数据
func ReadImgData(path string) ([]byte, error) {
	img := mustOpen(path)
	data := extract(img)

	// 校验头部
	if len(data) < len(nebulaHeader) || !bytes.Equal(data[:len(nebulaHeader)], nebulaHeader) {
		return nil, errors.New("未找到 Nebula 头部标识，可能不是有效的隐写数据")
	}

	// 去掉头部
	return data[len(nebulaHeader):], nil
}

// -------------------- 内部实现 --------------------

// 把 data 写进 img，返回新图像
func embed(img image.Image, data []byte) *image.RGBA {
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)

	// 拷贝原图
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}

	// 写长度（4 字节）
	length := uint32(len(data))
	var header bytes.Buffer
	binary.Write(&header, binary.LittleEndian, length)
	all := append(header.Bytes(), data...)

	idx := 0
	totalBits := len(all) * 8

	for y := bounds.Min.Y; y < bounds.Max.Y && idx < totalBits; y++ {
		for x := bounds.Min.X; x < bounds.Max.X && idx < totalBits; x++ {
			pixel := rgba.RGBAAt(x, y)
			bytePos := idx / 8
			bitPos := uint(idx % 8)
			bit := (all[bytePos] >> bitPos) & 1

			// 改 R 通道最低位
			pixel.R = (pixel.R & 0xFE) | bit
			rgba.SetRGBA(x, y, pixel)
			idx++
		}
	}
	if idx < totalBits {
		panic("图片太小，写不下")
	}
	return rgba
}

// 从 img 提取隐藏数据
func extract(img image.Image) []byte {
	bounds := img.Bounds()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		// 强制拷贝为 RGBA
		tmp := image.NewRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				tmp.Set(x, y, img.At(x, y))
			}
		}
		rgba = tmp
	}

	// 先读 4 字节长度
	var length uint32
	for i := 0; i < 32; i++ {
		x, y := coords(i, bounds)
		pixel := rgba.RGBAAt(x, y)
		bit := pixel.R & 1
		length |= uint32(bit) << i
	}

	data := make([]byte, length)
	for i := 0; i < int(length*8); i++ {
		x, y := coords(i+32, bounds)
		pixel := rgba.RGBAAt(x, y)
		bit := pixel.R & 1
		data[i/8] |= bit << (i % 8)
	}
	return data
}

// 顺序扫描像素，返回第 n 个 bit 对应的 x,y
func coords(n int, bounds image.Rectangle) (int, int) {
	w := bounds.Max.X - bounds.Min.X
	x := bounds.Min.X + (n % w)
	y := bounds.Min.Y + (n / w)
	return x, y
}

// -------------------- 文件工具 --------------------
func mustOpen(name string) image.Image {
	f, err := os.Open(name)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		panic(err)
	}
	return img
}
