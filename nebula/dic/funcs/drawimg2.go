package funcs

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"

	"github.com/disintegration/imaging"
	"github.com/llgcode/draw2d/draw2dimg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type NDrawImg struct {
	img *image.RGBA // 改为 *image.RGBA

	color *color.NRGBA // 画笔颜色

	font *opentype.Font // 字体
	size float64        // 线条宽度
}

// 获取画布
func (ndi *NDrawImg) GetImage() *image.RGBA {
	return ndi.img
}

// 获取颜色
func (ndi *NDrawImg) GetColor() *color.NRGBA {
	if ndi.color == nil {
		// 随机颜色
		return &color.NRGBA{uint8(rand.Intn(256)), uint8(rand.Intn(256)), uint8(rand.Intn(256)), 255}
	}
	return ndi.color
}

// 创建画布
func drawImgNew(d *dto.DicInputs) (any, error) {
	var img *image.RGBA

	p1, p1ok := d.Inputs.IntOk(1)
	p2, p2ok := d.Inputs.IntOk(2)
	if p1ok && p2ok && p1 > 0 && p2 > 0 {
		// 新建画布模式：第三个参数是背景色
		imgColor := &color.NRGBA{0, 0, 0, 255}
		if cVal, ok := d.Inputs.Get(3).(string); ok && strings.HasPrefix(cVal, "#") {
			c, err := hexToNRGBA(cVal)
			if err == nil {
				imgColor = c
			}
		} else if c, ok := d.Inputs.Get(3).(*color.NRGBA); ok {
			imgColor = c
		}

		img = image.NewRGBA(image.Rect(0, 0, p1, p2))
		draw.Draw(img, img.Bounds(), &image.Uniform{imgColor}, image.Point{}, draw.Src)
	} else {
		param1 := d.Inputs.Get(1)
		switch v := param1.(type) {
		case *NDrawImg:
			bounds := v.img.Bounds()
			origW := bounds.Dx()
			origH := bounds.Dy()

			// 读取新高宽（留空就用原图高宽）
			p2, p2ok = d.Inputs.IntOk(2)  // 高度
			p3, p3ok := d.Inputs.IntOk(3) // 宽度
			if !p2ok || p2 <= 0 {
				p2 = origH
			}
			if !p3ok || p3 <= 0 {
				p3 = origW
			}

			resized := imaging.Resize(v.img, p3, p2, imaging.Lanczos)
			rgbaImg := image.NewRGBA(resized.Bounds())
			draw.Draw(rgbaImg, rgbaImg.Bounds(), resized, image.Point{}, draw.Src)
			img = rgbaImg

		case string:
			if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
				// 网络图片处理，保持不变
				resp, err := httpClient.Get(v)
				if err != nil {
					return nil, fmt.Errorf("下载网络图片失败: %v", err)
				}
				defer resp.Body.Close()

				imgDecoded, _, err := image.Decode(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("解码网络图片失败: %v", err)
				}

				// 后面同前面逻辑...
				bounds := imgDecoded.Bounds()
				origW := bounds.Dx()
				origH := bounds.Dy()

				p2, p2ok = d.Inputs.IntOk(2)
				p3, p3ok := d.Inputs.IntOk(3)
				if !p2ok || p2 <= 0 {
					p2 = origH
				}
				if !p3ok || p3 <= 0 {
					p3 = origW
				}

				resized := imaging.Resize(imgDecoded, p3, p2, imaging.Lanczos)
				rgbaImg := image.NewRGBA(resized.Bounds())
				draw.Draw(rgbaImg, rgbaImg.Bounds(), resized, image.Point{}, draw.Src)
				img = rgbaImg
			} else {
				// 先尝试直接把字符串当作图片二进制数据解析
				imgDecoded, _, err := image.Decode(strings.NewReader(v))
				if err != nil {
					// 解析失败当成本地文件路径读取
					pp := v
					imgFile, err := utils.NewFileQueue(pp).ReadImage()
					if err != nil {
						return nil, fmt.Errorf("打开图片失败: %v", err)
					}
					imgDecoded = imgFile
				}

				bounds := imgDecoded.Bounds()
				origW := bounds.Dx()
				origH := bounds.Dy()

				p2, p2ok = d.Inputs.IntOk(2)
				p3, p3ok := d.Inputs.IntOk(3)
				if !p2ok || p2 <= 0 {
					p2 = origH
				}
				if !p3ok || p3 <= 0 {
					p3 = origW
				}

				resized := imaging.Resize(imgDecoded, p3, p2, imaging.Lanczos)
				rgbaImg := image.NewRGBA(resized.Bounds())
				draw.Draw(rgbaImg, rgbaImg.Bounds(), resized, image.Point{}, draw.Src)
				img = rgbaImg
			}

		default:
			return nil, errors.New("参数1类型错误，必须是 *NDrawImg、图片路径字符串 或 两个正整数尺寸")
		}
	}

	// 加载字体（不变）
	ttfDir := "private/ttf/"
	ttfFile := "font.ttf"
	imgDefaultTtf := ttfDir + ttfFile
	imgTtf, err := utils.NewFileQueue(imgDefaultTtf).ReadFileByte()
	if err != nil {
		return nil, fmt.Errorf("加载字体失败：%s", err)
	}
	font, err := opentype.Parse(imgTtf)
	if err != nil {
		return nil, fmt.Errorf("解析字体失败：%s", err)
	}

	return &NDrawImg{
		img:   img,
		color: &color.NRGBA{0, 0, 0, 255}, // 这里可以保留默认
		font:  font,
		size:  1.0,
	}, nil
}

// 获取画笔颜色
func drawImgGetColor(d *dto.DicInputs) (any, error) {
	if rStr, ok := d.Inputs.Get(1).(string); ok && rStr == "随机" {
		return &color.NRGBA{uint8(rand.Intn(256)), uint8(rand.Intn(256)), uint8(rand.Intn(256)), 255}, nil
	}
	// 判断参数1是不是字符串（十六进制颜色）
	if hexStr, ok := d.Inputs.Get(1).(string); ok && strings.HasPrefix(hexStr, "#") {
		c, err := hexToNRGBA(hexStr)
		if err != nil {
			return &color.NRGBA{}, fmt.Errorf("十六进制颜色解析失败: %v", err)
		}
		return c, nil
	}

	// 否则按RGB(A)取
	r := d.Inputs.Int(1)
	g := d.Inputs.Int(2)
	b := d.Inputs.Int(3)
	a := 255
	if aa, ok := d.Inputs.IntOk(4); ok {
		a = aa
	}
	return &color.NRGBA{uint8(r), uint8(g), uint8(b), uint8(a)}, nil
}

// 设置画笔颜色
func drawImgSetColor(d *dto.DicInputs) (any, error) {
	img, ok := d.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return "", errors.New("参数1必须是画布")
	}
	if img == nil {
		return "", errors.New("图片不能为空")
	}

	if rStr, ok := d.Inputs.Get(2).(string); ok && rStr == "随机" {
		img.color = nil
		return nil, nil
	}

	// 判断参数2是不是字符串（十六进制颜色）
	if hexStr, ok := d.Inputs.Get(2).(string); ok && strings.HasPrefix(hexStr, "#") {
		c, err := hexToNRGBA(hexStr)
		if err != nil {
			return "", fmt.Errorf("十六进制颜色解析失败: %v", err)
		}
		img.color = c
		return nil, nil
	}

	// 否则看参数2是否直接是 color.NRGBA
	if c, ok := d.Inputs.Get(2).(*color.NRGBA); ok {
		img.color = c
		return nil, nil
	}

	// 否则按RGB(A)取
	r := d.Inputs.Int(2)
	g := d.Inputs.Int(3)
	b := d.Inputs.Int(4)
	a := 255
	if aa, ok := d.Inputs.IntOk(5); ok {
		a = aa
	}
	img.color = &color.NRGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
	return nil, nil
}

// hexToNRGBA 解析 #RRGGBB 或 #RRGGBBAA 格式
func hexToNRGBA(s string) (*color.NRGBA, error) {
	s = strings.TrimPrefix(s, "#")
	var r, g, b, a uint8 = 0, 0, 0, 255

	if len(s) == 6 {
		// #RRGGBB
		v, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			return nil, err
		}
		r = uint8(v >> 16)
		g = uint8((v >> 8) & 0xFF)
		b = uint8(v & 0xFF)
	} else if len(s) == 8 {
		// #RRGGBBAA
		v, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			return nil, err
		}
		r = uint8(v >> 24)
		g = uint8((v >> 16) & 0xFF)
		b = uint8((v >> 8) & 0xFF)
		a = uint8(v & 0xFF)
	} else {
		return nil, errors.New("十六进制颜色长度必须是6或8")
	}
	return &color.NRGBA{r, g, b, a}, nil
}

// 设置线条宽度
func (f *DicFunc) DrawImgSetSize() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return errors.New("参数1必须是画布")
	}
	if img == nil {
		return errors.New("图片不能为空")
	}
	size := f.Inputs.Float64(2)
	if size <= 0 {
		return errors.New("线条宽度必须大于0")
	}
	img.size = size
	return nil
}

