package media

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

var ErrUnsafeURL = errors.New("URL resolves to a non-public network address")

type URLGuard struct {
	AllowPrivate bool
	Resolver     *net.Resolver
}

func (g URLGuard) Validate(ctx context.Context, raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("only http and https media URLs are supported")
	}
	if u.User != nil {
		return nil, errors.New("media URLs containing credentials are not allowed")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return nil, errors.New("media URL has no hostname")
	}
	if g.AllowPrivate {
		return u, nil
	}

	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return nil, ErrUnsafeURL
		}
		return u, nil
	}

	resolver := g.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve media hostname: %w", err)
	}
	if len(addresses) == 0 {
		return nil, errors.New("media hostname resolved to no addresses")
	}
	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			return nil, ErrUnsafeURL
		}
	}
	return u, nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil &&
		!ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast()
}

func IsHTTPURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func IsYouTubeURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	return host == "youtube.com" || strings.HasSuffix(host, ".youtube.com") || host == "youtu.be"
}
