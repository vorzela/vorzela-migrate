package gui

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

//go:embed all:web/dist
var webAssets embed.FS

// Options configures the local GUI server.
type Options struct {
	Addr        string
	ConfigPath  string
	StoreKind   StoreKind
	ConfigDir   string
	KeyPath     string
	AuditPath   string
	VM          string
	NoOpen      bool
	AllowRemote bool
	TLSCert     string
	TLSKey      string
	DevOrigin   string
	Version     string
	OpenBrowser func(string) error
}

// Server is the local HTTP API and static asset host for the GUI.
type Server struct {
	opts     Options
	store    Store
	secrets  *SecretBox
	runner   *Runner
	sessions *sessionManager
	plans    *planStore
	audit    *auditLog
	logger   *log.Logger
	assets   fs.FS

	stateMu sync.Mutex
	state   *State

	updateMu   sync.Mutex
	updateInfo *UpdateInfo
	updateAt   time.Time

	httpServer   *http.Server
	listener     net.Listener
	allowedHosts map[string]bool
	allowAnyHost bool
	loopbackURL  string
	bootstrap    string
}

// NewServer loads configuration, secrets and the static bundle.
func NewServer(opts Options) (*Server, error) {
	if opts.ConfigDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("locate user config directory: %w", err)
		}
		opts.ConfigDir = filepath.Join(dir, "vorzela-migrate")
	}
	if err := os.MkdirAll(opts.ConfigDir, 0o700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(opts.ConfigDir, 0o700)
	}
	if opts.StoreKind == "" {
		opts.StoreKind = StoreYAML
	}
	if opts.ConfigPath == "" {
		if opts.StoreKind == StoreSQLite {
			opts.ConfigPath = filepath.Join(opts.ConfigDir, "gui.db")
		} else {
			opts.ConfigPath = filepath.Join(opts.ConfigDir, "gui.yaml")
		}
	}
	if opts.KeyPath == "" {
		opts.KeyPath = filepath.Join(opts.ConfigDir, "gui.key")
	}
	if opts.AuditPath == "" {
		opts.AuditPath = filepath.Join(opts.ConfigDir, "audit.jsonl")
	}
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:7845"
	}

	store, err := OpenStore(opts.StoreKind, opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	state, err := store.Load()
	if err != nil {
		store.Close()
		return nil, err
	}
	secrets, err := LoadOrCreateSecretBox(opts.KeyPath)
	if err != nil {
		store.Close()
		return nil, err
	}
	runner, err := newRunner(opts.VM)
	if err != nil {
		store.Close()
		return nil, err
	}
	secureCookie := opts.TLSCert != "" && opts.TLSKey != ""
	sessions, err := newSessionManager(time.Duration(state.Settings.SessionTTLMinutes)*time.Minute, secureCookie)
	if err != nil {
		store.Close()
		return nil, err
	}
	plans := newPlanStore(time.Duration(state.Settings.ConfirmationTTLSeconds) * time.Second)
	audit := newAuditLog(opts.AuditPath)
	audit.loadAuditFromDisk(200)

	sub, err := fs.Sub(webAssets, "web/dist")
	if err != nil {
		store.Close()
		return nil, err
	}

	s := &Server{
		opts:     opts,
		store:    store,
		secrets:  secrets,
		runner:   runner,
		sessions: sessions,
		plans:    plans,
		audit:    audit,
		logger:   log.New(os.Stderr, "vm gui: ", log.LstdFlags),
		assets:   sub,
		state:    state,
	}
	token, _ := sessions.bootstrap()
	s.bootstrap = token
	return s, nil
}

// Close releases the store.
func (s *Server) Close() error { return s.store.Close() }

func (s *Server) snapshotState() *State {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.state.Clone()
}

func (s *Server) mutateState(fn func(*State) error) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	next := s.state.Clone()
	if err := fn(next); err != nil {
		return err
	}
	next.Version = 1
	if err := s.store.Save(next); err != nil {
		return err
	}
	s.state = next
	return nil
}

