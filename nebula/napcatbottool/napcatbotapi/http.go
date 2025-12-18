package napcatbotapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// POST
func postJson(c *RouterNapCatBot, urlpath string, payload any) ([]byte, error) {
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s%s", c.APIAddr, urlpath)

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Secret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwzhd: status=%d body=%s", resp.StatusCode, string(body))
	}
	return body, nil
}
