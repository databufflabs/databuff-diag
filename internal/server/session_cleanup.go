package server

import (
	"context"
	"log"
	"time"

	"github.com/databufflabs/databuff-diag/internal/store"
)

// SessionCleanup purges expired diagnostic sessions on a schedule.
type SessionCleanup struct {
	Sessions *store.SessionStore
	Config   *store.ConfigStore
	Now      func() time.Time
	Location *time.Location
}

// NewSessionCleanup builds a cleanup runner with sensible defaults.
func NewSessionCleanup(cfg *store.ConfigStore, sessions *store.SessionStore) *SessionCleanup {
	return &SessionCleanup{
		Sessions: sessions,
		Config:   cfg,
		Now:      time.Now,
		Location: time.Local,
	}
}

func (c *SessionCleanup) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *SessionCleanup) location() *time.Location {
	if c.Location != nil {
		return c.Location
	}
	return time.Local
}

// Run removes sessions older than the configured retention window.
func (c *SessionCleanup) Run() (int, error) {
	cfg, err := c.Config.Load()
	if err != nil {
		return 0, err
	}
	if !cfg.Sessions.CleanupEnabled() {
		return 0, nil
	}

	days := cfg.Sessions.RetentionDaysOrDefault()
	cutoff := c.now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	purged, err := c.Sessions.PurgeBefore(cutoff)
	if err != nil {
		return 0, err
	}
	if len(purged) > 0 {
		log.Printf("session cleanup: removed %d session(s) older than %d days", len(purged), days)
	}
	return len(purged), nil
}

// Start runs cleanup immediately and schedules a daily job at the configured local hour.
func (c *SessionCleanup) Start(ctx context.Context) {
	c.runStartup()

	go func() {
		for {
			cfg, err := c.Config.Load()
			if err != nil {
				log.Printf("session cleanup: load config: %v", err)
				if !c.wait(ctx, time.Hour) {
					return
				}
				continue
			}
			if !cfg.Sessions.CleanupEnabled() {
				if !c.wait(ctx, time.Hour) {
					return
				}
				continue
			}

			next := nextDailyRun(c.now(), cfg.Sessions.CleanupHourLocal(), c.location())
			if !c.wait(ctx, time.Until(next)) {
				return
			}

			if _, err := c.Run(); err != nil {
				log.Printf("session cleanup: %v", err)
			}
		}
	}()
}

func (c *SessionCleanup) runStartup() {
	if _, err := c.Run(); err != nil {
		log.Printf("session cleanup on startup: %v", err)
	}
}

func (c *SessionCleanup) wait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		d = time.Second
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextDailyRun(now time.Time, hour int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	local := now.In(loc)
	candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, 0, 0, 0, loc)
	if !now.Before(candidate) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate
}
