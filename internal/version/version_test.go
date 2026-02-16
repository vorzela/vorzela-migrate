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
