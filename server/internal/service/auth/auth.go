package auth

import (
	"context"
	"fmt"

	"github.com/balancetheworld/wechat-pet/server/internal/platform/wechat"
)

type Service struct {
	wechat wechat.Client
}

func NewService(client wechat.Client) (*Service, error) {
	if client == nil {
		return nil, fmt.Errorf("wechat client is required")
	}
	return &Service{wechat: client}, nil
}

func (s *Service) Login(ctx context.Context, code string) (wechat.SessionResult, error) {
	return s.wechat.Code2Session(ctx, code)
}