// 画字
func (f *DicFunc) DrawImgText() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return errors.New("参数1必须是画布")
	}
	if img == nil {
		return errors.New("图片不能为空")
	}

	x := int(f.Inputs.Float64(2))
	y := int(f.Inputs.Float64(3))
	text := f.Inputs.String(4)

	// 旋转角度
	rotateDeg := 0.0
	if deg, ok := f.Inputs.Float64Ok(5); ok {
		rotateDeg = deg
	}

	c := img.GetColor()
	if cc, ok := f.Inputs.Get(6).(*color.NRGBA); ok {
		c = cc
	}

	// 判断是否有参数 7 (描边颜色)
	strokeColor, hasStroke := f.Inputs.Get(7).(*color.NRGBA)

	// 如果有描边，读参数 8 (描边宽度)
	strokeWidth := 2.0
	if hasStroke {
		if sw, ok := f.Inputs.Float64Ok(8); ok {
			strokeWidth = sw
		}
	}

	// 加载字体
	face, err := opentype.NewFace(img.font, &opentype.FaceOptions{
		Size:    img.size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return err
	}
	defer face.Close()

	// 绘制函数
	drawText := func(dst draw.Image, col color.Color, ox, oy int) {
		d := &font.Drawer{
			Dst:  dst,
			Src:  image.NewUniform(col),
			Face: face,
			Dot:  fixed.P(x+ox, y+oy),
		}
		d.DrawString(text)
	}

	if rotateDeg == 0 {
		// 不旋转
		if hasStroke {
			// 画描边（四周八方向）
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					if dx != 0 || dy != 0 {
						ox := int(float64(dx) * strokeWidth)
						oy := int(float64(dy) * strokeWidth)
						drawText(img.img, strokeColor, ox, oy)
					}
				}
			}
		}
		// 画主文本
		drawText(img.img, c, 0, 0)

	} else {
		// 旋转：先绘制到临时透明图层
		textWidth := font.MeasureString(face, text).Ceil()
		metrics := face.Metrics()
		textHeight := (metrics.Ascent + metrics.Descent).Ceil()
		textImg := image.NewNRGBA(image.Rect(0, 0, textWidth+int(strokeWidth*2), textHeight+int(strokeWidth*2)))

		if hasStroke {
			// 画描边
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					if dx != 0 || dy != 0 {
						ox := int(float64(dx) * strokeWidth)
						oy := int(float64(dy) * strokeWidth)
						d := &font.Drawer{
							Dst:  textImg,
							Src:  image.NewUniform(strokeColor),
							Face: face,
							Dot:  fixed.P(ox, metrics.Ascent.Ceil()+oy),
						}
						d.DrawString(text)
					}
				}
			}
		}

		// 画文本
		d := &font.Drawer{
			Dst:  textImg,
			Src:  image.NewUniform(c),
			Face: face,
			Dot:  fixed.P(0, metrics.Ascent.Ceil()),
		}
		d.DrawString(text)

		// 旋转
		rotated := imaging.Rotate(textImg, rotateDeg, color.NRGBA{0, 0, 0, 0})

		// 粘贴到目标图像
		offset := image.Pt(x, y)
		draw.Draw(img.img, rotated.Bounds().Add(offset), rotated, image.Point{}, draw.Over)
	}

	return nil
}

// 加载字体
func (f *DicFunc) DrawImgLoadFont() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return errors.New("参数1必须是画布")
	}

	if img == nil {
		return errors.New("图片不能为空")
	}

	ttfDir := "private/ttf/"
	ttfFile := f.Inputs.String(2)
	imgTtf, err := utils.NewFileQueue(ttfDir + ttfFile).ReadFileByte()
	if err != nil {
		return fmt.Errorf("加载字体失败：%s", err)
	}
	font, err := opentype.Parse(imgTtf)
	if err != nil {
		return fmt.Errorf("解析字体失败：%s", err)
	}
	img.font = font
	return nil
}

// 画点
func (f *DicFunc) DrawImgPoint() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return errors.New("参数1必须是画布")
	}

	if img == nil {
		return errors.New("图片不能为空")
	}

	x := int(f.Inputs.Float64(2))
	y := int(f.Inputs.Float64(3))
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(4).(*color.NRGBA); ok {
		c = cc
	}

	if x >= 0 && x < img.img.Bounds().Dx() && y >= 0 && y < img.img.Bounds().Dy() {
		img.img.Set(x, y, c)
	}
	return nil
}

// 画线
func (f *DicFunc) DrawImgLine() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return errors.New("参数1必须是画布")
	}

	if img == nil {
		return errors.New("图片不能为空")
	}

	x1 := f.Inputs.Float64(2)
	y1 := f.Inputs.Float64(3)
	x2 := f.Inputs.Float64(4)
	y2 := f.Inputs.Float64(5)
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(6).(*color.NRGBA); ok {
		c = cc
	}

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(c)
	gc.SetLineWidth(img.size)
	gc.MoveTo(x1, y1)
	gc.LineTo(x2, y2)
	gc.Stroke()
	return nil
}

// 喷漆
func (f *DicFunc) DrawImgBrushLine() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是非空画布")
	}

	// 参数6：喷漆范围半径（喷涂范围）
	rangeRadius := int(img.size)
	if val, ok := f.Inputs.IntOk(6); ok && val > 0 {
		rangeRadius = val
	}
	if rangeRadius <= 0 {
		rangeRadius = 3
	}

	// 参数7：密度 (1-100)
	density := 50
	if val, ok := f.Inputs.IntOk(7); ok {
		if val < 1 {
			density = 1
		} else if val > 100 {
			density = 100
		} else {
			density = val
		}
	}

	// 参数9：点大小（单个喷点半径）
	pointRadius := 1
	if val, ok := f.Inputs.IntOk(9); ok && val > 0 {
		pointRadius = val
	}

	// 参数2-5：起点和终点坐标
	x1 := f.Inputs.Float64(2)
	y1 := f.Inputs.Float64(3)
	x2 := f.Inputs.Float64(4)
	y2 := f.Inputs.Float64(5)

	// 参数8：颜色
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(8).(*color.NRGBA); ok {
		c = cc
	}

	dx := x2 - x1
	dy := y2 - y1
	length := math.Hypot(dx, dy)
	steps := max(int(length), 1)

	prob := float64(density) / 100.0

	// 画单点函数
	drawPoint := func(img *image.RGBA, x, y, radius int, c color.NRGBA) {
		for ox := -radius; ox <= radius; ox++ {
			for oy := -radius; oy <= radius; oy++ {
				if ox*ox+oy*oy <= radius*radius {
					px := x + ox
					py := y + oy
					if image.Pt(px, py).In(img.Bounds()) {
						img.Set(px, py, c)
					}
				}
			}
		}
	}

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		cx := int(x1 + t*dx)
		cy := int(y1 + t*dy)

		if density == 100 {
			// 整个喷涂范围全涂满点，点大小用pointRadius
			for ox := -rangeRadius; ox <= rangeRadius; ox++ {
				for oy := -rangeRadius; oy <= rangeRadius; oy++ {
					if ox*ox+oy*oy <= rangeRadius*rangeRadius {
						drawPoint(img.img, cx+ox, cy+oy, pointRadius, *c)
					}
				}
			}
		} else {
			if rand.Float64() < prob {
				pointsCount := density * 20 / 100 // 最大20个点
				if pointsCount < 1 {
					pointsCount = 1
				}
				for j := 0; j < pointsCount; j++ {
					offsetX := rand.Intn(2*rangeRadius+1) - rangeRadius
					offsetY := rand.Intn(2*rangeRadius+1) - rangeRadius
					px := cx + offsetX
					py := cy + offsetY
					drawPoint(img.img, px, py, pointRadius, *c)
				}
			}
		}
	}

	return nil
}

// 绘制波浪线
func (f *DicFunc) DrawImgWaveLine() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return errors.New("参数1必须是画布")
	}
	if img == nil {
		return errors.New("图片不能为空")
	}

	x1 := f.Inputs.Float64(2)
	y1 := f.Inputs.Float64(3)
	x2 := f.Inputs.Float64(4)
	y2 := f.Inputs.Float64(5)

	// 使用画布内置颜色
	c := img.GetColor()

	// 可选参数：波浪高度（振幅）、波长、采样步长
	waveAmplitude := 5.0
	if val, ok := f.Inputs.Float64Ok(6); ok {
		waveAmplitude = val
	}
	waveLength := 20.0
	if val, ok := f.Inputs.Float64Ok(7); ok {
		waveLength = val
	}
	step := 2.0
	if val, ok := f.Inputs.Float64Ok(8); ok && val > 0 {
		step = val
	}

	// 计算直线长度和角度
	dx := x2 - x1
	dy := y2 - y1
	length := math.Hypot(dx, dy)
	angle := math.Atan2(dy, dx)

	// 创建绘图上下文
	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(*c) // 解引用 color.NRGBA
	gc.SetLineWidth(img.size)

	// 绘制波浪路径
	for t := 0.0; t <= length; t += step {
		baseX := x1 + math.Cos(angle)*t
		baseY := y1 + math.Sin(angle)*t

		offset := waveAmplitude * math.Sin(2*math.Pi*t/waveLength)
		normalAngle := angle + math.Pi/2
		waveX := baseX + math.Cos(normalAngle)*offset
		waveY := baseY + math.Sin(normalAngle)*offset

		if t == 0 {
			gc.MoveTo(waveX, waveY)
		} else {
			gc.LineTo(waveX, waveY)
		}
	}
	gc.Stroke()
	return nil
}

// 画矩形描边
func (f *DicFunc) DrawImgRectangleStroke() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是非空画布")
	}

	x := f.Inputs.Float64(2)
	y := f.Inputs.Float64(3)
	width := f.Inputs.Float64(4)
	height := f.Inputs.Float64(5)

	// 解析圆角
	radii := [4]float64{}
	if r, ok := f.Inputs.Float64Ok(6); ok {
		radii = [4]float64{r, r, r, r}
	} else if s, ok := f.Inputs.StringOk(6); ok && s != "" {
		parts := strings.Split(s, ",")
		for i := 0; i < len(parts) && i < 4; i++ {
			if val, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64); err == nil {
				radii[i] = val
			}
		}
	}

	// 参数7：颜色
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(7).(*color.NRGBA); ok {
		c = cc
	}

	// 限制每个角不超过宽/高的一半
	for i := range radii {
		if radii[i]*2 > width {
			radii[i] = width / 2
		}
		if radii[i]*2 > height {
			radii[i] = height / 2
		}
	}

	r0, r1, r2, r3 := radii[0], radii[1], radii[2], radii[3]

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(c)
	gc.SetLineWidth(img.size)
	gc.BeginPath()

	x0 := x
	y0 := y
	x1 := x + width
	y1 := y + height

	gc.MoveTo(x0+r0, y0)

	gc.LineTo(x1-r1, y0)
	if r1 > 0 {
		gc.ArcTo(x1-r1, y0+r1, r1, r1, -math.Pi/2, math.Pi/2)
	}

	gc.LineTo(x1, y1-r2)
	if r2 > 0 {
		gc.ArcTo(x1-r2, y1-r2, r2, r2, 0, math.Pi/2)
	}

	gc.LineTo(x0+r3, y1)
	if r3 > 0 {
		gc.ArcTo(x0+r3, y1-r3, r3, r3, math.Pi/2, math.Pi/2)
	}

	gc.LineTo(x0, y0+r0)
	if r0 > 0 {
		gc.ArcTo(x0+r0, y0+r0, r0, r0, math.Pi, math.Pi/2)
	}

	gc.Close()
	gc.Stroke()
	return nil
}

