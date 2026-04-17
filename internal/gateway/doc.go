// Package gateway adapts external messaging platforms to the Go runtime.
//
// Design role:
//   - webhook validation
//   - platform-specific parsing and reply sending
//   - command routing into app chat or multiagent flows
//
// Python lineage:
//   - gateway/run.py
//   - gateway/platforms/*
//
// Why it exists as a separate package:
// 平台接入需要存在，但轻量版不希望让平台逻辑污染业务主链。
// 所以 gateway 只负责适配和路由，核心执行统一交给 app。
package gateway
