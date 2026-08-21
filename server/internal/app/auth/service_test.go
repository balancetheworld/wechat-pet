package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	familyapp "github.com/balancetheworld/wechat-pet/server/internal/app/family"
	"github.com/balancetheworld/wechat-pet/server/internal/model"
	"github.com/balancetheworld/wechat-pet/server/internal/platform/wechat"
)

type fakeWeChat struct {
	session wechat.SessionResult
	err     error
}

func (f fakeWeChat) Code2Session(context.Context, string) (wechat.SessionResult, error) {
	return f.session, f.err
}

type fakeUsers struct {
	user model.User
}

func (f *fakeUsers) GetByID(context.Context, string) (model.User, error) { return f.user, nil }
func (f *fakeUsers) GetByOpenID(context.Context, string) (model.User, error) {
	return f.user, nil
}
func (f *fakeUsers) UpsertByOpenID(context.Context, string, *string, time.Time) (model.User, error) {
	if f.user.ID == "" {
		f.user = model.User{ID: "user-1", OpenID: "openid-1"}
	}
	return f.user, nil
}
func (f *fakeUsers) UpdateProfile(context.Context, string, string, string) (model.User, error) {
	return f.user, nil
}

type fakeFamily struct {
	summary *familyapp.Summary
}

func (f fakeFamily) GetActiveFamilySummary(context.Context, string) (*familyapp.Summary, error) {
	return f.summary, nil
}

type fakeTokens struct{}

func (fakeTokens) Issue(string) (string, error) { return "token", nil }

func TestLoginWithWeChat(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wechat   fakeWeChat
		family   *familyapp.Summary
		identity string
		wantErr  bool
	}{
		{name: "new guest", code: "code", wechat: fakeWeChat{session: wechat.SessionResult{OpenID: "openid-1"}}, identity: "guest"},
		{name: "existing user", code: "code", wechat: fakeWeChat{session: wechat.SessionResult{OpenID: "openid-1"}}, identity: "guest"},
		{name: "invalid code", code: "", wechat: fakeWeChat{}, wantErr: true},
		{name: "family owner", code: "code", wechat: fakeWeChat{session: wechat.SessionResult{OpenID: "openid-1"}}, family: &familyapp.Summary{ID: "family-1", Name: "家庭", Role: "owner"}, identity: "owner"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewService(test.wechat, &fakeUsers{}, fakeFamily{summary: test.family}, fakeTokens{})
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.LoginWithWeChat(context.Background(), test.code)
			if test.wantErr {
				if err == nil {
					t.Fatal("LoginWithWeChat() error = nil")
				}
				return
			}
			if err != nil || result.Identity != test.identity || result.Token != "token" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestLoginWithWeChatMapsWeChatError(t *testing.T) {
	service, err := NewService(fakeWeChat{err: errors.New("third-party failure")}, &fakeUsers{}, fakeFamily{}, fakeTokens{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.LoginWithWeChat(context.Background(), "code"); err == nil || err.Error() == "third-party failure" {
		t.Fatalf("error=%v, want mapped error", err)
	}
}
