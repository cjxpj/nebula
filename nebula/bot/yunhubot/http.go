package yunhubot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	yunhubot_dto "github.com/cjxpj/nebula/bot/yunhubot/dto"
	"github.com/cjxpj/nebula/dto"
)

var yunhuHTTPClient = &http.Client{Timeout: 30 * time.Second}

// post 公共 POST 逻辑
func postJson(urlpath string, payload any) error {
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/%s", yunhubot_dto.APIURL, urlpath)

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s?token=%s", url, dto.ServerConfig.YunHuBot.Secret),
		bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := yunhuHTTPClient.Do(req)
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
