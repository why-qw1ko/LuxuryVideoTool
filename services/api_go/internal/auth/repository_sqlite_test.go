package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
)

func TestSQLiteRepositoryRotatesRefreshSession(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "auth.db"))
	if err != nil { t.Fatal(err) }
	defer db.Close()
	repository := NewSQLiteRepository(db)
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	user := User{ID: "user", UsernameNormalized: "owner", DisplayName: "Owner", PasswordHash: "hash", Role: RoleAdmin, IsActive: true, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateUser(context.Background(), user); err != nil { t.Fatal(err) }
	old := Session{ID: "old", UserID: user.ID, TokenHash: "old-hash", Device: Device{ID: "device", Name: "PC", Platform: "windows", AppVersion: "1"}, ExpiresAt: now.Add(time.Hour), CreatedAt: now, LastUsedAt: now}
	if err := repository.CreateSession(context.Background(), old); err != nil { t.Fatal(err) }
	replacement := old; replacement.ID = "new"; replacement.TokenHash = "new-hash"
	if err := repository.RotateSession(context.Background(), old.ID, replacement, now.Add(time.Minute)); err != nil { t.Fatal(err) }
	rotated, _, err := repository.FindSessionByTokenHash(context.Background(), old.TokenHash)
	if err != nil { t.Fatal(err) }
	if rotated.RevokedAt == nil || rotated.ReplacedBy == nil || *rotated.ReplacedBy != replacement.ID { t.Fatalf("rotated = %#v", rotated) }
}