// Run starts the server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.opts.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.opts.Addr, err)
	}
	s.listener = ln
	host, _, _ := net.SplitHostPort(ln.Addr().String())
	tlsEnabled := s.opts.TLSCert != "" && s.opts.TLSKey != ""
	if !isLoopbackHost(host) {
		if !s.opts.AllowRemote {
			ln.Close()
			return fmt.Errorf("refusing to bind to non-loopback address %s without --allow-remote", s.opts.Addr)
		}
		if !tlsEnabled {
			ln.Close()
			return errors.New("refusing plain HTTP on a non-loopback address; pass --tls-cert and --tls-key (or use a TLS reverse proxy on loopback)")
		}
	}
	s.allowedHosts, s.allowAnyHost = allowedHosts(ln.Addr().String(), s.opts.AllowRemote)

	scheme := "http"
	if tlsEnabled {
		cert, err := tls.LoadX509KeyPair(s.opts.TLSCert, s.opts.TLSKey)
		if err != nil {
			ln.Close()
			return fmt.Errorf("load TLS certificate: %w", err)
		}
		s.sessions.secureCookie = true
		ln = tls.NewListener(ln, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})
		scheme = "https"
	}

	displayHost := host
	if host == "::" || host == "0.0.0.0" {
		displayHost = "127.0.0.1"
	}
	s.loopbackURL = fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(displayHost, portOf(ln.Addr().String())))

	mux := http.NewServeMux()
	s.registerRoutes(mux)
	s.httpServer = &http.Server{
		Handler:           s.middleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	tokenURL := s.loopbackURL + "/auth?token=" + s.bootstrap
	fmt.Fprintf(os.Stderr, "\n  Vorzela Migrate GUI\n")
	fmt.Fprintf(os.Stderr, "  ─────────────────────────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  URL:    %s\n", s.loopbackURL)
	fmt.Fprintf(os.Stderr, "  Access: %s\n", tokenURL)
	fmt.Fprintf(os.Stderr, "  Store:  %s (%s)\n", s.store.Path(), s.store.Kind())
	fmt.Fprintf(os.Stderr, "  Key:    %s\n", s.secrets.KeyPath())
	fmt.Fprintf(os.Stderr, "  Data:   %s\n", s.opts.ConfigDir)
	fmt.Fprintf(os.Stderr, "  Serve:  loopback only, one-time token, CSRF-protected API\n")
	fmt.Fprintf(os.Stderr, "  Stop:   Ctrl+C\n\n")

	if !s.opts.NoOpen {
		go func() {
			opener := s.opts.OpenBrowser
			if opener == nil {
				opener = openBrowser
			}
			_ = opener(tokenURL)
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.Serve(ln)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth", s.handleAuthTokenURL)
	mux.HandleFunc("POST /api/auth/token", s.handleAuthToken)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/activity", s.handleActivity)
	mux.HandleFunc("GET /api/updates/check", s.handleUpdateCheck)
	mux.HandleFunc("GET /api/projects/{id}/overview", s.handleProjectOverview)
	mux.HandleFunc("GET /api/projects/{id}/schema", s.handleProjectSchema)
	mux.HandleFunc("GET /api/projects/{id}/migrations/{file}", s.handleMigrationRead)
	mux.HandleFunc("POST /api/projects/{id}/migrations/analyze", s.handleMigrationAnalyze)
	mux.HandleFunc("POST /api/connections/test", s.handleConnectionTest)
	mux.HandleFunc("POST /api/plans", s.handlePreparePlan)
	mux.HandleFunc("POST /api/plans/{id}/execute", s.handleExecutePlan)
	mux.HandleFunc("GET /api/", s.handleAPINotFound)
	mux.HandleFunc("POST /api/", s.handleAPINotFound)
	mux.HandleFunc("GET /", s.handleStatic)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, recovered)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			}
		}()
		setSecurityHeaders(w)
		if !s.hostAllowed(r.Host) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "invalid host"})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/auth" {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				if !requestIsSameOrigin(r, s.opts.DevOrigin) {
					writeJSON(w, http.StatusForbidden, map[string]any{"error": "cross-origin request refused"})
					return
				}
				if r.Header.Get("X-VM-GUI") != "1" {
					writeJSON(w, http.StatusForbidden, map[string]any{"error": "missing X-VM-GUI header"})
					return
				}
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
		}
		next.ServeHTTP(w, r)
	})
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
}

func (s *Server) hostAllowed(hostport string) bool {
	if s.allowAnyHost {
		return true
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	return s.allowedHosts[host]
}

func allowedHosts(addr string, allowRemote bool) (map[string]bool, bool) {
	host, _, _ := net.SplitHostPort(addr)
	host = strings.ToLower(strings.Trim(host, "[]"))
	if allowRemote && !isLoopbackHost(host) && host != "0.0.0.0" && host != "::" {
		return nil, true
	}
	set := map[string]bool{
		"127.0.0.1": true,
		"localhost": true,
		"::1":       true,
	}
	if isLoopbackHost(host) {
		set[host] = true
	}
	return set, false
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func portOf(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "7845"
	}
	return port
}

func requestIsSameOrigin(r *http.Request, devOrigin string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		if devOrigin != "" && strings.EqualFold(origin, strings.TrimRight(devOrigin, "/")) {
			return true
		}
		expected := "http://" + r.Host
		if r.TLS != nil {
			expected = "https://" + r.Host
		}
		return strings.EqualFold(origin, expected)
	}
	site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	if site == "" || site == "none" || site == "same-origin" {
		return true
	}
	return false
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
