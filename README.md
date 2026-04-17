# Hermes Agent Go


落地一个更安全、可安装、可审计、便于继续演进的主干：

- 标准 Go 目录结构：`cmd/`、`internal/`、`configs/`、`docs/`、`scripts/`
- 本地账号与安全登录
- JWT 会话鉴权
- SQLite 状态存储
- Session / History / Search
- 内建文件记忆与回忆注入
- 受限版 Tool Registry
- 受控动态扩展面：plugin / skill script / MCP
- 受控 execution profile / extension lifecycle hook
- 基础 Webhook / Telegram / Slack / Weixin Gateway 适配
- OpenAI 兼容 LLM 调用边界
- 主流模型 / 本地模型 profile 切换
- 轻量 cron / scheduler
- HTTP API 服务
- 安装与卸载脚本

当前已接入的动态扩展能力：

- `plugin.yaml` 声明式插件发现与工具注册
- `SKILL.md + skill.yaml` 的 skill script 发现与受控执行
- `mcp_servers` 配置驱动的 stdio / HTTP MCP 工具发现与调用
- lifecycle hooks：`validate / on_enable / on_disable`
- `/v1/extensions`、`/v1/extensions/refresh`、`/v1/extensions/state` 扩展查询/刷新/启停接口
- `/v1/extensions/validate` 扩展验证接口
- `/v1/extensions/hooks` 扩展 hook 结果视图
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
- `GET /v1/multiagent/traces`
- `GET /v1/multiagent/traces/summary`
- `GET /v1/multiagent/traces/failures`
- `GET /v1/multiagent/traces/hotspots`
- `GET /v1/multiagent/replay`
- `POST /v1/multiagent/resume`
- 工具：`memory.read`、`memory.write`

多 Agent 运行说明：

- `run` 当前会优先尝试受控的 LLM child runtime
- 如果当前模型缺少可用 API key，或者本地模型端点不可用，会安全回退到 stub runtime
- 每个 child task 现在会写入独立 child session，并通过 parent session 串联 aggregate 回写
- child runtime 现在支持受控多轮循环，并可在 `allowed_tools` 白名单里真实调用安全工具
- child runtime 现在优先尝试原生 tool-calling，并在不支持时回退到 JSON 协议
- `run` 返回的每个 child result 现在会附带结构化 `trace`
- `trace` 也已经持久化到 `multiagent_traces`，可通过 `/v1/multiagent/traces` 查询
- `trace` 现在包含 child loop `snapshot`、delegated tool `verifier` 和 `verification_class` 信息
- `/v1/multiagent/traces/summary` 可以看工具级聚合和失败统计
- `/v1/multiagent/traces/verifiers` 可以看 verifier 成功/失败分类聚合
- `/v1/multiagent/traces/failures` 可以只看失败轨迹
- `/v1/multiagent/traces/hotspots` 可以看 child/task 维度的失败热点
- 上面这些 traces 视图都支持 `from/to` 的 RFC3339 时间过滤
- `GET /v1/multiagent/replay` 可以按 `child_session_id` 回放 child session、trace 和恢复提示
- `POST /v1/multiagent/resume` 现在会按最后成功/失败 trace step 生成恢复依据，并优先回放 snapshot 中保存的精确 loop history、next iteration、runtime 模式和累计 tool risks，再把成功 tool state 喂回 child loop
- `webhook / telegram / slack` 现在支持 `/multiagent ...` 命令路由
- Slack 现在支持 `slash command`、`url_verification`、`event_callback`、事件去重和 `chat.postMessage` 回复
- Gateway 现在通过统一 `PlatformAdapter` 契约注册平台路由，便于后续个人继续增加平台
- Weixin 现在支持轻量 public-account 风格的签名握手、文本消息路由和 XML 回复
- execution 现在支持配置驱动的 `profile -> steps` 受控执行链，可通过 `system.exec_profile` 调用
- `system.exec_profile` 已可进入 child delegated runtime，并支持 approval / capability token / rollback profile
- `/v1/audit/execution/profiles` 现在提供 `exec_profile` 专门的记录、action 聚合和 profile 聚合视图
- 这意味着 Go 版已经有“真实 child runtime 第一版”，但还没开放递归子代理和高风险 delegated runtime

已明确不实现的高风险能力：

- 任意终端命令执行
- 动态 Python 脚本执行
- 未经约束的插件热加载
- 自动浏览器执行链
- 弱校验的外部技能脚本直跑

这些能力如果后续确有必要实现，需要先经过白名单、沙箱、审批、审计等安全收口。

## 一键安装

```bash
bash scripts/install.sh
```

一键卸载：

```bash
bash scripts/uninstall.sh
```

彻底卸载（连配置和数据一起删）：

```bash
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
go mod tidy
go build -o bin/hermesd ./cmd/hermesd
go build -o bin/hermesctl ./cmd/hermesctl
./bin/hermesctl init-admin --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesctl chat --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesd --config ./configs/config.example.yaml
```

## 主要命令

