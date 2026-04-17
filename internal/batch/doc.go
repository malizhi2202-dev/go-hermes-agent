// Package batch provides a lightweight sequential batch runner.
//
// It is the Go counterpart to Python's batch_runner.py, narrowed for the
// lightweight single-node runtime:
//   - load prompts from JSONL
//   - run them sequentially through the existing App chat pipeline
//   - save one trajectory per successful session
//   - report summary stats suitable for CLI automation
package batch
