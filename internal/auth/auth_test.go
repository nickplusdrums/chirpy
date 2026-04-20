package auth

import (
	"testing"
	"time"
	"github.com/google/uuid"
	"net/http"
	"strings"
)

func TestMakeAndValidateJWT(t *testing.T) {
	newUser := uuid.New()
	token, err := MakeJWT(newUser, "secret", time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT returned error: %v", err)
	}
	validUser, err := ValidateJWT(token, "secret")
	if err != nil {
		t.Fatalf("ValidateJWT returned error: %v", err)
	}
	if validUser != newUser {
		t.Errorf("Expected %v but got %v", newUser, validUser)
	}
	
}

func TestValidateExpiredJWT(t *testing.T) {
	newUser := uuid.New()
	token, err := MakeJWT(newUser, "secret", -time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT errored: %v", err)
	}
	_, err = ValidateJWT(token, "secret")
	if err == nil {
		t.Fatalf("Expected error for expired token, got nil")
	}
}
	
func TestValidateWrongSecret(t *testing.T) {
	newUser := uuid.New()
	token, err := MakeJWT(newUser, "rightsecret", time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT errored: %v", err)
	}
	_, err = ValidateJWT(token, "wrongsecret")
	if err == nil {
		t.Fatalf("Expected error for wrong secret, got nil")
	}
}

func TestGetBearerToken(t *testing.T) {
	header := http.Header{}
	header.Set("Authorization", "Bearer mytoken")

	value, err = GetBearerToken(header)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if value != "mytoken" {
		t.Fatalf("Got %q, want %q", value, "mytoken")
	}
}

func TestGetBearerTokenNoHeader(t *testing.T) {
	header := http.Header{}

	value, err = GetBearer(header)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if value != "" {
		t.Fatalf("Expected empty string, got %v", value)
	}
}



