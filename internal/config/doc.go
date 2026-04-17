// Package config defines the typed configuration model for go-hermes-agent.
//
// Design role:
//   - defaults
//   - load/save
//   - validation
//   - strongly typed runtime settings
//
// Python lineage:
//   - CLI config loading
//   - env/config merging
//
// Why it exists as a separate package:
// Go 版不希望业务层到处处理弱类型 map。配置统一收口成强类型结构，
// 可以提升可读性、可测试性和迁移稳定性。
package config
