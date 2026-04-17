// Package security contains low-level security primitives.
//
// Design role:
//   - password hashing
//   - JWT helpers
//   - secret generation
//
// Why it exists as a separate package:
// 安全原语应保持可替换、可单测，不和认证业务逻辑混在一起。
package security
