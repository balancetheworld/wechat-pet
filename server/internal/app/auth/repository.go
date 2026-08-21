package auth

import (
	familyapp "github.com/balancetheworld/wechat-pet/server/internal/app/family"
	userapp "github.com/balancetheworld/wechat-pet/server/internal/app/user"
)

type UserRepository = userapp.Repository

type FamilyRepository = familyapp.Repository
