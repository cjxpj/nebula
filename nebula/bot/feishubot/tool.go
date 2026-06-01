package feishubot

import (
	"encoding/json"
	"strings"
)

// 结构体定义（当前包内）
type RichContent struct {
	Title   string    `json:"title"`
	Content [][]Block `json:"content"`
}
type Block struct {
	Tag       string `json:"tag"`
	Text      string `json:"text,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	UserName  string `json:"user_name,omitempty"`
	ImageKey  string `json:"image_key,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	EmojiType string `json:"emoji_type,omitempty"`
	Language  string `json:"language,omitempty"`
}

type MessageText struct {
	Text string `json:"text"`
}

type MessageImg struct {
	ImageKey string `json:"image_key"`
}

func extractText(content string) string {
	// 纯文本
	var simple MessageText
	if err := json.Unmarshal([]byte(content), &simple); err == nil && simple.Text != "" {
		return simple.Text
	}
	// 纯图片
	var img MessageImg
	if err := json.Unmarshal([]byte(content), &img); err == nil && img.ImageKey != "" {
		return "[img=" + img.ImageKey + "]"
	}

	// 富文本
	var rich RichContent
	if err := json.Unmarshal([]byte(content), &rich); err == nil && rich.Content != nil {
		var sb strings.Builder
		for _, line := range rich.Content {
			for _, b := range line {
				switch b.Tag {
				case "text":
					sb.WriteString(b.Text)
				case "at":
					sb.WriteString("@")
					sb.WriteString(b.UserName)
					sb.WriteString(" ")
				case "img":
					sb.WriteString("[img=")
					sb.WriteString(b.ImageKey)
					sb.WriteString("]")
				case "emotion":
					sb.WriteString("[表情]")
				case "code_block":
					sb.WriteString("[code_block=,code=]")
				}
			}
		}
		return sb.String()
	}
	// 3. 兜底
	return content
}
