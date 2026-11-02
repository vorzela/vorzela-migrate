package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

var (
	// CurrentVersion is injected at build time via -ldflags "-X 'github.com/vorzela/vorzela-migrate/v2/internal/version.CurrentVersion=vX.Y.Z'"
	// Default to "dev" when not provided.
	CurrentVersion = "dev"
	GitHubAPI      = "https://api.github.com/repos/vorzela/vorzela-migrate/releases/latest"
	Timeout        = 5 * time.Second

	// HTTPClient is overridable in tests.
	HTTPClient = http.DefaultClient
)

func init() {
	// `go install .../v2/cmd/vm@v2.3.x` does not run our release ldflags.
	// Read the module version from Go's build info so --version is accurate.
	if CurrentVersion == "" || CurrentVersion == "dev" {
		if v := moduleVersion(); v != "" {
			CurrentVersion = v
		}
	}
}

func moduleVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi == nil {
		return ""
	}
	v := bi.Main.Version
	if v == "" || v == "(devel)" {
		return ""
	}
	return v
}

// LatestRelease represents a GitHub release.
type LatestRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

// CheckForUpdate checks if a newer release than CurrentVersion is available.
// On network/API failure it returns err — callers must NOT treat that as
// "already latest".
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

	newer, err := IsNewer(CurrentVersion, version)
	if err != nil {
		return "", false, fmt.Errorf("compare versions: %w", err)
	}
	return version, newer, nil
}

// IsNewer reports whether candidate is strictly newer than current. A current
// version of "dev" (or an empty string) always treats a tagged release as
// newer. Downgrades are never reported as updates.
func IsNewer(current, candidate string) (bool, error) {
	current = normalizeVersion(current)
	candidate = normalizeVersion(candidate)

	if current == "" || strings.EqualFold(current, "dev") {
		return candidate != "", nil
	}
	if candidate == "" {
		return false, nil
	}

	cur, err := parseSemver(current)
	if err != nil {
		return false, fmt.Errorf("invalid current version %q: %w", current, err)
	}
	cand, err := parseSemver(candidate)
	if err != nil {
		return false, fmt.Errorf("invalid release version %q: %w", candidate, err)
	}
	return compareSemver(cand, cur) > 0, nil
}

type semver struct {
	major, minor, patch int
	prerelease          []string
}

func parseSemver(v string) (semver, error) {
	var out semver
	v = strings.TrimSpace(normalizeVersion(v))
	if v == "" {
		return out, fmt.Errorf("empty version")
	}

	// Ignore build metadata (+...).
	if idx := strings.IndexByte(v, '+'); idx >= 0 {
		v = v[:idx]
	}
	// Split pre-release.
	var pre string
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		pre = v[idx+1:]
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return out, fmt.Errorf("expected MAJOR.MINOR[.PATCH]")
	}
	nums := []*int{&out.major, &out.minor, &out.patch}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, fmt.Errorf("invalid numeric component %q", p)
		}
		*nums[i] = n
	}
	if pre != "" {
		out.prerelease = strings.Split(pre, ".")
	}
	return out, nil
}

func compareSemver(a, b semver) int {
	if c := compareInt(a.major, b.major); c != 0 {
		return c
	}
	if c := compareInt(a.minor, b.minor); c != 0 {
		return c
	}
	if c := compareInt(a.patch, b.patch); c != 0 {
		return c
	}
	// A release (no prerelease) is newer than any prerelease.
	if len(a.prerelease) == 0 && len(b.prerelease) == 0 {
		return 0
	}
	if len(a.prerelease) == 0 {
		return 1
	}
	if len(b.prerelease) == 0 {
		return -1
	}
	for i := 0; i < len(a.prerelease) && i < len(b.prerelease); i++ {
		if c := comparePrereleasePart(a.prerelease[i], b.prerelease[i]); c != 0 {
			return c
		}
	}
	return compareInt(len(a.prerelease), len(b.prerelease))
}

func comparePrereleasePart(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return compareInt(an, bn)
	case aErr == nil:
		return -1 // numeric identifiers have lower precedence
	case bErr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
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
// It also stays quiet for pipes, agents, CI, and JSON consumers.
func PrintVersionNotice() {
	if !shouldPrintNotice() {
		return
	}
	newVersion, available, err := CheckForUpdate()
	if err != nil || !available {
		return
	}
	fmt.Printf("\n⚠️  A new version is available: vm %s (current: %s)\n", newVersion, CurrentVersion)
	fmt.Printf("   Run 'vm upgrade' to update\n\n")
}

func shouldPrintNotice() bool {
	if os.Getenv("VM_NON_INTERACTIVE") != "" || os.Getenv("VM_NO_COLOR") != "" || os.Getenv("CI") != "" {
		return false
	}
	for _, arg := range os.Args[1:] {
		if arg == "--json" {
			return false
		}
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
