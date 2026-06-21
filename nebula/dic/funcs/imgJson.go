package funcs

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/disintegration/imaging"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/webp"
)

type JsonImage struct{}

func (f *DicFunc) DrawImg() string {
	if f.Len == 1 {
		var imgData []map[string]any
		if err := json.Unmarshal([]byte(f.Inputs.String(1)), &imgData); err != nil {
			return "解析JSON失败"
		}
		DJson := &JsonImage{}
		return DJson.Draw(imgData)
	}
	return ""
}

func (j *JsonImage) Draw(jsonData []map[string]interface{}) string {

	// GIF的图像切片和延迟切片
	gif_imgs := []*image.Paletted{}
	gif_time := []int{}

	var img *image.NRGBA
	ttfDir := "private/ttf/"
	ttfFile := "font.ttf"
	imgDefaultTtf := ttfDir + ttfFile
	imgTtf, err := utils.NewFileQueue(imgDefaultTtf).ReadFileByte()
	if err != nil {
		return "请检查[" + imgDefaultTtf + "]是否正常"
	}
	img_color := color.NRGBA{0, 0, 0, 0}
	img_size := 1
	var img_dx, img_dy int

	for _, json := range jsonData {
		key := json["需求"].(string)
		var value []string
		SetValue, ok := json["参数"].([]interface{})
		if ok {
			for _, vv := range SetValue {
				if s, ok := vv.(string); ok {
					value = append(value, s)
				}
			}
		}
		valueLen := len(value)
		switch key {

		case "创建":
			switch valueLen {
			case 1:
				B64, err := base64.StdEncoding.DecodeString(value[0])
				if err != nil {
					return "Not Base64"
				}
				imgDataStr := string(B64)
				imgData := strings.NewReader(imgDataStr)
				imge, _, err := image.Decode(imgData)
				if err != nil {
					imge, err = webp.Decode(imgData)
					if err != nil {
						return "Not image"
					}
				}

				bounds := imge.Bounds()
				img_dx = bounds.Dx()
				img_dy = bounds.Dy()

				img = imaging.New(img_dx, img_dy, img_color)

				img = imaging.Paste(img, imge, image.Point{0, 0})
			case 2:

				dx, err := strconv.Atoi(value[0])
				if err != nil {
					return "非数字"
				}

				dy, err := strconv.Atoi(value[1])
				if err != nil {
					return "非数字"
				}

				img_dx = dx
				img_dy = dy

				img = imaging.New(dx, dy, img_color)
			default:
				return "参数不对"
			}
		case "重构":
			if valueLen < 1 {
				return "参数不对"
			}
			if value[0] == "圆角矩形" {
				if !(valueLen == 1 || valueLen == 5) {
					return "参数不对"
				}

				var rTopLeft, rTopRight, rBottomLeft, rBottomRight int

				if valueLen == 1 {
					if num, err := strconv.Atoi(value[1]); err == nil {
						rTopLeft = num
						rTopRight = num
						rBottomLeft = num
						rBottomRight = num
					}
				}

				if valueLen == 5 {
					if num, err := strconv.Atoi(value[1]); err == nil {
						rTopLeft = num
					}
					if num, err := strconv.Atoi(value[2]); err == nil {
						rTopRight = num
					}
					if num, err := strconv.Atoi(value[3]); err == nil {
						rBottomLeft = num
					}
					if num, err := strconv.Atoi(value[4]); err == nil {
						rBottomRight = num
					}
				}

				for x := 0; x < img_dx; x++ {
					for y := 0; y < img_dy; y++ {
						if x < rTopLeft && y < rTopLeft {
							if (x-rTopLeft)*(x-rTopLeft)+(y-rTopLeft)*(y-rTopLeft) > rTopLeft*rTopLeft {
								img.Set(x, y, img_color)
							}
						}

						if x >= img_dx-rTopRight && y < rTopRight {
							if (x-(img_dx-rTopRight))*(x-(img_dx-rTopRight))+(y-rTopRight)*(y-rTopRight) > rTopRight*rTopRight {
								img.Set(x, y, img_color)
							}
						}

						if x < rBottomLeft && y >= img_dy-rBottomLeft {
							if (x-rBottomLeft)*(x-rBottomLeft)+(y-(img_dy-rBottomLeft))*(y-(img_dy-rBottomLeft)) > rBottomLeft*rBottomLeft {
								img.Set(x, y, img_color)
							}
						}

						if x >= img_dx-rBottomRight && y >= img_dy-rBottomRight {
							if (x-(img_dx-rBottomRight))*(x-(img_dx-rBottomRight))+(y-(img_dy-rBottomRight))*(y-(img_dy-rBottomRight)) > rBottomRight*rBottomRight {
								img.Set(x, y, img_color)
							}
						}
					}
				}
			}
			if value[0] == "圆形" {
				if !(valueLen == 1 || valueLen == 2) {
					return "参数不对"
				}

				// 圆润大小
				cx, cy := img_dx/2, img_dy/2
				r := cx
				if valueLen == 2 {
					if Ir, err := strconv.Atoi(value[1]); err == nil {
						r = Ir
					}
				}

				bounds := img.Bounds()

				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
						dx := float64(x - cx)
						dy := float64(y - cy)
						distanceSq := dx*dx + dy*dy
						if distanceSq > float64(r*r) {
							img.Set(x, y, img_color)
						}
					}
				}
			}

			if value[0] == "旋转" {
				if !(valueLen == 1 || valueLen == 2) {
					return "参数不对"
				}

				var f float64 = 180

				if valueLen == 2 {
					fs, err := strconv.ParseFloat(value[1], 64)
					if err == nil {
						f = fs
					}
				}

				img = imaging.Rotate(img, f, img_color)
			}

		case "二维码":
			if valueLen != 1 {
				return "参数不对"
			}

			str := value[0]
			qrCode, err := qr.Encode(str, qr.M, qr.Auto)
			if err != nil {
				return "生成失败"
			}
			qrCodes, _ := barcode.Scale(qrCode, img_dx, img_dy)
			img = imaging.Paste(img, qrCodes, image.Point{0, 0})

		case "颜色":
			if !(valueLen == 3 || valueLen == 4) {
				return "参数不对"
			}

			var R, G, B, A uint8

			if color, err := strconv.Atoi(value[0]); err == nil {
				R = uint8(color)
			}

			if color, err := strconv.Atoi(value[1]); err == nil {
				G = uint8(color)
			}

			if color, err := strconv.Atoi(value[2]); err == nil {
				B = uint8(color)
			}

			A = 255
			if valueLen == 4 {
				if color, err := strconv.Atoi(value[3]); err == nil {
					A = uint8(color)
				}
			}

			img_color = color.NRGBA{R, G, B, A}

		case "大小":
			if valueLen != 1 {
				return "参数不对"
			}
			size, err := strconv.Atoi(value[0])
			if err != nil {
				return "非数字"
			}

			img_size = size

		case "画圆":
			if valueLen != 2 {
				return "参数不对"
			}
			x, err := strconv.Atoi(value[0])
			if err != nil {
				return "非数字"
			}

			y, err := strconv.Atoi(value[1])
			if err != nil {
				return "非数字"
			}

			bounds := img.Bounds()
			center := image.Pt(x, y)
			for y := center.Y - img_size; y <= center.Y+img_size; y++ {
				for x := center.X - img_size; x <= center.X+img_size; x++ {
					// 计算当前点到圆心的距离的平方
					dx := float64(x - center.X)
					dy := float64(y - center.Y)
					distanceSq := dx*dx + dy*dy

					// 如果点在圆内或圆上，则设置颜色
					if distanceSq <= float64(img_size*img_size) && image.Pt(x, y).In(bounds) {
						img.SetNRGBA(x, y, img_color)
					}
				}
			}

		case "字体":
			if valueLen != 1 {
				return "参数不对"
			}
			imgTtf, err = utils.NewFileQueue(ttfDir + value[0]).ReadFileByte()
			if err != nil {
				if imgTtf, err = utils.NewFileQueue("system/ttf/" + value[0]).ReadFileByte(); err != nil {
					return "请检查字体是否正常"
				}
			}

		case "伽马值":
			if valueLen != 1 {
				return "参数不对"
			}
			f, err := strconv.ParseFloat(value[0], 64)
			if err != nil {
				return "非数字"
			}
			img = imaging.AdjustGamma(img, f)

		case "模糊":
			if valueLen != 1 {
				return "参数不对"
			}
			f, err := strconv.ParseFloat(value[0], 64)
			if err != nil {
				return "非数字"
			}
			img = imaging.Blur(img, f)

		case "锐化":
			if valueLen != 1 {
				return "参数不对"
			}
			f, err := strconv.ParseFloat(value[0], 64)
			if err != nil {
				return "非数字"
			}
			img = imaging.Sharpen(img, f)

		case "对比度":
			if valueLen != 1 {
				return "参数不对"
			}
			f, err := strconv.ParseFloat(value[0], 64)
			if err != nil {
				return "非数字"
			}
			img = imaging.AdjustContrast(img, f)

		case "亮度":
			if valueLen != 1 {
				return "参数不对"
			}
			f, err := strconv.ParseFloat(value[0], 64)
			if err != nil {
				return "非数字"
			}
			img = imaging.AdjustBrightness(img, f)

		case "饱和度":
			if valueLen != 1 {
				return "参数不对"
			}
			f, err := strconv.ParseFloat(value[0], 64)
			if err != nil {
				return "非数字"
			}
			img = imaging.AdjustBrightness(img, f)

		case "贴图":
			if !(valueLen == 1 || valueLen == 3 || valueLen == 5 || valueLen == 6) {
				return "参数不对"
			}

			var x, y, dx, dy int

			if valueLen == 3 || valueLen == 5 || valueLen == 6 {
				x, err = strconv.Atoi(value[1])
				if err != nil {
					return "x0非数字"
				}

				y, err = strconv.Atoi(value[2])
				if err != nil {
					return "y0非数字"
				}
			}

			if valueLen == 5 || valueLen == 6 {
				dx, err = strconv.Atoi(value[3])
				if err != nil {
					return "x1非数字"
				}

				dy, err = strconv.Atoi(value[4])
				if err != nil {
					return "y2非数字"
				}
			}

			imgB64 := value[0]

			imgByte, err := base64.StdEncoding.DecodeString(imgB64)
			if err != nil {
				return "Not Base64"
			}
			imgStr := string(imgByte)
			imgData := strings.NewReader(imgStr)
			imge, _, err := image.Decode(imgData)
			if err != nil {
				imge, err = webp.Decode(imgData)
				if err != nil {
					return "创建失败"
				}
			}

			if !(valueLen == 5 || valueLen == 6) {
				bounds := imge.Bounds()
				dx = bounds.Dx()
				dy = bounds.Dy()
			}

			imge = imaging.Resize(imge, dx, dy, imaging.Lanczos)

			var opacity float64 = 1

			if valueLen == 6 {
				floatValue, err := strconv.ParseFloat(value[5], 64)
				if err != nil {
					return "透明度数值在0到1"
				}
				opacity = floatValue
			}

			// img = imaging.Paste(img, imge, image.Point{x, y})

			img = imaging.Overlay(img, imge, image.Point{x, y}, opacity)

		case "插图":
			switch valueLen {
			case 1:
				time, err := strconv.Atoi(value[0])
				if err != nil {
					return "非数字"
				}

				NewImg := image.NewPaletted(img.Bounds(), palette.Plan9)
				draw.FloydSteinberg.Draw(NewImg, img.Bounds(), img, image.Point{})

				gif_imgs = append(gif_imgs, NewImg)
				gif_time = append(gif_time, time)

			case 2:
				time, err := strconv.Atoi(value[0])
				if err != nil {
					return "非数字"
				}

				B64, err := base64.StdEncoding.DecodeString(value[1])
				if err != nil {
					return "Not Base64"
				}
				imgDataStr := string(B64)

				imgData := strings.NewReader(imgDataStr)
				imge, _, err := image.Decode(imgData)
				if err != nil {
					return "Not image"
				}

				scaledImg := imaging.Resize(imge, img_dx, img_dy, imaging.Lanczos)
				palettedImage := image.NewPaletted(scaledImg.Bounds(), palette.Plan9)
				draw.FloydSteinberg.Draw(palettedImage, scaledImg.Bounds(), scaledImg, image.Point{})

				gif_imgs = append(gif_imgs, palettedImage)
				gif_time = append(gif_time, time)

			default:
				return "参数不对"
			}

		case "画字":
			if !(valueLen == 1 || valueLen == 3) {
				return "参数不对"
			}

			var x, y int
			if valueLen == 3 {
				x, err = strconv.Atoi(value[1])
				if err != nil {
					return "非数字"
				}

				y, err = strconv.Atoi(value[2])
				if err != nil {
					return "非数字"
				}
			}

			drawStr := value[0]

			fontFace, err := truetype.Parse(imgTtf)
			if err != nil {
				return "加载字体错误"
			}

			face := truetype.NewFace(fontFace, &truetype.Options{Size: float64(img_size), DPI: 72})
			d := &font.Drawer{
				Dst:  img,
				Src:  image.NewUniform(img_color),
				Face: face,
				Dot:  fixed.P(x, y),
			}

			// 获取图片的最大宽度
			maxWidth := img.Bounds().Max.X

			// 获取行高
			lineHeight := face.Metrics().Height.Ceil()

			// 当前绘制位置
			currentX := x
			currentY := y

			for _, r := range drawStr {
				if r == '\n' {
					// 换行符处理
					currentX = x
					currentY += lineHeight
					continue
				}

				awidth, ok := d.Face.GlyphAdvance(r)
				if !ok {
					continue
				}

				// 检查字符是否超出图片边界
				if currentX+awidth.Ceil() > maxWidth {
					currentX = x
					currentY += lineHeight
				}

				d.Dot = fixed.P(currentX, currentY)
				d.DrawString(string(r))
				currentX += awidth.Ceil()
			}

		case "画点":
			if !(valueLen >= 2) {
				return "参数不对"
			}

			if valueLen%2 != 0 {
				return "非双数"
			}

			for i := 0; i < valueLen; i += 2 {
				x, errX := strconv.Atoi(value[i])
				y, errY := strconv.Atoi(value[i+1])
				if errX != nil || errY != nil {
					return "参数不是整数"
				}
				if x < 0 || x >= img.Bounds().Dx() || y < 0 || y >= img.Bounds().Dy() {
					return "点超出图像范围"
				}
				img.Set(x, y, img_color)
			}

		case "画方":
			if valueLen != 4 {
				return "参数不对"
			}
			x, err := strconv.Atoi(value[0])
			if err != nil {
				return "非数字"
			}

			y, err := strconv.Atoi(value[1])
			if err != nil {
				return "非数字"
			}

			dx, err := strconv.Atoi(value[2])
			if err != nil {
				return "非数字"
			}

			dy, err := strconv.Atoi(value[3])
			if err != nil {
				return "非数字"
			}

			rect := image.Rect(x, y, dx, dy)
			draw.Draw(img, rect, &image.Uniform{img_color}, image.Point{}, draw.Over)

		case "画线":
			if valueLen != 4 {
				return "参数不对"
			}
			x0, err := strconv.Atoi(value[0])
			if err != nil {
				return "非数字"
			}

			y0, err := strconv.Atoi(value[1])
			if err != nil {
				return "非数字"
			}

			x1, err := strconv.Atoi(value[2])
			if err != nil {
				return "非数字"
			}

			y1, err := strconv.Atoi(value[3])
			if err != nil {
				return "非数字"
			}

			// 差值和方向
			dx := x1 - x0
			if dx < 0 {
				dx = -dx
			}
			dy := y1 - y0
			if dy < 0 {
				dy = -dy
			}
			sx := -1
			sy := -1
			if x0 < x1 {
				sx = 1
			}
			if y0 < y1 {
				sy = 1
			}
			drawErr := dx - dy

			for {
				// 粗细
				for tx := -img_size / 2; tx <= img_size/2; tx++ {
					for ty := -img_size / 2; ty <= img_size/2; ty++ {
						nx, ny := x0+tx, y0+ty
						if nx >= 0 && nx < img_dx && ny >= 0 && ny < img_dy {
							img.Set(nx, ny, img_color)
						}
					}
				}
				if x0 == x1 && y0 == y1 {
					break
				}
				e2 := 2 * drawErr
				if e2 > -dy {
					drawErr -= dy
					x0 += sx
				}
				if e2 < dx {
					drawErr += dx
					y0 += sy
				}
			}

		case "画虚线":
			if valueLen != 6 {
				return "参数不对"
			}
			x0, err := strconv.Atoi(value[0])
			if err != nil {
				return "非数字"
			}

			y0, err := strconv.Atoi(value[1])
			if err != nil {
				return "非数字"
			}

			x1, err := strconv.Atoi(value[2])
			if err != nil {
				return "非数字"
			}

			y1, err := strconv.Atoi(value[3])
			if err != nil {
				return "非数字"
			}

			dashLength, err := strconv.Atoi(value[4])
			if err != nil {
				return "非数字"
			}

			spaceLength, err := strconv.Atoi(value[5])
			if err != nil {
				return "非数字"
			}

			// 计算直线的差值和方向
			dx := x1 - x0
			if dx < 0 {
				dx = -dx
			}
			dy := y1 - y0
			if dy < 0 {
				dy = -dy
			}
			sx := -1
			sy := -1
			if x0 < x1 {
				sx = 1
			}
			if y0 < y1 {
				sy = 1
			}
			drawErr := dx - dy

			draw := true
			dashRemaining := dashLength

			for {
				if draw {
					for tx := -img_size / 2; tx <= img_size/2; tx++ {
						for ty := -img_size / 2; ty <= img_size/2; ty++ {
							nx, ny := x0+tx, y0+ty
							if nx >= 0 && nx < img.Bounds().Dx() && ny >= 0 && ny < img.Bounds().Dy() {
								img.Set(nx, ny, img_color)
							}
						}
					}
				}
				if x0 == x1 && y0 == y1 {
					break
				}
				e2 := 2 * drawErr
				if e2 > -dy {
					drawErr -= dy
					x0 += sx
				}
				if e2 < dx {
					drawErr += dx
					y0 += sy
				}
				dashRemaining--
				if dashRemaining == 0 {
					draw = !draw
					if draw {
						dashRemaining = dashLength
					} else {
						dashRemaining = spaceLength
					}
				}
			}

		case "输出":
			if valueLen != 1 {
				return "参数不对"
			}

			sendType := value[0]

			if sendType == "信息" {
				return "{\"dx\":" + strconv.Itoa(img_dx) + ",\"dy\":" + strconv.Itoa(img_dy) + "}"
			}

			var sendImgData []byte

			var err error

			var buf bytes.Buffer

			switch sendType {
			case "png":
				err = imaging.Encode(&buf, img, imaging.PNG)
				if err != nil {
					return "null"
				}
				sendImgData = buf.Bytes()
			case "jpeg", "jpg":
				err = imaging.Encode(&buf, img, imaging.JPEG)
				if err != nil {
					return "null"
				}
				sendImgData = buf.Bytes()
			case "gif":
				err := gif.EncodeAll(&buf, &gif.GIF{
					Image: gif_imgs,
					Delay: gif_time,
				})
				if err != nil {
					return "null"
				}
				sendImgData = buf.Bytes()
			default:
				return "不支持的图像类型"
			}

			imgStr := string(sendImgData)
			return imgStr

		}
	}
	return ""
}

func drawImg(d *dto.DicInputs) (any, error) {
	var imgData []map[string]any
	if err := json.Unmarshal([]byte(d.Inputs.String(1)), &imgData); err != nil {
		return "解析JSON失败", nil
	}
	return (&JsonImage{}).Draw(imgData), nil
}
