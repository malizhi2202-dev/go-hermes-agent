# Go Gap Checklist

这份文档用来对照 Python Hermes 和当前 Go 迁移版，说明：

- 已迁移到可用状态的能力
- 已有骨架但还没完全等价的能力
- 仍未迁移、后续需要继续填充的能力

## 已迁移到可用状态

- 配置、服务启动、安装卸载
- 本地账号、安全登录、JWT
- SQLite 会话、消息、审计
- FTS 搜索与过滤
- Telegram 基础 gateway
- 受限工具注册与安全执行边界
- plugin / skill / MCP 受控接入
- 模型 profiles、alias、本地模型发现
- 文件记忆与 recalled memory 注入
- 上下文窗口、规则压缩器、摘要持久化
- 多 Agent 的 plan / policy / orchestrator / aggregate
- `POST /v1/multiagent/plan`
- `POST /v1/multiagent/run`
- `GET /v1/multiagent/traces`
- `GET /v1/multiagent/traces/summary`
- `GET /v1/multiagent/traces/failures`
- `GET /v1/multiagent/traces/hotspots`
- `GET /v1/multiagent/replay`
- `POST /v1/multiagent/resume`

## 已有骨架但还没完全等价

### 上下文压缩

- Go 已有：
  - 规则型摘要
  - 摘要持久化
  - 可选 `summary_strategy=llm`
- 还缺：
  - Python 那种 token 预算驱动压缩
  - tool output pruning
  - 迭代式 summary merge 策略细化
  - 独立辅助压缩模型配置

### 多 Agent

- Go 已有：
  - 任务计划
  - 并发/串行判定
  - blocked tools 校验
  - 写入范围冲突校验
  - API plan/run 入口
- parent / child session 串联
- child trace 持久化
- replay / resume
- trace summary / failures / hotspots
- child runtime 第一版：优先走 LLM，总失败时回退 stub
- child runtime 第一版已支持受控多轮循环、安全工具白名单执行、原生 tool-calling 优先和 seed history 恢复
- 还缺：
- 更完整的 child loop state 恢复仍可继续增强；当前已能恢复最近 assistant/tool 历史和成功 tool state，但还没有完整状态快照
- 更完整的 tool 级权限注入与更多真实 delegated tools
- 更细粒度的父子 agent 结构化结果回注主会话
- 递归深度控制与更细粒度审计

### 记忆

- Go 已有：
  - 文件记忆
  - 读写 API
  - recalled memory 注入
- 还缺：
  - Python memory provider manager 的完整生态
  - 外部 memory provider 插件化
  - 自动 memory review / flush

## 仍未迁移的重点能力

### Python AIAgent 主循环等价能力

- 完整 tool-calling 多轮主循环
- reasoning / tool call 轨迹语义
- 更完整的 prompt builder / prompt caching

### Gateway 平台族

- Slack 已有 slash command + events 第一版
- Discord / WhatsApp / Signal / Home Assistant 等平台
- 平台特有审批 / 交互控件

### 高动态能力

- browser automation
- 更完整的 terminal environment backend
- 真正的 delegate child agent runtime
- 更完整的 plugin lifecycle / hook

### 生态兼容

- Python `hermes_cli` 更完整的命令体系
- `/model` 更完整的切换体验
- 技能创建/审核工作流
- 背景 review / trajectory / training 相关能力

## 建议的后续迁移顺序

1. 把多 Agent 恢复链从“恢复最后成功 tool state”升级成更完整的 child loop state 恢复
2. 继续补 delegated tools 与结构化结果回注
3. 再继续 gateway 平台族
4. 最后处理高动态能力
