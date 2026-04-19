package auth

import (
	"github.com/alexedwards/argon2id"
	"errors"
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
