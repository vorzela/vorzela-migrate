package version

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeVersionExported(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"v1.2.3", "1.2.3"},
		{"V1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeVersion(tt.in); got != tt.want {
			t.Errorf("normalizeVersion(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}

func TestCheckForUpdateHTTPErrorNotAlreadyLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()

	origAPI, origClient, origVer := GitHubAPI, HTTPClient, CurrentVersion
	GitHubAPI = srv.URL
	HTTPClient = srv.Client()
	CurrentVersion = "v2.2.4"
	defer func() {
		GitHubAPI, HTTPClient, CurrentVersion = origAPI, origClient, origVer
	}()

	newVersion, available, err := CheckForUpdate()
	if err == nil {
		t.Fatal("expected error on HTTP 403")
	}
	if available {
		t.Fatal("must not claim update available on error")
	}
	if newVersion != "" {
		t.Fatalf("newVersion=%q", newVersion)
	}
	if !strings.Contains(err.Error(), "403") && !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("error should mention HTTP/rate limit: %v", err)
	}
}

func TestCheckForUpdateDetectsNewer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v2.2.6","name":"v2.2.6"}`))
	}))
	defer srv.Close()

	origAPI, origClient, origVer := GitHubAPI, HTTPClient, CurrentVersion
	GitHubAPI = srv.URL
	HTTPClient = srv.Client()
	CurrentVersion = "v2.2.4"
	defer func() {
		GitHubAPI, HTTPClient, CurrentVersion = origAPI, origClient, origVer
	}()

	newVersion, available, err := CheckForUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if !available || newVersion != "v2.2.6" {
		t.Fatalf("got available=%v version=%q", available, newVersion)
	}
}

func TestCheckForUpdateSameVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v2.2.4"}`))
	}))
	defer srv.Close()

	origAPI, origClient, origVer := GitHubAPI, HTTPClient, CurrentVersion
	GitHubAPI = srv.URL
	HTTPClient = srv.Client()
	CurrentVersion = "v2.2.4"
	defer func() {
		GitHubAPI, HTTPClient, CurrentVersion = origAPI, origClient, origVer
	}()

	_, available, err := CheckForUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if available {
		t.Fatal("expected no update when versions match")
	}
}

func TestCheckForUpdateNetworkError(t *testing.T) {
	origAPI, origClient, origTimeout := GitHubAPI, HTTPClient, Timeout
	GitHubAPI = "http://127.0.0.1:1" // nothing listening
	Timeout = 200 * time.Millisecond
	HTTPClient = &http.Client{Timeout: Timeout}
	defer func() {
		GitHubAPI, HTTPClient, Timeout = origAPI, origClient, origTimeout
	}()

	_, available, err := CheckForUpdate()
	if err == nil {
		t.Fatal("expected network error")
	}
	if available {
		t.Fatal("must not claim update on network error")
	}
}

func TestPrintVersionNoticeNoPanicOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	origAPI, origClient := GitHubAPI, HTTPClient
	GitHubAPI = srv.URL
	HTTPClient = srv.Client()
	defer func() {
		GitHubAPI, HTTPClient = origAPI, origClient
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()

	PrintVersionNotice()
}
