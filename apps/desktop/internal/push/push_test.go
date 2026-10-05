package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newManager(t *testing.T) *Manager {
	t.Helper()
	manager, err := New(filepath.Join(t.TempDir(), "push.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestVapidJWT(t *testing.T) {
	manager := newManager(t)
	token, err := manager.vapidJWT("https://push.example.com")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt parts = %d, want 3", len(parts))
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(header), "ES256") {
		t.Fatalf("header = %s", header)
	}
}

func TestSubscribePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "push.json")
	manager, err := New(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if manager.PublicKey() == "" {
		t.Fatal("no VAPID public key")
	}
	if err := manager.Subscribe(Subscription{Endpoint: "https://push.example.com/1"}); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Count() != 1 {
		t.Fatalf("count = %d, want 1", reopened.Count())
	}
	if reopened.PublicKey() != manager.PublicKey() {
		t.Fatal("VAPID keys not persisted")
	}
}

func TestEncryptRoundTrip(t *testing.T) {
	curve := ecdh.P256()
	clientKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	subscription := Subscription{Endpoint: "https://push.example.com/x"}
	subscription.Keys.P256dh = base64.RawURLEncoding.EncodeToString(clientKey.PublicKey().Bytes())
	subscription.Keys.Auth = base64.RawURLEncoding.EncodeToString(auth)

	payload := []byte(`{"body":"hola"}`)
	body, err := encrypt(subscription, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 21 {
		t.Fatalf("body too short: %d", len(body))
	}
	salt := body[:16]
	recordSize := binary.BigEndian.Uint32(body[16:20])
	idLength := int(body[20])
	serverPublic := body[21 : 21+idLength]
	ciphertext := body[21+idLength:]
	if recordSize != 4096 || idLength != 65 {
		t.Fatalf("header rs=%d idlen=%d", recordSize, idLength)
	}

	serverKey, err := curve.NewPublicKey(serverPublic)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := clientKey.ECDH(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	info := append([]byte("WebPush: info\x00"), clientKey.PublicKey().Bytes()...)
	info = append(info, serverPublic...)
	ikm, err := hkdf.Key(sha256.New, shared, auth, string(info), 32)
	if err != nil {
		t.Fatal(err)
	}
	cek, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plaintext) == 0 || plaintext[len(plaintext)-1] != 0x02 {
		t.Fatalf("missing padding delimiter")
	}
	if got := string(plaintext[:len(plaintext)-1]); got != string(payload) {
		t.Fatalf("round trip = %s, want %s", got, payload)
	}
}

func TestSendPostsVapidAndDropsGone(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("authorization")
		if strings.HasSuffix(r.URL.Path, "/gone") {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	manager := newManager(t)
	if err := manager.Subscribe(Subscription{Endpoint: server.URL + "/ok"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Subscribe(Subscription{Endpoint: server.URL + "/gone"}); err != nil {
		t.Fatal(err)
	}
	if sent := manager.Send(); sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}
	if !strings.HasPrefix(gotAuth, "vapid t=") {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if manager.Count() != 1 {
		t.Fatalf("dead subscription not dropped: count = %d", manager.Count())
	}
}
