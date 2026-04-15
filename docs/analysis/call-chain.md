# Python 文件与程序调用链梳理

## 主入口链 1：CLI 聊天

1. `hermes_cli/main.py`
2. 解析 profile / config / env
3. 进入 `cli.py`
4. CLI 命令触发 `run_agent.AIAgent`
5. `run_agent.py` 调用 `model_tools.get_tool_definitions()`
6. `model_tools.py` 通过 `tools.registry` 获取 tool schema
7. 模型返回 tool call 后，`handle_function_call()` 分发到 `tools/*.py`
8. 工具结果写回 messages
9. 继续模型循环，得到最终答复

## 主入口链 2：Messaging Gateway

1. `gateway/run.py`
2. 加载 config / env / platform adapters
3. 平台适配器接收消息事件
4. `gateway/session.py` 构造会话上下文
5. 调用 `run_agent.AIAgent`
6. Agent 走同一条 tool-calling loop
7. 最终回复由平台适配器发回外部平台

## 主入口链 3：ACP / 编辑器接入

1. `acp_adapter/entry.py`
2. 创建 session
3. 调用 Agent
4. 共用 `hermes_state.py` 存储会话

## 工具调用链

1. `model_tools._discover_tools()`
2. 导入 `tools/*.py`
3. 各工具在模块导入时执行 `registry.register(...)`
4. `registry.get_definitions()` 生成 schema
5. `registry.dispatch()` 执行 handler

## 状态存储链

1. CLI / Gateway / ACP 都会访问 `hermes_state.SessionDB`
2. Session 写入 `sessions`
3. Message 写入 `messages`
4. FTS5 触发器同步更新 `messages_fts`

## 配置与认证链

1. `hermes_cli/main.py` 先解析 profile
2. `hermes_constants.get_hermes_home()` 决定状态目录
3. `config.yaml` / `.env` / `auth.json` 协同加载
4. `hermes_cli/auth.py` 解析 provider runtime credential

## 高风险调用链

以下链路在 Python 中功能强，但 Go 迁移中必须谨慎：

- `tool call -> terminal_tool -> shell`
- `tool call -> code_execution_tool -> Python script`
- `tool call -> delegate_tool -> child agent`
- `tool call -> browser_tool -> external browser session`
- `plugin / mcp dynamic discovery -> runtime tool injection`

Go 版本当前不直接复刻这些链路，而是先迁移配置、认证、存储、LLM 边界和服务主干。
