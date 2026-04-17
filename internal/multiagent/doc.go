// Package multiagent provides plan, policy, orchestration, aggregation, and
// trace types for delegated child-agent execution.
//
// Python lineage:
//   - tools/delegate_tool.py
//
// Why it exists as a separate package:
// Go 版把多 Agent 设计成显式 plan/policy/orchestrator/aggregator，
// 目的是让 delegated execution 更容易恢复、审计和测试，而不是只追求动态自由度。
package multiagent
