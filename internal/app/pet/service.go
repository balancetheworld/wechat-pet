package pet

import (
	"context"
	"errors"
	"strings"

	appErrors "github.com/balancetheworld/wechat-pet/server/internal/pkg/errors"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("pet service repository is required")
	}
	return &Service{repository: repository}, nil
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
	value, err := s.repository.Create(ctx, familyID, userID, name)
	if err != nil {
		return PetDTO{}, appErrors.Internal(err)
	}
	return toDTO(value), nil
}

func (s *Service) Update(ctx context.Context, familyID string, petID string, userID string, request UpdatePetRequest) (PetDTO, error) {
	if strings.TrimSpace(petID) == "" {
		return PetDTO{}, appErrors.InvalidParam("宠物 ID 不能为空")
	}
	name, err := validateName(request.Name)
	if err != nil {
		return PetDTO{}, err
	}
	value, err := s.repository.Update(ctx, familyID, petID, userID, name)
	if err != nil {
		return PetDTO{}, mapError(err)
	}
	return toDTO(value), nil
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
