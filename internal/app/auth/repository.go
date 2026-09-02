package auth

import (
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	userapp "github.com/balancetheworld/wechat-pet/internal/app/user"
)

type UserRepository = userapp.Repository

type FamilyRepository = familyapp.ActiveFamilyRepository
