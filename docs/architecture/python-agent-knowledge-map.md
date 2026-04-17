# Python Agent 知识点映射与 Go 设计原因

本文记录 Python 原版 Hermes Agent 中值得保留的知识点，并说明：

- Python 在哪里实现
- 它解决什么问题
- Go 版当前如何承接
- 为什么 Go 版这样设计

## 1. Agent 主循环

### Python 知识点

- `run_agent.py` 里的 `AIAgent`
- 同步 conversation loop
- assistant/tool 消息交替推进
- tool result 继续喂回模型

### 设计目的

- 把“推理”和“行动”放进同一个循环
- 保证每轮都能利用最新工具结果

### Go 承接

- `internal/app`
- `internal/llm`
- `internal/multiagent` child loop

### 为什么这样设计

- Go 版先保留“消息循环 + tool calling”的主骨架
- 再把复杂的动态策略后置

## 2. Prompt Builder

### Python 知识点

- `agent/prompt_builder.py`
- 系统身份、上下文文件、memory、skills、平台上下文拼装

### 设计目的

- 不同来源的上下文都能稳定拼到系统提示里

### Go 承接

- 当前散落在 `internal/app`、`internal/memory`、`internal/contextengine`
- 这是后续要补成独立模块的重要迁移点

### 为什么当前还没独立

- Go 版先做轻量主干，避免一开始引入过大的 prompt DSL

## 3. Prompt Caching

### Python 知识点

- `agent/prompt_caching.py`
- 缓存稳定前缀
- 尽量避免对系统提示和历史做破坏性重建

### 设计目的

- 降低成本
- 提高长对话稳定性

### Go 承接

- 当前只有“避免频繁重建上下文”的设计倾向
- 还没有独立 prompt caching 模块

### 后续方向

- 迁成 `internal/promptengine` 或 `internal/prompt`
- 给 summary / memory / context files 定义稳定区块

## 4. Auxiliary Client

### Python 知识点

- `agent/auxiliary_client.py`
- 让摘要、视觉、辅助任务使用独立模型或 provider

### 设计目的

- 主模型负责主对话
- 辅助模型负责压缩、摘要、视觉等副任务

### Go 承接

- 当前 LLM 仍以主模型为主
- `context summary_strategy = llm` 已留扩展位

### 后续方向

- 增加 `auxiliary_profiles`
- 把 summary / discover / classify 一类任务迁到辅助模型

## 5. Models.dev / 模型元数据

### Python 知识点

- `agent/model_metadata.py`
- `agent/models_dev.py`

### 设计目的

- 知道不同模型的上下文长度、推理特征、provider 兼容性

### Go 承接

- `internal/models` 当前有 profile、alias、本地发现
- 还没有完整的模型元数据与 provider-aware 上下文策略

## 6. Session / History / Search

### Python 知识点

- `hermes_state.py`
- SQLite + FTS5
- 历史检索、会话恢复、压缩分裂

### 设计目的

- 让 Agent 有“会话连续性”
- 让历史可查

### Go 承接

- `internal/store`
- 已迁到比较完整

### 为什么这么设计

- 这是轻量版最该优先保留的能力之一

## 7. Memory

### Python 知识点

- memory manager
- memory entries 注入系统提示

### 设计目的

- 给 Agent 提供长期稳定偏好和事实记忆

### Go 承接

- `internal/memory`
- `MEMORY.md / USER.md`
- recalled memory

### 为什么这么设计

- 文件记忆最适合轻量版

## 8. Context Compression

### Python 知识点

- `agent/context_compressor.py`
- 总结旧上下文，保留新上下文

### 设计目的

- 让长会话不因上下文爆炸而失控

### Go 承接

- `internal/contextengine`
- 规则摘要 + 可选 LLM summary

## 9. Tool Registry

### Python 知识点

- `tools/registry.py`
- `model_tools.py`
- 动态导入工具并注册

### 设计目的

- 工具生态统一暴露给 Agent

### Go 承接

- `internal/tools`
- 启动期注册 + 扩展注册

### 为什么这么设计

- Go 不适合复制 Python 的导入时副作用模式

## 10. Delegate / Multi-Agent

### Python 知识点

- `tools/delegate_tool.py`
- 父 agent 委派、子 agent 执行、父 agent 汇总

### 设计目的

- 把复杂任务切成多个可控子任务

### Go 承接

- `internal/multiagent`
- `internal/app` child runtime

### 当前状态

- 主干已迁
- 递归子代理还没做完整

## 11. MCP / Plugin / Skill

### Python 知识点

- `tools/mcp_tool.py`
- skills / plugins / hub

### 设计目的

- 把外部能力接进 Agent

### Go 承接

- `internal/extensions`
- plugin.yaml / skill.yaml / MCP stdio + http

### 为什么这么设计

- 轻量版更重视“受控接入”，不是“任意动态热加载”

## 12. Browser / Code Execution

### Python 知识点

- `tools/browser_tool.py`
- `tools/code_execution_tool.py`

### 设计目的

- 给 Agent 强执行能力

### Go 承接

- 当前没有完整迁移
- 只保留 `internal/execution` 的受控执行链

### 为什么这么设计

- 这两条风险最高
- 不适合在轻量版里先放开

## 13. Gateway 平台生态

### Python 知识点

- `gateway/run.py`
- `gateway/platforms/*`

### 设计目的

- 让 Agent 进入真实消息平台

### Go 承接

- `internal/gateway`
- webhook / telegram / slack

### 当前策略

- 先做最小可用平台
- 后续再做插件化平台适配

## 14. Cron / Scheduler / Delivery

### Python 知识点

- `cron/`
- `cronjob` 工具
- 平台投递

### 设计目的

- 让 Agent 具备自动化运行能力

### Go 承接

- 当前还未完整迁入

### 为什么延期

- 轻量版优先交互主链，不先做长生命周期调度系统

## 15. ACP / 编辑器集成

### Python 知识点

- `acp_adapter/`

### 设计目的

- 让 IDE / 编辑器成为 Agent 入口

### Go 承接

- 当前未迁

## 16. Batch Runner / RL / Trajectory

### Python 知识点

- `batch_runner.py`
- RL / Atropos 环境
- `trajectory.py`

### 设计目的

- 生成训练数据
- 进行批量评测和研究实验

### Go 承接

- 当前仅有 multiagent trace 审计主干
- 未形成完整 batch / RL / trajectory 产出链

## 17. 总结：为什么 Go 版不追求完全 1:1

Go 版当前的目标不是“复制 Python 的所有自由度”，而是：

- 保留 Agent 的核心工作原理
- 把高频能力变成稳定主干
- 把高风险动态能力延后
- 把系统变成更容易维护和理解的形态

这也是为什么 Go 版优先保留：

- chat 主链
- session/history/search
- memory/context
- tools
- extensions
- multiagent
- execution governance

而把完整 browser、完整 code execution、完整 cron、完整 RL 等能力，放到后续切片迁移。
