package auth

import (
	"context"
	"testing"

	"github.com/balancetheworld/wechat-pet/server/internal/platform/wechat"
)

type fakeWeChatClient struct{}

func (fakeWeChatClient) Code2Session(context.Context, string) (wechat.SessionResult, error) {
	return wechat.SessionResult{OpenID: "fake-openid", UnionID: "fake-unionid"}, nil
}

func TestLoginUsesFakeWeChatClient(t *testing.T) {
	service, err := NewService(fakeWeChatClient{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Login(context.Background(), "code")
	if err != nil || result.OpenID != "fake-openid" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
