package yunhubot

// recvType 支持 "user" 或 "group"
func SendText(recvID, recvType, text string) error {
	url := "bot/send"
	body := map[string]any{
		"recvId":      recvID,
		"recvType":    recvType,
		"contentType": "text",
		"content": map[string]string{
			"text": text,
		},
	}
	return postJson(url, body)
}
