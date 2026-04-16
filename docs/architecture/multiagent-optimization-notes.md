# Go 多 Agent 优化说明

这份说明不是“空谈论文”，而是把近期多 Agent / 记忆 / 工具调用方向里对 Go 版本最有价值的结论，直接映射到当前工程。

## 这轮已经落地的方向

- child runtime 已经优先尝试原生 tool-calling，再回退到 JSON 协议循环
- child trace 已经落到 `multiagent_traces` 表，可以单独查询
- parent session / child session / aggregate 回写已经串起来
- child 已经能在 `allowed_tools` 白名单内真实执行受限工具

这些点和近两年的主流趋势是对齐的：规划、执行、记忆、轨迹要分层，不能把所有状态都压在一轮自然语言里。

## 论文与理论带来的 5 个改进方向

### 1. 继续强化“规划层”和“执行层”分离

原因：
2025 年 1 月 10 日提交到 arXiv 的《Multi-Agent Collaboration Mechanisms: A Survey of LLMs》把多 Agent 协作拆成 actor、structure、strategy、protocol 等维度。对 Hermes Go 最直接的启发是，不要把 planner、dispatcher、worker runtime、aggregator 混成一坨。

对当前 Go 的建议：

- 保持 `internal/multiagent` 独立
- planner 只产 plan，不做执行
- runtime 只做 child loop，不直接做高层决策
- aggregator 负责冲突归并和结果压缩

来源：
- https://arxiv.org/abs/2501.06322

### 2. 记忆要从“堆历史”继续升级成“检索式注入”

原因：
2024 年 4 月 21 日提交到 arXiv 的《A Survey on the Memory Mechanism of Large Language Model based Agents》强调，LLM Agent 的记忆不只是长期存储，还包括召回、更新、压缩、遗忘和与任务的匹配。

对当前 Go 的建议：

- 继续保留 `history_window + summary + memory`
- 下一步把 child context 从“只取最近 N 条”升级成“recent + summary + retrieved memory + retrieved history”
- 搜索结果最好参与 child context 组装，而不是只作为工具回包

来源：
- https://arxiv.org/abs/2404.13501

### 3. 多 Agent 要把“结构化轨迹”当一等公民

原因：
2025 年 3 月 13 日提交到 arXiv 的《LLMs Working in Harmony: A Survey on the Technological Aspects of Building Effective LLM-Based Multi Agent Systems》把 architecture、memory、planning、framework 作为多 Agent 的关键维度。工程上最值钱的一条，是轨迹必须结构化，否则恢复、审计、评估都很难做。

对当前 Go 的建议：

- 保持 `multiagent_traces` 作为独立表
- 下一步补 replay / recovery
- 再补 trace-based failure analysis 和 tool reliability ranking

来源：
- https://arxiv.org/abs/2504.01963

### 4. 工具调用优先走原生协议，不要长期停留在提示词约定

原因：
当前主流 OpenAI-compatible 生态已经把工具调用做成了标准 API 能力。相比“模型按提示词返回 JSON”，原生 tool-calling 更稳，也更利于把 assistant/tool 历史明确落库。

对当前 Go 的建议：

- 继续优先 `tool_calls`
- JSON 协议保留为兼容回退
- 后续要把 tool schema 做得更精确，而不是现在这种通用字符串参数
- 要把 child tool messages 更完整地回接到 parent summary 和审计链

来源：
- OpenAI API Reference, Chat Completions / Tools:
  https://developers.openai.com/api/reference/overview

## 我对 Hermes Go 的工程化判断

如果只继续“加功能”，Go 版会越来越像 Python 版，但也会越来越难控。  
真正值得坚持的是下面这 4 条：

- 安全工具优先，而不是先开放高风险 delegated runtime
- 结构化轨迹优先，而不是只存自然语言总结
- 检索式上下文优先，而不是盲目扩大窗口
- 分层编排优先，而不是把 parent/child planner/runtime/summary 写死在一条链上

## 下一步最值钱的落地方向

1. 做 `trace replay / recovery`
2. 做 child 的“检索式上下文组装”
3. 做更精确的 tool schema 与参数校验
4. 做 gateway 到 multiagent 的真实路由
5. 做受控递归子代理和深度限制
