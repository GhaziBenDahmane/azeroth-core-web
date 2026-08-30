package web

import (
	"strings"
	"testing"
)

func TestValidateDownloadTrustMetadata(t *testing.T) {
	valid := portalDownload{
		Name:          "Windows full client",
		Platform:      "Windows",
		URL:           "https://downloads.example.test/client.zip",
		SHA256:        strings.Repeat("a", 64),
		SignatureURL:  "https://downloads.example.test/client.zip.sig",
		VirusTotalURL: "https://www.virustotal.com/gui/file/example",
		ChangelogURL:  "https://portal.example.test/changelog/client-1",
		ReleasedAt:    "2026-08-30",
		Requirements:  "Windows 10, 4 GB RAM, 25 GB storage",
		Mirrors:       []downloadMirror{{Label: "Europe", URL: "https://eu.example.test/client.zip"}},
	}
	if err := validateDownload(valid); err != nil {
		t.Fatalf("valid download rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*portalDownload)
	}{
		{"invalid release date", func(item *portalDownload) { item.ReleasedAt = "30/08/2026" }},
		{"invalid VirusTotal URL", func(item *portalDownload) { item.VirusTotalURL = "javascript:alert(1)" }},
		{"invalid changelog URL", func(item *portalDownload) { item.ChangelogURL = "/relative" }},
		{"oversized requirements", func(item *portalDownload) { item.Requirements = strings.Repeat("x", 1001) }},
		{"unsafe mirror URL", func(item *portalDownload) {
			item.Mirrors = []downloadMirror{{Label: "Mirror", URL: "javascript:alert(1)"}}
		}},
		{"duplicate mirror URL", func(item *portalDownload) {
			item.Mirrors = []downloadMirror{{Label: "One", URL: "https://mirror.example.test/client.zip"}, {Label: "Two", URL: "https://mirror.example.test/client.zip"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := valid
			tc.mutate(&item)
			if err := validateDownload(item); err == nil {
				t.Fatal("invalid download accepted")
			}
		})
	}
}

func TestValidateLauncherPatch(t *testing.T) {
	valid := launcherPatch{
		Platform:     "Windows",
		FromVersion:  "1.0.0",
		ToVersion:    "1.1.0",
		URL:          "https://downloads.example.test/patch.zip",
		SHA256:       strings.Repeat("b", 64),
		SignatureURL: "https://downloads.example.test/patch.zip.sig",
		ReleasedAt:   "2026-08-30",
		Mirrors:      []downloadMirror{{Label: "North America", URL: "https://na.example.test/patch.zip"}},
	}
	if err := validateLauncherPatch(valid); err != nil {
		t.Fatalf("valid launcher patch rejected: %v", err)
	}
	for name, mutate := range map[string]func(*launcherPatch){
		"missing primary URL": func(item *launcherPatch) { item.URL = "" },
		"same version":        func(item *launcherPatch) { item.ToVersion = item.FromVersion },
		"missing checksum":    func(item *launcherPatch) { item.SHA256 = "" },
		"unsafe signature":    func(item *launcherPatch) { item.SignatureURL = "file:///tmp/patch.sig" },
	} {
		t.Run(name, func(t *testing.T) {
			item := valid
			mutate(&item)
			if err := validateLauncherPatch(item); err == nil {
				t.Fatal("invalid launcher patch accepted")
			}
		})
	}
}
