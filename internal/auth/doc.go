// Package auth provides local-account authentication services.
//
// Design role:
//   - admin bootstrap
//   - password verification
//   - JWT issuance and parsing via internal/security
//
// Python lineage:
//   - local auth/config credential flows
//
// Why it exists as a separate package:
// 认证属于独立边界。把认证逻辑和安全原语分开，可以降低耦合并方便测试。
package auth
