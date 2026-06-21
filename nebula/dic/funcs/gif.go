package funcs

import (
	"bytes"
	"encoding/base64"
	"image/gif"
	"strings"
	stdjson "encoding/json"

	"github.com/cjxpj/nebula/dto"
	"github.com/disintegration/imaging"
)

func (f *DicFunc) GetGif() string {
	if f.Len < 1 {
		return ""
	}

	imgData := strings.NewReader(f.Inputs.String(1))
	imgs, err := gif.DecodeAll(imgData)
	if err != nil {
		return "解码图像失败"
	}

	type FrameData struct {
		Img   string `json:"img"`
		Delay int    `json:"delay"` // 单位是100分之一秒
	}

	var allFrames []FrameData

	for i, frame := range imgs.Image {
		var buf bytes.Buffer
		err := imaging.Encode(&buf, frame, imaging.PNG)
		if err != nil {
			return "null"
		}
		delay := 0
		if i < len(imgs.Delay) {
			delay = imgs.Delay[i] // delay单位是 1/100 秒
		}
		allFrames = append(allFrames, FrameData{
			Img:   base64.StdEncoding.EncodeToString(buf.Bytes()),
			Delay: delay,
		})
	}

	s, err := json.Marshal(allFrames)
	if err != nil {
		return "[]"
	}
	return string(s)
}

func getGif(d *dto.DicInputs) (any, error) {
	imgData := strings.NewReader(d.Inputs.String(1))
	imgs, err := gif.DecodeAll(imgData)
	if err != nil {
		return "解码图像失败", nil
	}

	type frameData struct {
		Img   string `json:"img"`
		Delay int    `json:"delay"`
	}

	var allFrames []frameData
	for i, frame := range imgs.Image {
		var buf bytes.Buffer
		if err := imaging.Encode(&buf, frame, imaging.PNG); err != nil {
			return "null", nil
		}
		delay := 0
		if i < len(imgs.Delay) {
			delay = imgs.Delay[i]
		}
		allFrames = append(allFrames, frameData{
			Img:   base64.StdEncoding.EncodeToString(buf.Bytes()),
			Delay: delay,
		})
	}

	s, err := stdjson.Marshal(allFrames)
	if err != nil {
		return "[]", nil
	}
	return string(s), nil
}
