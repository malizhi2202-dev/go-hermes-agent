# internal/store

这个目录封装 Go 版本的 SQLite 持久化层。

当前负责保存：

- 用户账号
- sessions / messages
- audit_log
- extension_states
- context_summaries
- processed_gateway_updates
- multiagent_traces

核心能力：

- 聊天会话与消息落库
- FTS 搜索
- 审计查询
- gateway 去重
- context summary 持久化
- child trace 持久化与回放查询
- child trace 聚合统计与失败分析

设计原则：

- 先把状态结构化入库，再由上层做恢复、审计和编排
- 所有高价值轨迹尽量单独成表，不只塞进自然语言消息