// DrawImgFloodFill 使用油漆桶从指定点填充区域
func (f *DicFunc) DrawImgFloodFill() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是非空画布")
	}

	x := int(f.Inputs.Float64(2))
	y := int(f.Inputs.Float64(3))

	// 参数4: 填充颜色（可选）
	fillColor := img.GetColor()
	if c, ok := f.Inputs.Get(4).(*color.NRGBA); ok {
		fillColor = c
	}

	bounds := img.img.Bounds()
	if !image.Pt(x, y).In(bounds) {
		return errors.New("起始点不在图像范围内")
	}

	// 获取起始颜色
	startColor := img.img.At(x, y)

	// 如果颜色一样就不需要填充
	if colorsEqual(startColor, fillColor) {
		return nil
	}

	// 泛洪填充（BFS 队列实现）
	visited := make(map[image.Point]bool)
	queue := []image.Point{{x, y}}

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]

		if !p.In(bounds) || visited[p] {
			continue
		}
		if !colorsEqual(img.img.At(p.X, p.Y), startColor) {
			continue
		}

		img.img.Set(p.X, p.Y, fillColor)
		visited[p] = true

		// 4邻域扩展
		queue = append(queue,
			image.Pt(p.X+1, p.Y),
			image.Pt(p.X-1, p.Y),
			image.Pt(p.X, p.Y+1),
			image.Pt(p.X, p.Y-1),
		)
	}

	return nil
}

// colorsEqual 判断颜色是否一致（忽略透明度微差）
func colorsEqual(c1, c2 color.Color) bool {
	r1, g1, b1, a1 := c1.RGBA()
	r2, g2, b2, a2 := c2.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

// 画填充矩形
func (f *DicFunc) DrawImgRectangleFill() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是非空画布")
	}

	x := f.Inputs.Float64(2)
	y := f.Inputs.Float64(3)
	width := f.Inputs.Float64(4)
	height := f.Inputs.Float64(5)

	// 解析圆角
	radii := [4]float64{}
	if r, ok := f.Inputs.Float64Ok(6); ok {
		radii = [4]float64{r, r, r, r}
	} else if s, ok := f.Inputs.StringOk(6); ok && s != "" {
		parts := strings.Split(s, ",")
		for i := 0; i < len(parts) && i < 4; i++ {
			if val, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64); err == nil {
				radii[i] = val
			}
		}
	}

	// 参数7：颜色
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(7).(*color.NRGBA); ok {
		c = cc
	}

	// 限制每个角不超过宽/高的一半
	for i := range radii {
		if radii[i]*2 > width {
			radii[i] = width / 2
		}
		if radii[i]*2 > height {
			radii[i] = height / 2
		}
	}

	r0, r1, r2, r3 := radii[0], radii[1], radii[2], radii[3]

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetFillColor(c)
	gc.BeginPath()

	x0 := x
	y0 := y
	x1 := x + width
	y1 := y + height

	gc.MoveTo(x0+r0, y0)

	gc.LineTo(x1-r1, y0)
	if r1 > 0 {
		gc.ArcTo(x1-r1, y0+r1, r1, r1, -math.Pi/2, math.Pi/2)
	}

	gc.LineTo(x1, y1-r2)
	if r2 > 0 {
		gc.ArcTo(x1-r2, y1-r2, r2, r2, 0, math.Pi/2)
	}

	gc.LineTo(x0+r3, y1)
	if r3 > 0 {
		gc.ArcTo(x0+r3, y1-r3, r3, r3, math.Pi/2, math.Pi/2)
	}

	gc.LineTo(x0, y0+r0)
	if r0 > 0 {
		gc.ArcTo(x0+r0, y0+r0, r0, r0, math.Pi, math.Pi/2)
	}

	gc.Close()
	gc.Fill()
	return nil
}

// 画椭圆
func (f *DicFunc) DrawImgEllipse() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return errors.New("参数1必须是画布")
	}
	if img == nil {
		return errors.New("图片不能为空")
	}

	x := f.Inputs.Float64(2)
	y := f.Inputs.Float64(3)
	width := f.Inputs.Float64(4)
	height := f.Inputs.Float64(5)
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(6).(*color.NRGBA); ok {
		c = cc
	}

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(c)
	gc.SetLineWidth(img.size)

	centerX := x + width/2
	centerY := y + height/2
	radiusX := width / 2
	radiusY := height / 2

	points := ellipsePoints(centerX, centerY, radiusX, radiusY, 40)
	if len(points) > 0 {
		gc.MoveTo(points[0][0], points[0][1])
		for _, p := range points[1:] {
			gc.LineTo(p[0], p[1])
		}
		gc.Close()
		gc.Stroke()
	}
	return nil
}

// 画填充椭圆
func (f *DicFunc) DrawImgEllipseFill() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return errors.New("参数1必须是画布")
	}
	if img == nil {
		return errors.New("图片不能为空")
	}

	x := f.Inputs.Float64(2)
	y := f.Inputs.Float64(3)
	width := f.Inputs.Float64(4)
	height := f.Inputs.Float64(5)
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(6).(*color.NRGBA); ok {
		c = cc
	}

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetFillColor(c)
	// 不需要设置线宽
	// gc.SetLineWidth(img.size)

	centerX := x + width/2
	centerY := y + height/2
	radiusX := width / 2
	radiusY := height / 2

	points := ellipsePoints(centerX, centerY, radiusX, radiusY, 40)
	if len(points) > 0 {
		gc.MoveTo(points[0][0], points[0][1])
		for _, p := range points[1:] {
			gc.LineTo(p[0], p[1])
		}
		gc.Close()
		gc.Fill()
	}
	return nil
}

// DrawImgRandomDots 在画布上绘制随机点
func (f *DicFunc) DrawImgRandomDots() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是非空画布")
	}

	dotCount := 20 // 默认点数量
	if dc, ok := f.Inputs.IntOk(2); ok && dc > 0 {
		dotCount = dc
	}

	radius := int(math.Max(img.size, 1)) // 点的半径

	canvas := img.img
	bounds := canvas.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	for i := 0; i < dotCount; i++ {
		col := img.GetColor()
		x := rand.Intn(width)
		y := rand.Intn(height)

		drawFilledCircle(canvas, x, y, radius, col)
	}

	return nil
}

// drawFilledCircle 画一个实心圆
func drawFilledCircle(img *image.RGBA, cx, cy, r int, c *color.NRGBA) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				x := cx + dx
				y := cy + dy
				if image.Pt(x, y).In(img.Bounds()) {
					img.Set(x, y, *c)
				}
			}
		}
	}
}

// 绘制随机线条
func (f *DicFunc) DrawImgRandomLines() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是非空画布")
	}

	lineCount := 20 // 默认线条数
	if lc, ok := f.Inputs.IntOk(2); ok && lc > 0 {
		lineCount = lc
	}

	thickness := max(int(img.size), 1)

	canvas := img.img
	bounds := canvas.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	for i := 0; i < lineCount; i++ {
		x1 := rand.Intn(width)
		y1 := rand.Intn(height)
		x2 := rand.Intn(width)
		y2 := rand.Intn(height)

		col := img.GetColor()

		drawThickLine(canvas, x1, y1, x2, y2, thickness, col)
	}

	return nil
}

// 画细线（Bresenham算法）
func drawLine(img *image.RGBA, x1, y1, x2, y2 int, col color.Color) {
	bounds := img.Bounds()
	dx := int(math.Abs(float64(x2 - x1)))
	dy := int(math.Abs(float64(y2 - y1)))
	sx := 1
	if x1 > x2 {
		sx = -1
	}
	sy := 1
	if y1 > y2 {
		sy = -1
	}
	err := dx - dy

	for {
		if x1 < bounds.Min.X || x1 >= bounds.Max.X || y1 < bounds.Min.Y || y1 >= bounds.Max.Y {
			break
		}
		img.Set(x1, y1, col)
		if x1 == x2 && y1 == y2 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
			if x1 < bounds.Min.X || x1 >= bounds.Max.X {
				break
			}
		}
		if e2 < dx {
			err += dx
			y1 += sy
			if y1 < bounds.Min.Y || y1 >= bounds.Max.Y {
				break
			}
		}
	}
}

// 画粗线条，线宽为thickness，粗线通过多条平行线模拟，步长0.5避免缝隙
func drawThickLine(img *image.RGBA, x1, y1, x2, y2, thickness int, col color.Color) {
	if thickness <= 1 {
		drawLine(img, x1, y1, x2, y2, col)
		return
	}

	dx := float64(x2 - x1)
	dy := float64(y2 - y1)
	length := math.Hypot(dx, dy)
	if length == 0 {
		bounds := img.Bounds()
		half := thickness / 2
		for tx := -half; tx <= half; tx++ {
			for ty := -half; ty <= half; ty++ {
				px, py := x1+tx, y1+ty
				if px >= bounds.Min.X && px < bounds.Max.X && py >= bounds.Min.Y && py < bounds.Max.Y {
					img.Set(px, py, col)
				}
			}
		}
		return
	}

	// 归一化垂直向量（垂直于线方向）
	px := -dy / length
	py := dx / length
	half := float64(thickness) / 2

	// 步长0.5，覆盖更密避免缝隙
	for t := -half; t <= half; t += 0.5 {
		offsetX := int(math.Round(px * t))
		offsetY := int(math.Round(py * t))
		drawLine(img, x1+offsetX, y1+offsetY, x2+offsetX, y2+offsetY, col)
	}
}

// 绘制灰度图
func (f *DicFunc) DrawImgGrayscale() error {
	imgObj, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || imgObj == nil {
		return errors.New("参数1必须是画布")
	}

	img := imgObj.img
	bounds := img.Bounds()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			r8 := float64(r >> 8)
			g8 := float64(g >> 8)
			b8 := float64(b >> 8)
			a8 := uint8(a >> 8)

			gray := uint8(0.299*r8 + 0.587*g8 + 0.114*b8)
			img.Set(x, y, color.NRGBA{gray, gray, gray, a8})
		}
	}

	return nil
}

