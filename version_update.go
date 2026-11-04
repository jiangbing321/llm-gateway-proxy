package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	Version                 = "0.0.0-dev"
	VersionServiceURL       = "https://llmgate.yongyitec.com"
	VersionReportSigningKey = ""
	versionHTTPClient       = &http.Client{Timeout: 5 * time.Second}
)

type releaseUpdate struct {
	Version         string `json:"version"`
	DownloadURL     string `json:"downloadUrl"`
	ReleaseNotesURL string `json:"releaseNotesUrl"`
	ReleaseNotes    string `json:"releaseNotes,omitempty"`
}

type versionReport struct {
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func signVersionReport(payload []byte, secret string, now time.Time, nonceSource io.Reader) (timestamp, nonce, signature string, err error) {
	if len([]byte(secret)) < 32 {
		return "", "", "", fmt.Errorf("version report signing key must be at least 32 bytes")
	}
	nonceBytes := make([]byte, 16)
	if _, err := io.ReadFull(nonceSource, nonceBytes); err != nil {
		return "", "", "", fmt.Errorf("generate report nonce: %w", err)
	}

	timestamp = strconv.FormatInt(now.Unix(), 10)
	nonce = hex.EncodeToString(nonceBytes)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "version-report\n%s\n%s\n", timestamp, nonce)
	_, _ = mac.Write(payload)
	return timestamp, nonce, hex.EncodeToString(mac.Sum(nil)), nil
}

func (a *App) checkForVersionUpdates() {
	serviceURL := strings.TrimRight(strings.TrimSpace(VersionServiceURL), "/")
	if serviceURL == "" {
		return
	}

	parsed, err := url.Parse(serviceURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		log.Printf("[version] invalid version service URL")
		return
	}

	go a.reportInstalledVersion(serviceURL)
	go a.fetchLatestVersion(serviceURL)
}

func (a *App) reportInstalledVersion(serviceURL string) {
	if len([]byte(VersionReportSigningKey)) < 32 {
		log.Printf("[version] version report signing key is not configured; skipping report")
		return
	}

	payload, err := json.Marshal(versionReport{
		Version: Version,
		OS:      goruntime.GOOS,
		Arch:    goruntime.GOARCH,
	})
	if err != nil {
		log.Printf("[version] could not encode version report: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serviceURL+"/api/version/report", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[version] could not create version report: %v", err)
		return
	}
	timestamp, nonce, signature, err := signVersionReport(payload, VersionReportSigningKey, time.Now(), rand.Reader)
	if err != nil {
		log.Printf("[version] could not sign version report: %v", err)
		return
	}
	req.Header.Set("X-Version-Timestamp", timestamp)
	req.Header.Set("X-Version-Nonce", nonce)
	req.Header.Set("X-Version-Signature", signature)
	req.Header.Set("Content-Type", "application/json")
	resp, err := versionHTTPClient.Do(req)
	if err != nil {
		log.Printf("[version] version report failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		log.Printf("[version] version report returned HTTP %d", resp.StatusCode)
	}
}

func (a *App) fetchLatestVersion(serviceURL string) {
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serviceURL+"/api/version/latest", nil)
	if err != nil {
		log.Printf("[version] could not create update check: %v", err)
		return
	}
	resp, err := versionHTTPClient.Do(req)
	if err != nil {
		log.Printf("[version] update check failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[version] update check returned HTTP %d", resp.StatusCode)
		return
	}

	var latest releaseUpdate
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&latest); err != nil {
		log.Printf("[version] invalid latest-version response: %v", err)
		return
	}
	comparison, err := compareSemanticVersions(Version, latest.Version)
	if err != nil {
		log.Printf("[version] invalid version in update response: %v", err)
		return
	}
	if comparison >= 0 {
		return
	}
	if !isSecureReleaseURL(latest.DownloadURL) || !isSecureReleaseURL(latest.ReleaseNotesURL) {
		log.Printf("[version] update response contains an invalid release URL")
		return
	}

	a.updateMu.Lock()
	a.update = &latest
	a.updateMu.Unlock()
	runtime.EventsEmit(a.ctx, "version:update-available", latest)
}

