package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	// CurrentVersion is injected at build time via -ldflags "-X 'github.com/vorzela/vorzela-migrate/internal/version.CurrentVersion=vX.Y.Z'"
	// Default to "dev" when not provided.
	CurrentVersion = "dev"
	GitHubAPI      = "https://api.github.com/repos/vorzela/vorzela-migrate/releases/latest"
	Timeout        = 5 * time.Second

	// HTTPClient is overridable in tests.
	HTTPClient = http.DefaultClient
)

// LatestRelease represents a GitHub release
type LatestRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

// CheckForUpdate checks if a newer release than CurrentVersion is available.
// On network/API failure it returns err — callers must NOT treat that as "already latest".
func CheckForUpdate() (newVersion string, available bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", GitHubAPI, nil)
	if err != nil {
		return "", false, fmt.Errorf("build update request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "vorzela-migrate/"+normalizeVersion(CurrentVersion))

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("check for updates failed (network): %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return "", false, fmt.Errorf("read update response: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		// GitHub often returns a JSON {"message":"..."} on 403 rate limit.
		var apiErr struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
			msg = apiErr.Message
		}
		return "", false, fmt.Errorf("check for updates failed (HTTP %d): %s", resp.StatusCode, msg)
	}

	var release LatestRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return "", false, fmt.Errorf("parse update response: %w", err)
	}

	version := strings.TrimSpace(release.TagName)
	if version == "" {
		return "", false, fmt.Errorf("check for updates failed: empty tag_name in latest release")
	}

	currentVer := normalizeVersion(CurrentVersion)
	latestVer := normalizeVersion(version)

	// "dev" / empty always treat remote as newer when we got a tag.
	if currentVer == "" || currentVer == "dev" {
		return version, true, nil
	}
	if latestVer != "" && latestVer != currentVer {
		return version, true, nil
	}

	return version, false, nil
}

func normalizeVersion(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if s[0] == 'v' || s[0] == 'V' {
		return s[1:]
	}
	return s
}

// PrintVersionNotice prints an update notice if a new version is available.
// Network/API errors are ignored here so everyday commands stay quiet.
func PrintVersionNotice() {
	newVersion, available, err := CheckForUpdate()
	if err != nil || !available {
		return
	}
	fmt.Printf("\n⚠️  A new version is available: vm %s (current: %s)\n", newVersion, CurrentVersion)
	fmt.Printf("   Run 'vm upgrade' to update\n\n")
}
