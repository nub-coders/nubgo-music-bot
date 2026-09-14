package session

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/nub-coders/gogram/telegram"
)

var ErrUnsupported = errors.New("unsupported Telegram session string format")

// Normalize converts supported Pyrogram sessions to Gogram's native format.
// Unknown values fail closed instead of being passed to the Telegram client.
func Normalize(raw string, fallbackAppID int32) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty Telegram session string")
	}
	if strings.HasPrefix(raw, "1BvE") || strings.HasPrefix(raw, "1BvX") {
		return raw, nil
	}

	data, err := decode(raw)
	if err != nil {
		return "", fmt.Errorf("decode Telegram session: %w", err)
	}

	const authKeySize = 256
	switch len(data) {
	case 271: // Pyrogram v2: dc + api_id + test + auth_key + user_id + bot
		appID := int32(uint32(data[1])<<24 | uint32(data[2])<<16 | uint32(data[3])<<8 | uint32(data[4]))
		if appID <= 0 {
			appID = fallbackAppID
		}
		sess := &telegram.Session{
			Hostname: telegram.ResolveDC(int(data[0]), data[5] != 0, false),
			AppID:    appID,
			Key:      append([]byte(nil), data[6:6+authKeySize]...),
		}
		return sess.Encode(), nil
	case 263: // Pyrogram v1
		sess := &telegram.Session{
			Hostname: telegram.ResolveDC(int(data[0]), false, false),
			AppID:    fallbackAppID,
			Key:      append([]byte(nil), data[2:2+authKeySize]...),
		}
		return sess.Encode(), nil
	default:
		return "", fmt.Errorf("%w: decoded length %d", ErrUnsupported, len(data))
	}
}

func decode(raw string) ([]byte, error) {
	padded := raw
	for len(padded)%4 != 0 {
		padded += "="
	}
	if data, err := base64.URLEncoding.DecodeString(padded); err == nil {
		return data, nil
	}
	return base64.StdEncoding.DecodeString(padded)
}
