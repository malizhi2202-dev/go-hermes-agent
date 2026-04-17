// Package tools implements the allowlisted tool registry used by the Go runtime.
//
// Design role:
//   - register built-in and extension tools
//   - expose structured schemas
//   - execute tools for app and child runtimes
//
// Python lineage:
//   - tools/registry.py
//   - model_tools.py
//
// Why it exists as a separate package:
// 工具就是 Agent 的能力边界。Go 版选择显式注册和显式 schema，
// 以换取更好的审计、权限控制和 child runtime 可恢复性。
package tools
