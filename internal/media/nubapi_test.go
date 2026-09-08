package media

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseNUBDuration(t *testing.T) {
	cases := []struct {
		raw      string
		expected time.Duration
	}{
		{`213`, 213 * time.Second},
		{`"213"`, 213 * time.Second},
		{`"03:33"`, 213 * time.Second},
		{`"1:02:03"`, 3723 * time.Second},
	}
	for _, test := range cases {
		if got := parseNUBDuration(json.RawMessage(test.raw)); got != test.expected {
			t.Errorf("parseNUBDuration(%s) = %v, want %v", test.raw, got, test.expected)
		}
	}
}
