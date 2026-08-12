package files

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
)

var ErrNotFound = errors.New("file not found")

type File struct {
	ID string `json:"id"`
	UserID string `json:"-"`
	JobID string `json:"jobId"`
	Kind string `json:"kind"`
	RelativePath string `json:"-"`
	OriginalName string `json:"name"`
	MIMEType string `json:"mimeType"`
	SizeBytes int64 `json:"sizeBytes"`
	SHA256 string `json:"sha256"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Repository struct{ db *sql.DB }
func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, file File) error {
	var expires any; if file.ExpiresAt != nil { expires = file.ExpiresAt.UTC().UnixMilli() }
	_, err := r.db.ExecContext(ctx, `INSERT INTO files(id, user_id, job_id, kind, relative_path, original_name,
		mime_type, size_bytes, sha256, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		file.ID, file.UserID, file.JobID, file.Kind, file.RelativePath, file.OriginalName, file.MIMEType,
		file.SizeBytes, file.SHA256, expires, file.CreatedAt.UTC().UnixMilli())
	if err != nil { return fmt.Errorf("register file: %w", err) }; return nil
}

func (r *Repository) FindOwned(ctx context.Context, userID, fileID string) (File, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, user_id, job_id, kind, relative_path, original_name, mime_type,
		size_bytes, sha256, expires_at, created_at FROM files WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, fileID, userID)
	return scanFile(row)
}
func (r *Repository) FindByID(ctx context.Context, fileID string) (File, error) { row := r.db.QueryRowContext(ctx, `SELECT id, user_id, job_id, kind, relative_path, original_name, mime_type, size_bytes, sha256, expires_at, created_at FROM files WHERE id = ? AND deleted_at IS NULL`, fileID); return scanFile(row) }

func (r *Repository) ListByJob(ctx context.Context, userID, jobID string) ([]File, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, user_id, job_id, kind, relative_path, original_name, mime_type,
		size_bytes, sha256, expires_at, created_at FROM files WHERE job_id = ? AND user_id = ? AND deleted_at IS NULL ORDER BY created_at, id`, jobID, userID)
	if err != nil { return nil, fmt.Errorf("list job files: %w", err) }; defer rows.Close()
	var result []File; for rows.Next() { item, err := scanFile(rows); if err != nil { return nil, err }; result = append(result, item) }; return result, rows.Err()
}

func (r *Repository) Expired(ctx context.Context, now time.Time, limit int) ([]File, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, user_id, job_id, kind, relative_path, original_name, mime_type,
		size_bytes, sha256, expires_at, created_at FROM files WHERE expires_at <= ? AND deleted_at IS NULL ORDER BY expires_at LIMIT ?`, now.UTC().UnixMilli(), limit)
	if err != nil { return nil, err }; defer rows.Close(); var result []File
	for rows.Next() { file, err := scanFile(rows); if err != nil { return nil, err }; result = append(result, file) }; return result, rows.Err()
}

func (r *Repository) MarkDeleted(ctx context.Context, fileID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE files SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, at.UTC().UnixMilli(), fileID); return err
}

type scanner interface{ Scan(...any) error }
func scanFile(row scanner) (File, error) {
	var file File; var expires sql.NullInt64; var created int64
	err := row.Scan(&file.ID, &file.UserID, &file.JobID, &file.Kind, &file.RelativePath, &file.OriginalName,
		&file.MIMEType, &file.SizeBytes, &file.SHA256, &expires, &created)
	if errors.Is(err, sql.ErrNoRows) { return File{}, ErrNotFound }; if err != nil { return File{}, fmt.Errorf("scan file: %w", err) }
	file.CreatedAt = time.UnixMilli(created).UTC(); if expires.Valid { value := time.UnixMilli(expires.Int64).UTC(); file.ExpiresAt = &value }; return file, nil
}

