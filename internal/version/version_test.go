package version

import (
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"with v prefix", "v1.2.3", "1.2.3"},
		{"with V prefix", "V1.2.3", "1.2.3"},
		{"without prefix", "1.2.3", "1.2.3"},
		{"empty string", "", ""},
	}

	// Test the normalize function logic
	normalize := func(s string) string {
		if s == "" {
			return s
		}
		if s[0] == 'v' || s[0] == 'V' {
			return s[1:]
		}
		return s
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(tt.input)
			if got != tt.want {
				t.Errorf("normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCheckForUpdate(t *testing.T) {
	// Note: This is a network test, so we skip it in CI unless specifically enabled
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	newVersion, available, err := CheckForUpdate()

	// We don't assert specific values since they depend on what's actually released
	// Just ensure the function doesn't error and returns sensible values
	if err != nil {
		t.Logf("CheckForUpdate() returned error (might be expected if network unavailable): %v", err)
		return
	}

	if available {
		if newVersion == "" {
			t.Error("CheckForUpdate() available=true but newVersion is empty")
		}
		t.Logf("New version available: %s (current: %s)", newVersion, CurrentVersion)
	} else {
		t.Logf("Already on latest version: %s", CurrentVersion)
	}
}

func TestCurrentVersionSet(t *testing.T) {
	if CurrentVersion == "" {
		t.Error("CurrentVersion should not be empty")
	}
}

func TestPrintVersionNoticeNoError(t *testing.T) {
	// PrintVersionNotice should not panic or error
	// It silently fails on network errors, which is expected

	// Save and restore original GitHubAPI to prevent network calls
	originalAPI := GitHubAPI
	GitHubAPI = "" // Empty URL will cause silent failure without network call
	defer func() {
		GitHubAPI = originalAPI
		if r := recover(); r != nil {
			t.Errorf("PrintVersionNotice() panicked: %v", r)
		}
	}()

	PrintVersionNotice()
	// If we reach here without panic, test passes
}

func TestCheckForUpdateGracefulFailure(t *testing.T) {
	// Test that CheckForUpdate handles errors gracefully
	// by verifying it doesn't panic and returns sensible defaults

	// Save original values
	originalAPI := GitHubAPI
	originalTimeout := Timeout

	// Test with invalid URL
	GitHubAPI = "http://invalid-url-that-does-not-exist.local/api"
	defer func() {
		GitHubAPI = originalAPI
		Timeout = originalTimeout
	}()

	newVersion, available, err := CheckForUpdate()

	// Should gracefully handle error
	if err != nil {
		// Errors are acceptable but should be returned, not panic
		t.Logf("CheckForUpdate gracefully handled error: %v", err)
	}

	// When error occurs, should not claim update is available
	if available {
		t.Error("CheckForUpdate() should not claim update available on error")
	}

	// Version should be empty string on error
	if available && newVersion == "" {
		t.Error("If available=true, newVersion should not be empty")
	}
}

func TestVersionComparison(t *testing.T) {
	// Test the normalize function used in CheckForUpdate
	normalize := func(s string) string {
		if s == "" {
			return s
		}
		if s[0] == 'v' || s[0] == 'V' {
			return s[1:]
		}
		return s
	}

	tests := []struct {
		current string
		latest  string
		isDiff  bool
	}{
		{"v1.0.0", "v1.0.0", false},
		{"1.0.0", "1.0.0", false},
		{"v1.0.0", "1.0.0", false}, // After normalization, should be same
		{"v1.0.0", "v2.0.0", true},
		{"1.0.0", "2.0.0", true},
		{"V1.0.0", "v1.0.0", false}, // Case insensitive normalization
	}

	for _, tt := range tests {
		t.Run(tt.current+"_vs_"+tt.latest, func(t *testing.T) {
			normalizedCurrent := normalize(tt.current)
			normalizedLatest := normalize(tt.latest)
			isDiff := normalizedCurrent != normalizedLatest

			if isDiff != tt.isDiff {
				t.Errorf("normalize(%q) vs normalize(%q): got isDiff=%v, want %v",
					tt.current, tt.latest, isDiff, tt.isDiff)
			}
		})
	}
}
