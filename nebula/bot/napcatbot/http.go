package napcatbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cjxpj/nebula/dto"
)

var napcatHTTPClient = &http.Client{Timeout: 30 * time.Second}

// POST
func postJson(urlpath string, payload any) ([]byte, error) {
	secret := dto.ServerConfig.NapCatBot.Secret
	apiUrl := dto.ServerConfig.NapCatBot.APIAddr
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s%s", apiUrl, urlpath)

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)

	resp, err := napcatHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jwzhd: read body failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwzhd: status=%d body=%s", resp.StatusCode, string(body))
	}
	return body, nil
}