// 高斯模糊
func (f *DicFunc) DrawImgGaussianBlur() error {
	imgObj, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || imgObj == nil {
		return errors.New("参数1必须是画布")
	}

	// 读取参数：左上角坐标 + 宽高
	x := f.Inputs.Int(2)
	y := f.Inputs.Int(3)
	width := f.Inputs.Int(4)
	height := f.Inputs.Int(5)

	if width <= 0 || height <= 0 {
		return errors.New("宽度和高度必须大于0")
	}

	x2 := x + width
	y2 := y + height

	img := imgObj.img
	bounds := img.Bounds()

	// 限制绘制区域不超出图片边界
	if x < bounds.Min.X {
		x = bounds.Min.X
	}
	if y < bounds.Min.Y {
		y = bounds.Min.Y
	}
	if x2 > bounds.Max.X {
		x2 = bounds.Max.X
	}
	if y2 > bounds.Max.Y {
		y2 = bounds.Max.Y
	}

	blurRect := image.Rect(x, y, x2, y2)

	// 截取区域
	cropped := imaging.Crop(img, blurRect)

	// 模糊强度来自 size 字段（或默认值）
	sigma := imgObj.size
	if sigma <= 0 {
		sigma = 3.0
	}

	// 高斯模糊处理
	blurred := imaging.Blur(cropped, sigma)

	// 贴回原图
	draw.Draw(img, blurRect, blurred, image.Point{}, draw.Over)

	return nil
}

// 绘制马赛克
func (f *DicFunc) DrawImgMosaic() error {
	imgObj, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || imgObj == nil {
		return errors.New("参数1必须是画布")
	}

	// 读取参数：左上角坐标 + 宽高
	x := f.Inputs.Int(2)
	y := f.Inputs.Int(3)
	width := f.Inputs.Int(4)
	height := f.Inputs.Int(5)

	if width <= 0 || height <= 0 {
		return errors.New("宽度和高度必须大于0")
	}

	x2 := x + width
	y2 := y + height

	img := imgObj.img
	bounds := img.Bounds()

	// 限制绘制区域不超出图片边界
	if x < bounds.Min.X {
		x = bounds.Min.X
	}
	if y < bounds.Min.Y {
		y = bounds.Min.Y
	}
	if x2 > bounds.Max.X {
		x2 = bounds.Max.X
	}
	if y2 > bounds.Max.Y {
		y2 = bounds.Max.Y
	}

	blockSize := int(imgObj.size)
	if blockSize <= 0 {
		blockSize = 8 // 默认块大小
	}

	// 按块大小遍历绘制区域，绘制马赛克
	for yy := y; yy < y2; yy += blockSize {
		for xx := x; xx < x2; xx += blockSize {
			xEnd := xx + blockSize
			yEnd := yy + blockSize
			if xEnd > x2 {
				xEnd = x2
			}
			if yEnd > y2 {
				yEnd = y2
			}

			var rSum, gSum, bSum, aSum, count uint32
			for py := yy; py < yEnd; py++ {
				for px := xx; px < xEnd; px++ {
					r, g, b, a := img.At(px, py).RGBA()
					rSum += r
					gSum += g
					bSum += b
					aSum += a
					count++
				}
			}

			if count == 0 {
				continue
			}

			rAvg := uint8((rSum / count) >> 8)
			gAvg := uint8((gSum / count) >> 8)
			bAvg := uint8((bSum / count) >> 8)
			aAvg := uint8((aSum / count) >> 8)

			avgColor := color.NRGBA{rAvg, gAvg, bAvg, aAvg}

			for py := yy; py < yEnd; py++ {
				for px := xx; px < xEnd; px++ {
					img.Set(px, py, avgColor)
				}
			}
		}
	}

	return nil
}

// 重构绘制马赛克
func (f *DicFunc) DrawImgAllMosaic() error {
	imgObj, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || imgObj == nil {
		return errors.New("参数1必须是画布")
	}

	blockSize := int(imgObj.size)
	if bs, ok := f.Inputs.IntOk(2); ok && bs > 0 {
		blockSize = bs
	}

	img := imgObj.img
	bounds := img.Bounds()

	// 遍历图像每个块
	for y := bounds.Min.Y; y < bounds.Max.Y; y += blockSize {
		for x := bounds.Min.X; x < bounds.Max.X; x += blockSize {
			// 计算块区域右下角坐标，防止越界
			xEnd := min(x+blockSize, bounds.Max.X)
			yEnd := min(y+blockSize, bounds.Max.Y)

			// 计算块内平均颜色
			var rSum, gSum, bSum, aSum, count uint32
			for yy := y; yy < yEnd; yy++ {
				for xx := x; xx < xEnd; xx++ {
					r, g, b, a := img.At(xx, yy).RGBA()
					rSum += r
					gSum += g
					bSum += b
					aSum += a
					count++
				}
			}
			if count == 0 {
				continue
			}
			// RGBA 返回值是16位颜色，缩小到8位
			rAvg := uint8((rSum / count) >> 8)
			gAvg := uint8((gSum / count) >> 8)
			bAvg := uint8((bSum / count) >> 8)
			aAvg := uint8((aSum / count) >> 8)

			avgColor := color.NRGBA{rAvg, gAvg, bAvg, aAvg}

			// 用平均颜色填充块
			for yy := y; yy < yEnd; yy++ {
				for xx := x; xx < xEnd; xx++ {
					img.Set(xx, yy, avgColor)
				}
			}
		}
	}

	return nil
}

// 整体画布图片圆角裁剪
func (f *DicFunc) DrawImgRoundCorners() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是非空画布")
	}

	// 背景颜色，默认为透明
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(3).(*color.NRGBA); ok {
		c = cc
	}

	// 圆角半径
	radius := 0
	if r, ok := f.Inputs.IntOk(2); ok && r > 0 {
		radius = r
	}
	if radius <= 0 {
		return nil
	}

	// 新建一张背景填充 c 的图
	bounds := img.img.Bounds()
	dst := image.NewNRGBA(bounds)
	// 用背景色填充整张图
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.SetNRGBA(x, y, *c)
		}
	}

	// 把原图画到新图，透明区域用圆角遮罩控制
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if inRoundedRect(x-bounds.Min.X, y-bounds.Min.Y, bounds.Dx(), bounds.Dy(), radius) {
				dst.Set(x, y, img.img.At(x, y))
			}
		}
	}

	// 转成RGBA
	rgba := image.NewRGBA(dst.Bounds())
	draw.Draw(rgba, rgba.Bounds(), dst, image.Point{}, draw.Src)

	img.img = rgba
	return nil
}

// 整体画布图片旋转并缩放
func (f *DicFunc) DrawImgRotate() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是非空画布")
	}

	rotateDeg := f.Inputs.Float64(2)
	if rotateDeg == 0 {
		return nil // 不旋转
	}

	c := img.GetColor()
	if cc, ok := f.Inputs.Get(3).(*color.NRGBA); ok {
		c = cc
	}

	// 旋转 + 背景透明
	rotated := imaging.Rotate(img.img, rotateDeg, c)

	// 转为 RGBA（你的数据结构需要）
	rgba := image.NewRGBA(rotated.Bounds())
	draw.Draw(rgba, rgba.Bounds(), rotated, image.Point{}, draw.Src)

	img.img = rgba
	return nil
}

// 粘贴图片
func (f *DicFunc) DrawImgPaste() error {
	// 参数1：目标画布（*NDrawImg）
	target, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || target == nil {
		return errors.New("参数1必须是目标画布 *NDrawImg")
	}

	// 参数2：图片来源（*NDrawImg、网络链接、本地文件路径、字符串二进制、image.Image）
	param2 := f.Inputs.Get(2)
	var img image.Image

	switch v := param2.(type) {
	case *NDrawImg:
		if v == nil {
			return errors.New("参数2画布不能为空")
		}
		img = v.img

	case string:
		// 网络链接
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			resp, err := httpClient.Get(v)
			if err != nil {
				return fmt.Errorf("下载网络图片失败: %v", err)
			}
			defer resp.Body.Close()
			imgDecoded, _, err := image.Decode(resp.Body)
			if err != nil {
				return fmt.Errorf("解码网络图片失败: %v", err)
			}
			img = imgDecoded
		} else {
			// 直接二进制解码
			imgDecoded, _, err := image.Decode(strings.NewReader(v))
			if err != nil {
				// 解析失败，尝试当成本地文件
				fileData, fileErr := utils.NewFileQueue(v).ReadFileByte()
				if fileErr != nil {
					return fmt.Errorf("读取本地文件失败: %v", fileErr)
				}

				img, _, decodeErr := image.Decode(bytes.NewReader(fileData))
				if decodeErr != nil {
					return fmt.Errorf("解码本地图片失败: %v", decodeErr)
				}

				imgDecoded = img
			}
			img = imgDecoded
		}

	case image.Image:
		img = v

	default:
		return fmt.Errorf("参数2格式错误，支持: NDrawImg、文件路径、网络链接、image.Image")
	}

	// 读取原始高宽
	bounds := img.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()

	// 读取目标高度/宽度（可选）
	p3, p3ok := f.Inputs.IntOk(3) // 高度
	p4, p4ok := f.Inputs.IntOk(4) // 宽度
	if !p3ok || p3 <= 0 {
		p3 = origH
	}
	if !p4ok || p4 <= 0 {
		p4 = origW
	}

	// 缩放到目标尺寸
	resized := imaging.Resize(img, p4, p3, imaging.Lanczos)

	// 旋转角度
	rotateDeg := 0.0
	if deg, ok := f.Inputs.Float64Ok(5); ok {
		rotateDeg = deg
	}
	if rotateDeg != 0 {
		resized = imaging.Rotate(resized, rotateDeg, color.NRGBA{0, 0, 0, 0})
	}

	// 插入圆角处理（第9参数为圆角半径）
	if radius, ok := f.Inputs.IntOk(9); ok && radius > 0 {
		resized = applyRoundedCorners(resized, radius)
	}

	// 粘贴位置
	x := int(f.Inputs.Float64(6))
	y := int(f.Inputs.Float64(7))

	// 透明度
	alpha := 1.0
	if a, ok := f.Inputs.Float64Ok(8); ok {
		alpha = a
	}

	// imaging.Overlay返回*image.NRGBA类型
	overlayed := imaging.Overlay(target.img, resized, image.Pt(x, y), alpha)

	// 转换为*image.RGBA类型
	rgba := image.NewRGBA(overlayed.Bounds())
	draw.Draw(rgba, rgba.Bounds(), overlayed, image.Point{}, draw.Src)

	// 替换目标画布图像
	target.img = rgba

	return nil
}

