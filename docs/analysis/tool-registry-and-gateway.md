# Go 迁移：受限 Tool Registry、动态扩展与基础 Gateway

## 受限 Tool Registry

已实现：

- 编译期注册，不做运行时动态发现
- 工具白名单
- 只允许安全工具
- 通过 `internal/extensions` 接入受控动态扩展

当前内置工具：

- `system.health`
- `session.history`
- `session.search`
- `system.exec`（默认关闭，显式白名单）

当前已接入的动态扩展工具：

- `plugin.<name>`：来自 `plugin.yaml` 的声明式插件工具
- `skill.<name>`：来自 `SKILL.md + skill.yaml` 的 skill script 工具
- `mcp.<server>.<tool>`：来自 `mcp_servers` 的 stdio MCP 工具

扩展治理能力：

- `extension_states` 表持久化 plugin / skill 启停状态
- `/v1/extensions/state` 可对扩展做启用/禁用
- 刷新时会先卸载旧扩展工具，再重新注册当前有效工具
- plugin / skill 会计算内容 hash；当数据库里的历史状态和当前文件内容不一致时，会标记为 `database-mismatch`

动态扩展的安全边界：

- 不做 Python 代码热加载
- 不做 `__init__.py register()` 这类运行时注入
- plugin / skill 只允许固定命令 + 参数模板执行
- 参数值会做长度和元字符拦截
- MCP 当前只接入 `stdio`，并通过配置控制 server 与 tool 范围

未迁移的高风险工具：

- shell / terminal
- code execution
- browser automation
- dynamic Python plugin hook injection

## 基础 Gateway 适配

已实现：

- `POST /gateway/webhook`
- 使用 `X-Gateway-Token` 校验
- 接收统一 JSON 消息
- 将消息路由到 Go 的 chat 主干
- `POST /gateway/telegram/webhook`
- Telegram webhook secret 校验
- Telegram Bot API `sendMessage` 回复
- Telegram update 去重
- Telegram 发送失败重试

示例：

```json
{
  "platform": "webhook",
  "user_id": "user-1",
  "username": "alice",
  "text": "hello"
}
```

## 为什么先先做 webhook / Telegram 这条线

这样能先把最重要的“平台消息 -> agent -> 回复”这条链打通，同时避免一开始就把 Telegram/Slack/Discord 的 SDK 和事件语义全部带进 Go 迁移里。
