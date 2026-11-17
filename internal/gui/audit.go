package gui

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// AuditEntry is one security-relevant event. Secrets are never included.
type AuditEntry struct {
	Time       time.Time `json:"time"`
	Action     string    `json:"action"`
	Result     string    `json:"result"`
	Project    string    `json:"project,omitempty"`
	Connection string    `json:"connection,omitempty"`
	Database   string    `json:"database,omitempty"`
	File       string    `json:"file,omitempty"`
	PlanID     string    `json:"planId,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	Remote     string    `json:"remote,omitempty"`
}

type auditLog struct {
	mu     sync.Mutex
	path   string
	recent []AuditEntry
}

func newAuditLog(path string) *auditLog {
	return &auditLog{path: path, recent: make([]AuditEntry, 0, 200)}
}

func (a *auditLog) append(entry AuditEntry) {
	entry.Time = time.Now().UTC()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.recent = append(a.recent, entry)
	if len(a.recent) > 400 {
		a.recent = append([]AuditEntry(nil), a.recent[len(a.recent)-400:]...)
	}
	if a.path == "" {
		return
	}
	file, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_ = tightenFileMode(a.path)
	encoder := json.NewEncoder(file)
	_ = encoder.Encode(entry)
}

func (a *auditLog) list(limit int) []AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	if limit <= 0 || limit > 400 {
		limit = 100
	}
	out := make([]AuditEntry, 0, limit)
	start := len(a.recent) - limit
	if start < 0 {
		start = 0
	}
	for i := len(a.recent) - 1; i >= start; i-- {
		out = append(out, a.recent[i])
	}
	return out
}

// loadAuditFromDisk restores recent entries after a restart.
func (a *auditLog) loadAuditFromDisk(limit int) {
	if a.path == "" || limit <= 0 {
		return
	}
	file, err := os.Open(a.path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	entries := make([]AuditEntry, 0, limit)
	for scanner.Scan() {
		var entry AuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
		if len(entries) > limit {
			entries = entries[1:]
		}
	}
	a.mu.Lock()
	a.recent = append(entries, a.recent...)
	a.mu.Unlock()
}
