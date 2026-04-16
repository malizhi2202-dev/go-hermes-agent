# internal/extensions

这个目录负责动态扩展面的受控接入。

当前支持：

- plugin.yaml
- SKILL.md + skill.yaml
- MCP stdio / HTTP server
- 扩展启停状态持久化

设计原则：

- 声明式发现优先
- 白名单工具注册优先
- 不直接照搬 Python 的高风险热加载

MCP 传输说明：

- `transport: "stdio"`：使用本地命令启动 MCP server
- `transport: "http"`：通过受控 HTTP JSON-RPC 连接远端或本地 MCP server
- `include_tools` / `exclude_tools` 继续生效
- 发现状态会在 `/v1/extensions` 的 MCP 摘要里体现 `transport / command / url`
