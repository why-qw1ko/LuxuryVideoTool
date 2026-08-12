package jobs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
)

func TestQueueClaimIsExclusiveAndRecoveryRequeues(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "queue.db")); if err != nil { t.Fatal(err) }; defer db.Close()
	now := time.Now().UTC(); users := auth.NewSQLiteRepository(db)
	if err := users.CreateUser(context.Background(), auth.User{ID: "user-1", UsernameNormalized: "user", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}); err != nil { t.Fatal(err) }
	repo := NewSQLiteRepository(db)
	for index, id := range []string{"job-1", "job-2"} { if err := repo.CreateQueued(context.Background(), Job{ID: id, UserID: "user-1", InputText: "https://www.douyin.com/video/123", InputURL: "https://www.douyin.com/video/123", Action: "download", Status: "queued", IdempotencyKey: id, CreatedAt: now.Add(time.Duration(index)*time.Millisecond), UpdatedAt: now}); err != nil { t.Fatal(err) } }
	first, err := repo.ClaimNext(context.Background(), "worker-1", now, time.Minute); if err != nil { t.Fatal(err) }
	second, err := repo.ClaimNext(context.Background(), "worker-2", now, time.Minute); if err != nil { t.Fatal(err) }
	if first.ID == second.ID { t.Fatalf("same job claimed twice: %s", first.ID) }
	if _, err := repo.Recover(context.Background(), now.Add(2*time.Minute)); err != nil { t.Fatal(err) }
	recovered, err := repo.ClaimNext(context.Background(), "worker-3", now.Add(2*time.Minute), time.Minute); if err != nil { t.Fatal(err) }
	if recovered.ID != first.ID && recovered.ID != second.ID { t.Fatalf("unexpected recovered job: %s", recovered.ID) }
}

func TestQueuedJobCanBeCancelled(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "cancel.db")); if err != nil { t.Fatal(err) }; defer db.Close()
	now := time.Now().UTC(); users := auth.NewSQLiteRepository(db)
	if err := users.CreateUser(context.Background(), auth.User{ID: "user-1", UsernameNormalized: "user", DisplayName: "User", PasswordHash: "hash", Role: auth.RoleUser, IsActive: true, CreatedAt: now, UpdatedAt: now}); err != nil { t.Fatal(err) }
	repo := NewSQLiteRepository(db); job := Job{ID: "job-1", UserID: "user-1", InputText: "x", InputURL: "https://www.douyin.com/video/1", Action: "download", Status: "queued", IdempotencyKey: "key", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateQueued(context.Background(), job); err != nil { t.Fatal(err) }
	if _, err := repo.Cancel(context.Background(), "user-1", job.ID, now); err != nil { t.Fatal(err) }
	cancelled, err := repo.FindByID(context.Background(), "user-1", job.ID); if err != nil || cancelled.Status != "cancelled" { t.Fatalf("job=%#v err=%v", cancelled, err) }
}
