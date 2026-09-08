package media

import (
	"testing"
	"time"
)

func TestParseISODuration(t *testing.T) {
	if got := parseISODuration("PT1H2M3S"); got != time.Hour+2*time.Minute+3*time.Second {
		t.Fatalf("unexpected duration: %v", got)
	}
	if got := parseISODuration("PT3M33S"); got != 213*time.Second {
		t.Fatalf("unexpected duration: %v", got)
	}
}
