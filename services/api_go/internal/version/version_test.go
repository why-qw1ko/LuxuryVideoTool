package version

import "testing"

func TestCurrent(t *testing.T) {
	got := Current()
	if got.Version == "" || got.Commit == "" || got.BuildTime == "" {
		t.Fatalf("Current() = %#v, want non-empty fields", got)
	}
}

