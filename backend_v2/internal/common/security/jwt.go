package security

import (
	"errors"
	"time"
	"tle_zone_v2/internal/platform/config"

	"github.com/go-chi/jwtauth/v5"
	"github.com/golang-jwt/jwt/v5"
)

var TokenAuth *jwtauth.JWTAuth

func InitJWT() {
	TokenAuth = jwtauth.New("HS256", config.AppConfig.JWTKey, nil)
}

func GenerateToken(userID, role string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"exp":     time.Now().Add(config.AppConfig.JWTExp).Unix(),
		"iat":     time.Now().Unix(),
	}
	_, tokenString, err := TokenAuth.Encode(claims)
	return tokenString, err
}

// Helper functions to extract claims, can be used in middleware or services
func GetUserIDFromClaims(claims jwt.MapClaims) (string, error) {
	id, ok := claims["user_id"].(string)
	if !ok {
		return "", errors.New("user_id claim is missing or not a string")
	}
	return id, nil
}

func GetUserRoleFromClaims(claims jwt.MapClaims) (string, error) {
	role, ok := claims["role"].(string)
	if !ok {
		return "", errors.New("role claim is missing or not a string")
	}
	return role, nil
}
