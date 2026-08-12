package resolver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
)

type SQLiteCache struct{ db *sql.DB }

func NewSQLiteCache(db *sql.DB) *SQLiteCache { return &SQLiteCache{db: db} }

func (c *SQLiteCache) FindFresh(ctx context.Context, douyinWorkID, version string, after time.Time) (Work, error) {
	row := c.db.QueryRowContext(ctx, `SELECT id, douyin_work_id, content_type, canonical_url, author_id,
		author_name, title, description, cover_url, published_at, metadata_json, resolver_name,
		resolver_version, resolved_at FROM works WHERE douyin_work_id = ? AND resolver_version = ? AND resolved_at >= ?`,
		douyinWorkID, version, after.UTC().UnixMilli())
	return scanWork(row)
}

func (c *SQLiteCache) Save(ctx context.Context, work Work) (Work, error) {
	now := work.ResolvedAt.UTC()
	if now.IsZero() { now = time.Now().UTC(); work.ResolvedAt = now }
	metadata := work.Metadata
	if metadata == nil { metadata = map[string]any{} }
	metadata["videoUrl"], metadata["durationMs"], metadata["width"], metadata["height"] = work.VideoURL, work.DurationMS, work.Width, work.Height
	metadata["images"], metadata["hashtags"] = work.Images, work.Hashtags
	encoded, err := json.Marshal(metadata)
	if err != nil { return Work{}, fmt.Errorf("encode work metadata: %w", err) }
	if work.ID == "" { work.ID, err = auth.NewID(now); if err != nil { return Work{}, err } }
	var published any
	if work.PublishedAt != nil { published = work.PublishedAt.UTC().UnixMilli() }
	_, err = c.db.ExecContext(ctx, `INSERT INTO works(id, douyin_work_id, content_type, canonical_url, author_id,
		author_name, title, description, cover_url, published_at, metadata_json, resolver_name, resolver_version,
		resolved_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(douyin_work_id) DO UPDATE SET content_type=excluded.content_type, canonical_url=excluded.canonical_url,
		author_id=excluded.author_id, author_name=excluded.author_name, title=excluded.title, description=excluded.description,
		cover_url=excluded.cover_url, published_at=excluded.published_at, metadata_json=excluded.metadata_json,
		resolver_name=excluded.resolver_name, resolver_version=excluded.resolver_version, resolved_at=excluded.resolved_at,
		updated_at=excluded.updated_at`, work.ID, work.DouyinWorkID, work.Type, work.CanonicalURL, nullString(work.AuthorID),
		nullString(work.AuthorName), work.Title, work.Description, nullString(work.CoverURL), published, string(encoded),
		work.ResolverName, work.ResolverVersion, now.UnixMilli(), now.UnixMilli(), now.UnixMilli())
	if err != nil { return Work{}, fmt.Errorf("save work: %w", err) }
	return c.findByID(ctx, work.DouyinWorkID, work.ResolverVersion)
}

func (c *SQLiteCache) findByID(ctx context.Context, douyinWorkID, version string) (Work, error) {
	row := c.db.QueryRowContext(ctx, `SELECT id, douyin_work_id, content_type, canonical_url, author_id,
		author_name, title, description, cover_url, published_at, metadata_json, resolver_name,
		resolver_version, resolved_at FROM works WHERE douyin_work_id = ? AND resolver_version = ?`, douyinWorkID, version)
	return scanWork(row)
}

type rowScanner interface{ Scan(...any) error }
func scanWork(row rowScanner) (Work, error) {
	var work Work
	var authorID, authorName, cover sql.NullString
	var published sql.NullInt64
	var metadata string
	var resolved int64
	err := row.Scan(&work.ID, &work.DouyinWorkID, &work.Type, &work.CanonicalURL, &authorID, &authorName,
		&work.Title, &work.Description, &cover, &published, &metadata, &work.ResolverName, &work.ResolverVersion, &resolved)
	if errors.Is(err, sql.ErrNoRows) { return Work{}, ErrCacheMiss }
	if err != nil { return Work{}, fmt.Errorf("scan work: %w", err) }
	work.AuthorID, work.AuthorName, work.CoverURL, work.ResolvedAt = authorID.String, authorName.String, cover.String, time.UnixMilli(resolved).UTC()
	if published.Valid { value := time.UnixMilli(published.Int64).UTC(); work.PublishedAt = &value }
	if json.Unmarshal([]byte(metadata), &work.Metadata) == nil {
		work.VideoURL, _ = work.Metadata["videoUrl"].(string)
		work.DurationMS = int64(number(work.Metadata["durationMs"]))
		work.Width, work.Height = int(number(work.Metadata["width"])), int(number(work.Metadata["height"]))
		remarshal(work.Metadata["images"], &work.Images); remarshal(work.Metadata["hashtags"], &work.Hashtags)
	}
	return work, nil
}
func remarshal(value any, target any) { body, _ := json.Marshal(value); _ = json.Unmarshal(body, target) }
func number(value any) float64 { if n, ok := value.(float64); ok { return n }; return 0 }
func nullString(value string) any { if value == "" { return nil }; return value }
