package resolver

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidShareLink = errors.New("invalid share link")
	ErrURLNotAllowed    = errors.New("URL not allowed")
	ErrWorkUnavailable  = errors.New("Douyin work unavailable")
	ErrResolveFailed    = errors.New("Douyin resolve failed")
	ErrVideoRequired    = errors.New("Douyin work does not contain a video")
)

type Image struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	// AnimatedURL 是动图（live photo）的动态版地址，通常为 MP4 片段；
	// 仅动图类图片有值，普通配图为空。
	AnimatedURL string `json:"animatedUrl,omitempty"`
}

type Work struct {
	ID              string         `json:"id,omitempty"`
	DouyinWorkID    string         `json:"douyinWorkId"`
	Type            string         `json:"type"`
	CanonicalURL    string         `json:"canonicalUrl"`
	AuthorID        string         `json:"authorId,omitempty"`
	AuthorName      string         `json:"authorName,omitempty"`
	Title           string         `json:"title"`
	Description     string         `json:"description"`
	CoverURL        string         `json:"coverUrl,omitempty"`
	VideoURL        string         `json:"videoUrl,omitempty"`
	DurationMS      int64          `json:"durationMs,omitempty"`
	Width           int            `json:"width,omitempty"`
	Height          int            `json:"height,omitempty"`
	Images          []Image        `json:"images,omitempty"`
	Hashtags        []string       `json:"hashtags,omitempty"`
	PublishedAt     *time.Time     `json:"publishedAt,omitempty"`
	MusicURL        string         `json:"musicUrl,omitempty"`
	MusicTitle      string         `json:"musicTitle,omitempty"`
	MusicArtist     string         `json:"musicArtist,omitempty"`
	ResolverName    string         `json:"resolverName"`
	ResolverVersion string         `json:"resolverVersion"`
	ResolvedAt      time.Time      `json:"resolvedAt"`
	Metadata        map[string]any `json:"-"`
}

type Adapter interface {
	Resolve(ctx context.Context, shareText string) (Work, error)
}

type Cache interface {
	FindFresh(ctx context.Context, douyinWorkID, resolverVersion string, after time.Time) (Work, error)
	Save(ctx context.Context, work Work) (Work, error)
}

type Fake struct {
	Work  Work
	Err   error
	Calls int
}

func (f *Fake) Resolve(_ context.Context, _ string) (Work, error) {
	f.Calls++
	return f.Work, f.Err
}
