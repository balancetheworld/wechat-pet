package pet

import (
	"context"
	"errors"
	"strings"

	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	fileservice "github.com/balancetheworld/wechat-pet/internal/service/file"
)

type Service struct {
	repository Repository
	assets     interface {
		AuthorizeFamily(context.Context, string, string) error
	}
}

func NewService(repository Repository, assets ...interface {
	AuthorizeFamily(context.Context, string, string) error
}) (*Service, error) {
	if repository == nil {
		return nil, errors.New("pet service repository is required")
	}
	var assetAuth interface {
		AuthorizeFamily(context.Context, string, string) error
	}
	if len(assets) > 0 {
		assetAuth = assets[0]
	}
	return &Service{repository: repository, assets: assetAuth}, nil
}

func (s *Service) List(ctx context.Context, familyID string) ([]PetDTO, error) {
	values, err := s.repository.List(ctx, familyID)
	if err != nil {
		return nil, appErrors.Internal(err)
	}
	result := make([]PetDTO, 0, len(values))
	for _, value := range values {
		result = append(result, toDTO(value))
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, familyID string, petID string) (PetDTO, error) {
	if strings.TrimSpace(petID) == "" {
		return PetDTO{}, appErrors.InvalidParam("宠物 ID 不能为空")
	}
	value, err := s.repository.Get(ctx, familyID, petID)
	if err != nil {
		return PetDTO{}, mapError(err)
	}
	return toDTO(value), nil
}

func (s *Service) Create(ctx context.Context, familyID string, userID string, request CreatePetRequest) (PetDTO, error) {
	name, err := validateName(request.Name)
	if err != nil {
		return PetDTO{}, err
	}
	if err := s.authorizeAsset(ctx, request.AvatarAssetID, familyID); err != nil {
		return PetDTO{}, err
	}
	value, err := s.repository.Create(ctx, familyID, userID, name)
	if err != nil {
		return PetDTO{}, appErrors.Internal(err)
	}
	/* 创建时若带档案字段, 立即补一次全量更新 (与 Update 共用 SQL 路径) */
	if request.Breed != "" || request.Gender != "" || request.Birthday != "" || request.HomeDate != "" {
		full := UpdatePetRequest{
			Name:       name,
			Breed:      request.Breed,
			Gender:     request.Gender,
			Sterilized: request.Sterilized,
			Birthday:   request.Birthday,
			HomeDate:   request.HomeDate,
		}
		full.Gender = normalizeGender(full.Gender)
		if _, err := s.repository.Update(ctx, familyID, value.ID, userID, full); err != nil {
			return PetDTO{}, appErrors.Internal(err)
		}
	}
	if request.AvatarAssetID != "" {
		if creator, ok := s.repository.(interface {
			SetAvatar(context.Context, string, string, string) error
		}); ok {
			if err := creator.SetAvatar(ctx, familyID, value.ID, request.AvatarAssetID); err != nil {
				return PetDTO{}, appErrors.Internal(err)
			}
		}
	}
	return toDTO(value), nil
}

func (s *Service) Update(ctx context.Context, familyID string, petID string, userID string, request UpdatePetRequest) (PetDTO, error) {
	if strings.TrimSpace(petID) == "" {
		return PetDTO{}, appErrors.InvalidParam("宠物 ID 不能为空")
	}
	if _, err := validateName(request.Name); err != nil {
		return PetDTO{}, err
	}
	/* gender 归一: 空值视为 unknown, 仅接受三个合法值 */
	request.Gender = normalizeGender(request.Gender)
	request.Breed = strings.TrimSpace(request.Breed)
	value, err := s.repository.Update(ctx, familyID, petID, userID, request)
	if err != nil {
		return PetDTO{}, mapError(err)
	}
	return toDTO(value), nil
}

func normalizeGender(value string) string {
	switch strings.TrimSpace(value) {
	case "male", "female":
		return strings.TrimSpace(value)
	default:
		return "unknown"
	}
}

func (s *Service) authorizeAsset(ctx context.Context, assetID, familyID string) error {
	if s.assets == nil || strings.TrimSpace(assetID) == "" {
		return nil
	}
	if err := s.assets.AuthorizeFamily(ctx, assetID, familyID); err != nil {
		if errors.Is(err, fileservice.ErrAssetNotOwnedByFamily) {
			return appErrors.Forbidden()
		}
		return appErrors.Internal(err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, familyID string, petID string, userID string) error {
	if strings.TrimSpace(petID) == "" {
		return appErrors.InvalidParam("宠物 ID 不能为空")
	}
	return mapError(s.repository.Delete(ctx, familyID, petID, userID))
}

func validateName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" || len(name) > 50 {
		return "", appErrors.InvalidParam("宠物名称长度需为 1-50 个字符")
	}
	return name, nil
}

func toDTO(value Pet) PetDTO {
	return PetDTO{ID: value.ID, Name: value.Name}
}

func mapError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return appErrors.NotFound("宠物不存在")
	}
	if err == nil {
		return nil
	}
	return appErrors.Internal(err)
}
