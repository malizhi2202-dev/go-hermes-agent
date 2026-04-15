# Agent 常见问题与 Hermes 工程对应拆解

这份文档回答一个核心问题：

“一个现代 Agent 系统通常会遇到哪些共性问题，这些问题在 Hermes 工程里分别落在哪些模块上？”

为了便于理解，下面每个主题都按 4 个维度拆解：

1. 这个问题本身是什么
2. 为什么 Agent 一定会遇到它
3. Hermes 工程里对应哪些模块
4. Python 原版与 Go 版当前各承担到哪一步

---

## 1. 单 Agent 主循环

### 这是什么

最基本的 Agent 问题是：

- 收到用户输入
- 组织 system prompt / history / tools
- 调模型
- 判断是否需要调用工具
- 继续循环直到拿到最终答案

### 为什么一定会遇到

因为没有这条主循环，就没有 Agent，只剩下普通的一次性聊天接口。

### Hermes 对应模块

- Python
  - `run_agent.py`
  - `model_tools.py`
  - `tools/registry.py`

- Go
  - `go/internal/app`
  - `go/internal/llm`
  - `go/internal/api`

### 工程解释

Python 版的 `AIAgent.run_conversation()` 是 Hermes 最核心的主循环。  
它负责把消息、工具、模型调用和结果回填串起来。

Go 版目前没有完整复刻 Python 的多轮 tool-calling agent loop，而是先实现了“安全主干”：

- `App.Chat()`
- `LLM.Chat()`
- 会话落库
- 审计记录

也就是说，Go 版先搭好了 Agent 的运行边界，但还不是 Python 那种完整的长循环编排器。

---

## 2. 多 Agent / 子代理 / 委派

### 这是什么

多 Agent 的典型问题是：

- 一个主 Agent 是否要把某个任务拆给子 Agent
- 子 Agent 能不能并行
- 子 Agent 能看到多少父上下文
- 子 Agent 是否共享工具、记忆、权限

### 为什么一定会遇到

当任务开始变复杂，例如：

- 代码分析
- 并行搜索
- 多模块修改
- 一个 Agent 既要“主控”，又要“分工”

这时就会自然出现 delegation。

### Hermes 对应模块

- Python
  - `tools/delegate_tool.py`
  - `run_agent.py`
  - `model_tools.py`

- Go
  - 当前未完整迁移
  - 只保留了更安全的扩展与执行边界骨架

### 工程解释

Hermes 的多 Agent 不是“多个对等 agent 自由聊天”，而是更典型的：

- 父 Agent 发起委派
- 子 Agent 拿到一个更小、更聚焦的问题
- 子 Agent 用自己的 iteration budget、task_id、toolset 去做事
- 最后返回摘要给父 Agent

Python 版在 `tools/delegate_tool.py` 里把这个机制做得比较清楚：

- 子 Agent 默认没有父历史
- 子 Agent 有独立任务上下文
- 子 Agent 的工具集会被裁剪
- 明确屏蔽递归委派、共享 memory 写入、用户交互等高风险能力

这说明 Hermes 对“多 Agent”的工程理解是：

不是把所有 agent 放开，而是把它做成“有边界的受控委派”。

Go 版目前还没有完整迁移这条线，这也是有意的，因为多 Agent 一旦和高权限工具结合，安全复杂度会上升很多。

---

## 3. 记忆力：短期记忆、长期记忆、回忆

### 这是什么

“记忆力”不是单一概念，通常至少分三层：

1. 短期记忆
   - 当前会话历史

2. 中期记忆
   - 会话内压缩后的摘要

3. 长期记忆
   - 跨会话保留的用户偏好、项目背景、历史事实

### 为什么一定会遇到

因为模型的上下文窗口有限，Agent 又需要：

- 记住刚刚做了什么
- 不忘掉很久以前的重要信息
- 在有限 token 内维持连续性

### Hermes 对应模块

