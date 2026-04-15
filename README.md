# Hermes Agent Go

`go/` 是对原 Python Hermes Agent 的安全化、增量式 Go 迁移工作区。

当前版本不是对 Python 项目的逐文件机械翻译，而是先落地一个更安全、可安装、可审计、便于继续演进的主干：

- 标准 Go 目录结构：`cmd/`、`internal/`、`configs/`、`docs/`、`scripts/`
- 本地账号与安全登录
- JWT 会话鉴权
- SQLite 状态存储
- Session / History / Search
- 内建文件记忆与回忆注入
- 受限版 Tool Registry
- 受控动态扩展面：plugin / skill script / MCP
- 基础 Webhook / Telegram Gateway 适配
- OpenAI 兼容 LLM 调用边界
- 主流模型 / 本地模型 profile 切换
- HTTP API 服务
- 安装与卸载脚本

当前已接入的动态扩展能力：

- `plugin.yaml` 声明式插件发现与工具注册
- `SKILL.md + skill.yaml` 的 skill script 发现与受控执行
- `mcp_servers` 配置驱动的 stdio MCP 工具发现与调用
- `/v1/extensions`、`/v1/extensions/refresh`、`/v1/extensions/state` 扩展查询/刷新/启停接口
- 插件与 skill 的 SQLite 启停状态持久化
- 基于内容 hash 的基础完整性标记，检测“数据库状态对应的扩展内容已变化”

当前已接入的模型能力：

- 通过 `model_profiles` 管理多模型档案
- 支持远端 OpenAI-compatible provider
- 支持本地 OpenAI-compatible 服务
- 预置 `OpenAI / OpenRouter / Ollama / LM Studio` 示例 profile
- 支持常用模型 alias，例如 `sonnet / claude / gpt / qwen / ollama / lmstudio`
- 支持本地模型自动发现（Ollama / LM Studio）
- `GET /v1/models` 查看当前模型与全部 profile
- `GET /v1/models/discover` 自动发现本地模型
- `POST /v1/models/switch` 运行时切换模型 profile
- `hermesctl models`
- `hermesctl discover-models`
- `hermesctl switch-model --profile <name>`

当前已接入的记忆能力：

- 每个用户独立的 `MEMORY.md / USER.md` 文件记忆
- 聊天前按关键词做基础 recalled memory 注入
- 最近 N 条历史消息窗口注入
- 基础 context budget 估算
- 规则型 context compressor 骨架
- 多 Agent 主干骨架：planner / policy / orchestrator / aggregator
- `GET /v1/memory`
- `POST /v1/memory`
- `GET /v1/context`
- `POST /v1/multiagent/plan`
- `POST /v1/multiagent/run`
- 工具：`memory.read`、`memory.write`

多 Agent 运行说明：

- `run` 当前会优先尝试受控的 LLM child runtime
- 如果当前模型缺少可用 API key，或者本地模型端点不可用，会安全回退到 stub runtime
- 每个 child task 现在会写入独立 child session，并通过 parent session 串联 aggregate 回写
- 这意味着 Go 版已经有“真实 child runtime 第一版”，但还没开放真实 delegated tools 执行

已明确不直接迁移的高风险能力：

- 任意终端命令执行
- 动态 Python 脚本执行
- 未经约束的插件热加载
- 自动浏览器执行链
- 弱校验的外部技能脚本直跑

这些能力如果后续确有必要迁移，需要先经过白名单、沙箱、审批、审计等安全收口。

## 一键安装

```bash
cd go
bash scripts/install.sh
```

一键卸载：

```bash
cd go
bash scripts/uninstall.sh
```

彻底卸载（连配置和数据一起删）：

```bash
cd go
bash scripts/uninstall.sh --purge
```

安装脚本会自动完成这些事：

- 编译 `hermesd` 和 `hermesctl`
- 安装到 `~/.hermes-go/bin`
- 在 `~/.local/bin` 建立命令入口
- 自动生成 `~/.hermes-go/configs/config.yaml`
- 自动把 `data_dir / plugins_dir / skills_dirs` 改成绝对路径
- 生成 `~/.hermes-go/QUICKSTART.txt`

## 零门槛启动

安装完成后，照着下面 2 条命令做就能跑起来：

```bash
hermesctl init-admin --config ~/.hermes-go/configs/config.yaml --username admin --password 'ChangeMe123!'
hermesd --config ~/.hermes-go/configs/config.yaml
```

