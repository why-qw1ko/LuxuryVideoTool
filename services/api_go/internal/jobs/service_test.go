package jobs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/resolver"
)

func TestCreateInfoAndReuseIdempotencyKey(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db")); if err != nil { t.Fatal(err) }; defer db.Close()
	users := auth.NewSQLiteRepository(db)
	now := time.Now().UTC()
	if err := users.CreateUser(context.Background(), auth.User{ID: "user-1", UsernameNormalized: "user", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}); err != nil { t.Fatal(err) }
	fake := &resolver.Fake{Work: resolver.Work{DouyinWorkID: "123", Type: "video", CanonicalURL: "https://www.douyin.com/video/123", Title: "标题", ResolverName: "fake", ResolverVersion: resolver.DouyinResolverVersion, ResolvedAt: now}}
	service := NewService(NewSQLiteRepository(db), resolver.NewService(fake, resolver.NewSQLiteCache(db), 6*time.Hour, resolver.DouyinResolverVersion))
	input := CreateInput{UserID: "user-1", ShareText: "https://www.douyin.com/video/123", IdempotencyKey: "key-1"}
	job, reused, err := service.CreateInfo(context.Background(), input)
	if err != nil || reused || job.Status != "completed" || job.Work == nil { t.Fatalf("job=%#v reused=%v err=%v", job, reused, err) }
	second, reused, err := service.CreateInfo(context.Background(), input)
	if err != nil || !reused || second.ID != job.ID || fake.Calls != 1 { t.Fatalf("second=%#v reused=%v calls=%d err=%v", second, reused, fake.Calls, err) }
}
