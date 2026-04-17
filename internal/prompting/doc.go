// Package prompting assembles chat prompts for the lightweight Go runtime.
//
// It is the Go counterpart to the Python prompt_builder + prompt_caching line,
// but intentionally narrower:
//   - collect memory, persisted summary, and recent history
//   - apply context compression when needed
//   - trim history to the configured prompt budget
//   - keep a lightweight local cache of assembled prompt plans
//
// The design stays explicit and dependency-light so the prompt path remains
// easy to inspect and reason about in a single-process deployment.
package prompting
