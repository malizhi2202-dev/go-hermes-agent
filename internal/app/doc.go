// Package app wires together the Go runtime's core subsystems.
//
// Design role:
//   - assemble config, auth, store, llm, memory, tools, extensions, execution
//   - provide chat, model, memory, and multiagent orchestration entrypoints
//   - keep cross-module coordination in one place
//
// Python lineage:
//   - run_agent.py
//   - parts of cli.py and model_tools.py
//
// Why it exists as a separate package:
// Python can tolerate business logic being spread across dynamic modules.
// The Go version aims to be easier to understand, so orchestration is
// intentionally centralized here.
package app
