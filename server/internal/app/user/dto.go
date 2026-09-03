package user

import (
	"strings"

	"github.com/balancetheworld/wechat-pet/internal/model"
)

type UserDTO struct {
	ID              string `json:"id"`
	Nickname        string `json:"nickname"`
	Avatar          string `json:"avatar"`
	ProfileComplete bool   `json:"profile_completed"`
}

type UpdateProfileRequest struct {
	Nickname      string `json:"nickname" binding:"required,min=1,max=50"`
	AvatarAssetID string `json:"avatar_asset_id" binding:"required,min=1,max=128"`
}

func NewUserDTO(value model.User, avatar string) UserDTO {
	nickname := ""
	if value.Nickname != nil {
		nickname = strings.TrimSpace(*value.Nickname)
	}
	avatar = strings.TrimSpace(avatar)
	return UserDTO{ID: value.ID, Nickname: nickname, Avatar: avatar, ProfileComplete: nickname != "" && avatar != ""}
}
