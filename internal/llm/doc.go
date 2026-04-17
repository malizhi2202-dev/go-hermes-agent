// Package llm provides the OpenAI-compatible model boundary.
//
// Design role:
//   - chat/completions requests
//   - native tool-calling support
//   - provider response normalization
//
// Python lineage:
//   - provider chat client paths
//   - parts of auxiliary/model routing groundwork
//
// Why it exists as a separate package:
// LLM 调用边界要尽量统一。这样上层不需要关心 provider 差异，
// 同时方便以后扩展 prompt caching、auxiliary client 和 model metadata。
package llm