// applyRoundedCorners 给图片添加圆角遮罩（透明）
// radius 为圆角半径，单位为像素
func applyRoundedCorners(src image.Image, radius int) *image.NRGBA {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))

	// 拷贝原图像像素
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)

	// 创建圆角遮罩
	for y := range h {
		for x := range w {
			if !inRoundedRect(x, y, w, h, radius) {
				i := dst.PixOffset(x, y)
				dst.Pix[i+3] = 0 // alpha = 0，透明
			}
		}
	}
	return dst
}

// inRoundedRect 判断一个点是否在带圆角的矩形内
func inRoundedRect(x, y, w, h, r int) bool {
	// 四角判定（圆心 + 半径圆形判断）
	switch {
	case x < r && y < r:
		return distance(x, y, r, r) <= float64(r)
	case x >= w-r && y < r:
		return distance(x, y, w-r-1, r) <= float64(r)
	case x < r && y >= h-r:
		return distance(x, y, r, h-r-1) <= float64(r)
	case x >= w-r && y >= h-r:
		return distance(x, y, w-r-1, h-r-1) <= float64(r)
	default:
		return true
	}
}

func distance(x1, y1, x2, y2 int) float64 {
	return math.Hypot(float64(x1-x2), float64(y1-y2))
}

// 圆弧
func (f *DicFunc) DrawImgArc() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是非空画布")
	}

	cx := f.Inputs.Float64(2)
	cy := f.Inputs.Float64(3)
	radius := f.Inputs.Float64(4)
	startDeg := f.Inputs.Float64(5)
	endDeg := f.Inputs.Float64(6)

	// 设置颜色
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(7).(*color.NRGBA); ok {
		c = cc
	}

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(c)
	gc.SetLineWidth(img.size)

	// 转为弧度
	startRad := startDeg * math.Pi / 180
	endRad := endDeg * math.Pi / 180

	// 起点
	startX := cx + radius*math.Cos(startRad)
	startY := cy + radius*math.Sin(startRad)

	gc.BeginPath()
	gc.MoveTo(startX, startY)
	gc.ArcTo(cx, cy, radius, radius, startRad, endRad-startRad)
	gc.Stroke()

	return nil
}

// 画扇形 - 仅描边
func (f *DicFunc) DrawImgPie() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是非空画布")
	}

	cx := f.Inputs.Float64(2)
	cy := f.Inputs.Float64(3)

	// radius 默认 50
	radius := 50.0
	if r, ok := f.Inputs.Float64Ok(4); ok {
		radius = r
	}

	// 角度，默认整圆
	startDeg := 0.0
	endDeg := 360.0
	if f.Len >= 6 {
		startDeg = f.Inputs.Float64(5)
		endDeg = f.Inputs.Float64(6)
	}

	// 线条颜色
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(7).(*color.NRGBA); ok {
		c = cc
	}

	startRad := startDeg * math.Pi / 180
	endRad := endDeg * math.Pi / 180
	angle := endRad - startRad

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(c)
	gc.SetLineWidth(img.size)

	if math.Abs(angle) >= 2*math.Pi {
		// 整圆
		gc.ArcTo(cx, cy, radius, radius, 0, 2*math.Pi)
	} else {
		// 扇形路径
		gc.MoveTo(cx, cy)
		gc.LineTo(
			cx+radius*math.Cos(startRad),
			cy+radius*math.Sin(startRad),
		)
		gc.ArcTo(cx, cy, radius, radius, startRad, angle)
		gc.Close()
	}

	gc.Stroke()
	return nil
}

// 画扇形
func (f *DicFunc) DrawImgPieFill() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是画布")
	}

	// 圆心坐标
	cx := f.Inputs.Float64(2)
	cy := f.Inputs.Float64(3)

	// 半径（默认 50）
	radius := 50.0
	if val, ok := f.Inputs.Float64Ok(4); ok && val > 0 {
		radius = val
	}

	// 起始角度（默认 0）
	startDeg := 0.0
	if val, ok := f.Inputs.Float64Ok(5); ok {
		startDeg = val
	}

	// 结束角度（默认 360）
	endDeg := 360.0
	if val, ok := f.Inputs.Float64Ok(6); ok {
		endDeg = val
	}

	// 填充颜色（优先使用参数7，否则使用画笔）
	c := img.GetColor()
	if cc, ok := f.Inputs.Get(7).(*color.NRGBA); ok {
		c = cc
	}

	// 创建上下文
	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetFillColor(*c)

	// 转换角度为弧度
	startRad := startDeg * math.Pi / 180
	endRad := endDeg * math.Pi / 180

	// 扇形路径
	gc.MoveTo(cx, cy)
	gc.LineTo(
		cx+radius*math.Cos(startRad),
		cy+radius*math.Sin(startRad),
	)
	gc.ArcTo(cx, cy, radius, radius, startRad, endRad-startRad)
	gc.Close()
	gc.Fill()

	return nil
}

// 画线
func ellipsePoints(centerX, centerY, radiusX, radiusY float64, numPoints int) [][]float64 {
	points := make([][]float64, numPoints)
	for i := range numPoints {
		angle := float64(i) * 2.0 * math.Pi / float64(numPoints)
		x := centerX + radiusX*math.Cos(angle)
		y := centerY + radiusY*math.Sin(angle)
		points[i] = []float64{x, y}
	}
	return points
}

// 画多边形
func (f *DicFunc) DrawImgPolygon() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是画布")
	}

	numArgs := f.Len

	var c *color.NRGBA
	lastVal := f.Inputs.Get(numArgs)
	if cc, ok := lastVal.(*color.NRGBA); ok {
		c = cc
		numArgs--
	} else {
		c = img.GetColor()
	}

	points := [][]float64{}
	for i := 2; i <= numArgs; i++ {
		str := f.Inputs.String(i)
		coords := strings.Split(str, ",")
		if len(coords) != 2 {
			return fmt.Errorf("参数%d格式错误，期望 x,y 格式", i)
		}
		x, err1 := strconv.ParseFloat(strings.TrimSpace(coords[0]), 64)
		y, err2 := strconv.ParseFloat(strings.TrimSpace(coords[1]), 64)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("参数%d坐标转换错误", i)
		}
		points = append(points, []float64{x, y})
	}

	if len(points) < 3 {
		return errors.New("至少需要3个点")
	}

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetFillColor(*c)
	gc.MoveTo(points[0][0], points[0][1])
	for _, p := range points[1:] {
		gc.LineTo(p[0], p[1])
	}
	gc.Close()
	gc.Fill()

	return nil
}

// 画多边形描边
func (f *DicFunc) DrawImgPolygons() error {
	img, ok := f.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return errors.New("参数1必须是画布")
	}

	numArgs := f.Len

	// 最后一个参数可能是颜色，如果是 *color.NRGBA 就取出
	var c *color.NRGBA
	lastVal := f.Inputs.Get(numArgs)
	if cc, ok := lastVal.(*color.NRGBA); ok {
		c = cc
		numArgs--
	} else {
		c = img.GetColor()
	}

	// 参数2 到 numArgs 是 "x,y" 格式字符串
	points := [][]float64{}
	for i := 2; i <= numArgs; i++ {
		str := f.Inputs.String(i)
		coords := strings.Split(str, ",")
		if len(coords) != 2 {
			return fmt.Errorf("参数%d格式错误，期望 x,y 格式", i)
		}
		x, err1 := strconv.ParseFloat(strings.TrimSpace(coords[0]), 64)
		y, err2 := strconv.ParseFloat(strings.TrimSpace(coords[1]), 64)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("参数%d坐标转换错误", i)
		}
		points = append(points, []float64{x, y})
	}

	if len(points) < 2 {
		return errors.New("至少需要2个点")
	}

	// 绘图
	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(*c)
	gc.SetLineWidth(img.size)
	gc.MoveTo(points[0][0], points[0][1])
	for _, p := range points[1:] {
		gc.LineTo(p[0], p[1])
	}
	gc.Close()
	gc.Stroke()

	return nil
}

// 获取图片
func drawImgGet(d *dto.DicInputs) (any, error) {
	img, ok := d.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return "", errors.New("参数1必须是画布")
	}

	if img == nil {
		return "", errors.New("图片为空")
	}

	var buf bytes.Buffer
	format := strings.ToLower(d.Inputs.String(2))

	// 释放图片资源
	defer img.Close()

	switch format {
	case "png":
		if err := imaging.Encode(&buf, img.img, imaging.PNG); err != nil {
			return "", fmt.Errorf("不支持的图片格式: %s", err)
		}
	case "jpg", "jpeg":
		if err := imaging.Encode(&buf, img.img, imaging.JPEG); err != nil {
			return "", fmt.Errorf("不支持的图片格式: %s", err)
		}
	default:
		// 默认使用PNG格式
		if err := imaging.Encode(&buf, img.img, imaging.PNG); err != nil {
			return "", fmt.Errorf("不支持的图片格式: %s", err)
		}
	}

	encoded := buf.String()
	return encoded, nil
}

// 关闭图片
func (i *NDrawImg) Close() {
	if i.img != nil {
		i.img = nil
	}
}

// ========== 以下为 migration.go 迁移来的 free function 版本 ==========

func getDrawImg(d *dto.DicInputs, idx int) (*NDrawImg, error) {
	img, ok := d.Inputs.Get(idx).(*NDrawImg)
	if !ok {
		return nil, errors.New("参数必须是画布")
	}
	if img == nil {
		return nil, errors.New("图片不能为空")
	}
	return img, nil
}

func drawImgLoadFont(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	ttfDir := "private/ttf/"
	ttfFile := d.Inputs.String(2)
	imgTtf, err := utils.NewFileQueue(ttfDir + ttfFile).ReadFileByte()
	if err != nil {
		return nil, fmt.Errorf("加载字体失败：%s", err)
	}
	fontParsed, err := opentype.Parse(imgTtf)
	if err != nil {
		return nil, fmt.Errorf("解析字体失败：%s", err)
	}
	img.font = fontParsed
	return nil, nil
}

