# Python 项目架构与核心点梳理

## 总体定位

原仓库是一个大型多入口 AI Agent 平台，而不是单一 CLI 程序。它同时包含：

- 交互式 CLI
- 消息平台 Gateway
- ACP 编辑器接入
- 定时任务 Cron
- Web/API 服务
- Tool Registry 与 Toolset 编排
- Skills / Plugins / Memory / Session Store
- RL / Benchmark / Batch Runner

## 核心架构主线

### 1. Agent 主循环

核心在 `run_agent.py` 的 `AIAgent`。

主行为：

1. 组装 system/user/history messages
2. 解析可用工具定义
3. 调用模型 API
4. 如果模型返回工具调用，则交给工具分发器执行
5. 把工具结果继续喂给模型
6. 循环直到获得最终自然语言响应

这是一条典型的 “LLM + tool-calling loop”。

### 2. 工具编排层

核心在 `model_tools.py` 与 `tools/registry.py`。

职责：

- 导入各工具模块并触发注册
- 按 toolset / availability 解析可用工具
- 统一执行工具 handler
- 兼容同步/异步 handler

这里是 Python 版本最关键的扩展点之一。

### 3. CLI 编排层

核心在 `cli.py` 与 `hermes_cli/main.py`。

职责：

- 终端 UI
- slash commands
- config 加载
- profile 选择
- 模型与工具配置
- 调起 agent 会话

CLI 不只是一个简单入口，而是较重的交互编排层。

### 4. Gateway 平台层

核心在 `gateway/run.py` 与 `gateway/platforms/*`。

职责：

- Telegram / Discord / Slack / Signal / WhatsApp 等平台接入
- 消息事件转换
- 会话上下文恢复
- 与 agent 共享消息/会话存储

这是第二条大型运行主线。

### 5. 状态与会话存储

核心在 `hermes_state.py`。

特点：

- SQLite
- WAL 模式
- FTS5 全文搜索
- session/messages 分表
- 并发重试与 checkpoint

它是 CLI、Gateway、ACP 等多入口共享的数据底座。

### 6. 认证与配置层

核心在 `hermes_cli/auth.py`、`hermes_cli/config.py`、`hermes_constants.py`。

职责：

- 多 provider 认证
- OAuth / API key / 外部 process
- HERMES_HOME / profile 隔离
- config.yaml / .env / auth.json 协同

### 7. 高风险动态能力

原 Python 仓库包含多种高动态、高权限能力：

- terminal execution
- execute_code
- delegate_task
- browser automation
- plugin / MCP / skills 动态接入

这些能力对 Python 版本是功能亮点，但迁移到 Go 时也是最高风险区。

## Go 迁移核心结论

Go 版本不应逐文件等价复制，而应按运行边界重构成：

1. Config
2. Auth
3. State Store
4. API / CLI Entrypoint
5. LLM Boundary
6. Safe Tool Abstraction

其中动态执行链必须收口，不能直接照搬。
