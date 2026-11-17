package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	message := "request failed"
	if err != nil {
		message = err.Error()
	}
	writeJSON(w, status, map[string]any{"error": message})
}

func (s *Server) sessionFrom(r *http.Request) *browserSession {
	return s.sessions.get(r)
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) *browserSession {
	session := s.sessionFrom(r)
	if session == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not authenticated", "authenticated": false})
		return nil
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if r.Header.Get("X-VM-CSRF") == "" || r.Header.Get("X-VM-CSRF") != session.CSRF {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "invalid CSRF token"})
			return nil
		}
	}
	return session
}

func (s *Server) auditRequest(r *http.Request, action, result, planID, detail string) {
	s.audit.append(AuditEntry{
		Action: action,
		Result: result,
		PlanID: planID,
		Detail: detail,
		Remote: r.RemoteAddr,
	})
}

func (s *Server) auditPlan(r *http.Request, plan *PublicPlan, result, detail string) {
	entry := AuditEntry{
		Action:     plan.Action,
		Result:     result,
		PlanID:     plan.ID,
		Detail:     detail,
		Remote:     r.RemoteAddr,
		Project:    plan.Target.ProjectName,
		Connection: plan.Target.ConnectionName,
		Database:   plan.Target.Database,
		File:       plan.Target.File,
	}
	s.audit.append(entry)
}

func (s *Server) handleAuthTokenURL(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if !s.sessions.checkLoginAllowed(r.RemoteAddr) {
		s.renderLockError(w, "Too many attempts. Wait 30 seconds and try again.")
		return
	}
	session, err := s.sessions.exchangeBootstrap(token, r.RemoteAddr)
	if err != nil {
		s.auditRequest(r, "auth.token", "denied", "", err.Error())
		s.renderLockError(w, err.Error())
		return
	}
	s.sessions.setCookie(w, session)
	s.auditRequest(r, "auth.token", "ok", "", "session created")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) renderLockError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Vorzela Migrate</title>