func drawImgSetSize(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	size := d.Inputs.Float64(2)
	if size <= 0 {
		return nil, errors.New("线条宽度必须大于0")
	}
	img.size = size
	return nil, nil
}

func drawImgText(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	x := int(d.Inputs.Float64(2))
	y := int(d.Inputs.Float64(3))
	text := d.Inputs.String(4)
	rotateDeg := 0.0
	if deg, ok := d.Inputs.Float64Ok(5); ok {
		rotateDeg = deg
	}
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(6).(*color.NRGBA); ok {
		c = cc
	}
	strokeColor, hasStroke := d.Inputs.Get(7).(*color.NRGBA)
	strokeWidth := 2.0
	if hasStroke {
		if sw, ok := d.Inputs.Float64Ok(8); ok {
			strokeWidth = sw
		}
	}
	face, err := opentype.NewFace(img.font, &opentype.FaceOptions{
		Size:    img.size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}
	defer face.Close()

	drawTextFunc := func(dst draw.Image, col color.Color, ox, oy int) {
		d := &font.Drawer{
			Dst:  dst,
			Src:  image.NewUniform(col),
			Face: face,
			Dot:  fixed.P(x+ox, y+oy),
		}
		d.DrawString(text)
	}

	if rotateDeg == 0 {
		if hasStroke {
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					if dx != 0 || dy != 0 {
						ox := int(float64(dx) * strokeWidth)
						oy := int(float64(dy) * strokeWidth)
						drawTextFunc(img.img, strokeColor, ox, oy)
					}
				}
			}
		}
		drawTextFunc(img.img, c, 0, 0)
	} else {
		textWidth := font.MeasureString(face, text).Ceil()
		metrics := face.Metrics()
		textHeight := (metrics.Ascent + metrics.Descent).Ceil()
		textImg := image.NewNRGBA(image.Rect(0, 0, textWidth+int(strokeWidth*2), textHeight+int(strokeWidth*2)))
		if hasStroke {
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					if dx != 0 || dy != 0 {
						ox := int(float64(dx) * strokeWidth)
						oy := int(float64(dy) * strokeWidth)
						fd := &font.Drawer{
							Dst:  textImg,
							Src:  image.NewUniform(strokeColor),
							Face: face,
							Dot:  fixed.P(ox, metrics.Ascent.Ceil()+oy),
						}
						fd.DrawString(text)
					}
				}
			}
		}
		fd := &font.Drawer{
			Dst:  textImg,
			Src:  image.NewUniform(c),
			Face: face,
			Dot:  fixed.P(0, metrics.Ascent.Ceil()),
		}
		fd.DrawString(text)
		rotated := imaging.Rotate(textImg, rotateDeg, color.NRGBA{0, 0, 0, 0})
		offset := image.Pt(x, y)
		draw.Draw(img.img, rotated.Bounds().Add(offset), rotated, image.Point{}, draw.Over)
	}
	return nil, nil
}

func drawImgPoint(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	x := int(d.Inputs.Float64(2))
	y := int(d.Inputs.Float64(3))
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(4).(*color.NRGBA); ok {
		c = cc
	}
	if x >= 0 && x < img.img.Bounds().Dx() && y >= 0 && y < img.img.Bounds().Dy() {
		img.img.Set(x, y, c)
	}
	return nil, nil
}

func drawImgLine(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	x1 := d.Inputs.Float64(2)
	y1 := d.Inputs.Float64(3)
	x2 := d.Inputs.Float64(4)
	y2 := d.Inputs.Float64(5)
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(6).(*color.NRGBA); ok {
		c = cc
	}
	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(c)
	gc.SetLineWidth(img.size)
	gc.MoveTo(x1, y1)
	gc.LineTo(x2, y2)
	gc.Stroke()
	return nil, nil
}

func drawImgBrushLine(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	rangeRadius := int(img.size)
	if val, ok := d.Inputs.IntOk(6); ok && val > 0 {
		rangeRadius = val
	}
	if rangeRadius <= 0 {
		rangeRadius = 3
	}
	density := 50
	if val, ok := d.Inputs.IntOk(7); ok {
		if val < 1 {
			density = 1
		} else if val > 100 {
			density = 100
		} else {
			density = val
		}
	}
	pointRadius := 1
	if val, ok := d.Inputs.IntOk(9); ok && val > 0 {
		pointRadius = val
	}
	x1 := d.Inputs.Float64(2)
	y1 := d.Inputs.Float64(3)
	x2 := d.Inputs.Float64(4)
	y2 := d.Inputs.Float64(5)
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(8).(*color.NRGBA); ok {
		c = cc
	}
	dx := x2 - x1
	dy := y2 - y1
	length := math.Hypot(dx, dy)
	steps := max(int(length), 1)
	prob := float64(density) / 100.0

	drawPoint := func(imgRGBA *image.RGBA, px, py, radius int, cl color.NRGBA) {
		for ox := -radius; ox <= radius; ox++ {
			for oy := -radius; oy <= radius; oy++ {
				if ox*ox+oy*oy <= radius*radius {
					nx := px + ox
					ny := py + oy
					if image.Pt(nx, ny).In(imgRGBA.Bounds()) {
						imgRGBA.Set(nx, ny, cl)
					}
				}
			}
		}
	}

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		cx := int(x1 + t*dx)
		cy := int(y1 + t*dy)
		if density == 100 {
			for ox := -rangeRadius; ox <= rangeRadius; ox++ {
				for oy := -rangeRadius; oy <= rangeRadius; oy++ {
					if ox*ox+oy*oy <= rangeRadius*rangeRadius {
						drawPoint(img.img, cx+ox, cy+oy, pointRadius, *c)
					}
				}
			}
		} else {
			if rand.Float64() < prob {
				pointsCount := density * 20 / 100
				if pointsCount < 1 {
					pointsCount = 1
				}
				for j := 0; j < pointsCount; j++ {
					offsetX := rand.Intn(2*rangeRadius+1) - rangeRadius
					offsetY := rand.Intn(2*rangeRadius+1) - rangeRadius
					drawPoint(img.img, cx+offsetX, cy+offsetY, pointRadius, *c)
				}
			}
		}
	}
	return nil, nil
}

func drawImgWaveLine(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	x1 := d.Inputs.Float64(2)
	y1 := d.Inputs.Float64(3)
	x2 := d.Inputs.Float64(4)
	y2 := d.Inputs.Float64(5)
	c := img.GetColor()
	waveAmplitude := 5.0
	if val, ok := d.Inputs.Float64Ok(6); ok {
		waveAmplitude = val
	}
	waveLength := 20.0
	if val, ok := d.Inputs.Float64Ok(7); ok {
		waveLength = val
	}
	step := 2.0
	if val, ok := d.Inputs.Float64Ok(8); ok && val > 0 {
		step = val
	}
	dx := x2 - x1
	dy := y2 - y1
	length := math.Hypot(dx, dy)
	angle := math.Atan2(dy, dx)

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(*c)
	gc.SetLineWidth(img.size)

	for t := 0.0; t <= length; t += step {
		baseX := x1 + math.Cos(angle)*t
		baseY := y1 + math.Sin(angle)*t
		offset := waveAmplitude * math.Sin(2*math.Pi*t/waveLength)
		normalAngle := angle + math.Pi/2
		waveX := baseX + math.Cos(normalAngle)*offset
		waveY := baseY + math.Sin(normalAngle)*offset
		if t == 0 {
			gc.MoveTo(waveX, waveY)
		} else {
			gc.LineTo(waveX, waveY)
		}
	}
	gc.Stroke()
	return nil, nil
}

func drawImgFloodFill(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	x := int(d.Inputs.Float64(2))
	y := int(d.Inputs.Float64(3))
	fillColor := img.GetColor()
	if c, ok := d.Inputs.Get(4).(*color.NRGBA); ok {
		fillColor = c
	}
	bounds := img.img.Bounds()
	if !image.Pt(x, y).In(bounds) {
		return nil, errors.New("起始点不在图像范围内")
	}
	startColor := img.img.At(x, y)
	if colorsEqualRGBA(startColor, fillColor) {
		return nil, nil
	}
	visited := make(map[image.Point]bool)
	queue := []image.Point{{x, y}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if !p.In(bounds) || visited[p] {
			continue
		}
		if !colorsEqualRGBA(img.img.At(p.X, p.Y), startColor) {
			continue
		}
		img.img.Set(p.X, p.Y, fillColor)
		visited[p] = true
		queue = append(queue,
			image.Pt(p.X+1, p.Y),
			image.Pt(p.X-1, p.Y),
			image.Pt(p.X, p.Y+1),
			image.Pt(p.X, p.Y-1),
		)
	}
	return nil, nil
}

func colorsEqualRGBA(c1, c2 color.Color) bool {
	r1, g1, b1, a1 := c1.RGBA()
	r2, g2, b2, a2 := c2.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

func drawImgRectangleFill(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	x := d.Inputs.Float64(2)
	y := d.Inputs.Float64(3)
	width := d.Inputs.Float64(4)
	height := d.Inputs.Float64(5)
	radii := [4]float64{}
	if r, ok := d.Inputs.Float64Ok(6); ok {
		radii = [4]float64{r, r, r, r}
	} else if s, ok := d.Inputs.StringOk(6); ok && s != "" {
		parts := strings.Split(s, ",")
		for i := 0; i < len(parts) && i < 4; i++ {
			if val, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64); err == nil {
				radii[i] = val
			}
		}
	}
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(7).(*color.NRGBA); ok {
		c = cc
	}
	for i := range radii {
		if radii[i]*2 > width {
			radii[i] = width / 2
		}
		if radii[i]*2 > height {
			radii[i] = height / 2
		}
	}
	r0, r1, r2, r3 := radii[0], radii[1], radii[2], radii[3]
	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetFillColor(c)
	gc.BeginPath()
	x0, y0 := x, y
	x1, y1 := x+width, y+height
	gc.MoveTo(x0+r0, y0)
	gc.LineTo(x1-r1, y0)
	if r1 > 0 {
		gc.ArcTo(x1-r1, y0+r1, r1, r1, -math.Pi/2, math.Pi/2)
	}
	gc.LineTo(x1, y1-r2)
	if r2 > 0 {
		gc.ArcTo(x1-r2, y1-r2, r2, r2, 0, math.Pi/2)
	}
	gc.LineTo(x0+r3, y1)
	if r3 > 0 {
		gc.ArcTo(x0+r3, y1-r3, r3, r3, math.Pi/2, math.Pi/2)
	}
	gc.LineTo(x0, y0+r0)
	if r0 > 0 {
		gc.ArcTo(x0+r0, y0+r0, r0, r0, math.Pi, math.Pi/2)
	}
	gc.Close()
	gc.Fill()
	return nil, nil
}

