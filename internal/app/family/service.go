package family

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
		return nil, errors.New("family service repository is required")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) Create(ctx context.Context, userID string, request CreateFamilyRequest) (FamilyDetailDTO, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" || len(name) > 50 {
		return FamilyDetailDTO{}, appErrors.InvalidParam("家庭名称长度需为 1-50 个字符")
	}
	current, err := s.repository.GetActiveFamilySummary(ctx, userID)
	if err != nil {
		return FamilyDetailDTO{}, appErrors.Internal(err)
	}
	if current != nil {
		return FamilyDetailDTO{}, appErrors.Conflict("已有家庭，不能重复创建")
	}
	value, err := s.repository.Create(ctx, userID, name)
	if err != nil {
		return FamilyDetailDTO{}, mapError(err)
	}
	return toFamilyDetailDTO(value), nil
}

func (s *Service) Current(ctx context.Context, userID string) (FamilyDetailDTO, error) {
	value, err := s.repository.GetCurrent(ctx, userID)
	if err != nil {
		return FamilyDetailDTO{}, mapError(err)
	}
	if value == nil {
		return FamilyDetailDTO{}, appErrors.NotFound("当前没有家庭")
	}
	return toFamilyDetailDTO(*value), nil
}

func (s *Service) ApplyJoin(ctx context.Context, userID string, request JoinFamilyRequest) (JoinApplicationDTO, error) {
	code := strings.ToUpper(strings.TrimSpace(request.Code))
	if len(code) < 4 || len(code) > 32 {
		return JoinApplicationDTO{}, appErrors.InvalidParam("家庭码长度无效")
	}
	current, err := s.repository.GetActiveFamilySummary(ctx, userID)
	if err != nil {
		return JoinApplicationDTO{}, appErrors.Internal(err)
	}
	if current != nil {
		return JoinApplicationDTO{}, appErrors.Conflict("已有家庭，不能申请加入")
	}
	value, err := s.repository.CreateJoinApplication(ctx, userID, code)
	if err != nil {
		return JoinApplicationDTO{}, mapError(err)
	}
	return toJoinApplicationDTO(value), nil
}

func (s *Service) MyJoinApplication(ctx context.Context, userID string) (*JoinApplicationDTO, error) {
	value, err := s.repository.GetMyJoinApplication(ctx, userID)
	if err != nil {
		return nil, mapError(err)
	}
	if value == nil {
		return nil, nil
	}
	result := toJoinApplicationDTO(*value)
	return &result, nil
}

func (s *Service) Members(ctx context.Context, familyID string) ([]FamilyMemberDTO, error) {
	values, err := s.repository.ListMembers(ctx, familyID)
	if err != nil {
		return nil, mapError(err)
	}
	result := make([]FamilyMemberDTO, 0, len(values))
	for _, value := range values {
		result = append(result, FamilyMemberDTO{ID: value.ID, UserID: value.UserID, Nickname: value.Nickname, Avatar: value.Avatar, Role: value.Role, Status: value.Status})
	}
	return result, nil
}

func (s *Service) PendingApplications(ctx context.Context, familyID string) ([]JoinApplicationDTO, error) {
	values, err := s.repository.ListPendingApplications(ctx, familyID)
	if err != nil {
		return nil, mapError(err)
	}
	result := make([]JoinApplicationDTO, 0, len(values))
	for _, value := range values {
		result = append(result, toJoinApplicationDTO(value))
	}
	return result, nil
}

func (s *Service) ApproveApplication(ctx context.Context, familyID string, applicationID string) error {
	if strings.TrimSpace(applicationID) == "" {
		return appErrors.InvalidParam("申请 ID 不能为空")
	}
	return mapError(s.repository.ApproveApplication(ctx, familyID, applicationID))
}

func (s *Service) RejectApplication(ctx context.Context, familyID string, applicationID string) error {
	if strings.TrimSpace(applicationID) == "" {
		return appErrors.InvalidParam("申请 ID 不能为空")
	}
	return mapError(s.repository.RejectApplication(ctx, familyID, applicationID))
}

func (s *Service) RemoveMember(ctx context.Context, familyID string, memberID string, currentUserID string) error {
	if strings.TrimSpace(memberID) == "" {
		return appErrors.InvalidParam("成员 ID 不能为空")
	}
	return mapError(s.repository.RemoveMember(ctx, familyID, memberID, currentUserID))
}

func toFamilyDetailDTO(value Detail) FamilyDetailDTO {
	return FamilyDetailDTO{ID: value.ID, Name: value.Name, JoinCode: value.Code, Role: value.Role}
}

func toJoinApplicationDTO(value JoinApplication) JoinApplicationDTO {
	return JoinApplicationDTO{ID: value.ID, FamilyID: value.FamilyID, FamilyName: value.FamilyName, UserID: value.UserID, Nickname: value.Nickname, Avatar: value.Avatar, Status: value.Status}
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return appErrors.NotFound("家庭或申请不存在")
	}
	if errors.Is(err, ErrAlreadyInFamily) {
		return appErrors.Conflict("已有 active 家庭")
	}
	if errors.Is(err, ErrApplicationProcessed) {
		return appErrors.Conflict("申请已处理")
	}
	if errors.Is(err, ErrCannotRemoveSelf) {
		return appErrors.Conflict("owner 不能移除自己")
	}
	return appErrors.Internal(err)
}
