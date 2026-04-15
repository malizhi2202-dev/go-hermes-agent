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
  - child runtime 第一版：优先走 LLM，总失败时回退 stub
- 还缺：
  - 真正独立的 child agent 会话
  - 每个 child 的独立 history/context window
  - tool 级权限注入与真实 delegated tools
  - 父子 agent 结果回注主会话
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

- Slack 正式适配
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

1. 把多 Agent child runtime 从“LLM / stub”升级成真正独立 child session
2. 给 child runtime 加受限工具注入
3. 把 child 结果回注父会话链
4. 再继续 gateway 平台族和高动态能力
