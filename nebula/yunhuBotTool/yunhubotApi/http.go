package yunhubotapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	APIURL = "https://chat-go.jwzhd.com/open-apis/v1" // 您要求的常量
)

// post 公共 POST 逻辑
func postJson(c *RouterYunHuBot, urlpath string, payload any) error {
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/%s", APIURL, urlpath)

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s?token=%s", url, c.Secret),
		bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jwzhd: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}
