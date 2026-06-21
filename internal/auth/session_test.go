package auth

import (
	"testing"
	"time"
)

func TestPasswordAndSessionRoundTrip(t *testing.T) {
	hash, err := HashPassword("a safe password")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "a safe password") || VerifyPassword(hash, "wrong password") {
		t.Fatal("password verification did not behave as expected")
	}
	now := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	token, err := Sign("test-secret", Claims{Subject: "user_1", Username: "alice", Role: "admin", SessionVersion: 1, ExpiresAt: now.Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := Parse("test-secret", token, now)
	if err != nil || claims.Subject != "user_1" || claims.Role != "admin" {
		t.Fatalf("unexpected claims: %#v, %v", claims, err)
	}
}
