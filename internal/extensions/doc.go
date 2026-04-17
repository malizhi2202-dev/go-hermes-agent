// Package extensions manages the controlled dynamic extension surface.
//
// Design role:
//   - discover plugin, skill, and MCP definitions
//   - persist enable/disable state
//   - run lifecycle hooks under controlled execution
//
// Python lineage:
//   - skills/plugins ecosystem
//   - MCP integration
//
// Why it exists as a separate package:
// Python 原版更偏动态加载。Go 轻量版选择声明式发现 + 受控注册，
// 让扩展能力可治理、可审计、可测试。
package extensions