func (a *App) GetAvailableUpdate() string {
	a.updateMu.RLock()
	defer a.updateMu.RUnlock()
	if a.update == nil {
		return jsonOK(nil)
	}
	return jsonOK(*a.update)
}

func isSecureReleaseURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

type semanticVersion struct {
	major      string
	minor      string
	patch      string
	prerelease []string
}

func parseSemanticVersion(raw string) (semanticVersion, error) {
	raw = strings.TrimPrefix(raw, "v")
	coreAndMetadata := strings.SplitN(raw, "+", 2)
	coreAndPrerelease := strings.SplitN(coreAndMetadata[0], "-", 2)
	core := strings.Split(coreAndPrerelease[0], ".")
	if len(core) != 3 {
		return semanticVersion{}, fmt.Errorf("expected major.minor.patch")
	}

	version := semanticVersion{}
	components := []*string{&version.major, &version.minor, &version.patch}
	for i, part := range core {
		if !isNumericIdentifier(part) || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, fmt.Errorf("invalid numeric component")
		}
		*components[i] = part
	}

	if len(coreAndPrerelease) == 2 {
		version.prerelease = strings.Split(coreAndPrerelease[1], ".")
		for _, identifier := range version.prerelease {
			if identifier == "" || !isValidVersionIdentifier(identifier) {
				return semanticVersion{}, fmt.Errorf("invalid prerelease identifier")
			}
			if isNumericIdentifier(identifier) && len(identifier) > 1 && identifier[0] == '0' {
				return semanticVersion{}, fmt.Errorf("numeric prerelease identifiers cannot have leading zeroes")
			}
		}
	}

	if len(coreAndMetadata) == 2 {
		for _, identifier := range strings.Split(coreAndMetadata[1], ".") {
			if identifier == "" || !isValidVersionIdentifier(identifier) {
				return semanticVersion{}, fmt.Errorf("invalid build identifier")
			}
		}
	}
	return version, nil
}

func compareSemanticVersions(current, latest string) (int, error) {
	a, err := parseSemanticVersion(current)
	if err != nil {
		return 0, fmt.Errorf("current version %q: %w", current, err)
	}
	b, err := parseSemanticVersion(latest)
	if err != nil {
		return 0, fmt.Errorf("latest version %q: %w", latest, err)
	}
	for _, pair := range [][2]string{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if comparison := compareNumericIdentifiers(pair[0], pair[1]); comparison != 0 {
			return comparison, nil
		}
	}
	if len(a.prerelease) == 0 && len(b.prerelease) == 0 {
		return 0, nil
	}
	if len(a.prerelease) == 0 {
		return 1, nil
	}
	if len(b.prerelease) == 0 {
		return -1, nil
	}
	for i := 0; i < len(a.prerelease) && i < len(b.prerelease); i++ {
		left, right := a.prerelease[i], b.prerelease[i]
		if left == right {
			continue
		}
		switch {
		case isNumericIdentifier(left) && isNumericIdentifier(right):
			return compareNumericIdentifiers(left, right), nil
		case isNumericIdentifier(left):
			return -1, nil
		case isNumericIdentifier(right):
			return 1, nil
		case left < right:
			return -1, nil
		default:
			return 1, nil
		}
	}
	if len(a.prerelease) < len(b.prerelease) {
		return -1, nil
	}
	if len(a.prerelease) > len(b.prerelease) {
		return 1, nil
	}
	return 0, nil
}

func isValidVersionIdentifier(value string) bool {
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'A' && char <= 'Z') ||
			(char >= 'a' && char <= 'z') || char == '-' {
			continue
		}
		return false
	}
	return true
}

func isNumericIdentifier(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
}

func compareNumericIdentifiers(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
