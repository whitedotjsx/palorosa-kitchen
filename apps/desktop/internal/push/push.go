// Package push delivers Web Push notifications with VAPID. It sends an empty
// push (the service worker shows a generic notification) so it needs no payload
// encryption and only the standard library. Subscriptions are persisted, and a
// 404/410 from the push service removes a dead one.
package push

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	vapidSubject = "mailto:palorosa@palorosabreakfast.com"
	pushTTL      = "60"
)

// Subscription is a browser push subscription.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh,omitempty"`
		Auth   string `json:"auth,omitempty"`
	} `json:"keys,omitempty"`
}

type state struct {
	PrivateKey    string         `json:"privateKey"`
	PublicKey     string         `json:"publicKey"`
	Subscriptions []Subscription `json:"subscriptions"`
}

// Manager owns the VAPID keys and the subscriptions.
type Manager struct {
	path string
	log  *log.Logger

	mu    sync.Mutex
	state state
	key   *ecdsa.PrivateKey
}

// New loads push.json, generating the VAPID key pair on first run.
func New(path string, logger *log.Logger) (*Manager, error) {
	manager := &Manager{path: path, log: logger}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(raw, &manager.state); err != nil {
			return nil, fmt.Errorf("push: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if manager.state.PrivateKey == "" {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		manager.key = key
		manager.state.PrivateKey = base64.RawURLEncoding.EncodeToString(padScalar(key.D))
		manager.state.PublicKey = base64.RawURLEncoding.EncodeToString(elliptic.Marshal(elliptic.P256(), key.PublicKey.X, key.PublicKey.Y))
		if err := manager.saveLocked(); err != nil {
			return nil, err
		}
		return manager, nil
	}

	private, err := base64.RawURLEncoding.DecodeString(manager.state.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("push: private key: %w", err)
	}
	key := new(ecdsa.PrivateKey)
	key.Curve = elliptic.P256()
	key.D = new(big.Int).SetBytes(private)
	key.PublicKey.Curve = elliptic.P256()
	key.PublicKey.X, key.PublicKey.Y = elliptic.P256().ScalarBaseMult(private)
	manager.key = key
	return manager, nil
}

// PublicKey is the VAPID application server key (base64url, uncompressed).
func (m *Manager) PublicKey() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.PublicKey
}

// Count is the number of subscriptions.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.state.Subscriptions)
}

