package auth

import userdto "github.com/balancetheworld/wechat-pet/server/internal/app/user"

type LoginRequest struct {
	Code string `json:"code" binding:"required,min=1,max=512"`
}

type FamilySummaryDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type LoginResponse struct {
	Token    string            `json:"token"`
	User     userdto.UserDTO   `json:"user"`
	Family   *FamilySummaryDTO `json:"family"`
	Identity string            `json:"identity"`
}

type MeResponse struct {
	User     userdto.UserDTO   `json:"user"`
	Family   *FamilySummaryDTO `json:"family"`
	Identity string            `json:"identity"`
}
