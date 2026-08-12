package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
)

type Recorder struct {
	db  *sql.DB
	key []byte
	now func() time.Time
}

type Event struct {
	ActorUserID string
	Action      string
	TargetType  string
	TargetID    string
	RequestID   string
	IP          string
	Metadata    map[string]any
}

func New(db *sql.DB, key []byte) *Recorder {
	return &Recorder{db: db, key: append([]byte(nil), key...), now: time.Now}
}

func (r *Recorder) Record(ctx context.Context, event Event) error {
	id, err := auth.NewID(r.now())
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	var actor, target any
	if event.ActorUserID != "" {
		actor = event.ActorUserID
	}
	if event.TargetID != "" {
		target = event.TargetID
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO audit_logs(
		id, actor_user_id, action, target_type, target_id, request_id, ip_hash, metadata_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, actor, event.Action, event.TargetType, target,
		event.RequestID, r.hashIP(event.IP), string(metadata), r.now().UTC().UnixMilli())
	if err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

func (r *Recorder) hashIP(value string) string {
	if value == "" {
		return ""
	}
	mac := hmac.New(sha256.New, r.key)
	_, _ = mac.Write([]byte("audit-ip:" + value))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

