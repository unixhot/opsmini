package jwt

import (
	"testing"
	"time"
)

func TestGenerateAndParse(t *testing.T) {
	m := NewManager("test-secret", time.Minute, time.Hour)
	access, refresh, err := m.Generate(42, "admin")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if access == "" || refresh == "" {
		t.Fatal("empty tokens")
	}

	claims, err := m.Parse(access)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != 42 || claims.Role != "admin" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseInvalid(t *testing.T) {
	m := NewManager("test-secret", time.Minute, time.Hour)
	if _, err := m.Parse("invalid-token"); err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestParseWrongSecret(t *testing.T) {
	m1 := NewManager("secret-a", time.Minute, time.Hour)
	m2 := NewManager("secret-b", time.Minute, time.Hour)
	access, _, _ := m1.Generate(1, "admin")
	if _, err := m2.Parse(access); err == nil {
		t.Fatal("expected error when parsing with wrong secret")
	}
}
