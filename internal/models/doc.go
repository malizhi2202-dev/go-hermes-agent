// Package models manages model catalog data, aliases, and local discovery.
//
// Design role:
//   - resolve human-friendly aliases
//   - discover local model runtimes
//   - support profile-oriented model switching
//
// Python lineage:
//   - hermes_cli/models.py
//   - model switch workflow
//   - future models.dev style metadata work
//
// Why it exists as a separate package:
// 模型选择不应散落在 CLI、API、LLM 客户端各处。单独模块更适合承接
// 后续的 metadata、provider-aware context 和 auxiliary model 能力。
package models
