// Package cron provides the lightweight single-node scheduler for the Go
// Hermes agent.
//
// Python parity reference:
//   - cron/jobs.py
//   - cron/scheduler.py
//
// Go keeps the slice intentionally smaller:
//   - JSON-backed job persistence under data_dir/cron/
//   - one-process ticker suitable for hermesd
//   - CLI-first job management
//   - local output persistence and audit logging
//
// This matches the lightweight edition goal: easy deployment, easy debugging,
// and no extra infrastructure requirements.
package cron
