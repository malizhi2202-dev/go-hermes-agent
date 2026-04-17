// Package memory implements the lightweight long-term memory layer.
//
// Design role:
//   - MEMORY.md / USER.md storage
//   - recalled memory reads
//   - memory write operations
//
// Python lineage:
//   - memory manager / memory prompt injection concepts
//
// Why it exists as a separate package:
// 轻量版优先采用可读、可调试、可恢复的文件记忆，而不是一开始就引入
// 更复杂的向量或外部 provider 体系。
package memory
