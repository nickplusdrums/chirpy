package auth

import (
	"github.com/alexedwards/argon2id"
	"errors"
	"time"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"crypto/rand"
	"encoding/hex"
)

func HashPassword(password string) (string, error) {
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", errors.New("Failed to Hash password")
	}
	return hash, nil
}

func CheckPasswordHash(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, errors.New("Failed to Call Compare Password and Hash")
	}
	return match, nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {

	claims := jwt.RegisteredClaims{
		Issuer: "chirpy-access",
		IssuedAt:	jwt.NewNumericDate(time.Now()),
		ExpiresAt:	jwt.NewNumericDate(time.Now().Add(expiresIn)),
		Subject:	userID.String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(tokenSecret))
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claimsStruct := &jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString, 
		claimsStruct, 
		func(token *jwt.Token) (interface{}, error) {
			return []byte(tokenSecret), nil
		},
	)
	if err != nil {
		return uuid.Nil, err
	}
	subject, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.Nil, err
	}
	userID, err := uuid.Parse(subject)
	if err != nil {
		return uuid.Nil, err
	}
	return userID, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	
	value := headers.Get("Authorization")
	if value == "" {
		return "", errors.New("NO Authorization Header")
	}
	value = strings.TrimPrefix(value, "Bearer ")
	value = strings.TrimSpace(value)
	return value, nil
}

func MakeRefreshToken() string {
	random := rand.Read
	random = hex.EncodeToString(random)
	return random	
}
