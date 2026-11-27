package releases

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxAPKSize int64 = 256 << 20

var (
	ErrInvalidAPK = errors.New("invalid APK or package identity")
	ErrTooLarge   = errors.New("APK exceeds size limit")
	ErrInput      = errors.New("invalid release metadata")
	ErrConflict   = errors.New("release changed, reload before editing")
	ErrNotFound   = errors.New("release not found")
	ErrDowngrade  = errors.New("release version must increase")
	ErrBusy       = errors.New("another APK is uploading")
)

type Release struct {
	ID          string `json:"id"`
	VersionCode int    `json:"versionCode"`
	VersionName string `json:"versionName"`
	Changelog   string `json:"changelog"`
	ForceUpdate bool   `json:"forceUpdate"`
	PackageName string `json:"packageName"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	DownloadURL string `json:"downloadUrl"`
	PublishedAt string `json:"publishedAt"`
}

type State struct {
	Draft   *Release `json:"draft"`
	Release *Release `json:"release"`
}

type DraftInput struct {
	DraftID     string `json:"draftId"`
	VersionCode int    `json:"versionCode"`
	VersionName string `json:"versionName"`
	Changelog   string `json:"changelog"`
	ForceUpdate bool   `json:"forceUpdate"`
}

type Service struct {
	db        *sql.DB
	directory string
	uploading chan struct{}
}

func New(db *sql.DB, dataDir string) (*Service, error) {
	if db == nil || strings.TrimSpace(dataDir) == "" {
		return nil, errors.New("invalid Android release storage")
	}
	directory, err := filepath.Abs(filepath.Join(dataDir, "releases", "android"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create APK directory: %w", err)
	}
	return &Service{db: db, directory: directory, uploading: make(chan struct{}, 1)}, nil
}

type stateReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readState(ctx context.Context, reader stateReader) (State, error) {
	var payload string
	if err := reader.QueryRowContext(ctx, "SELECT payload FROM android_release_state WHERE id=1").Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return State{}, nil
		}
		return State{}, err
	}
	var state State
	if err := json.Unmarshal([]byte(payload), &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *Service) State(ctx context.Context) (State, error) {
	return readState(ctx, s.db)
}

func (s *Service) mutate(ctx context.Context, change func(*sql.Tx, *State) error) (State, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	state, err := readState(ctx, tx)
	if err != nil {
		return State{}, err
	}
	if err := change(tx, &state); err != nil {
		return State{}, err
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return State{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO android_release_state(id,payload) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", string(payload)); err != nil {
		return State{}, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *Service) Upload(ctx context.Context, source io.Reader) (State, error) {
	select {
	case s.uploading <- struct{}{}:
		defer func() { <-s.uploading }()
	default:
		return State{}, ErrBusy
	}
	file, err := os.CreateTemp(s.directory, ".upload-*.part")
	if err != nil {
		return State{}, err
	}
	temporary := file.Name()
	defer func() { file.Close(); os.Remove(temporary) }()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(contextReader{ctx, source}, MaxAPKSize+1))
	if size > MaxAPKSize {
		return State{}, ErrTooLarge
	}
	if err != nil {
		return State{}, err
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	if err := file.Sync(); err != nil {
		return State{}, err
	}
	if err := file.Close(); err != nil {
		return State{}, err
	}
	identity, err := inspectAPK(temporary)
	if err != nil {
		return State{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return State{}, err
	}
	id := hex.EncodeToString(random[:])
	release := Release{ID: id, VersionCode: identity.VersionCode, VersionName: identity.VersionName,
		PackageName: identity.PackageName, Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)),
		DownloadURL: "/api/v1/app/android/releases/" + id + "/apk"}
	final := s.apkPath(id)
	if err := os.Rename(temporary, final); err != nil {
		return State{}, err
	}
	var obsolete string
	state, err := s.mutate(ctx, func(tx *sql.Tx, state *State) error {
		payload, err := json.Marshal(release)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO android_release_apks(id,metadata) VALUES(?,?)", id, string(payload)); err != nil {
			return err
		}
		if state.Draft != nil {
			result, err := tx.ExecContext(ctx, "DELETE FROM android_release_apks WHERE id=? AND published_at IS NULL", state.Draft.ID)
			if err != nil {
				return err
			}
			if count, err := result.RowsAffected(); err != nil {
				return err
			} else if count == 1 {
				obsolete = state.Draft.ID
			}
		}
		state.Draft = &release
		return nil
	})
	if err != nil {
		os.Remove(final)
		return State{}, err
	}
	if obsolete != "" {
		if err := os.Remove(s.apkPath(obsolete)); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Error("remove replaced APK draft", "release_id", obsolete)
		}
	}
	return state, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

func (s *Service) SaveDraft(ctx context.Context, input DraftInput) (State, error) {
	if input.DraftID == "" || utf8.RuneCountInString(input.Changelog) > 10000 || !utf8.ValidString(input.Changelog) {
		return State{}, ErrInput
	}
	return s.mutate(ctx, func(_ *sql.Tx, state *State) error {
		if state.Draft == nil || state.Draft.ID != input.DraftID {
			return ErrConflict
		}
		if input.VersionCode != state.Draft.VersionCode || input.VersionName != state.Draft.VersionName {
			return ErrInput
		}
		state.Draft.Changelog = strings.TrimSpace(input.Changelog)
		state.Draft.ForceUpdate = input.ForceUpdate
		return nil
	})
}

func (s *Service) Publish(ctx context.Context, draftID string) (State, error) {
	if draftID == "" {
		return State{}, ErrInput
	}
	return s.mutate(ctx, func(tx *sql.Tx, state *State) error {
		if state.Draft == nil || state.Draft.ID != draftID {
			return ErrConflict
		}
		draft := *state.Draft
		if state.Release != nil && (draft.VersionCode < state.Release.VersionCode || (draft.VersionCode == state.Release.VersionCode && draft.SHA256 != state.Release.SHA256)) {
			return ErrDowngrade
		}
		file, err := os.Open(s.apkPath(draft.ID))
		if err != nil {
			return ErrNotFound
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.Size() != draft.Size {
			return ErrInvalidAPK
		}
		draft.PublishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, "UPDATE android_release_apks SET published_at=? WHERE id=?", time.Now().UTC().UnixMilli(), draft.ID)
		if err != nil {
			return err
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			return ErrNotFound
		}
		state.Release = &draft
		draftCopy := draft
		state.Draft = &draftCopy
		return nil
	})
}

func (s *Service) SetPolicy(ctx context.Context, releaseID string, force bool) (State, error) {
	if releaseID == "" {
		return State{}, ErrInput
	}
	return s.mutate(ctx, func(_ *sql.Tx, state *State) error {
		if state.Release == nil || state.Release.ID != releaseID {
			return ErrConflict
		}
		state.Release.ForceUpdate = force
		return nil
	})
}

func (s *Service) apkPath(id string) string {
	return filepath.Join(s.directory, id+".apk")
}

func (s *Service) OpenPublished(ctx context.Context, id string) (*os.File, Release, error) {
	if len(id) != 32 {
		return nil, Release{}, ErrNotFound
	}
	if _, err := hex.DecodeString(id); err != nil {
		return nil, Release{}, ErrNotFound
	}
	var payload string
	if err := s.db.QueryRowContext(ctx, "SELECT metadata FROM android_release_apks WHERE id=? AND published_at IS NOT NULL", id).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, Release{}, ErrNotFound
		}
		return nil, Release{}, err
	}
	var release Release
	if err := json.Unmarshal([]byte(payload), &release); err != nil {
		return nil, Release{}, err
	}
	file, err := os.Open(s.apkPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, Release{}, ErrNotFound
	}
	return file, release, err
}
