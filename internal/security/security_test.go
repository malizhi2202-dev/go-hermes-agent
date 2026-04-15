package security

import (
	"testing"
	"time"
)

func TestPasswordHashAndCheck(t *testing.T) {
	hash, err := HashPassword("ChangeMe123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := CheckPassword(hash, "ChangeMe123!"); err != nil {
		t.Fatalf("password should match: %v", err)
	}
	if err := CheckPassword(hash, "wrong-password"); err == nil {
		t.Fatal("expected password mismatch")
	}
}

func TestJWTSignAndParse(t *testing.T) {
	token, err := SignJWT([]byte("secret"), "hermes-go", "admin", "admin", time.Hour)
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	claims, err := ParseJWT([]byte("secret"), token)
	if err != nil {
		t.Fatalf("parse jwt: %v", err)
	}
	if claims.Username != "admin" {
		t.Fatalf("unexpected username: %s", claims.Username)
	}
}
