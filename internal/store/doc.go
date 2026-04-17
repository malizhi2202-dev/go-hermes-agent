// Package store provides the SQLite persistence layer for go-hermes-agent.
//
// Design role:
//   - structured session/message storage
//   - audit and trace persistence
//   - search and aggregation queries
//
// Python lineage:
//   - hermes_state.py
//
// Why it exists as a separate package:
// Go 版强调“状态结构化入库”，这样 replay、resume、audit、search、
// extensions hooks 和 multiagent traces 都能建立在同一个持久层之上。
package store