- Python
  - `hermes_state.py`
  - `agent/memory_manager.py`
  - `tools/memory_tool.py`
  - memory provider plugins

- Go
  - `go/internal/store`
  - `go/internal/extensions`
  - 还没有完整迁移 Python 的 memory provider 体系

### 工程解释

Hermes 把“记忆力”拆成了两个正交系统：

#### A. 会话记忆

这是最基础的一层，由 `hermes_state.py` 负责：

- 保存 session
- 保存 messages
- 支持 FTS 搜索
- 支持恢复历史会话

这本质上是“对话历史持久化”，不是语义型长期记忆。

#### B. 记忆提供器

这是更高级的一层，由 `agent/memory_manager.py` 负责统一编排。

它的特点很关键：

- 内建 memory provider 永远存在
- 最多只允许一个外部 memory provider
- 所有 provider 都通过统一管理器接入

这说明 Hermes 的记忆设计不是“谁都能塞一个记忆库进来”，而是：

- 有总线
- 有接入点
- 有上限
- 有隔离

Go 版目前先迁了“会话历史”和“搜索”，即短期记忆底座；  
但 Python 的完整 memory provider 体系还没有迁完。

---

## 4. 上下文窗口与上下文压缩

### 这是什么

Agent 很快会碰到一个常见问题：

- 对话越来越长
- token 越来越多
- 模型上下文会爆

所以必须做：

- 截断
- 摘要
- 压缩
- 分段会话

### 为什么一定会遇到

因为真实任务不是 1 轮，而是多轮、多工具、多输出的持续会话。

### Hermes 对应模块

- Python
  - `agent/context_compressor.py`
  - `agent/prompt_builder.py`
  - `agent/model_metadata.py`
  - `agent/trajectory.py`

- Go
  - 当前还没有完整复刻 Python 的 context compressor
  - 只先落了 session/history/search 底层能力

### 工程解释

Hermes 的上下文压缩不是简单“砍掉前面的消息”，而是：

- 保护头部
- 保护尾部
- 压缩中间
- 对旧 tool output 做预裁剪
- 维护上一次摘要
- 必要时做 session split

这代表 Hermes 对“上下文问题”的工程处理是成熟的：

- 不只考虑 token 数
- 也考虑历史信息完整性
- 也考虑多次压缩后的连续性

Go 版现在还没有把这整套压缩逻辑迁过去，所以当前 Go 的多轮上下文能力明显弱于 Python。

---

## 5. 工具使用（Tool Use）

### 这是什么

Agent 常见的核心问题之一是：

- 模型怎么知道有哪些工具
- 模型调用工具后，结果怎么回到对话
- 工具怎么分组、过滤、限权

### 为什么一定会遇到

因为现代 Agent 的核心竞争力往往不是“能聊天”，而是“能用工具做事”。

### Hermes 对应模块

- Python
  - `model_tools.py`
  - `tools/registry.py`
  - `toolsets.py`
  - `tools/*.py`

- Go
  - `go/internal/tools`
  - `go/internal/extensions`
  - `go/internal/execution`

### 工程解释

Hermes 把工具系统做成了三层：

1. registry
   - 负责注册和分发

2. toolset
   - 负责按功能分组与启停

3. 具体工具实现
   - terminal、file、web、memory、delegate、browser、MCP 等

这套结构的关键价值在于：

- 工具不是散的
- 可以做权限边界
- 可以动态启停
- 可以跨 CLI / Gateway / Batch 复用

Go 版沿用了这个思路，但做了更安全的收缩：

- 内置白名单工具
- 扩展工具单独进入 `extensions`
- 高风险命令单独进入 `execution`

---

## 6. Session / 会话恢复 / 会话分支

### 这是什么

Agent 常见问题：

- 用户离开再回来，能不能继续上次会话
- 同一任务能不能分叉出 branch
- 消息平台里不同 chat / user / thread 要不要隔离

### 为什么一定会遇到

因为真实生产环境不是一次性问答，而是长期连续对话。

### Hermes 对应模块

