package asr

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
)

func TestConfiguredBudgetUsesBilledCost(t *testing.T) {
	for _, tc := range []struct {
		name   string
		budget Budget
	}{
		{name: "daily", budget: Budget{DailyCNY: 2, PricePerMinuteCNY: 2}},
		{name: "monthly", budget: Budget{MonthlyCNY: 2, PricePerMinuteCNY: 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := database.Open(ctx, filepath.Join(t.TempDir(), "budget.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			_, err = db.ExecContext(ctx, `INSERT INTO users (id, username_normalized, display_name, password_hash, role, created_at, updated_at)
				VALUES ('user','user','User','test','user',0,0);
				INSERT INTO jobs (id,user_id,input_text,input_url,action,status,progress,idempotency_key,created_at,updated_at)
				VALUES ('job','user','','','full','transcribing',0,'key',0,0)`)
			if err != nil {
				t.Fatal(err)
			}
			provider := &Fake{Result: TranscribeResult{RawText: "text", BilledSeconds: 30}}
			service := NewService(provider, nil, NewRepository(db), tc.budget)
			service.now = func() time.Time { return time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC) }
			verify := func(duration time.Duration, want error, calls int) {
				t.Helper()
				_, err := service.Transcribe(ctx, "job", "audio", "", duration, nil, nil)
				if !errors.Is(err, want) || provider.Calls != calls {
					t.Fatalf("transcribe error=%v, want %v, provider calls=%d, want %d", err, want, provider.Calls, calls)
				}
			}
			verify(90*time.Second, ErrBudgetExceeded, 0)
			verify(time.Minute, nil, 1)
			verify(30*time.Second, nil, 2)
			verify(30*time.Second, ErrBudgetExceeded, 2)
		})
	}
}
