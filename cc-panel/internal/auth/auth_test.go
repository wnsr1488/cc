package auth

import (
	"testing"
	"time"
)

func TestIssueAndParseToken(t *testing.T) {
	service := NewService(nil, "12345678901234567890123456789012", time.Hour)
	user := User{ID: 42, Username: "admin", Role: "super_admin"}

	token, err := service.IssueToken(user)
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}

	claims, err := service.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if claims.UserID != user.ID || claims.Username != user.Username || claims.Role != user.Role {
		t.Fatalf("claims = %+v, want user %+v", claims, user)
	}
}
