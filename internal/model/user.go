package model

import "time"

type User struct {
	ID            string
	OpenID        string
	UnionID       *string
	Nickname      *string
	AvatarAssetID *string
	LastLoginAt   time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