- Python
  - `hermes_state.py`
  - `gateway/session.py`
  - `gateway/run.py`

- Go
  - `go/internal/store`
  - `go/internal/gateway/telegram.go`
  - `go/internal/api/server.go`

### 工程解释

Hermes 的 session 不只是“聊天记录”：

- 它还承担用户隔离
- 平台隔离
- thread 隔离
- 恢复和分支

尤其在 Gateway 里，这点特别重要。

Python 版通过 `gateway/session.py` 和 `gateway/run.py`，把：

- 平台来源
- chat_id
- user_id
- thread_id
- session_key

这些都做成显式上下文。

Go 版已经迁了其中最关键的一步：

- Telegram 使用 `chat_id + user_id` 构造最小会话主体

这其实就是“会话隔离”能力的一部分。

---

## 7. Platform Context：同一个 Agent 在不同平台怎么保持正确行为

### 这是什么

一个 Agent 同时跑在：

- CLI
- Telegram
- Slack
- Discord

时，会遇到同一个问题：

- 模型是否知道自己现在在哪个平台
- 是否知道回复该发回哪里
- 是否知道不同平台的约束

### 为什么一定会遇到

因为多平台接入不是简单换一个 webhook，而是运行上下文变了。

### Hermes 对应模块

- Python
  - `gateway/session.py`
  - `gateway/session_context.py`
  - `gateway/run.py`
  - `agent/prompt_builder.py`

- Go
  - `go/internal/gateway`
  - `go/internal/api`

### 工程解释

Hermes 的做法不是把“平台”藏在业务代码里，而是显式建模：

- `SessionSource`
- `SessionContext`
- 平台上下文注入

也就是说，平台本身是 Agent prompt 的一部分，而不是仅仅是传输层。

这对多平台 Agent 很关键，因为：

- 同一条“发消息”行为在 CLI / Telegram / Slack 意义完全不同

Go 版当前只迁了 webhook + Telegram 的最小链路，还没有迁完整的平台上下文注入体系。

---

## 8. 权限、审批与高风险行为

### 这是什么

Agent 一旦能执行：

- shell
- 写文件
- 浏览器操作
- 调子代理

就会遇到典型问题：

- 哪些动作要审批
- 审批一次还是整个 session
- 怎么阻止危险命令

### 为什么一定会遇到

因为 Agent 一旦具备行动能力，就会产生权限风险。

### Hermes 对应模块

- Python
  - `tools/approval.py`
  - `tools/terminal_tool.py`
  - `tools/code_execution_tool.py`
  - `gateway/run.py`
  - `hermes_cli/callbacks.py`

- Go
  - `go/internal/execution`
  - `go/internal/tools`
  - `go/internal/api`
  - `go/internal/store` 的 audit

### 工程解释

Hermes 对安全不是“最后再加”，而是深嵌在工程里：

- 危险模式识别
- per-session approval
- always / session / deny
- gateway 侧审批回路

Go 版则把这条链先收缩成更小、更稳的形态：

- 默认禁用动态执行
- 命令白名单
- 参数白名单
- 输出限制
- 审计日志

这说明 Go 版优先级是“先别出事”，再考虑把 Python 的灵活性补回来。

---

## 9. Skills、Plugins、MCP：动态扩展性

### 这是什么

Agent 系统常见的另一个问题是：

- 怎么接入外部能力
- 怎么让能力不是写死在主程序里
- 怎么做到可扩展但不失控

### 为什么一定会遇到

因为一个 Agent 平台不可能把所有能力都写死在内核里。

### Hermes 对应模块

- Python
  - `tools/skills_tool.py`
  - `agent/skill_utils.py`
  - `hermes_cli/plugins.py`
  - `tools/mcp_tool.py`

- Go
  - `go/internal/extensions`

### 工程解释

Hermes 的动态扩展其实有三条线：

1. Skills
   - 偏 prompt / procedure / capability packaging

2. Plugins
   - 偏工程扩展和工具注入