查看和切换模型：

```bash
hermesctl models --config ~/.hermes-go/configs/config.yaml
hermesctl discover-models --config ~/.hermes-go/configs/config.yaml
hermesctl switch-model --config ~/.hermes-go/configs/config.yaml --profile ollama-qwen3
hermesctl switch-model --config ~/.hermes-go/configs/config.yaml --profile sonnet
hermesctl switch-model --config ~/.hermes-go/configs/config.yaml --model deepseek-r1:8b --base-url http://127.0.0.1:11434/v1 --local
```

记忆读写示例：

```bash
curl -H "Authorization: Bearer <token>" http://127.0.0.1:8080/v1/memory
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"target":"memory","action":"add","content":"Project uses Telegram gateway"}' \
  http://127.0.0.1:8080/v1/memory
```

上下文预算查看示例：

```bash
curl -H "Authorization: Bearer <token>" \
  "http://127.0.0.1:8080/v1/context?prompt=Summarize+the+latest+session"
```

其中：

- `context.history_window_messages` 控制聊天前最多带入多少条最近历史消息
- `context.max_prompt_chars` 控制系统块、历史和本轮 prompt 的总字符预算
- `context.compression_enabled` 控制是否启用规则型上下文压缩
- `context.compress_threshold_messages` 控制达到多少条历史消息后开始压缩
- `context.protect_last_messages` 控制压缩时保留多少条最近消息
- `context.summary_max_chars` 控制压缩摘要块最大长度
- `context.summary_strategy` 当前支持 `rule`，并为后续 `llm` 摘要策略预留了扩展位
- `POST /v1/chat` 响应现在会附带 `context` 字段，便于观察本轮预算使用情况

## 快速开始

```bash
cd go
go mod tidy
go build -o bin/hermesd ./cmd/hermesd
go build -o bin/hermesctl ./cmd/hermesctl
./bin/hermesctl init-admin --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesd --config ./configs/config.example.yaml
```

## 主要命令

```bash
./bin/hermesctl init-admin --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesctl login --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesd --config ./configs/config.example.yaml
```

## 目录说明

- `cmd/hermesd`：服务端入口
- `cmd/hermesctl`：管理与登录 CLI
- `internal/config`：配置加载
- `internal/store`：SQLite 存储
- `internal/auth`：账号、登录、JWT
- `internal/security`：密码哈希与令牌工具
- `internal/llm`：OpenAI 兼容 LLM 边界
- `internal/memory`：文件记忆与 recalled memory 注入
- `internal/contextengine`：规则型上下文压缩与后续 LLM compressor 扩展点
- `internal/multiagent`：多 Agent 计划、权限策略、调度编排、结果汇总骨架
- `internal/api`：HTTP API
- `internal/tools`：受限白名单工具注册
- `internal/extensions`：plugin / skill / MCP 扩展发现、注册与受控执行
- `internal/gateway`：基础 webhook / telegram gateway 适配
- `internal/execution`：受控版高风险动态执行边界
- `internal/app`：应用装配
- `docs/`：Python 梳理、迁移映射、安全清理说明

## 当前迁移定位

当前 Go 版本属于“安全主干版”。它优先承接：

1. 配置
2. 鉴权
3. 存储
4. 服务启动
5. LLM 接口边界
6. Session / History / Search
7. Safe Tool Abstraction
8. Basic Gateway Webhook
9. 会话审计与安装部署

复杂浏览器自动化、未经白名单约束的代码执行、热插拔插件生命周期钩子等能力，仍保留在“后续分切片迁移”范围。

## Go 文档导航

- [Go 版本架构总结](/home/malizhi/project/go-hermes-agent/docs/architecture/go-architecture-summary.md)
- [Go 版本执行流程图](/home/malizhi/project/go-hermes-agent/docs/architecture/go-execution-flow.md)
- [Go 版本设计图](/home/malizhi/project/go-hermes-agent/docs/architecture/go-design-diagram.md)
- [Agent 常见问题与 Hermes 工程对应拆解](/home/malizhi/project/go-hermes-agent/docs/architecture/agent-concepts-vs-hermes.md)
- [Go 迁移差距清单](/home/malizhi/project/go-hermes-agent/docs/migration/go-gap-checklist.md)
