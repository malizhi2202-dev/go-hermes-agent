// Package execution implements the controlled dynamic execution boundary.
//
// Design role:
//   - allowlisted command execution
//   - profile-based execution chains
//   - approval, capability token, and rollback handling
//
// Python lineage:
//   - terminal/code execution governance ideas
//   - approval-aware execution surfaces
//
// Why it exists as a separate package:
// 执行链是风险最高的能力之一。Go 版把它独立出来，明确约束边界，
// 避免“文本生成直接变成任意系统执行”。
package execution