3. MCP
   - 偏外部工具协议接入

Python 版更自由，支持运行时发现与较强动态加载。  
Go 版则把它们统一收敛进 `internal/extensions`：

- plugin.yaml
- SKILL.md + skill.yaml
- mcp_servers

再配合：

- 启停状态
- hash 校验
- 审计
- 受控执行

这就是“动态扩展性”和“工程可治理性”的平衡。

---

## 10. 记忆 vs 会话历史：最容易混淆的点

### 常见误解

很多人会把：

- 会话历史
- 长期记忆
- session search
- MEMORY.md

全混成“记忆力”。

### Hermes 里的正确拆法

#### 1. 会话历史

是谁说过什么。  
对应：

- `hermes_state.py`
- `go/internal/store`

#### 2. Session Search

是在历史消息里做检索。  
对应：

- `tools/session_search_tool.py`
- `go/internal/store` FTS

#### 3. 长期记忆

是抽取出的、跨 session 的结构化背景。  
对应：

- `agent/memory_manager.py`
- `tools/memory_tool.py`
- memory provider plugin

#### 4. Prompt 内 recalled memory

是当前 turn 调用时临时注入的“回忆块”，不是用户新输入。  
对应：

- `build_memory_context_block()`

这几层是不同问题，不应该混着理解。

---

## 11. 观测、轨迹、审计

### 这是什么

Agent 系统常见问题：

- 出错时怎么复盘
- 用户问“刚才做了什么”时怎么回答
- 如何定位模型决策、工具调用、上下文压缩问题

### Hermes 对应模块

- Python
  - `agent/trajectory.py`
  - `hermes_state.py`
  - gateway usage / audit / logs

- Go
  - `go/internal/store`
  - `audit_log`
  - `extension_states`

### 工程解释

Hermes 对观测分成两种：

1. 行为轨迹
   - 更偏 Agent 研究 / 回放 / 训练

2. 审计日志
   - 更偏生产安全 / 合规 / 运维

Go 版目前把“审计”先做好了，轨迹回放能力还没有像 Python 那样完整。

---

## 12. 为什么 Go 版没有一次性把所有 Agent 能力都搬过来

### 原因一：Python 版的能力很强，但很多是高动态、高权限的

例如：

- delegate
- code execution
- browser
- plugin runtime hook
- MCP 动态注入

### 原因二：Go 迁移的第一目标不是“功能数量最大化”，而是“安全主干先成立”

所以 Go 版优先迁移的是：

- auth
- store
- api
- gateway
- search
- audit
- controlled tools
- controlled extensions

### 原因三：Agent 工程最怕“看起来全有，实际上不可控”

Hermes Go 当前的思路很清楚：

- 先把骨架做稳
- 再把高动态能力一条条放回来

---

## 13. 一句话理解 Hermes

如果把 Hermes 当成一个 Agent 工程平台，而不是一个聊天脚本，可以这样理解：

- `run_agent.py`
  - 是“大脑主循环”

- `model_tools.py + tools/registry.py`
  - 是“工具总线”

- `hermes_state.py`
  - 是“会话与历史底座”

- `agent/memory_manager.py`
  - 是“记忆编排层”

- `agent/context_compressor.py`
  - 是“上下文续航系统”

- `tools/delegate_tool.py`
  - 是“多 Agent 委派机制”

- `gateway/*`
  - 是“多平台运行层”

- `go/internal/*`
  - 是“把这些能力重构成更安全、更可治理形态的迁移主干”

---

## 14. 给你一个最实用的理解框架

以后再看 Hermes，建议按下面这个顺序理解：

1. 先看单 Agent 主循环
2. 再看工具系统
3. 再看 session/history/search
4. 再看 memory 和 context compression
5. 再看 gateway 多平台
6. 最后看 delegate / plugin / MCP / execution 这些高动态能力

这样不会乱。

如果反过来先看 plugin、delegate、MCP，很容易把工程理解成“很多功能堆一起”，看不清主线。
