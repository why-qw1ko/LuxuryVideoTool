package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
)

func TestUserAccessChangesPreserveAnActiveAdministrator(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository := NewSQLiteRepository(db)
	now := time.Now().UTC()
	for _, id := range []string{"first", "second"} {
		if err := repository.CreateUser(ctx, User{
			ID: id, UsernameNormalized: id, DisplayName: id, Role: RoleAdmin, IsActive: true, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		results <- repository.DeleteUser(ctx, "first", now, nil)
	}()
	go func() {
		<-start
		results <- repository.SetUserActive(ctx, "second", false, now)
	}()
	close(start)
	var success, conflicts int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			success++
		case errors.Is(err, ErrLastActiveAdmin):
			conflicts++
		default:
			t.Fatalf("unexpected admin access change error: %v", err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("successful removals = %d, conflicts = %d", success, conflicts)
	}
	var activeAdmins int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role = 'admin' AND is_active = 1 AND deleted_at IS NULL").Scan(&activeAdmins); err != nil || activeAdmins != 1 {
		t.Fatalf("active admin count = %d, error = %v", activeAdmins, err)
	}
}