<style>body{font:16px system-ui;max-width:38rem;margin:10vh auto;padding:0 1.25rem;line-height:1.5}code{background:#eee;padding:.15rem .35rem;border-radius:4px}</style>
<h1>Vorzela Migrate GUI is locked</h1>
<p>%s</p>
<p>Restart <code>vm gui</code> in your terminal and open the access URL it prints.</p>`, htmlEscape(message))
}

func htmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return replacer.Replace(value)
}

func (s *Server) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	if !s.sessions.checkLoginAllowed(r.RemoteAddr) {
		writeError(w, http.StatusTooManyRequests, errors.New("too many attempts; wait 30 seconds"))
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body"))
		return
	}
	session, err := s.sessions.exchangeBootstrap(body.Token, r.RemoteAddr)
	if err != nil {
		s.auditRequest(r, "auth.token", "denied", "", err.Error())
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error(), "authenticated": false})
		return
	}
	s.sessions.setCookie(w, session)
	s.auditRequest(r, "auth.token", "ok", "", "session created")
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "expiresAt": session.Expires, "csrfToken": session.CSRF})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	session := s.requireSession(w, r)
	if session == nil {
		return
	}
	s.sessions.revoke(session)
	s.sessions.clearCookie(w)
	s.auditRequest(r, "auth.logout", "ok", "", "session revoked")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	session := s.sessionFrom(r)
	if session == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"csrfToken":     session.CSRF,
		"expiresAt":     session.Expires,
	})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	session := s.requireSession(w, r)
	if session == nil {
		return
	}
	state := s.snapshotState()
	projects := make([]PublicProject, 0, len(state.Projects))
	for _, project := range state.Projects {
		projects = append(projects, s.publicProject(project))
	}
	connections := make([]PublicConnection, 0, len(state.Connections))
	for _, conn := range state.Connections {
		connections = append(connections, publicConnection(conn))
	}
	writeJSON(w, http.StatusOK, PublicState{
		Version:         s.opts.Version,
		StoreKind:       s.store.Kind(),
		StorePath:       s.store.Path(),
		ConfigPath:      s.opts.ConfigPath,
		ActiveProjectID: state.ActiveProjectID,
		Projects:        projects,
		Connections:     connections,
		Settings:        state.Settings,
		VMPath:          s.runner.vmPath,
		SessionExpires:  session.Expires,
		CSRFToken:       session.CSRF,
	})
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r) == nil {
		return
	}
	force := r.URL.Query().Get("force") == "1"
	writeJSON(w, http.StatusOK, s.checkForUpdate(force))
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r) == nil {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	writeJSON(w, http.StatusOK, map[string]any{"entries": s.audit.list(limit)})
}

func (s *Server) handleProjectOverview(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r) == nil {
		return
	}
	project, conn, _, err := s.resolveActionTarget(r.PathValue("id"), r.URL.Query().Get("connectionId"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	password := ""
	if conn != nil {
		password, _ = s.connectionPassword(conn)
	}
	overview, err := s.projectOverview(r.Context(), project, conn, password)
	if err != nil {
		// Return the partial overview so the UI can still show check results.
		writeJSON(w, http.StatusOK, overview)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleProjectSchema(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r) == nil {
		return
	}
	project, conn, password, err := s.resolveActionTarget(r.PathValue("id"), r.URL.Query().Get("connectionId"), true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	schema, err := s.projectSchema(r.Context(), project, conn, password)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, schema)
}

func (s *Server) handleMigrationRead(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r) == nil {
		return
	}
	project, conn, _, err := s.resolveActionTarget(r.PathValue("id"), r.URL.Query().Get("connectionId"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	password := ""
	if conn != nil {
		password, _ = s.connectionPassword(conn)
	}
	detail, err := s.readMigration(r.Context(), project, conn, password, r.PathValue("file"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleMigrationAnalyze(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r) == nil {
		return
	}
	project, conn, _, err := s.resolveActionTarget(r.PathValue("id"), r.URL.Query().Get("connectionId"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		File    string `json:"file"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body"))
		return
	}
	if len(body.Content) > 2<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("content is too large"))
		return
	}
	password := ""
	if conn != nil {
		password, _ = s.connectionPassword(conn)
	}
	detail, err := s.analyzeContent(r.Context(), project, conn, password, body.File, body.Content)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleConnectionTest(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r) == nil {
		return
	}
	var body struct {
		ID       string `json:"id,omitempty"`
		Name     string `json:"name,omitempty"`
		Engine   string `json:"engine,omitempty"`
		Host     string `json:"host,omitempty"`
		Port     int    `json:"port,omitempty"`
		Database string `json:"database,omitempty"`
		Username string `json:"username,omitempty"`
		Password string `json:"password,omitempty"`
		SSLMode  string `json:"sslMode,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body"))
		return
	}
	var conn *Connection
	state := s.snapshotState()
	if body.ID != "" {
		saved := state.FindConnection(body.ID)
		if saved == nil {
			writeError(w, http.StatusNotFound, errors.New("connection not found"))
			return
		}
		// Test the values currently in the edit form, not the stale saved row.
		// A blank password still falls back to the saved secret below.
		if body.Name == "" && body.Host == "" && body.Database == "" {
			conn = saved
		} else {
			payload := connectionSavePayload{
				ID: body.ID, Name: body.Name, Engine: body.Engine, Host: body.Host, Port: body.Port,
				Database: body.Database, Username: body.Username, SSLMode: body.SSLMode,
			}
			if err := validateConnectionPayload(&payload); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			conn = &Connection{
				ID: body.ID, Name: payload.Name, Engine: payload.Engine, Host: payload.Host, Port: payload.Port,
				Database: payload.Database, Username: payload.Username, SSLMode: payload.SSLMode,
				PasswordEnc: saved.PasswordEnc,
			}
		}
	} else {
		payload := connectionSavePayload{
			Name: body.Name, Engine: body.Engine, Host: body.Host, Port: body.Port,
			Database: body.Database, Username: body.Username, SSLMode: body.SSLMode,
		}
		if err := validateConnectionPayload(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		conn = &Connection{
			Name: payload.Name, Engine: payload.Engine, Host: payload.Host, Port: payload.Port,
			Database: payload.Database, Username: payload.Username, SSLMode: payload.SSLMode,
		}
	}
	password := body.Password
	if password == "" && conn.ID != "" {
		password, _ = s.connectionPassword(conn)
	}
	start := time.Now()
	err := testConnection(conn, password)
	latency := time.Since(start).Milliseconds()
	if conn.ID != "" && body.Name == "" && body.Host == "" && body.Database == "" {
		ok := err == nil
		_ = s.mutateState(func(state *State) error {
			if saved := state.FindConnection(conn.ID); saved != nil {
				saved.LastTestedAt = timePtr(time.Now().UTC())
				saved.LastTestOK = boolPtr(ok)
			}
			return nil
		})
	}
	if err != nil {
		s.auditRequest(r, "connection.test", "failed", "", conn.Name)
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": redactSecrets(err.Error(), password), "latencyMs": latency})
		return
	}
	s.auditRequest(r, "connection.test", "ok", "", conn.Name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latencyMs": latency})
}

func (s *Server) handlePreparePlan(w http.ResponseWriter, r *http.Request) {
	session := s.requireSession(w, r)
	if session == nil {
		return
	}
	var req prepareRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON body: %w", err))
		return
	}
	plan, err := s.preparePlan(r.Context(), session.ID, req)
	if err != nil {
		s.auditRequest(r, req.Action+".prepare", "failed", "", err.Error())
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	s.auditPlan(r, plan, "prepared", plan.Summary)
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleExecutePlan(w http.ResponseWriter, r *http.Request) {
	session := s.requireSession(w, r)
	if session == nil {
		return
	}
	planID := r.PathValue("id")
	plan, err := s.plans.peek(planID, session.ID)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	var body executeRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid JSON body"))
			return
		}
	}
	if plan.RequiresTypedConfirmation && strings.TrimSpace(body.Phrase) != plan.TypingPhrase {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "the confirmation phrase does not match",
			"planId":  plan.ID,
			"phrase":  plan.TypingPhrase,
			"plan":    plan.PublicPlan,
			"expired": false,
		})
		return
	}
	consumed, err := s.plans.take(planID, session.ID)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	result, execErr := s.executePlan(r.Context(), session, consumed, body.Phrase)
	if execErr != nil {
		s.auditPlan(r, &consumed.PublicPlan, "failed", execErr.Error())
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  execErr.Error(),
			"result": result,
			"plan":   consumed.PublicPlan,
		})
		return
	}
	s.auditPlan(r, &consumed.PublicPlan, "ok", result.Message)
	writeJSON(w, http.StatusOK, map[string]any{"result": result, "plan": consumed.PublicPlan})
}

func (s *Server) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "API route not found"})
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	clean := path.Clean("/" + r.URL.Path)
	rel := strings.TrimPrefix(clean, "/")
	if rel == "" || rel == "." {
		rel = "index.html"
	}
	if strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}
	if data, err := readAsset(s.assets, rel); err == nil {
		w.Header().Set("Content-Type", contentTypeFor(rel))
		if rel == "index.html" {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		_, _ = w.Write(data)
		return
	}
	if strings.HasPrefix(rel, "assets/") {
		http.NotFound(w, r)
		return
	}
	data, err := readAsset(s.assets, "index.html")
	if err != nil {
		http.Error(w, "GUI assets are missing. Build the GUI with `make gui`.", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func readAsset(assets fs.FS, name string) ([]byte, error) {
	return fs.ReadFile(assets, name)
}

func contentTypeFor(name string) string {
	switch {
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}
