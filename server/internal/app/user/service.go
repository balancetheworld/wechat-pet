package user

import (
	"context"
	"errors"
	"strings"

	familyapp "github.com/balancetheworld/wechat-pet/server/internal/app/family"
	"github.com/balancetheworld/wechat-pet/server/internal/model"
	appErrors "github.com/balancetheworld/wechat-pet/server/internal/pkg/errors"
)

type Service struct {
	users  Repository
	family familyapp.Repository
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

func NewService(users Repository, family familyapp.Repository) (*Service, error) {
	if users == nil || family == nil {
		return nil, errors.New("user service dependencies are required")
	}
	return &Service{users: users, family: family}, nil
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
	avatar := ""
	if value.AvatarAssetID != nil {
		avatar = *value.AvatarAssetID
	}
	return Profile{User: NewUserDTO(value, avatar), Family: familyDTO, Identity: identity}, nil
}
