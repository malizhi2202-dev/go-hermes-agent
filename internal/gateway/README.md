# internal/gateway

这个目录负责把外部消息平台接进 Go 主干。

当前已实现：

- `webhook.go`：通用 webhook 入口
- `telegram.go`：Telegram webhook 适配
- `slack.go`：Slack slash command 适配
- `commands.go`：gateway 命令解析

当前能力：

- 鉴权与 webhook 校验
- Telegram update 去重
- Telegram 发送失败重试
- Slack 签名校验
- chat/user 维度最小会话隔离
- `/multiagent ...` 命令路由到多 Agent 执行链

设计原则：

- gateway 只负责适配和路由
- 具体业务执行落到 `internal/app`
- 高风险能力不直接在 gateway 层开放