type Storage struct{ root string }
func NewStorage(root string) (*Storage, error) {
	abs, err := filepath.Abs(root); if err != nil { return nil, err }
	if err := os.MkdirAll(abs, 0o700); err != nil { return nil, fmt.Errorf("create data root: %w", err) }
	return &Storage{root: filepath.Clean(abs)}, nil
}

func (s *Storage) NewTarget(userID, jobID, extension string) (relative, temporary, final string, err error) {
	if !safeSegment(userID) || !safeSegment(jobID) { return "", "", "", errors.New("unsafe storage segment") }
	if extension == "" || len(extension) > 10 || strings.ContainsAny(extension, `/\\`) { extension = ".bin" }
	relative = filepath.Join("media", userID, jobID, randomName()+extension)
	final, err = s.Resolve(relative); if err != nil { return "", "", "", err }
	if err = os.MkdirAll(filepath.Dir(final), 0o700); err != nil { return "", "", "", err }
	temporary = final + ".part"; return relative, temporary, final, nil
}
func (s *Storage) NewScopedTarget(scope, userID, jobID, extension string) (relative, temporary, final string, err error) { if !safeSegment(scope) || !safeSegment(userID) || !safeSegment(jobID) || extension == "" || len(extension) > 10 || strings.ContainsAny(extension, `/\\`) { return "", "", "", errors.New("unsafe storage target") }; relative = filepath.Join(scope, userID, jobID, randomName()+extension); final, err = s.Resolve(relative); if err != nil { return "", "", "", err }; if err = os.MkdirAll(filepath.Dir(final), 0o700); err != nil { return "", "", "", err }; return relative, final+".part", final, nil }
func (s *Storage) WriteAtomic(temporary, final string, body []byte) error { file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600); if err != nil { return err }; keep := false; defer func() { file.Close(); if !keep { os.Remove(temporary) } }(); if _, err := file.Write(body); err != nil { return err }; if err := file.Sync(); err != nil { return err }; if err := file.Close(); err != nil { return err }; if err := os.Rename(temporary, final); err != nil { return err }; keep = true; return nil }

func (s *Storage) Resolve(relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) { return "", errors.New("unsafe relative path") }
	target := filepath.Clean(filepath.Join(s.root, relative)); relation, err := filepath.Rel(s.root, target)
	if err != nil || relation == "." || relation == ".." || strings.HasPrefix(relation, ".."+string(os.PathSeparator)) || filepath.IsAbs(relation) { return "", errors.New("path escapes data root") }; return target, nil
}
func (s *Storage) Relative(path string) (string, error) { absolute, err := filepath.Abs(path); if err != nil { return "", err }; relative, err := filepath.Rel(s.root, absolute); if err != nil { return "", err }; if _, err := s.Resolve(relative); err != nil { return "", err }; return relative, nil }

func (s *Storage) Open(file File) (*os.File, error) { path, err := s.Resolve(file.RelativePath); if err != nil { return nil, err }; return os.Open(path) }
func (s *Storage) Remove(file File) error { path, err := s.Resolve(file.RelativePath); if err != nil { return err }; err = os.Remove(path); if errors.Is(err, os.ErrNotExist) { return nil }; return err }

func NewFile(now time.Time, userID, jobID, kind, relative, name, mime, sha string, size int64, expires *time.Time) (File, error) {
	id, err := auth.NewID(now); if err != nil { return File{}, err }
	return File{ID: id, UserID: userID, JobID: jobID, Kind: kind, RelativePath: relative, OriginalName: name,
		MIMEType: mime, SizeBytes: size, SHA256: sha, ExpiresAt: expires, CreatedAt: now.UTC()}, nil
}

func safeSegment(value string) bool { if value == "" { return false }; for _, r := range value { if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') { return false } }; return true }
func randomName() string { var body [8]byte; if _, err := io.ReadFull(rand.Reader, body[:]); err != nil { return fmt.Sprintf("%d", time.Now().UnixNano()) }; return fmt.Sprintf("%x", body[:]) }