func drawImgRectangleStroke(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	x := d.Inputs.Float64(2)
	y := d.Inputs.Float64(3)
	width := d.Inputs.Float64(4)
	height := d.Inputs.Float64(5)
	radii := [4]float64{}
	if r, ok := d.Inputs.Float64Ok(6); ok {
		radii = [4]float64{r, r, r, r}
	} else if s, ok := d.Inputs.StringOk(6); ok && s != "" {
		parts := strings.Split(s, ",")
		for i := 0; i < len(parts) && i < 4; i++ {
			if val, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64); err == nil {
				radii[i] = val
			}
		}
	}
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(7).(*color.NRGBA); ok {
		c = cc
	}
	for i := range radii {
		if radii[i]*2 > width {
			radii[i] = width / 2
		}
		if radii[i]*2 > height {
			radii[i] = height / 2
		}
	}
	r0, r1, r2, r3 := radii[0], radii[1], radii[2], radii[3]
	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(c)
	gc.SetLineWidth(img.size)
	gc.BeginPath()
	x0, y0 := x, y
	x1, y1 := x+width, y+height
	gc.MoveTo(x0+r0, y0)
	gc.LineTo(x1-r1, y0)
	if r1 > 0 {
		gc.ArcTo(x1-r1, y0+r1, r1, r1, -math.Pi/2, math.Pi/2)
	}
	gc.LineTo(x1, y1-r2)
	if r2 > 0 {
		gc.ArcTo(x1-r2, y1-r2, r2, r2, 0, math.Pi/2)
	}
	gc.LineTo(x0+r3, y1)
	if r3 > 0 {
		gc.ArcTo(x0+r3, y1-r3, r3, r3, math.Pi/2, math.Pi/2)
	}
	gc.LineTo(x0, y0+r0)
	if r0 > 0 {
		gc.ArcTo(x0+r0, y0+r0, r0, r0, math.Pi, math.Pi/2)
	}
	gc.Close()
	gc.Stroke()
	return nil, nil
}

func drawImgPaste(d *dto.DicInputs) (any, error) {
	target, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	param2 := d.Inputs.Get(2)
	var srcImg image.Image
	switch v := param2.(type) {
	case *NDrawImg:
		if v == nil {
			return nil, errors.New("参数2画布不能为空")
		}
		srcImg = v.img
	case string:
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			resp, err := httpClient.Get(v)
			if err != nil {
				return nil, fmt.Errorf("下载网络图片失败: %v", err)
			}
			defer resp.Body.Close()
			imgDecoded, _, err := image.Decode(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("解码网络图片失败: %v", err)
			}
			srcImg = imgDecoded
		} else {
			imgDecoded, _, err := image.Decode(strings.NewReader(v))
			if err != nil {
				fileData, fileErr := utils.NewFileQueue(v).ReadFileByte()
				if fileErr != nil {
					return nil, fmt.Errorf("读取本地文件失败: %v", fileErr)
				}
				var decodeErr error
				imgDecoded, _, decodeErr = image.Decode(bytes.NewReader(fileData))
				if decodeErr != nil {
					return nil, fmt.Errorf("解码本地图片失败: %v", decodeErr)
				}
			}
			srcImg = imgDecoded
		}
	case image.Image:
		srcImg = v
	default:
		return nil, fmt.Errorf("参数2格式错误")
	}
	bounds := srcImg.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()
	p3, p3ok := d.Inputs.IntOk(3)
	p4, p4ok := d.Inputs.IntOk(4)
	if !p3ok || p3 <= 0 {
		p3 = origH
	}
	if !p4ok || p4 <= 0 {
		p4 = origW
	}
	resized := imaging.Resize(srcImg, p4, p3, imaging.Lanczos)
	rotateDeg := 0.0
	if deg, ok := d.Inputs.Float64Ok(5); ok {
		rotateDeg = deg
	}
	if rotateDeg != 0 {
		resized = imaging.Rotate(resized, rotateDeg, color.NRGBA{0, 0, 0, 0})
	}
	if radius, ok := d.Inputs.IntOk(9); ok && radius > 0 {
		resized = applyRoundedCorners(resized, radius)
	}
	px := int(d.Inputs.Float64(6))
	py := int(d.Inputs.Float64(7))
	alpha := 1.0
	if a, ok := d.Inputs.Float64Ok(8); ok {
		alpha = a
	}
	overlayed := imaging.Overlay(target.img, resized, image.Pt(px, py), alpha)
	rgba := image.NewRGBA(overlayed.Bounds())
	draw.Draw(rgba, rgba.Bounds(), overlayed, image.Point{}, draw.Src)
	target.img = rgba
	return nil, nil
}

func drawImgRotate(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	rotateDeg := d.Inputs.Float64(2)
	if rotateDeg == 0 {
		return nil, nil
	}
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(3).(*color.NRGBA); ok {
		c = cc
	}
	rotated := imaging.Rotate(img.img, rotateDeg, c)
	rgba := image.NewRGBA(rotated.Bounds())
	draw.Draw(rgba, rgba.Bounds(), rotated, image.Point{}, draw.Src)
	img.img = rgba
	return nil, nil
}

func drawImgRoundCorners(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(3).(*color.NRGBA); ok {
		c = cc
	}
	radius := 0
	if r, ok := d.Inputs.IntOk(2); ok && r > 0 {
		radius = r
	}
	if radius <= 0 {
		return nil, nil
	}
	bounds := img.img.Bounds()
	dst := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.SetNRGBA(x, y, *c)
		}
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if inRoundedRect(x-bounds.Min.X, y-bounds.Min.Y, bounds.Dx(), bounds.Dy(), radius) {
				dst.Set(x, y, img.img.At(x, y))
			}
		}
	}
	rgba := image.NewRGBA(dst.Bounds())
	draw.Draw(rgba, rgba.Bounds(), dst, image.Point{}, draw.Src)
	img.img = rgba
	return nil, nil
}

func drawImgRandomDots(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	dotCount := 20
	if dc, ok := d.Inputs.IntOk(2); ok && dc > 0 {
		dotCount = dc
	}
	radius := int(math.Max(img.size, 1))
	canvas := img.img
	bounds := canvas.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	for i := 0; i < dotCount; i++ {
		col := img.GetColor()
		x := rand.Intn(width)
		y := rand.Intn(height)
		drawFilledCircle(canvas, x, y, radius, col)
	}
	return nil, nil
}

func drawImgRandomLines(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	lineCount := 20
	if lc, ok := d.Inputs.IntOk(2); ok && lc > 0 {
		lineCount = lc
	}
	thickness := max(int(img.size), 1)
	canvas := img.img
	bounds := canvas.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	for i := 0; i < lineCount; i++ {
		x1 := rand.Intn(width)
		y1 := rand.Intn(height)
		x2 := rand.Intn(width)
		y2 := rand.Intn(height)
		col := img.GetColor()
		drawThickLine(canvas, x1, y1, x2, y2, thickness, col)
	}
	return nil, nil
}

func drawImgGrayscale(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	canvas := img.img
	bounds := canvas.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := canvas.At(x, y).RGBA()
			r8 := float64(r >> 8)
			g8 := float64(g >> 8)
			b8 := float64(b >> 8)
			a8 := uint8(a >> 8)
			gray := uint8(0.299*r8 + 0.587*g8 + 0.114*b8)
			canvas.Set(x, y, color.NRGBA{gray, gray, gray, a8})
		}
	}
	return nil, nil
}

func drawImgGaussianBlur(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	x := d.Inputs.Int(2)
	y := d.Inputs.Int(3)
	width := d.Inputs.Int(4)
	height := d.Inputs.Int(5)
	if width <= 0 || height <= 0 {
		return nil, errors.New("宽度和高度必须大于0")
	}
	x2 := x + width
	y2 := y + height
	canvas := img.img
	bounds := canvas.Bounds()
	if x < bounds.Min.X {
		x = bounds.Min.X
	}
	if y < bounds.Min.Y {
		y = bounds.Min.Y
	}
	if x2 > bounds.Max.X {
		x2 = bounds.Max.X
	}
	if y2 > bounds.Max.Y {
		y2 = bounds.Max.Y
	}
	blurRect := image.Rect(x, y, x2, y2)
	cropped := imaging.Crop(canvas, blurRect)
	sigma := img.size
	if sigma <= 0 {
		sigma = 3.0
	}
	blurred := imaging.Blur(cropped, sigma)
	draw.Draw(canvas, blurRect, blurred, image.Point{}, draw.Over)
	return nil, nil
}

func drawImgMosaic(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	x := d.Inputs.Int(2)
	y := d.Inputs.Int(3)
	width := d.Inputs.Int(4)
	height := d.Inputs.Int(5)
	if width <= 0 || height <= 0 {
		return nil, errors.New("宽度和高度必须大于0")
	}
	x2 := x + width
	y2 := y + height
	canvas := img.img
	bounds := canvas.Bounds()
	if x < bounds.Min.X {
		x = bounds.Min.X
	}
	if y < bounds.Min.Y {
		y = bounds.Min.Y
	}
	if x2 > bounds.Max.X {
		x2 = bounds.Max.X
	}
	if y2 > bounds.Max.Y {
		y2 = bounds.Max.Y
	}
	blockSize := int(img.size)
	if blockSize <= 0 {
		blockSize = 8
	}
	for yy := y; yy < y2; yy += blockSize {
		for xx := x; xx < x2; xx += blockSize {
			xEnd := xx + blockSize
			yEnd := yy + blockSize
			if xEnd > x2 {
				xEnd = x2
			}
			if yEnd > y2 {
				yEnd = y2
			}
			var rSum, gSum, bSum, aSum, count uint32
			for py := yy; py < yEnd; py++ {
				for px := xx; px < xEnd; px++ {
					r, g, b, a := canvas.At(px, py).RGBA()
					rSum += r
					gSum += g
					bSum += b
					aSum += a
					count++
				}
			}
			if count == 0 {
				continue
			}
			rAvg := uint8((rSum / count) >> 8)
			gAvg := uint8((gSum / count) >> 8)
			bAvg := uint8((bSum / count) >> 8)
			aAvg := uint8((aSum / count) >> 8)
			avgColor := color.NRGBA{rAvg, gAvg, bAvg, aAvg}
			for py := yy; py < yEnd; py++ {
				for px := xx; px < xEnd; px++ {
					canvas.Set(px, py, avgColor)
				}
			}
		}
	}
	return nil, nil
}

