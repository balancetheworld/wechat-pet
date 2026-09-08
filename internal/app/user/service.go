package user

import (
	"context"
	"errors"
	"strings"

	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	"github.com/balancetheworld/wechat-pet/internal/model"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
)

type Service struct {
	users      Repository
	family     familyapp.Repository
	avatarURLs AvatarURLResolver
	assetAuth  interface {
		AuthorizeUser(context.Context, string, string) error
		AuthorizeFamily(context.Context, string, string) error
	}
}

type AvatarURLResolver interface {
	URL(ctx context.Context, key string) (string, error)
}

type FamilySummary struct {
	ID   string
	Name string
}

type Profile struct {
	User     UserDTO
	Family   *FamilySummary
	Identity string
}

func NewService(users Repository, family familyapp.Repository, avatarURLs ...AvatarURLResolver) (*Service, error) {
	if users == nil || family == nil {
		return nil, errors.New("user service dependencies are required")
	}
	var avatarURLResolver AvatarURLResolver
	if len(avatarURLs) > 0 {
		avatarURLResolver = avatarURLs[0]
	}
	return &Service{users: users, family: family, avatarURLs: avatarURLResolver}, nil
}

func (s *Service) SetAssetAuthorizer(value interface {
	AuthorizeUser(context.Context, string, string) error
	AuthorizeFamily(context.Context, string, string) error
}) {
	s.assetAuth = value
}

func (s *Service) Me(ctx context.Context, userID string) (Profile, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return Profile{}, appErrors.Internal(err)
	}
	return s.meResponse(ctx, user)
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, request UpdateProfileRequest) (Profile, error) {
	if strings.TrimSpace(request.Nickname) == "" || strings.TrimSpace(request.AvatarAssetID) == "" {
		return Profile{}, appErrors.InvalidParam("昵称和头像不能为空")
	}
	if s.assetAuth != nil {
		if err := s.assetAuth.AuthorizeUser(ctx, request.AvatarAssetID, userID); err != nil {
			family, familyErr := s.family.GetActiveFamilySummary(ctx, userID)
			if familyErr != nil {
				return Profile{}, appErrors.Internal(familyErr)
			}
			if family == nil || s.assetAuth.AuthorizeFamily(ctx, request.AvatarAssetID, family.ID) != nil {
				return Profile{}, appErrors.Forbidden()
			}
		}
	}
	user, err := s.users.UpdateProfile(ctx, userID, request.Nickname, request.AvatarAssetID)
	if err != nil {
		return Profile{}, appErrors.Internal(err)
	}
	return s.meResponse(ctx, user)
}

func (s *Service) meResponse(ctx context.Context, value model.User) (Profile, error) {
	family, err := s.family.GetActiveFamilySummary(ctx, value.ID)
	if err != nil {
		return Profile{}, appErrors.Internal(err)
	}
	identity := "guest"
	var familyDTO *FamilySummary
	if family != nil {
		identity = family.Role
		if identity != "member" && identity != "owner" {
			identity = "guest"
		}
		familyDTO = &FamilySummary{ID: family.ID, Name: family.Name}
	}
	avatar, err := s.avatarURL(ctx, value.AvatarAssetID)
	if err != nil {
		return Profile{}, appErrors.Internal(err)
	}
	return Profile{User: NewUserDTO(value, avatar), Family: familyDTO, Identity: identity}, nil
}

func (s *Service) avatarURL(ctx context.Context, assetID *string) (string, error) {
	if assetID == nil || strings.TrimSpace(*assetID) == "" {
		return "", nil
	}
	if s.avatarURLs == nil {
		return *assetID, nil
	}
	return s.avatarURLs.URL(ctx, *assetID)
}
