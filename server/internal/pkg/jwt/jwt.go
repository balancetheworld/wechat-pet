package jwt

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt"
)

type Signer struct {
	secret []byte
	expiry time.Duration
}

type Claims struct {
	UserID    string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

func NewSigner(secret string, expiry time.Duration) (*Signer, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("jwt secret is required")
	}
	if expiry == 0 {
		expiry = 2 * time.Hour
	}
	if expiry < 0 {
		return nil, errors.New("jwt expiry must be positive")
	}
	return &Signer{secret: []byte(secret), expiry: expiry}, nil
}

func (s *Signer) Issue(userID string) (string, error) {
	return s.Sign(userID)
}

func (s *Signer) Sign(userID string) (string, error) {
	if strings.TrimSpace(userID) == "" {
		return "", errors.New("user id is required")
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": now.Unix(),
		"exp": now.Add(s.expiry).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *Signer) Verify(tokenString string) (Claims, error) {
	if strings.TrimSpace(tokenString) == "" {
		return Claims{}, errors.New("token is required")
	}
	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return Claims{}, err
	}
	if !token.Valid {
		return Claims{}, errors.New("invalid token")
	}
	values, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, errors.New("invalid claims")
	}
	userID, ok := values["sub"].(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return Claims{}, errors.New("invalid subject")
	}
	issuedAt, err := claimTime(values["iat"])
	if err != nil {
		return Claims{}, err
	}
	expiresAt, err := claimTime(values["exp"])
	if err != nil {
		return Claims{}, err
	}
	return Claims{UserID: userID, IssuedAt: issuedAt, ExpiresAt: expiresAt}, nil
}

func claimTime(value interface{}) (time.Time, error) {
	var seconds int64
	switch value := value.(type) {
	case float64:
		seconds = int64(value)
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return time.Time{}, errors.New("invalid timestamp")
		}
		seconds = parsed
	default:
		return time.Time{}, errors.New("invalid timestamp")
	}
	return time.Unix(seconds, 0), nil
}
