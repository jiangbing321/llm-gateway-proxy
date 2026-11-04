package main

import "testing"

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
