package gui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Confirmation actions. The browser can only ask for these exact actions;
// there is no generic command endpoint.
const (
	ActionProjectAdd          = "project.add"
	ActionProjectInit         = "project.init"
	ActionProjectUpdate       = "project.update"
	ActionProjectRemove       = "project.remove"
	ActionConnectionSave      = "connection.save"
	ActionConnectionDelete    = "connection.delete"
	ActionMigrationCreate     = "migration.create"
	ActionMigrationSave       = "migration.save"
	ActionMigrationRevert     = "migration.revert"
	ActionMigrationApplyEdits = "migration.apply_edits"
	ActionColumnAdd           = "column.add"
	ActionMigrateRun          = "migrate.run"
	ActionRollbackRun         = "rollback.run"
	ActionFreshRun            = "fresh.run"
	ActionSystemUpgrade       = "system.upgrade"
)

// PlanPreview is optional content shown in the confirmation dialog.
type PlanPreview struct {
	Kind    string `json:"kind"` // sql, text, diff
	Title   string `json:"title,omitempty"`
	Content string `json:"content,omitempty"`
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
}

// PublicPlan is the only data the browser receives before a mutation.
type PublicPlan struct {
	ID                        string        `json:"id"`
	Action                    string        `json:"action"`
	Title                     string        `json:"title"`
	Summary                   string        `json:"summary"`
	Risk                      string        `json:"risk"` // low, medium, high, critical
	RequiresTypedConfirmation bool          `json:"requiresTypedConfirmation"`
	TypingPhrase              string        `json:"typingPhrase,omitempty"`
	Command                   string        `json:"command,omitempty"`
	Target                    PlanTarget    `json:"target"`
	Warnings                  []string      `json:"warnings,omitempty"`
	Preview                   *PlanPreview  `json:"preview,omitempty"`
	Analysis                  *EditAnalysis `json:"analysis,omitempty"`
	ExpiresAt                 time.Time     `json:"expiresAt"`
	Destructive               bool          `json:"destructive"`
}

// PlanTarget names what will be affected. It never contains a password.
type PlanTarget struct {
	ProjectID      string `json:"projectId,omitempty"`
	ProjectName    string `json:"projectName,omitempty"`
	ProjectPath    string `json:"projectPath,omitempty"`
	ConnectionID   string `json:"connectionId,omitempty"`
	ConnectionName string `json:"connectionName,omitempty"`
	Database       string `json:"database,omitempty"`
	File           string `json:"file,omitempty"`
}

type preparedPlan struct {
	PublicPlan
	SessionID  string
	Payload    json.RawMessage
	PayloadSum string
	CreatedAt  time.Time
	used       bool
}

type planStore struct {
	mu    sync.Mutex
	plans map[string]*preparedPlan
	ttl   time.Duration
}

func newPlanStore(ttl time.Duration) *planStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &planStore{plans: map[string]*preparedPlan{}, ttl: ttl}
}

func (s *planStore) create(sessionID, action string, payload any, build func(expires time.Time) PublicPlan) (*PublicPlan, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode plan payload: %w", err)
	}
	sum := sha256.Sum256(raw)
	expires := time.Now().UTC().Add(s.ttl)
	public := build(expires)
	public.ID = newID("plan")
	public.Action = action
	public.ExpiresAt = expires
	if public.Warnings == nil {
		public.Warnings = []string{}
	}
	plan := &preparedPlan{
		PublicPlan: public,
		SessionID:  sessionID,
		Payload:    raw,
		PayloadSum: hex.EncodeToString(sum[:]),
		CreatedAt:  time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	if len(s.plans) >= 200 {
		return nil, errors.New("too many pending confirmations; wait a moment and try again")
	}
	s.plans[plan.ID] = plan
	copy := public
	return &copy, nil
}

// peek returns a plan without consuming it.
func (s *planStore) peek(id, sessionID string) (*preparedPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	plan, ok := s.plans[id]
	if !ok || plan.used {
		return nil, errors.New("confirmation expired or already used")
	}
	if plan.SessionID != sessionID {
		return nil, errors.New("confirmation belongs to another session")
	}
	if time.Now().UTC().After(plan.ExpiresAt) {
		delete(s.plans, id)
		return nil, errors.New("confirmation expired")
	}
	return plan, nil
}

// take consumes a plan exactly once.
func (s *planStore) take(id, sessionID string) (*preparedPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	plan, ok := s.plans[id]
	if !ok {
		return nil, errors.New("confirmation expired or already used")
	}
	if plan.used {
		delete(s.plans, id)
		return nil, errors.New("confirmation was already used")
	}
	if plan.SessionID != sessionID {
		return nil, errors.New("confirmation belongs to another session")
	}
	if time.Now().UTC().After(plan.ExpiresAt) {
		delete(s.plans, id)
		return nil, errors.New("confirmation expired")
	}
	plan.used = true
	delete(s.plans, id)
	return plan, nil
}

func (s *planStore) cleanupLocked() {
	now := time.Now().UTC()
	for id, plan := range s.plans {
		if plan.used || now.After(plan.ExpiresAt) {
			delete(s.plans, id)
		}
	}
}
