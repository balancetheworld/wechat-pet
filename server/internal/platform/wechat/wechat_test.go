package wechat

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCode2SessionParsesSessionResult(t *testing.T) {
	client := NewHTTPClient("app", "secret", time.Second)
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("secret") != "secret" || r.URL.Query().Get("js_code") != "code" {
			t.Fatal("wechat credentials or code were not sent")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"openid":"openid-1","unionid":"unionid-1"}`)), Header: make(http.Header)}, nil
	})}
	client.endpoint = "https://wechat.test/session"
	result, err := client.Code2Session(context.Background(), "code")
	if err != nil || result.OpenID != "openid-1" || result.UnionID != "unionid-1" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestCode2SessionHandlesWechatErrorWithoutSecrets(t *testing.T) {
	client := NewHTTPClient("app", "secret-value", time.Second)
	client.http = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"errcode":40029,"errmsg":"invalid code"}`)), Header: make(http.Header)}, nil
	})}
	client.endpoint = "https://wechat.test/session"
	_, err := client.Code2Session(context.Background(), "code")
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
