package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageRejectsEscapingPath(t *testing.T) {
	storage, err := NewStorage(t.TempDir()); if err != nil { t.Fatal(err) }
	if _, err := storage.Resolve(filepath.Join("..", "outside")); err == nil { t.Fatal("escaping path accepted") }
}

func TestStorageTargetUsesPrivateDirectory(t *testing.T) {
	storage, err := NewStorage(t.TempDir()); if err != nil { t.Fatal(err) }
	relative, temporary, final, err := storage.NewTarget("user-1", "job-1", ".mp4"); if err != nil { t.Fatal(err) }
	if relative == "" || temporary != final+".part" { t.Fatalf("paths = %q %q %q", relative, temporary, final) }
	info, err := os.Stat(filepath.Dir(final)); if err != nil { t.Fatal(err) }; if info.Mode().Perm()&0o077 != 0 { t.Fatalf("directory permissions = %o", info.Mode().Perm()) }
}
