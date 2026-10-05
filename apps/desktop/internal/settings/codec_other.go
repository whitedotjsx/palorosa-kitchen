//go:build !windows

package settings

import (
	"encoding/base64"
	"strings"
)

// On non-Windows builds (CI and the engine tests) secrets are only obscured,
// not encrypted. The desktop shell ships on Windows, where the DPAPI codec
// applies; this fallback exists so the package still compiles and round-trips.
const secretPrefix = "plain:"

func seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return secretPrefix + base64.StdEncoding.EncodeToString([]byte(plain)), nil
}

func open(sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	if !strings.HasPrefix(sealed, secretPrefix) {
		return sealed, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, secretPrefix))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