// Subscribe stores (or replaces) a subscription by endpoint.
func (m *Manager) Subscribe(subscription Subscription) error {
	if subscription.Endpoint == "" {
		return fmt.Errorf("push: empty endpoint")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.state.Subscriptions {
		if m.state.Subscriptions[index].Endpoint == subscription.Endpoint {
			m.state.Subscriptions[index] = subscription
			return m.saveLocked()
		}
	}
	m.state.Subscriptions = append(m.state.Subscriptions, subscription)
	return m.saveLocked()
}

// Unsubscribe removes a subscription by endpoint.
func (m *Manager) Unsubscribe(endpoint string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.state.Subscriptions[:0]
	for _, subscription := range m.state.Subscriptions {
		if subscription.Endpoint != endpoint {
			kept = append(kept, subscription)
		}
	}
	m.state.Subscriptions = kept
	return m.saveLocked()
}

// Send pushes a notification to every subscription. The payload is encrypted
// per subscription (RFC 8291, aes128gcm); a subscription without keys gets an
// empty push. It is best effort: failures are logged and a dead subscription is
// dropped.
func (m *Manager) Send() int {
	m.mu.Lock()
	subs := append([]Subscription{}, m.state.Subscriptions...)
	m.mu.Unlock()
	if len(subs) == 0 {
		return 0
	}

	payload, _ := json.Marshal(map[string]string{
		"title": "Panel de cocina",
		"body":  "Hay novedades en la cocina.",
	})

	client := &http.Client{Timeout: 10 * time.Second}
	sent := 0
	dead := map[string]bool{}
	for _, subscription := range subs {
		audience, err := origin(subscription.Endpoint)
		if err != nil {
			continue
		}
		jwt, err := m.vapidJWT(audience)
		if err != nil {
			m.logf("vapid token: %v", err)
			continue
		}

		var body []byte
		encrypted := false
		if subscription.Keys.P256dh != "" && subscription.Keys.Auth != "" {
			if body, err = encrypt(subscription, payload); err != nil {
				m.logf("encrypt: %v", err)
				continue
			}
			encrypted = true
		}

		request, err := http.NewRequest(http.MethodPost, subscription.Endpoint, bytes.NewReader(body))
		if err != nil {
			continue
		}
		request.Header.Set("authorization", "vapid t="+jwt+", k="+m.PublicKey())
		request.Header.Set("ttl", pushTTL)
		if encrypted {
			request.Header.Set("content-encoding", "aes128gcm")
			request.Header.Set("content-type", "application/octet-stream")
		}
		response, err := client.Do(request)
		if err != nil {
			m.logf("push failed: %v", err)
			continue
		}
		_ = response.Body.Close()
		switch response.StatusCode {
		case http.StatusNotFound, http.StatusGone:
			dead[subscription.Endpoint] = true
		default:
			if response.StatusCode < 300 {
				sent++
			}
		}
	}
	if len(dead) > 0 {
		m.mu.Lock()
		kept := m.state.Subscriptions[:0]
		for _, subscription := range m.state.Subscriptions {
			if !dead[subscription.Endpoint] {
				kept = append(kept, subscription)
			}
		}
		m.state.Subscriptions = kept
		_ = m.saveLocked()
		m.mu.Unlock()
	}
	return sent
}

// encrypt builds the aes128gcm body for one subscription per RFC 8291.
func encrypt(subscription Subscription, payload []byte) ([]byte, error) {
	clientPublic, err := base64.RawURLEncoding.DecodeString(subscription.Keys.P256dh)
	if err != nil {
		return nil, err
	}
	authSecret, err := base64.RawURLEncoding.DecodeString(subscription.Keys.Auth)
	if err != nil {
		return nil, err
	}

	curve := ecdh.P256()
	clientKey, err := curve.NewPublicKey(clientPublic)
	if err != nil {
		return nil, err
	}
	serverKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := serverKey.ECDH(clientKey)
	if err != nil {
		return nil, err
	}
	serverPublic := serverKey.PublicKey().Bytes()

	// IKM = HKDF(salt=auth, ikm=shared, info="WebPush: info\0"||ua||as).
	info := append([]byte("WebPush: info\x00"), clientPublic...)
	info = append(info, serverPublic...)
	ikm, err := hkdf.Key(sha256.New, shared, authSecret, string(info), 32)
	if err != nil {
		return nil, err
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext := append(append([]byte{}, payload...), 0x02)
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	body := make([]byte, 0, 16+4+1+len(serverPublic)+len(ciphertext))
	body = append(body, salt...)
	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, 4096)
	body = append(body, size...)
	body = append(body, byte(len(serverPublic)))
	body = append(body, serverPublic...)
	body = append(body, ciphertext...)
	return body, nil
}

func (m *Manager) vapidJWT(audience string) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims := fmt.Sprintf(`{"aud":%q,"exp":%d,"sub":%q}`, audience, time.Now().Add(12*time.Hour).Unix(), vapidSubject)
	payload := base64.RawURLEncoding.EncodeToString([]byte(claims))
	signingInput := header + "." + payload

	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, m.key, digest[:])
	if err != nil {
		return "", err
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (m *Manager) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m.state, "", "\t")
	if err != nil {
		return err
	}
	temporary := m.path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, m.path)
}

func (m *Manager) logf(format string, args ...any) {
	if m.log != nil {
		m.log.Printf(format, args...)
	}
}

func origin(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("push: bad endpoint")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func padScalar(value *big.Int) []byte {
	out := make([]byte, 32)
	value.FillBytes(out)
	return out
}
