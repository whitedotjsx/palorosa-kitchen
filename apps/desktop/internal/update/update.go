// Package update implements the GitHub Releases self-update: resolve the
// latest release, download the Windows exe asset, verify its SHA-256 digest,
// replace the running executable (Windows allows renaming a running exe) and
// relaunch.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultRepo is the public repository that publishes the desktop exe.
const DefaultRepo = "wsuites/palorosa-kitchen"

// DefaultAsset is the release asset name the updater looks for.
const DefaultAsset = "palorosa-kitchen.exe"

// defaultAPIBase is the GitHub REST API root; tests point it at httptest.
const defaultAPIBase = "https://api.github.com"

// maxDownload caps the asset size (the exe embeds cloudflared and the panel).
const maxDownload = 256 << 20

// maxErrorBody caps how much of an error response is quoted back.
const maxErrorBody = 200

// ErrNoRelease means the repository has no published release yet.
var ErrNoRelease = errors.New("no releases published")

// Status is the update lifecycle reported to the panel.
type Status string

// Update lifecycle values.
const (
	StatusIdle        Status = "idle"
	StatusChecking    Status = "checking"
	StatusUpToDate    Status = "up-to-date"
	StatusAvailable   Status = "available"
	StatusDownloading Status = "downloading"
	StatusApplying    Status = "applying"
	StatusUpdated     Status = "updated"
	StatusError       Status = "error"
)

