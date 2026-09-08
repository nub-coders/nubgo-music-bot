package session

import "testing"

func TestNativeSessionPassesThrough(t *testing.T) {
	value := "1BvEeyJrZXkiOiJ4eHgiLCJkY19pZCI6NH0"
	got, err := Normalize(value, 2040)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}
	if got != value {
		t.Fatalf("got %q, want %q", got, value)
	}
}

func TestUnknownSessionFailsClosed(t *testing.T) {
	if _, err := Normalize("not-a-session", 2040); err == nil {
		t.Fatal("expected unsupported session to fail")
	}
}
