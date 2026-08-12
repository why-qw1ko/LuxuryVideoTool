package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil { t.Fatalf("HashPassword() error = %v", err) }
	valid, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !valid { t.Fatalf("VerifyPassword() = %v, %v", valid, err) }
	invalid, err := VerifyPassword("incorrect password", hash)
	if err != nil || invalid { t.Fatalf("wrong password = %v, %v", invalid, err) }
}

func TestPasswordRejectsShortValue(t *testing.T) {
	if _, err := HashPassword("too-short"); err == nil { t.Fatal("HashPassword() error = nil") }
}

