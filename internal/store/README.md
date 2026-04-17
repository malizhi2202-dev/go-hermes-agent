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

轨迹设计补充：

- 普通 chat 轨迹当前不单独建表
- 直接复用 `sessions / messages`
- 再由 `internal/trajectory` 组装成 JSONL 训练前格式

这样做能保持轻量版 schema 简洁，同时兼顾批处理和导出场景。