```bash
./bin/hermesctl init-admin --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesctl login --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesctl chat --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesctl context --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --prompt 'summarize latest work'
./bin/hermesctl sessions --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesctl history --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --messages-limit 10
./bin/hermesctl search --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --q gateway
./bin/hermesctl audit --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesctl extensions --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesctl tools --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesctl multiagent-plan --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --objective 'inspect gateway' --tasks-file ./examples/multiagent/tasks.json
./bin/hermesctl multiagent-run --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --plan-file ./examples/multiagent/plan.json
./bin/hermesctl multiagent-replay --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --child-session-id 12
./bin/hermesctl batch-run --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --dataset-file ./examples/batch/prompts.jsonl --run-name demo
./bin/hermesctl batch-run --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --dataset-file ./examples/batch/prompts.jsonl --run-name demo --resume
./bin/hermesctl trajectories --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --limit 10 --run-name demo
./bin/hermesctl trajectory-summary --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --run-name demo
./bin/hermesctl cron-add --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!' --name daily-summary --prompt 'summarize the latest project state' --schedule 'every 2h'
./bin/hermesctl cron-list --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesctl cron-tick --config ./configs/config.example.yaml --username admin --password 'ChangeMe123!'
./bin/hermesd --config ./configs/config.example.yaml
```

`hermesctl` 现在不只是登录和对话入口，也覆盖了大部分原 Web/API 运维面：

- prompt 观测：`prompt-inspect` / `prompt-cache-stats` / `prompt-cache-clear` / `prompt-config`
- 辅助模型：`auxiliary-info` / `auxiliary-chat` / `auxiliary-switch`
- 模型元数据：`model-metadata`
- 上下文预算：`context`
- 会话与历史：`sessions` / `history` / `search`
- 审计与执行审计：`audit` / `execution-audit` / `execution-profile-audit`
- 扩展治理：`extensions` / `extension-hooks` / `extension-refresh` / `extension-state` / `extension-validate`
- 工具治理：`tools` / `tool-exec`
- 多 Agent 运维：`multiagent-plan` / `multiagent-run` / `multiagent-traces` / `multiagent-summary` / `multiagent-verifiers` / `multiagent-failures` / `multiagent-hotspots` / `multiagent-replay` / `multiagent-resume`
- 批处理与轨迹：`batch-run` / `trajectories` / `trajectory-summary` / `trajectory-show`
- 定时任务：`cron-add` / `cron-list` / `cron-show` / `cron-delete` / `cron-tick`

## 目录说明

- `cmd/hermesd`：服务端入口
- `cmd/hermesctl`：管理与登录 CLI
- `internal/api/README.md`：HTTP API 入口说明
- `internal/app/README.md`：应用装配与业务协调说明
- `internal/auth/README.md`：本地认证说明
- `internal/config/README.md`：配置模型说明
- `internal/config`：配置加载
- `internal/store`：SQLite 存储
- `internal/auth`：账号、登录、JWT
- `internal/security`：密码哈希与令牌工具
- `internal/security/README.md`：安全基础组件说明
- `internal/llm`：OpenAI 兼容 LLM 边界
- `internal/llm/README.md`：LLM 调用和原生 tool-calling 说明
- `internal/memory`：文件记忆与 recalled memory 注入
- `internal/prompting`：prompt builder / prompt cache 轻量实现
- `internal/trajectory`：chat 轨迹 JSONL 存储与导出
- `internal/batch`：轻量批处理运行器
- `internal/cron`：轻量单机定时任务与调度器
- `internal/memory/README.md`：记忆系统说明
- `internal/contextengine`：规则型上下文压缩与后续 LLM compressor 扩展点
- `internal/contextengine/README.md`：上下文压缩说明
- `internal/multiagent`：多 Agent 计划、权限策略、调度编排、结果汇总骨架
- `internal/multiagent/README.md`：多 Agent 主干说明
- `internal/api`：HTTP API
- `internal/tools`：受限白名单工具注册
- `internal/tools/README.md`：工具注册说明
- `internal/extensions`：plugin / skill / MCP 扩展发现、注册与受控执行
- `internal/extensions/README.md`：扩展面说明
- `internal/gateway`：基础 webhook / telegram gateway 适配
- 当前也包含 Slack slash command / events 适配，以及 Weixin 文本 webhook 适配
- `internal/gateway/README.md`：gateway 适配与 `/multiagent` 路由说明
- `internal/execution`：受控版高风险动态执行边界
- `internal/execution/README.md`：受控执行说明
- `internal/models/README.md`：模型目录与发现说明
- `internal/app`：应用装配
- `internal/store/README.md`：SQLite 状态与轨迹持久化说明
- `internal/version/README.md`：版本说明


## 当前实现定位

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

- [Go Hermes-Agent 完整总文档](./docs/delivery/go-complete-architecture-and-optimization.md)
- [总体架构与时序](./docs/architecture/overall-architecture-and-sequences.md)
- [模块架构与时序](./docs/architecture/module-architecture-and-sequences.md)
- [Python Agent 知识映射](./docs/architecture/python-agent-knowledge-map.md)
- [轻量版迁移蓝图](./docs/migration/lightweight-migration-blueprint.md)
