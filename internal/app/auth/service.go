package auth

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	userapp "github.com/balancetheworld/wechat-pet/server/internal/app/user"
	appErrors "github.com/balancetheworld/wechat-pet/server/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/server/internal/platform/wechat"
)

type TokenIssuer interface {
	Issue(userID string) (string, error)
}

type Service struct {
	wechat wechat.Client
	users  UserRepository
	family FamilyRepository
	tokens TokenIssuer
}

func NewService(client wechat.Client, users UserRepository, family FamilyRepository, tokens TokenIssuer) (*Service, error) {
	if client == nil || users == nil || family == nil || tokens == nil {
		return nil, stderrors.New("auth service dependencies are required")
	}
	return &Service{wechat: client, users: users, family: family, tokens: tokens}, nil
}

func (s *Service) LoginWithWeChat(ctx context.Context, code string) (LoginResponse, error) {
	if strings.TrimSpace(code) == "" {
		return LoginResponse{}, appErrors.InvalidParam("微信登录 code 不能为空")
	}
	session, err := s.wechat.Code2Session(ctx, code)
	if err != nil || strings.TrimSpace(session.OpenID) == "" {
		return LoginResponse{}, appErrors.InvalidParam("微信登录失败")
	}
	var unionID *string
	if strings.TrimSpace(session.UnionID) != "" {
		value := session.UnionID
		unionID = &value
	}
	user, err := s.users.UpsertByOpenID(ctx, session.OpenID, unionID, time.Now().UTC())
	if err != nil {
		return LoginResponse{}, appErrors.Internal(err)
	}
	family, err := s.family.GetActiveFamilySummary(ctx, user.ID)
	if err != nil {
		return LoginResponse{}, appErrors.Internal(err)
	}
	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return LoginResponse{}, appErrors.Internal(err)
	}
	identity := "guest"
	var familyDTO *FamilySummaryDTO
	if family != nil {
		identity = family.Role
		if identity != "member" && identity != "owner" {
			identity = "guest"
		}
		familyDTO = &FamilySummaryDTO{ID: family.ID, Name: family.Name}
	}
	avatar := ""
	if user.AvatarAssetID != nil {
		avatar = *user.AvatarAssetID
	}
	return LoginResponse{Token: token, User: userapp.NewUserDTO(user, avatar), Family: familyDTO, Identity: identity}, nil
}
