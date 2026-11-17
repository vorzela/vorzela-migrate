package gui

import (
	"context"
	"fmt"
	"time"

	"github.com/vorzela/vorzela-migrate/v2/internal/version"
)

// UpdateInfo is the result of checking GitHub for a newer release.
type UpdateInfo struct {
	Current   string    `json:"current"`
	Latest    string    `json:"latest,omitempty"`
	Available bool      `json:"available"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

const updateCacheTTL = 10 * time.Minute

// checkForUpdate returns a cached or fresh update result. Network failures are
// reported in Error rather than failing the API call.
func (s *Server) checkForUpdate(force bool) UpdateInfo {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	if !force && s.updateInfo != nil && time.Since(s.updateAt) < updateCacheTTL {
		return *s.updateInfo
	}

	info := UpdateInfo{
		Current:   version.CurrentVersion,
		CheckedAt: time.Now().UTC(),
	}
	latest, available, err := version.CheckForUpdate()
	if err != nil {
		info.Error = err.Error()
	} else {
		info.Latest = latest
		info.Available = available
	}
	s.updateInfo = &info
	s.updateAt = time.Now()
	return info
}

func (s *Server) invalidateUpdateCache() {
	s.updateMu.Lock()
	s.updateInfo = nil
	s.updateAt = time.Time{}
	s.updateMu.Unlock()
}

func (s *Server) prepareSystemUpgrade(sessionID string) (*PublicPlan, error) {
	info := s.checkForUpdate(true)
	if info.Error != "" {
		return nil, fmt.Errorf("update check failed: %s", info.Error)
	}
	if !info.Available {
		return nil, fmt.Errorf("already on the latest version (%s)", info.Current)
	}
	plan := PublicPlan{
		Title:   "Upgrade vm",
		Summary: fmt.Sprintf("Download and install %s over the current vm binary (%s).", info.Latest, info.Current),
		Risk:    "medium",
		Command: vmCommand("upgrade"),
		Warnings: []string{
			"The running GUI keeps using the old binary until you restart `vm gui`.",
			"Restart `vm gui` after the upgrade so the new binary and embedded GUI assets are used.",
		},
	}
	return s.plans.create(sessionID, ActionSystemUpgrade, prepareRequest{Action: ActionSystemUpgrade}, func(time.Time) PublicPlan { return plan })
}

func (s *Server) executeSystemUpgrade(ctx context.Context) (ExecutionResult, error) {
	res, err := s.runner.Run(ctx, s.opts.ConfigDir, nil, "", "upgrade")
	if err != nil {
		return ExecutionResult{}, err
	}
	result := ExecutionResult{
		OK:         res.Success(),
		Action:     ActionSystemUpgrade,
		Message:    "Upgrade finished. Restart `vm gui` to run the new version.",
		Command:    vmCommand("upgrade"),
		ExitCode:   res.ExitCode,
		Stdout:     res.Stdout,
		Stderr:     res.Stderr,
		DurationMS: res.DurationMS,
	}
	if !res.Success() {
		return result, fmt.Errorf("vm upgrade failed (exit %d)", res.ExitCode)
	}
	s.invalidateUpdateCache()
	return result, nil
}
