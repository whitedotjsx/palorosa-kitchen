//go:build windows

package settings

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// secretPrefix marks a value encrypted with the Windows DPAPI.
const secretPrefix = "dpapi:"

// CRYPTPROTECT_UI_FORBIDDEN keeps the call from showing a prompt.
const cryptProtectUIForbidden = 0x1

// seal encrypts a plaintext secret with DPAPI (per user, per machine) and
// returns it prefixed for storage. An empty value stays empty.
func seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	sealed, err := dpapiProtect([]byte(plain))
	if err != nil {
		return "", err
	}
	return secretPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// open decrypts a stored secret. A value without the prefix is returned as is,
// so a manually written or migrated plaintext still loads.
func open(sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	if !strings.HasPrefix(sealed, secretPrefix) {
		return sealed, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, secretPrefix))
	if err != nil {
		return "", fmt.Errorf("settings: decode secret: %w", err)
	}
	plain, err := dpapiUnprotect(raw)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func dpapiProtect(data []byte) ([]byte, error) {
	in := dataBlob(data)
	var out windows.DataBlob
	var inPtr *windows.DataBlob
	if in.Size > 0 {
		inPtr = &in
	}
	if err := windows.CryptProtectData(inPtr, nil, nil, 0, nil, cryptProtectUIForbidden, &out); err != nil {
		return nil, fmt.Errorf("settings: CryptProtectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) //nolint:errcheck
	return copyBlob(&out), nil
}

func dpapiUnprotect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	in := dataBlob(data)
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, cryptProtectUIForbidden, &out); err != nil {
		return nil, fmt.Errorf("settings: CryptUnprotectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) //nolint:errcheck
	return copyBlob(&out), nil
}

func dataBlob(data []byte) windows.DataBlob {
	blob := windows.DataBlob{Size: uint32(len(data))}
	if len(data) > 0 {
		blob.Data = &data[0]
	}
	return blob
}

// copyBlob copies the output before LocalFree releases it.
func copyBlob(blob *windows.DataBlob) []byte {
	if blob.Data == nil || blob.Size == 0 {
		return nil
	}
	out := make([]byte, blob.Size)
	copy(out, unsafe.Slice(blob.Data, blob.Size))
	return out
}
