package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	stableAccessTokenURL = "https://api.weixin.qq.com/cgi-bin/stable_token"
	subscribeMessageURL  = "https://api.weixin.qq.com/cgi-bin/message/subscribe/send"
	accessTokenMargin    = 60 * time.Second
	subscribeMessageLang = "zh_CN"
)

const (
	errCodeAccessTokenInvalid    = 40001
	errCodeAccessTokenExpired    = 42001
	errCodeSubscribeRejected     = 43101
	errCodeSubscribeTemplate     = 40037
	errCodeSubscribePage         = 41030
	errCodeSubscribeParamInvalid = 47003
)

type SubscribeError struct {
	ErrCode int
	ErrMsg  string
}

func (e *SubscribeError) Error() string {
	return fmt.Sprintf("wechat subscribe message error %d: %s", e.ErrCode, e.ErrMsg)
}

func (e *SubscribeError) TokenInvalid() bool {
	return e.ErrCode == errCodeAccessTokenInvalid || e.ErrCode == errCodeAccessTokenExpired
}

func (e *SubscribeError) Permanent() bool {
	switch e.ErrCode {
	case errCodeSubscribeRejected, errCodeSubscribeTemplate, errCodeSubscribePage, errCodeSubscribeParamInvalid:
		return true
	}
	return false
}

type SubscribeClient struct {
	appID     string
	appSecret string
	state     string
	tokenURL  string
	sendURL   string
	http      *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewSubscribeClient(appID, appSecret, miniprogramState string, timeout time.Duration) (*SubscribeClient, error) {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	if appID == "" || appSecret == "" {
		return nil, errors.New("wechat app id and secret are required for subscribe messages")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	miniprogramState = strings.TrimSpace(miniprogramState)
	if miniprogramState == "" {
		miniprogramState = "formal"
	}
	return &SubscribeClient{
		appID:     appID,
		appSecret: appSecret,
		state:     miniprogramState,
		tokenURL:  stableAccessTokenURL,
		sendURL:   subscribeMessageURL,
		http:      &http.Client{Timeout: timeout},
	}, nil
}

func (c *SubscribeClient) SendSubscribeMessage(ctx context.Context, openid, templateID, page string, data map[string]string) error {
	if strings.TrimSpace(openid) == "" {
		return errors.New("wechat openid is required")
	}
	if strings.TrimSpace(templateID) == "" {
		return errors.New("wechat subscribe template id is required")
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.accessToken(ctx, attempt > 0)
		if err != nil {
			return err
		}
		lastErr = c.sendMessage(ctx, token, openid, templateID, page, data)
		var subscribeErr *SubscribeError
		if errors.As(lastErr, &subscribeErr) && subscribeErr.TokenInvalid() && attempt == 0 {
			continue
		}
		return lastErr
	}
	return lastErr
}

func (c *SubscribeClient) accessToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.token != "" && time.Now().Before(c.expiresAt) {
		return c.token, nil
	}
	payload := struct {
		GrantType    string `json:"grant_type"`
		AppID        string `json:"appid"`
		Secret       string `json:"secret"`
		ForceRefresh bool   `json:"force_refresh"`
	}{GrantType: "client_credential", AppID: c.appID, Secret: c.appSecret, ForceRefresh: force}
	raw, err := c.request(ctx, c.tokenURL, payload)
	if err != nil {
		return "", err
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("wechat access token response invalid: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("wechat access token missing: errcode %d", result.ErrCode)
	}
	ttl := time.Duration(result.ExpiresIn)*time.Second - accessTokenMargin
	if ttl <= 0 {
		ttl = time.Minute
	}
	c.token, c.expiresAt = result.AccessToken, time.Now().Add(ttl)
	return c.token, nil
}

func (c *SubscribeClient) sendMessage(ctx context.Context, token, openid, templateID, page string, data map[string]string) error {
	values := make(map[string]map[string]string, len(data))
	for key, value := range data {
		values[key] = map[string]string{"value": value}
	}
	payload := struct {
		ToUser           string                       `json:"touser"`
		TemplateID       string                       `json:"template_id"`
		Page             string                       `json:"page,omitempty"`
		MiniProgramState string                       `json:"miniprogram_state"`
		Lang             string                       `json:"lang"`
		Data             map[string]map[string]string `json:"data"`
	}{ToUser: openid, TemplateID: templateID, Page: page, MiniProgramState: c.state, Lang: subscribeMessageLang, Data: values}
	raw, err := c.request(ctx, c.sendURL+"?access_token="+url.QueryEscape(token), payload)
	if err != nil {
		return err
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("wechat subscribe message response invalid: %w", err)
	}
	if result.ErrCode != 0 {
		return &SubscribeError{ErrCode: result.ErrCode, ErrMsg: result.ErrMsg}
	}
	return nil
}

func (c *SubscribeClient) request(ctx context.Context, endpoint string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create wechat subscribe request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("wechat subscribe request failed: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return nil, fmt.Errorf("read wechat subscribe response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wechat subscribe http status %d", response.StatusCode)
	}
	return raw, nil
}
