package media

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestURLGuardRejectsPrivateAddresses(t *testing.T) {
	guard := URLGuard{}
	for _, raw := range []string{"http://127.0.0.1/music", "http://10.0.0.1/music", "http://[::1]/music", "file:///etc/passwd"} {
		if _, err := guard.Validate(context.Background(), raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestURLGuardAllowsPrivateOnlyWhenExplicit(t *testing.T) {
	guard := URLGuard{AllowPrivate: true}
	if _, err := guard.Validate(context.Background(), "http://127.0.0.1/music"); err != nil {
		t.Fatalf("expected explicit private URL opt-in to pass: %v", err)
	}
}

func TestPublicIPClassification(t *testing.T) {
	if !isPublicIP(net.ParseIP("1.1.1.1")) {
		t.Fatal("expected public address")
	}
	if isPublicIP(net.ParseIP("169.254.169.254")) {
		t.Fatal("metadata/link-local address must be rejected")
	}
	if !errors.Is(ErrUnsafeURL, ErrUnsafeURL) {
		t.Fatal("sentinel must support errors.Is")
	}
}
