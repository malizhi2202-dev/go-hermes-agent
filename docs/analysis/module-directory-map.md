# Python 目录与功能模块梳理

## 顶层关键文件

- `run_agent.py`：Agent 主循环与工具调用驱动
- `model_tools.py`：工具发现、解析、分发
- `toolsets.py`：工具集合定义
- `cli.py`：交互式 CLI
- `hermes_state.py`：SQLite 会话数据库
- `batch_runner.py`：批量任务运行

## `agent/`

Agent 内核组件：

- prompt builder
- context compressor
- prompt caching
- auxiliary client
- model metadata
- display
- trajectory
- memory manager

这是 `run_agent.py` 的支撑层。

## `hermes_cli/`

CLI 与本地配置编排：

- `main.py`：`hermes` 入口
- `config.py`：默认配置与迁移
- `commands.py`：slash command 注册中心
- `auth.py`：provider 认证
- `model_switch.py`：模型切换
- `setup.py`：安装向导
- `skin_engine.py`：终端视觉主题

## `tools/`

工具实现层，一工具一文件：

- `registry.py`：统一注册中心
- `terminal_tool.py`：终端执行
- `file_tools.py`：文件操作
- `web_tools.py`：网络搜索/抽取
- `browser_tool.py`：浏览器自动化
- `code_execution_tool.py`：脚本执行
- `delegate_tool.py`：子代理委派
- `mcp_tool.py`：MCP 客户端

这是 Python 项目最复杂、最动态的模块群。

## `gateway/`

消息平台网关：

- `run.py`：主循环
- `session.py`：网关会话封装
- `platforms/*`：多平台适配器

## `acp_adapter/`

编辑器协议适配层，供 VS Code / Zed / JetBrains 等工具接入。

## `cron/`

任务调度：

- `jobs.py`
- `scheduler.py`

## `environments/`

RL、benchmark、研究环境与工具调用解析器。

## `plugins/`

插件扩展，当前主要体现为 memory provider 等。

## `skills/` 与 `optional-skills/`

技能文档、脚本模板、迁移工具与参考资料。

## `tests/`

大规模 pytest 套件，覆盖 CLI、Gateway、Tool、Agent、Plugin 等多个层次。

## Go 版本模块映射

| Python 区域 | Go 目标 |
| --- | --- |
| `run_agent.py` | `internal/app` + `internal/llm` + `internal/api` |
| `hermes_state.py` | `internal/store` |
| `hermes_cli/auth.py` | `internal/auth` + `internal/security` |
| `hermes_constants.py` | `internal/config` |
| `cli.py` / `hermes_cli/main.py` | `cmd/hermesctl` |
| `gateway/run.py` | `cmd/hermesd` + `internal/api` |
| 动态工具系统 | 后续按白名单切片迁移 |
