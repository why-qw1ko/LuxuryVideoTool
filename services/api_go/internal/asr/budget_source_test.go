package asr

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
)

func TestRuntimeBudgetReadsCurrentLimitsAndPrice(t *testing.T) {
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
	service := NewService(provider, nil, NewRepository(db), Budget{})
	budget := Budget{DailyCNY: 1, MonthlyCNY: 10, PricePerMinuteCNY: 2}
	service.SetBudgetSource(func(context.Context) (Budget, error) { return budget, nil })
	call := func(want error, calls int) {
		t.Helper()
		_, err := service.Transcribe(ctx, "job", "audio", "", time.Minute, nil, nil)
		if !errors.Is(err, want) || provider.Calls != calls {
			t.Fatalf("transcribe error=%v, want %v, provider calls=%d, want %d", err, want, provider.Calls, calls)
		}
	}
	call(ErrBudgetExceeded, 0)
	budget.DailyCNY = 2
	call(nil, 1)
	budget.DailyCNY = 0
	budget.MonthlyCNY = 1.25
	budget.PricePerMinuteCNY = 0.25
	call(nil, 2)
	budget.PricePerMinuteCNY = 0.2
	call(ErrBudgetExceeded, 2)
}
