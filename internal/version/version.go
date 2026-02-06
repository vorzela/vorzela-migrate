package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	CurrentVersion = "1.0.0"
	GitHubAPI      = "https://api.github.com/repos/vorzela/vorzela-migrate/releases/latest"
	Timeout        = 5 * time.Second
)

// LatestRelease represents a GitHub release
type LatestRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

// CheckForUpdate checks if a new version is available
func CheckForUpdate() (newVersion string, available bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", GitHubAPI, nil)
	if err != nil {
		return "", false, nil // Silently fail, don't disrupt user
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, nil // Network error, silently fail
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, nil
	}

	var release LatestRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return "", false, nil
	}

	version := release.TagName
	if version == "" {
		return "", false, nil
	}

	// Remove 'v' prefix if present for comparison
	currentVer := CurrentVersion
	latestVer := version
	if latestVer[0] == 'v' {
		latestVer = latestVer[1:]
	}

	if latestVer != currentVer {
		return latestVer, true, nil
	}

	return "", false, nil
}

// PrintVersionNotice prints an update notice if a new version is available
func PrintVersionNotice() {
	newVersion, available, _ := CheckForUpdate()
	if available {
		fmt.Printf("\n⚠️  A new version is available: vm %s (current: %s)\n", newVersion, CurrentVersion)
		fmt.Printf("   Run 'vm upgrade' to update\n\n")
	}
}
