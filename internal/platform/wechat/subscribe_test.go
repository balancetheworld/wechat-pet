package wechat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
)

func wechatJSONResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestSubscribeClientSendsMessageWithCachedToken(t *testing.T) {
	client, err := NewSubscribeClient("app", "secret", "developer", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	tokenCalls, sendCalls := 0, 0
	client.tokenURL = "https://wechat.test/token"
	client.sendURL = "https://wechat.test/send"
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/token":
			tokenCalls++
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"grant_type":"client_credential"`) || !strings.Contains(string(body), `"appid":"app"`) {
				t.Fatalf("token payload = %s", body)
			}
			return wechatJSONResponse(`{"access_token":"token-1","expires_in":7200}`), nil
		case "/send":
			sendCalls++
			if r.URL.Query().Get("access_token") != "token-1" {
				t.Fatalf("access token = %q", r.URL.Query().Get("access_token"))
			}
			var payload struct {
				ToUser   string                       `json:"touser"`
				Template string                       `json:"template_id"`
				Page     string                       `json:"page"`
				State    string                       `json:"miniprogram_state"`
				Data     map[string]map[string]string `json:"data"`
			}
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("send payload = %s: %v", body, err)
			}
			if payload.ToUser != "openid-1" || payload.Template != "tpl-1" || payload.Page != reminderTemplatePage || payload.State != "developer" {
				t.Fatalf("send payload = %+v", payload)
			}
			if payload.Data["thing1"]["value"] != "团子" {
				t.Fatalf("send data = %+v", payload.Data)
			}
			return wechatJSONResponse(`{"errcode":0,"errmsg":"ok"}`), nil
		}
		return nil, errors.New("unexpected request path " + r.URL.Path)
	})}
	for index := 0; index < 2; index++ {
		if err := client.SendSubscribeMessage(context.Background(), "openid-1", "tpl-1", reminderTemplatePage, map[string]string{"thing1": "团子"}); err != nil {
			t.Fatal(err)
		}
	}
	if tokenCalls != 1 || sendCalls != 2 {
		t.Fatalf("token calls = %d, send calls = %d, want cached token reused", tokenCalls, sendCalls)
	}
}

func TestSubscribeClientRefreshesInvalidToken(t *testing.T) {
	client, err := NewSubscribeClient("app", "secret", "formal", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	tokenCalls, forceRefresh := 0, 0
	client.tokenURL = "https://wechat.test/token"
	client.sendURL = "https://wechat.test/send"
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/token" {
			tokenCalls++
			var payload struct {
				ForceRefresh bool `json:"force_refresh"`
			}
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.ForceRefresh {
				forceRefresh++
			}
			return wechatJSONResponse(`{"access_token":"token-` + string(rune('0'+tokenCalls)) + `","expires_in":7200}`), nil
		}
		if tokenCalls == 1 {
			return wechatJSONResponse(`{"errcode":40001,"errmsg":"invalid credential"}`), nil
		}
		return wechatJSONResponse(`{"errcode":0,"errmsg":"ok"}`), nil
	})}
	if err := client.SendSubscribeMessage(context.Background(), "openid-1", "tpl-1", reminderTemplatePage, map[string]string{"thing1": "团子"}); err != nil {
		t.Fatal(err)
	}
	if tokenCalls != 2 || forceRefresh != 1 {
		t.Fatalf("token calls = %d, force refresh = %d, want one forced refresh", tokenCalls, forceRefresh)
	}
}

func TestSubscribeClientReportsRejectedSubscription(t *testing.T) {
	client, err := NewSubscribeClient("app", "secret", "formal", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.tokenURL = "https://wechat.test/token"
	client.sendURL = "https://wechat.test/send"
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/token" {
			return wechatJSONResponse(`{"access_token":"token-1","expires_in":7200}`), nil
		}
		return wechatJSONResponse(`{"errcode":43101,"errmsg":"user refuse to accept the msg"}`), nil
	})}
	err = client.SendSubscribeMessage(context.Background(), "openid-1", "tpl-1", reminderTemplatePage, map[string]string{"thing1": "团子"})
	var subscribeErr *SubscribeError
	if !errors.As(err, &subscribeErr) || !subscribeErr.Permanent() || subscribeErr.ErrCode != errCodeSubscribeRejected {
		t.Fatalf("error = %v, want permanent 43101", err)
	}
}

func TestReminderNotifierMapsPermanentRejection(t *testing.T) {
	client, err := NewSubscribeClient("app", "secret", "formal", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.tokenURL = "https://wechat.test/token"
	client.sendURL = "https://wechat.test/send"
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/token" {
			return wechatJSONResponse(`{"access_token":"token-1","expires_in":7200}`), nil
		}
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Data map[string]map[string]string `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if len([]rune(payload.Data[reminderTemplateItemKeyword]["value"])) != reminderKeywordMaxRunes {
			t.Fatalf("item value = %q, want truncated to %d runes", payload.Data[reminderTemplateItemKeyword]["value"], reminderKeywordMaxRunes)
		}
		return wechatJSONResponse(`{"errcode":43101,"errmsg":"user refuse to accept the msg"}`), nil
	})}
	notifier, err := NewReminderNotifier(client, "tpl-1")
	if err != nil {
		t.Fatal(err)
	}
	notice := calendarapp.ReminderNotice{OpenID: "openid-1", PetName: "团子", Content: strings.Repeat("喵", 40), ReminderDate: "2026-10-22"}
	if err := notifier.NotifyReminder(context.Background(), notice); !errors.Is(err, calendarapp.ErrReminderNotDeliverable) {
		t.Fatalf("error = %v, want not deliverable", err)
	}
}