// Release is the resolved GitHub release for this platform.
type Release struct {
	Version     string    `json:"version"`
	Tag         string    `json:"tag"`
	AssetName   string    `json:"assetName"`
	AssetURL    string    `json:"assetUrl"`
	Digest      string    `json:"digest,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	PublishedAt time.Time `json:"publishedAt,omitempty"`
}

// State is the updater snapshot the panel reads.
type State struct {
	Current   string    `json:"current"`
	Latest    string    `json:"latest,omitempty"`
	Available bool      `json:"available"`
	Status    Status    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checkedAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// Config configures a Manager.
type Config struct {
	// Repo is "owner/name". Defaults to DefaultRepo.
	Repo string
	// Asset is the release asset to download. Defaults to DefaultAsset.
	Asset string
	// Current is the running version, without the leading v.
	Current string
	// APIBase overrides the GitHub API root (tests).
	APIBase string
	// ExePath overrides the executable to replace (tests). Empty uses the
	// running executable.
	ExePath string
	// HTTP overrides the client used for the API and the download.
	HTTP *http.Client
	// Restart relaunches the app after a successful replacement. Nil applies
	// the update without restarting.
	Restart func() error
	// Logger defaults to the standard logger.
	Logger *log.Logger
}

// Manager resolves, downloads and applies desktop updates.
type Manager struct {
	cfg    Config
	client *http.Client
	log    *log.Logger

	mu       sync.Mutex
	state    State
	updateMu sync.Mutex
}

// New builds an updater. It performs no network work until Check, Update or
// Run is called.
func New(cfg Config) *Manager {
	if cfg.Repo == "" {
		cfg.Repo = DefaultRepo
	}
	if cfg.Asset == "" {
		cfg.Asset = DefaultAsset
	}
	if cfg.APIBase == "" {
		cfg.APIBase = defaultAPIBase
	}
	client := cfg.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	manager := &Manager{cfg: cfg, client: client, log: logger}
	manager.state = State{Current: cfg.Current, Status: StatusIdle}
	return manager
}

// State returns a copy of the current update state.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Current returns the running version.
func (m *Manager) Current() string {
	return m.cfg.Current
}

// Check resolves the latest release and reports whether it is newer.
func (m *Manager) Check(ctx context.Context) (Release, bool, error) {
	m.setStatus(StatusChecking)
	release, err := m.latest(ctx)
	if errors.Is(err, ErrNoRelease) {
		m.finish(Release{}, false)
		return Release{}, false, nil
	}
	if err != nil {
		m.fail(err)
		return Release{}, false, err
	}
	newer := CompareVersions(m.cfg.Current, release.Version) < 0
	m.finish(release, newer)
	return release, newer, nil
}

// Update checks for a newer release, downloads it, verifies its digest,
// replaces the executable and restarts when a Restart callback is set. The
// bool reports whether the exe was replaced.
func (m *Manager) Update(ctx context.Context) (Release, bool, error) {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	release, newer, err := m.Check(ctx)
	if err != nil || !newer {
		return release, false, err
	}
	m.setStatus(StatusDownloading)
	newFile, err := m.download(ctx, release)
	if err != nil {
		m.fail(err)
		return release, false, err
	}
	exePath, err := m.exePath()
	if err != nil {
		_ = os.Remove(newFile)
		m.fail(err)
		return release, false, err
	}
	m.setStatus(StatusApplying)
	if err := Replace(newFile, exePath); err != nil {
		m.fail(err)
		return release, false, err
	}
	m.mu.Lock()
	m.state.Status = StatusUpdated
	m.state.Available = false
	m.state.UpdatedAt = time.Now()
	m.mu.Unlock()
	m.log.Printf("update: %s installed at %s", release.Version, exePath)
	if m.cfg.Restart != nil {
		if err := m.cfg.Restart(); err != nil {
			m.fail(err)
			return release, true, fmt.Errorf("relaunch: %w", err)
		}
	}
	return release, true, nil
}

// Run updates now and then every interval until ctx is cancelled. A zero
// interval checks once.
func (m *Manager) Run(ctx context.Context, interval time.Duration) {
	Cleanup(m.mustExePath())
	for {
		if ctx.Err() != nil {
			return
		}
		if _, _, err := m.Update(ctx); err != nil && ctx.Err() == nil {
			m.log.Printf("update: %v", err)
		}
		if interval <= 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// latest fetches the newest published release for the configured repo.
func (m *Manager) latest(ctx context.Context) (Release, error) {
	endpoint := strings.TrimRight(m.cfg.APIBase, "/") + "/repos/" + m.cfg.Repo + "/releases/latest"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "palorosa-kitchen-updater")
	response, err := m.client.Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("consultar GitHub: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return Release{}, ErrNoRelease
	}
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub respondió %s: %s", response.Status, readSnippet(response.Body))
	}
	var payload githubRelease
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("leer la release: %w", err)
	}
	version := strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v")
	if version == "" {
		return Release{}, errors.New("la release no tiene tag")
	}
	asset, ok := findAsset(payload.Assets, m.cfg.Asset)
	if !ok {
		return Release{}, fmt.Errorf("la release %s no trae el archivo %s", payload.TagName, m.cfg.Asset)
	}
	return Release{
		Version:     version,
		Tag:         payload.TagName,
		AssetName:   asset.Name,
		AssetURL:    asset.BrowserDownloadURL,
		Digest:      asset.Digest,
		Notes:       strings.TrimSpace(payload.Body),
		PublishedAt: payload.PublishedAt,
	}, nil
}

// download streams the asset next to the executable and verifies its digest.
func (m *Manager) download(ctx context.Context, release Release) (string, error) {
	exePath, err := m.exePath()
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, release.AssetURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "palorosa-kitchen-updater")
	response, err := m.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("descargar la actualización: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("descarga %s: %s", response.Status, readSnippet(response.Body))
	}

	target := exePath + ".new"
	file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", fmt.Errorf("crear el archivo temporal: %w", err)
	}
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, maxDownload+1))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return "", fmt.Errorf("guardar la descarga: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return "", fmt.Errorf("guardar la descarga: %w", closeErr)
	}
	if size > maxDownload {
		_ = os.Remove(target)
		return "", errors.New("la descarga excede el tamaño máximo")
	}
	if err := verifyDigest(hex.EncodeToString(hasher.Sum(nil)), release.Digest); err != nil {
		_ = os.Remove(target)
		return "", err
	}
	return target, nil
}

// exePath resolves the executable to replace.
func (m *Manager) exePath() (string, error) {
	if m.cfg.ExePath != "" {
		return m.cfg.ExePath, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("localizar el ejecutable: %w", err)
	}
	return exe, nil
}

// mustExePath ignores the error; cleanup is best effort.
func (m *Manager) mustExePath() string {
	exe, err := m.exePath()
	if err != nil {
		return ""
	}
	return exe
}

// Replace swaps the executable at exePath with newFile. A running exe cannot
// be overwritten on Windows but it can be renamed, so the current file moves
// to exePath+".old" before the new one takes its place. On failure the old
// file is restored.
func Replace(newFile, exePath string) error {
	oldPath := exePath + ".old"
	_ = os.Remove(oldPath)
	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("renombrar el ejecutable actual: %w", err)
	}
	if err := os.Rename(newFile, exePath); err != nil {
		_ = os.Rename(oldPath, exePath)
		return fmt.Errorf("colocar la actualización: %w", err)
	}
	return nil
}

// Cleanup removes the leftovers of a previous update. The old exe may still
// be locked while its process is running; any failure is ignored.
func Cleanup(exePath string) {
	if exePath == "" {
		return
	}
	_ = os.Remove(exePath + ".old")
	_ = os.Remove(exePath + ".new")
}

// CompareVersions compares dotted versions with an optional -suffix. It
// returns -1 when a is older, 0 when equal and 1 when newer. A plain version
// is newer than the same version with a suffix.
func CompareVersions(a, b string) int {
	aCore, aPre := parseVersion(a)
	bCore, bPre := parseVersion(b)
	for index := range aCore {
		if aCore[index] != bCore[index] {
			if aCore[index] < bCore[index] {
				return -1
			}
			return 1
		}
	}
	switch {
	case aPre == "" && bPre != "":
		return 1
	case aPre != "" && bPre == "":
		return -1
	case aPre < bPre:
		return -1
	case aPre > bPre:
		return 1
	}
	return 0
}

// parseVersion splits "v1.2.3-rc1" into [1,2,3] and "rc1".
func parseVersion(version string) ([3]int, string) {
	trimmed := strings.TrimSpace(version)
	trimmed = strings.TrimPrefix(trimmed, "v")
	core, pre, _ := strings.Cut(trimmed, "-")
	var numbers [3]int
	for index, part := range strings.Split(core, ".") {
		if index > 2 {
			break
		}
		digits := strings.TrimFunc(part, func(r rune) bool {
			return r < '0' || r > '9'
		})
		numbers[index], _ = strconv.Atoi(digits)
	}
	return numbers, pre
}

// findAsset matches the asset by name, case-insensitively.
func findAsset(assets []githubAsset, name string) (githubAsset, bool) {
	for _, asset := range assets {
		if strings.EqualFold(asset.Name, name) {
			return asset, true
		}
	}
	return githubAsset{}, false
}

// verifyDigest checks the sha256 when the release publishes one.
func verifyDigest(got, digest string) error {
	digest = strings.TrimSpace(digest)
	if !strings.HasPrefix(digest, "sha256:") {
		return nil
	}
	want := strings.ToLower(strings.TrimPrefix(digest, "sha256:"))
	if got != want {
		return fmt.Errorf("la verificación sha256 falló: esperado %s, obtenido %s", want, got)
	}
	return nil
}

// readSnippet reads a short, single-line excerpt of an error body.
func readSnippet(body io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(body, maxErrorBody))
	text := strings.Join(strings.Fields(string(raw)), " ")
	if len(text) > maxErrorBody {
		text = text[:maxErrorBody]
	}
	return text
}

func (m *Manager) setStatus(status Status) {
	m.mu.Lock()
	m.state.Status = status
	m.state.Error = ""
	m.mu.Unlock()
}

func (m *Manager) finish(release Release, newer bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.CheckedAt = time.Now()
	m.state.Error = ""
	m.state.Latest = release.Version
	m.state.Available = newer
	if newer {
		m.state.Status = StatusAvailable
	} else {
		m.state.Status = StatusUpToDate
	}
}

func (m *Manager) fail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Status = StatusError
	m.state.Error = err.Error()
}

// githubRelease is the subset of the GitHub release payload the updater uses.
type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Body        string        `json:"body"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

// githubAsset is the subset of a release asset.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}
