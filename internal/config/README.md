# internal/config

这个目录负责 Go 版本的配置模型、默认值、读写和校验。

当前涵盖：

- LLM profile
- memory / context
- gateway / telegram
- execution
- extensions
- MCP servers

设计原则：

- 所有默认值集中在这里
- 对外暴露强类型配置，不把字符串字典传到业务层
