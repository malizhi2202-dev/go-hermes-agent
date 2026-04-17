// Package trajectory provides lightweight chat trajectory persistence.
//
// It is the Go counterpart to Python's agent/trajectory.py, but narrowed for
// the single-node lightweight runtime:
//   - build a structured transcript from one stored chat session
//   - append JSONL trajectory records under the runtime data directory
//   - list, read, and export recent trajectory records for batch and debugging
//
// The package intentionally uses filesystem JSONL storage instead of a more
// complex indexing layer to keep deployment and inspection simple.
package trajectory
