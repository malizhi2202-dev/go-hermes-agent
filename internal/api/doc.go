// Package api exposes the authenticated HTTP boundary for go-hermes-agent.
//
// Design role:
//   - validate input
//   - enforce auth
//   - serialize responses
//   - keep business orchestration inside internal/app
//
// Python lineage:
//   - API server and gateway-facing HTTP entrypoints
//   - session/history/search/audit query surfaces
//
// Why it exists as a separate package:
// Go 轻量版强调“边界清晰”。HTTP 层不承担业务主脑，只负责把
// 外部请求转换成对 App 的显式调用。
package api
