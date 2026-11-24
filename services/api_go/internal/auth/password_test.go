package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	password := "测试密碼甲乙"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	valid, err := VerifyPassword(password, hash)
	if err != nil || !valid {
		t.Fatalf("VerifyPassword() = %v, %v", valid, err)
	}
	invalid, err := VerifyPassword("incorrect", hash)
	if err != nil || invalid {
		t.Fatalf("wrong password = %v, %v", invalid, err)
	}
}

func TestNewPasswordLength(t *testing.T) {
	for _, tc := range []struct {
		password string
		valid    bool
	}{
		{"12345", false}, {"123456", true}, {"123456789012", true}, {"1234567890123", false},
		{"测试密碼甲", false}, {"测试密碼甲乙", true}, {"12345😀", true}, {"12345\xff", false},
	} {
		if got := ValidNewPassword(tc.password); got != tc.valid {
			t.Errorf("ValidNewPassword(%q) = %v, want %v", tc.password, got, tc.valid)
		}
		if !tc.valid {
			if _, err := HashPassword(tc.password); err == nil {
				t.Errorf("HashPassword(%q) accepted invalid length or encoding", tc.password)
			}
		}
	}
}

