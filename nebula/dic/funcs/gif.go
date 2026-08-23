package funcs

import (
	"bytes"
	"encoding/base64"
	stdjson "encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"strings"

	"github.com/cjxpj/nebula/dto"
	"github.com/disintegration/imaging"
)

type frameData struct {
	Img   string `json:"img"`
	Delay int    `json:"delay"` // 单位是100分之一秒
}

// splitGif 将 GIF 拆分为逐帧合成后的完整画面，返回 Base64 编码的 PNG 及每帧延迟。
func splitGif(r io.Reader) ([]frameData, error) {
	g, err := gif.DecodeAll(r)
	if err != nil {
		return nil, err
	}

	w, h := g.Config.Width, g.Config.Height
	if (w <= 0 || h <= 0) && len(g.Image) > 0 {
		b := g.Image[0].Bounds()
		w, h = b.Dx(), b.Dy()
	}

	// 逻辑屏幕背景色，用于 DisposalBackground 处理。
	var bg color.Color = color.Transparent
	if p, ok := g.Config.ColorModel.(color.Palette); ok {
		if idx := int(g.BackgroundIndex); idx >= 0 && idx < len(p) {
			bg = p[idx]
		}
	}

	bounds := image.Rect(0, 0, w, h)
	canvas := image.NewRGBA(bounds)

	frames := make([]frameData, 0, len(g.Image))
	for i, src := range g.Image {
		// 记录绘制前状态，供 DisposalPrevious 恢复。
		prev := image.NewRGBA(bounds)
		draw.Draw(prev, bounds, canvas, image.Point{}, draw.Src)

		// 将当前帧合成到画布上，透明像素不会覆盖已有内容。
		draw.Draw(canvas, src.Bounds(), src, src.Bounds().Min, draw.Over)

		// 输出合成后的快照。
		frame := image.NewRGBA(bounds)
		draw.Draw(frame, bounds, canvas, image.Point{}, draw.Src)

		var buf bytes.Buffer
		if err := imaging.Encode(&buf, frame, imaging.PNG); err != nil {
			return nil, err
		}

		delay := 0
		if i < len(g.Delay) {
			delay = g.Delay[i]
		}
		frames = append(frames, frameData{
			Img:   base64.StdEncoding.EncodeToString(buf.Bytes()),
			Delay: delay,
		})

		// 应用当前帧的 disposal 处理。
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, src.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			draw.Draw(canvas, bounds, prev, image.Point{}, draw.Src)
		}
	}

	return frames, nil
}

func (f *DicFunc) GetGif() string {
	if f.Len < 1 {
		return ""
	}

	frames, err := splitGif(strings.NewReader(f.Inputs.String(1)))
	if err != nil {
		return "解码图像失败"
	}

	s, err := stdjson.Marshal(frames)
	if err != nil {
		return "[]"
	}
	return string(s)
}

func getGif(d *dto.DicInputs) (any, error) {
	frames, err := splitGif(strings.NewReader(d.Inputs.String(1)))
	if err != nil {
		return "解码图像失败", nil
	}

	s, err := stdjson.Marshal(frames)
	if err != nil {
		return "[]", nil
	}
	return string(s), nil
}
