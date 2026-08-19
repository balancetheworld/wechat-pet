package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const code2SessionURL = "https://api.weixin.qq.com/sns/jscode2session"

type SessionResult struct {
	OpenID  string
	UnionID string
}

type Client interface {
	Code2Session(ctx context.Context, code string) (SessionResult, error)
}

type HTTPClient struct {
	appID     string
	appSecret string
	endpoint  string
	http      *http.Client
}

type apiResponse struct {
	OpenID     string `json:"openid"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMessage string `json:"errmsg"`
}

func NewHTTPClient(appID, appSecret string, timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPClient{appID: appID, appSecret: appSecret, endpoint: code2SessionURL, http: &http.Client{Timeout: timeout}}
}

func (c *HTTPClient) Code2Session(ctx context.Context, code string) (SessionResult, error) {
	if strings.TrimSpace(code) == "" {
		return SessionResult{}, fmt.Errorf("wechat code is required")
	}
	query := url.Values{}
	query.Set("appid", c.appID)
	query.Set("secret", c.appSecret)
	query.Set("js_code", code)
	query.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return SessionResult{}, fmt.Errorf("create wechat request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return SessionResult{}, fmt.Errorf("wechat request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return SessionResult{}, fmt.Errorf("wechat request returned status %d", resp.StatusCode)
	}
	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return SessionResult{}, fmt.Errorf("decode wechat response: %w", err)
	}
	if result.ErrCode != 0 {
		return SessionResult{}, fmt.Errorf("wechat api error %d: %s", result.ErrCode, result.ErrMessage)
	}
	return SessionResult{OpenID: result.OpenID, UnionID: result.UnionID}, nil
}