func drawImgAllMosaic(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	blockSize := int(img.size)
	if bs, ok := d.Inputs.IntOk(2); ok && bs > 0 {
		blockSize = bs
	}
	canvas := img.img
	bounds := canvas.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y += blockSize {
		for x := bounds.Min.X; x < bounds.Max.X; x += blockSize {
			xEnd := min(x+blockSize, bounds.Max.X)
			yEnd := min(y+blockSize, bounds.Max.Y)
			var rSum, gSum, bSum, aSum, count uint32
			for yy := y; yy < yEnd; yy++ {
				for xx := x; xx < xEnd; xx++ {
					r, g, b, a := canvas.At(xx, yy).RGBA()
					rSum += r
					gSum += g
					bSum += b
					aSum += a
					count++
				}
			}
			if count == 0 {
				continue
			}
			rAvg := uint8((rSum / count) >> 8)
			gAvg := uint8((gSum / count) >> 8)
			bAvg := uint8((bSum / count) >> 8)
			aAvg := uint8((aSum / count) >> 8)
			avgColor := color.NRGBA{rAvg, gAvg, bAvg, aAvg}
			for yy := y; yy < yEnd; yy++ {
				for xx := x; xx < xEnd; xx++ {
					canvas.Set(xx, yy, avgColor)
				}
			}
		}
	}
	return nil, nil
}

func drawImgArc(d *dto.DicInputs) (any, error) {
	img, err := getDrawImg(d, 1)
	if err != nil {
		return nil, err
	}
	cx := d.Inputs.Float64(2)
	cy := d.Inputs.Float64(3)
	radius := d.Inputs.Float64(4)
	startDeg := d.Inputs.Float64(5)
	endDeg := d.Inputs.Float64(6)
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(7).(*color.NRGBA); ok {
		c = cc
	}
	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(c)
	gc.SetLineWidth(img.size)
	startRad := startDeg * math.Pi / 180
	endRad := endDeg * math.Pi / 180
	startX := cx + radius*math.Cos(startRad)
	startY := cy + radius*math.Sin(startRad)
	gc.BeginPath()
	gc.MoveTo(startX, startY)
	gc.ArcTo(cx, cy, radius, radius, startRad, endRad-startRad)
	gc.Stroke()
	return nil, nil
}

// ========== 绘制额外图形 (free function 版本) ==========

func drawImgEllipseFill(d *dto.DicInputs) (any, error) {
	img, ok := d.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return nil, errors.New("参数1必须是画布")
	}
	if img == nil {
		return nil, errors.New("图片不能为空")
	}

	x := d.Inputs.Float64(2)
	y := d.Inputs.Float64(3)
	width := d.Inputs.Float64(4)
	height := d.Inputs.Float64(5)
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(6).(*color.NRGBA); ok {
		c = cc
	}

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetFillColor(c)

	centerX := x + width/2
	centerY := y + height/2
	radiusX := width / 2
	radiusY := height / 2

	points := ellipsePoints(centerX, centerY, radiusX, radiusY, 40)
	if len(points) > 0 {
		gc.MoveTo(points[0][0], points[0][1])
		for _, p := range points[1:] {
			gc.LineTo(p[0], p[1])
		}
		gc.Close()
		gc.Fill()
	}
	return nil, nil
}

func drawImgEllipse(d *dto.DicInputs) (any, error) {
	img, ok := d.Inputs.Get(1).(*NDrawImg)
	if !ok {
		return nil, errors.New("参数1必须是画布")
	}
	if img == nil {
		return nil, errors.New("图片不能为空")
	}

	x := d.Inputs.Float64(2)
	y := d.Inputs.Float64(3)
	width := d.Inputs.Float64(4)
	height := d.Inputs.Float64(5)
	c := img.GetColor()
	if cc, ok := d.Inputs.Get(6).(*color.NRGBA); ok {
		c = cc
	}

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(c)
	gc.SetLineWidth(img.size)

	centerX := x + width/2
	centerY := y + height/2
	radiusX := width / 2
	radiusY := height / 2

	points := ellipsePoints(centerX, centerY, radiusX, radiusY, 40)
	if len(points) > 0 {
		gc.MoveTo(points[0][0], points[0][1])
		for _, p := range points[1:] {
			gc.LineTo(p[0], p[1])
		}
		gc.Close()
		gc.Stroke()
	}
	return nil, nil
}

func drawImgPieFill(d *dto.DicInputs) (any, error) {
	img, ok := d.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return nil, errors.New("参数1必须是画布")
	}

	cx := d.Inputs.Float64(2)
	cy := d.Inputs.Float64(3)

	radius := 50.0
	if val, ok := d.Inputs.Float64Ok(4); ok && val > 0 {
		radius = val
	}

	startDeg := 0.0
	if val, ok := d.Inputs.Float64Ok(5); ok {
		startDeg = val
	}

	endDeg := 360.0
	if val, ok := d.Inputs.Float64Ok(6); ok {
		endDeg = val
	}

	c := img.GetColor()
	if cc, ok := d.Inputs.Get(7).(*color.NRGBA); ok {
		c = cc
	}

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetFillColor(*c)

	startRad := startDeg * math.Pi / 180
	endRad := endDeg * math.Pi / 180

	gc.MoveTo(cx, cy)
	gc.LineTo(
		cx+radius*math.Cos(startRad),
		cy+radius*math.Sin(startRad),
	)
	gc.ArcTo(cx, cy, radius, radius, startRad, endRad-startRad)
	gc.Close()
	gc.Fill()

	return nil, nil
}

func drawImgPie(d *dto.DicInputs) (any, error) {
	img, ok := d.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return nil, errors.New("参数1必须是非空画布")
	}

	cx := d.Inputs.Float64(2)
	cy := d.Inputs.Float64(3)

	radius := 50.0
	if r, ok := d.Inputs.Float64Ok(4); ok {
		radius = r
	}

	startDeg := 0.0
	endDeg := 360.0
	if d.Inputs.Len() >= 6 {
		startDeg = d.Inputs.Float64(5)
		endDeg = d.Inputs.Float64(6)
	}

	c := img.GetColor()
	if cc, ok := d.Inputs.Get(7).(*color.NRGBA); ok {
		c = cc
	}

	startRad := startDeg * math.Pi / 180
	endRad := endDeg * math.Pi / 180
	angle := endRad - startRad

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(c)
	gc.SetLineWidth(img.size)

	if math.Abs(angle) >= 2*math.Pi {
		gc.ArcTo(cx, cy, radius, radius, 0, 2*math.Pi)
	} else {
		gc.MoveTo(cx, cy)
		gc.LineTo(
			cx+radius*math.Cos(startRad),
			cy+radius*math.Sin(startRad),
		)
		gc.ArcTo(cx, cy, radius, radius, startRad, angle)
		gc.Close()
	}

	gc.Stroke()
	return nil, nil
}

func drawImgPolygon(d *dto.DicInputs) (any, error) {
	img, ok := d.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return nil, errors.New("参数1必须是画布")
	}

	numArgs := d.Inputs.Len()

	var c *color.NRGBA
	lastVal := d.Inputs.Get(numArgs)
	if cc, ok := lastVal.(*color.NRGBA); ok {
		c = cc
		numArgs--
	} else {
		c = img.GetColor()
	}

	points := [][]float64{}
	for i := 2; i <= numArgs; i++ {
		str := d.Inputs.String(i)
		coords := strings.Split(str, ",")
		if len(coords) != 2 {
			return nil, fmt.Errorf("参数%d格式错误，期望 x,y 格式", i)
		}
		x, err1 := strconv.ParseFloat(strings.TrimSpace(coords[0]), 64)
		y, err2 := strconv.ParseFloat(strings.TrimSpace(coords[1]), 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("参数%d坐标转换错误", i)
		}
		points = append(points, []float64{x, y})
	}

	if len(points) < 3 {
		return nil, errors.New("至少需要3个点")
	}

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetFillColor(*c)
	gc.MoveTo(points[0][0], points[0][1])
	for _, p := range points[1:] {
		gc.LineTo(p[0], p[1])
	}
	gc.Close()
	gc.Fill()

	return nil, nil
}

func drawImgPolygons(d *dto.DicInputs) (any, error) {
	img, ok := d.Inputs.Get(1).(*NDrawImg)
	if !ok || img == nil {
		return nil, errors.New("参数1必须是画布")
	}

	numArgs := d.Inputs.Len()

	var c *color.NRGBA
	lastVal := d.Inputs.Get(numArgs)
	if cc, ok := lastVal.(*color.NRGBA); ok {
		c = cc
		numArgs--
	} else {
		c = img.GetColor()
	}

	points := [][]float64{}
	for i := 2; i <= numArgs; i++ {
		str := d.Inputs.String(i)
		coords := strings.Split(str, ",")
		if len(coords) != 2 {
			return nil, fmt.Errorf("参数%d格式错误，期望 x,y 格式", i)
		}
		x, err1 := strconv.ParseFloat(strings.TrimSpace(coords[0]), 64)
		y, err2 := strconv.ParseFloat(strings.TrimSpace(coords[1]), 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("参数%d坐标转换错误", i)
		}
		points = append(points, []float64{x, y})
	}

	if len(points) < 2 {
		return nil, errors.New("至少需要2个点")
	}

	gc := draw2dimg.NewGraphicContext(img.img)
	gc.SetStrokeColor(*c)
	gc.SetLineWidth(img.size)
	gc.MoveTo(points[0][0], points[0][1])
	for _, p := range points[1:] {
		gc.LineTo(p[0], p[1])
	}
	gc.Close()
	gc.Stroke()

	return nil, nil
}
