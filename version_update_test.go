package main

import (
	"bytes"
	"testing"
	"time"
)

func TestSignVersionReportMatchesWorkerProtocol(t *testing.T) {
	payload := []byte(`{"version":"1.2.3","os":"windows","arch":"amd64"}`)
	nonceSource := bytes.NewReader(bytes.Repeat([]byte{0x11}, 16))
	timestamp, nonce, signature, err := signVersionReport(
		payload,
		"0123456789abcdef0123456789abcdef",
		time.Unix(1730000000, 0),
		nonceSource,
	)
	if err != nil {
		t.Fatal(err)
	}
	if timestamp != "1730000000" {
		t.Fatalf("timestamp = %q, want 1730000000", timestamp)
	}
	if nonce != "11111111111111111111111111111111" {
		t.Fatalf("nonce = %q", nonce)
	}
	const wantSignature = "02d5269f0f1a3ae381c40e09978310c19dde77022e814d84df10dcb71f264156"
	if signature != wantSignature {
		t.Fatalf("signature = %q, want %q", signature, wantSignature)
	}
}

func TestSignVersionReportRequiresStrongKeyAndNonce(t *testing.T) {
	payload := []byte(`{"version":"1.2.3","os":"windows","arch":"amd64"}`)
	now := time.Unix(1730000000, 0)
	if _, _, _, err := signVersionReport(payload, "short", now, bytes.NewReader(make([]byte, 16))); err == nil {
		t.Fatal("expected a short signing key to be rejected")
	}
	if _, _, _, err := signVersionReport(payload, "0123456789abcdef0123456789abcdef", now, bytes.NewReader(nil)); err == nil {
		t.Fatal("expected an unavailable nonce source to be rejected")
	}
}

func TestCompareSemanticVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    int
	}{
		{name: "newer patch", current: "1.2.3", latest: "1.2.4", want: -1},
		{name: "same version", current: "v1.2.3+build.2", latest: "1.2.3+build.9", want: 0},
		{name: "stable is newer than prerelease", current: "1.2.3-rc.2", latest: "1.2.3", want: -1},
		{name: "numeric prerelease identifiers", current: "1.2.3-rc.2", latest: "1.2.3-rc.10", want: -1},
		{name: "large numeric prerelease identifiers", current: "1.2.3-999999999999999999999", latest: "1.2.3-1000000000000000000000", want: -1},
		{name: "numeric prerelease sorts before text", current: "1.2.3-1", latest: "1.2.3-alpha", want: -1},
		{name: "large core version components", current: "999999999999999999999999.0.0", latest: "1000000000000000000000000.0.0", want: -1},
		{name: "current is newer", current: "2.0.0", latest: "1.99.99", want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := compareSemanticVersions(test.current, test.latest)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("compareSemanticVersions(%q, %q) = %d, want %d", test.current, test.latest, got, test.want)
			}
		})
	}
}

func TestCompareSemanticVersionsRejectsMalformedVersions(t *testing.T) {
	for _, version := range []string{"1.2", "01.2.3", "1.2.3-01", "1.2.3-"} {
		t.Run(version, func(t *testing.T) {
			if _, err := compareSemanticVersions("1.2.3", version); err == nil {
				t.Fatalf("expected %q to be rejected", version)
			}
		})
	}
}
