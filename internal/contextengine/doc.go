// Package contextengine handles history budgeting and context compression.
//
// Design role:
//   - history window trimming
//   - summary generation
//   - persisted context handoff
//
// Python lineage:
//   - agent/context_compressor.py
//
// Why it exists as a separate package:
// 上下文压缩是一个独立的工程问题。把它从聊天主链拆开，便于以后
// 从规则压缩升级到 LLM 压缩，而不污染其他模块。
package contextengine
