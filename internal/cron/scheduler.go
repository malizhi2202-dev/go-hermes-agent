package cron

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"go-hermes-agent/internal/app"
)

// Scheduler drives periodic cron ticks for hermesd.
type Scheduler struct {
	manager  *Manager
	app      *app.App
	interval time.Duration
	running  atomic.Bool
}

// NewScheduler creates a scheduler with one fixed tick interval.
func NewScheduler(manager *Manager, application *app.App, interval time.Duration) *Scheduler {
	return &Scheduler{
		manager:  manager,
		app:      application,
		interval: interval,
	}
}

// Start begins the background ticker until the context is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	if s == nil || s.manager == nil || s.app == nil || s.interval <= 0 {
		return
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	if !s.running.CompareAndSwap(false, true) {
		return
	}
	defer s.running.Store(false)
	results, err := s.manager.Tick(ctx, s.app)
	if err != nil {
		log.Printf("cron tick error: %v", err)
		return
	}
	if len(results) > 0 {
		log.Printf("cron tick ran %d job(s)", len(results))
	}
}
